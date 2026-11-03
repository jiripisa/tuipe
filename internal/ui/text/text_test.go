package text_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/jiripisa/tuipe/internal/ui/text"
)

func TestCzechPlurals(t *testing.T) {
	for n, want := range map[int]string{0: "0 kol", 1: "1 kolo", 2: "2 kola", 4: "4 kola", 5: "5 kol", 21: "21 kol"} {
		if got := text.Rounds(n); got != want {
			t.Errorf("Rounds(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestOfferPassedAgreesWithItsNumber(t *testing.T) {
	for n, want := range map[int]string{
		1: "Zvládli jste 1 fázi z 6.",
		3: "Zvládli jste 3 fáze z 6.",
		5: "Zvládli jste 5 fází z 6.",
	} {
		if got := text.OfferPassed(n, 6); got != want {
			t.Errorf("OfferPassed(%d, 6) = %q", n, got)
		}
	}
}

func TestPartDeltaShowsDirectionAndSize(t *testing.T) {
	for _, c := range []struct {
		v       float64
		percent bool
		want    string
	}{
		{68, false, "▲ +68"},
		{-12, false, "▼ −12"},
		{0, false, "= 0"},
		{0.8, true, "▲ +0,8 %"},
		{-0.7, true, "▼ −0,7 %"},
		{0, true, "= 0 %"},
	} {
		if got := text.PartDelta(c.v, c.percent); got != c.want {
			t.Errorf("PartDelta(%v, %v) = %q, want %q", c.v, c.percent, got, c.want)
		}
	}
}

func TestPartFigures(t *testing.T) {
	if got := text.PartPercent(0.974); got != "97,4 %" {
		t.Errorf("PartPercent = %q", got)
	}
	if got := text.PartPercent(1); got != "100,0 %" {
		t.Errorf("PartPercent = %q", got)
	}
	for sec, want := range map[int]string{0: "0:00", 9: "0:09", 88: "1:28", 600: "10:00"} {
		if got := text.PartClock(sec); got != want {
			t.Errorf("PartClock(%d) = %q, want %q", sec, got, want)
		}
	}
}

func TestPartMissedListNamesWhatCannotBeSeen(t *testing.T) {
	want := "mezera · Enter · čárka"
	if got := text.PartMissedList([]rune{' ', '\n', ','}); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSkippedContentNamesWhatIsThere(t *testing.T) {
	if got := text.SkippedContent("X", 0); got != "" {
		t.Errorf("nothing skipped: %q", got)
	}
	want := "Rozložení X neumí některé znaky. Přeskočeno: 5 lekcí."
	if got := text.SkippedContent("X", 5); got != want {
		t.Errorf("got %q", got)
	}
}

// forbidden is the vocabulary the app's own wording must not use. A stem hits
// every word that starts with it ("kariér" hits "kariéra" and "kariérní"). A
// whole word hits only that word, which keeps short items from matching by
// accident: "sla" is in "slabé", "hr" in "Ohrožení" and "diff" in "different".
var forbidden = []struct {
	term  string
	whole bool
}{
	{"sprint", false}, {"incident", false}, {"standup", false}, {"retrospective", false},
	{"retrospektiv", false}, {"postmortem", false}, {"git log", false}, {"head~1", false},
	{"uptime", false}, {"error budget", false}, {"rozpočet chyb", false}, {"nástup", false},
	{"commit", false}, {"kariér", false}, {"duch", false}, {"wpm", false},
	{"sla", true}, {"hr", true}, {"diff", true}, {"ghost", true},
}

// hits lists the forbidden terms in s, ignoring case. A term starts at the
// beginning of a word; a whole word ends at the end of one as well. Letters
// and digits make a word, so Czech letters count as letters.
func hits(s string) []string {
	s = strings.ToLower(s)
	word := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }
	var out []string
	for _, f := range forbidden {
		for from := 0; ; {
			i := strings.Index(s[from:], f.term)
			if i < 0 {
				break
			}
			start, end := from+i, from+i+len(f.term)
			from = start + 1
			if r, _ := utf8.DecodeLastRuneInString(s[:start]); start > 0 && word(r) {
				continue
			}
			if r, _ := utf8.DecodeRuneInString(s[end:]); f.whole && end < len(s) && word(r) {
				continue
			}
			out = append(out, f.term)
			break
		}
	}
	return out
}

func TestHitsMatchWholeWordsAndStemsOnly(t *testing.T) {
	for s, want := range map[string]int{
		"Prší slabé písmeno, nejslabší prst": 0, // "sla" inside words
		"Ghostty: do konfigurace":            0, // a terminal, not a ghost
		"different, Ohrožení, hra, duben":    0,
		"Běží SLA, ne HR":                    2,
		"Rozdíl: diff":                       1,
		"Kariérní postmortem":                2,
		"Mluví duchové":                      1,
		"zn/min, ne WPM":                     1,
		"vypíše commit.":                     1,
	} {
		if got := hits(s); len(got) != want {
			t.Errorf("hits(%q) = %v, want %d", s, got, want)
		}
	}
}

// TestNoWorkplaceWords parses text.go and checks the value of every string
// literal in it, so identifiers such as ResultsWPM are no hits, and so are
// comments.
func TestNoWorkplaceWords(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "text.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		value, err := strconv.Unquote(lit.Value)
		if err != nil {
			t.Fatalf("%s: %v", fset.Position(lit.Pos()), err)
		}
		if h := hits(value); len(h) > 0 {
			t.Errorf("%s: %v in %q", fset.Position(lit.Pos()), h, value)
		}
		return true
	})
}

// TestNoWorkplaceCommentsInText checks the comments of text.go too, for the
// terms that must not be called the app's own.
func TestNoWorkplaceCommentsInText(t *testing.T) {
	src, err := os.ReadFile("text.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(src), "own term") {
		t.Error("text.go still calls words the app's own terms")
	}
}
