package ui_test

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/jiripisa/tuipe/internal/content"
	"github.com/jiripisa/tuipe/internal/course"
	"github.com/jiripisa/tuipe/internal/play"
	"github.com/jiripisa/tuipe/internal/score"
	"github.com/jiripisa/tuipe/internal/store"
	"github.com/jiripisa/tuipe/internal/ui"
	"github.com/jiripisa/tuipe/internal/ui/style"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

// The rows of the lesson screen, as downs from Výuka.
const (
	partTraining = 1
	partTest     = 2
)

// typingStep is the pause between keys of a round typed well: 300 zn/min.
const typingStep = 200 * time.Millisecond

// openPart starts a part from the lesson screen.
func openPart(t *testing.T, a *ui.App, id string, row int) {
	t.Helper()
	ui.SetScreen(a, ui.NewLessonScreen(a, id))
	for range row {
		a.Update(down)
	}
	a.Update(enter)
	wantScreen(t, a, "round")
	a.Update(ui.TickMsg(a))
}

// finishRound types what is left of the round, step apart, without a mistake.
func finishRound(t *testing.T, a *ui.App, clk *clock, step time.Duration) {
	t.Helper()
	f := ui.RoundFrame(a)
	for _, c := range f.Cells[f.Pos:] {
		if !c.Auto {
			clk.advance(step)
			a.Update(key(c.R))
		}
	}
	wantScreen(t, a, "part-results")
}

// playPart plays a part from start to finish.
func playPart(t *testing.T, a *ui.App, clk *clock, id string, row int) {
	t.Helper()
	openPart(t, a, id, row)
	finishRound(t, a, clk, typingStep)
}

// runsOf reads the stored runs.
func runsOf(t *testing.T, st *store.Store) []store.Run {
	t.Helper()
	runs, err := st.Runs()
	if err != nil {
		t.Fatal(err)
	}
	return runs
}

// starsBelow gives other lessons stars, one at a time, until a Test of id that
// earns at least g would raise the rank to want, which the others alone do not
// reach. Each lesson gets one Test with its stars.
func starsBelow(t *testing.T, a *ui.App, st *store.Store, id string, want course.Rank, g score.Grade) {
	t.Helper()
	grades := map[string]score.Grade{}
	reached := func() bool {
		probe := maps.Clone(grades)
		probe[id] = g
		return a.Course.Rank(probe) == want
	}
	for _, ls := range a.Course.Lessons() {
		if ls.ID == id {
			continue
		}
		for !reached() && grades[ls.ID].Total() < 6 {
			next := grades[ls.ID]
			if next.Accuracy < 3 {
				next.Accuracy++
			} else {
				next.Speed++
			}
			grades[ls.ID] = next
		}
		if grades[ls.ID].Total() > 0 {
			addGraded(t, a, st, ls.ID, grades[ls.ID])
		}
		if reached() {
			return
		}
	}
	t.Fatalf("no number of stars reaches %s", want)
}

// accuracyFor is an accuracy that earns n accuracy stars and no more.
var accuracyFor = [...]float64{0.9, 0.97, 0.99, 1}

// gradedRuns numbers the runs addGraded stores, so that their ids differ.
var gradedRuns int

// gradedRun is a Test of lesson id, a month before the clock, whose speed and
// accuracy earn g: the speed of its last star target, or half the first.
func gradedRun(t *testing.T, a *ui.App, id string, g score.Grade) store.Run {
	t.Helper()
	targets := a.Course.Targets(id)
	wpm := targets.Speed[0] / 2
	if g.Speed > 0 {
		wpm = targets.Speed[g.Speed-1]
	}
	gradedRuns++
	r := store.Run{
		ID: fmt.Sprintf("g%d", gradedRuns), At: newClock().t.AddDate(0, -1, 0), Mode: store.ModeTest,
		LessonID: id, WPM: wpm, Accuracy: accuracyFor[g.Accuracy], Score: score.Value(wpm, accuracyFor[g.Accuracy]),
	}
	if got := score.GradeOf(r.WPM, r.Accuracy, targets); got != g {
		t.Fatalf("the run for %+v earns %+v", g, got)
	}
	return r
}

// addGraded stores a Test of lesson id that earns g.
func addGraded(t *testing.T, a *ui.App, st *store.Store, id string, g score.Grade) {
	t.Helper()
	addRun(t, st, gradedRun(t, a, id, g))
}

