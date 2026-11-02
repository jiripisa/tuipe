package ui_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jiripisa/tuipe/internal/content"
	"github.com/jiripisa/tuipe/internal/stats"
	"github.com/jiripisa/tuipe/internal/store"
	"github.com/jiripisa/tuipe/internal/ui"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

const homeCareer = 0 // downs from the first home entry to Kariéra

// openFromHome presses Down n times on the home screen and Enter.
func openFromHome(t *testing.T, a *ui.App, n int, want string) {
	t.Helper()
	for range n {
		a.Update(down)
	}
	a.Update(enter)
	if got := ui.ScreenName(a); got != want {
		t.Fatalf("after opening the entry: screen %s, want %s", got, want)
	}
}

// tall gives the app a window that shows the whole career list.
func tall(a *ui.App) { a.Update(tea.WindowSizeMsg{Width: 100, Height: 90}) }

// lessonRows counts the lessons a chapter lists: an unlocked one all of them,
// a locked one none.
func lessonRows(a *ui.App, ch content.ChapterDef) int {
	if a.Curr.IsUnlocked(a.Store.Profile(), ch.ID) {
		return len(ch.Lessons)
	}
	return 0
}

// rowsBefore counts the selectable rows of the career list above a chapter's
// own rows on a fresh profile: lessons of unlocked chapters and one test-out
// for each chapter but the first.
func rowsBefore(a *ui.App, chapterID string) int {
	n := 0
	for _, ch := range a.Curr.Chapters() {
		if ch.ID == chapterID {
			return n
		}
		n += lessonRows(a, ch)
		if ch.Number > 0 {
			n++
		}
	}
	return n
}

// openTestOut opens the career list and starts the test-out of a chapter.
func openTestOut(t *testing.T, a *ui.App, chapterID string) {
	t.Helper()
	openFromHome(t, a, homeCareer, "career")
	downs := rowsBefore(a, chapterID)
	for _, ch := range a.Curr.Chapters() {
		if ch.ID == chapterID {
			downs += lessonRows(a, ch)
		}
	}
	for range downs {
		a.Update(down)
	}
	a.Update(enter)
	if ui.ScreenName(a) != "round" {
		t.Fatalf("test-out of %s did not start a round:\n%s", chapterID, view(a))
	}
}

// typeWrongThenRight types every key twice over: a wrong one, then the right one.
func typeWrongThenRight(a *ui.App, clk *clock) {
	for _, r := range ui.RoundKeys(a) {
		wrong := '~'
		if r == wrong {
			wrong = '^'
		}
		clk.advance(100 * time.Millisecond)
		a.Update(key(wrong))
		clk.advance(100 * time.Millisecond)
		a.Update(key(r))
	}
}

func TestCareerShowsChaptersStarsAndRank(t *testing.T) {
	a, st, _ := playApp(t)
	tall(a)
	p := st.Profile()
	chs := a.Curr.Chapters()
	for _, ch := range chs[1:5] { // chapters 1 to 4, up to Junior
		a.Curr.CompleteChapter(p, ch.ID)
	}
	p.Stars[chs[1].Lessons[0].ID] = 2
	p.Stars[chs[1].Lessons[1].ID] = 3
	openFromHome(t, a, homeCareer, "career")
	v := view(a)

	rankLine := ""
	for _, l := range strings.Split(v, "\n") {
		if strings.Contains(l, text.HomeRank) {
			rankLine = l
		}
	}
	if !strings.Contains(rankLine, "Junior") {
		t.Errorf("rank line %q lacks the held rank Junior", rankLine)
	}
	for _, ch := range chs[:6] {
		if !strings.Contains(v, ch.Title) {
			t.Errorf("chapter %q not listed:\n%s", ch.Title, v)
		}
	}
	// A completed chapter still lists its lessons with their best stars.
	wantLine := func(title, stars string) {
		t.Helper()
		for _, l := range strings.Split(v, "\n") {
			if strings.Contains(l, title) {
				if !strings.Contains(l, stars) {
					t.Errorf("lesson %q shows %q, want %q", title, l, stars)
				}
				return
			}
		}
		t.Errorf("lesson %q not listed:\n%s", title, v)
	}
	wantLine(chs[1].Lessons[0].Title, "★★☆")
	wantLine(chs[1].Lessons[1].Title, "★★★")
	wantLine(chs[1].Lessons[2].Title, "☆☆☆")
	// The current chapter 5 is open: its lessons are listed. Chapter 6 is locked.
	if !strings.Contains(v, chs[5].Lessons[0].Title) {
		t.Errorf("lessons of the unlocked chapter 5 not listed:\n%s", v)
	}
	if strings.Contains(v, chs[6].Lessons[0].Title) {
		t.Errorf("lessons of the locked chapter 6 are listed:\n%s", v)
	}
}

