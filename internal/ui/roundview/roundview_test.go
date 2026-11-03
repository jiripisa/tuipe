package roundview_test

import (
	"flag"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/jiripisa/tuipe/internal/engine"
	"github.com/jiripisa/tuipe/internal/hint"
	"github.com/jiripisa/tuipe/internal/layout"
	"github.com/jiripisa/tuipe/internal/play"
	"github.com/jiripisa/tuipe/internal/ui/roundview"
	"github.com/jiripisa/tuipe/internal/ui/style"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

var update = flag.Bool("update", false, "rewrite the golden files")

var fullSizes = [][2]int{{60, 18}, {80, 24}, {80, 45}, {100, 50}}

const goSource = `// parsePort reads a TCP port from text.
func parsePort(s string) (int, error) {
    n, err := strconv.Atoi(strings.TrimSpace(s))
    if err != nil {
        return 0, fmt.Errorf("bad port %q: %w", s, err)
    }
    if n < 1 || n > 65535 {
        return 0, errors.New("port out of range")
    }
    return n, nil
}

// Handle answers one request and logs a failure.
func (s *Server) Get(w http.ResponseWriter, r *Req) {
    if err := s.auth(r); err != nil {
        http.Error(w, "unauthorized", 401)
        return
    }
    port, err := parsePort(r.URL.Query().Get("port"))
    if err != nil {
        http.Error(w, err.Error(), 400)
        return
    }
    fmt.Fprintf(w, "port %d\n", port)
}`

const pythonSource = `def parse_port(text):
    value = int(text.strip())
    if value < 1 or value > 65535:
        raise ValueError("port out of range")
    return value


def main():
    print(parse_port("8080"))`

const shellSource = `kubectl -n auth get pods | grep -v Running
kubectl -n auth logs deploy/auth --since=10m
kubectl -n auth rollout undo deploy/auth`

const prose = "oprava: přihlášení už nepadá na prázdném hesle a správně " +
	"vrací chybu 401. Čeština v chybových hláškách se zobrazuje bez rozbitých " +
	"znaků, protože odpověď má konečně správné kódování. Přidali jsme také " +
	"test pro dlouhé uživatelské jméno a odstranili starý přepínač, který " +
	"už nikdo nepoužíval. Příští týden doplníme stránkování do seznamu " +
	"uživatelů a zrychlíme hledání podle e-mailu."

var czech = sync.OnceValue(func() *layout.Layout {
	all, err := layout.Builtin()
	if err != nil {
		panic(err)
	}
	for _, l := range all {
		if l.ID == "macos-czech-qwerty" {
			return l
		}
	}
	panic("no macos-czech-qwerty layout")
})

// frame builds a frame on src with the cursor on the given occurrence of at.
func frame(src, at string) play.Frame {
	t := engine.Prepare(src)
	pos := len([]rune(src[:strings.Index(src, at)]))
	return play.Frame{
		Cells: t.Cells, Pos: pos, WPM: 62, Accuracy: 0.981, WrongPos: map[int]bool{},
	}
}

func hintFor(r rune) *play.HintInfo {
	route, _ := czech().Route(r)
	return &play.HintInfo{Rune: r, Text: hint.Text(czech(), r), Route: route}
}

func sample(name string) roundview.View {
	v := roundview.View{Layout: czech(), Stars: 2}
	switch name {
	case "plain":
		v.Frame = frame(prose, "správně vrací")
		v.Frame.WrongPos[12] = true
		v.Title = "kapitola 3 / diakritika  ::  lekce 4"
	case "code":
		v.Frame = frame(goSource, "strconv.Atoi")
		v.Frame.WrongPos[v.Frame.Pos-30] = true
		v.Title, v.Lang = "kapitola 6 / symboly  ::  lekce 4", "go"
		v.Throughput = []float64{41, 44, 47, 52, 55, 58, 54, 60, 63, 59, 62}
	case "hint":
		v.Frame = frame(goSource, "{\n    if err := s.auth(r)")
		v.Frame.Hint = hintFor('{')
		v.Frame.Weak = []rune{'{', '}', '~', '\\'}
		v.Title, v.Lang = "kapitola 6 / symboly  ::  lekce 4", "go"
		v.Throughput = []float64{41, 44, 47, 52, 55, 58, 54, 60, 63, 59, 62}
	case "shell":
		v.Frame = frame(shellSource, "logs deploy")
		v.Frame.Weak = []rune{'|', '-', ' '}
		v.Title, v.Lang = "kapitola 2 / shell  ::  lekce 3", "shell"
		v.Throughput = []float64{58, 60, 57, 62, 66, 64, 61}
	case "paused":
		v.Frame = frame(pythonSource, "raise ValueError")
		v.Frame.Paused = true
		v.Title, v.Lang = "kapitola 7 / kód  ::  lekce 2", "python"
	}
	return v
}

func assertFits(t *testing.T, out string, w, h int) {
	t.Helper()
	lines := strings.Split(out, "\n")
	if len(lines) != h {
		t.Fatalf("%d×%d: got %d lines", w, h, len(lines))
	}
	for i, line := range lines {
		if got := ansi.StringWidth(line); got != w {
			t.Fatalf("%d×%d: line %d is %d cells wide: %q", w, h, i, got, ansi.Strip(line))
		}
	}
}

func golden(t *testing.T, id style.ID, sz [2]int, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", fmt.Sprintf("%s-%dx%d-%s.golden", id, sz[0], sz[1], name))
	// Trailing spaces say nothing and make the goldens hard to read and edit.
	got = regexp.MustCompile(`(?m) +$`).ReplaceAllString(got, "")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to create it)", err)
	}
	if strings.ReplaceAll(string(want), "\r\n", "\n") != got {
		t.Errorf("%s differs\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
	}
}