func TestTrainingGivesNoStarsAndStoresLength(t *testing.T) {
	a, st, clk := playApp(t)
	id := firstTeachLesson
	ui.SetScreen(a, ui.NewLessonScreen(a, id))
	a.Update(down)
	a.Update(left) // krátký
	a.Update(enter)
	wantScreen(t, a, "round")
	if s := ui.RoundSpec(a); s.Kind != play.KindTraining || s.Length != "short" || s.LessonID != id {
		t.Fatalf("spec = %+v", s)
	}
	finishRound(t, a, clk, typingStep) // fast and clean: a Test would give three stars

	r := onlyRun(t, st)
	if r.Mode != store.ModeTraining || r.Length != "short" || r.LessonID != id {
		t.Errorf("run = %+v", r)
	}
	showsNone(t, a, "★", "☆", "bez chyby")
	if len(ui.StatsOf(a).Chars) == 0 {
		t.Error("a Trénink did not feed the statistics")
	}
}

// tableRow is the row of the results that holds label, without the frame
// around it.
func tableRow(t *testing.T, a *ui.App, label string) string {
	t.Helper()
	return strings.TrimFunc(lineWith(t, a, label), func(r rune) bool { return unicode.IsSpace(r) || strings.ContainsRune("│┃║|", r) })
}

// A Test ends its rows of speed and accuracy with the stars it earned there.
func TestTestShowsItsStarsInTheTable(t *testing.T) {
	a, st, clk := playApp(t)
	id := firstTeachLesson
	openPart(t, a, id, partTest)
	if s := ui.RoundSpec(a); s.Kind != play.KindTest || s.LessonID != id || s.Targets.Speed[1] == 0 {
		t.Fatalf("spec = %+v", s)
	}
	finishRound(t, a, clk, typingStep)

	r := onlyRun(t, st)
	if r.Mode != store.ModeTest || r.Length != "" || r.LessonID != id {
		t.Errorf("run = %+v", r)
	}
	if row := tableRow(t, a, text.PartSpeed); !strings.HasSuffix(row, "zn/min  ★★★") {
		t.Errorf("speed row %q, want its three stars after the unit", row)
	}
	if row := tableRow(t, a, text.PartAccuracy); !strings.HasSuffix(row, "★★★ bez chyby") {
		t.Errorf("accuracy row %q, want three stars and bez chyby", row)
	}
	if len(ui.StatsOf(a).Chars) == 0 {
		t.Error("a Test did not feed the statistics")
	}
}

// Three accuracy stars are for no error at all, however slow, and say so.
func TestFlawlessResultsSayBezChyby(t *testing.T) {
	a, _, clk := playApp(t)
	id := firstTeachLesson
	openPart(t, a, id, partTest)
	finishRound(t, a, clk, 2500*time.Millisecond) // below the one-star speed
	if row := tableRow(t, a, text.PartAccuracy); !strings.HasSuffix(row, "★★★ bez chyby") {
		t.Errorf("accuracy row %q, want three stars and bez chyby", row)
	}
	if row := tableRow(t, a, text.PartSpeed); !strings.HasSuffix(row, "☆☆☆") {
		t.Errorf("speed row %q, want no speed star", row)
	}

	// One wrong key and it is no longer flawless.
	a.Update(enter)
	openPart(t, a, id, partTest)
	a.Update(key('§'))
	finishRound(t, a, clk, typingStep)
	showsNone(t, a, "bez chyby")
	if row := tableRow(t, a, text.PartAccuracy); strings.Contains(row, "★★★") {
		t.Errorf("accuracy row %q: three stars with an error", row)
	}
}

// The accuracy stars agree with the percentage shown beside them, for every
// Test of up to 600 characters: ★★ from 99,0 % and ★ from 97,0 %.
func TestShownAccuracyAgreesWithItsStars(t *testing.T) {
	for reached := 1; reached <= 600; reached++ {
		for right := reached * 9 / 10; right <= reached; right++ {
			acc := float64(right) / float64(reached)
			shown := text.PartPercent(acc)
			pct, err := strconv.ParseFloat(strings.Replace(strings.TrimSuffix(shown, " %"), ",", ".", 1), 64)
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			switch {
			case right == reached:
				want = 3
			case pct >= 99:
				want = 2
			case pct >= 97:
				want = 1
			}
			if got := score.GradeOf(0, acc, score.Targets{}).Accuracy; got != want {
				t.Fatalf("%d/%d right shows %s with %d accuracy stars, want %d", right, reached, shown, got, want)
			}
		}
	}
}

