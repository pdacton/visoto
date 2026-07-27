package main

import (
	"os"
	"strconv"
	"strings"
	"sync"

	"hutzli.org/visoto/internal/parser"
)

// Memoized scanning of the async template directories.
//
// findAsyncQuery and findFacetSpecs both walk every .html file under
// asyncQueryDirs, read it, and re-parse it — on EVERY request, and twice per
// faceted-table request (findFacetSpecs is called again by collectConstraints).
// The templates only change when a file on disk changes, so the parse is cached
// and invalidated by a cheap directory stat.
//
// Invalidation is by (path, modtime, size) across all scanned files, so editing a
// template during development still takes effect without a restart — matching the
// previous read-every-time behaviour.

type templateScanCache struct {
	mu    sync.Mutex
	stamp string
	// parsed elements per extractor, keyed by extractor identity (see scanKey)
	byExtractor map[string][]parser.ExtractedElement
}

var scanCache = templateScanCache{byExtractor: map[string][]parser.ExtractedElement{}}

// dirStamp builds a cheap fingerprint of every scanned template file: path, size
// and modtime. Any edit, addition or removal changes it.
func dirStamp() string {
	var b strings.Builder
	for _, dir := range asyncQueryDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".html") {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			b.WriteString(dir)
			b.WriteByte('/')
			b.WriteString(e.Name())
			b.WriteByte(':')
			b.WriteString(info.ModTime().String())
			b.WriteByte(':')
			b.WriteString(strconv.FormatInt(info.Size(), 10))
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// scanTemplateElements returns every element the given extractor finds across the
// async template directories, in directory/file/document order. Results are cached
// until a template file changes. `key` identifies the extractor for caching.
func scanTemplateElements(extract func(string) ([]parser.ExtractedElement, error), key string) []parser.ExtractedElement {
	stamp := dirStamp()

	scanCache.mu.Lock()
	defer scanCache.mu.Unlock()
	if scanCache.stamp != stamp {
		// Something on disk changed — drop every extractor's cached parse.
		scanCache.stamp = stamp
		scanCache.byExtractor = map[string][]parser.ExtractedElement{}
	}
	if cached, ok := scanCache.byExtractor[key]; ok {
		return cached
	}

	var out []parser.ExtractedElement
	for _, dir := range asyncQueryDirs {
		files, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".html") {
				continue
			}
			content, err := os.ReadFile(dir + "/" + f.Name())
			if err != nil {
				continue
			}
			elements, err := extract(string(content))
			if err != nil {
				continue
			}
			out = append(out, elements...)
		}
	}
	scanCache.byExtractor[key] = out
	return out
}