func TestSizes(t *testing.T) {
	for _, st := range style.All() {
		for _, sz := range append(fullSizes, [2]int{50, 12}) {
			for _, f := range []string{"plain", "code", "hint", "paused"} {
				out := roundview.Render(st, sample(f), sz[0], sz[1])
				assertFits(t, out, sz[0], sz[1])
				golden(t, st.ID, sz, f, ansi.Strip(out))
			}
		}
	}
}

func TestEveryWindowSizeFitsExactly(t *testing.T) {
	v := sample("shell")
	v.Title = strings.Repeat("velmi dlouhý nadpis ", 12)
	wide := sample("hint")
	wide.Frame.Cells = engine.Prepare("漢字 ve výuce\n\tkód s tabulátorem a " + strings.Repeat("x", 200)).Cells
	wide.Frame.Pos = 3
	done := sample("code")
	done.Frame.Pos = len(done.Frame.Cells)
	done.Frame.Done = true
	empty := sample("plain")
	empty.Frame = play.Frame{}
	for _, st := range style.All() {
		for name, view := range map[string]roundview.View{"long": v, "wide": wide, "done": done, "empty": empty} {
			t.Run(string(st.ID)+"/"+name, func(t *testing.T) {
				for w := 1; w <= 130; w += 3 {
					for h := 1; h <= 60; h += 3 {
						assertFits(t, roundview.Render(st, view, w, h), w, h)
					}
				}
				for _, sz := range [][2]int{{59, 40}, {200, 17}, {60, 18}} {
					assertFits(t, roundview.Render(st, view, sz[0], sz[1]), sz[0], sz[1])
				}
			})
		}
	}
}

func TestSmallWindowsUseTheCompactStrip(t *testing.T) {
	for _, st := range style.All() {
		for _, sz := range [][2]int{{59, 40}, {120, 17}, {50, 12}, {40, 8}} {
			v := sample("shell")
			v.Frame.Hint = hintFor('{')
			s := ansi.Strip(roundview.Render(st, v, sz[0], sz[1]))
			for _, want := range []string{"310 zn/min", "98.1 %", "logs", v.Frame.Hint.Text} {
				if !strings.Contains(s, want) {
					t.Errorf("%s %v lacks %q:\n%s", st.ID, sz, want, s)
				}
			}
			for _, no := range []string{"⌥", "⇧", "mezera", "krok"} {
				if strings.Contains(s, no) {
					t.Errorf("%s %v shows %q, compact has text and core numbers only:\n%s", st.ID, sz, no, s)
				}
			}
		}
	}
}

func TestCompactHintIsOneLineCutToWidth(t *testing.T) {
	st := style.ByID(style.Editor)
	v := sample("hint")
	out := roundview.Render(st, v, 30, 12)
	assertFits(t, out, 30, 12)
	s := ansi.Strip(out)
	if n := strings.Count(s, "→"); n != 1 {
		t.Errorf("hint shown %d times:\n%s", n, s)
	}
	if strings.Contains(s, v.Frame.Hint.Text) || !strings.Contains(s, "{ → levý Option") || !strings.Contains(s, "…") {
		t.Errorf("hint is not one line cut to width:\n%s", s)
	}
	if strings.Contains(s, "prsteník") {
		t.Errorf("hint wrapped to a second line:\n%s", s)
	}
}