// A worse Test shows its own stars, and the lesson keeps those of its best.
func TestWorseTestKeepsTheBestStars(t *testing.T) {
	a, _, clk := playApp(t)
	id := firstTeachLesson
	playPart(t, a, clk, id, partTest)
	openPart(t, a, id, partTest)
	a.Update(key('§'))
	finishRound(t, a, clk, 2500*time.Millisecond) // below the one-star speed
	if row := tableRow(t, a, text.PartSpeed); !strings.HasSuffix(row, "☆☆☆") {
		t.Errorf("speed row %q, want no speed star", row)
	}
	a.Update(enter)
	wantScreen(t, a, "lesson")
	if l := lineWith(t, a, text.LessonPartTest+"  "); !hasRows(l, "★★★", "★★★") {
		t.Errorf("Test row %q, want the stars of the first Test", l)
	}
}

// Three columns, the unit and three accuracy stars with bez chyby fit the
// narrowest window whole.
func TestResultsStarsFitIn60Columns(t *testing.T) {
	a, st, clk := playApp(t)
	id := firstTeachLesson
	addRun(t, st, store.Run{ID: "a", At: daysAgo(clk, 2), Mode: store.ModeTest, LessonID: id, WPM: 400, Accuracy: 1, Score: 400})
	addRun(t, st, store.Run{ID: "b", At: daysAgo(clk, 1), Mode: store.ModeTest, LessonID: id, WPM: 30, Accuracy: 0.995, Score: 29})
	playPart(t, a, clk, id, partTest)
	for _, w := range []int{60, 80} {
		a.Update(tea.WindowSizeMsg{Width: w, Height: 18})
		showsAll(t, a, text.PartNow, text.PartLast, text.PartBest, "2000", text.PartCPMUnit)
		accuracy, speed := tableRow(t, a, text.PartAccuracy), tableRow(t, a, text.PartSpeed)
		if !strings.HasSuffix(accuracy, "★★★ bez chyby") || !strings.HasSuffix(speed, "★★★") {
			t.Errorf("%d columns: rows\n%s\n%s", w, speed, accuracy)
		}
		col := func(row string) int { return utf8.RuneCountInString(row[:max(strings.Index(row, "★"), 0)]) }
		if col(speed) != col(accuracy) {
			t.Errorf("%d columns: the stars do not stand in one column:\n%s\n%s", w, speed, accuracy)
		}
		if strings.Contains(screen(a), "…") {
			t.Errorf("%d columns: the table is cut:\n%s", w, screen(a))
		}
	}
}

func TestTestHidesHints(t *testing.T) {
	a, _, _ := playApp(t)
	openPart(t, a, firstTeachLesson, partTest)
	if !ui.RoundSpec(a).HintsOff {
		t.Error("a Test does not turn hints off")
	}
	a.Update(key('§')) // a new character after two misses: every other round would hint
	a.Update(key('§'))
	a.Update(ui.TickMsg(a))
	if h := ui.RoundHint(a); h != nil {
		t.Errorf("a Test shows a hint: %+v", h)
	}
}

func TestTrainingShowsHints(t *testing.T) {
	a, _, _ := playApp(t)
	openPart(t, a, firstTeachLesson, partTraining)
	if ui.RoundSpec(a).HintsOff {
		t.Error("a Trénink turns hints off")
	}
	a.Update(ui.TickMsg(a))
	if ui.RoundHint(a) == nil {
		t.Error("a new character in a Trénink gets no hint")
	}
}

func TestAbandonedRoundStoresNoRun(t *testing.T) {
	leave := map[string]func(a *ui.App){
		"pause menu": func(a *ui.App) {
			a.Update(esc)
			a.Update(down)
			a.Update(down)
			a.Update(enter)
		},
		"quit": func(a *ui.App) { a.Shutdown() },
	}
	for name, how := range leave {
		for _, row := range []int{partTraining, partTest} {
			t.Run(fmt.Sprintf("%s %d", name, row), func(t *testing.T) {
				a, st, clk := playApp(t)
				openPart(t, a, firstTeachLesson, row)
				typeKeys(a, clk, 5)
				how(a)
				if runs := runsOf(t, st); len(runs) != 0 {
					t.Errorf("%d runs stored", len(runs))
				}
				if len(ui.StatsOf(a).Chars) == 0 {
					t.Error("the keys so far did not reach the statistics")
				}
			})
		}
	}
}

