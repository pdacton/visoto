// Package dcatsparql harvests catalogues that publish DCAT-AP over a SPARQL
// endpoint, such as data.europa.eu.
//
// This is the cheap case: the source already speaks the vocabulary the pipeline
// wants, so the adapter's job is paging, blank-node skolemization and extracting
// the distribution work items — not mapping.
package dcatsparql

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/logger"
	"hutzli.org/visoto/internal/pipeline/rdf"
	"hutzli.org/visoto/internal/pipeline/source"
	"hutzli.org/visoto/internal/pipeline/sparqlio"
)

// TypeName is the value of `type` in [[pipeline.sources]] that selects this adapter.
const TypeName = "dcat-sparql"

func init() {
	source.Register(TypeName, New)
}

// Source harvests DCAT-AP from a SPARQL endpoint.
type Source struct {
	name      string
	client    *sparqlio.Client
	limiter   *source.Limiter
	minter    *rdf.Minter
	pageSize  int
	batchSize int
	maxRetry  int
	log       *slog.Logger
}

// New builds the adapter from its configuration.
func New(cfg config.PipelineSource, opts source.Options) (source.Source, error) {
	if strings.TrimSpace(cfg.URL) == "" {
		return nil, errors.New("dcat-sparql source needs a url")
	}
	interval, err := cfg.ParseRateLimit()
	if err != nil {
		return nil, err
	}

	clientOpts := []sparqlio.Option{sparqlio.WithUserAgent(opts.UserAgent)}
	switch {
	case cfg.AccessToken != "":
		clientOpts = append(clientOpts, sparqlio.WithBearer(cfg.AccessToken))
	case cfg.Username != "":
		clientOpts = append(clientOpts, sparqlio.WithBasicAuth(cfg.Username, cfg.Password))
	}

	minter := opts.Minter
	if minter == nil {
		minter = rdf.NewMinter("")
	}

	return &Source{
		name:      cfg.Name,
		client:    sparqlio.NewClient(cfg.URL, clientOpts...),
		limiter:   source.NewLimiter(interval),
		minter:    minter,
		pageSize:  cfg.PageSize,
		batchSize: defaultBatchSize,
		maxRetry:  defaultMaxRetry,
		log:       logger.Get().With(slog.String("source", cfg.Name), slog.String("type", TypeName)),
	}, nil
}

const (
	// defaultBatchSize is how many datasets are described per query. Large enough
	// that paging overhead is negligible, small enough that one slow response
	// does not blow the endpoint's result limit.
	defaultBatchSize = 50
	defaultMaxRetry  = 4
)

// Name returns the configured source name.
func (s *Source) Name() string { return s.name }

// Harvest pages through the catalogue and emits one Record per dataset.
func (s *Source) Harvest(ctx context.Context, since time.Time, emit source.EmitFunc) error {
	offset := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		iris, err := s.listDatasets(ctx, since, offset)
		if err != nil {
			return fmt.Errorf("list datasets at offset %d: %w", offset, err)
		}
		if len(iris) == 0 {
			return nil
		}
		s.log.Debug("listed datasets", slog.Int("count", len(iris)), slog.Int("offset", offset))

		for start := 0; start < len(iris); start += s.batchSize {
			end := min(start+s.batchSize, len(iris))
			records, err := s.describeBatch(ctx, iris[start:end])
			if err != nil {
				return fmt.Errorf("describe datasets: %w", err)
			}
			for _, r := range records {
				if err := emit(r); err != nil {
					return err
				}
			}
		}

		// A short page means the catalogue is exhausted. Endpoints that silently
		// cap results below the requested LIMIT end the harvest here too, which
		// is the safe direction: a truncated harvest, not an endless loop.
		if len(iris) < s.pageSize {
			return nil
		}
		offset += len(iris)
	}
}