func TestCompactKeepsTextAndCursorAtSmallestHeights(t *testing.T) {
	for _, st := range style.All() {
		for h := 1; h <= 8; h++ {
			v := sample("shell")
			v.Frame.Hint = hintFor('{')
			v.Frame.AltWarning = true
			out := roundview.Render(st, v, 50, h)
			assertFits(t, out, 50, h)
			s := ansi.Strip(out)
			if cellsWith(out, func(c styledCell) bool { return c.bg == rgb(st.Accent) && c.text == "l" }) == "" {
				t.Errorf("%s height %d: no cursor:\n%s", st.ID, h, s)
			}
			for from, want := range map[int]string{
				1: "kubectl",       // the text and the cursor always
				2: "310 zn/min",    // then the numbers
				4: "levý Option",   // then the hint
				5: "Option je Alt", // and last the warning
			} {
				if h >= from && !strings.Contains(s, want) {
					t.Errorf("%s height %d: no %q:\n%s", st.ID, h, want, s)
				}
			}
		}
	}
}

func TestCompactHintAndWarningDoNotMoveTheText(t *testing.T) {
	for _, st := range style.All() {
		for _, sz := range [][2]int{{50, 12}, {40, 8}, {50, 5}} {
			v := sample("code")
			rows := func(v roundview.View) string {
				lines := strings.Split(ansi.Strip(roundview.Render(st, v, sz[0], sz[1])), "\n")
				return strings.Join(lines[1:sz[1]-2], "\n")
			}
			before := rows(v)
			v.Frame.Hint = hintFor('s')
			v.Frame.AltWarning = true
			if after := rows(v); after != before {
				t.Errorf("%s %v: text moved:\n%s\nwas:\n%s", st.ID, sz, after, before)
			}
		}
	}
}

func TestCompactPausedKeepsTheNumbersAtNarrowestWidth(t *testing.T) {
	for _, st := range style.All() {
		v := sample("shell")
		v.Frame.Paused = true
		s := ansi.Strip(roundview.Render(st, v, 40, 8))
		for _, want := range []string{text.RoundPaused, "310 zn/min", "98.1 %"} {
			if !strings.Contains(s, want) {
				t.Errorf("%s lacks %q:\n%s", st.ID, want, s)
			}
		}
	}
}

func TestSpaceBarSaysMezera(t *testing.T) {
	for _, st := range style.All() {
		for _, sz := range fullSizes {
			s := ansi.Strip(roundview.Render(st, sample("hint"), sz[0], sz[1]))
			if !strings.Contains(s, "mezera") || strings.Contains(s, "space") {
				t.Errorf("%s %v: space bar label:\n%s", st.ID, sz, s)
			}
		}
	}
}

func TestNoHintMeansNoKeyHighlightOrChord(t *testing.T) {
	v := sample("hint")
	v.Frame.Hint = nil
	for _, st := range style.All() {
		for _, sz := range append(fullSizes, [2]int{50, 12}) {
			s := ansi.Strip(roundview.Render(st, v, sz[0], sz[1]))
			for _, no := range []string{"Option", "Shift", "prsteník", "→", "⌥", "⇧"} {
				if strings.Contains(s, no) {
					t.Errorf("%s %v reveals %q without a hint:\n%s", st.ID, sz, no, s)
				}
			}
		}
	}
}

func TestHintShowsTextAndHighlightsChord(t *testing.T) {
	for _, st := range style.All() {
		out := roundview.Render(st, sample("hint"), 80, 24)
		s := ansi.Strip(out)
		for _, want := range []string{"levý Option + í", "pravý prsteník", "⌥"} {
			if !strings.Contains(strings.Join(strings.Fields(s), " "), want) {
				t.Errorf("%s lacks %q:\n%s", st.ID, want, s)
			}
		}
		// The key í and the left Option carry the highlight; cursor is the third.
		marked := cellsWith(out, func(c styledCell) bool { return c.bg == rgb(st.Accent) })
		for _, want := range []string{"í", "⌥"} {
			if !strings.Contains(marked, want) {
				t.Errorf("%s: %q is not highlighted, marked: %q", st.ID, want, marked)
			}
		}
	}
}