func TestLeavingARoundReturnsToTheLesson(t *testing.T) {
	a, _, clk := playApp(t)
	openPart(t, a, firstTeachLesson, partTest)
	typeKeys(a, clk, 2)
	a.Update(esc)
	a.Update(down)
	a.Update(down)
	a.Update(enter)
	wantScreen(t, a, "lesson")
	if got := ui.LessonScreenRow(a); got != partTest {
		t.Errorf("row = %d, want the Test", got)
	}
}

func TestCtrlRRestartsTheSameText(t *testing.T) {
	a, st, clk := playApp(t)
	openPart(t, a, firstTeachLesson, partTraining)
	want := ui.RoundSpec(a).Text
	typeKeys(a, clk, 3)
	clk.advance(time.Second)
	a.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	wantScreen(t, a, "round")
	if ui.RoundPos(a) != 0 || ui.RoundSpec(a).Text != want {
		t.Errorf("pos %d, text %q, want the same text from the start", ui.RoundPos(a), ui.RoundSpec(a).Text)
	}
	if runs := runsOf(t, st); len(runs) != 0 {
		t.Errorf("a restart stored %d runs", len(runs))
	}
}

func TestResultsCompareWithLastAndBest(t *testing.T) {
	a, st, clk := playApp(t)
	id := firstTeachLesson
	for _, r := range []store.Run{
		{ID: "best", At: daysAgo(clk, 5), WPM: 80, Accuracy: 0.99, Score: 76},
		{ID: "paused", At: daysAgo(clk, 4), WPM: 200, Accuracy: 1, Score: 200, Paused: true},
		{ID: "last", At: daysAgo(clk, 3), WPM: 40, Accuracy: 0.95, Score: 30},
	} {
		r.Mode, r.LessonID = store.ModeTest, id
		addRun(t, st, r)
	}
	// Runs of another lesson or another part are no comparison.
	addRun(t, st, store.Run{ID: "other", At: daysAgo(clk, 2), Mode: store.ModeTest, LessonID: "ch01-02", WPM: 10, Score: 10})
	addRun(t, st, store.Run{ID: "train", At: daysAgo(clk, 1), Mode: store.ModeTraining, Length: "medium", LessonID: id, WPM: 12, Score: 12})

	playPart(t, a, clk, id, partTest)
	runs := runsOf(t, st)
	cpm := math.Round(5 * runs[len(runs)-1].WPM)
	if cpm <= 200 || cpm >= 400 {
		t.Fatalf("the round ran at %.0f zn/min, between last and best was expected", cpm)
	}
	showsAll(t, a,
		text.PartNow, text.PartLast, text.PartBest,
		"200", "400", "95,0 %", "99,0 %",
		text.PartDelta(cpm-200, false), text.PartDelta(cpm-400, false),
		text.PartDelta(5, true), text.PartDelta(1, true),
		text.PartFaster,
	)
	if !strings.HasPrefix(text.PartDelta(cpm-200, false), "▲ +") || !strings.HasPrefix(text.PartDelta(cpm-400, false), "▼ ") {
		t.Errorf("deltas %q and %q do not point the right way", text.PartDelta(cpm-200, false), text.PartDelta(cpm-400, false))
	}
	for _, old := range []string{"+++", "---", "HEAD~1", "/dev/null", "p. b.", "oproti", runs[len(runs)-1].ID, text.PartRecord} {
		showsNone(t, a, old)
	}
}

func TestResultsAnnounceANewRecord(t *testing.T) {
	a, st, clk := playApp(t)
	id := firstTeachLesson
	addRun(t, st, store.Run{ID: "old", At: daysAgo(clk, 1), Mode: store.ModeTest, LessonID: id, WPM: 20, Accuracy: 0.9, Score: 18})
	playPart(t, a, clk, id, partTest)
	showsAll(t, a, text.PartRecord)
	showsNone(t, a, text.PartFaster, text.PartSlower, text.PartSame)
}

