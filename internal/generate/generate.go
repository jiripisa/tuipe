// Package generate builds deterministic drill text from the embedded material.
package generate

import (
	"math/rand/v2"
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

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
	vowels        = "aeiouy"
	repeatWindow  = 12 // tokens in which a repeat is avoided
	repeatShare   = 7  // tenths of the length that repeated coverage tokens may fill
	shortWord     = 5  // longest word used inside a coverage token
	tinyWord      = 3  // the same when there are enough such words
	wordSample    = 16 // real words weighed for a coverage token
)

// Drill returns text of at least in.Length characters made of tokens separated
// by single spaces, using only characters admitted by in.Allowed. It returns an
// empty string when no character other than space is allowed.
func Drill(lib *content.Library, in Input) string {
	g := newGen(lib, in)
	if g == nil {
		return ""
	}
	return strings.Join(g.fill(nil, g.length, 0), g.sep)
}

// Cover is a drill that is sure to hold the characters of must, each at least
// times times. A coverage loop first writes tokens that hold what is still
// missing, then the drill fills the text up to in.Length, and the tokens are
// shuffled. The first appearance of a character is always written, however long
// the text gets; repeats stop leaving room for the drill. Space, newline and
// characters in.Allowed does not admit are left out of must.
func Cover(lib *content.Library, in Input, must []rune, times int) string {
	g := newGen(lib, in)
	if g == nil {
		return ""
	}
	g.collectCandidates(lib)
	limit := g.length + g.length/2
	tokens := g.fill(g.cover(must, times), g.length, limit)
	g.rng.Shuffle(len(tokens), func(i, j int) { tokens[i], tokens[j] = tokens[j], tokens[i] })
	return strings.Join(tokens, g.sep)
}

// newGen prepares the generator of in. It is nil when no character other than
// space is allowed.
func newGen(lib *content.Library, in Input) *gen {
	allowed := in.Allowed
	if allowed == nil {
		allowed = func(rune) bool { return true }
	}
	g := &gen{
		rng:     rand.New(rand.NewPCG(uint64(in.Seed), uint64(in.Seed)^0x9e3779b97f4a7c15)),
		allowed: allowed,
		length:  in.Length,
		sep:     "",
	}
	if g.length <= 0 {
		g.length = defaultLength
	}
	if allowed(' ') {
		g.sep = " "
	}
	for _, r := range in.Focus {
		if r != ' ' && allowed(r) {
			g.focus = append(g.focus, r)
		}
	}
	for _, b := range in.Bigrams {
		if b != "" && g.within(b) {
			g.bigrams = append(g.bigrams, b)
		}
	}
	if !g.collectAlphabet(lib) {
		return nil
	}
	g.collectPools(lib)
	return g
}

// runeLen is the length of a list of tokens once they are joined.
func (g *gen) runeLen(tokens []string) int {
	n := 0
	for i, t := range tokens {
		if i > 0 {
			n += utf8.RuneCountInString(g.sep)
		}
		n += utf8.RuneCountInString(t)
	}
	return n
}

// fill adds drill tokens to tokens until they reach length. A positive limit
// keeps a long token, such as a commit message, from running past it.
func (g *gen) fill(tokens []string, length, limit int) []string {
	n := g.runeLen(tokens)
	sepLen := utf8.RuneCountInString(g.sep)
	var recent []string
	for i := 0; n < length; i++ {
		tok := g.token(i == 0)
		for try := 0; contains(recent, tok) && try < 8; try++ {
			tok = g.token(false)
		}
		if contains(recent, tok) {
			tok = g.pseudo()
		}
		if limit > 0 && n+sepLen+utf8.RuneCountInString(tok) > limit {
			tok = g.pseudo()
		}
		if tok == "" {
			continue
		}
		if len(tokens) > 0 {
			n += sepLen
		}
		tokens = append(tokens, tok)
		n += utf8.RuneCountInString(tok)
		recent = append(recent, tok)
		if len(recent) > repeatWindow {
			recent = recent[1:]
		}
	}
	return tokens
}

