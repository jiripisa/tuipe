// Package curriculum turns the embedded chapters into lessons, test-outs and
// progress rules: which lesson comes next, which chapters are open, what
// skipping a chapter grants, and the rank the user holds. It never reads the
// clock or global randomness; all text is fixed by seeds.
package curriculum

import (
	"fmt"
	"strings"

	"github.com/jiripisa/tuipe/internal/content"
	"github.com/jiripisa/tuipe/internal/generate"
	"github.com/jiripisa/tuipe/internal/layout"
	"github.com/jiripisa/tuipe/internal/score"
	"github.com/jiripisa/tuipe/internal/stats"
	"github.com/jiripisa/tuipe/internal/store"
)

// Rank is the job title earned by completing chapters.
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

// String returns the English job title, which is also its name in the data.
func (r Rank) String() string {
	if r < 0 || int(r) >= len(rankNames) {
		return "Intern"
	}
	return rankNames[r]
}

func parseRank(name string) Rank {
	for i, n := range rankNames {
		if n == name {
			return Rank(i)
		}
	}
	return Intern
}

const (
	testOutLength = 120
	// Skipped characters that were typed badly enough in the test stay learning.
	weakMinSamples = 3
	weakErrorRate  = 0.2
)

// Curriculum is the chapter list resolved against a layout.
type Curriculum struct {
	lib      *content.Library
	lay      *layout.Layout
	raw      []content.ChapterDef // as in the data, for targets and test-outs
	chapters []content.ChapterDef // only lessons that have text
	index    map[string]int       // chapter id to position
	allowed  []map[rune]bool      // cumulative characters by chapter position
	texts    map[string]string    // lesson id to text
	errs     map[string]error     // lesson id to why it has no text
	targets  map[string][3]float64
	incident map[string]content.Incident // chapter id to a typable incident
}

// New resolves the curriculum of lib against l. A lesson whose text cannot be
// built or typed on l is left out, and so is an incident with untypable text.
func New(lib *content.Library, l *layout.Layout) *Curriculum {
	c := &Curriculum{
		lib: lib, lay: l, raw: lib.Chapters,
		index:    map[string]int{},
		texts:    map[string]string{},
		errs:     map[string]error{},
		targets:  map[string][3]float64{},
		incident: map[string]content.Incident{},
	}
	snippets := map[string]content.Snippet{}
	for _, s := range lib.Snippets {
		snippets[s.ID] = s
	}
	incidents := map[string]content.Incident{}
	for _, inc := range lib.Incidents {
		incidents[inc.ID] = inc
	}

	cumulative := map[rune]bool{}
	for i, ch := range c.raw {
		c.index[ch.ID] = i
		before := cloneSet(cumulative)
		for _, r := range ch.Introduces + ch.Allowed {
			cumulative[r] = true
		}
		c.allowed = append(c.allowed, cloneSet(cumulative))

		// Early lessons use only what earlier chapters taught plus the focus
		// of the lessons so far, so they stay small.
		known := before
		out := ch
		out.Lessons = nil
		for _, ls := range ch.Lessons {
			c.targets[ls.ID] = ls.Speed
			for _, r := range ls.Focus {
				if cumulative[r] {
					known[r] = true
				}
			}
			known[' '] = cumulative[' ']
			text, err := c.lessonText(ls, known, snippets)
			if err != nil {
				c.errs[ls.ID] = err
				continue
			}
			c.texts[ls.ID] = text
			out.Lessons = append(out.Lessons, ls)
		}
		c.chapters = append(c.chapters, out)

		if inc, ok := incidents[ch.IncidentID]; ok && c.typable(incidentText(inc)) {
			c.incident[ch.ID] = inc
		}
	}
	return c
}

func cloneSet(m map[rune]bool) map[rune]bool {
	out := make(map[rune]bool, len(m))
	for r := range m {
		out[r] = true
	}
	return out
}

func incidentText(inc content.Incident) string {
	var sb strings.Builder
	for _, s := range inc.Steps {
		sb.WriteString(s.Text)
	}
	return sb.String()
}

func (c *Curriculum) typable(text string) bool {
	return c.lay == nil || len(c.lay.Unresolved(text)) == 0
}

// CanType reports whether the layout has a route for r.
func (c *Curriculum) CanType(r rune) bool {
	if c.lay == nil {
		return true
	}
	_, ok := c.lay.Route(r)
	return ok
}

