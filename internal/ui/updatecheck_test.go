package ui_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jiripisa/tuipe/internal/store"
	"github.com/jiripisa/tuipe/internal/ui"
	"github.com/jiripisa/tuipe/internal/ui/text"
	"github.com/jiripisa/tuipe/internal/upgrade"
)

// releaseServer answers the latest-release query with the given status and tag
// and counts the requests it gets.
func releaseServer(t *testing.T, status int, tag string) (*upgrade.Client, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(status)
		if status == http.StatusOK {
			_, _ = w.Write([]byte(`{"tag_name":"` + tag + `","assets":[]}`))
		}
	}))
	t.Cleanup(srv.Close)
	return &upgrade.Client{APIBase: srv.URL, HTTP: srv.Client(), Repo: "x/y"}, &hits
}

// updateApp is an app on the lesson list with the check wired to client, running as
// release current.
func updateApp(t *testing.T, client *upgrade.Client, release bool, current string) (*ui.App, *store.Store) {
	t.Helper()
	a, st := newApp(t)
	b, err := ui.New(st, a.Lib, ui.WithClock(newClock().now), ui.WithUpdateCheck(client, release, current))
	if err != nil {
		t.Fatal(err)
	}
	b.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return b, st
}

// drive runs a command the way the program would and feeds its messages back.
func drive(a *ui.App, cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	switch m := cmd().(type) {
	case nil:
	case tea.BatchMsg:
		for _, c := range m {
			drive(a, c)
		}
	default:
		_, next := a.Update(m)
		drive(a, next)
	}
}

func TestNoticeShownOnTheListWhenNewer(t *testing.T) {
	client, _ := releaseServer(t, http.StatusOK, "v1.2.0")
	a, st := updateApp(t, client, true, "v1.0.0")
	drive(a, a.Init())
	v := screen(a)
	if !strings.Contains(v, "v1.2.0") || !strings.Contains(v, "tuipe upgrade") {
		t.Errorf("the list lacks the notice:\n%s", v)
	}
	if got := st.Profile().LatestSeen; got != "v1.2.0" {
		t.Errorf("LatestSeen = %q", got)
	}
}

func TestNoNoticeWhenNotNewer(t *testing.T) {
	client, _ := releaseServer(t, http.StatusOK, "v1.2.0")
	a, _ := updateApp(t, client, true, "v1.2.0")
	drive(a, a.Init())
	if a.UpdateNotice != "" {
		t.Errorf("notice for the running version: %q", a.UpdateNotice)
	}
}

func TestNoticeKeptOnLaterLaunchSameDayUntilUpgraded(t *testing.T) {
	client, hits := releaseServer(t, http.StatusOK, "v1.2.0")
	a, st := updateApp(t, client, true, "v1.0.0")
	drive(a, a.Init())
	hits.Store(0)

	again, err := ui.New(st, a.Lib, ui.WithClock(newClock().now), ui.WithUpdateCheck(client, true, "v1.0.0"))
	if err != nil {
		t.Fatal(err)
	}
	drive(again, again.Init())
	if !strings.Contains(again.UpdateNotice, "v1.2.0") || hits.Load() != 0 {
		t.Errorf("same-day launch: notice %q, requests %d", again.UpdateNotice, hits.Load())
	}

	upgraded, err := ui.New(st, a.Lib, ui.WithClock(newClock().now), ui.WithUpdateCheck(client, true, "v1.2.0"))
	if err != nil {
		t.Fatal(err)
	}
	drive(upgraded, upgraded.Init())
	if upgraded.UpdateNotice != "" {
		t.Errorf("notice after upgrading: %q", upgraded.UpdateNotice)
	}
}

func TestCheckedAtMostOnceADay(t *testing.T) {
	client, hits := releaseServer(t, http.StatusOK, "v1.0.0")
	clk := newClock()
	a, st := updateApp(t, client, true, "v1.0.0")
	a.Now = clk.now
	drive(a, a.Init())
	drive(a, a.Init())
	if hits.Load() != 1 {
		t.Errorf("requests the same day = %d, want 1", hits.Load())
	}
	if st.Profile().LastUpdateCheck != today {
		t.Errorf("LastUpdateCheck = %q, want %q", st.Profile().LastUpdateCheck, today)
	}
	clk.advance(24 * time.Hour)
	drive(a, a.Init())
	if hits.Load() != 2 {
		t.Errorf("requests the next day = %d, want 2", hits.Load())
	}
}