// The best Test is often a slow flawless one, so a speed record is measured
// against the fastest earlier Test, not against the best.
func TestSpeedRecordBeatsTheFastestNotTheBest(t *testing.T) {
	a, st, clk := playApp(t)
	id := firstTeachLesson
	addRun(t, st, store.Run{ID: "fast", At: daysAgo(clk, 3), Mode: store.ModeTest, LessonID: id, WPM: 40, Accuracy: 0.98, Score: 37})
	addRun(t, st, store.Run{ID: "fastest", At: daysAgo(clk, 2), Mode: store.ModeTest, LessonID: id, WPM: 80, Accuracy: 0.9, Score: 52, Paused: true})
	addRun(t, st, store.Run{ID: "best", At: daysAgo(clk, 1), Mode: store.ModeTest, LessonID: id, WPM: 20, Accuracy: 1, Score: 20})

	openPart(t, a, id, partTest)
	a.Update(key('§'))
	finishRound(t, a, clk, 2*typingStep) // 150 zn/min with a slip
	showsAll(t, a, text.PartBest, "100", text.PartFaster)
	showsNone(t, a, text.PartRecord) // 200 zn/min was faster; the paused 400 does not count

	a.Update(enter)
	playPart(t, a, clk, id, partTest) // 300 zn/min
	showsAll(t, a, text.PartRecord)
}

func TestResultsCompareTheSameLength(t *testing.T) {
	a, st, clk := playApp(t)
	id := firstTeachLesson
	addRun(t, st, store.Run{ID: "long", At: daysAgo(clk, 1), Mode: store.ModeTraining, Length: "long", LessonID: id, WPM: 40, Accuracy: 1, Score: 40})
	addRun(t, st, store.Run{ID: "test", At: daysAgo(clk, 1), Mode: store.ModeTest, LessonID: id, WPM: 40, Accuracy: 1, Score: 40})
	playPart(t, a, clk, id, partTraining) // střední
	showsNone(t, a, text.PartLast, text.PartBest)
}

func TestResultsOfFirstAttempt(t *testing.T) {
	for _, row := range []int{partTraining, partTest} {
		a, _, clk := playApp(t)
		playPart(t, a, clk, firstTeachLesson, row)
		showsNone(t, a, text.PartNow, text.PartLast, text.PartBest, text.PartRecord, text.PartFaster, text.PartSlower, text.PartSame, "▲", "▼")
		showsAll(t, a, text.PartSpeed, text.PartAccuracy, text.PartTime, "zn/min")
	}
}

func TestSecondAttemptComparesWithTheFirst(t *testing.T) {
	a, st, clk := playApp(t)
	playPart(t, a, clk, firstTeachLesson, partTest)
	a.Update(enter)
	openPart(t, a, firstTeachLesson, partTest)
	finishRound(t, a, clk, 2*typingStep)
	runs := runsOf(t, st)
	if len(runs) != 2 {
		t.Fatalf("%d runs", len(runs))
	}
	diff := math.Round(5*runs[1].WPM) - math.Round(5*runs[0].WPM)
	if diff >= 0 {
		t.Fatalf("the slower run compares as %.0f", diff)
	}
	showsAll(t, a, text.PartSlower, text.PartDelta(diff, false), text.PartDelta(0, true))
	showsNone(t, a, text.PartRecord, text.PartFaster)
}

func TestPausedRunIsNeverBest(t *testing.T) {
	a, st, clk := playApp(t)
	id := firstTeachLesson
	addRun(t, st, store.Run{ID: "p", At: daysAgo(clk, 1), Mode: store.ModeTest, LessonID: id, WPM: 40, Accuracy: 1, Score: 40, Paused: true})

	openPart(t, a, id, partTest)
	typeKeys(a, clk, 2)
	a.Update(esc)
	clk.advance(time.Minute)
	a.Update(esc)
	finishRound(t, a, clk, typingStep)
	showsAll(t, a, text.ResultsPausedTest, text.PartLast)
	showsNone(t, a, text.PartBest, text.PartRecord) // nothing earlier counts as best

	a.Update(enter)
	playPart(t, a, clk, id, partTest)
	showsNone(t, a, text.PartBest, text.ResultsPausedTest, text.PartRecord) // nor does the paused round above
}

