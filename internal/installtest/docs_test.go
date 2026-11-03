package installtest

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// docPages are the pages that describe the app to people.
var docPages = []string{"README.md", "docs/layouts.md", "site/index.html", "site/manual.html"}

func repoFile(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// forbiddenWords are the words of the old game. A word with a capital
// is matched exactly, the others in any case.
var forbiddenWords = regexp.MustCompile(`\bSLA\b|(?i:sprint|standup|incident|postmortem|retrospectiv|retrospektiv|\bghosts?\b|\bduch|head~1|git log|uptime|error budget|rychlovk|kariér|test-out|locked chapters|nástup|pro programátory)`)

// Every page of the docs is in docPages, so that the checks below read it.
func TestEveryDocPageIsChecked(t *testing.T) {
	for _, pattern := range []string{"*.md", "docs/*.md", "site/*.html"} {
		found, err := filepath.Glob(filepath.Join("..", "..", filepath.FromSlash(pattern)))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range found {
			rel, err := filepath.Rel(filepath.Join("..", ".."), f)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Contains(docPages, filepath.ToSlash(rel)) {
				t.Errorf("%s is not in docPages", filepath.ToSlash(rel))
			}
		}
	}
}

func TestDocsUseNoWorkplaceWords(t *testing.T) {
	for _, page := range docPages {
		for i, line := range strings.Split(repoFile(t, page), "\n") {
			if w := forbiddenWords.FindString(line); w != "" {
				t.Errorf("%s:%d uses %q: %s", page, i+1, w, strings.TrimSpace(line))
			}
		}
	}
}

var pictureRef = regexp.MustCompile(`assets/([A-Za-z0-9_-]+\.png)`)

func TestSiteImagesExist(t *testing.T) {
	used := map[string]bool{}
	for _, page := range docPages {
		for _, m := range pictureRef.FindAllStringSubmatch(repoFile(t, page), -1) {
			used[m[1]] = true
			if _, err := os.Stat(filepath.Join("..", "..", "site", "assets", m[1])); err != nil {
				t.Errorf("%s refers to a missing picture: %s", page, m[1])
			}
		}
	}
	files, err := filepath.Glob(filepath.Join("..", "..", "site", "assets", "*.png"))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if name := filepath.Base(f); !used[name] {
			t.Errorf("no page uses site/assets/%s", name)
		}
	}
}