func TestWidestHintFitsAtMinimumWidth(t *testing.T) {
	var widest string
	for _, r := range []rune("ďťňóůú°^~`{}[]\\|@#$&*<>'\"+-=ě") {
		if h := hint.Text(czech(), r); ansi.StringWidth(h) > ansi.StringWidth(widest) {
			widest = h
		}
	}
	v := sample("hint")
	v.Frame.Hint = &play.HintInfo{Rune: []rune(widest)[0], Text: widest}
	for _, id := range []style.ID{style.Editor, style.Fosfor} {
		s := ansi.Strip(roundview.Render(style.ByID(id), v, 60, 18))
		if !strings.Contains(s, widest) {
			t.Errorf("%s cuts the hint %q:\n%s", id, widest, s)
		}
	}
	// Velín wraps it in the next-character panel, the right column of the grid.
	var panel []string
	for _, line := range strings.Split(ansi.Strip(roundview.Render(style.ByID(style.Velin), v, 60, 18)), "\n") {
		if _, right, ok := strings.Cut(line, "││"); ok && strings.Contains(line, "│") {
			panel = append(panel, strings.Trim(right, " │"))
		}
	}
	joined := strings.Join(panel, " ")
	_, chord, _ := strings.Cut(widest, " → ")
	chord, _, _ = strings.Cut(chord, " (")
	if !strings.Contains(joined, chord) || strings.Contains(joined, "…") {
		t.Errorf("Velín cuts the hint %q: %q", widest, joined)
	}
}

func TestFosforShowsMapOnlyWithHint(t *testing.T) {
	st := style.ByID(style.Fosfor)
	for _, sz := range fullSizes {
		with := ansi.Strip(roundview.Render(st, sample("hint"), sz[0], sz[1]))
		if !strings.Contains(with, "⌥") {
			t.Errorf("%v: no map with a hint:\n%s", sz, with)
		}
		lines := strings.Split(with, "\n")
		if !strings.Contains(strings.Join(lines[len(lines)-3:], ""), "⌥") {
			t.Errorf("%v: the map is not at the bottom:\n%s", sz, with)
		}
		v := sample("hint")
		v.Frame.Hint = nil
		without := ansi.Strip(roundview.Render(st, v, sz[0], sz[1]))
		if strings.Contains(without, "⌥") {
			t.Errorf("%v: map without a hint:\n%s", sz, without)
		}
	}
}

func TestHintDoesNotMoveTheText(t *testing.T) {
	for _, st := range style.All() {
		for _, sz := range fullSizes {
			with := strings.Split(ansi.Strip(roundview.Render(st, sample("hint"), sz[0], sz[1])), "\n")
			v := sample("hint")
			v.Frame.Hint = nil
			without := strings.Split(ansi.Strip(roundview.Render(st, v, sz[0], sz[1])), "\n")
			row := func(lines []string) int {
				for i, l := range lines {
					if strings.Contains(l, "if err := s.auth(r)") {
						return i
					}
				}
				return -1
			}
			if row(with) < 0 || row(with) != row(without) {
				t.Errorf("%s %v: text moved between rows %d and %d", st.ID, sz, row(without), row(with))
			}
		}
	}
}

func TestCursorAlwaysVisibleInLongText(t *testing.T) {
	var b strings.Builder
	for i := 1; i <= 40; i++ {
		fmt.Fprintf(&b, "radek cislo %d\n", i)
	}
	src := strings.TrimSuffix(b.String(), "\n")
	for _, st := range style.All() {
		for _, sz := range append(fullSizes, [2]int{50, 12}, [2]int{40, 8}) {
			f := frame(src, "radek cislo 40")
			f.Pos = len(f.Cells) - 1
			out := roundview.Render(st, roundview.View{Frame: f, Layout: czech()}, sz[0], sz[1])
			s := ansi.Strip(out)
			if !strings.Contains(s, "radek cislo 40") {
				t.Errorf("%s %v: cursor line is off screen:\n%s", st.ID, sz, s)
			}
			if !strings.Contains(s, "radek cislo 39") {
				t.Errorf("%s %v: no context above the cursor:\n%s", st.ID, sz, s)
			}
			at := cellsWith(out, func(c styledCell) bool { return c.bg == rgb(st.Accent) })
			if !strings.HasSuffix(at, "0") {
				t.Errorf("%s %v: cursor cell not marked, marked: %q", st.ID, sz, at)
			}
		}
	}
}