func TestResultsListMissedCharacters(t *testing.T) {
	a, _, clk := playApp(t)
	openPart(t, a, "ch01-02", partTraining)
	wrong := map[rune]int{' ': 4, 'd': 3, 'k': 2, 'f': 1}
	for _, r := range ui.RoundKeys(a) {
		if wrong[r] > 0 {
			wrong[r]--
			clk.advance(typingStep)
			a.Update(key('§'))
		}
		clk.advance(typingStep)
		a.Update(key(r))
	}
	wantScreen(t, a, "part-results")
	for r, n := range wrong {
		if n != 0 {
			t.Fatalf("%q was missed %d times too few", r, n)
		}
	}
	showsAll(t, a, text.PartMissed, text.PartMissedList([]rune{' ', 'd', 'k'}), "mezera · d · k")
	showsNone(t, a, "§")
}

func TestCleanRoundListsNoMissedCharacters(t *testing.T) {
	a, _, clk := playApp(t)
	playPart(t, a, clk, firstTeachLesson, partTraining)
	showsNone(t, a, text.PartMissed)
}

func TestRRepeatsWithNewText(t *testing.T) {
	for _, row := range []int{partTraining, partTest} {
		a, st, clk := playApp(t)
		openPart(t, a, firstTeachLesson, row)
		first := ui.RoundSpec(a)
		finishRound(t, a, clk, typingStep)

		clk.advance(time.Second)
		a.Update(key('r'))
		wantScreen(t, a, "round")
		next := ui.RoundSpec(a)
		if next.Text == first.Text {
			t.Errorf("row %d: R gave the same text", row)
		}
		if next.Kind != first.Kind || next.Length != first.Length || next.LessonID != first.LessonID {
			t.Errorf("row %d: R gave %+v after %+v", row, next, first)
		}
		a.Update(ui.TickMsg(a))
		finishRound(t, a, clk, typingStep) // a repeat ends in the same results
		if got := len(runsOf(t, st)); got != 2 {
			t.Errorf("row %d: %d runs", row, got)
		}
		showsAll(t, a, text.PartLast)
	}
}

func TestEnterAndEscReturnToTheLessonWithTheNewResult(t *testing.T) {
	for _, leave := range []tea.KeyPressMsg{enter, esc} {
		a, st, clk := playApp(t)
		id := firstTeachLesson
		ui.SetScreen(a, ui.NewLessonScreen(a, id))
		a.Update(down)
		a.Update(right) // dlouhý
		a.Update(enter)
		a.Update(ui.TickMsg(a))
		finishRound(t, a, clk, typingStep)
		a.Update(leave)
		wantScreen(t, a, "lesson")
		if got := ui.LessonScreenRow(a); got != partTraining {
			t.Errorf("row = %d, want Trénink", got)
		}
		r := onlyRun(t, st)
		showsAll(t, a, "[dlouhý]", text.LessonLast(text.LessonSpeed(5*r.WPM)))
	}
}

func TestPartThatCannotBeBuiltSaysSo(t *testing.T) {
	a, st, _ := playApp(t)
	ui.SetScreen(a, ui.NewLessonScreen(a, firstTeachLesson))
	a.Course = course.New(&content.Library{}, a.Layout)
	a.Update(down)
	a.Update(enter)
	wantScreen(t, a, "lesson")
	showsAll(t, a, text.PartNotReady)
	if runs := runsOf(t, st); len(runs) != 0 {
		t.Errorf("%d runs stored", len(runs))
	}
}

func TestRThatCannotBuildStaysOnTheResults(t *testing.T) {
	a, _, clk := playApp(t)
	playPart(t, a, clk, firstTeachLesson, partTraining)
	a.Course = course.New(&content.Library{}, a.Layout)
	a.Update(key('r'))
	wantScreen(t, a, "part-results")
	showsAll(t, a, text.PartNotReady)
}

func TestRankLineOnPromotion(t *testing.T) {
	a, st, clk := playApp(t)
	id := firstTeachLesson
	starsBelow(t, a, st, id, course.Junior, score.Grade{Accuracy: 3, Speed: 3})
	playPart(t, a, clk, id, partTest)
	showsAll(t, a, "Nová hodnost: Junior")
	showsNone(t, a, "Intern", "HR")

	a.Update(enter)
	playPart(t, a, clk, id, partTest) // the same stars again: no rise
	showsNone(t, a, "Nová hodnost")
}

