package recommend_test

import (
	"math"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/jiripisa/tuipe/internal/content"
	"github.com/jiripisa/tuipe/internal/curriculum"
	"github.com/jiripisa/tuipe/internal/layout"
	"github.com/jiripisa/tuipe/internal/recommend"
	"github.com/jiripisa/tuipe/internal/stats"
	"github.com/jiripisa/tuipe/internal/store"
)

func eq[T comparable](t *testing.T, got, want T) {
	t.Helper()
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

type fixture struct {
	lib  *content.Library
	curr *curriculum.Curriculum
	prof *store.Profile
	st   *stats.Stats
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	lib, err := content.Load()
	if err != nil {
		t.Fatal(err)
	}
	all, err := layout.Builtin()
	if err != nil {
		t.Fatal(err)
	}
	var lay *layout.Layout
	for _, l := range all {
		if l.ID == "macos-czech-qwerty" {
			lay = l
		}
	}
	if lay == nil {
		t.Fatal("no Czech layout")
	}
	return &fixture{
		lib:  lib,
		curr: curriculum.New(lib, lay),
		prof: &store.Profile{
			Stats:        map[string]*stats.Stats{},
			Stars:        map[string]int{},
			Completed:    map[string]bool{},
			IncidentBest: map[string]string{},
		},
		st: stats.New(),
	}
}

// completeBefore completes every chapter before the one with the given number.
func (f *fixture) completeBefore(number int) {
	for _, ch := range f.curr.Chapters() {
		if ch.Number < number {
			f.curr.CompleteChapter(f.prof, ch.ID)
		}
	}
}

func (f *fixture) state(runCount int, seed uint64) recommend.State {
	return recommend.State{
		Curr: f.curr, Lib: f.lib, Profile: f.prof, Stats: f.st,
		RunCount: runCount, Rand: rand.New(rand.NewPCG(seed, 1)),
	}
}

func (f *fixture) weak(chars string) {
	for _, r := range chars {
		f.st.Chars[string(r)] = &stats.Entry{Samples: 20, ErrorRate: 0.4, LatencyMs: 600}
	}
}

func TestRecommenderMix(t *testing.T) {
	f := newFixture(t)
	f.completeBefore(11) // mid-curriculum: chapter 11 is current, code is open
	counts := map[recommend.Kind]int{}
	s := f.state(0, 7)
	for i := 0; i < 1000; i++ {
		s.RunCount = i
		p := recommend.Next(s)
		counts[p.Kind]++
		if i%8 == 7 {
			eq(t, p.Kind, recommend.Incident)
		} else {
			eq(t, p.Kind == recommend.Incident, false)
		}
	}
	lesson, drill, code := counts[recommend.Lesson], counts[recommend.Drill], counts[recommend.Code]
	if lesson < 500 || lesson > 700 || drill < 200 || drill > 400 || code > 150 || code == 0 {
		t.Fatalf("mix out of range: %v", counts)
	}
}

func TestIncidentComesFromUnlockedChapter(t *testing.T) {
	f := newFixture(t)
	f.completeBefore(3)
	p := recommend.Next(f.state(7, 1))
	eq(t, p.Kind, recommend.Incident)
	unlocked := map[string]bool{}
	for _, ch := range f.curr.Chapters() {
		if f.curr.IsUnlocked(f.prof, ch.ID) {
			if inc, ok := f.curr.IncidentDef(ch.ID); ok {
				unlocked[inc.ID] = true
			}
		}
	}
	eq(t, unlocked[p.IncidentID], true)
}

func TestNoCodeBeforeChapter11(t *testing.T) {
	f := newFixture(t)
	f.completeBefore(5)
	s := f.state(0, 3)
	for i := 0; i < 500; i++ {
		s.RunCount = i
		if p := recommend.Next(s); p.Kind == recommend.Code {
			t.Fatalf("run %d: code before chapter 11", i)
		}
	}
}

func TestNextPicksLessonOfCurrentChapter(t *testing.T) {
	f := newFixture(t)
	want, ok := f.curr.NextLesson(f.prof)
	eq(t, ok, true)
	s := f.state(0, 5)
	for i := 0; i < 50; i++ {
		if p := recommend.Next(s); p.Kind == recommend.Lesson {
			eq(t, p.LessonID, want.ID)
			return
		}
	}
	t.Fatal("no lesson recommended in 50 calls")
}

func TestIncidentOwedWhenLessonsAreDone(t *testing.T) {
	f := newFixture(t)
	f.completeBefore(2) // chapter 2 current
	var ch content.ChapterDef
	for _, c := range f.curr.Chapters() {
		if c.Number == 2 {
			ch = c
		}
	}
	for _, l := range ch.Lessons {
		f.prof.Stars[l.ID] = 1
	}
	inc, ok := f.curr.IncidentDef(ch.ID)
	eq(t, ok, true)
	_, more := f.curr.NextLesson(f.prof)
	eq(t, more, false)
	for i := 0; i < 20; i++ {
		p := recommend.Next(f.state(i, 9))
		eq(t, p.Kind, recommend.Incident)
		eq(t, p.IncidentID, inc.ID)
	}
}

func TestAllCompleteStillRecommends(t *testing.T) {
	f := newFixture(t)
	f.completeBefore(99)
	s := f.state(0, 11)
	seen := map[recommend.Kind]int{}
	for i := 0; i < 400; i++ {
		s.RunCount = i
		p := recommend.Next(s)
		seen[p.Kind]++
		if i%8 != 7 && p.Kind != recommend.Drill && p.Kind != recommend.Code {
			t.Fatalf("run %d: got kind %v", i, p.Kind)
		}
	}
	eq(t, seen[recommend.Drill] > 0 && seen[recommend.Code] > 0, true)
}

func TestDrillFocusIsWeakAllowedChars(t *testing.T) {
	f := newFixture(t)
	f.weak("jž") // ž is not allowed in chapter 1
	s := f.state(0, 2)
	for i := 0; i < 200; i++ {
		if p := recommend.Next(s); p.Kind == recommend.Drill {
			eq(t, slices.Equal(p.Focus, []rune{'j'}), true)
			return
		}
	}
	t.Fatal("no drill in 200 calls")
}

func TestCodePickIsTypableAtCurrentChapter(t *testing.T) {
	f := newFixture(t)
	f.completeBefore(11)
	allowed := f.curr.AllowedAt(f.curr.CurrentChapter(f.prof).ID)
	byID := map[string]content.Snippet{}
	for _, sn := range f.lib.Snippets {
		byID[sn.ID] = sn
	}
	s := f.state(0, 4)
	for i := 0; i < 300; i++ {
		s.RunCount = i
		if p := recommend.Next(s); p.Kind == recommend.Code {
			sn, ok := byID[p.SnippetID]
			eq(t, ok, true)
			for _, r := range sn.Text {
				if !allowed(r) {
					t.Fatalf("snippet %s has %q", sn.ID, r)
				}
			}
		}
	}
}

func TestStandupFiveRoundsOnWeakChars(t *testing.T) {
	f := newFixture(t)
	f.completeBefore(5)
	f.weak("jfk")
	picks := recommend.Standup(f.state(0, 6))
	eq(t, len(picks), 5)
	seeds := map[int64]bool{}
	for _, p := range picks {
		eq(t, p.Kind, recommend.Drill)
		if len(p.Focus) == 0 {
			t.Fatal("empty focus")
		}
		for _, r := range p.Focus {
			eq(t, r == 'j' || r == 'f' || r == 'k', true)
		}
		seeds[p.Seed] = true
	}
	eq(t, len(seeds), 5)
}

func TestSprintMixEndsWithIncident(t *testing.T) {
	f := newFixture(t)
	f.completeBefore(11)
	f.weak("jf")
	for _, minutes := range []int{10, 20, 30} {
		picks := recommend.Sprint(f.state(0, 8), minutes)
		eq(t, picks[0].Kind, recommend.Drill)
		eq(t, picks[len(picks)-1].Kind, recommend.Incident)
		est := float64(len(picks)) * 75
		budget := float64(minutes * 60)
		if math.Abs(est-budget) > budget*0.2 {
			t.Fatalf("%d min: %d rounds = %.0f s, budget %.0f s", minutes, len(picks), est, budget)
		}
		kinds := map[recommend.Kind]bool{}
		for _, p := range picks {
			kinds[p.Kind] = true
		}
		eq(t, kinds[recommend.Lesson] && kinds[recommend.Code] && kinds[recommend.Drill], true)
	}
}

func TestSprintBeforeChapter11HasNoCode(t *testing.T) {
	f := newFixture(t)
	picks := recommend.Sprint(f.state(0, 8), 10)
	for _, p := range picks {
		eq(t, p.Kind == recommend.Code, false)
	}
	eq(t, picks[len(picks)-1].Kind, recommend.Incident)
}

func TestBaseWPMFloor(t *testing.T) {
	eq(t, recommend.BaseWPM(nil), 15.0)
	eq(t, recommend.BaseWPM([]store.Run{{WPM: 0}, {WPM: math.NaN()}}), 15.0)
	eq(t, recommend.BaseWPM([]store.Run{{WPM: 8}}), 15.0)
}

func TestBaseWPMMedianOfLast20(t *testing.T) {
	var runs []store.Run
	for i := 0; i < 10; i++ { // old and slow, outside the window
		runs = append(runs, store.Run{WPM: 20})
	}
	for _, w := range []float64{40, 0, 50, 60, 30, 70, 40, 45, 55, 35, 65, 42, 48, 52, 38, 58, 44, 46, 50, 41} {
		runs = append(runs, store.Run{WPM: w})
	}
	// Last 20 runs hold one zero-WPM run, ignored; 19 values remain.
	got := recommend.BaseWPM(runs[len(runs)-20:])
	eq(t, got, 46.0)
	eq(t, recommend.BaseWPM(runs), 46.0)
	// Incident runs count like any other.
	eq(t, recommend.BaseWPM([]store.Run{{WPM: 30, Incident: &store.IncidentResult{}}, {WPM: 50}}), 40.0)
}
