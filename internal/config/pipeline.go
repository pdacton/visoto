package config

import (
	"fmt"
	"strings"
	"time"
)

// PipelineConfig configures the harvesting pipeline (cmd/visoto-harvest). It is
// read from the same visoto.config as the web app, but nothing in the web app
// depends on it: an absent [pipeline] block, or Enabled = false, leaves the
// server unchanged (R-CFG-1).
type PipelineConfig struct {
	Enabled bool `toml:"enabled"`

	// TargetEndpoint is the slug of the [[application.sparqlEndpoints]] entry the
	// pipeline writes into. A slug rather than a URL, so credentials and the
	// endpoint definition stay in one place.
	TargetEndpoint string `toml:"target_endpoint"`

	WorkDir string `toml:"work_dir"` // blobs, staged N-Quads
	StateDB string `toml:"state_db"` // SQLite job state

	Workers          int   `toml:"workers"`
	MaxDownloadBytes int64 `toml:"max_download_bytes"`
	SampleRows       int   `toml:"sample_rows"`
	TopK             int   `toml:"top_k"`

	// Loader selects how quads reach the triplestore: "bulk-file" stages N-Quads
	// for a QLever re-index, "sparql-update" writes them over the protocol.
	// Bulk-file is the default because heavy UPDATE volume degrades QLever query
	// performance (R-LOD-2).
	Loader string `toml:"loader"`

	// UpdateBatchQuads caps how many quads go into one INSERT DATA request.
	UpdateBatchQuads int `toml:"update_batch_quads"`

	EmitCSVW  bool `toml:"emit_csvw"`
	EmitSHACL bool `toml:"emit_shacl"`
	EmitVOID  bool `toml:"emit_void"`

	KeepRuns int `toml:"keep_runs"`

	// BaseIRI roots every minted IRI. Freeze it before the first load: rewriting
	// minted IRIs afterwards means re-minting the whole store (D2).
	BaseIRI   string `toml:"base_iri"`
	UserAgent string `toml:"user_agent"`

	Sources []PipelineSource `toml:"sources"`
}

// PipelineSource configures one catalogue adapter. Type selects the registered
// adapter; every other field is interpreted by that adapter.
type PipelineSource struct {
	Name    string `toml:"name"` // stable slug, used in graph IRIs and state
	Type    string `toml:"type"` // registered adapter type, e.g. "dcat-sparql"
	URL     string `toml:"url"`  // endpoint or API root
	Enabled bool   `toml:"enabled"`

	// RateLimit is the ceiling on requests to this source, as "N/s" or "N/m".
	// Politeness is not optional: a naive worker pool gets the harvester blocked
	// (R-FET-5).
	RateLimit string `toml:"rate_limit"`

	PageSize int `toml:"page_size"`

	// DatasetIRIBase overrides how an adapter derives dataset IRIs from records
	// that carry only an identifier, as CKAN does.
	DatasetIRIBase string `toml:"dataset_iri_base"`

	// Credentials, if the source needs them. json:"-" for the same reason as on
	// SparqlEndpoint: this struct must never reach a template dump.
	AccessToken string `toml:"access_token" json:"-"`
	Username    string `toml:"username"     json:"-"`
	Password    string `toml:"password"     json:"-"`
}

// Pipeline defaults. Every one of these is overridable; they are set so that a
// [pipeline] block naming only sources is already runnable.
const (
	defaultPipelineWorkers          = 8
	defaultPipelineMaxDownloadBytes = 512 << 20
	defaultPipelineSampleRows       = 200_000
	defaultPipelineTopK             = 10
	defaultPipelineKeepRuns         = 3
	defaultPipelineWorkDir          = "./pipeline/work"
	defaultPipelineStateDB          = "./pipeline/state.sqlite"
	defaultPipelineLoader           = "bulk-file"
	defaultPipelineUpdateBatch      = 5000
	defaultPipelinePageSize         = 500
	defaultPipelineUserAgent        = "visoto-harvest/1.0 (+https://visoto.hutzli.org)"
)

// LoaderBulkFile and LoaderSparqlUpdate are the recognized loader names.
const (
	LoaderBulkFile     = "bulk-file"
	LoaderSparqlUpdate = "sparql-update"
)

