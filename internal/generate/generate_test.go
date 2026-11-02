package generate_test

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/jiripisa/tuipe/internal/content"
	"github.com/jiripisa/tuipe/internal/generate"
)

type runeSet map[rune]bool

func setOf(s string) runeSet {
	m := runeSet{}
	for _, r := range s {
		m[r] = true
	}
	return m
}

func (s runeSet) Has(r rune) bool { return s[r] }

func lib(t *testing.T) *content.Library {
	t.Helper()
	l, err := content.Load()
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func all(rune) bool { return true }

func TestDrillUsesOnlyAllowedChars(t *testing.T) {
	allowed := setOf("asdfjklů ")
	s := generate.Drill(lib(t), generate.Input{Focus: []rune("fj"), Allowed: allowed.Has, Length: 100, Seed: 1})
	for _, r := range s {
		if !allowed.Has(r) {
			t.Fatalf("disallowed %q in %q", r, s)
		}
	}
	if n := utf8.RuneCountInString(s); n < 100 {
		t.Fatalf("got %d runes, want at least 100: %q", n, s)
	}
}

func TestDrillIsDeterministic(t *testing.T) {
	l := lib(t)
	in := generate.Input{Focus: []rune("rť"), Bigrams: []string{"st"}, Allowed: all, Length: 150, Seed: 42}
	a, b := generate.Drill(l, in), generate.Drill(l, in)
	if a != b {
		t.Fatalf("same input gave different text:\n%q\n%q", a, b)
	}
	in.Seed = 43
	if generate.Drill(l, in) == a {
		t.Error("a different seed gave the same text")
	}
}

func TestDrillContainsFocus(t *testing.T) {
	l := lib(t)
	allowed := setOf("asdfjklů ")
	cases := []generate.Input{
		{Focus: []rune("fj"), Allowed: allowed.Has, Length: 100, Seed: 1},
		{Focus: []rune("fj"), Allowed: allowed.Has, Length: 300, Seed: 2},
		{Focus: []rune("r"), Allowed: all, Length: 200, Seed: 3},
		{Focus: []rune("{}"), Allowed: all, Length: 200, Seed: 4},
	}
	for _, in := range cases {
		s := generate.Drill(l, in)
		focus, total := 0, 0
		for _, r := range s {
			if r == ' ' {
				continue
			}
			total++
			if strings.ContainsRune(string(in.Focus), r) {
				focus++
			}
		}
		if total == 0 || focus*5 < total {
			t.Errorf("focus %q: %d of %d characters, want at least 20 %%: %q", string(in.Focus), focus, total, s)
		}
	}
}

func TestDrillPrefersRealWords(t *testing.T) {
	l := lib(t)
	real := map[string]bool{}
	for _, w := range l.WordsCS {
		real[w] = true
	}
	for _, w := range l.WordsEN {
		real[w] = true
	}
	s := generate.Drill(l, generate.Input{Focus: []rune("r"), Allowed: all, Length: 300, Seed: 7})
	tokens := strings.Fields(s)
	hits := 0
	for _, tok := range tokens {
		if real[tok] {
			hits++
		}
	}
	if hits*2 <= len(tokens) {
		t.Errorf("only %d of %d tokens are real words: %q", hits, len(tokens), s)
	}
}

func TestDiacriticFocusUsesCommitMessages(t *testing.T) {
	l := lib(t)
	for seed := int64(1); seed <= 5; seed++ {
		s := generate.Drill(l, generate.Input{Focus: []rune("řž"), Allowed: all, Length: 200, Seed: seed})
		found := false
		for _, c := range l.CommitsCS {
			if strings.Contains(s, c) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("seed %d: no whole commit message in %q", seed, s)
		}
	}
}

func TestDrillShape(t *testing.T) {
	l := lib(t)
	for _, in := range []generate.Input{
		{Focus: []rune("fj"), Allowed: setOf("asdfjklů ").Has, Seed: 1},
		{Focus: []rune("řž"), Allowed: all, Length: 120, Seed: 2},
		{Allowed: all, Length: 80, Seed: 3},
	} {
		s := generate.Drill(l, in)
		if s != strings.TrimSpace(s) || strings.Contains(s, "  ") || strings.ContainsAny(s, "\n\t\r ") {
			t.Errorf("badly shaped text %q", s)
		}
	}
}

func TestDrillDefaultLengthIs100(t *testing.T) {
	s := generate.Drill(lib(t), generate.Input{Focus: []rune("fj"), Allowed: setOf("asdfjklů ").Has, Seed: 9})
	if n := utf8.RuneCountInString(s); n < 100 || n > 130 {
		t.Errorf("got %d runes, want about 100", n)
	}
}

func TestDrillWithOnlySpaceAllowedIsEmpty(t *testing.T) {
	s := generate.Drill(lib(t), generate.Input{Focus: []rune("a"), Allowed: setOf(" ").Has, Length: 50, Seed: 1})
	if s != "" {
		t.Errorf("got %q, want empty", s)
	}
	s = generate.Drill(lib(t), generate.Input{Allowed: func(rune) bool { return false }, Seed: 1})
	if s != "" {
		t.Errorf("got %q, want empty", s)
	}
}

func TestDrillUnallowedFocusIsIgnored(t *testing.T) {
	allowed := setOf("asdf ")
	s := generate.Drill(lib(t), generate.Input{Focus: []rune("ž"), Allowed: allowed.Has, Length: 60, Seed: 5})
	if utf8.RuneCountInString(s) < 60 {
		t.Errorf("text too short: %q", s)
	}
	for _, r := range s {
		if !allowed.Has(r) {
			t.Fatalf("disallowed %q in %q", r, s)
		}
	}
}

func TestBigramDrillOnNarrowAlphabetContainsBigram(t *testing.T) {
	s := generate.Drill(lib(t), generate.Input{Bigrams: []string{"fj"}, Allowed: setOf("asdfjkl ").Has, Length: 60, Seed: 1})
	if n := strings.Count(s, "fj"); n < 3 {
		t.Errorf("only %d occurrences of fj in %q", n, s)
	}
}

func TestUppercaseFocusUsesCapitalisedRealWords(t *testing.T) {
	l := lib(t)
	real := map[string]bool{}
	for _, w := range append(append([]string{}, l.WordsCS...), l.WordsEN...) {
		real[w] = true
	}
	s := generate.Drill(l, generate.Input{Focus: []rune("R"), Allowed: all, Length: 200, Seed: 3})
	tokens := strings.Fields(s)
	good := 0
	for _, tok := range tokens {
		r, n := utf8.DecodeRuneInString(tok)
		if r == 'R' && real[strings.ToLower(tok[:1])+tok[n:]] {
			good++ // capitalised real word
		} else if strings.Contains(tok, "R") && !strings.ContainsAny(tok, "{}[]") {
			good++ // camelCase such as userRole
		}
	}
	if good*10 < len(tokens)*8 {
		t.Errorf("only %d of %d tokens are capitalised or camelCase real words: %q", good, len(tokens), s)
	}
}

func TestSymbolFocusBuildsTokensFromRealWords(t *testing.T) {
	l := lib(t)
	real := map[string]bool{}
	for _, w := range l.WordsEN {
		real[w] = true
	}
	for _, w := range l.WordsCS {
		real[w] = true
	}
	s := generate.Drill(l, generate.Input{Focus: []rune("{}"), Allowed: all, Length: 200, Seed: 4})
	if strings.Contains(s, "{{{{") {
		t.Errorf("gibberish in %q", s)
	}
	withWord := 0
	tokens := strings.Fields(s)
	for _, tok := range tokens {
		for _, part := range strings.FieldsFunc(tok, func(r rune) bool { return !unicode.IsLetter(r) }) {
			if real[part] {
				withWord++
				break
			}
		}
	}
	if withWord*10 < len(tokens)*8 {
		t.Errorf("only %d of %d tokens contain a real word: %q", withWord, len(tokens), s)
	}
}

func TestDigitFocusBuildsTokens(t *testing.T) {
	s := generate.Drill(lib(t), generate.Input{Focus: []rune("7"), Allowed: all, Length: 100, Seed: 2})
	if !strings.Contains(s, "7") || strings.ContainsAny(s, "ďřŘ") {
		t.Errorf("unexpected %q", s)
	}
}

func TestNoRepeatedTokensInWindow(t *testing.T) {
	for seed := int64(1); seed <= 20; seed++ {
		s := generate.Drill(lib(t), generate.Input{Focus: []rune("fj"), Allowed: setOf("asdfjklů ").Has, Length: 100, Seed: seed})
		counts := map[string]int{}
		for _, tok := range strings.Fields(s) {
			counts[tok]++
			if counts[tok] > 3 || utf8.RuneCountInString(tok) < 2 {
				t.Fatalf("seed %d: token %q repeated or too short in %q", seed, tok, s)
			}
		}
	}
}

func count(s string, r rune) int { return strings.Count(s, string(r)) }

// Rare characters have no words and few templates, so a plain drill may never
// show them. Cover must, whatever the seed.
func TestCoverHoldsEveryRequiredCharacter(t *testing.T) {
	l := lib(t)
	must := []rune("°ˇ¨§ĎŤŇÓÚŮÝ{}[]~^7")
	for seed := int64(1); seed <= 200; seed++ {
		s := generate.Cover(l, generate.Input{Focus: []rune("ab"), Allowed: all, Length: 40, Seed: seed}, must, 1)
		for _, r := range must {
			if count(s, r) == 0 {
				t.Fatalf("seed %d: %q is missing from %q", seed, r, s)
			}
		}
	}
}

func TestCoverIsDeterministicAndKeepsToAllowed(t *testing.T) {
	l := lib(t)
	allowed := setOf("abcdefghijklmnopqrstuvwxyz ěšč")
	in := generate.Input{Focus: []rune("ě"), Allowed: allowed.Has, Length: 60, Seed: 5}
	a, b := generate.Cover(l, in, []rune("šč{"), 1), generate.Cover(l, in, []rune("šč{"), 1)
	if a != b {
		t.Fatalf("same input gave different text:\n%q\n%q", a, b)
	}
	for _, r := range a {
		if !allowed.Has(r) {
			t.Fatalf("disallowed %q in %q", r, a)
		}
	}
	if count(a, 'š') == 0 || count(a, 'č') == 0 {
		t.Errorf("required characters missing from %q", a)
	}
}

func TestCoverRepeatsWhenAsked(t *testing.T) {
	s := generate.Cover(lib(t), generate.Input{Focus: []rune("a"), Allowed: all, Length: 200, Seed: 3}, []rune("§ˇ"), 3)
	if count(s, '§') < 3 || count(s, 'ˇ') < 3 {
		t.Errorf("§ %d times and ˇ %d times, want at least 3 each: %q", count(s, '§'), count(s, 'ˇ'), s)
	}
}

// A drill must stay near its length: the first appearance of each character
// is owed, but repeats and long commit messages must not overrun it.
func TestCoverStaysNearItsLength(t *testing.T) {
	l := lib(t)
	for seed := int64(1); seed <= 100; seed++ {
		s := generate.Cover(l, generate.Input{Focus: []rune("ěšč§ˇ"), Allowed: all, Length: 75, Seed: seed}, []rune("ěšč§ˇ"), 3)
		if n := utf8.RuneCountInString(s); n < 75 || n > 112 {
			t.Fatalf("seed %d: %d characters, want 75 to 112: %q", seed, n, s)
		}
	}
}

func TestCoverShowsItsBigrams(t *testing.T) {
	l := lib(t)
	for seed := int64(1); seed <= 100; seed++ {
		s := generate.Cover(l, generate.Input{Bigrams: []string{"st", "{q"}, Focus: []rune("a"), Allowed: all, Length: 60, Seed: seed}, nil, 1)
		if !strings.Contains(s, "st") || !strings.Contains(s, "{q") {
			t.Fatalf("seed %d: a bigram is missing from %q", seed, s)
		}
	}
}

func TestCoverHoldsOnceCharactersOnce(t *testing.T) {
	l := lib(t)
	for seed := int64(1); seed <= 50; seed++ {
		in := generate.Input{Focus: []rune("a"), Allowed: all, Length: 100, Seed: seed, Once: []rune("§°")}
		s := generate.Cover(l, in, []rune("ˇ"), 3)
		if count(s, 'ˇ') < 3 || count(s, '§') != 1 || count(s, '°') != 1 {
			t.Fatalf("seed %d: ˇ %d, § %d, ° %d times in %q", seed, count(s, 'ˇ'), count(s, '§'), count(s, '°'), s)
		}
	}
}

func TestCoverOnceDoesNotLowerMust(t *testing.T) {
	s := generate.Cover(lib(t), generate.Input{Focus: []rune("a"), Allowed: all, Length: 100, Seed: 1, Once: []rune("§")}, []rune("§"), 3)
	if count(s, '§') < 3 {
		t.Errorf("§ %d times, want at least 3: %q", count(s, '§'), s)
	}
}

func TestCoverOnceKeepsToAllowed(t *testing.T) {
	allowed := setOf("abcdefghijklmnopqrstuvwxyz ")
	s := generate.Cover(lib(t), generate.Input{Focus: []rune("a"), Allowed: allowed.Has, Length: 60, Seed: 1, Once: []rune("§ \nb")}, nil, 1)
	for _, r := range s {
		if !allowed.Has(r) {
			t.Fatalf("disallowed %q in %q", r, s)
		}
	}
}

// Unlike must, Once gives way to the limit: the sample is one attempt of many.
func TestCoverOnceStopsAtTheLimit(t *testing.T) {
	l := lib(t)
	once := []rune("°ˇ¨§ĎŤŇÓÚŮÝ{}[]~^7ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	for seed := int64(1); seed <= 50; seed++ {
		s := generate.Cover(l, generate.Input{Focus: []rune("a"), Allowed: all, Length: 40, Seed: seed, Once: once}, []rune("ab"), 1)
		if n := utf8.RuneCountInString(s); n < 40 || n > 60 {
			t.Fatalf("seed %d: %d characters, want 40 to 60: %q", seed, n, s)
		}
		if count(s, 'a') == 0 || count(s, 'b') == 0 {
			t.Fatalf("seed %d: must is missing from %q", seed, s)
		}
	}
}

func TestDenseKeepsEveryTokenOnFocus(t *testing.T) {
	l := lib(t)
	for _, in := range []generate.Input{
		{Focus: []rune("ěí"), Allowed: all, Length: 300},   // commit messages are thin
		{Focus: []rune("FJDK"), Allowed: all, Length: 300}, // capitalised words are thin
		{Focus: []rune("+="), Allowed: all, Length: 300},   // two-word templates are thin
		{Focus: []rune("ó"), Allowed: all, Length: 300},    // few real words
		{Focus: []rune("fj"), Allowed: setOf("fj ").Has, Length: 300},
	} {
		in.Dense = true
		for seed := int64(1); seed <= 20; seed++ {
			in.Seed = seed
			for _, s := range []string{generate.Drill(l, in), generate.Cover(l, in, in.Focus, 3)} {
				if utf8.RuneCountInString(s) < in.Length {
					t.Fatalf("%q seed %d: too short: %q", string(in.Focus), seed, s)
				}
				for _, tok := range strings.Fields(s) {
					hits := 0
					for _, r := range tok {
						if strings.ContainsRune(string(in.Focus), r) {
							hits++
						}
					}
					if hits*4 < utf8.RuneCountInString(tok) {
						t.Fatalf("%q seed %d: thin token %q in %q", string(in.Focus), seed, tok, s)
					}
				}
			}
		}
	}
}

func TestDenseCoverShowsItsBigrams(t *testing.T) {
	s := generate.Cover(lib(t), generate.Input{Bigrams: []string{"st"}, Allowed: all, Length: 60, Seed: 1, Dense: true}, nil, 1)
	if !strings.Contains(s, "st") {
		t.Errorf("no bigram in %q", s)
	}
}

func TestDenseWithoutFocusIsAPlainDrill(t *testing.T) {
	l := lib(t)
	in := generate.Input{Allowed: all, Length: 80, Seed: 3}
	plain := generate.Drill(l, in)
	in.Dense = true
	if got := generate.Drill(l, in); got != plain {
		t.Errorf("got %q, want %q", got, plain)
	}
}
