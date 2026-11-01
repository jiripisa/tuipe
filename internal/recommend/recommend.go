// Package recommend picks the next round: the career lesson, a drill on weak
// characters, a piece of real code, or an incident. It also lays out the daily
// standup and the Sprint. All randomness comes from State.Rand.
package recommend

import (
	"math"
	"math/rand/v2"
	"slices"
	"sort"
	"time"

	"github.com/jiripisa/tuipe/internal/content"
	"github.com/jiripisa/tuipe/internal/curriculum"
	"github.com/jiripisa/tuipe/internal/stats"
	"github.com/jiripisa/tuipe/internal/store"
)

// Kind is the type of a recommended round.
type Kind int

// Kinds of round.
const (
	Lesson Kind = iota
	Drill
	Code
	Incident
)

// Pick is one recommended round. Only the fields of its Kind are set, except
// Seed, which fixes the generated text of a Drill.
type Pick struct {
	Kind                            Kind
	LessonID, SnippetID, IncidentID string
	Focus                           []rune
	Seed                            int64
	Length                          int // characters of a Drill
}

// State is everything the recommender looks at.
type State struct {
	Curr     *curriculum.Curriculum
	Lib      *content.Library
	Profile  *store.Profile
	Stats    *stats.Stats
	RunCount int // finished rounds so far
	Rand     *rand.Rand
	Now      time.Time // ages the characters; a Sprint passes the time it began
	WPM      float64   // the player's pace for drill length, see PaceWPM; a Sprint passes the one it began with
}

const (
	incidentEvery  = 8  // every 8th run is an incident
	codeChapter    = 11 // code is recommended from this chapter on
	focusChars     = 5  // characters a drill focuses on
	staleSlots     = 2  // of those, kept for the characters unseen the longest
	roundSeconds   = 75 // budget of one round in a Sprint
	standupRounds  = 5
	baseWPMWindow  = 20
	baseWPMFloor   = 15.0
	drillSeconds   = 60 // typing time of a drill at the player's pace
	charsPerWord   = 5
	minDrillChars  = 30
	maxDrillChars  = 600
	weightLesson   = 60
	weightDrill    = 30
	weightCode     = 10
	weightDrillEnd = 60 // when no lesson remains
	weightCodeEnd  = 40
)

// Next picks the round for a quick run. Every 8th run is an incident when one
// is unlocked. Otherwise it is mostly the next lesson, then drills on weak
// characters, then code from chapter 11 on. When the current chapter's lessons
// are done and its incident is owed, that incident is the only way forward.
func Next(s State) Pick {
	cur := s.Curr.CurrentChapter(s.Profile)
	lesson, hasLesson := s.Curr.NextLesson(s.Profile)
	if !hasLesson {
		if p, ok := s.owedIncident(); ok {
			return p
		}
	}
	if s.RunCount%incidentEvery == incidentEvery-1 {
		if p, ok := s.incident(); ok {
			return p
		}
		return s.drill(cur.ID)
	}
	wLesson, wDrill, wCode := weightLesson, weightDrill, weightCode
	if !hasLesson {
		wLesson, wDrill, wCode = 0, weightDrillEnd, weightCodeEnd
	}
	if cur.Number < codeChapter || len(s.snippets(cur.ID)) == 0 {
		wDrill += wCode
		wCode = 0
	}
	n := s.Rand.IntN(wLesson + wDrill + wCode)
	switch {
	case n < wLesson:
		return Pick{Kind: Lesson, LessonID: lesson.ID, Seed: s.seed()}
	case n < wLesson+wDrill:
		return s.drill(cur.ID)
	}
	return s.code(cur.ID)
}

// Standup returns the 5 rounds of the daily standup: drills on the weakest
// characters and the stalest ones, each with its own text.
func Standup(s State) []Pick {
	cur := s.Curr.CurrentChapter(s.Profile)
	picks := make([]Pick, standupRounds)
	for i := range picks {
		picks[i] = s.drillAt(cur.ID, i)
	}
	return picks
}

