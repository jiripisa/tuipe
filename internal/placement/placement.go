// Package placement defines the placement test (Vstupní pohovor): short stages
// of rising difficulty, one per chapter group, and what their result grants.
// It never reads the clock or global randomness.
package placement

import (
	"slices"
	"strings"
	"unicode"

	"github.com/jiripisa/tuipe/internal/content"
	"github.com/jiripisa/tuipe/internal/curriculum"
	"github.com/jiripisa/tuipe/internal/generate"
	"github.com/jiripisa/tuipe/internal/score"
	"github.com/jiripisa/tuipe/internal/store"
)

const (
	minAccuracy  = 0.9
	abortMinChar = 20   // characters typed before a stage may be aborted
	abortWrong   = 0.25 // wrong share above which it is
	codeMaxChars = 70   // the code stage is whole lines up to this length
	minCapitals  = 5    // capitals a stage with capitals must show
	codeLang     = "go"
)

// Stage is one part of the placement test.
type Stage struct {
	ID, Title, Text     string
	MinWPM, MinAccuracy float64
	Chapters            []string // chapters passing the stage completes
}

// Outcome is the result of the test.
type Outcome struct {
	Completed    []string // chapters to complete, in order
	StartChapter string
	Rank         curriculum.Rank
}

type group struct {
	id, title   string
	first, last int // chapter numbers
	length      int // characters of text
	minWPM      float64
}

// Lengths give each stage about 30 s at its minimum speed before coverage,
// which writes whatever the group still lacks; the code stage is whole lines of
// a snippet.
var groups = []group{
	{"letters", "Letters", 0, 4, 50, 20},
	{"diacritics", "Diacritics", 5, 5, 38, 15},
	{"digits", "Digits", 6, 6, 20, 12},
	{"shift", "Shift symbols", 7, 7, 20, 12},
	{"option", "Option symbols", 8, 10, 25, 10},
	{"code", "Real code", 11, 11, 0, 12},
}

// Stages returns the six stages in order. The text of each is fixed by seed,
// uses only characters unlocked up to the last chapter of its group and covers
// that group, see required.
// Hints are off during the test; the UI applies that.
func Stages(c *curriculum.Curriculum, lib *content.Library, seed int64) []Stage {
	chapters := c.Chapters()
	var stages []Stage
	for i, g := range groups {
		st := Stage{ID: g.id, Title: g.title, MinWPM: g.minWPM, MinAccuracy: minAccuracy}
		lastID := ""
		for _, ch := range chapters {
			if ch.Number >= g.first && ch.Number <= g.last {
				st.Chapters = append(st.Chapters, ch.ID)
				lastID = ch.ID
			}
		}
		unlocked := c.AllowedAt(lastID)
		allowed := func(r rune) bool { return unlocked(r) && c.CanType(r) }
		if g.id == "code" {
			st.Text = codeText(lib, allowed, seed+int64(i))
		} else {
			focus := introduced(chapters, g.first, g.last)
			st.Text = generate.Cover(lib, generate.Input{
				Focus:   focus,
				Allowed: allowed,
				Length:  g.length,
				Seed:    seed + int64(i),
			}, required(focus, seed), 1)
		}
		stages = append(stages, st)
	}
	return stages
}

// introduced lists the characters chapters first..last introduce.
func introduced(chapters []content.ChapterDef, first, last int) []rune {
	var rs []rune
	for _, ch := range chapters {
		if ch.Number >= max(first, 1) && ch.Number <= last {
			rs = append(rs, []rune(ch.Introduces)...)
		}
	}
	return rs
}

// required is what a stage must show of its group: every lower-case letter and
// symbol, and a handful of capitals that changes with the seed. A capital has
// no key of its own, so that handful proves the pattern. Uppercase-only entries
// bring their lower case with them: ů is taught early, Ů with the diacritics.
func required(group []rune, seed int64) []rune {
	var must, capitals []rune
	for _, r := range group {
		if unicode.IsUpper(r) {
			capitals = append(capitals, r)
			r = unicode.ToLower(r)
		}
		if !slices.Contains(must, r) {
			must = append(must, r)
		}
	}
	if len(capitals) == 0 {
		return must
	}
	from := int(uint64(seed) % uint64(len(capitals)))
	n := min(minCapitals, len(capitals))
	for i := range n {
		must = append(must, capitals[(from+i*len(capitals)/n)%len(capitals)])
	}
	return must
}

// codeText is the first two lines of a typable Go snippet: Enter is what the
// code group adds, so one line would prove nothing. Snippets that open with a
// comment, have a blank second line or whose two lines run over 70 characters
// are passed over while others are left.
func codeText(lib *content.Library, allowed func(rune) bool, seed int64) string {
	var fits, typable []string
	for _, sn := range lib.Snippets {
		if sn.Lang != codeLang || slices.ContainsFunc([]rune(sn.Text), func(r rune) bool { return r != '\n' && !allowed(r) }) {
			continue
		}
		lines := strings.SplitN(sn.Text, "\n", 3)
		text := strings.Join(lines[:min(2, len(lines))], "\n")
		typable = append(typable, text)
		twoLines := len(lines) > 1 && strings.TrimSpace(lines[1]) != ""
		if twoLines && len([]rune(text)) <= codeMaxChars && !strings.HasPrefix(text, "//") {
			fits = append(fits, text)
		}
	}
	if len(fits) > 0 {
		typable = fits
	}
	if len(typable) == 0 {
		return ""
	}
	return typable[uint64(seed)%uint64(len(typable))]
}

// StageSucceeded reports whether a stage was typed fast and cleanly enough.
func StageSucceeded(st Stage, m score.Metrics) bool {
	return m.WPM >= st.MinWPM && m.Accuracy >= st.MinAccuracy
}

// AbortEarly reports whether a stage goes so badly that it should end now:
// at least 20 characters typed and over a quarter of them wrong.
func AbortEarly(chars, wrong int) bool {
	return chars >= abortMinChar && float64(wrong)/float64(chars) > abortWrong
}

// Evaluate turns the passed stages into the chapters to complete, the chapter
// to start at (the first one not completed, the last chapter when all are) and
// the rank the completed chapters hold.
func Evaluate(c *curriculum.Curriculum, passed []Stage) Outcome {
	done := map[string]bool{}
	for _, st := range passed {
		for _, id := range st.Chapters {
			done[id] = true
		}
	}
	var out Outcome
	profile := &store.Profile{}
	chapters := c.Chapters()
	for _, ch := range chapters {
		if done[ch.ID] {
			out.Completed = append(out.Completed, ch.ID)
			c.CompleteChapter(profile, ch.ID)
		} else if out.StartChapter == "" {
			out.StartChapter = ch.ID
		}
	}
	if out.StartChapter == "" && len(chapters) > 0 {
		out.StartChapter = chapters[len(chapters)-1].ID
	}
	out.Rank = c.RankOf(profile)
	return out
}
