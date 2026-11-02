package course

import (
	"fmt"
	"slices"
	"strings"

	"github.com/jiripisa/tuipe/internal/generate"
	"github.com/jiripisa/tuipe/internal/hint"
	"github.com/jiripisa/tuipe/internal/layout"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

const (
	repeatCount = 8  // tokens in the line of one character
	pairCount   = 3  // times a pair and its reverse are written
	lineLength  = 40 // characters of the closing line
)

// Tutorial is the teaching steps of a lesson: what to learn and how, the keys
// to show, and the lines to type.
type Tutorial struct {
	Explain []string // one line per character, or the Intro of a review or code lesson
	Keys    []string // key ids to highlight in the drawing
	Lines   []string // the lines to type, in order
}

// Tutorial builds the teaching steps of a lesson. A lesson that teaches
// characters explains each one, left hand first, then has a line of each
// character, a line of each neighbouring pair and one line of the focus in
// words. A review lesson has its Intro and one line, a code lesson its Intro
// and one snippet. The seed varies the closing line and the snippet.
func (c *Course) Tutorial(id string, seed int64) (Tutorial, error) {
	ls, ok := c.Lesson(id)
	if !ok {
		return Tutorial{}, fmt.Errorf("course: unknown lesson %q", id)
	}
	if ls.Lang != "" {
		code := c.codeText(ls, 1, seed)
		if code == "" {
			return Tutorial{}, fmt.Errorf("course: lesson %s: no snippet", ls.ID)
		}
		return Tutorial{Explain: []string{ls.Intro}, Lines: []string{code}}, nil
	}
	focus := c.handOrder(ls.Focus)
	closing, err := c.drillText(ls, generate.Input{Length: lineLength, Seed: seed, Dense: !ls.Review}, 1)
	if err != nil {
		return Tutorial{}, err
	}
	t := Tutorial{Keys: c.keysOf(focus)}
	if ls.Review {
		t.Explain = []string{ls.Intro}
		t.Lines = []string{closing}
		return t, nil
	}
	t.Explain = append([]string{text.TutorialFocus(focus)}, c.explanations(focus)...)
	letters := slices.DeleteFunc(slices.Clone(focus), func(r rune) bool { return r == ' ' })
	for _, r := range letters {
		t.Lines = append(t.Lines, strings.TrimSpace(strings.Repeat(string(r)+" ", repeatCount)))
	}
	for i := 0; i+1 < len(letters); i++ {
		a, b := string(letters[i]), string(letters[i+1])
		t.Lines = append(t.Lines, strings.TrimSpace(strings.Repeat(a+b+" "+b+a+" ", pairCount)))
	}
	t.Lines = append(t.Lines, closing)
	return t, nil
}

// handOrder puts the characters typed by the left hand first, then those of
// the right hand, then space. The order within a hand is kept.
func (c *Course) handOrder(focus []rune) []rune {
	if c.lay == nil {
		return slices.Clone(focus)
	}
	order := slices.Clone(focus)
	rank := func(r rune) int {
		switch {
		case r == ' ':
			return 2
		case c.lastKey(r).Hand == layout.Left:
			return 0
		}
		return 1
	}
	slices.SortStableFunc(order, func(a, b rune) int { return rank(a) - rank(b) })
	return order
}

// lastKey is the key pressed last to type r, the zero Key when there is none.
func (c *Course) lastKey(r rune) layout.Key {
	route, ok := c.lay.Route(r)
	if !ok || len(route) == 0 {
		return layout.Key{}
	}
	k, _ := c.lay.Key(route[len(route)-1].Key)
	return k
}

// keysOf is the id of the last key of each character's route, without repeats.
// Modifiers are not layout keys, so they are not listed.
func (c *Course) keysOf(focus []rune) []string {
	if c.lay == nil {
		return nil
	}
	var keys []string
	for _, r := range focus {
		if k := c.lastKey(r); k.ID != "" && !slices.Contains(keys, k.ID) {
			keys = append(keys, k.ID)
		}
	}
	return keys
}

// explanations is how to type each character of focus.
func (c *Course) explanations(focus []rune) []string {
	if c.lay == nil {
		return nil
	}
	var out []string
	for _, r := range focus {
		if h := hint.Text(c.lay, r); h != "" {
			out = append(out, h)
		}
	}
	return out
}