// Sprint lays out a sit-down session of the given minutes (10, 20 or 30): a
// warm-up drill, lessons, code, a closing drill and an incident as the finale,
// which is always the last pick. One round is budgeted as 75 s; a typist who
// gets through them early goes on with SprintExtra until the minutes are spent,
// and only then the finale comes. Lessons are the unfinished ones from the
// current chapter on; whatever runs out becomes a drill, and code is left out
// before chapter 11.
func Sprint(s State, minutes int) []Pick {
	rounds := max(minutes*60/roundSeconds, 3)
	cur := s.Curr.CurrentChapter(s.Profile)
	warm := max(rounds/8, 1)
	cool := max(rounds/8, 1)
	code := rounds / 4
	if cur.Number < codeChapter || len(s.snippets(cur.ID)) == 0 {
		code = 0
	}
	lessons := s.unfinishedLessons()
	nLessons := min(rounds-warm-cool-code-1, len(lessons))
	// Unfilled lesson slots become warm-up drills.
	warm += rounds - warm - cool - code - 1 - nLessons

	var picks []Pick
	drills := 0
	drill := func() Pick {
		drills++
		return s.drillAt(cur.ID, drills-1)
	}
	for i := 0; i < warm; i++ {
		picks = append(picks, drill())
	}
	for _, id := range lessons[:nLessons] {
		picks = append(picks, Pick{Kind: Lesson, LessonID: id, Seed: s.seed()})
	}
	for i := 0; i < code; i++ {
		picks = append(picks, s.code(cur.ID))
	}
	for i := 0; i < cool; i++ {
		picks = append(picks, drill())
	}
	finale, ok := s.owedIncident()
	if !ok {
		finale, ok = s.incident()
	}
	if !ok {
		finale = drill()
	}
	return append(picks, finale)
}

// SprintExtra is the drill of round i of a Sprint that has played all its
// planned rounds and has minutes left. It follows from s alone, so give it the
// Sprint's snapshot and a Rand seeded from its seed and i.
func SprintExtra(s State, i int) Pick {
	return s.drillAt(s.Curr.CurrentChapter(s.Profile).ID, i)
}

// DrillLength is the length of a drill: about a minute of typing at wpm, the
// player's pace, or at the floor pace without a history. The bounds keep it
// within 30 to 90 s at any speed a person types.
func DrillLength(wpm float64) int {
	if wpm <= 0 {
		wpm = baseWPMFloor
	}
	chars := int(math.Round(wpm * charsPerWord * drillSeconds / 60))
	return min(max(chars, minDrillChars), maxDrillChars)
}

// BaseWPM is the median WPM of the last 20 runs, never below 15. Runs without
// a valid WPM are ignored.
func BaseWPM(runs []store.Run) float64 {
	return max(PaceWPM(runs), baseWPMFloor)
}

// PaceWPM is the median WPM of the last 20 runs, or 0 without a valid one.
func PaceWPM(runs []store.Run) float64 {
	if len(runs) > baseWPMWindow {
		runs = runs[len(runs)-baseWPMWindow:]
	}
	var wpm []float64
	for _, r := range runs {
		if r.WPM > 0 && !math.IsNaN(r.WPM) && !math.IsInf(r.WPM, 0) {
			wpm = append(wpm, r.WPM)
		}
	}
	if len(wpm) == 0 {
		return 0
	}
	sort.Float64s(wpm)
	mid := len(wpm) / 2
	if len(wpm)%2 == 0 {
		return (wpm[mid-1] + wpm[mid]) / 2
	}
	return wpm[mid]
}

func (s State) seed() int64 { return s.Rand.Int64() }

// drill is a drill on the weakest and the stalest characters of the chapter.
func (s State) drill(chapterID string) Pick { return s.drillAt(chapterID, 0) }