func TestTrainingNeverShowsARank(t *testing.T) {
	a, st, clk := playApp(t)
	starsBelow(t, a, st, firstTeachLesson, course.Junior, score.Grade{Accuracy: 3, Speed: 3})
	playPart(t, a, clk, firstTeachLesson, partTraining)
	showsNone(t, a, "Nová hodnost")
}

func TestEarlierLessonOpenedLaterStillWorks(t *testing.T) {
	a, _, clk := playApp(t)
	lessons := a.Course.Lessons()
	later, earlier := lessons[20], lessons[3]

	// A late lesson opened first asks for what came before it.
	openPart(t, a, later.ID, partTest)
	spec := ui.RoundSpec(a)
	finishRound(t, a, clk, typingStep)
	if got := ui.Grades(a)[later.ID]; got.Total() == 0 {
		t.Fatalf("no stars on %s", later.ID)
	}
	allowed := a.Course.AllowedUpTo(later.ID)
	for _, r := range spec.Text {
		if !allowed(r) {
			t.Fatalf("the Test of %s holds %q", later.ID, r)
		}
	}
	if !strings.ContainsFunc(spec.Text, func(r rune) bool { return !slices.Contains(later.Focus, r) && r != ' ' }) {
		t.Errorf("the Test of %s holds nothing taught earlier: %q", later.ID, spec.Text)
	}

	// Then an earlier one still plays, and keeps to what it had taught by then.
	a.Update(enter)
	openPart(t, a, earlier.ID, partTest)
	spec = ui.RoundSpec(a)
	allowed = a.Course.AllowedUpTo(earlier.ID)
	for _, r := range spec.Text {
		if !allowed(r) {
			t.Fatalf("the Test of %s holds %q", earlier.ID, r)
		}
	}
	finishRound(t, a, clk, typingStep)
	if g := ui.Grades(a); g[earlier.ID].Total() == 0 || g[later.ID].Total() == 0 {
		t.Errorf("grades = %v", g)
	}
}

func TestCodePartsKeepTheirLanguage(t *testing.T) {
	a, st, clk := playApp(t)
	ls := lessonWhere(t, a, func(l course.Lesson) bool { return l.Lang == "go" })
	for _, row := range []int{partTraining, partTest} {
		openPart(t, a, ls.ID, row)
		if got := ui.RoundSpec(a).Lang; got != "go" {
			t.Errorf("row %d: Lang = %q", row, got)
		}
		if v := screen(a); strings.Contains(v, text.RoundProseFile) || !strings.Contains(v, "main.go") {
			t.Errorf("row %d does not read as Go:\n%s", row, v)
		}
		finishRound(t, a, clk, typingStep)
		a.Update(enter)
	}
	if got := len(runsOf(t, st)); got != 2 {
		t.Errorf("%d runs", got)
	}
}

func TestOnlyTheTestRoundShowsStars(t *testing.T) {
	a, st, _ := playApp(t)
	addGraded(t, a, st, firstTeachLesson, score.Grade{Accuracy: 2, Speed: 1})
	openPart(t, a, firstTeachLesson, partTraining)
	if strings.Contains(screen(a), "★") {
		t.Errorf("a Trénink shows stars:\n%s", screen(a))
	}
	openPart(t, a, firstTeachLesson, partTest)
	wantText(t, a, "★★☆ ★☆☆") // the best Test, not this one
}

// finishedWithHistory is a Test with both comparisons and three missed characters.
func finishedWithHistory(t *testing.T, a *ui.App, st *store.Store, clk *clock) {
	t.Helper()
	id := firstTeachLesson
	addRun(t, st, store.Run{ID: "a", At: daysAgo(clk, 2), Mode: store.ModeTest, LessonID: id, WPM: 40, Accuracy: 0.9, Score: 30})
	addRun(t, st, store.Run{ID: "b", At: daysAgo(clk, 1), Mode: store.ModeTest, LessonID: id, WPM: 30, Accuracy: 0.8, Score: 20})
	openPart(t, a, id, partTest)
	seen := map[rune]bool{}
	for _, r := range ui.RoundKeys(a) {
		if !seen[r] {
			seen[r] = true
			a.Update(key('§'))
		}
		clk.advance(typingStep)
		a.Update(key(r))
	}
	wantScreen(t, a, "part-results")
}