func (c *Curriculum) lessonText(ls content.LessonDef, known map[rune]bool, snippets map[string]content.Snippet) (string, error) {
	if len(ls.Snippets) > 0 {
		parts := make([]string, 0, len(ls.Snippets))
		for _, id := range ls.Snippets {
			s, ok := snippets[id]
			if !ok {
				return "", fmt.Errorf("curriculum: lesson %s: snippet %q not found", ls.ID, id)
			}
			parts = append(parts, s.Text)
		}
		text := strings.Join(parts, "\n")
		if !c.typable(text) {
			return "", fmt.Errorf("curriculum: lesson %s: text cannot be typed on this layout", ls.ID)
		}
		return text, nil
	}
	text := generate.Drill(c.lib, generate.Input{
		Focus:   []rune(ls.Focus),
		Allowed: func(r rune) bool { return known[r] && c.CanType(r) },
		Length:  ls.Length,
		Seed:    ls.Seed,
	})
	if text == "" {
		return "", fmt.Errorf("curriculum: lesson %s: no text can be generated", ls.ID)
	}
	return text, nil
}

// Chapters returns the chapters in order, with the lessons that cannot be
// built or typed removed.
func (c *Curriculum) Chapters() []content.ChapterDef {
	return append([]content.ChapterDef(nil), c.chapters...)
}

// AllowedAt reports the characters unlocked at the end of a chapter: what it
// introduces and what every chapter before it introduced. An unknown chapter
// allows nothing.
func (c *Curriculum) AllowedAt(chapterID string) func(rune) bool {
	i, ok := c.index[chapterID]
	if !ok {
		return func(rune) bool { return false }
	}
	set := c.allowed[i]
	return func(r rune) bool { return set[r] }
}

// LessonText returns the fixed text of a lesson.
func (c *Curriculum) LessonText(lessonID string) (string, error) {
	if text, ok := c.texts[lessonID]; ok {
		return text, nil
	}
	if err, ok := c.errs[lessonID]; ok {
		return "", err
	}
	return "", fmt.Errorf("curriculum: unknown lesson %q", lessonID)
}

// Targets returns the star speeds of a lesson; unknown lessons have none.
func (c *Curriculum) Targets(lessonID string) score.Targets {
	return score.Targets{Speed: c.targets[lessonID]}
}

// TestOutText returns the test-out of a chapter: drill text on what the
// chapter introduces, fixed by the chapter's test-out seed, and the targets of
// its last lesson. An unknown chapter gives no text.
func (c *Curriculum) TestOutText(chapterID string) (string, score.Targets) {
	i, ok := c.index[chapterID]
	if !ok {
		return "", score.Targets{}
	}
	ch := c.raw[i]
	set := c.allowed[i]
	text := generate.Drill(c.lib, generate.Input{
		Focus:   []rune(ch.Introduces),
		Allowed: func(r rune) bool { return set[r] && c.CanType(r) },
		Length:  testOutLength,
		Seed:    ch.TestOutSeed,
	})
	// Targets come from the last lesson, or from the nearest earlier chapter
	// that has lessons.
	for j := i; j >= 0; j-- {
		if n := len(c.raw[j].Lessons); n > 0 {
			return text, score.Targets{Speed: c.raw[j].Lessons[n-1].Speed}
		}
	}
	return text, score.Targets{}
}

// IncidentDef returns the incident that ends a chapter. It is false for a
// chapter without one, for an incident that does not exist, and for one that
// cannot be typed on the layout; such a chapter completes through its lessons.
func (c *Curriculum) IncidentDef(chapterID string) (content.Incident, bool) {
	inc, ok := c.incident[chapterID]
	return inc, ok
}

// empty reports whether a chapter has nothing to play: no lesson and no
// incident. Progress must never wait on such a chapter.
func (c *Curriculum) empty(i int) bool {
	ch := c.chapters[i]
	_, hasIncident := c.incident[ch.ID]
	return len(ch.Lessons) == 0 && !hasIncident
}

// done reports whether the chapter at position i is completed. A chapter with
// an incident is done when the incident was won (at SEV3 or harder, which is
// all the profile records) and its lessons have stars. Otherwise its last lesson needs one star. A
// chapter with nothing to play counts as done.
func (c *Curriculum) done(p *store.Profile, i int) bool {
	ch := c.chapters[i]
	if p.Completed[ch.ID] || c.empty(i) {
		return true
	}
	if inc, ok := c.incident[ch.ID]; ok {
		// A win before the lessons (free play) is recorded but closes nothing.
		return p.IncidentBest[inc.ID] != "" && allStarred(p, ch.Lessons)
	}
	n := len(ch.Lessons)
	return p.Stars[ch.Lessons[n-1].ID] >= 1
}

