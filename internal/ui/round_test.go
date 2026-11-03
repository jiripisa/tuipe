package ui_test

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jiripisa/tuipe/internal/content"
	"github.com/jiripisa/tuipe/internal/engine"
	"github.com/jiripisa/tuipe/internal/play"
	"github.com/jiripisa/tuipe/internal/score"
	"github.com/jiripisa/tuipe/internal/store"
	"github.com/jiripisa/tuipe/internal/ui"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

// newClock is the fixed fake clock every test app starts on.
func newClock() *clock { return &clock{t: time.Date(2026, 10, 31, 12, 0, 0, 0, time.Local)} }

func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

// playApp is an app on a fixed clock and an 80 × 24 window.
func playApp(t *testing.T) (*ui.App, *store.Store, *clock) {
	t.Helper()
	a, st := newApp(t)
	clk := newClock()
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

// startLesson starts the Test of the first lesson.
func startLesson(t *testing.T, a *ui.App) string {
	t.Helper()
	id := a.Course.Lessons()[0].ID
	ui.SetScreen(a, testRound(t, a, id))
	a.Update(ui.TickMsg(a))
	return id
}

// testRound builds the round of a Test, as the lesson screen starts it.
func testRound(t *testing.T, a *ui.App, id string) ui.Screen {
	t.Helper()
	s, err := ui.LessonTestRound(a, id)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// onlyRun returns the single run in the store.
func onlyRun(t *testing.T, st *store.Store) store.Run {
	t.Helper()
	runs, _ := st.Runs()
	if len(runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(runs))
	}
	return runs[0]
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
	st.Profile().FirstRunDone = true // these tests start on the lesson list
	clk := newClock()
	a, err := ui.New(st, lib, ui.WithClock(clk.now))
	if err != nil {
		t.Fatal(err)
	}
	return a, st, clk, filepath.Join(dir, "data")
}

// tall gives the app a window that shows the whole lesson list.
func tall(a *ui.App) { a.Update(tea.WindowSizeMsg{Width: 100, Height: 150}) }

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

// today is the date of the fixed test clock.
const today = "2026-10-31"

var up = tea.KeyPressMsg{Code: tea.KeyUp}

func wantScreen(t *testing.T, a *ui.App, name string) {
	t.Helper()
	if got := ui.ScreenName(a); got != name {
		t.Fatalf("screen = %s, want %s\n%s", got, name, screen(a))
	}
}

func wantText(t *testing.T, a *ui.App, want string) {
	t.Helper()
	if v := screen(a); !strings.Contains(v, want) {
		t.Fatalf("screen lacks %q:\n%s", want, v)
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
	if n := len(st.Profile().Stats[a.Layout.ID].Chars); n == 0 {
		t.Error("the keys before the restart must feed the stats")
	}
}

func TestAbandonedRoundFeedsStatsButNotRuns(t *testing.T) {
	a, st, clk := playApp(t)
	startLesson(t, a)
	typeKeys(a, clk, 5)
	a.Update(esc)
	a.Update(down)
	a.Update(down)
	a.Update(enter)
	wantScreen(t, a, "lesson")
	if runs, _ := st.Runs(); len(runs) != 0 {
		t.Errorf("abandoning added %d runs", len(runs))
	}
	if n := len(st.Profile().Stats[a.Layout.ID].Chars); n == 0 {
		t.Error("keystrokes did not reach the stats")
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

func TestFinishingSavesStars(t *testing.T) {
	a, st, clk := playApp(t)
	id := startLesson(t, a)
	typeKeys(a, clk, -1)
	runs, _ := st.Runs()
	if st.Profile().Stars[id] != runs[0].Stars {
		t.Errorf("stars %d, run %d", st.Profile().Stars[id], runs[0].Stars)
	}
}

func TestSaveFailureStillShowsResults(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod does not make a directory read-only on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	st, err := store.Open(filepath.Join(root, "cfg"), dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	lib, _ := content.Load()
	st.Profile().FirstRunDone = true // these tests start on the lesson list
	clk := newClock()
	a, err := ui.New(st, lib, ui.WithClock(clk.now))
	if err != nil {
		t.Fatal(err)
	}
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	startLesson(t, a)
	if err := os.Chmod(dataDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dataDir, 0o755) })
	typeKeys(a, clk, -1)
	wantScreen(t, a, "part-results")
	v := screen(a)
	if !strings.Contains(v, text.PartSpeed) || !strings.Contains(v, "Uložení se nepovedlo") {
		t.Errorf("results or warning missing:\n%s", v)
	}
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
	a.Update(enter) // the lesson the list selects
	a.Update(down)
	a.Update(down)
	_, cmd := a.Update(enter) // its Test
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
	st.Profile().FirstRunDone = true // these tests start on the lesson list
	a, err := ui.New(st, lib, ui.WithClock(newClock().now))
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
	startLesson(t, a)
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
	wantScreen(t, b, "lessons")
}

func TestCtrlCMidRoundKeepsKeystrokes(t *testing.T) {
	testLeave(t, "\x03", func() {})
}

func TestAltChordReachesTheSession(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "") // an unknown terminal gets the general advice
	t.Setenv("KITTY_WINDOW_ID", "")
	t.Setenv("ALACRITTY_WINDOW_ID", "")
	t.Setenv("TERM", "xterm-256color")
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
	if !strings.Contains(screen(a), "Option je Alt") {
		t.Errorf("no Alt warning:\n%s", screen(a))
	}
}

func TestEnterTypesNewlineInMultiLineText(t *testing.T) {
	a, _, clk := playApp(t)
	var id string
	for _, l := range a.Course.Lessons() {
		if l.Lang != "" {
			id = l.ID
		}
	}
	ui.SetScreen(a, testRound(t, a, id))
	keys := ui.RoundKeys(a)
	if !slices.Contains(keys, '\n') {
		t.Fatal("no newline to type")
	}
	for _, r := range keys {
		clk.advance(100 * time.Millisecond)
		a.Update(key(r))
	}
	if ui.ScreenName(a) != "part-results" {
		t.Fatalf("Enter did not type the newlines: screen %s pos %d", ui.ScreenName(a), ui.RoundPos(a))
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

func syntheticResult(stars int) play.Result {
	return play.Result{
		Stars: stars, Text: engine.Prepare("ab"),
		Metrics: score.Metrics{WPM: 30, Accuracy: 1, Score: 30, Duration: time.Second},
	}
}

func TestFinishKeepsTheBestStars(t *testing.T) {
	a, st, _ := playApp(t)
	id := a.Course.Lessons()[0].ID
	st.Profile().Stars[id] = 3
	run, _ := a.Finish(play.Spec{LessonID: id, Kind: play.KindTest}, syntheticResult(1))
	if st.Profile().Stars[id] != 3 || run.Stars != 1 {
		t.Errorf("stars: profile %d run %d", st.Profile().Stars[id], run.Stars)
	}
	a.Finish(play.Spec{LessonID: id, Kind: play.KindTest}, syntheticResult(2))
	if st.Profile().Stars[id] != 3 {
		t.Error("a worse round lowered the stars")
	}
}

func TestFinishRecordsStats(t *testing.T) {
	a, st, clk := playApp(t)
	startLesson(t, a)
	typeKeys(a, clk, -1)
	if len(st.Profile().Stats[a.Layout.ID].Chars) == 0 {
		t.Error("Finish did not feed the stats")
	}
}

func TestSmallWindows(t *testing.T) {
	a, _, clk := playApp(t)
	a.Update(tea.WindowSizeMsg{Width: 40, Height: 8})
	a.Update(up) // wraps to the last lesson
	lessons := a.Course.Lessons()
	if !strings.Contains(screen(a), "> ") || !strings.Contains(screen(a), lessons[len(lessons)-1].Title[:10]) {
		t.Errorf("the selection scrolled out of view:\n%s", screen(a))
	}
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	startLesson(t, a)
	typeKeys(a, clk, -1)
	a.Update(tea.WindowSizeMsg{Width: 40, Height: 8})
	if !strings.Contains(screen(a), "☆") && !strings.Contains(screen(a), "★") {
		t.Errorf("no stars in a small results window:\n%s", screen(a))
	}
}

func TestCodeLessonInEditorShowsItsFileNotProse(t *testing.T) {
	a, _, _ := playApp(t)
	ui.SetScreen(a, testRound(t, a, "ch11-01"))
	a.Update(ui.TickMsg(a))
	if v := screen(a); strings.Contains(v, text.RoundProseFile) || !strings.Contains(v, "main.go") {
		t.Errorf("the Go lesson does not read as Go:\n%s", v)
	}
}

func TestAltWarningNamesTheSettingOfThisTerminal(t *testing.T) {
	t.Setenv("KITTY_WINDOW_ID", "")
	for term, want := range map[string]string{
		"ghostty":        "macos-option-as-alt = false",
		"Apple_Terminal": "vypněte: Use Option as Meta key",
		"iTerm.app":      "Left Option key = Normal",
		"":               text.RoundAltWarning,
	} {
		t.Setenv("TERM_PROGRAM", term)
		t.Setenv("TERM", "xterm-256color")
		t.Setenv("ALACRITTY_WINDOW_ID", "")
		a, _, _ := playApp(t)
		startLesson(t, a)
		a.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModAlt}) // Option arrives as Alt
		if v := screen(a); !strings.Contains(v, want) {
			t.Errorf("TERM_PROGRAM=%q: warning lacks %q:\n%s", term, want, v)
		}
	}
}

func TestRoundsWithoutALessonShowNoStars(t *testing.T) {
	a, _, _ := playApp(t)
	ui.SetScreen(a, ui.NewPlacementScreen(a, nil))
	a.Update(enter)
	wantScreen(t, a, "round")
	a.Update(ui.TickMsg(a))
	if v := screen(a); strings.ContainsAny(v, "★☆") {
		t.Errorf("a placement round shows stars:\n%s", v)
	}
}

func TestLessonResultsKeepTheirStars(t *testing.T) {
	a, _, clk := playApp(t)
	startLesson(t, a)
	typeKeys(a, clk, -1)
	if v := screen(a); !strings.Contains(v, "★") {
		t.Errorf("a lesson shows its stars:\n%s", v)
	}
}

func TestRoundShowsNoSecondCursor(t *testing.T) {
	a, _, clk := playApp(t)
	startLesson(t, a)
	typeKeys(a, clk, -1)
	a.Update(key('R'))
	wantScreen(t, a, "round")
	for i := range 3 {
		a.Update(ui.TickMsg(a))
		if v := screen(a); strings.Contains(v, "HEAD~1") {
			t.Errorf("style %d: a repeated round shows a second cursor:\n%s", i, v)
		}
		a.Update(tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	}
}
