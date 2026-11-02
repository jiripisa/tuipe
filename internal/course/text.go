package course

import (
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"strings"

	"github.com/jiripisa/tuipe/internal/generate"
	"github.com/jiripisa/tuipe/internal/score"
)

// Length is how long a Trénink runs.
type Length int

// Lengths, shortest first.
const (
	Short Length = iota
	Medium
	Long
)

var lengthNames = []string{"short", "medium", "long"}

// trainingSeconds is the time of each Length at the lesson's two-star speed.
var trainingSeconds = []int{30, 60, 120}

const (
	testSeconds  = 90
	testSnippets = 3
	focusRepeats = 3  // how often a text holds each focus character, at least
	focusSpacing = 10 // characters of a Trénink for each token written for a focus character
	wordLength   = 5  // characters in a word of WPM
	earlierShare = 6  // a Test holds earlier characters up to this fraction of its length
	seedMix      = 0x9e3779b97f4a7c15
)

// String returns the name stored on runs.
func (l Length) String() string {
	if l < Short || l > Long {
		return lengthNames[Short]
	}
	return lengthNames[l]
}

// characters is the length of a text that takes seconds at the two-star speed of ls.
func characters(ls Lesson, seconds int) int {
	return int(math.Ceil(float64(seconds) * ls.Speed[1] * wordLength / 60))
}

// Training returns the text of a Trénink. Code lessons join n+1 snippets. The
// others hold every focus character, and a lesson that teaches them keeps earlier
// characters to a supporting role: at least a quarter of its text is focus. A
// review lesson has no such floor.
func (c *Course) Training(id string, n Length, seed int64) (string, error) {
	ls, ok := c.Lesson(id)
	if !ok {
		return "", fmt.Errorf("course: unknown lesson %q", id)
	}
	if n < Short || n > Long {
		return "", fmt.Errorf("course: unknown length %d", int(n))
	}
	if ls.Lang != "" {
		return c.codeText(ls, int(n)+1, seed), nil
	}
	length := characters(ls, trainingSeconds[n])
	return c.drillText(ls, generate.Input{Length: length, Seed: seed, Dense: !ls.Review},
		max(focusRepeats, length/(focusSpacing*len(ls.Focus))))
}

// Test returns the text of a Test and its star targets. Code lessons join
// snippets of their language. The others hold the lesson's characters and a
// sample of the earlier ones: over attempts all of them appear.
func (c *Course) Test(id string, seed int64) (string, score.Targets, error) {
	ls, ok := c.Lesson(id)
	if !ok {
		return "", score.Targets{}, fmt.Errorf("course: unknown lesson %q", id)
	}
	targets := score.Targets{Speed: ls.Speed}
	if ls.Lang != "" {
		return c.codeText(ls, testSnippets, seed), targets, nil
	}
	length := characters(ls, testSeconds)
	text, err := c.drillText(ls, generate.Input{
		Length: length,
		Seed:   seed,
		Once:   c.earlierSample(ls, length/earlierShare, seed),
	}, focusRepeats)
	return text, targets, err
}

// drillText is a Cover of in over the focus of ls, each character times times.
func (c *Course) drillText(ls Lesson, in generate.Input, times int) (string, error) {
	in.Focus, in.Allowed = ls.Focus, c.AllowedUpTo(ls.ID)
	text := generate.Cover(c.lib, in, ls.Focus, times)
	if text == "" {
		return "", fmt.Errorf("course: lesson %s: no text can be generated", ls.ID)
	}
	return text, nil
}

// earlierSample is up to n characters taught before ls and not by it, picked by seed.
func (c *Course) earlierSample(ls Lesson, n int, seed int64) []rune {
	var list []rune
	for _, e := range c.lessons[:ls.Index] {
		for _, r := range e.Focus {
			if r != ' ' && !slices.Contains(ls.Focus, r) && !slices.Contains(list, r) {
				list = append(list, r)
			}
		}
	}
	shuffle(list, seed)
	return list[:min(n, len(list))]
}

// codeText joins n random snippets of the lesson's language, or as many as there
// are. Enter joins them, so a layout without it gets one snippet.
func (c *Course) codeText(ls Lesson, n int, seed int64) string {
	pool := slices.Clone(c.snippets[ls.Lang])
	shuffle(pool, seed)
	if !c.CanType('\n') {
		n = 1
	}
	parts := make([]string, 0, n)
	for _, s := range pool[:min(n, len(pool))] {
		parts = append(parts, s.Text)
	}
	return strings.Join(parts, "\n")
}

// shuffle orders list by seed.
func shuffle[T any](list []T, seed int64) {
	rng := rand.New(rand.NewPCG(uint64(seed), uint64(seed)^seedMix))
	rng.Shuffle(len(list), func(i, j int) { list[i], list[j] = list[j], list[i] })
}
