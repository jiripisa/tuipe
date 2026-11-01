package layout_test

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/jiripisa/tuipe/internal/layout"
)

// tomlBlock finds the fenced TOML examples of a Markdown file.
var tomlBlock = regexp.MustCompile("(?s)```toml\n(.*?)```")

// TestDocumentedExampleLoads keeps docs/layouts.md honest: the example is read
// from the guide and goes through the same loader as a user's file.
func TestDocumentedExampleLoads(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "layouts.md"))
	if err != nil {
		t.Fatal(err)
	}
	blocks := tomlBlock.FindAllSubmatch(doc, -1)
	if len(blocks) != 1 {
		t.Fatalf("docs/layouts.md has %d TOML examples, want 1", len(blocks))
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "example.toml"), blocks[0][1], 0o644); err != nil {
		t.Fatal(err)
	}

	layouts, errs := layout.LoadDir(dir)
	if len(errs) != 0 {
		t.Fatalf("the documented example does not load: %v", errs)
	}
	if len(layouts) != 1 {
		t.Fatalf("loaded %d layouts, want 1", len(layouts))
	}
	l := layouts[0]
	if l.ID != "example-small" || l.ModifierName != "AltGr" {
		t.Errorf("identity: id %q, modifier %q", l.ID, l.ModifierName)
	}
	for r, want := range map[rune]string{
		'f': "k3", 'F': "shift+k3", 'ě': "k19", '@': "altgr+k19",
		'ď': "shift+k24 k2", ' ': "k49", '\n': "k36",
	} {
		assertRoute(t, l, r, want)
	}
}
