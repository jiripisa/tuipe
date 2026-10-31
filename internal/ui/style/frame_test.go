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

func TestClipHookFiresOnOversizedBody(t *testing.T) {
	var got []string
	style.ClipHook = func(what string) { got = append(got, what) }
	t.Cleanup(func() { style.ClipHook = nil })
	s := style.All()[0]

	style.Frame(s, 30, 8, "t", "a\nb", "foot")
	if len(got) != 0 {
		t.Fatalf("a body that fits fired the hook: %v", got)
	}
	style.Frame(s, 30, 8, "t", strings.Repeat("row\n", 20), "foot")
	if len(got) == 0 {
		t.Error("too many rows did not fire the hook")
	}
	got = nil
	style.Frame(s, 30, 8, "t", strings.Repeat("x", 60), "foot")
	if len(got) == 0 {
		t.Error("a too wide line did not fire the hook")
	}
	got = nil
	style.Frame(s, 30, 8, "t", "a", strings.Repeat("f", 60))
	if len(got) == 0 {
		t.Error("a too wide footer did not fire the hook")
	}
}
