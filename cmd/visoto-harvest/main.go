// Command visoto-harvest runs the data structure harvesting pipeline.
//
// It shares visoto.config with the web server but nothing else: the server never
// imports the pipeline, and the pipeline never serves HTTP. See
// .project/todo/pipelinePlan.md for the requirements this implements.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/logger"
	"hutzli.org/visoto/internal/pipeline/harvest"
	"hutzli.org/visoto/internal/pipeline/load"
	"hutzli.org/visoto/internal/pipeline/source"
	"hutzli.org/visoto/internal/pipeline/state"

	// Adapters register themselves. Adding a source means adding an import here
	// and nothing else (R-SRC-2).
	_ "hutzli.org/visoto/internal/pipeline/source/ckan"
	_ "hutzli.org/visoto/internal/pipeline/source/dcatsparql"
)

const usage = `visoto-harvest — harvest open data catalogues and derive dataset structure

Usage:
  visoto-harvest [flags] <command>

Commands:
  sources    list configured sources and the registered adapter types
  run        harvest catalogues into the triplestore
  status     show job state: stage counts and recent runs

Flags:
  -config string   path to visoto.config (default "visoto.config")
  -source string   limit the command to one configured source, by name
  -full            ignore the watermark and harvest the whole catalogue
  -dry-run         resolve config and build adapters, but do not harvest
  -limit int       stop after N datasets per source (0 = no limit). A limited
                   run is partial, so it does not advance the watermark — use it
                   to validate a newly configured source against a live catalogue.
`

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		configPath = flag.String("config", "visoto.config", "path to visoto.config")
		sourceName = flag.String("source", "", "limit to one configured source")
		full       = flag.Bool("full", false, "ignore the watermark and harvest everything")
		dryRun     = flag.Bool("dry-run", false, "resolve config and adapters without harvesting")
		limit      = flag.Int("limit", 0, "stop after N datasets per source (0 = no limit)")
	)
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	flag.Parse()

	command := flag.Arg(0)
	if command == "" {
		flag.Usage()
		return fmt.Errorf("no command given")
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	logger.MustInit(logger.Config{
		Level:  cfg.Logging.Level,
		Format: cfg.Logging.Format,
		Output: cfg.Logging.Output,
	})

	switch command {
	case "sources":
		return listSources(cfg)
	case "run":
		return runHarvest(cfg, *sourceName, *full, *dryRun, *limit)
	case "status":
		return showStatus(cfg, *sourceName)
	default:
		flag.Usage()
		return fmt.Errorf("unknown command %q", command)
	}
}

// listSources prints the configured sources next to the adapter types that are
// compiled in, which is the quickest way to diagnose a config typo.
func listSources(cfg *config.Config) error {
	fmt.Printf("registered source types: %s\n", strings.Join(source.Types(), ", "))
	fmt.Printf("registered loaders:      %s\n\n", strings.Join(load.Names(), ", "))

	if len(cfg.Pipeline.Sources) == 0 {
		fmt.Println("no [[pipeline.sources]] configured")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tTYPE\tENABLED\tRATE\tURL")
	for _, s := range cfg.Pipeline.Sources {
		known := ""
		if !contains(source.Types(), s.Type) {
			known = "  ← unknown type"
		}
		fmt.Fprintf(w, "%s\t%s%s\t%t\t%s\t%s\n",
			s.Name, s.Type, known, s.Enabled, orDash(s.RateLimit), s.URL)
	}
	return w.Flush()
}

// runHarvest executes the harvest for one source or for every enabled source.
func runHarvest(cfg *config.Config, sourceName string, full, dryRun bool, limit int) error {
	if !cfg.Pipeline.Enabled {
		return fmt.Errorf("pipeline is disabled; set enabled = true under [pipeline]")
	}

	target, err := resolveTarget(cfg)
	if err != nil {
		return err
	}

	store, err := state.Open(cfg.Pipeline.StateDB)
	if err != nil {
		return err
	}
	defer store.Close()

	runner := harvest.NewRunner(&cfg.Pipeline, target, store)
	runner.Limit = limit

	// Ctrl-C cancels the context rather than killing the process, so the run is
	// recorded as failed and the staged output is committed and inspectable.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var results []harvest.Result
	if sourceName != "" {
		sc, ok := cfg.Pipeline.SourceByName(sourceName)
		if !ok {
			return fmt.Errorf("no configured source named %q", sourceName)
		}
		if dryRun {
			return dryRunSource(&cfg.Pipeline, sc)
		}
		res, runErr := runner.RunSource(ctx, sc, full)
		if res != nil {
			results = append(results, *res)
		}
		printResults(results)
		return runErr
	}

	if dryRun {
		for _, sc := range cfg.Pipeline.EnabledSources() {
			if err := dryRunSource(&cfg.Pipeline, sc); err != nil {
				return err
			}
		}
		return nil
	}

	results, runErr := runner.RunAll(ctx, full)
	printResults(results)
	return runErr
}

// dryRunSource builds the adapter without harvesting, so a config error surfaces
// in a second rather than after the first page of a catalogue.
func dryRunSource(cfg *config.PipelineConfig, sc config.PipelineSource) error {
	adapter, err := source.New(sc, source.Options{UserAgent: cfg.UserAgent})
	if err != nil {
		return fmt.Errorf("source %q: %w", sc.Name, err)
	}
	interval, _ := sc.ParseRateLimit()
	fmt.Printf("ok  %-20s type=%-12s rate=%v url=%s\n", adapter.Name(), sc.Type, orUnlimited(interval), sc.URL)
	return nil
}

func printResults(results []harvest.Result) {
	if len(results) == 0 {
		return
	}
	fmt.Println()
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "SOURCE\tRUN\tMODE\tDATASETS\tDISTRIBUTIONS\tQUADS\tSTATUS")
	for _, r := range results {
		mode := "full"
		if r.Incremental {
			mode = "since " + r.Since.Format(time.RFC3339)
		}
		if r.Partial {
			mode += " (partial: limit)"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%d\t%d\t%s\n",
			r.Run.Source, r.Run.ID, mode,
			r.Run.DatasetsSeen, r.Run.DistributionsSeen, r.Run.QuadsWritten, r.Run.Status)
	}
	w.Flush()

	for _, r := range results {
		if r.Load.NextStep != "" {
			fmt.Printf("\n%s: %s\n", r.Run.Source, r.Load.NextStep)
		}
	}
}

