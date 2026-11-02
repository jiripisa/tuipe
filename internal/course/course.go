// Package course is the lesson model: chapters of lessons in the order they
// are taught, which of them a keyboard layout can type, the stars and rank
// earned, the lesson to open next, and the text of each Trénink and Test.
package course

import (
	"github.com/jiripisa/tuipe/internal/content"
	"github.com/jiripisa/tuipe/internal/layout"
)

// Lesson is one lesson of the course. Focus holds the characters it teaches;
// it is empty for a lesson with a Lang, which types real code.
type Lesson struct {
	ID, Title, ChapterID, Lang, Intro string
	Focus                             []rune
	Review                            bool
	Speed                             [3]float64
	Index                             int // position in the course among lessons this layout can type
}

// Chapter is a group of lessons.
type Chapter struct {
	ID, Title string
	Lessons   []Lesson
}

// Rank is the job title earned by stars.
type Rank int

// Ranks, lowest first.
const (
	Intern Rank = iota
	Junior
	Medior
	Senior
	Staff
	Principal
)

var rankNames = []string{"Intern", "Junior", "Medior", "Senior", "Staff", "Principal"}

// rankFrom is the share of all stars, in percent, from which a rank is held.
var rankFrom = []int{0, 10, 30, 55, 80, 95}

const maxStars = 3

// String returns the English job title.
func (r Rank) String() string {
	if r < 0 || int(r) >= len(rankNames) {
		return "Intern"
	}
	return rankNames[r]
}

// Course is the lessons a layout can type.
type Course struct {
	lib        *content.Library
	lay        *layout.Layout
	snippets   map[string][]content.Snippet // the typable snippets of each language
	chapterIDs []string                     // every chapter, also one whose lessons are all left out
	chapters   []Chapter
	lessons    []Lesson
	chapterPos map[string]int
	byID       map[string]int
	skipped    int
}

// New builds the course from lib. A lesson is left out, and counted by
// Skipped, when its focus has a character l cannot type or its language has
// no typable snippet. A nil layout types everything.
func New(lib *content.Library, l *layout.Layout) *Course {
	c := &Course{
		lib: lib, lay: l, snippets: map[string][]content.Snippet{},
		chapterPos: map[string]int{}, byID: map[string]int{},
	}
	for _, s := range lib.Snippets {
		if c.typable(s.Text) {
			c.snippets[s.Lang] = append(c.snippets[s.Lang], s)
		}
	}
	for pos, ch := range lib.Course {
		c.chapterIDs = append(c.chapterIDs, ch.ID)
		c.chapterPos[ch.ID] = pos
		out := Chapter{ID: ch.ID, Title: ch.Title}
		for _, def := range ch.Lessons {
			if !c.typable(def.Focus) || (def.Lang != "" && len(c.snippets[def.Lang]) == 0) {
				c.skipped++
				continue
			}
			ls := Lesson{
				ID: def.ID, Title: def.Title, ChapterID: ch.ID, Lang: def.Lang, Intro: def.Intro,
				Focus: []rune(def.Focus), Review: def.Review, Speed: def.Speed, Index: len(c.lessons),
			}
			c.byID[ls.ID] = ls.Index
			c.lessons = append(c.lessons, ls)
			out.Lessons = append(out.Lessons, ls)
		}
		if len(out.Lessons) > 0 {
			c.chapters = append(c.chapters, out)
		}
	}
	return c
}

func (c *Course) typable(text string) bool {
	return c.lay == nil || len(c.lay.Unresolved(text)) == 0
}

// CanType reports whether the layout has a route for r.
func (c *Course) CanType(r rune) bool {
	if c.lay == nil {
		return true
	}
	_, ok := c.lay.Route(r)
	return ok
}

// Chapters returns the chapters that have at least one typable lesson.
func (c *Course) Chapters() []Chapter {
	out := make([]Chapter, len(c.chapters))
	for i, ch := range c.chapters {
		out[i] = Chapter{ID: ch.ID, Title: ch.Title, Lessons: append([]Lesson(nil), ch.Lessons...)}
	}
	return out
}

// Lessons returns every typable lesson in teaching order.
func (c *Course) Lessons() []Lesson { return append([]Lesson(nil), c.lessons...) }

// Lesson returns the typable lesson with the given id.
func (c *Course) Lesson(id string) (Lesson, bool) {
	i, ok := c.byID[id]
	if !ok {
		return Lesson{}, false
	}
	return c.lessons[i], true
}

// Skipped is the number of lessons left out because the layout cannot type them.
func (c *Course) Skipped() int { return c.skipped }

// allowed returns the characters of the lessons for which keep is true, plus
// space, and Enter once a code lesson is among them. Only typable characters
// are admitted.
func (c *Course) allowed(keep func(Lesson) bool) func(rune) bool {
	set := map[rune]bool{' ': true}
	for _, ls := range c.lessons {
		if !keep(ls) {
			continue
		}
		for _, r := range ls.Focus {
			if c.CanType(r) {
				set[r] = true
			}
		}
		if ls.Lang != "" {
			set['\n'] = true
		}
	}
	return func(r rune) bool { return set[r] }
}

// AllowedUpTo admits the focus of the lesson and of every lesson before it,
// typable characters only, plus space. A code lesson admits Enter too. An
// unknown lesson admits space only.
func (c *Course) AllowedUpTo(lessonID string) func(rune) bool {
	at, ok := c.byID[lessonID]
	if !ok {
		return c.allowed(func(Lesson) bool { return false })
	}
	return c.allowed(func(ls Lesson) bool { return ls.Index <= at })
}

// ChapterAllowed is AllowedUpTo for the end of a chapter, which placement
// uses. An unknown chapter admits space only.
func (c *Course) ChapterAllowed(chapterID string) func(rune) bool {
	at, ok := c.chapterPos[chapterID]
	if !ok {
		return c.allowed(func(Lesson) bool { return false })
	}
	return c.allowed(func(ls Lesson) bool { return c.chapterPos[ls.ChapterID] <= at })
}

// Stars counts the stars earned on lessons of this course, up to three each,
// and the stars there are in all.
func (c *Course) Stars(stars map[string]int) (got, total int) {
	for _, ls := range c.lessons {
		got += min(max(stars[ls.ID], 0), maxStars)
	}
	return got, maxStars * len(c.lessons)
}

// Rank is the highest rank whose share of all stars is reached.
func (c *Course) Rank(stars map[string]int) Rank {
	got, total := c.Stars(stars)
	rank := Intern
	for i, pct := range rankFrom {
		if total > 0 && got*100 >= pct*total {
			rank = Rank(i)
		}
	}
	return rank
}

// Next is the first lesson at or after recommended that has no star, else the
// first lesson without a star anywhere. It reports false when every lesson has
// a star. An unknown recommended counts as none.
func (c *Course) Next(stars map[string]int, recommended string) (Lesson, bool) {
	from := c.byID[recommended] // 0 when unknown
	for _, ls := range c.lessons[from:] {
		if stars[ls.ID] <= 0 {
			return ls, true
		}
	}
	for _, ls := range c.lessons[:from] {
		if stars[ls.ID] <= 0 {
			return ls, true
		}
	}
	return Lesson{}, false
}
