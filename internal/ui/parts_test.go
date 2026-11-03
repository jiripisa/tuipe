package ui_test

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/jiripisa/tuipe/internal/content"
	"github.com/jiripisa/tuipe/internal/course"
	"github.com/jiripisa/tuipe/internal/play"
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

// starsBelow gives other lessons three stars each, until a Test of id with
// three stars would raise the rank to want.
func starsBelow(t *testing.T, a *ui.App, st *store.Store, id string, want course.Rank) {
	t.Helper()
	p := st.Profile()
	for _, ls := range a.Course.Lessons() {
		probe := maps.Clone(p.Stars)
		probe[id] = 3
		if a.Course.Rank(probe) == want {
			return
		}
		if ls.ID != id {
			p.Stars[ls.ID] = 3
		}
	}
	t.Fatalf("no number of stars reaches %s", want)
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
	if r.Mode != store.ModeTraining || r.Length != "short" || r.Stars != 0 || r.LessonID != id {
		t.Errorf("run = %+v", r)
	}
	if got := st.Profile().Stars[id]; got != 0 {
		t.Errorf("a Trénink gave %d stars", got)
	}
	showsNone(t, a, text.PartStars, "★", "☆")
	if len(ui.StatsOf(a).Chars) == 0 {
		t.Error("a Trénink did not feed the statistics")
	}
}

func TestTestStoresStars(t *testing.T) {
	a, st, clk := playApp(t)
	id := firstTeachLesson
	openPart(t, a, id, partTest)
	if s := ui.RoundSpec(a); s.Kind != play.KindTest || s.LessonID != id || s.Targets.Speed[1] == 0 {
		t.Fatalf("spec = %+v", s)
	}
	finishRound(t, a, clk, typingStep)

	r := onlyRun(t, st)
	if r.Mode != store.ModeTest || r.Stars != 3 || r.Length != "" || r.LessonID != id {
		t.Errorf("run = %+v", r)
	}
	if got := st.Profile().Stars[id]; got != 3 {
		t.Errorf("stars = %d, want 3", got)
	}
	showsAll(t, a, text.PartStars, "★★★")
	if len(ui.StatsOf(a).Chars) == 0 {
		t.Error("a Test did not feed the statistics")
	}
}

func TestPartsLeaveTheOldProgressAlone(t *testing.T) {
	a, st, clk := playApp(t)
	st.Profile().Resume = &store.Resume{Kind: "lesson", LessonID: "ch01-02"}
	for _, row := range []int{partTraining, partTest} {
		playPart(t, a, clk, firstTeachLesson, row)
	}
	p := st.Profile()
	if p.Resume == nil || p.Resume.LessonID != "ch01-02" {
		t.Errorf("Resume = %+v", p.Resume)
	}
	if len(p.Completed) != 0 {
		t.Errorf("a part completed chapters: %v", p.Completed)
	}
}