func TestCursorStaysVisibleThroughEveryPosition(t *testing.T) {
	for _, st := range style.All() {
		for _, sz := range [][2]int{{60, 18}, {80, 24}, {50, 12}} {
			f := frame(goSource, "func parsePort")
			for f.Pos = 0; f.Pos < len(f.Cells); f.Pos++ {
				if f.Cells[f.Pos].Auto {
					continue
				}
				out := roundview.Render(st, roundview.View{Frame: f, Layout: czech()}, sz[0], sz[1])
				if cellsWith(out, func(c styledCell) bool { return c.bg == rgb(st.Accent) }) == "" {
					t.Fatalf("%s %v: cursor at %d is not on screen", st.ID, sz, f.Pos)
				}
			}
		}
	}
}

func TestCellKindsLookDifferent(t *testing.T) {
	src := "αβγδεζηθ"
	for _, st := range style.All() {
		for _, sz := range append(fullSizes, [2]int{50, 12}) {
			f := frame(src, "ε")
			f.WrongPos[1] = true
			out := roundview.Render(st, roundview.View{Frame: f}, sz[0], sz[1])
			look := func(r string) styledCell { return cellOfText(t, out, r) }
			kinds := map[string]styledCell{
				"typed": look("α"), "wrong": look("β"),
				"cursor": look("ε"), "untyped": look("ζ"),
			}
			seen := map[styledCell]string{}
			for name, c := range kinds {
				if other, dup := seen[c]; dup {
					t.Errorf("%s %v: %s and %s look the same: %+v", st.ID, sz, name, other, c)
				}
				seen[c] = name
			}
			if !look("β").underline || look("β").fg != rgb(st.Bad) {
				t.Errorf("%s %v: wrong press is not marked: %+v", st.ID, sz, look("β"))
			}
		}
	}
}

func TestNewlineIsAVisibleMark(t *testing.T) {
	for _, st := range style.All() {
		f := frame("prvni\ndruhy", "druhy")
		f.Pos = 5
		out := roundview.Render(st, roundview.View{Frame: f}, 80, 24)
		if !strings.Contains(ansi.Strip(out), "prvni↵") {
			t.Errorf("%s: no newline mark:\n%s", st.ID, ansi.Strip(out))
		}
		if got := cellOfText(t, out, "↵"); got.bg != rgb(st.Accent) {
			t.Errorf("%s: cursor on the newline is not visible: %+v", st.ID, got)
		}
	}
}

func TestSpaceUnderCursorIsVisible(t *testing.T) {
	for _, st := range style.All() {
		for _, sz := range append(fullSizes, [2]int{50, 12}) {
			f := frame("αβ γδ", "γδ")
			f.Pos = 2
			out := roundview.Render(st, roundview.View{Frame: f}, sz[0], sz[1])
			if got := cellAfter(t, out, "β"); got.bg != rgb(st.Accent) {
				t.Errorf("%s %v: cursor on a space has no block: %+v", st.ID, sz, got)
			}
		}
	}
}

func TestWrongSpaceIsAColouredBlock(t *testing.T) {
	for _, st := range style.All() {
		for _, sz := range append(fullSizes, [2]int{50, 12}) {
			f := frame("αβ γδ", "δ")
			f.WrongPos[2] = true
			out := roundview.Render(st, roundview.View{Frame: f}, sz[0], sz[1])
			if got := cellAfter(t, out, "β"); got.bg != rgb(st.Bad) {
				t.Errorf("%s %v: wrong space is not a block: %+v", st.ID, sz, got)
			}
		}
	}
}

func TestWrongPressUnderCursorTurnsCursorBad(t *testing.T) {
	for _, st := range style.All() {
		f := frame("αβγδ", "γ")
		f.WrongPos[2] = true
		out := roundview.Render(st, roundview.View{Frame: f}, 80, 24)
		if got := cellOfText(t, out, "γ"); got.bg != rgb(st.Bad) {
			t.Errorf("%s: cursor on a wrong press: %+v", st.ID, got)
		}
	}
}

func TestAutoIndentIsDimAndCursorSkipsIt(t *testing.T) {
	for _, st := range style.All() {
		f := frame("if x:\n    ω = 1", "ω = 1")
		out := roundview.Render(st, roundview.View{Frame: f, Lang: "python"}, 80, 24)
		if got := cellOfText(t, out, "ω"); got.bg != rgb(st.Accent) {
			t.Errorf("%s: cursor is not on the first real cell: %+v", st.ID, got)
		}
		if got := cellAfter(t, out, "↵"); got.bg == rgb(st.Accent) {
			t.Errorf("%s: cursor drawn on an indentation cell", st.ID)
		}
		// Indentation behind the cursor keeps the dim colour too.
		f.Pos = len(f.Cells) - 1
		out = roundview.Render(st, roundview.View{Frame: f}, 80, 24)
		indent := styleOfRow(t, out, "ω = ")
		if indent.fg != rgb(st.Dim) {
			t.Errorf("%s: indentation is %+v, want dim %s", st.ID, indent, rgb(st.Dim))
		}
	}
}