// cover is the coverage loop of Cover. It starts with one token for each bigram.
// Each pass then writes the tokens that hold what the text still lacks; only the
// first pass ignores the length.
func (g *gen) cover(must []rune, times int) []string {
	need := map[rune]int{}
	var order []rune
	for _, r := range must {
		if r == ' ' || r == '\n' || !g.allowed(r) {
			continue
		}
		if _, dup := need[r]; !dup {
			order = append(order, r)
		}
		need[r] = max(times, 1)
	}
	var tokens []string
	for _, b := range g.bigrams {
		tokens = append(tokens, g.tokenWithBigram(b))
	}
	for pass := 0; pass < max(times, 1); pass++ {
		for _, r := range order {
			if need[r] < max(times, 1)-pass {
				continue // an earlier token already held it often enough
			}
			if pass > 0 && g.runeLen(tokens) >= g.length*repeatShare/10 {
				return tokens
			}
			tok := g.tokenWith(r, need)
			tokens = append(tokens, tok)
			for _, c := range tok {
				if need[c] > 0 {
					need[c]--
				}
			}
		}
	}
	return tokens
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// templates turn a real word into a code-like token. WORD is a random short
// English word and DIGIT is one of the focus digits.
var templates = []string{
	"{WORD}", "[WORD]", "(WORD)", `"WORD"`, "WORD;", "@WORD", "#WORD", "$WORD",
	"WORD=WORD", "WORD->WORD", "WORD&&WORD", "~/WORD", "WORD|WORD", `\WORD`,
	"<WORD>", "*WORD", "%WORD", "^WORD", "WORD!", "'WORD'", "`WORD`",
	"WORD.WORD", "WORD:WORD", "WORD?", "WORD+WORD", "WORD-WORD", "WORD_WORD",
	"WORD/WORD", "WORD<WORD", "WORD>WORD", "{WORD:WORD}", "${WORD}", "{WORD}{WORD}",
	"WORD()", "{}", "[]", "()", "[WORD]=WORD",
	"WORDDIGIT", "[DIGIT]", "DIGITDIGITDIGIT", "DIGITDIGITDIGITDIGIT", "WORDDIGITDIGIT",
}

type gen struct {
	rng     *rand.Rand
	allowed func(rune) bool
	focus   []rune
	bigrams []string
	sep     string
	length  int

	letters   []rune // allowed letters, sorted
	vowels    []rune
	consonant []rune
	symbols   []rune // allowed non-space characters, sorted

	strong, loose []string   // real words drilling the focus
	baseWords     []string   // short allowed words used inside templates
	templates     []string   // templates usable with the focus
	upperFocus    []rune     // uppercase focus letters
	upperWords    [][]string // words starting with the lowercase of each upperFocus
	plain         []string   // real words without a focus requirement
	commits       []string   // commit messages drilling a diacritic focus
	diacritic     bool

	byRune map[rune][]string // allowed real words by the characters they hold
	short  []string          // short allowed real words, for coverage tokens
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
		if r >= 'a' && r <= 'z' {
			g.letters = append(g.letters, r)
		}
	}
	if len(g.letters) == 0 { // no plain lowercase letters: take any lowercase letter
		for _, r := range g.symbols {
			if unicode.IsLower(r) {
				g.letters = append(g.letters, r)
			}
		}
	}
	for _, r := range g.letters {
		if strings.ContainsRune(vowels, r) {
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
			if utf8.RuneCountInString(w) < 2 || !g.within(w) {
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
	g.collectDerived(lib)
	if g.diacritic {
		for _, c := range lib.CommitsCS {
			if g.within(c) && g.focusScore(c) > 0 {
				g.commits = append(g.commits, c)
			}
		}
	}
}

// collectDerived prepares the sources for capitalised words and code-like tokens.
func (g *gen) collectDerived(lib *content.Library) {
	for _, r := range g.focus {
		if unicode.IsUpper(r) {
			g.upperFocus = append(g.upperFocus, r)
			var ws []string
			for _, list := range [][]string{lib.WordsEN, lib.WordsCS} {
				for _, w := range list {
					if n := utf8.RuneCountInString(w); n >= 2 && n <= 10 && g.within(w) &&
						strings.HasPrefix(w, string(unicode.ToLower(r))) {
						ws = append(ws, w)
					}
				}
			}
			g.upperWords = append(g.upperWords, ws)
		}
	}
	symbolic := false
	for _, r := range g.focus {
		symbolic = symbolic || !unicode.IsLetter(r)
	}
	for _, b := range g.bigrams {
		for _, r := range b {
			symbolic = symbolic || !unicode.IsLetter(r)
		}
	}
	if !symbolic && len(g.upperFocus) == 0 {
		return
	}
	for _, list := range [][]string{lib.WordsEN, lib.WordsCS} {
		for _, w := range list {
			if n := utf8.RuneCountInString(w); n >= 2 && n <= 5 && g.within(w) {
				g.baseWords = append(g.baseWords, w)
			}
		}
		if len(g.baseWords) >= minRealWords {
			break
		}
	}
	if !symbolic {
		return
	}
	digits := ""
	for _, r := range g.focus {
		if unicode.IsDigit(r) {
			digits += string(r)
		}
	}
	for _, t := range templates {
		fixed := strings.ReplaceAll(t, "WORD", "")
		if strings.Contains(fixed, "DIGIT") {
			if digits == "" {
				continue
			}
			fixed = strings.ReplaceAll(fixed, "DIGIT", digits)
		}
		if strings.Contains(t, "WORD") && len(g.baseWords) == 0 {
			continue
		}
		if g.within(fixed) && g.focusScore(fixed) > 0 {
			g.templates = append(g.templates, t)
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

// collectCandidates indexes the allowed real words for coverage tokens. Upper
// case is reached through the lower-case letter, so that a capital can come from
// a word that exists only in lower case.
func (g *gen) collectCandidates(lib *content.Library) {
	g.byRune = map[rune][]string{}
	for _, list := range [][]string{lib.WordsCS, lib.WordsEN} {
		for _, w := range list {
			if utf8.RuneCountInString(w) < 2 {
				continue
			}
			seen := map[rune]bool{}
			for _, r := range w {
				if !seen[r] {
					seen[r] = true
					g.byRune[r] = append(g.byRune[r], w)
				}
			}
			if g.within(w) && utf8.RuneCountInString(w) <= shortWord {
				g.short = append(g.short, w)
			}
		}
	}
	// The shortest words keep coverage tokens cheap, when there are enough of them.
	tiny := slices.DeleteFunc(slices.Clone(g.short), func(w string) bool {
		return utf8.RuneCountInString(w) > tinyWord
	})
	if len(tiny) >= minRealWords {
		g.short = tiny
	}
}

// tokenWith returns a token that holds r and as many other needed characters
// as it can for its length: a real word when r occurs in one, else a capitalised
// word, a word in capitals, a code-like template or a word with r attached.
func (g *gen) tokenWith(r rune, need map[rune]int) string {
	best, bestScore := "", -1.0
	consider := func(tok string) {
		if tok == "" || !strings.ContainsRune(tok, r) || !g.within(tok) {
			return
		}
		hit, seen := 0, map[rune]bool{}
		for _, c := range tok {
			if need[c] > 0 && !seen[c] {
				seen[c] = true
				hit++
			}
		}
		// The separator counts: one long word beats two short ones.
		if s := float64(hit) / float64(utf8.RuneCountInString(tok)+1); s > bestScore {
			best, bestScore = tok, s
		}
	}
	for _, w := range g.sample(g.byRune[r]) {
		consider(w)
	}
	if unicode.IsUpper(r) {
		for _, w := range g.sample(g.byRune[unicode.ToLower(r)]) {
			if strings.HasPrefix(w, string(unicode.ToLower(r))) {
				consider(capitalise(w))
			}
			consider(strings.ToUpper(w))
		}
	}
	if !unicode.IsLetter(r) {
		for _, t := range templates {
			consider(g.fillTemplate(t, r, need))
		}
	}
	if len(g.short) > 0 && !unicode.IsUpper(r) && !unicode.IsLower(r) {
		consider(g.pick(g.short) + string(r))
		consider(string(r) + g.pick(g.short))
		for _, o := range neededSymbols(need) { // a word between two symbols carries both
			if o != r {
				consider(string(r) + g.pick(g.short) + string(o))
				break
			}
		}
	}
	if best == "" {
		return g.attach(r)
	}
	return best
}

// neededSymbols lists the needed characters that are neither cased letters nor
// digits, sorted, so that the choice stays fixed by the seed.
func neededSymbols(need map[rune]int) []rune {
	var out []rune
	for r, n := range need {
		if n > 0 && !unicode.IsUpper(r) && !unicode.IsLower(r) && !unicode.IsDigit(r) {
			out = append(out, r)
		}
	}
	slices.Sort(out)
	return out
}

// tokenWithBigram is a real word that holds b, or else b alone.
func (g *gen) tokenWithBigram(b string) string {
	r, _ := utf8.DecodeRuneInString(b)
	var words []string
	for _, w := range g.byRune[r] {
		if strings.Contains(w, b) && g.within(w) {
			words = append(words, w)
		}
	}
	if len(words) > 0 {
		return g.pick(words)
	}
	return b
}

// sample is up to wordSample words of list, chosen at random, so that the same
// characters do not always give the same token.
func (g *gen) sample(list []string) []string {
	if len(list) <= wordSample {
		return list
	}
	out := make([]string, wordSample)
	for i := range out {
		out[i] = g.pick(list)
	}
	return out
}

// fillTemplate writes template t with short words. When r is a digit it is the
// first DIGIT, and the others are digits still needed, else r again. It is empty
// when the template cannot be written.
func (g *gen) fillTemplate(t string, r rune, need map[rune]int) string {
	if strings.Contains(t, "WORD") && len(g.short) == 0 {
		return ""
	}
	for strings.Contains(t, "WORD") {
		t = strings.Replace(t, "WORD", g.pick(g.short), 1)
	}
	if !strings.Contains(t, "DIGIT") {
		return t
	}
	if !unicode.IsDigit(r) {
		return ""
	}
	t = strings.Replace(t, "DIGIT", string(r), 1)
	for d := '0'; d <= '9'; d++ {
		if need[d] > 0 && d != r && strings.Contains(t, "DIGIT") {
			t = strings.Replace(t, "DIGIT", string(d), 1)
		}
	}
	return strings.ReplaceAll(t, "DIGIT", string(r))
}

// attach is the last resort: r beside the allowed letters, or alone.
func (g *gen) attach(r rune) string {
	pool := g.letters
	if len(pool) == 0 {
		pool = g.symbols
	}
	if len(pool) == 0 {
		return string(r)
	}
	return string(pickRune(g.rng, pool)) + string(r)
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
	if (len(g.templates) > 0 || g.hasUpper()) && (len(pool) < minRealWords || g.rng.IntN(2) == 0) {
		if d := g.derived(); d != "" {
			return d
		}
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

func (g *gen) hasUpper() bool {
	for _, ws := range g.upperWords {
		if len(ws) > 0 {
			return true
		}
	}
	return false
}

// derived builds a capitalised word, a camelCase identifier or a code-like
// token from real words. It returns "" when nothing fits.
func (g *gen) derived() string {
	for try := 0; try < 10; try++ {
		var tok string
		if g.hasUpper() && (len(g.templates) == 0 || g.rng.IntN(2) == 0) {
			k := g.rng.IntN(len(g.upperWords))
			if len(g.upperWords[k]) == 0 {
				continue
			}
			w := capitalise(g.pick(g.upperWords[k]))
			tok = w
			if len(g.baseWords) > 0 && g.rng.IntN(2) == 0 {
				tok = g.pick(g.baseWords) + w
			}
		} else if len(g.templates) > 0 {
			tok = g.pick(g.templates)
			for strings.Contains(tok, "WORD") {
				tok = strings.Replace(tok, "WORD", g.pick(g.baseWords), 1)
			}
			for strings.Contains(tok, "DIGIT") {
				tok = strings.Replace(tok, "DIGIT", string(pickRune(g.rng, g.digits())), 1)
			}
		} else {
			continue
		}
		if g.within(tok) && g.focusScore(tok) > 0 && !strings.Contains(tok, " ") {
			return tok
		}
	}
	return ""
}

func (g *gen) digits() []rune {
	var ds []rune
	for _, r := range g.focus {
		if unicode.IsDigit(r) {
			ds = append(ds, r)
		}
	}
	return ds
}

func capitalise(w string) string {
	r, n := utf8.DecodeRuneInString(w)
	return string(unicode.ToUpper(r)) + w[n:]
}

// pseudo builds a pronounceable word of 3 to 6 lowercase characters, about a
// third of them focus characters, with a bigram spliced in when there are any.
func (g *gen) pseudo() string {
	var out []rune
	for try := 0; try < 5; try++ {
		n := 3 + g.rng.IntN(4)
		vowel := g.rng.IntN(2) == 0
		out = out[:0]
		for len(out) < n {
			var r rune
			switch {
			case len(g.focus) > 0 && g.rng.IntN(3) == 0:
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
		if len(g.bigrams) > 0 && g.rng.IntN(4) != 0 {
			b := []rune(g.pick(g.bigrams))
			at := g.rng.IntN(len(out) + 1)
			out = append(out[:at], append(b, out[at:]...)...)
		}
		if distinct(out) >= 2 {
			break
		}
	}
	return string(out)
}

func distinct(rs []rune) int {
	seen := map[rune]bool{}
	for _, r := range rs {
		seen[r] = true
	}
	return len(seen)
}

// pattern repeats a short group of focus characters, such as "fjkfj" or "fafa".
func (g *gen) pattern() string {
	if len(g.bigrams) > 0 && g.rng.IntN(2) == 0 {
		b := g.pick(g.bigrams)
		if g.rng.IntN(2) == 0 {
			return b + b
		}
		other := g.letters
		if len(other) == 0 {
			other = g.symbols
		}
		return b + string(pickRune(g.rng, other)) + b
	}
	pool := g.focus
	if len(pool) == 0 {
		pool = g.symbols
	}
	a, b := pickRune(g.rng, pool), pickRune(g.rng, pool)
	if a == b {
		other := g.letters
		if len(other) == 0 {
			other = g.symbols
		}
		b = pickRune(g.rng, other)
	}
	if g.rng.IntN(2) == 0 {
		return string([]rune{a, b, a, b})
	}
	return string([]rune{a, a, b, b})
}
