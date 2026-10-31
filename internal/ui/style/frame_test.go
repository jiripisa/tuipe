package style_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/jiripisa/tuipe/internal/ui/style"
)

func TestFrameIsExactlyWindowSizedAtAnyWidth(t *testing.T) {
	for _, s := range style.All() {
		for w := 1; w <= 30; w++ {
			for _, h := range []int{1, 2, 3, 4, 8} {
				out := style.Frame(s, w, h, "Dlouhý název obrazovky", "tělo\ndruhý řádek", "nápověda k obrazovce")
				lines := strings.Split(out, "\n")
				if len(lines) != h {
					t.Fatalf("%s %dx%d: %d lines", s.ID, w, h, len(lines))
				}
				for i, l := range lines {
					if got := ansi.StringWidth(l); got != w {
						t.Fatalf("%s %dx%d line %d: width %d: %q", s.ID, w, h, i, got, ansi.Strip(l))
					}
				}
			}
		}
	}
}