func TestLockedChapterOffersTestOutOnly(t *testing.T) {
	a, _, _ := playApp(t)
	tall(a)
	openFromHome(t, a, homeCareer, "career")
	chs := a.Curr.Chapters()
	v := view(a)
	if !strings.Contains(v, chs[2].Title) {
		t.Fatalf("locked chapter missing:\n%s", v)
	}
	if strings.Contains(v, chs[2].Lessons[0].Title) {
		t.Errorf("a locked chapter lists its lessons:\n%s", v)
	}
	if got := strings.Count(v, "test-out:"); got != len(chs)-1 {
		t.Errorf("test-out offered %d times, want once per chapter but the first (%d)", got, len(chs)-1)
	}

	// Enter on the test-out of chapter 2 starts it on the test-out text.
	for range rowsBefore(a, chs[2].ID) {
		a.Update(down)
	}
	a.Update(enter)
	if ui.ScreenName(a) != "round" {
		t.Fatalf("screen = %s, want round", ui.ScreenName(a))
	}
	want, _ := a.Curr.TestOutText(chs[2].ID)
	if got := string(ui.RoundKeys(a)); got != want {
		t.Errorf("round text = %q, want the test-out text %q", got, want)
	}
	if ui.RoundLessonID(a) != "" {
		t.Errorf("a test-out belongs to no lesson, got %q", ui.RoundLessonID(a))
	}
}

// playAppAt is playApp with a known data directory.
func playAppAt(t *testing.T) (*ui.App, *store.Store, *clock, string) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "cfg"), filepath.Join(dir, "data"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	lib, err := content.Load()
	if err != nil {
		t.Fatal(err)
	}
	st.Profile().FirstRunDone = true // these tests start on the home screen
	clk := newClock()
	a, err := ui.New(st, lib, ui.WithClock(clk.now))
	if err != nil {
		t.Fatal(err)
	}
	return a, st, clk, filepath.Join(dir, "data")
}

func TestUnlockedChapterOffersItsTestOutAfterItsLessons(t *testing.T) {
	a, _, _ := playApp(t)
	tall(a)
	openFromHome(t, a, homeCareer, "career")
	v := view(a)
	ch := a.Curr.Chapters()[1] // unlocked, not done
	last := strings.Index(v, ch.Lessons[len(ch.Lessons)-1].Title)
	row := strings.Index(v, "test-out:")
	if last < 0 || row < last || row > strings.Index(v, a.Curr.Chapters()[2].Title) {
		t.Fatalf("no test-out between the last lesson of %s and the next chapter:\n%s", ch.ID, v)
	}
}

func TestChapterZeroOffersNoTestOut(t *testing.T) {
	a, _, _ := playApp(t)
	tall(a)
	openFromHome(t, a, homeCareer, "career")
	v := view(a)
	if at := strings.Index(v, "test-out:"); at < strings.Index(v, a.Curr.Chapters()[1].Title) {
		t.Errorf("a test-out is listed before chapter 1:\n%s", v)
	}
}

func TestDoneChapterOffersNoTestOut(t *testing.T) {
	a, st, _ := playApp(t)
	tall(a)
	chs := a.Curr.Chapters()
	a.Curr.CompleteChapter(st.Profile(), chs[1].ID)
	openFromHome(t, a, homeCareer, "career")
	if got, want := strings.Count(view(a), "test-out:"), len(chs)-2; got != want {
		t.Errorf("test-out offered %d times, want %d: a done chapter has none:\n%s", got, want, view(a))
	}
}

// Before a test-out that closes several chapters starts, its row says how many.
func TestTestOutRowSaysHowManyChaptersItCloses(t *testing.T) {
	a, st, _ := playApp(t)
	tall(a)
	chs := a.Curr.Chapters()
	st.Profile().Stars[chs[0].Lessons[0].ID] = 1 // chapter 0 is done
	openFromHome(t, a, homeCareer, "career")
	v := view(a)
	if !strings.Contains(v, text.CareerTestOut) {
		t.Errorf("the test-out of chapter 1 closes one chapter and should say so plainly:\n%s", v)
	}
	for n := 2; n <= 12; n++ {
		if !strings.Contains(v, text.CareerTestOutMany(n)) {
			t.Errorf("no row says it closes %d chapters:\n%s", n, v)
		}
	}
}

