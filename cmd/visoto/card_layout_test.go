package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Resource pages space their cards with Tabler's card grid alone: every card in a
// column of a `row row-deck row-cards`, nothing carrying a margin
// (docs/templating.md#card-layout). The margin-wrapper convention it replaced
// still reads as working when copied into a new template, and doubles or drops
// the gap next to a card row, so these pin the rule. pages/ is exempt: the theme
// pages are bespoke and lay out sections of their own.
func TestCardsUseRowCards(t *testing.T) {
	var (
		cardMargin = regexp.MustCompile(`class="card(?:\s[^"]*)?\sm[bt]-\d`)
		plainRow   = regexp.MustCompile(`<div class="row( m[bt]-\d)?">`)
		wrapper    = regexp.MustCompile(`^(\s*)<div class="m[bt]-\d">\s*$`)
		cardCall   = regexp.MustCompile(`\{\{-?\s*template\s+"(sparql\w+|bnodeTree|shapeSummary|schemaGraph|mediaCard)"`)
	)

	for _, dir := range []string{"classes", "instances", "components", "layout", "partials"} {
		err := filepath.WalkDir(filepath.Join("../../templates", dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".html") {
				return err
			}
			body, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			lines := strings.Split(string(body), "\n")
			for i, line := range lines {
				if cardMargin.MatchString(line) {
					t.Errorf("%s:%d gives a card a margin; the row-cards gutter spaces it:\n  %s", path, i+1, strings.TrimSpace(line))
				}
				if dir != "layout" && dir != "partials" && plainRow.MatchString(line) {
					t.Errorf("%s:%d lays cards out in a plain row; use `row row-deck row-cards`:\n  %s", path, i+1, strings.TrimSpace(line))
				}
				m := wrapper.FindStringSubmatch(line)
				if m == nil {
					continue
				}
				// The wrapper's body runs to the </div> at its own indentation.
				for j := i + 1; j < len(lines) && lines[j] != m[1]+"</div>"; j++ {
					if cardCall.MatchString(lines[j]) {
						t.Errorf("%s:%d wraps a card in a margin div; put it in a row-cards column instead:\n  %s", path, i+1, strings.TrimSpace(line))
						break
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking templates/%s: %v", dir, err)
		}
	}
}
