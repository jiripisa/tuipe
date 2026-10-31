package ui_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/jiripisa/tuipe/internal/content"
	"github.com/jiripisa/tuipe/internal/store"
	"github.com/jiripisa/tuipe/internal/ui"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

func newApp(t *testing.T) (*ui.App, *store.Store) {
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
	a, err := ui.New(st, lib)
	if err != nil {
		t.Fatal(err)
	}
	return a, st
}

func TestNewDefaults(t *testing.T) {
	a, _ := newApp(t)
	if a.Style.ID != "editor" || a.Layout.ID != "macos-czech-qwerty" || a.Curr == nil || a.Now == nil {
		t.Errorf("unexpected defaults: style %q layout %q", a.Style.ID, a.Layout.ID)
	}
	if strings.Contains(a.Now().String(), "m=") {
		t.Error("Now must not carry a monotonic reading")
	}
}

func TestCtrlTCyclesStyleAndPersists(t *testing.T) {
	a, st := newApp(t)
	tm := teatest.NewTestModel(t, a, teatest.WithInitialTermSize(80, 24))
	tm.Send(tea.KeyPressMsg{Code: 't', Mod: tea.ModCtrl})
	tm.Send(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))
	if got := st.Config().Style; got != "velin" {
		t.Errorf("config style = %q, want velin", got)
	}
	if a.Style.ID != "velin" {
		t.Errorf("app style = %q, want velin", a.Style.ID)
	}
}

func TestCtrlCQuits(t *testing.T) {
	a, _ := newApp(t)
	tm := teatest.NewTestModel(t, a, teatest.WithInitialTermSize(80, 24))
	tm.Send(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))
}

func TestViewIsAltScreenWithFocusReporting(t *testing.T) {
	a, _ := newApp(t)
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	v := a.View()
	if !v.AltScreen || !v.ReportFocus || v.BackgroundColor == nil {
		t.Errorf("view: alt %v focus %v bg %v", v.AltScreen, v.ReportFocus, v.BackgroundColor)
	}
	lines := strings.Split(v.Content, "\n")
	if len(lines) != 24 {
		t.Errorf("lines = %d, want 24", len(lines))
	}
	for _, l := range lines {
		if w := ansi.StringWidth(l); w != 80 {
			t.Fatalf("line width %d, want 80", w)
		}
	}
	if !strings.Contains(ansi.Strip(v.Content), text.AppName) {
		t.Error("start screen does not show the app name")
	}
}

func TestWindowTooSmallPauses(t *testing.T) {
	a, _ := newApp(t)
	a.Update(tea.WindowSizeMsg{Width: 39, Height: 7})
	plain := ansi.Strip(a.View().Content)
	if !strings.Contains(plain, "víc místa") {
		t.Errorf("no 'more room' message:\n%s", plain)
	}
	// Keys are not delivered to screens; Ctrl+T is still a global key.
	before := a.Style.ID
	a.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if a.Style.ID != before {
		t.Error("style changed by a plain key")
	}
	a.Update(tea.WindowSizeMsg{Width: 40, Height: 8})
	if strings.Contains(ansi.Strip(a.View().Content), "víc místa") {
		t.Error("40 x 8 must not show the message")
	}
}

func TestSaveErrShowsWarning(t *testing.T) {
	a, _ := newApp(t)
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	a.SaveErr(os.ErrPermission)
	if !strings.Contains(ansi.Strip(a.View().Content), "Uložení se nepovedlo") {
		t.Error("save warning not shown")
	}
	a.SaveErr(nil)
	if strings.Contains(ansi.Strip(a.View().Content), "Uložení se nepovedlo") {
		t.Error("warning not cleared")
	}
}

func useTempDirs(t *testing.T) (cfg, data string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "cfg"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	cfg, data, err := store.Paths()
	if err != nil {
		t.Fatal(err)
	}
	return cfg, data
}

func TestSecondInstanceMessage(t *testing.T) {
	cfg, data := useTempDirs(t)
	st, err := store.Open(cfg, data)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	err = ui.Run()
	if err == nil || !strings.Contains(err.Error(), "už běží") {
		t.Errorf("Run error = %v, want the Czech second-instance message", err)
	}
}

func TestNewerDataMessage(t *testing.T) {
	_, data := useTempDirs(t)
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, "profile.json"), []byte(`{"Schema": 999}`), 0o644); err != nil {
		t.Fatal(err)
	}
	err := ui.Run()
	if err == nil || !strings.Contains(err.Error(), "tuipe upgrade") {
		t.Errorf("Run error = %v, want the upgrade message", err)
	}
}