func TestPausedIsShownInsideTheLayout(t *testing.T) {
	for _, st := range style.All() {
		for _, sz := range append(fullSizes, [2]int{50, 12}) {
			out := roundview.Render(st, sample("paused"), sz[0], sz[1])
			if s := ansi.Strip(out); !strings.Contains(s, text.RoundPaused) {
				t.Errorf("%s %v: no paused state:\n%s", st.ID, sz, s)
			}
			running := sample("paused")
			running.Frame.Paused = false
			if s := ansi.Strip(roundview.Render(st, running, sz[0], sz[1])); strings.Contains(s, text.RoundPaused) {
				t.Errorf("%s %v: paused state on a running round", st.ID, sz)
			}
		}
	}
}

func TestAltWarningIsOneLineInEveryLayout(t *testing.T) {
	for _, st := range style.All() {
		for _, sz := range append(fullSizes, [2]int{50, 12}) {
			for _, paused := range []bool{false, true} {
				v := sample("code")
				v.Frame.AltWarning = true
				v.Frame.Paused = paused
				s := ansi.Strip(roundview.Render(st, v, sz[0], sz[1]))
				want := text.RoundAltWarning
				if sz[0] < 60 {
					want = ansi.Truncate(want, sz[0]-3, "")
				}
				if !strings.Contains(s, want) {
					t.Errorf("%s %v: no warning %q:\n%s", st.ID, sz, want, s)
				}
				if paused && !strings.Contains(s, text.RoundPaused) {
					t.Errorf("%s %v: warning hid the paused state:\n%s", st.ID, sz, s)
				}
			}
		}
	}
}

func TestProseWrapsAtWordBoundaries(t *testing.T) {
	v := roundview.View{Frame: frame(prose, "oprava")}
	for _, st := range style.All() {
		for _, sz := range [][2]int{{100, 50}, {80, 45}, {50, 30}} {
			s := ansi.Strip(roundview.Render(st, v, sz[0], sz[1]))
			var rows []string
			for _, line := range strings.Split(s, "\n") {
				line = strings.Trim(line, " 0123456789│")
				if line != "" && strings.Contains(prose, line) {
					rows = append(rows, line)
				}
			}
			if got := strings.Join(rows, " "); got != prose {
				t.Errorf("%s %v: rows do not rejoin at spaces:\n%s", st.ID, sz, strings.Join(rows, "\n"))
			}
		}
	}
}

func TestWrapDoesNotShiftWhileTyping(t *testing.T) {
	st := style.ByID(style.Editor)
	f := frame(prose, "oprava")
	text0 := func(pos int) []string {
		f.Pos = pos
		lines := strings.Split(ansi.Strip(roundview.Render(st, roundview.View{Frame: f}, 80, 45)), "\n")
		return lines[1:12]
	}
	start := text0(0)
	for pos := 1; pos < len(f.Cells); pos += 7 {
		if got := text0(pos); strings.Join(got, "\n") != strings.Join(start, "\n") {
			t.Fatalf("text moved at pos %d:\n%s\nwas:\n%s", pos, strings.Join(got, "\n"), strings.Join(start, "\n"))
		}
	}
}

func TestLongestCodeLineFitsAtMinimumWidth(t *testing.T) {
	line := "func (s *Server) Handle(w http.ResponseWriter, r *http.R"
	if n := len(line); n != 56 {
		t.Fatalf("test line is %d long", n)
	}
	src := line + "\n" + line + "x\n" + "next"
	for _, st := range style.All() {
		f := frame(src, "func")
		s := ansi.Strip(roundview.Render(st, roundview.View{Frame: f, Lang: "go"}, 60, 18))
		if !strings.Contains(s, line+"↵") {
			t.Errorf("%s: 56-cell line and its mark do not fit width 60:\n%s", st.ID, s)
		}
	}
}

