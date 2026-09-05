// Package ckan harvests CKAN catalogues, such as opendata.swiss, and maps them
// into DCAT-AP.
//
// This is the expensive case the Source interface exists for: CKAN's JSON is not
// DCAT, so the mapping lives here and nothing downstream learns that CKAN was
// ever involved. Where the mapping is lossy, fields are dropped rather than
// invented (R-SRC-4) — a wrong triple in the catalogue graph is worse than a
// missing one, because the whole point of the graph is to be trusted.
package ckan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/logger"
	"hutzli.org/visoto/internal/pipeline/rdf"
	"hutzli.org/visoto/internal/pipeline/source"
)

// TypeName is the value of `type` in [[pipeline.sources]] that selects this adapter.
const TypeName = "ckan"

func init() {
	source.Register(TypeName, New)
}

// Source harvests a CKAN catalogue through package_search.
type Source struct {
	name       string
	apiRoot    string
	datasetsAt string
	client     *http.Client
	limiter    *source.Limiter
	minter     *rdf.Minter
	userAgent  string
	token      string
	pageSize   int
	maxRetry   int
	log        *slog.Logger
}

// New builds the adapter from its configuration.
func New(cfg config.PipelineSource, opts source.Options) (source.Source, error) {
	root := strings.TrimSuffix(strings.TrimSpace(cfg.URL), "/")
	if root == "" {
		return nil, errors.New("ckan source needs a url")
	}
	interval, err := cfg.ParseRateLimit()
	if err != nil {
		return nil, err
	}

	// Dataset IRIs default to the portal's own page for the dataset, which is
	// what the portal itself publishes as the dataset identifier.
	datasetsAt := strings.TrimSuffix(strings.TrimSpace(cfg.DatasetIRIBase), "/")
	if datasetsAt == "" {
		datasetsAt = strings.TrimSuffix(strings.TrimSuffix(root, "/api/3/action"), "/api") + "/dataset"
	}

	minter := opts.Minter
	if minter == nil {
		minter = rdf.NewMinter("")
	}

	return &Source{
		name:       cfg.Name,
		apiRoot:    root,
		datasetsAt: datasetsAt,
		client:     &http.Client{Timeout: 120 * time.Second},
		limiter:    source.NewLimiter(interval),
		minter:     minter,
		userAgent:  opts.UserAgent,
		token:      cfg.AccessToken,
		pageSize:   cfg.PageSize,
		maxRetry:   defaultMaxRetry,
		log:        logger.Get().With(slog.String("source", cfg.Name), slog.String("type", TypeName)),
	}, nil
}

const (
	defaultMaxRetry = 4
	// maxCKANRows is CKAN's own ceiling on package_search rows. Asking for more
	// silently returns fewer, which would end paging early.
	maxCKANRows = 1000
)

// Name returns the configured source name.
func (s *Source) Name() string { return s.name }

// Harvest pages through package_search and emits one Record per dataset.
func (s *Source) Harvest(ctx context.Context, since time.Time, emit source.EmitFunc) error {
	rows := min(s.pageSize, maxCKANRows)
	start := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		page, err := s.search(ctx, since, start, rows)
		if err != nil {
			return fmt.Errorf("package_search at start=%d: %w", start, err)
		}
		if len(page.Results) == 0 {
			return nil
		}
		s.log.Debug("fetched ckan page",
			slog.Int("count", len(page.Results)),
			slog.Int("start", start),
			slog.Int("total", page.Count))

		for _, pkg := range page.Results {
			rec, ok := s.toRecord(pkg)
			if !ok {
				continue
			}
			if err := emit(rec); err != nil {
				return err
			}
		}

		start += len(page.Results)
		if start >= page.Count {
			return nil
		}
	}
}

// search runs one page of package_search.
func (s *Source) search(ctx context.Context, since time.Time, start, rows int) (*searchResult, error) {
	q := url.Values{}
	q.Set("rows", strconv.Itoa(rows))
	q.Set("start", strconv.Itoa(start))
	// Sorting by a stable key keeps paging consistent while the catalogue is
	// being edited underneath us.
	q.Set("sort", "metadata_modified asc")
	if !since.IsZero() {
		q.Set("fq", fmt.Sprintf("metadata_modified:[%s TO *]", since.UTC().Format("2006-01-02T15:04:05Z")))
	}
	endpoint := s.apiRoot + "/package_search?" + q.Encode()

	body, err := s.get(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Success bool         `json:"success"`
		Result  searchResult `json:"result"`
		Error   any          `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("parse package_search response: %w", err)
	}
	if !envelope.Success {
		return nil, fmt.Errorf("package_search reported failure: %v", envelope.Error)
	}
	return &envelope.Result, nil
}

// get fetches a URL, pacing and retrying it.
func (s *Source) get(ctx context.Context, endpoint string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < s.maxRetry; attempt++ {
		if err := s.limiter.Wait(ctx); err != nil {
			return nil, err
		}
		body, retryable, err := s.getOnce(ctx, endpoint)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retryable || ctx.Err() != nil {
			return nil, err
		}
		backoff := time.Duration(1<<attempt) * time.Second
		s.log.Warn("retrying ckan request",
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

func (s *Source) getOnce(ctx context.Context, endpoint string) ([]byte, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", s.userAgent)
	if s.token != "" {
		req.Header.Set("Authorization", s.token)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, true, err // transport failures are worth another attempt
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, true, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		return nil, retryable, fmt.Errorf("ckan returned HTTP %d for %s", resp.StatusCode, endpoint)
	}
	return body, false, nil
}

const maxResponseBytes = 64 << 20