// applyDefaults fills unset fields. Called from Load before validation so that
// validation sees the values the pipeline will actually run with.
func (p *PipelineConfig) applyDefaults() {
	if p.Workers <= 0 {
		p.Workers = defaultPipelineWorkers
	}
	if p.MaxDownloadBytes <= 0 {
		p.MaxDownloadBytes = defaultPipelineMaxDownloadBytes
	}
	if p.SampleRows <= 0 {
		p.SampleRows = defaultPipelineSampleRows
	}
	if p.TopK <= 0 {
		p.TopK = defaultPipelineTopK
	}
	if p.KeepRuns <= 0 {
		p.KeepRuns = defaultPipelineKeepRuns
	}
	if strings.TrimSpace(p.WorkDir) == "" {
		p.WorkDir = defaultPipelineWorkDir
	}
	if strings.TrimSpace(p.StateDB) == "" {
		p.StateDB = defaultPipelineStateDB
	}
	if strings.TrimSpace(p.Loader) == "" {
		p.Loader = defaultPipelineLoader
	}
	if p.UpdateBatchQuads <= 0 {
		p.UpdateBatchQuads = defaultPipelineUpdateBatch
	}
	if strings.TrimSpace(p.UserAgent) == "" {
		p.UserAgent = defaultPipelineUserAgent
	}
	for i := range p.Sources {
		if p.Sources[i].PageSize <= 0 {
			p.Sources[i].PageSize = defaultPipelinePageSize
		}
	}
}

// validate rejects a pipeline config that would fail confusingly at run time.
// It is only enforced when the pipeline is enabled, so a web-only deployment is
// never blocked by a half-written [pipeline] block.
func (p *PipelineConfig) validate() error {
	if !p.Enabled {
		return nil
	}
	if p.Loader != LoaderBulkFile && p.Loader != LoaderSparqlUpdate {
		return fmt.Errorf("loader %q: must be %q or %q", p.Loader, LoaderBulkFile, LoaderSparqlUpdate)
	}
	if len(p.Sources) == 0 {
		return fmt.Errorf("no [[pipeline.sources]] configured")
	}
	seen := make(map[string]bool, len(p.Sources))
	for i, s := range p.Sources {
		name := strings.TrimSpace(s.Name)
		if name == "" {
			return fmt.Errorf("pipeline source #%d has no name", i+1)
		}
		// The name reaches graph IRIs and state keys, so a duplicate would make
		// two sources overwrite each other's catalogue graph.
		if seen[name] {
			return fmt.Errorf("duplicate pipeline source name %q", name)
		}
		seen[name] = true
		if strings.TrimSpace(s.Type) == "" {
			return fmt.Errorf("pipeline source %q has no type", name)
		}
		if strings.TrimSpace(s.URL) == "" {
			return fmt.Errorf("pipeline source %q has no url", name)
		}
		if _, err := s.ParseRateLimit(); err != nil {
			return fmt.Errorf("pipeline source %q: %w", name, err)
		}
	}
	return nil
}

// EnabledSources returns the sources that are switched on, in config order.
func (p *PipelineConfig) EnabledSources() []PipelineSource {
	out := make([]PipelineSource, 0, len(p.Sources))
	for _, s := range p.Sources {
		if s.Enabled {
			out = append(out, s)
		}
	}
	return out
}

// SourceByName finds a configured source, enabled or not, so the CLI can run one
// by name without editing the config.
func (p *PipelineConfig) SourceByName(name string) (PipelineSource, bool) {
	for _, s := range p.Sources {
		if strings.EqualFold(s.Name, name) {
			return s, true
		}
	}
	return PipelineSource{}, false
}

// ParseRateLimit turns "2/s" or "30/m" into the minimum interval between two
// requests to the source. An empty value means unlimited, which is only
// appropriate for a local endpoint.
func (s PipelineSource) ParseRateLimit() (time.Duration, error) {
	spec := strings.TrimSpace(s.RateLimit)
	if spec == "" {
		return 0, nil
	}
	count, unit, ok := strings.Cut(spec, "/")
	if !ok {
		return 0, fmt.Errorf("rate_limit %q: want \"N/s\", \"N/m\" or \"N/h\"", spec)
	}
	var n int
	if _, err := fmt.Sscanf(strings.TrimSpace(count), "%d", &n); err != nil || n <= 0 {
		return 0, fmt.Errorf("rate_limit %q: %q is not a positive count", spec, count)
	}
	var window time.Duration
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "s", "sec", "second":
		window = time.Second
	case "m", "min", "minute":
		window = time.Minute
	case "h", "hour":
		window = time.Hour
	default:
		return 0, fmt.Errorf("rate_limit %q: unknown unit %q", spec, unit)
	}
	return window / time.Duration(n), nil
}
