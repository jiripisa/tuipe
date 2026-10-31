// Package generate builds deterministic drill text from the embedded material.
package generate

import (
	"math/rand/v2"
	"sort"
	"strings"
	"unicode"

	"github.com/jiripisa/tuipe/internal/content"
)

// Input describes the drill to generate.
type Input struct {
	Focus   []rune   // characters to practise
	Bigrams []string // character pairs to practise
	Allowed func(rune) bool
	Length  int // minimum length in characters, 100 when zero
	Seed    int64
}

const (
	defaultLength = 100
	minRealWords  = 8    // fewer real focus words than this are mixed with pseudo-words
	strongDensity = 0.25 // share of focus characters that makes a word a strong drill word
	vowels        = "aeiouyáéěíóúůý"
)

// Drill returns text of at least in.Length characters made of tokens separated
// by single spaces, using only characters admitted by in.Allowed. It returns an
// empty string when no character other than space is allowed.
func Drill(lib *content.Library, in Input) string {
	length := in.Length
	if length <= 0 {
		length = defaultLength
	}
	allowed := in.Allowed
	if allowed == nil {
		allowed = func(rune) bool { return true }
	}
	g := &gen{
		rng:     rand.New(rand.NewPCG(uint64(in.Seed), uint64(in.Seed)^0x9e3779b97f4a7c15)),
		allowed: allowed,
		bigrams: in.Bigrams,
		sep:     "",
	}
	if allowed(' ') {
		g.sep = " "
	}
	for _, r := range in.Focus {
		if r != ' ' && allowed(r) {
			g.focus = append(g.focus, r)
		}
	}
	if !g.collectAlphabet(lib) {
		return ""
	}
	g.collectPools(lib)

	var sb strings.Builder
	n, prev := 0, ""
	for i := 0; n < length; i++ {
		tok := g.token(i == 0)
		for try := 0; tok == prev && try < 5; try++ {
			tok = g.token(false)
		}
		if tok == "" {
			continue
		}
		if sb.Len() > 0 {
			sb.WriteString(g.sep)
			n += len([]rune(g.sep))
		}
		sb.WriteString(tok)
		n += len([]rune(tok))
		prev = tok
	}
	return sb.String()
}

type gen struct {
	rng     *rand.Rand
	allowed func(rune) bool
	focus   []rune
	bigrams []string
	sep     string

	letters   []rune // allowed letters, sorted
	vowels    []rune
	consonant []rune
	symbols   []rune // allowed non-space characters, sorted

	strong, loose []string // real words drilling the focus
	plain         []string // real words without a focus requirement
	commits       []string // commit messages drilling a diacritic focus
	diacritic     bool
}

// collectAlphabet finds the allowed non-space characters. Allowed is a
// predicate, so candidates are the characters of the material, printable ASCII
// and the focus. It reports whether any exist.
func (g *gen) collectAlphabet(lib *content.Library) bool {
	seen := map[rune]bool{}
	add := func(r rune) {
		if r != ' ' && !seen[r] && g.allowed(r) {
			seen[r] = true
			g.symbols = append(g.symbols, r)
		}
	}
	for r := rune('!'); r <= '~'; r++ {
		add(r)
	}
	for _, list := range [][]string{lib.WordsCS, lib.WordsEN, lib.CommitsCS} {
		for _, s := range list {
			for _, r := range s {
				add(r)
			}
		}
	}
	for _, r := range g.focus {
		add(r)
	}
	sort.Slice(g.symbols, func(i, j int) bool { return g.symbols[i] < g.symbols[j] })
	for _, r := range g.symbols {
		if !unicode.IsLetter(r) {
			continue
		}
		g.letters = append(g.letters, r)
		if strings.ContainsRune(vowels, unicode.ToLower(r)) {
			g.vowels = append(g.vowels, r)
		} else {
			g.consonant = append(g.consonant, r)
		}
	}
	return len(g.symbols) > 0
}