func TestResultsShedRowsInOrder(t *testing.T) {
	a, st, clk := playApp(t)
	finishedWithHistory(t, a, st, clk)
	runs := runsOf(t, st)
	cpm := math.Round(5 * runs[len(runs)-1].WPM)
	arrows := []string{text.PartDelta(cpm-150, false), text.PartDelta(cpm-200, false)} // minule 30, nejlepší 40 wpm
	everything := append([]string{text.PartRecord, text.PartNow, text.PartMissed, text.PartTime}, arrows...)

	// The body is twelve rows: the verdict, a gap, the table of six rows with a
	// gap after the speed and after the accuracy, a gap and the missed
	// characters. The frame takes three more.
	frame := func(r rune) bool { return unicode.IsSpace(r) || strings.ContainsRune("│┃║|", r) }
	// blank counts the empty rows between the first and the last row of the body.
	blank := func() int {
		lines := strings.Split(screen(a), "\n")
		body := lines[1 : len(lines)-2]
		empty := func(l string) bool { return strings.IndexFunc(l, func(r rune) bool { return !frame(r) }) < 0 }
		first, last := -1, -1
		for i, l := range body {
			if !empty(l) {
				if first < 0 {
					first = i
				}
				last = i
			}
		}
		n := 0
		for i := first + 1; i < last; i++ {
			if empty(body[i]) {
				n++
			}
		}
		return n
	}
	a.Update(tea.WindowSizeMsg{Width: 60, Height: 16})
	showsAll(t, a, everything...)
	if got := blank(); got != 4 {
		t.Errorf("%d blank rows with room for the gaps:\n%s", got, screen(a))
	}

	a.Update(tea.WindowSizeMsg{Width: 60, Height: 11}) // the gaps go first
	showsAll(t, a, everything...)
	if got := blank(); got != 0 {
		t.Errorf("%d blank rows in a full window:\n%s", got, screen(a))
	}

	a.Update(tea.WindowSizeMsg{Width: 60, Height: 10}) // then the arrows
	showsAll(t, a, text.PartRecord, text.PartNow, text.PartMissed, text.PartTime)
	showsNone(t, a, arrows...)

	for _, s := range style.All() {
		a.Style = s
		for _, w := range []int{40, 60} {
			a.Update(tea.WindowSizeMsg{Width: w, Height: 8})
			showsAll(t, a, text.PartSpeed, text.PartAccuracy, text.PartNow, text.PartTime)
			showsNone(t, a, text.PartMissed)
		}
	}
}

func TestResultsTableKeepsItsColumns(t *testing.T) {
	a, st, clk := playApp(t)
	finishedWithHistory(t, a, st, clk)
	a.Update(tea.WindowSizeMsg{Width: 60, Height: 18})
	lines := strings.Split(screen(a), "\n")
	// col is the screen column where value starts, in the row that holds label.
	col := func(label, value string) int {
		for _, l := range lines {
			if strings.Contains(l, label) {
				if i := strings.Index(l, value); i >= 0 {
					return utf8.RuneCountInString(l[:i])
				}
			}
		}
		t.Fatalf("no row with %q and %q:\n%s", label, value, screen(a))
		return 0
	}
	if col(text.PartNow, text.PartLast) != col(text.PartSpeed, "150") || col(text.PartNow, text.PartBest) != col(text.PartSpeed, "200") {
		t.Errorf("the speed row is not under its headers:\n%s", screen(a))
	}
	if col(text.PartNow, text.PartLast) != col(text.PartAccuracy, "80,0 %") {
		t.Errorf("the accuracy row is not under its headers:\n%s", screen(a))
	}
}

func TestResultsDrawInTinyWindows(t *testing.T) {
	a, st, clk := playApp(t)
	finishedWithHistory(t, a, st, clk)
	for w := 40; w <= 60; w += 5 {
		for h := 8; h <= 18; h++ {
			a.Update(tea.WindowSizeMsg{Width: w, Height: h})
			if got := strings.Count(ui.RawView(a), "\n") + 1; got != h {
				t.Errorf("%dx%d: %d lines", w, h, got)
			}
		}
	}
}
