package ui_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jiripisa/tuipe/internal/content"
	"github.com/jiripisa/tuipe/internal/recommend"
	"github.com/jiripisa/tuipe/internal/store"
	"github.com/jiripisa/tuipe/internal/ui"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

// playApp is an app on a fixed clock and an 80 × 24 window.
func playApp(t *testing.T) (*ui.App, *store.Store, *clock) {
	t.Helper()
	a, st := newApp(t)
	clk := &clock{t: time.Date(2026, 10, 31, 12, 0, 0, 0, time.Local)}
	a.Now = clk.now
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return a, st, clk
}

func key(r rune) tea.KeyPressMsg {
	if r == '\n' {
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	}
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

var (
	enter = tea.KeyPressMsg{Code: tea.KeyEnter}
	esc   = tea.KeyPressMsg{Code: tea.KeyEscape}
	down  = tea.KeyPressMsg{Code: tea.KeyDown}
)

// typeKeys types n runes of the round, 200 ms apart (all of them when n < 0).
func typeKeys(a *ui.App, clk *clock, n int) {
	keys := ui.RoundKeys(a)
	if n < 0 || n > len(keys) {
		n = len(keys)
	}
	for _, r := range keys[:n] {
		clk.advance(200 * time.Millisecond)
		a.Update(key(r))
	}
}

func screen(a *ui.App) string { return ansi.Strip(a.View().Content) }

func firstLesson(t *testing.T, a *ui.App) (id, title string) {
	t.Helper()
	for _, ch := range a.Curr.Chapters() {
		if len(ch.Lessons) > 0 {
			return ch.Lessons[0].ID, ch.Lessons[0].Title
		}
	}
	t.Fatal("no lesson")
	return "", ""
}

// startLesson opens the first lesson as a round.
func startLesson(t *testing.T, a *ui.App) string {
	t.Helper()
	id, _ := firstLesson(t, a)
	s := ui.NewRoundScreen(a, recommend.Pick{Kind: recommend.Lesson, LessonID: id}, func(*ui.App) ui.Screen {
		return ui.NewRoundScreen(a, recommend.Pick{Kind: recommend.Lesson, LessonID: id}, nil)
	})
	ui.SetScreen(a, s)
	a.Update(ui.TickMsg(a))
	return id
}

func TestEnterStartsRecommendedRoundImmediately(t *testing.T) {
	a, _, _ := playApp(t)
	if ui.ScreenName(a) != "home" {
		t.Fatalf("start screen = %s, want home", ui.ScreenName(a))
	}
	_, cmd := a.Update(enter)
	if ui.ScreenName(a) != "round" {
		t.Fatalf("after Enter: %s, want round", ui.ScreenName(a))
	}
	if cmd == nil {
		t.Error("the round must start its tick")
	}
	if len(ui.RoundKeys(a)) == 0 {
		t.Error("the round has no text to type")
	}
}

func TestFinishingARoundShowsDiffAgainstGhost(t *testing.T) {
	a, st, clk := playApp(t)
	startLesson(t, a)
	typeKeys(a, clk, -1)
	if ui.ScreenName(a) != "results" {
		t.Fatalf("after the last key: %s, want results", ui.ScreenName(a))
	}
	first := screen(a)
	if !strings.Contains(first, "+ wpm") || strings.Contains(first, "- wpm") {
		t.Errorf("first attempt must only add lines:\n%s", first)
	}
	runs, _ := st.Runs()
	if len(runs) != 1 || !strings.Contains(first, runs[0].ID) || len(runs[0].ID) != 7 {
		t.Fatalf("run id not shown or not 7 characters: %+v", runs)
	}

	// Second attempt at the same text, twice as slow.
	clk.advance(time.Minute)
	a.Update(enter)
	if ui.ScreenName(a) != "round" {
		t.Fatalf("Enter on results: %s", ui.ScreenName(a))
	}
	for _, r := range ui.RoundKeys(a) {
		clk.advance(400 * time.Millisecond)
		a.Update(key(r))
	}
	second := screen(a)
	for _, want := range []string{"- wpm", "+ wpm", "- přes.", "+ přes.", "- čas", "+ čas"} {
		if !strings.Contains(second, want) {
			t.Errorf("missing %q in\n%s", want, second)
		}
	}
	runs, _ = st.Runs()
	if len(runs) != 2 {
		t.Errorf("runs = %d, want 2", len(runs))
	}
}

func TestEnterOnResultsStartsNextRound(t *testing.T) {
	a, _, clk := playApp(t)
	startLesson(t, a)
	typeKeys(a, clk, -1)
	_, cmd := a.Update(enter)
	if ui.ScreenName(a) != "round" || cmd == nil {
		t.Errorf("screen %s, tick cmd nil: %v", ui.ScreenName(a), cmd == nil)
	}
}

func TestEscOnResultsGoesHome(t *testing.T) {
	a, _, clk := playApp(t)
	startLesson(t, a)
	typeKeys(a, clk, -1)
	a.Update(esc)
	if ui.ScreenName(a) != "home" {
		t.Errorf("screen = %s", ui.ScreenName(a))
	}
}

func TestEscPausesAndOffersResumeRestartLeave(t *testing.T) {
	a, _, clk := playApp(t)
	startLesson(t, a)
	typeKeys(a, clk, 3)
	a.Update(esc)
	v := screen(a)
	for _, want := range []string{"pokračovat", "začít znovu", "odejít"} {
		if !strings.Contains(v, want) {
			t.Errorf("pause menu lacks %q:\n%s", want, v)
		}
	}
	// A letter is ignored while the menu is open; Esc resumes.
	pos := ui.RoundPos(a)
	a.Update(key(ui.RoundKeys(a)[pos]))
	if ui.RoundPos(a) != pos {
		t.Error("typing moved the cursor during the Esc pause")
	}
	a.Update(esc)
	if strings.Contains(screen(a), "začít znovu") {
		t.Error("menu still shown after resume")
	}
	typeKeys(a, clk, 0)
	a.Update(key(ui.RoundKeys(a)[pos]))
	if ui.RoundPos(a) != pos+1 {
		t.Errorf("pos = %d, want %d", ui.RoundPos(a), pos+1)
	}
}

func TestCtrlRRestartsWithoutCountingAFailure(t *testing.T) {
	a, st, clk := playApp(t)
	startLesson(t, a)
	typeKeys(a, clk, 4)
	a.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	if ui.ScreenName(a) != "round" || ui.RoundPos(a) != 0 {
		t.Errorf("screen %s pos %d, want a fresh round", ui.ScreenName(a), ui.RoundPos(a))
	}
	if runs, _ := st.Runs(); len(runs) != 0 {
		t.Errorf("restart added %d runs", len(runs))
	}
	if st.Profile().Resume != nil {
		t.Error("restart must not set Resume")
	}
	// The new round has its own tick; the old one is dropped.
	if _, cmd := a.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl}); cmd == nil {
		t.Error("restart must schedule a tick")
	}
}