func TestFailingTestOutShowsVerdictWithoutClosingAnything(t *testing.T) {
	a, _, clk := playApp(t)
	tall(a)
	openTestOut(t, a, a.Curr.Chapters()[1].ID) // an unlocked chapter
	typeWrongThenRight(a, clk)
	wantText(t, a, text.TestOutFailed)
}

func TestTestOutOfUnlockedChapterClosesIt(t *testing.T) {
	a, st, clk := playApp(t)
	tall(a)
	ch := a.Curr.Chapters()[1]
	openTestOut(t, a, ch.ID)
	want, _ := a.Curr.TestOutText(ch.ID)
	if got := string(ui.RoundKeys(a)); got != want {
		t.Fatalf("round text = %q, want the test-out text %q", got, want)
	}
	typeKeys(a, clk, -1)
	if !st.Profile().Completed[ch.ID] {
		t.Errorf("the test-out of unlocked %s did not close it:\n%s", ch.ID, view(a))
	}
	wantText(t, a, text.TestOutPassed(2)) // onboarding and chapter 1
}

// The walk that made a fresh Intern a Principal in 33 seconds: plain words do
// not pass any more, because the text asks for the whole keyboard.
func TestTestOutOfEverythingAsksForTheRareKeys(t *testing.T) {
	a, _, _ := playApp(t)
	tall(a)
	chs := a.Curr.Chapters()
	openTestOut(t, a, chs[12].ID)
	keys := string(ui.RoundKeys(a))
	for _, r := range "{}§°ˇ¨\n" {
		if !strings.ContainsRune(keys, r) {
			t.Errorf("the test-out of everything does not ask for %q: %q", r, keys)
		}
	}
}

