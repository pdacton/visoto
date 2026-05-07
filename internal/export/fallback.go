package export

import (
	"errors"
	"fmt"
	"io"
	"log/slog"

	"hutzli.org/visoto/internal/logger"
)

// ExportWithFallback tries export providers in order until one succeeds.
// Fallback order: graphdb → gsp → construct.
//
// If ep.ExportProvider is set, only that provider is tried (no fallback).
// A provider signals it cannot handle the endpoint by returning ErrNotApplicable,
// which causes the next provider to be tried.
// Any other error from an applicable provider stops the chain immediately.
//
// Returns the stream, the winning provider name, and any error.
func ExportWithFallback(params ExportParams) (io.ReadCloser, string, error) {
	log := logger.Get()

	// Pinned provider — skip fallback chain entirely.
	if params.Endpoint.ExportProvider != "" {
		p, ok := GetProvider(params.Endpoint.ExportProvider)
		if !ok {
			return nil, "", fmt.Errorf("configured export provider %q is not registered", params.Endpoint.ExportProvider)
		}
		rc, err := p.Export(params)
		if err != nil {
			return nil, "", fmt.Errorf("export provider %q failed: %w", p.Name(), err)
		}
		log.Info("export succeeded", slog.String("provider", p.Name()))
		return rc, p.Name(), nil
	}

	// Fallback chain: try each provider in canonical order.
	var lastErr error
	for _, p := range allProviders() {
		rc, err := p.Export(params)
		if err == nil {
			log.Info("export succeeded", slog.String("provider", p.Name()))
			return rc, p.Name(), nil
		}
		if errors.Is(err, ErrNotApplicable) {
			log.Debug("provider not applicable, trying next", slog.String("provider", p.Name()))
			continue
		}
		// Real error from an applicable provider — stop here.
		log.Warn("export provider failed",
			slog.String("provider", p.Name()),
			slog.String("error", err.Error()))
		lastErr = err
		break
	}

	if lastErr != nil {
		return nil, "", lastErr
	}
	return nil, "", fmt.Errorf("no export provider could handle the request")
}
