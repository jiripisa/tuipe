package style_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/jiripisa/tuipe/internal/layout"
	"github.com/jiripisa/tuipe/internal/ui/style"
)

func czech(t *testing.T) *layout.Layout {
	t.Helper()
	all, err := layout.Builtin()
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range all {
		if l.ID == "macos-czech-qwerty" {
			return l
		}
	}
	t.Fatal("no macos-czech-qwerty layout")
	return nil
}

func TestKeyboardMapContainsAllRows(t *testing.T) {
	s := ansi.Strip(style.Keyboard(style.ByID("editor"), czech(t), nil, nil, 60))
	for _, label := range []string{"<", "ě", "ú", ")", "ů", "\\", "-", "⇧", "⌥"} {
		if !strings.Contains(s, label) {
			t.Errorf("keyboard map lacks %q:\n%s", label, s)
		}
	}
}

func TestKeyboardFitsWidth(t *testing.T) {
	for _, w := range []int{30, 56, 76, 120} {
		for _, id := range []style.ID{"fosfor", "editor", "velin"} {
			out := style.Keyboard(style.ByID(id), czech(t), []string{"k0", "shift-left"}, map[string]float64{"k1": 0.5}, w)
			for _, line := range strings.Split(out, "\n") {
				if got := ansi.StringWidth(line); got > w {
					t.Errorf("%s width %d: line is %d wide: %q", id, w, got, ansi.Strip(line))
				}
			}
		}
	}
}

func TestKeyboardHighlightsPseudoKeys(t *testing.T) {
	plain := style.Keyboard(style.ByID("editor"), czech(t), nil, nil, 60)
	hl := style.Keyboard(style.ByID("editor"), czech(t), []string{"shift-left", "option-right"}, nil, 60)
	if plain == hl {
		t.Error("highlighting the modifiers did not change the output")
	}
	if ansi.Strip(plain) != ansi.Strip(hl) {
		t.Error("highlight must change only colours, not text")
	}
}

func TestThreeStyles(t *testing.T) {
	if got := len(style.All()); got != 3 {
		t.Errorf("styles = %d, want 3", got)
	}
	if got := style.Next("velin"); got != style.ID("fosfor") {
		t.Errorf("Next(velin) = %q, want fosfor", got)
	}
	if got := style.Next("fosfor"); got != style.ID("editor") {
		t.Errorf("Next(fosfor) = %q, want editor", got)
	}
}

func TestFrameHasExactSize(t *testing.T) {
	out := style.Frame(style.ByID("velin"), 40, 8, "Titul", "řádek 1\nřádek 2", "patička")
	lines := strings.Split(out, "\n")
	if len(lines) != 8 {
		t.Fatalf("lines = %d, want 8", len(lines))
	}
	for _, l := range lines {
		if got := ansi.StringWidth(l); got != 40 {
			t.Errorf("line width %d, want 40: %q", got, ansi.Strip(l))
		}
	}
	plain := ansi.Strip(out)
	for _, want := range []string{"Titul", "řádek 2", "patička"} {
		if !strings.Contains(plain, want) {
			t.Errorf("frame lacks %q", want)
		}
	}
}