func (g *gen) collectPools(lib *content.Library) {
	for _, r := range g.focus {
		if r > unicode.MaxASCII && unicode.IsLetter(r) {
			g.diacritic = true
		}
	}
	hasFocus := len(g.focus) > 0 || len(g.bigrams) > 0
	for _, list := range [][]string{lib.WordsCS, lib.WordsEN} {
		for _, w := range list {
			if !g.within(w) {
				continue
			}
			if !hasFocus {
				g.plain = append(g.plain, w)
				continue
			}
			score := g.focusScore(w)
			if score == 0 {
				continue
			}
			g.loose = append(g.loose, w)
			if float64(score)/float64(len([]rune(w))) >= strongDensity {
				g.strong = append(g.strong, w)
			}
		}
	}
	if g.diacritic {
		for _, c := range lib.CommitsCS {
			if g.within(c) && g.focusScore(c) > 0 {
				g.commits = append(g.commits, c)
			}
		}
	}
}

func (g *gen) within(s string) bool {
	for _, r := range s {
		if !g.allowed(r) {
			return false
		}
	}
	return true
}

// focusScore counts the characters of w that are focus characters or part of a focus bigram.
func (g *gen) focusScore(w string) int {
	rs := []rune(w)
	hit := make([]bool, len(rs))
	for i, r := range rs {
		for _, f := range g.focus {
			if r == f {
				hit[i] = true
			}
		}
	}
	for _, b := range g.bigrams {
		br := []rune(b)
		if len(br) == 0 {
			continue
		}
		for i := 0; i+len(br) <= len(rs); i++ {
			if string(rs[i:i+len(br)]) == b {
				for j := range br {
					hit[i+j] = true
				}
			}
		}
	}
	n := 0
	for _, h := range hit {
		if h {
			n++
		}
	}
	return n
}

// token returns the next word, commit message, pseudo-word or pattern.
func (g *gen) token(first bool) string {
	if len(g.commits) > 0 && (first || g.rng.IntN(4) == 0) {
		return g.pick(g.commits)
	}
	if len(g.focus) == 0 && len(g.bigrams) == 0 {
		if len(g.plain) >= minRealWords || (len(g.plain) > 0 && g.rng.IntN(3) == 0) {
			return g.pick(g.plain)
		}
		return g.pseudo()
	}
	pool := g.loose
	if len(g.strong) >= minRealWords {
		pool = g.strong
	}
	switch {
	case len(pool) >= minRealWords:
		return g.pick(pool)
	case len(pool) > 0 && g.rng.IntN(3) == 0:
		return g.pick(pool)
	case g.rng.IntN(4) == 0:
		return g.pattern()
	default:
		return g.pseudo()
	}
}

func (g *gen) pick(list []string) string { return list[g.rng.IntN(len(list))] }

func pickRune(rng *rand.Rand, rs []rune) rune { return rs[rng.IntN(len(rs))] }

// pseudo builds a pronounceable word of 3 to 6 characters, about half of them focus characters.
func (g *gen) pseudo() string {
	n := 3 + g.rng.IntN(4)
	vowel := g.rng.IntN(2) == 0
	out := make([]rune, 0, n)
	for len(out) < n {
		var r rune
		switch {
		case len(g.focus) > 0 && g.rng.IntN(2) == 0:
			r = pickRune(g.rng, g.focus)
		case len(g.letters) == 0:
			r = pickRune(g.rng, g.symbols)
		case vowel && len(g.vowels) > 0:
			r = pickRune(g.rng, g.vowels)
		case !vowel && len(g.consonant) > 0:
			r = pickRune(g.rng, g.consonant)
		default:
			r = pickRune(g.rng, g.letters)
		}
		out = append(out, r)
		vowel = !vowel
	}
	return string(out)
}

// pattern repeats a short group of focus characters, such as "fjfj" or "ffjj".
func (g *gen) pattern() string {
	pool := g.focus
	if len(pool) == 0 {
		pool = g.symbols
	}
	a, b := pickRune(g.rng, pool), pickRune(g.rng, pool)
	if g.rng.IntN(2) == 0 {
		return string([]rune{a, b, a, b})
	}
	return string([]rune{a, a, b, b})
}