// listDatasets returns one page of dataset IRIs, ordered so paging is stable.
func (s *Source) listDatasets(ctx context.Context, since time.Time, offset int) ([]string, error) {
	var b strings.Builder
	b.WriteString("PREFIX dcat: <" + rdf.NSDCAT + ">\n")
	b.WriteString("PREFIX dct: <" + rdf.NSDCT + ">\n")
	b.WriteString("SELECT DISTINCT ?dataset WHERE {\n  ?dataset a dcat:Dataset .\n")
	if !since.IsZero() {
		// Datasets carrying no dct:modified are invisible to an incremental
		// harvest. That is the documented cost of incrementality (R-SRC-3); a
		// periodic full harvest is what catches them.
		b.WriteString("  ?dataset dct:modified ?modified .\n")
		b.WriteString(fmt.Sprintf("  FILTER(?modified >= %q^^<%sdateTime>)\n",
			since.UTC().Format(time.RFC3339), rdf.NSXSD))
	}
	b.WriteString("}\nORDER BY ?dataset\n")
	b.WriteString("LIMIT " + strconv.Itoa(s.pageSize) + " OFFSET " + strconv.Itoa(offset))

	res, err := s.query(ctx, b.String())
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(res.Bindings))
	for _, row := range res.Bindings {
		if iri := row.IRI("dataset"); iri != "" {
			out = append(out, iri)
		}
	}
	return out, nil
}

// describeBatch fetches the statements of the given datasets plus the nodes
// DCAT-AP hangs off them, and groups the result into records.
func (s *Source) describeBatch(ctx context.Context, iris []string) ([]source.Record, error) {
	values := make([]string, 0, len(iris))
	for _, iri := range iris {
		safe, ok := sparqlio.EscapeIRI(iri)
		if !ok {
			s.log.Warn("skipping dataset with malformed IRI", slog.String("iri", iri))
			continue
		}
		values = append(values, "<"+safe+">")
	}
	if len(values) == 0 {
		return nil, nil
	}

	query := fmt.Sprintf(`PREFIX dcat: <%s>
PREFIX dct: <%s>
SELECT ?dataset ?s ?p ?o WHERE {
  VALUES ?dataset { %s }
  {
    ?dataset ?p ?o .
    BIND(?dataset AS ?s)
  } UNION {
    ?dataset dcat:distribution ?s .
    ?s ?p ?o .
  } UNION {
    ?dataset dct:publisher ?s .
    ?s ?p ?o .
  } UNION {
    ?dataset dcat:contactPoint ?s .
    ?s ?p ?o .
  }
}`, rdf.NSDCAT, rdf.NSDCT, strings.Join(values, " "))

	res, err := s.query(ctx, query)
	if err != nil {
		return nil, err
	}
	return s.groupRecords(iris, res), nil
}

// groupRecords turns a flat solution sequence into one Record per dataset,
// preserving the order the datasets were requested in so runs are reproducible.
func (s *Source) groupRecords(order []string, res *sparqlio.Results) []source.Record {
	byDataset := make(map[string][]rdf.Quad, len(order))
	for _, row := range res.Bindings {
		ds := row.IRI("dataset")
		subj, okS := row.Term("s")
		pred, okP := row.Term("p")
		obj, okO := row.Term("o")
		if ds == "" || !okS || !okP || !okO {
			continue
		}
		q := rdf.NewQuad(subj, pred, obj, "")
		if !q.Valid() {
			continue
		}
		byDataset[ds] = append(byDataset[ds], q)
	}

	out := make([]source.Record, 0, len(byDataset))
	for _, ds := range order {
		quads, ok := byDataset[ds]
		if !ok {
			continue
		}
		quads = source.NewSkolemizer(s.minter, ds).Apply(quads)
		out = append(out, source.Record{
			DatasetIRI:    ds,
			Quads:         quads,
			Distributions: source.ExtractDistributions(ds, quads),
		})
	}
	return out
}

// query runs a SELECT, pacing and retrying it. Retries cover the transient
// failures a public endpoint produces under load; a 4xx other than 429 is a bug
// in the query and is returned immediately.
func (s *Source) query(ctx context.Context, q string) (*sparqlio.Results, error) {
	var lastErr error
	for attempt := 0; attempt < s.maxRetry; attempt++ {
		if err := s.limiter.Wait(ctx); err != nil {
			return nil, err
		}
		res, err := s.client.Select(ctx, q)
		if err == nil {
			return res, nil
		}
		lastErr = err

		var statusErr *sparqlio.StatusError
		if errors.As(err, &statusErr) && !statusErr.Retryable() {
			return nil, err
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		backoff := time.Duration(1<<attempt) * time.Second
		s.log.Warn("retrying sparql query",
			slog.Int("attempt", attempt+1),
			slog.Duration("backoff", backoff),
			slog.String("error", err.Error()))
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return nil, fmt.Errorf("after %d attempts: %w", s.maxRetry, lastErr)
}
