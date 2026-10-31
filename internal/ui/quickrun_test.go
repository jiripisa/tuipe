package ui_test

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jiripisa/tuipe/internal/content"
	"github.com/jiripisa/tuipe/internal/engine"
	"github.com/jiripisa/tuipe/internal/play"
	"github.com/jiripisa/tuipe/internal/recommend"
	"github.com/jiripisa/tuipe/internal/score"
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
	runs, _ = st.Runs()
	if len(runs) != 2 {
		t.Fatalf("runs = %d, want 2", len(runs))
	}
	assertDiff(t, second, runs[0], runs[1])
	if !strings.Contains(second, "--- HEAD~1") {
		t.Errorf("no baseline header:\n%s", second)
	}
	if strings.Contains(first, "--- HEAD~1") {
		t.Errorf("a first attempt has no HEAD~1:\n%s", first)
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
	clk.advance(200 * time.Millisecond)
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
	if r := st.Profile().Resume; r == nil {
		t.Error("the lesson stays offered for continuing after a restart")
	}
	if n := len(st.Profile().Stats[a.Layout.ID].Chars); n == 0 {
		t.Error("the keys before the restart must feed the stats")
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
	if len(a.Store.Profile().FocusTerminals) != 1 {
		t.Errorf("focus reporting not remembered: %v", a.Store.Profile().FocusTerminals)
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
	a, _, _ := playApp(t)
	startLesson(t, a)
	want := ui.RoundKeys(a)[0]
	a.Update(tea.KeyPressMsg{Code: want, Text: string(want), Mod: tea.ModCtrl})
	a.Update(tea.KeyPressMsg{Code: want, Text: string(want), Mod: tea.ModMeta})
	if ui.RoundPos(a) != 0 {
		t.Error("a Ctrl or Meta chord was typed")
	}
}

func TestResizeKeepsPositionAndTooSmallDoesNotCountTime(t *testing.T) {
	a, _, clk := playApp(t)
	startLesson(t, a)
	a.Update(tea.FocusMsg{}) // focus is reported, so only the window can pause
	typeKeys(a, clk, 6)
	for _, size := range [][2]int{{60, 18}, {45, 10}, {120, 45}, {80, 24}} {
		a.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		if ui.RoundPos(a) != 6 {
			t.Fatalf("pos = %d at %v, want 6", ui.RoundPos(a), size)
		}
	}
	before := ui.RoundActive(a)
	a.Update(tea.WindowSizeMsg{Width: 39, Height: 7})
	for range 60 {
		clk.advance(time.Second)
		a.Update(ui.TickMsg(a))
	}
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if got := ui.RoundActive(a); got > before+time.Second {
		t.Errorf("a minute at 39x7 counted: %v -> %v", before, got)
	}
	if ui.RoundPos(a) != 6 {
		t.Errorf("pos = %d after the small window, want 6", ui.RoundPos(a))
	}
	a.Update(enter) // resume in the pause menu
	clk.advance(200 * time.Millisecond)
	a.Update(key(ui.RoundKeys(a)[6]))
	if ui.RoundPos(a) != 7 {
		t.Errorf("pos = %d, want 7 after resuming", ui.RoundPos(a))
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

func TestUnregisteredHomeEntryIsNotReady(t *testing.T) {
	a, _, _ := playApp(t)
	a.Update(down)
	a.Update(enter)
	if ui.ScreenName(a) != "home" || !strings.Contains(screen(a), "zatím není hotové") {
		t.Errorf("expected the not-ready note:\n%s", screen(a))
	}
}

// assertDiff checks that old values are on '-' lines and new values on '+' lines.
func assertDiff(t *testing.T, view string, old, cur store.Run) {
	t.Helper()
	if old.WPM == cur.WPM {
		t.Fatal("test needs different speeds")
	}
	for _, want := range []string{
		fmt.Sprintf("- wpm   %6.1f", old.WPM),
		fmt.Sprintf("+ wpm   %6.1f", cur.WPM),
		fmt.Sprintf("- přes. %5.1f %%", old.Accuracy*100),
		fmt.Sprintf("+ přes. %5.1f %%", cur.Accuracy*100),
		fmt.Sprintf("- čas   %6.1f s", float64(old.DurationMs)/1000),
		fmt.Sprintf("+ čas   %6.1f s", float64(cur.DurationMs)/1000),
	} {
		if !strings.Contains(view, want) {
			t.Errorf("missing %q in\n%s", want, view)
		}
	}
}

// playLesson plays the first lesson to the end at the given pace, optionally with a pause.
func playLesson(t *testing.T, a *ui.App, clk *clock, step time.Duration, withPause bool) {
	t.Helper()
	startLesson(t, a)
	for i, r := range ui.RoundKeys(a) {
		clk.advance(step)
		if withPause && i == 5 {
			a.Update(esc)
			clk.advance(time.Minute)
			a.Update(esc)
		}
		a.Update(key(r))
	}
	if ui.ScreenName(a) != "results" {
		t.Fatalf("screen = %s", ui.ScreenName(a))
	}
}

func TestDiffIsAgainstTheBestRunThatDidNotPause(t *testing.T) {
	a, st, clk := playApp(t)
	playLesson(t, a, clk, 100*time.Millisecond, true) // fast but paused
	playLesson(t, a, clk, 200*time.Millisecond, false)
	playLesson(t, a, clk, 400*time.Millisecond, false)
	runs, _ := st.Runs()
	if len(runs) != 3 || !runs[0].Paused || runs[0].Score <= runs[1].Score {
		t.Fatalf("setup: %+v", runs)
	}
	assertDiff(t, screen(a), runs[1], runs[2])
}

func execCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-ch:
	case <-time.After(2 * time.Second):
		return nil
	}
	if b, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range b {
			out = append(out, execCmd(c)...)
		}
		return out
	}
	if msg == nil {
		return nil
	}
	return []tea.Msg{msg}
}

func TestTickLoop(t *testing.T) {
	a, _, clk := playApp(t)
	_, cmd := a.Update(enter)
	first := execCmd(cmd)
	if len(first) != 1 {
		t.Fatalf("starting a round must schedule one tick, got %v", first)
	}
	// An update that is not a tick does not start a second loop.
	clk.advance(200 * time.Millisecond)
	if _, cmd := a.Update(key(ui.RoundKeys(a)[0])); len(execCmd(cmd)) != 0 {
		t.Error("a key press scheduled another tick")
	}
	// A tick reschedules itself.
	_, cmd = a.Update(first[0])
	again := execCmd(cmd)
	if len(again) != 1 {
		t.Fatalf("a tick must schedule the next one, got %v", again)
	}
	// A restart replaces the loop: the old tick is dropped, the new one lives.
	_, cmd = a.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	fresh := execCmd(cmd)
	if len(fresh) != 1 || fmt.Sprint(fresh[0]) == fmt.Sprint(again[0]) {
		t.Fatalf("restart must start a tick of a new generation: %v vs %v", fresh, again)
	}
	if _, cmd := a.Update(again[0]); len(execCmd(cmd)) != 0 {
		t.Error("a tick of the old generation kept running")
	}
	if _, cmd := a.Update(fresh[0]); len(execCmd(cmd)) != 1 {
		t.Error("the new generation stopped")
	}
}

func TestTickAdvancesTheSession(t *testing.T) {
	a, _, clk := playApp(t)
	startLesson(t, a)
	typeKeys(a, clk, 1)
	clk.advance(6 * time.Second) // no focus reports: 5 s of silence pause the round
	if strings.Contains(screen(a), "PAUZA") {
		t.Fatal("paused before any tick")
	}
	a.Update(ui.TickMsg(a))
	if !strings.Contains(screen(a), "PAUZA") {
		t.Error("a tick did not reach the session")
	}
}

func TestFocusKnowledgeIsPerTerminal(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TERM_PROGRAM", "alpha")
	t.Setenv("TMUX", "")
	a1, st1 := openApp(t, dir)
	a1.Update(tea.FocusMsg{})
	_ = st1.Close()

	idle := func(a *ui.App, clk *clock) bool {
		startLesson(t, a)
		typeKeys(a, clk, 1)
		for range 8 {
			clk.advance(time.Second)
			a.Update(ui.TickMsg(a))
		}
		return strings.Contains(screen(a), "PAUZA")
	}
	a2, st2 := openApp(t, dir)
	clk := &clock{t: time.Now()}
	a2.Now = clk.now
	if idle(a2, clk) {
		t.Error("a terminal known to report focus must not pause on idle")
	}
	_ = st2.Close()

	t.Setenv("TERM_PROGRAM", "beta")
	a3, _ := openApp(t, dir)
	clk3 := &clock{t: time.Now()}
	a3.Now = clk3.now
	if !idle(a3, clk3) {
		t.Error("a terminal without focus reports must pause on idle")
	}
}

func openApp(t *testing.T, dir string) (*ui.App, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(dir, "cfg"), filepath.Join(dir, "data"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	lib, _ := content.Load()
	a, err := ui.New(st, lib)
	if err != nil {
		t.Fatal(err)
	}
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return a, st
}

// runProgram runs the app in a real tea.Program on a pipe and returns once it ends.
func runProgram(t *testing.T, a *ui.App, input string, stop func()) {
	t.Helper()
	pr, pw := io.Pipe()
	defer pw.Close()
	done := make(chan error, 1)
	go func() {
		done <- ui.RunApp(a, tea.WithInput(pr), tea.WithOutput(io.Discard), tea.WithWindowSize(80, 24))
	}()
	time.Sleep(300 * time.Millisecond)
	_, _ = pw.Write([]byte(input))
	time.Sleep(300 * time.Millisecond)
	stop()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the program did not end")
	}
}

func testLeave(t *testing.T, suffix string, stop func()) {
	dir := t.TempDir()
	a, st := openApp(t, dir)
	id := startLesson(t, a)
	keys := string(ui.RoundKeys(a)[:3])
	runProgram(t, a, keys+suffix, stop)
	if runs, _ := st.Runs(); len(runs) != 0 {
		t.Errorf("leaving added %d runs", len(runs))
	}
	if n := len(st.Profile().Stats[a.Layout.ID].Chars); n == 0 {
		t.Error("the partial keystrokes were lost")
	}
	_ = st.Close()

	b, _ := openApp(t, dir) // relaunch
	if want := "> pokračuj: "; !strings.Contains(screen(b), want) {
		t.Fatalf("home does not offer to continue:\n%s", screen(b))
	}
	b.Update(enter)
	if ui.RoundLessonID(b) != id {
		t.Errorf("continued lesson %q, want %q", ui.RoundLessonID(b), id)
	}
}

func TestCtrlCMidRoundKeepsResumeAndKeystrokes(t *testing.T) {
	testLeave(t, "\x03", func() {})
}

func TestSIGHUPMidRoundKeepsResumeAndKeystrokes(t *testing.T) {
	testLeave(t, "", func() { _ = syscall.Kill(os.Getpid(), syscall.SIGHUP) })
}

func TestResumeIsWrittenAtTheFirstKeystroke(t *testing.T) {
	a, st, clk := playApp(t)
	id := startLesson(t, a)
	if st.Profile().Resume != nil {
		t.Fatal("Resume set before any key")
	}
	typeKeys(a, clk, 1)
	if r := st.Profile().Resume; r == nil || r.LessonID != id {
		t.Errorf("Resume = %+v, want lesson %s", r, id)
	}
}

func TestAltChordReachesTheSession(t *testing.T) {
	a, _, clk := playApp(t)
	startLesson(t, a)
	clk.advance(time.Second)
	want := ui.RoundKeys(a)[0]
	// A decoder-accurate key: no Text for an Alt chord.
	a.Update(tea.KeyPressMsg{Code: want, Mod: tea.ModAlt})
	if ui.RoundPos(a) != 1 {
		t.Errorf("an expected rune with Alt did not advance: pos %d", ui.RoundPos(a))
	}
	a.Update(tea.KeyPressMsg{Code: '8', Mod: tea.ModAlt})
	if !strings.Contains(screen(a), "Option jako Alt") {
		t.Errorf("no Alt warning:\n%s", screen(a))
	}
}

func TestEnterTypesNewlineInMultiLineText(t *testing.T) {
	a, _, clk := playApp(t)
	var id string
	for _, s := range a.Lib.Snippets {
		if strings.Contains(s.Text, "\n") {
			id = s.ID
			break
		}
	}
	if id == "" {
		t.Fatal("no multi-line snippet")
	}
	ui.SetScreen(a, ui.NewRoundScreen(a, recommend.Pick{Kind: recommend.Code, SnippetID: id}, nil))
	keys := ui.RoundKeys(a)
	if !slices.Contains(keys, '\n') {
		t.Fatal("no newline to type")
	}
	for _, r := range keys {
		clk.advance(100 * time.Millisecond)
		a.Update(key(r))
	}
	if ui.ScreenName(a) != "results" {
		t.Fatalf("Enter did not type the newlines: screen %s pos %d", ui.ScreenName(a), ui.RoundPos(a))
	}
	// A nil next function returns home instead of panicking.
	a.Update(enter)
	if ui.ScreenName(a) != "home" {
		t.Errorf("screen = %s, want home", ui.ScreenName(a))
	}
}

func TestThroughputKeepsThirtySamples(t *testing.T) {
	a, _, clk := playApp(t)
	startLesson(t, a)
	a.Update(tea.FocusMsg{})
	typeKeys(a, clk, 1)
	for range 40 {
		clk.advance(time.Second)
		a.Update(ui.TickMsg(a))
	}
	if n := len(ui.RoundSamples(a)); n != 30 {
		t.Errorf("samples = %d, want 30", n)
	}
}

func TestCtrlRCarriesFocusKnowledge(t *testing.T) {
	a, _, clk := playApp(t)
	startLesson(t, a)
	a.Update(tea.FocusMsg{})
	typeKeys(a, clk, 3)
	a.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	typeKeys(a, clk, 1)
	for range 10 {
		clk.advance(time.Second)
		a.Update(ui.TickMsg(a))
	}
	if strings.Contains(screen(a), "PAUZA") {
		t.Error("the restarted round forgot that focus is reported")
	}
}

func lastLessonOf(t *testing.T, a *ui.App, withIncident bool) (chapter, lesson string) {
	t.Helper()
	for _, ch := range a.Curr.Chapters() {
		if len(ch.Lessons) == 0 {
			continue
		}
		if _, has := a.Curr.IncidentDef(ch.ID); has == withIncident {
			return ch.ID, ch.Lessons[len(ch.Lessons)-1].ID
		}
	}
	t.Fatal("no such chapter")
	return "", ""
}

func syntheticResult(stars int) play.Result {
	return play.Result{
		Stars: stars, Text: engine.Prepare("ab"),
		Metrics: score.Metrics{WPM: 30, Accuracy: 1, Score: 30, Duration: time.Second},
	}
}

func TestFinishCompletesAChapterOnItsLastLessonOnly(t *testing.T) {
	a, st, _ := playApp(t)
	chapter, last := lastLessonOf(t, a, false)
	if _, err := a.Finish(play.Spec{LessonID: last, Kind: play.KindLesson}, syntheticResult(1)); err != nil {
		t.Fatal(err)
	}
	if !st.Profile().Completed[chapter] {
		t.Errorf("chapter %s not completed", chapter)
	}
	incChapter, incLast := lastLessonOf(t, a, true)
	if _, err := a.Finish(play.Spec{LessonID: incLast, Kind: play.KindLesson}, syntheticResult(1)); err != nil {
		t.Fatal(err)
	}
	if st.Profile().Completed[incChapter] {
		t.Error("a chapter with an incident must wait for the incident")
	}
}

func TestFinishKeepsTheBestStars(t *testing.T) {
	a, st, _ := playApp(t)
	id, _ := firstLesson(t, a)
	st.Profile().Stars[id] = 3
	run, _ := a.Finish(play.Spec{LessonID: id, Kind: play.KindLesson}, syntheticResult(1))
	if st.Profile().Stars[id] != 3 || run.Stars != 1 {
		t.Errorf("stars: profile %d run %d", st.Profile().Stars[id], run.Stars)
	}
	a.Finish(play.Spec{LessonID: id, Kind: play.KindLesson}, syntheticResult(2))
	if st.Profile().Stars[id] != 3 {
		t.Error("a worse round lowered the stars")
	}
}

func TestFinishRecordsStats(t *testing.T) {
	a, st, clk := playApp(t)
	playLesson(t, a, clk, 100*time.Millisecond, false)
	if len(st.Profile().Stats[a.Layout.ID].Chars) == 0 {
		t.Error("Finish did not feed the stats")
	}
}

func TestSpecFor(t *testing.T) {
	a, _, clk := playApp(t)
	var late string
	for _, ch := range a.Curr.Chapters() {
		if ch.Number == 12 && len(ch.Lessons) > 0 {
			late = ch.Lessons[0].ID
		}
	}
	if late == "" {
		t.Fatal("no chapter 12 lesson")
	}
	spec, err := a.SpecFor(recommend.Pick{Kind: recommend.Lesson, LessonID: late})
	if err != nil || !spec.HintsOff {
		t.Errorf("chapter 12: err %v hintsOff %v", err, spec.HintsOff)
	}
	id, _ := firstLesson(t, a)
	spec, err = a.SpecFor(recommend.Pick{Kind: recommend.Lesson, LessonID: id})
	if err != nil || spec.HintsOff || spec.Ghost != nil || spec.Text == "" {
		t.Errorf("first lesson: err %v hintsOff %v ghost %v", err, spec.HintsOff, spec.Ghost)
	}
	drill, err := a.SpecFor(recommend.Pick{Kind: recommend.Drill, Focus: []rune("fj"), Seed: 1})
	if err != nil || !drill.GhostIsPace || drill.Ghost == nil || drill.Kind != play.KindDrill {
		t.Errorf("drill: err %v pace %v ghost %v", err, drill.GhostIsPace, drill.Ghost)
	}
	inc, _ := a.SpecFor(recommend.Pick{Kind: recommend.Incident, Seed: 1})
	if inc.Kind != play.KindDrill {
		t.Errorf("incident maps to %v, want a drill for now", inc.Kind)
	}
	playLesson(t, a, clk, 200*time.Millisecond, false)
	spec, _ = a.SpecFor(recommend.Pick{Kind: recommend.Lesson, LessonID: id})
	if spec.Ghost == nil || spec.GhostIsPace {
		t.Errorf("after a run the lesson must carry a replay ghost: %v pace %v", spec.Ghost, spec.GhostIsPace)
	}
}

func TestHomeShowsRankAndUptime(t *testing.T) {
	a, st, clk := playApp(t)
	for i, d := range []int{2, 1} {
		_ = st.AppendRun(store.Run{ID: fmt.Sprint("r", i), At: clk.t.AddDate(0, 0, -d)})
	}
	ui.SetScreen(a, ui.NewHomeScreen(a))
	v := screen(a)
	for _, want := range []string{"hodnost  Intern", "up 2 days"} {
		if !strings.Contains(v, want) {
			t.Errorf("home lacks %q:\n%s", want, v)
		}
	}
}

func TestSmallWindows(t *testing.T) {
	a, _, clk := playApp(t)
	a.Update(tea.WindowSizeMsg{Width: 40, Height: 8})
	for range 8 {
		a.Update(down)
	}
	a.Update(tea.KeyPressMsg{Code: tea.KeyUp}) // the last entry
	if !strings.Contains(screen(a), "> Nastavení") {
		t.Errorf("the selection scrolled out of view:\n%s", screen(a))
	}
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	playLesson(t, a, clk, 200*time.Millisecond, false)
	a.Update(tea.WindowSizeMsg{Width: 40, Height: 8})
	if !strings.Contains(screen(a), "☆") && !strings.Contains(screen(a), "★") {
		t.Errorf("no stars in a small results window:\n%s", screen(a))
	}
}

func TestRegisteredEntryOpens(t *testing.T) {
	a, _, _ := playApp(t)
	opened := false
	a.RegisterEntry(ui.EntryStandup, func(*ui.App) (ui.Screen, tea.Cmd) {
		opened = true
		return ui.NewHomeScreen(a), nil
	})
	a.Update(down)
	a.Update(enter)
	if !opened {
		t.Error("the registered entry was not opened")
	}
}