func TestEditorSyntaxColoursKeepTextIntact(t *testing.T) {
	st := style.ByID(style.Editor)
	for _, sz := range fullSizes {
		plain := sample("code")
		plain.Lang = ""
		a := roundview.Render(st, plain, sz[0], sz[1])
		b := roundview.Render(st, sample("code"), sz[0], sz[1])
		if a == b {
			t.Errorf("%v: no syntax colours", sz)
		}
		// The first row names the file by language; the text below is the same.
		_, plainText, _ := strings.Cut(ansi.Strip(a), "\n")
		_, colouredText, _ := strings.Cut(ansi.Strip(b), "\n")
		if plainText != colouredText {
			t.Errorf("%v: colouring changed the text:\n%s\n%s", sz, plainText, colouredText)
		}
	}
}

func TestEditorColoursUntypedTextByLang(t *testing.T) {
	st := style.ByID(style.Editor)
	cases := map[string]string{
		"go":     "func main() {\n    return 0\n}",
		"python": "def main():\n    return 0",
		"js":     "function main() {\n    return 0;\n}",
		"ts":     "function main(): number {\n    return 0;\n}",
		"shell":  "if true; then\n    echo 'x'\nfi",
		"sql":    "SELECT id FROM users WHERE id = 1;",
		"regex":  `^[\w.+-]+@[\w-]+(\.[\w-]+)+$`,
	}
	for lang, src := range cases {
		f := frame(src, src[:1])
		f.Pos = 0
		coloured := colours(roundview.Render(st, roundview.View{Frame: f, Lang: lang}, 80, 24), src)
		plain := colours(roundview.Render(st, roundview.View{Frame: f}, 80, 24), src)
		if coloured <= plain {
			t.Errorf("%s: %d colours in the text, %d without a language", lang, coloured, plain)
		}
	}
	for _, lang := range []string{"", "cobol", "regexx", "main.go"} {
		f := frame(cases["go"], "func")
		a := roundview.Render(st, roundview.View{Frame: f, Lang: lang}, 80, 24)
		b := roundview.Render(st, roundview.View{Frame: f}, 80, 24)
		if a != b {
			t.Errorf("unknown lang %q is coloured", lang)
		}
	}
}

func TestTypedTextIsNotSyntaxColoured(t *testing.T) {
	st := style.ByID(style.Editor)
	f := frame(goSource, "strconv.Atoi")
	out := roundview.Render(st, roundview.View{Frame: f, Lang: "go"}, 80, 24)
	if got := cellOfText(t, out, "func"); got.fg != rgb(st.OK) {
		t.Errorf("typed keyword is %+v, want one readable colour %s", got, rgb(st.OK))
	}
}

func TestSyntaxIsCachedPerText(t *testing.T) {
	f := frame(goSource, "func")
	a := roundview.SyntaxClasses(f.Cells, "go")
	b := roundview.SyntaxClasses(f.Cells, "go")
	if len(a) != len(f.Cells) {
		t.Fatalf("got %d classes for %d cells", len(a), len(f.Cells))
	}
	if &a[0] != &b[0] {
		t.Error("the same text was tokenised twice")
	}
	if c := roundview.SyntaxClasses(f.Cells, "python"); &c[0] == &a[0] {
		t.Error("a different language reused the cache")
	}
}

func TestVelinPanels(t *testing.T) {
	st := style.ByID(style.Velin)
	for _, sz := range fullSizes {
		s := ansi.Strip(roundview.Render(st, sample("hint"), sz[0], sz[1]))
		for _, want := range []string{text.RoundPanelSpeed, text.RoundPanelStats, text.RoundNextChar, text.RoundWeakest, "{ } ~ \\"} {
			if !strings.Contains(s, want) {
				t.Errorf("%v: lacks %q:\n%s", sz, want, s)
			}
		}
	}
}

func TestVelinSparkline(t *testing.T) {
	st := style.ByID(style.Velin)
	v := sample("hint")
	v.Throughput = []float64{10, 20, 40, 80}
	s := ansi.Strip(roundview.Render(st, v, 80, 24))
	if !strings.Contains(s, "▁▂▄█") {
		t.Errorf("no sparkline:\n%s", s)
	}
	v.Throughput = nil
	assertFits(t, roundview.Render(st, v, 80, 24), 80, 24)
	v.Throughput = make([]float64, 500) // all zero, and wider than the panel
	assertFits(t, roundview.Render(st, v, 80, 24), 80, 24)
}

func TestNonDefaultNumbersDoNotBreakFrames(t *testing.T) {
	for _, st := range style.All() {
		v := sample("hint")
		v.Stars = 9
		v.Frame.WPM = 1234
		v.Frame.Accuracy = 0
		assertFits(t, roundview.Render(st, v, 60, 18), 60, 18)
		v.Stars = -1
		assertFits(t, roundview.Render(st, v, 80, 24), 80, 24)
	}
}

