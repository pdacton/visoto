package main

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Lucide renders nothing for a name it does not know: no error, just an empty
// slot where the icon should be. Tabler Icons names (photo, flask,
// file-description) read as plausible and went unnoticed that way, so every
// literal icon name in the templates and the JS is checked against the canonical
// names of the Lucide version base.html loads. Deprecated aliases fail too: they
// still render today, but Lucide drops them between releases.
func TestLucideIconNamesExist(t *testing.T) {
	known, version := readLucideNames(t, "testdata/lucide-icons.txt")

	base, err := os.ReadFile("../../templates/layout/base.html")
	if err != nil {
		t.Fatal(err)
	}
	if m := regexp.MustCompile(`lucide@([0-9.]+)/`).FindSubmatch(base); m == nil {
		t.Fatal("no lucide@<version> in templates/layout/base.html")
	} else if string(m[1]) != version {
		t.Fatalf("base.html loads lucide %s but testdata/lucide-icons.txt lists %s; regenerate the list (see its header)", m[1], version)
	}

	// Literal names only: a templated one (data-lucide="{{ $icon }}") is checked
	// where its value is written, as an "icon" dict argument.
	uses := []*regexp.Regexp{
		regexp.MustCompile(`data-lucide="([a-z0-9-]+)"`),
		regexp.MustCompile(`"icon"\s+"([a-z0-9-]+)"`),
		regexp.MustCompile(`\bicon="([a-z0-9-]+)"`),
		regexp.MustCompile(`data-lucide['"]?\s*,\s*['"]([a-z0-9-]+)['"]`),
		regexp.MustCompile(`icon:\s*'([a-z0-9-]+)'`),
	}
	check := func(path string) error {
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(body), "\n") {
			for _, re := range uses {
				for _, m := range re.FindAllStringSubmatch(line, -1) {
					if !known[m[1]] {
						t.Errorf("%s:%d: %q is not a Lucide %s icon name", path, i+1, m[1], version)
					}
				}
			}
		}
		return nil
	}

	for root, ext := range map[string]string{"../../templates": ".html", "../../static/js": ".js"} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ext) {
				return err
			}
			return check(path)
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
}

// readLucideNames reads the vendored name list; its first line is "# lucide <version>".
func readLucideNames(t *testing.T, path string) (map[string]bool, string) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	names, version := map[string]bool{}, ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case version == "" && strings.HasPrefix(line, "# lucide "):
			version = strings.TrimPrefix(line, "# lucide ")
		case line == "" || strings.HasPrefix(line, "#"):
		default:
			names[line] = true
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if version == "" || len(names) == 0 {
		t.Fatalf("%s: missing the \"# lucide <version>\" header or the names", path)
	}
	return names, version
}