// showStatus reports the job store's contents without touching the network.
func showStatus(cfg *config.Config, sourceName string) error {
	store, err := state.Open(cfg.Pipeline.StateDB)
	if err != nil {
		return err
	}
	defer store.Close()

	total, err := store.CountDistributions(sourceName)
	if err != nil {
		return err
	}
	counts, err := store.StageCounts(sourceName)
	if err != nil {
		return err
	}

	fmt.Printf("state db: %s\n", store.Path())
	fmt.Printf("distributions: %d\n\n", total)

	if len(counts) > 0 {
		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "STAGE\tCOUNT")
		stages := make([]string, 0, len(counts))
		for s := range counts {
			stages = append(stages, string(s))
		}
		sort.Strings(stages)
		for _, s := range stages {
			fmt.Fprintf(w, "%s\t%d\n", s, counts[state.Stage(s)])
		}
		w.Flush()
		fmt.Println()
	}

	runs, err := store.RecentRuns(sourceName, 10)
	if err != nil {
		return err
	}
	if len(runs) == 0 {
		fmt.Println("no runs recorded")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "RUN\tSOURCE\tSTARTED\tDURATION\tSTATUS\tDATASETS\tDISTRIBUTIONS\tQUADS\tMESSAGE")
	for _, r := range runs {
		duration := "—"
		if !r.EndedAt.IsZero() {
			duration = r.EndedAt.Sub(r.StartedAt).Round(time.Second).String()
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%d\t%d\t%d\t%s\n",
			r.ID, r.Source, r.StartedAt.Format(time.RFC3339), duration, r.Status,
			r.DatasetsSeen, r.DistributionsSeen, r.QuadsWritten, truncate(r.Message, 60))
	}
	return w.Flush()
}

// resolveTarget finds the endpoint the pipeline writes to. The bulk-file loader
// needs none, so a missing target is only an error for a loader that talks to an
// endpoint.
func resolveTarget(cfg *config.Config) (*config.SparqlEndpoint, error) {
	slug := strings.TrimSpace(cfg.Pipeline.TargetEndpoint)
	if slug == "" {
		if cfg.Pipeline.Loader == config.LoaderSparqlUpdate {
			return nil, fmt.Errorf("loader %q needs target_endpoint under [pipeline]", cfg.Pipeline.Loader)
		}
		return nil, nil
	}
	ep := cfg.Application.GetEndpointBySlug(slug)
	if ep == nil {
		return nil, fmt.Errorf("target_endpoint %q matches no [[application.sparqlEndpoints]] slug", slug)
	}
	return ep, nil
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

func orUnlimited(d time.Duration) string {
	if d <= 0 {
		return "unlimited"
	}
	return fmt.Sprintf("1 per %v", d)
}

func truncate(s string, max int) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), "\n", " ")
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