// drillAt is the n-th drill of a plan. The statistics do not move while a plan
// is made, so each drill takes the next of the stalest characters. Those slots
// keep rare characters in training, which weakness alone would never pick: it
// ignores what has not been typed lately. The other slots go to the weakest.
func (s State) drillAt(chapterID string, n int) Pick {
	p := Pick{Kind: Drill, Seed: s.seed(), Length: DrillLength(s.WPM)}
	if s.Stats == nil {
		return p
	}
	unlocked := s.Curr.UnlockedAt(chapterID)
	stale := turn(s.Stats.StalestChars(len(unlocked), unlocked, s.Now), n)
	p.Focus = append(p.Focus, s.Stats.WeakestChars(focusChars-len(stale), func(r rune) bool {
		return slices.Contains(unlocked, r) && !slices.Contains(stale, r)
	})...)
	p.Focus = append(p.Focus, stale...)
	return p
}

// turn is the n-th group of staleSlots characters, going round the list.
func turn(list []rune, n int) []rune {
	if len(list) <= staleSlots {
		return list
	}
	out := make([]rune, staleSlots)
	for i := range out {
		out[i] = list[(n*staleSlots+i)%len(list)]
	}
	return out
}

// snippets are the code snippets whose characters are all unlocked.
func (s State) snippets(chapterID string) []content.Snippet {
	allowed := s.Curr.AllowedAt(chapterID)
	var out []content.Snippet
	for _, sn := range s.Lib.Snippets {
		if !slices.ContainsFunc([]rune(sn.Text), func(r rune) bool { return !allowed(r) }) {
			out = append(out, sn)
		}
	}
	return out
}

func (s State) code(chapterID string) Pick {
	list := s.snippets(chapterID)
	if len(list) == 0 {
		return s.drill(chapterID)
	}
	return Pick{Kind: Code, SnippetID: list[s.Rand.IntN(len(list))].ID, Seed: s.seed()}
}

// incident picks a replay: the boss of a chapter that is already done, or a
// free-play extra of an unlocked chapter. A boss that is still owed is not
// offered here, because its chapter's lessons may be unplayed (see owedIncident).
func (s State) incident() (Pick, bool) {
	var ids []string
	for _, inc := range s.Lib.Incidents {
		if !s.Curr.IsUnlocked(s.Profile, inc.Chapter) || !s.typable(inc) {
			continue
		}
		boss, isBoss := s.Curr.IncidentDef(inc.Chapter)
		if isBoss && boss.ID == inc.ID && !s.Curr.IsDone(s.Profile, inc.Chapter) {
			continue
		}
		ids = append(ids, inc.ID)
	}
	if len(ids) == 0 {
		return Pick{}, false
	}
	return Pick{Kind: Incident, IncidentID: ids[s.Rand.IntN(len(ids))], Seed: s.seed()}, true
}

// typable reports whether the layout can type every step of inc.
func (s State) typable(inc content.Incident) bool {
	for _, st := range inc.Steps {
		if slices.ContainsFunc([]rune(st.Text), func(r rune) bool { return r != '\n' && !s.Curr.CanType(r) }) {
			return false
		}
	}
	return true
}

// owedIncident is the incident of the current chapter once its lessons are
// done and while it is not won.
func (s State) owedIncident() (Pick, bool) {
	cur := s.Curr.CurrentChapter(s.Profile)
	inc, ok := s.Curr.IncidentDef(cur.ID)
	if _, lessonsLeft := s.Curr.NextLesson(s.Profile); lessonsLeft ||
		!ok || s.Profile.Completed[cur.ID] || s.Profile.IncidentBest[inc.ID] != "" {
		return Pick{}, false
	}
	return Pick{Kind: Incident, IncidentID: inc.ID, Seed: s.seed()}, true
}

// unfinishedLessons are the lessons without a star in unlocked chapters, from
// the current chapter on.
func (s State) unfinishedLessons() []string {
	if _, ok := s.Curr.NextLesson(s.Profile); !ok {
		return nil // all done, or the incident is owed first
	}
	cur := s.Curr.CurrentChapter(s.Profile)
	var ids []string
	started := false
	for _, ch := range s.Curr.Chapters() {
		started = started || ch.ID == cur.ID
		if !started || !s.Curr.IsUnlocked(s.Profile, ch.ID) {
			continue
		}
		for _, l := range ch.Lessons {
			if s.Profile.Stars[l.ID] == 0 {
				ids = append(ids, l.ID)
			}
		}
	}
	return ids
}