// allStarred reports whether every lesson has a star.
func allStarred(p *store.Profile, lessons []content.LessonDef) bool {
	for _, l := range lessons {
		if p.Stars[l.ID] == 0 {
			return false
		}
	}
	return true
}

// earned is done without the vacuous case: the chapter was really completed.
func (c *Curriculum) earned(p *store.Profile, i int) bool {
	return c.done(p, i) && (p.Completed[c.chapters[i].ID] || !c.empty(i))
}

// current is the position of the first chapter that is not done. Onboarding
// is passed over once any later chapter is done. When all are done it is the
// last chapter, and -1 when there are no chapters.
func (c *Curriculum) current(p *store.Profile) int {
	laterDone := false
	for i, ch := range c.chapters {
		if ch.Number > 0 && c.earned(p, i) {
			laterDone = true
		}
	}
	for i, ch := range c.chapters {
		if ch.Number == 0 && laterDone {
			continue
		}
		if !c.done(p, i) {
			return i
		}
	}
	return len(c.chapters) - 1
}

// CurrentChapter is the first chapter that is not completed.
func (c *Curriculum) CurrentChapter(p *store.Profile) content.ChapterDef {
	if i := c.current(p); i >= 0 {
		return c.chapters[i]
	}
	return content.ChapterDef{}
}

// NextLesson is the first lesson without a star in the current chapter. It is
// false when the current chapter has no lesson left, which means its incident
// is owed (see IncidentDef), or when the curriculum is finished.
func (c *Curriculum) NextLesson(p *store.Profile) (content.LessonDef, bool) {
	i := c.current(p)
	if i < 0 || c.done(p, i) {
		return content.LessonDef{}, false
	}
	for _, l := range c.chapters[i].Lessons {
		if p.Stars[l.ID] == 0 {
			return l, true
		}
	}
	return content.LessonDef{}, false
}

// IsUnlocked reports whether a chapter may be played: every chapter before it,
// except onboarding, is completed.
func (c *Curriculum) IsUnlocked(p *store.Profile, chapterID string) bool {
	i, ok := c.index[chapterID]
	if !ok {
		return false
	}
	for j := 0; j < i; j++ {
		if c.chapters[j].Number > 0 && !c.done(p, j) {
			return false
		}
	}
	return true
}

// RankOf is the rank of the highest completed chapter. It follows completion,
// never mastery, so it cannot fall. A chapter that is only vacuously done
// (nothing to play) earns no rank.
func (c *Curriculum) RankOf(p *store.Profile) Rank {
	rank := Intern
	for i, ch := range c.chapters {
		if c.earned(p, i) {
			rank = max(rank, parseRank(ch.Rank))
		}
	}
	return rank
}

// CompleteChapter marks a chapter completed. It grants no stars.
func (c *Curriculum) CompleteChapter(p *store.Profile, chapterID string) {
	if _, ok := c.index[chapterID]; !ok {
		return
	}
	if p.Completed == nil {
		p.Completed = map[string]bool{}
	}
	p.Completed[chapterID] = true
}

// ApplySkip completes the chapters without stars and gives their characters
// mastery: a character still new becomes solid, except one typed at least 3
// times with an error rate over 0.2, which becomes learning. A short test
// cannot reach the normal sample counts, so this keeps hints quiet for a placed
// user. Unknown chapter ids are ignored and s may be nil.
func (c *Curriculum) ApplySkip(p *store.Profile, s *stats.Stats, chapterIDs []string) {
	for _, id := range chapterIDs {
		i, ok := c.index[id]
		if !ok {
			continue
		}
		c.CompleteChapter(p, id)
		if s == nil {
			continue
		}
		ch := c.raw[i]
		for _, r := range ch.Introduces + ch.Allowed {
			if s.MasteryOf(r) != stats.NewChar {
				continue
			}
			if e := s.Chars[string(r)]; e != nil && e.Samples >= weakMinSamples && e.ErrorRate > weakErrorRate {
				s.SetMastery(r, stats.Learning)
			} else {
				s.SetMastery(r, stats.Solid)
			}
		}
	}
}

// IsDone reports whether a chapter is completed, by the same rule that opens
// the next one. An unknown chapter is not done.
func (c *Curriculum) IsDone(p *store.Profile, chapterID string) bool {
	i, ok := c.index[chapterID]
	return ok && c.done(p, i)
}