func TestStars(t *testing.T) {
	for _, st := range style.All() {
		v := sample("code")
		for n, want := range map[int]string{0: "☆☆☆", 2: "★★☆", 3: "★★★"} {
			v.Stars = n
			if s := ansi.Strip(roundview.Render(st, v, 80, 24)); !strings.Contains(s, want) {
				t.Errorf("%s: no %q:\n%s", st.ID, want, s)
			}
		}
	}
}

func BenchmarkRender(b *testing.B) {
	v := sample("hint")
	for _, st := range style.All() {
		b.Run(string(st.ID), func(b *testing.B) {
			for b.Loop() {
				roundview.Render(st, v, 80, 45)
			}
		})
	}
}

// styledCell is what one column of the output looks like.
type styledCell struct {
	text, fg, bg string
	underline    bool
}

var sgr = regexp.MustCompile(`\x1b\[[0-9;:]*m`)

// parseCells walks one rendered screen and resolves the style of every cell.
func parseCells(out string) []styledCell {
	var cells []styledCell
	var cur styledCell
	for len(out) > 0 {
		loc := sgr.FindStringIndex(out)
		plain := out
		if loc != nil {
			plain = out[:loc[0]]
		}
		for _, r := range plain {
			c := cur
			c.text = string(r)
			cells = append(cells, c)
		}
		if loc == nil {
			break
		}
		cur = applySGR(cur, out[loc[0]+2:loc[1]-1])
		out = out[loc[1]:]
	}
	return cells
}

func applySGR(c styledCell, params string) styledCell {
	parts := strings.Split(params, ";")
	for i := 0; i < len(parts); i++ {
		switch p := parts[i]; {
		case p == "" || p == "0":
			c = styledCell{}
		case p == "4" || p == "4:1":
			c.underline = true
		case p == "24":
			c.underline = false
		case p == "39":
			c.fg = ""
		case p == "49":
			c.bg = ""
		case (p == "38" || p == "48") && i+4 < len(parts) && parts[i+1] == "2":
			v := strings.Join(parts[i+2:i+5], ";")
			if p == "38" {
				c.fg = v
			} else {
				c.bg = v
			}
			i += 4
		}
	}
	return c
}

func rgb(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return strconv.Itoa(int(r>>8)) + ";" + strconv.Itoa(int(g>>8)) + ";" + strconv.Itoa(int(b>>8))
}

func cellsWith(out string, keep func(styledCell) bool) string {
	var b strings.Builder
	for _, c := range parseCells(out) {
		if keep(c) {
			b.WriteString(c.text)
		}
	}
	return b.String()
}

// cellOfText finds the first cell of the first occurrence of text.
func cellOfText(t *testing.T, out, text string) styledCell {
	t.Helper()
	cells := parseCells(out)
	want := []rune(text)
next:
	for i := 0; i+len(want) <= len(cells); i++ {
		for j, r := range want {
			if cells[i+j].text != string(r) {
				continue next
			}
		}
		return cells[i]
	}
	t.Fatalf("%q is not on screen:\n%s", text, ansi.Strip(out))
	return styledCell{}
}

// cellAfter finds the cell right after the first cell showing text.
func cellAfter(t *testing.T, out, text string) styledCell {
	t.Helper()
	cells := parseCells(out)
	for i, c := range cells {
		if c.text == text {
			return cells[i+1]
		}
	}
	t.Fatalf("%q is not on screen:\n%s", text, ansi.Strip(out))
	return styledCell{}
}

// styleOfRow returns the cell just before the text, which is indentation.
func styleOfRow(t *testing.T, out, text string) styledCell {
	t.Helper()
	cells := parseCells(out)
	want := []rune(text)
next:
	for i := 1; i+len(want) <= len(cells); i++ {
		for j, r := range want {
			if cells[i+j].text != string(r) {
				continue next
			}
		}
		return cells[i-1]
	}
	t.Fatalf("%q is not on screen", text)
	return styledCell{}
}

// colours counts the foreground colours used on the cells of src.
func colours(out, src string) int {
	seen := map[string]bool{}
	in := map[string]bool{}
	for _, r := range src {
		if r != ' ' && r != '\n' {
			in[string(r)] = true
		}
	}
	for _, line := range strings.Split(out, "\n")[1:6] {
		for _, c := range parseCells(line) {
			if in[c.text] {
				seen[c.fg] = true
			}
		}
	}
	return len(seen)
}