func TestAbandonedRoundFeedsStatsButNotRuns(t *testing.T) {
	a, st, clk := playApp(t)
	id := startLesson(t, a)
	typeKeys(a, clk, 5)
	a.Update(esc)
	a.Update(down)
	a.Update(down)
	a.Update(enter)
	if ui.ScreenName(a) != "home" {
		t.Fatalf("screen = %s, want home", ui.ScreenName(a))
	}
	if runs, _ := st.Runs(); len(runs) != 0 {
		t.Errorf("abandoning added %d runs", len(runs))
	}
	if n := len(st.Profile().Stats[a.Layout.ID].Chars); n == 0 {
		t.Error("keystrokes did not reach the stats")
	}
	if r := st.Profile().Resume; r == nil || r.LessonID != id {
		t.Errorf("Resume = %+v, want lesson %s", r, id)
	}
}

func TestLostFocusPauses(t *testing.T) {
	a, _, clk := playApp(t)
	startLesson(t, a)
	typeKeys(a, clk, 2)
	a.Update(tea.BlurMsg{})
	if !strings.Contains(screen(a), "PAUZA") {
		t.Errorf("no paused state:\n%s", screen(a))
	}
	if !a.Store.Profile().FocusEvents {
		t.Error("FocusEvents not remembered")
	}
	pos := ui.RoundPos(a)
	clk.advance(time.Minute)
	a.Update(key(ui.RoundKeys(a)[pos]))
	if ui.RoundPos(a) != pos+1 {
		t.Error("the next key must resume and count")
	}
}

func TestPasteIgnored(t *testing.T) {
	a, _, clk := playApp(t)
	startLesson(t, a)
	typeKeys(a, clk, 2)
	pos := ui.RoundPos(a)
	a.Update(tea.PasteMsg{Content: string(ui.RoundKeys(a)[pos : pos+3])})
	if ui.RoundPos(a) != pos {
		t.Errorf("paste moved the cursor from %d to %d", pos, ui.RoundPos(a))
	}
}

func TestShortcutsAreNotTypedInput(t *testing.T) {
	a, _, clk := playApp(t)
	startLesson(t, a)
	typeKeys(a, clk, 0)
	want := ui.RoundKeys(a)[0]
	a.Update(tea.KeyPressMsg{Code: want, Text: string(want), Mod: tea.ModCtrl})
	a.Update(tea.KeyPressMsg{Code: want, Text: string(want), Mod: tea.ModMeta})
	if ui.RoundPos(a) != 0 {
		t.Error("a Ctrl or Meta chord was typed")
	}
}

