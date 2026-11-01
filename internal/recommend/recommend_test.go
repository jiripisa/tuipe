package recommend_test

import (
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

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
		RunCount: runCount, Rand: rand.New(rand.NewPCG(seed, 1)), Now: fixtureNow,
	}
}

var fixtureNow = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

func (f *fixture) weak(chars string) {
	for _, r := range chars {
		f.st.Chars[string(r)] = &stats.Entry{Samples: 20, ErrorRate: 0.4, LatencyMs: 600, LastSeen: fixtureNow}
	}
}

func TestRecommenderMix(t *testing.T) {
	f := newFixture(t)
	f.completeBefore(11) // mid-curriculum: chapter 11 is current, code is open
	f.weak("asdf")
	const calls = 20000
	counts := map[recommend.Kind]int{}
	s := f.state(0, 7)
	for i := 0; i < calls; i++ {
		s.RunCount = i
		p := recommend.Next(s)
		counts[p.Kind]++
		eq(t, p.Kind == recommend.Incident, i%8 == 7)
	}
	// Every 8th run is an incident (12.5 %); the rest splits 60/30/10.
	want := map[recommend.Kind]float64{
		recommend.Lesson: 0.525, recommend.Drill: 0.2625, recommend.Code: 0.0875, recommend.Incident: 0.125,
	}
	for k, w := range want {
		if got := float64(counts[k]) / calls; math.Abs(got-w) > 0.02 {
			t.Errorf("kind %v: share %.3f, want %.3f +- 0.02", k, got, w)
		}
	}
}

func TestIncidentComesFromCompletedChaptersOnly(t *testing.T) {
	f := newFixture(t)
	f.completeBefore(3) // chapters 1 and 2 are completed, 3 is current
	done := map[string]bool{}
	for _, ch := range f.curr.Chapters() {
		if inc, ok := f.curr.IncidentDef(ch.ID); ok && f.curr.IsDone(f.prof, ch.ID) {
			done[inc.ID] = true
		}
	}
	eq(t, len(done) >= 2, true)
	for seed := uint64(1); seed < 40; seed++ {
		p := recommend.Next(f.state(7, seed))
		eq(t, p.Kind, recommend.Incident)
		eq(t, done[p.IncidentID], true) // never the boss of chapter 3, which is not finished
	}
}

func TestNoIncidentBeforeAnyChapterIsCompleted(t *testing.T) {
	f := newFixture(t)
	f.completeBefore(1) // only onboarding is done: no boss may be offered early
	for seed := uint64(1); seed < 20; seed++ {
		p := recommend.Next(f.state(7, seed))
		eq(t, p.Kind, recommend.Lesson) // the 8th run falls back to the next lesson
	}
	f.weak("fj")
	for seed := uint64(1); seed < 20; seed++ {
		eq(t, recommend.Next(f.state(7, seed)).Kind, recommend.Drill) // or a drill, with data
	}
	picks := recommend.Sprint(f.state(0, 3), 10)
	eq(t, picks[len(picks)-1].Kind, recommend.Drill)
}

