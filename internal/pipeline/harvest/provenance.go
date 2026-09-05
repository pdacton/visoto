package harvest

import (
	"context"
	"strconv"
	"time"

	"hutzli.org/visoto/internal/config"
	"hutzli.org/visoto/internal/pipeline/load"
	"hutzli.org/visoto/internal/pipeline/rdf"
	"hutzli.org/visoto/internal/pipeline/state"
)

// writeProvenance records the run itself and repoints the source's current-graph
// pointer at the catalogue this run produced.
//
// Two graphs with deliberately different lifecycles: run provenance is
// cumulative, so "when did this dataset's structure change" stays answerable
// (R-VER-1), while the pointer graph is replaced, so a reader following it never
// sees two candidate catalogues for one source (R-CAT-4).
func (r *Runner) writeProvenance(ctx context.Context, loader load.Loader,
	sc config.PipelineSource, run *state.Run, catalogGraph string, watermark time.Time) error {

	runIRI := r.minter.RunIRI(run.ID)
	sourceIRI := r.minter.SourceIRI(sc.Name)
	ended := r.now()

	runQuads := []rdf.Quad{
		q(runIRI, rdf.A, rdf.VisoHarvestRun),
		q(runIRI, rdf.A, rdf.ProvActivity),
		q(runIRI, rdf.VisoRunID, rdf.Literal(run.ID)),
		q(runIRI, rdf.VisoSource, sourceIRI),
		q(runIRI, rdf.VisoSourceType, rdf.Literal(sc.Type)),
		q(runIRI, rdf.VisoCatalogGraph, rdf.IRI(catalogGraph)),
		q(runIRI, rdf.VisoRunStatus, rdf.Literal(string(state.RunSucceeded))),
		q(runIRI, rdf.ProvStartedAtTime, dateTime(run.StartedAt)),
		q(runIRI, rdf.ProvEndedAtTime, dateTime(ended)),
		q(runIRI, rdf.VisoDatasetsSeen, integer(run.DatasetsSeen)),
		q(runIRI, rdf.VisoDistributionsSeen, integer(run.DistributionsSeen)),
		q(runIRI, rdf.VisoQuadsWritten, integer(run.QuadsWritten)),
	}
	if err := loader.Append(ctx, r.minter.RunGraph(), runQuads); err != nil {
		return err
	}
	run.QuadsWritten += int64(len(runQuads))

	pointerQuads := []rdf.Quad{
		q(sourceIRI, rdf.A, rdf.VisoCatalogSource),
		q(sourceIRI, rdf.RDFSLabel, rdf.Literal(sc.Name)),
		q(sourceIRI, rdf.VisoSourceType, rdf.Literal(sc.Type)),
		q(sourceIRI, rdf.VisoHarvestedFrom, rdf.IRI(sc.URL)),
		q(sourceIRI, rdf.VisoCurrentCatalogGraph, rdf.IRI(catalogGraph)),
		q(sourceIRI, rdf.VisoLastRun, runIRI),
	}
	if !watermark.IsZero() {
		pointerQuads = append(pointerQuads, q(sourceIRI, rdf.VisoWatermark, dateTime(watermark)))
	}
	if err := load.ReplaceGraph(ctx, loader, r.minter.CurrentGraph(sc.Name), pointerQuads); err != nil {
		return err
	}
	run.QuadsWritten += int64(len(pointerQuads))
	return nil
}

// q builds a graph-less quad; the loader assigns the graph.
func q(s, p, o rdf.Term) rdf.Quad { return rdf.NewQuad(s, p, o, "") }

func dateTime(t time.Time) rdf.Term {
	return rdf.TypedLiteral(t.UTC().Format(time.RFC3339), rdf.Xsd("dateTime"))
}

func integer(n int64) rdf.Term {
	return rdf.TypedLiteral(strconv.FormatInt(n, 10), rdf.Xsd("integer"))
}