func TestResizeKeepsPosition(t *testing.T) {
	a, _, clk := playApp(t)
	startLesson(t, a)
	typeKeys(a, clk, 6)
	for _, size := range [][2]int{{60, 18}, {45, 10}, {120, 45}, {80, 24}} {
		a.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		if ui.RoundPos(a) != 6 {
			t.Fatalf("pos = %d at %v, want 6", ui.RoundPos(a), size)
		}
	}
}

func TestPauseAddsNoThroughputSamples(t *testing.T) {
	a, _, clk := playApp(t)
	startLesson(t, a)
	typeKeys(a, clk, 12) // 2.4 s of active time
	n := len(ui.RoundSamples(a))
	if n != 2 {
		t.Fatalf("samples = %d, want 2", n)
	}
	a.Update(esc)
	for range 100 {
		clk.advance(time.Second)
		a.Update(ui.TickMsg(a))
	}
	if got := len(ui.RoundSamples(a)); got != n {
		t.Errorf("pause added samples: %d -> %d", n, got)
	}
}

func TestPausedRunNeverReplacesGhost(t *testing.T) {
	a, st, clk := playApp(t)
	startLesson(t, a)
	typeKeys(a, clk, 3)
	a.Update(esc)
	clk.advance(time.Minute)
	a.Update(esc) // resume
	typeKeys(a, clk, -1)
	if ui.ScreenName(a) != "results" {
		t.Fatalf("screen = %s", ui.ScreenName(a))
	}
	runs, _ := st.Runs()
	g, err := st.Ghost(runs[0].TextHash)
	if err != nil || g != nil {
		t.Errorf("a run with a pause became a ghost: %+v %v", g, err)
	}
	if !runs[0].Paused {
		t.Error("run not marked paused")
	}
}

func TestFinishingSavesStarsAndClearsResume(t *testing.T) {
	a, st, clk := playApp(t)
	id := startLesson(t, a)
	st.Profile().Resume = &store.Resume{Kind: "lesson", LessonID: id}
	typeKeys(a, clk, -1)
	if st.Profile().Resume != nil {
		t.Error("Resume not cleared")
	}
	runs, _ := st.Runs()
	if st.Profile().Stars[id] != runs[0].Stars {
		t.Errorf("stars %d, run %d", st.Profile().Stars[id], runs[0].Stars)
	}
	if g, _ := st.Ghost(runs[0].TextHash); g == nil {
		t.Error("the first clean run must become the ghost")
	}
}

func TestSaveFailureStillShowsResults(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	st, err := store.Open(filepath.Join(root, "cfg"), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	lib, _ := content.Load()
	a, err := ui.New(st, lib)
	if err != nil {
		t.Fatal(err)
	}
	clk := &clock{t: time.Date(2026, 10, 31, 12, 0, 0, 0, time.Local)}
	a.Now = clk.now
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	startLesson(t, a)
	if err := os.Chmod(dataDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dataDir, 0o755) })
	typeKeys(a, clk, -1)
	if ui.ScreenName(a) != "results" {
		t.Fatalf("screen = %s, want results", ui.ScreenName(a))
	}
	v := screen(a)
	if !strings.Contains(v, "+ wpm") || !strings.Contains(v, "Uložení se nepovedlo") {
		t.Errorf("results or warning missing:\n%s", v)
	}
}

func TestResumeOfferedAfterRelaunch(t *testing.T) {
	a, st, clk := playApp(t)
	id := startLesson(t, a)
	_, title := firstLesson(t, a)
	typeKeys(a, clk, 3)
	a.Update(esc)
	a.Update(down)
	a.Update(down)
	a.Update(enter) // leave

	lib, _ := content.Load()
	b, err := ui.New(st, lib) // same data, as after a relaunch
	if err != nil {
		t.Fatal(err)
	}
	b.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if want := "> pokračuj: " + title; !strings.Contains(ansi.Strip(b.View().Content), want) {
		t.Errorf("home lacks %q:\n%s", want, ansi.Strip(b.View().Content))
	}
	b.Update(enter)
	if ui.ScreenName(b) != "round" {
		t.Fatalf("screen = %s", ui.ScreenName(b))
	}
	_ = id
}

func TestUnregisteredHomeEntryIsNotReady(t *testing.T) {
	a, _, _ := playApp(t)
	a.Update(down)
	a.Update(enter)
	if ui.ScreenName(a) != "home" || !strings.Contains(screen(a), "zatím není hotové") {
		t.Errorf("expected the not-ready note:\n%s", screen(a))
	}
}