func TestFreePlayExtrasComeWithTheirUnlockedChapter(t *testing.T) {
	f := newFixture(t)
	f.completeBefore(11)
	extras := map[string]bool{}
	for seed := uint64(1); seed < 300; seed++ {
		p := recommend.Next(f.state(7, seed))
		eq(t, p.Kind, recommend.Incident)
		for _, inc := range f.lib.Incidents {
			if inc.ID == p.IncidentID {
				extras[inc.ID] = inc.Chapter != "ch12" && inc.ID != "INC-1011"
			}
		}
		eq(t, p.IncidentID != "INC-1011" && p.IncidentID != "INC-2004", true) // unfinished boss, locked chapter
	}
	eq(t, extras["INC-2001"] || extras["INC-2002"] || extras["INC-2003"], true)
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
			eq(t, slices.Contains(p.Focus, 'j'), true)
			eq(t, slices.Contains(p.Focus, 'ž'), false)
			eq(t, len(p.Focus) <= 5, true)
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
		eq(t, len(p.Focus), 5)
		weak := 0
		for _, r := range p.Focus {
			if r == 'j' || r == 'f' || r == 'k' {
				weak++
			}
		}
		eq(t, weak, 3) // the two other slots belong to the stalest characters
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
	eq(t, picks[len(picks)-1].Kind, recommend.Drill) // no boss is owed or finished yet
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

func TestSprintLessonsAreInUnlockedChapters(t *testing.T) {
	f := newFixture(t)
	f.completeBefore(4)
	chapterOf := map[string]string{}
	for _, ch := range f.curr.Chapters() {
		for _, l := range ch.Lessons {
			chapterOf[l.ID] = ch.ID
		}
	}
	for _, minutes := range []int{10, 20, 30} {
		picks := recommend.Sprint(f.state(0, 8), minutes)
		eq(t, len(picks), minutes*60/75)
		for _, p := range picks {
			if p.Kind == recommend.Lesson {
				ch, ok := chapterOf[p.LessonID]
				eq(t, ok && f.curr.IsUnlocked(f.prof, ch), true)
			}
		}
	}
}

func TestEmptyProfileDoesNotPanic(t *testing.T) {
	f := newFixture(t)
	s := recommend.State{
		Curr: f.curr, Lib: f.lib, Profile: &store.Profile{},
		Rand: rand.New(rand.NewPCG(1, 2)),
	}
	for i := 0; i < 50; i++ {
		s.RunCount = i
		if p := recommend.Next(s); i%8 != 7 && p.Kind != recommend.Lesson && p.Kind != recommend.Drill {
			t.Fatalf("run %d: kind %v", i, p.Kind)
		}
	}
	standup := recommend.Standup(s)
	eq(t, len(standup), 5)
	for _, p := range standup {
		eq(t, p.Kind, recommend.Drill)
	}
	for _, minutes := range []int{10, 20, 30} {
		picks := recommend.Sprint(s, minutes)
		eq(t, picks[0].Kind, recommend.Drill)
		eq(t, picks[len(picks)-1].Kind, recommend.Drill) // an empty profile owes no incident yet
	}
}

// seen marks every character as typed long ago, except that chars are older.
func (f *fixture) seen(unlocked []rune, at time.Time) {
	for _, r := range unlocked {
		f.st.Chars[string(r)] = &stats.Entry{Samples: 50, ErrorRate: 0.01, LatencyMs: 200, LastSeen: at}
	}
}

func TestDrillReservesSlotsForTheStalestCharacters(t *testing.T) {
	f := newFixture(t)
	f.completeBefore(99)
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	f.seen(f.curr.UnlockedAt("ch12"), now.Add(-time.Hour))
	f.weak("qwertyu") // more weak characters than a drill has room for
	for _, r := range "qwertyu" {
		f.st.Chars[string(r)].LastSeen = now.Add(-time.Minute)
	}
	f.st.Chars["§"].LastSeen = now.Add(-40 * 24 * time.Hour)
	f.st.Chars["ˇ"].LastSeen = now.Add(-30 * 24 * time.Hour)
	s := f.state(0, 2)
	s.Now = now
	picks := recommend.Standup(s)
	first := picks[0].Focus
	eq(t, len(first), 5)
	eq(t, slices.Contains(first, '§') && slices.Contains(first, 'ˇ'), true)
	weak := 0
	for _, r := range first {
		if strings.ContainsRune("qwertyu", r) {
			weak++
		}
	}
	eq(t, weak, 3)
}

// The statistics do not move while a plan is made, so without rotation every
// drill of a Standup would reserve the same two characters.
func TestPlannedDrillsTakeTurnsWithTheStalest(t *testing.T) {
	f := newFixture(t)
	f.completeBefore(99)
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	s := f.state(0, 2)
	s.Now = now
	covered := map[rune]bool{}
	for _, p := range recommend.Standup(s) {
		for _, r := range p.Focus {
			if covered[r] {
				t.Fatalf("%q is drilled twice in one Standup", r)
			}
			covered[r] = true
		}
	}
	eq(t, len(covered) >= 10, true)
	plan := recommend.Sprint(s, 30)
	sprint := map[rune]bool{}
	for _, p := range plan {
		for _, r := range p.Focus {
			sprint[r] = true
		}
	}
	eq(t, len(sprint) >= 10, true)
}

func TestDrillFocusNeverHoldsSpaceOrEnter(t *testing.T) {
	f := newFixture(t)
	f.completeBefore(99)
	f.weak(" \nab")
	for i := range 20 {
		s := f.state(0, uint64(i))
		for _, p := range recommend.Standup(s) {
			eq(t, slices.Contains(p.Focus, ' ') || slices.Contains(p.Focus, '\n'), false)
		}
	}
}

func TestDrillLengthIsAboutAMinuteOfTyping(t *testing.T) {
	for wpm, want := range map[float64]int{9: 45, 25: 125, 40: 200, 60: 300, 80: 400} {
		eq(t, recommend.DrillLength(wpm), want)
	}
	eq(t, recommend.DrillLength(0), 75) // no history: the floor pace
	for wpm := 5.0; wpm <= 200; wpm += 5 {
		if secs := float64(recommend.DrillLength(wpm)) / (wpm * 5 / 60); secs < 30 || secs > 90 {
			t.Errorf("%.0f wpm: a drill takes %.0f s", wpm, secs)
		}
	}
}

func TestPaceWPMHasNoFloor(t *testing.T) {
	eq(t, recommend.PaceWPM(nil), 0.0)
	eq(t, recommend.PaceWPM([]store.Run{{WPM: 8}, {WPM: 10}}), 9.0)
	eq(t, recommend.BaseWPM([]store.Run{{WPM: 8}, {WPM: 10}}), 15.0)
}

func TestPicksCarryTheDrillLength(t *testing.T) {
	f := newFixture(t)
	s := f.state(0, 3)
	s.WPM = 60
	for _, p := range recommend.Standup(s) {
		eq(t, p.Length, 300)
	}
	eq(t, recommend.Sprint(s, 10)[0].Length, 300)
	eq(t, recommend.SprintExtra(s, 9).Kind, recommend.Drill)
	eq(t, recommend.SprintExtra(s, 9).Length, 300)
}

func (f *fixture) weakBigram(bg string) {
	f.st.Bigrams[bg] = &stats.Entry{Samples: 20, ErrorRate: 0.5, LatencyMs: 900, LastSeen: fixtureNow}
}

func TestDrillCarriesTheWeakestBigrams(t *testing.T) {
	f := newFixture(t)
	f.completeBefore(5)
	f.weakBigram("st")
	f.weakBigram("až")
	f.weakBigram("1a") // digits come in chapter 6
	f.weakBigram("a ") // a space is no drill material
	picks := recommend.Standup(f.state(0, 1))
	for _, p := range picks {
		eq(t, slices.Contains(p.Bigrams, "st"), true)
		eq(t, slices.Contains(p.Bigrams, "až"), true)
		eq(t, slices.Contains(p.Bigrams, "1a"), false)
		eq(t, slices.Contains(p.Bigrams, "a "), false)
	}
}

// Nothing is known yet: a drill would be a guess, and the lesson that teaches
// hand position has not even been played.
func TestBeginnerGetsLessonsNotDrills(t *testing.T) {
	f := newFixture(t)
	want, _ := f.curr.NextLesson(f.prof)
	for seed := uint64(1); seed < 300; seed++ {
		p := recommend.Next(f.state(int(seed), seed))
		eq(t, p.Kind, recommend.Lesson)
		eq(t, p.LessonID, want.ID)
	}
}

func TestDrillsComeOnceThereIsDataOnAWeakness(t *testing.T) {
	f := newFixture(t)
	f.weak("fj")
	drills := 0
	for seed := uint64(1); seed < 300; seed++ {
		if recommend.Next(f.state(0, seed)).Kind == recommend.Drill {
			drills++
		}
	}
	eq(t, drills > 0, true)
}

func TestWithNoLessonLeftADrillComesEvenWithoutData(t *testing.T) {
	f := newFixture(t)
	f.completeBefore(99)
	seen := map[recommend.Kind]bool{}
	for seed := uint64(1); seed < 100; seed++ {
		seen[recommend.Next(f.state(0, seed)).Kind] = true
	}
	eq(t, seen[recommend.Drill], true)
}

func TestAfterALostBossOneDrillThenTheBossAgain(t *testing.T) {
	f := newFixture(t)
	f.completeBefore(2)
	f.weak("qw")
	ch := f.curr.Chapters()[2]
	for _, l := range ch.Lessons {
		f.prof.Stars[l.ID] = 1
	}
	boss, _ := f.curr.IncidentDef(ch.ID)
	for seed := uint64(1); seed < 20; seed++ {
		s := f.state(int(seed), seed)
		s.LostBoss = true
		p := recommend.Next(s)
		eq(t, p.Kind, recommend.Drill)
		eq(t, slices.Contains(p.Focus, 'q') && slices.Contains(p.Focus, 'w'), true)
		s.LostBoss = false
		p = recommend.Next(s)
		eq(t, p.Kind, recommend.Incident)
		eq(t, p.IncidentID, boss.ID)
	}
}

func TestLostBossLeavesARoundWithALessonDueAlone(t *testing.T) {
	f := newFixture(t) // a lesson is due: nothing is owed, so nothing changes
	s := f.state(0, 3)
	s.LostBoss = true
	eq(t, recommend.Next(s).Kind, recommend.Lesson)
}