func TestBestStarsNeverDrop(t *testing.T) {
	a, st, clk := playApp(t)
	id := firstTeachLesson
	playPart(t, a, clk, id, partTest)
	openPart(t, a, id, partTest)
	finishRound(t, a, clk, 2500*time.Millisecond) // below the one-star speed
	showsAll(t, a, "☆☆☆", text.ResultsNoStars)

	runs := runsOf(t, st)
	if got := runs[len(runs)-1].Stars; got != 0 {
		t.Errorf("the slow run has %d stars", got)
	}
	if got := st.Profile().Stars[id]; got != 3 {
		t.Errorf("the best stars dropped to %d", got)
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
				p := st.Profile()
				if p.Stars[firstTeachLesson] != 0 {
					t.Error("an abandoned round gave stars")
				}
				if p.Resume != nil {
					t.Errorf("a part set Resume: %+v", p.Resume)
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
	showsAll(t, a,
		text.PartCompare(cpm-200, 5, text.PartVsLast),
		text.PartCompare(cpm-400, 1, text.PartVsBest),
		fmt.Sprintf("%+.0f zn/min oproti minule", cpm-200),
		fmt.Sprintf("%.0f zn/min oproti nejlepšímu", cpm-400),
	)
	for _, old := range []string{"+++", "---", text.ResultsOld, text.ResultsNoBase, runs[len(runs)-1].ID} {
		showsNone(t, a, old)
	}
}

func TestResultsCompareTheSameLength(t *testing.T) {
	a, st, clk := playApp(t)
	id := firstTeachLesson
	addRun(t, st, store.Run{ID: "long", At: daysAgo(clk, 1), Mode: store.ModeTraining, Length: "long", LessonID: id, WPM: 40, Accuracy: 1, Score: 40})
	addRun(t, st, store.Run{ID: "test", At: daysAgo(clk, 1), Mode: store.ModeTest, LessonID: id, WPM: 40, Accuracy: 1, Score: 40})
	playPart(t, a, clk, id, partTraining) // střední
	showsNone(t, a, text.PartVsLast, text.PartVsBest)
}

func TestResultsOfFirstAttempt(t *testing.T) {
	for _, row := range []int{partTraining, partTest} {
		a, _, clk := playApp(t)
		playPart(t, a, clk, firstTeachLesson, row)
		showsNone(t, a, text.PartVsLast, text.PartVsBest, "zn/min oproti")
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
	showsAll(t, a, text.PartCompare(diff, 0, text.PartVsLast), text.PartCompare(diff, 0, text.PartVsBest))
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
	showsAll(t, a, text.ResultsPaused, text.PartVsLast)
	showsNone(t, a, text.PartVsBest) // nothing earlier counts as best

	a.Update(enter)
	playPart(t, a, clk, id, partTest)
	showsNone(t, a, text.PartVsBest, text.ResultsPaused) // nor does the paused round above
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
	showsAll(t, a, text.PartMissedList([]rune{' ', 'd', 'k'}), "mezera, d, k")
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
		showsAll(t, a, text.PartVsLast)
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
	starsBelow(t, a, st, id, course.Junior)
	playPart(t, a, clk, id, partTest)
	showsAll(t, a, fmt.Sprintf(text.ResultsRank, course.Intern, course.Junior))

	a.Update(enter)
	playPart(t, a, clk, id, partTest) // the same stars again: no rise
	showsNone(t, a, "Hodnost")
}

func TestTrainingNeverShowsARank(t *testing.T) {
	a, st, clk := playApp(t)
	starsBelow(t, a, st, firstTeachLesson, course.Junior)
	playPart(t, a, clk, firstTeachLesson, partTraining)
	showsNone(t, a, "Hodnost")
}

func TestEarlierLessonOpenedLaterStillWorks(t *testing.T) {
	a, st, clk := playApp(t)
	lessons := a.Course.Lessons()
	later, earlier := lessons[20], lessons[3]

	// A late lesson opened first asks for what came before it.
	openPart(t, a, later.ID, partTest)
	spec := ui.RoundSpec(a)
	finishRound(t, a, clk, typingStep)
	if got := st.Profile().Stars[later.ID]; got == 0 {
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
	p := st.Profile()
	if p.Stars[earlier.ID] == 0 || p.Stars[later.ID] == 0 {
		t.Errorf("stars = %v", p.Stars)
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
	st.Profile().Stars[firstTeachLesson] = 2
	openPart(t, a, firstTeachLesson, partTraining)
	if strings.Contains(screen(a), "★") {
		t.Errorf("a Trénink shows stars:\n%s", screen(a))
	}
	openPart(t, a, firstTeachLesson, partTest)
	wantText(t, a, "★★☆")
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
	showsAll(t, a, text.PartVsLast, text.PartVsBest, text.PartMissed, text.PartTime)

	// The body is seven rows and two gaps, and the frame takes three more.
	blank := func() int {
		lines := strings.Split(screen(a), "\n")
		n := 0
		for _, l := range lines[1 : len(lines)-2] {
			if strings.IndexFunc(l, unicode.IsLetter) < 0 {
				n++
			}
		}
		return n
	}
	a.Update(tea.WindowSizeMsg{Width: 60, Height: 12})
	showsAll(t, a, text.PartVsLast, text.PartVsBest)
	if got := blank(); got != 2 {
		t.Errorf("%d blank rows with room for the gaps:\n%s", got, screen(a))
	}

	a.Update(tea.WindowSizeMsg{Width: 60, Height: 10}) // the gaps go first
	showsAll(t, a, text.PartVsLast, text.PartVsBest, text.PartMissed, text.PartTime)
	if got := blank(); got != 0 {
		t.Errorf("%d blank rows in a full window:\n%s", got, screen(a))
	}

	a.Update(tea.WindowSizeMsg{Width: 60, Height: 9})
	showsAll(t, a, text.PartVsLast, text.PartMissed, text.PartTime)
	showsNone(t, a, text.PartVsBest)

	for _, s := range style.All() {
		a.Style = s
		a.Update(tea.WindowSizeMsg{Width: 40, Height: 8})
		showsAll(t, a, text.PartSpeed, text.PartAccuracy, text.PartStars, "★")
		showsNone(t, a, text.PartVsLast, text.PartVsBest)
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