func TestDisabledMeansNoRequest(t *testing.T) {
	client, hits := releaseServer(t, http.StatusOK, "v9.0.0")
	a, st := updateApp(t, client, true, "v1.0.0")
	cfg := st.Config()
	cfg.UpdateCheck = false
	if err := st.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if cmd := a.Init(); cmd != nil {
		drive(a, cmd)
	}
	if hits.Load() != 0 || a.UpdateNotice != "" {
		t.Errorf("disabled: %d requests, notice %q", hits.Load(), a.UpdateNotice)
	}
}

func TestDevBuildMakesNoRequest(t *testing.T) {
	client, hits := releaseServer(t, http.StatusOK, "v9.0.0")
	a, _ := updateApp(t, client, false, "dev")
	drive(a, a.Init())
	if hits.Load() != 0 || a.UpdateNotice != "" {
		t.Errorf("dev build: %d requests, notice %q", hits.Load(), a.UpdateNotice)
	}
}

func TestOfflineOr403StaysSilent(t *testing.T) {
	forbidden, _ := releaseServer(t, http.StatusForbidden, "")
	dead := httptest.NewServer(http.NotFoundHandler())
	offline := &upgrade.Client{APIBase: dead.URL, HTTP: dead.Client(), Repo: "x/y"}
	dead.Close()
	for name, client := range map[string]*upgrade.Client{"403": forbidden, "offline": offline} {
		t.Run(name, func(t *testing.T) {
			a, st := updateApp(t, client, true, "v1.0.0")
			before := screen(a)
			drive(a, a.Init())
			if a.UpdateNotice != "" || screen(a) != before {
				t.Errorf("the list changed:\n%s", screen(a))
			}
			if st.Profile().LastUpdateCheck != today {
				t.Errorf("a failed check must still count as today's: %q", st.Profile().LastUpdateCheck)
			}
			if st.Profile().LatestSeen != "" {
				t.Errorf("LatestSeen = %q", st.Profile().LatestSeen)
			}
		})
	}
}

func TestDoesNotDelayStartup(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() { close(release); srv.Close() })
	client := &upgrade.Client{APIBase: srv.URL, HTTP: srv.Client(), Repo: "x/y"}
	a, _ := updateApp(t, client, true, "v1.0.0")

	start := time.Now()
	cmd := a.Init()
	if cmd == nil {
		t.Fatal("no check command")
	}
	if d := time.Since(start); d > 200*time.Millisecond {
		t.Errorf("Init blocked for %v", d)
	}
	if !strings.Contains(screen(a), text.HintLessons) {
		t.Error("the list is not ready")
	}
	start = time.Now()
	drive(a, cmd)
	if d := time.Since(start); d < time.Second || d > 4*time.Second {
		t.Errorf("hanging server took %v, want about 2 s", d)
	}
	if a.UpdateNotice != "" {
		t.Errorf("notice after timeout: %q", a.UpdateNotice)
	}
}

func TestNoticeArrivesDuringARoundWithoutDisturbingIt(t *testing.T) {
	client, _ := releaseServer(t, http.StatusOK, "v1.2.0")
	a, _ := updateApp(t, client, true, "v1.0.0")
	a.Update(enter) // the selected lesson
	a.Update(down)
	a.Update(down)
	a.Update(enter) // its Test
	if ui.ScreenName(a) != "round" {
		t.Fatalf("screen = %s", ui.ScreenName(a))
	}
	before := screen(a)
	a.Update(ui.UpdateAvailableMsg{Version: "v1.2.0"})
	if ui.ScreenName(a) != "round" || screen(a) != before {
		t.Error("the round was disturbed")
	}
	if !strings.Contains(a.UpdateNotice, "v1.2.0") {
		t.Errorf("notice = %q", a.UpdateNotice)
	}
}

func TestStaleNoticeIsCleared(t *testing.T) {
	client, _ := releaseServer(t, http.StatusOK, "v1.0.0")
	a, _ := updateApp(t, client, true, "1.0.0")
	a.Update(ui.UpdateAvailableMsg{Version: "v1.0.0"})
	if a.UpdateNotice != "" {
		t.Fatalf("a version that is not newer set the notice %q", a.UpdateNotice)
	}
	a.UpdateNotice = "stale"
	a.Update(ui.UpdateAvailableMsg{Version: "v1.0.0"})
	if a.UpdateNotice != "" {
		t.Errorf("a stale notice stayed: %q", a.UpdateNotice)
	}
}

func TestLaterCheckClearsANoticeThatNoLongerHolds(t *testing.T) {
	client, _ := releaseServer(t, http.StatusOK, "v1.0.0") // the newer release was withdrawn
	a, st := updateApp(t, client, true, "1.0.0")
	st.Profile().LastUpdateCheck = ""
	a.UpdateNotice = "stale"
	drive(a, a.Init())
	if a.UpdateNotice != "" {
		t.Errorf("the notice stayed after a check found nothing newer: %q", a.UpdateNotice)
	}
}