func TestPassingTestOutCompletesChapterWithoutStars(t *testing.T) {
	a, st, clk, dataDir := playAppAt(t)
	tall(a)
	chs := a.Curr.Chapters()
	ch := chs[4] // Junior
	p := st.Profile()
	p.Resume = &store.Resume{Kind: "lesson", LessonID: chs[1].Lessons[0].ID, Day: "2026-10-30"}
	var introduced []rune
	for _, r := range ch.Introduces {
		if a.Layout.Unresolved(string(r)) == nil && r != ' ' {
			introduced = append(introduced, r)
		}
	}
	if a.Curr.RankOf(p).String() != "Intern" || p.Completed[ch.ID] {
		t.Fatal("precondition: nothing is completed")
	}
	for _, r := range introduced {
		if a.Layout.Unresolved(string(r)) == nil && stats.New().MasteryOf(r) != stats.NewChar {
			t.Fatal("precondition: characters start new")
		}
	}

	openTestOut(t, a, ch.ID)
	typeKeys(a, clk, -1)
	if ui.ScreenName(a) != "results" {
		t.Fatalf("screen = %s, want results", ui.ScreenName(a))
	}
	if !strings.Contains(view(a), text.TestOutPassed(5)) {
		t.Errorf("results do not say the test-out passed and closed 5 chapters:\n%s", view(a))
	}
	if strings.ContainsAny(view(a), "★☆") {
		t.Errorf("a test-out shows stars:\n%s", view(a))
	}
	if !strings.Contains(view(a), fmt.Sprintf(text.ResultsRank, "Intern", "Junior")) {
		t.Errorf("results do not name the new rank:\n%s", view(a))
	}
	if !p.Completed[ch.ID] {
		t.Error("the chapter is not completed")
	}
	if got := a.Curr.RankOf(p).String(); got != "Junior" {
		t.Errorf("rank = %s, want Junior", got)
	}
	if len(p.Stars) != 0 {
		t.Errorf("a test-out wrote stars: %v", p.Stars)
	}
	if p.Resume == nil || p.Resume.LessonID != chs[1].Lessons[0].ID {
		t.Errorf("a test-out touched Resume: %+v", p.Resume)
	}
	s := p.Stats[a.Layout.ID]
	for _, r := range introduced {
		if s.MasteryOf(r) == stats.NewChar {
			t.Errorf("%q is still new after the skip", r)
		}
	}
	runs, _ := st.Runs()
	if len(runs) != 1 || runs[0].Mode != "testout" || runs[0].LessonID != "" || runs[0].Stars < 2 {
		t.Fatalf("runs = %+v, want one testout run with at least 2 stars", runs)
	}
	if g, _ := st.Ghost(runs[0].TextHash); g != nil {
		t.Error("a test-out result must not become a ghost")
	}
	raw, err := os.ReadFile(filepath.Join(dataDir, "profile.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved store.Profile
	if err := json.Unmarshal(raw, &saved); err != nil || !saved.Completed[ch.ID] || len(saved.Stars) != 0 {
		t.Errorf("saved profile: completed %v stars %v err %v", saved.Completed, saved.Stars, err)
	}

	a.Update(enter)
	if ui.ScreenName(a) != "career" {
		t.Errorf("Enter on the results: %s, want the career list", ui.ScreenName(a))
	}
}

func TestFailingTestOutChangesNothing(t *testing.T) {
	a, st, clk := playApp(t)
	tall(a)
	ch := a.Curr.Chapters()[4]
	p := st.Profile()
	openTestOut(t, a, ch.ID)
	typeWrongThenRight(a, clk)
	if ui.ScreenName(a) != "results" {
		t.Fatalf("screen = %s, want results", ui.ScreenName(a))
	}
	if !strings.Contains(view(a), text.TestOutFailed) {
		t.Errorf("results do not say the test-out failed:\n%s", view(a))
	}
	if strings.ContainsAny(view(a), "★☆") {
		t.Errorf("a failed test-out shows stars:\n%s", view(a))
	}
	if len(p.Completed) != 0 || len(p.Stars) != 0 || a.Curr.RankOf(p).String() != "Intern" {
		t.Errorf("a failed test-out changed progress: completed %v stars %v", p.Completed, p.Stars)
	}
	for _, r := range ch.Introduces {
		if r != ' ' && p.Stats[a.Layout.ID].MasteryOf(r) == stats.Solid {
			t.Errorf("%q became solid without a pass", r)
		}
	}
	runs, _ := st.Runs()
	if len(runs) != 1 || runs[0].Mode != "testout" || runs[0].Stars >= 2 {
		t.Errorf("runs = %+v, want one testout run under 2 stars", runs)
	}
	if g, _ := st.Ghost(runs[0].TextHash); g != nil {
		t.Error("a failed test-out must not become a ghost")
	}
}

func TestPassingTestOutMovesTheCareerOn(t *testing.T) {
	a, st, clk := playApp(t)
	tall(a)
	chs := a.Curr.Chapters()
	p := st.Profile()
	openTestOut(t, a, chs[4].ID)
	typeKeys(a, clk, -1)
	if !p.Completed[chs[4].ID] {
		t.Fatal("test-out not passed")
	}
	for _, ch := range chs[:5] {
		if !a.Curr.IsDone(p, ch.ID) {
			t.Errorf("chapter %s (%s) is not done, though the test-out proves it", ch.ID, ch.Title)
		}
	}
	if len(p.Stars) != 0 {
		t.Errorf("a test-out wrote stars: %v", p.Stars)
	}
	if got := a.Curr.CurrentChapter(p).ID; got != chs[5].ID {
		t.Errorf("current chapter = %s, want %s", got, chs[5].ID)
	}
	if !a.Curr.IsUnlocked(p, chs[5].ID) {
		t.Error("chapter 5 is still locked")
	}
	next, ok := a.Curr.NextLesson(p)
	if !ok || next.ID != chs[5].Lessons[0].ID {
		t.Errorf("next lesson = %v %v, want %s", next.ID, ok, chs[5].Lessons[0].ID)
	}
}

func selectedLine(a *ui.App) string {
	for _, l := range strings.Split(view(a), "\n") {
		if strings.Contains(l, "> ") {
			return l
		}
	}
	return ""
}

func TestCareerOpensOnTheNextLesson(t *testing.T) {
	a, st, _ := playApp(t)
	a.Update(tea.WindowSizeMsg{Width: 60, Height: 18})
	p := st.Profile()
	chs := a.Curr.Chapters()
	a.Curr.CompleteChapter(p, chs[0].ID)
	for _, ch := range chs[1:4] {
		a.Curr.CompleteChapter(p, ch.ID)
	}
	p.Stars[chs[4].Lessons[0].ID] = 1
	p.Stars[chs[4].Lessons[1].ID] = 2
	openFromHome(t, a, homeCareer, "career")
	if want := chs[4].Lessons[2].Title; !strings.Contains(selectedLine(a), want) {
		t.Errorf("selected line %q, want the next lesson %q", selectedLine(a), want)
	}
}

func TestCareerListHasNoIncidentRows(t *testing.T) {
	a, st, _ := playApp(t)
	tall(a)
	p := st.Profile()
	chs := a.Curr.Chapters()
	a.Curr.CompleteChapter(p, chs[0].ID)
	for _, l := range chs[1].Lessons {
		p.Stars[l.ID] = 1
	}
	openFromHome(t, a, homeCareer, "career")
	if v := view(a); strings.Contains(v, "INC-") {
		t.Errorf("the career list shows an incident row:\n%s", v)
	}
	// Chapter 1 is done, so the list opens on the first lesson of chapter 2.
	if want := chs[2].Lessons[0].Title; !strings.Contains(selectedLine(a), want) {
		t.Errorf("selected line %q, want the next lesson %q", selectedLine(a), want)
	}
	for range 60 {
		a.Update(down)
		if got := ui.ScreenName(a); got != "career" {
			t.Fatalf("pressing Down left the list for %s", got)
		}
		if strings.Contains(selectedLine(a), "INC-") {
			t.Fatalf("an incident row was selected: %q", selectedLine(a))
		}
	}
}

func TestCareerKeepsSelectionAfterATestOut(t *testing.T) {
	a, _, clk := playApp(t)
	tall(a)
	chs := a.Curr.Chapters()
	openTestOut(t, a, chs[4].ID)
	typeWrongThenRight(a, clk)
	a.Update(enter)
	if ui.ScreenName(a) != "career" || !strings.Contains(selectedLine(a), "test-out:") {
		t.Fatalf("after a failed test-out the same test-out stays selected, got %q", selectedLine(a))
	}
	a.Update(enter)
	want, _ := a.Curr.TestOutText(chs[4].ID)
	if got := string(ui.RoundKeys(a)); got != want {
		t.Fatalf("the selection moved to another row: round text %q", got)
	}
	typeKeys(a, clk, -1) // this time it passes
	a.Update(enter)
	if want := chs[5].Lessons[0].Title; !strings.Contains(selectedLine(a), want) {
		t.Errorf("after the pass the list reflows: selected %q, want next lesson %q", selectedLine(a), want)
	}
}

func TestTestOutWithOneStarDoesNotPass(t *testing.T) {
	a, st, clk := playApp(t)
	tall(a)
	ch := a.Curr.Chapters()[4]
	_, targets := a.Curr.TestOutText(ch.ID)
	if targets.Speed[0] <= 0 || targets.Speed[1] <= targets.Speed[0] {
		t.Fatalf("unusable targets %v", targets.Speed)
	}
	// Between the speeds of the first and the second star, without a mistake:
	// a characters-per-word of 5 makes the pace 12 / gap words per minute.
	wpm := (targets.Speed[0] + targets.Speed[1]) / 2
	gap := time.Duration(12 / wpm * float64(time.Second))
	openTestOut(t, a, ch.ID)
	for _, r := range ui.RoundKeys(a) {
		clk.advance(gap)
		a.Update(key(r))
	}
	runs, _ := st.Runs()
	if len(runs) != 1 || runs[0].Stars != 1 {
		t.Fatalf("runs = %+v, want one run with exactly 1 star", runs)
	}
	if st.Profile().Completed[ch.ID] || !strings.Contains(view(a), text.TestOutFailed) {
		t.Errorf("one star must not pass a test-out:\n%s", view(a))
	}
}

func TestTestOutShowsNoHints(t *testing.T) {
	a, _, _ := playApp(t)
	tall(a)
	// Control: the same fresh profile shows a hint at the start of a lesson.
	startLesson(t, a)
	if ui.RoundHint(a) == nil {
		t.Fatal("control: a lesson on a fresh profile should show a hint")
	}
	ui.SetScreen(a, ui.NewHomeScreen(a))
	openTestOut(t, a, a.Curr.Chapters()[2].ID)
	a.Update(ui.TickMsg(a))
	if h := ui.RoundHint(a); h != nil {
		t.Errorf("the test-out shows a hint: %+v", h)
	}
}

func TestCareerScrollsKeepingSelectionVisible(t *testing.T) {
	a, _, _ := playApp(t)
	a.Update(tea.WindowSizeMsg{Width: 60, Height: 18})
	openFromHome(t, a, homeCareer, "career")
	total := rowsBefore(a, "") // every selectable row on a fresh profile
	for i := 1; i < total; i++ {
		a.Update(down)
		v := view(a)
		if n := len(strings.Split(screen(a), "\n")); n > 18 {
			t.Fatalf("row %d: %d lines, want at most 18", i, n)
		}
		if strings.Count(v, "> ") != 1 {
			t.Fatalf("row %d: the selection is not on screen exactly once:\n%s", i, v)
		}
	}
	if !strings.Contains(view(a), "test-out:") {
		t.Errorf("the last row should be a test-out:\n%s", view(a))
	}
	// Up again to the very first row.
	for i := 1; i < total; i++ {
		a.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	}
	first := a.Curr.Chapters()[0].Lessons[0].Title
	if !strings.Contains(view(a), "> ") || !strings.Contains(view(a), first) {
		t.Errorf("did not scroll back to %q:\n%s", first, view(a))
	}
}

func TestEscLeavesCareerForHome(t *testing.T) {
	a, _, _ := playApp(t)
	openFromHome(t, a, homeCareer, "career")
	a.Update(esc)
	if ui.ScreenName(a) != "home" {
		t.Errorf("screen = %s, want home", ui.ScreenName(a))
	}
}

func TestCareerLessonRoundKeepsStars(t *testing.T) {
	a, st, clk := playApp(t)
	tall(a)
	openFromHome(t, a, homeCareer, "career")
	a.Update(enter) // the first lesson
	id, _ := firstLesson(t, a)
	if ui.RoundLessonID(a) != id {
		t.Fatalf("round lesson = %q, want %q", ui.RoundLessonID(a), id)
	}
	typeKeys(a, clk, -1)
	runs, _ := st.Runs()
	if len(runs) != 1 || runs[0].Mode != "lesson" {
		t.Fatalf("runs = %+v, want one lesson run", runs)
	}
	if st.Profile().Stars[id] == 0 {
		t.Error("a career lesson must record its stars")
	}
	a.Update(enter)
	if ui.ScreenName(a) != "career" {
		t.Errorf("Enter on the results: %s, want the career list", ui.ScreenName(a))
	}
	if !strings.Contains(view(a), "★") {
		t.Errorf("the list does not show the new stars:\n%s", view(a))
	}
}

// typeText types a string into the focused field, one key press per rune.
func typeText(a *ui.App, s string) {
	for _, r := range s {
		a.Update(tea.KeyPressMsg{Text: string(r)})
	}
}

// view is the screen without its blank lines, so failures stay readable.
func view(a *ui.App) string {
	var out []string
	for _, l := range strings.Split(screen(a), "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, strings.TrimRight(l, " "))
		}
	}
	return strings.Join(out, "\n")
}

func TestTestOutVerdictStaysInAShortWindow(t *testing.T) {
	a, _, clk, _ := playAppAt(t)
	tall(a)
	openTestOut(t, a, a.Curr.Chapters()[1].ID)
	typeKeys(a, clk, -1)
	for _, h := range []int{8, 9} {
		a.Update(tea.WindowSizeMsg{Width: 60, Height: h})
		if !strings.Contains(view(a), text.TestOutPassed(2)) {
			t.Errorf("height %d shows no verdict:\n%s", h, view(a))
		}
	}
}

func TestFailedTestOutCanBeRepeated(t *testing.T) {
	a, _, clk := playApp(t)
	tall(a)
	openTestOut(t, a, a.Curr.Chapters()[4].ID)
	typeWrongThenRight(a, clk)
	if !strings.Contains(view(a), text.TestOutFailed) {
		t.Fatalf("precondition: the test-out failed:\n%s", view(a))
	}
	a.Update(key('r'))
	if ui.ScreenName(a) != "round" {
		t.Fatalf("screen = %s, want round", ui.ScreenName(a))
	}
}

func TestPassedTestOutHasNothingToRepeat(t *testing.T) {
	a, _, clk := playApp(t)
	tall(a)
	openTestOut(t, a, a.Curr.Chapters()[1].ID)
	typeKeys(a, clk, -1)
	if strings.Contains(view(a), text.HintResultsRepeat) {
		t.Errorf("the repeat is offered:\n%s", view(a))
	}
	a.Update(key('r'))
	if ui.ScreenName(a) != "results" {
		t.Fatalf("screen = %s, want results", ui.ScreenName(a))
	}
}
