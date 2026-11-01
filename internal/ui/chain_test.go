package ui_test

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jiripisa/tuipe/internal/incident"
	"github.com/jiripisa/tuipe/internal/play"
	"github.com/jiripisa/tuipe/internal/recommend"
	"github.com/jiripisa/tuipe/internal/stats"
	"github.com/jiripisa/tuipe/internal/store"
	"github.com/jiripisa/tuipe/internal/ui"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

const (
	homeStandup = 1 // downs from the quick run to the Standup entry
	homeSprint  = 2
	today       = "2026-10-31"
)

var up = tea.KeyPressMsg{Code: tea.KeyUp}

func windowSize(w, h int) tea.WindowSizeMsg { return tea.WindowSizeMsg{Width: w, Height: h} }

// openEntry moves down the home menu and presses Enter.
func openEntry(a *ui.App, downs int) {
	for range downs {
		a.Update(down)
	}
	a.Update(enter)
}

// typeFrom types the keys of the round from the n-th on, 200 ms apart.
func typeFrom(a *ui.App, clk *clock, n int) {
	for _, r := range ui.RoundKeys(a)[n:] {
		clk.advance(200 * time.Millisecond)
		a.Update(key(r))
	}
}

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

func sprintClock(ms int64) string {
	s := int(ms / 1000)
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

func TestStandupRunsFiveRoundsAndMarksTheDay(t *testing.T) {
	a, st, clk := playApp(t)
	openEntry(a, homeStandup)
	for i := 1; i <= 5; i++ {
		wantScreen(t, a, "round")
		label := fmt.Sprintf(text.ChainStandup, i, 5)
		wantText(t, a, label)
		if k := ui.RoundSpec(a).Kind; k != play.KindDrill {
			t.Fatalf("round %d is kind %v, want a drill", i, k)
		}
		typeKeys(a, clk, -1)
		wantScreen(t, a, "results")
		wantText(t, a, label) // the results show the progress too
		if got := st.Profile().StandupDay; (i < 5) != (got == "") {
			t.Fatalf("after round %d StandupDay = %q", i, got)
		}
		clk.advance(time.Second)
		a.Update(enter)
	}
	wantScreen(t, a, "home")
	if got := st.Profile().StandupDay; got != today {
		t.Errorf("StandupDay = %q, want %q", got, today)
	}
	wantText(t, a, text.MenuStandup+" ("+text.HomeDone+")")
	if runs, _ := st.Runs(); len(runs) != 5 {
		t.Errorf("runs = %d, want 5", len(runs))
	}

	// A standup is not locked: it can be played again.
	openEntry(a, homeStandup)
	wantScreen(t, a, "round")
	wantText(t, a, fmt.Sprintf(text.ChainStandup, 1, 5))
}

func TestPartialStandupDoesNotMarkTheDay(t *testing.T) {
	a, st, clk := playApp(t)
	openEntry(a, homeStandup)
	for range 2 {
		typeKeys(a, clk, -1)
		clk.advance(time.Second)
		a.Update(enter)
	}
	typeKeys(a, clk, 4)
	a.Update(esc)
	a.Update(down)
	a.Update(down)
	a.Update(enter) // leave from the pause menu
	wantScreen(t, a, "home")
	if got := st.Profile().StandupDay; got != "" {
		t.Errorf("StandupDay = %q after a partial standup", got)
	}
	if strings.Contains(screen(a), text.HomeDone) {
		t.Errorf("home shows a partial standup as done:\n%s", screen(a))
	}
}

func TestSprintMinutesCountActiveTimeOnly(t *testing.T) {
	a, st, clk := playApp(t)
	openEntry(a, homeSprint)
	wantScreen(t, a, "sprint-minutes")
	a.Update(up) // 20 -> 10 minutes
	a.Update(enter)
	wantScreen(t, a, "round")
	wantText(t, a, "Sprint 0:00 / 10:00")

	typeKeys(a, clk, 5)
	a.Update(esc) // a long pause: coffee
	clk.advance(30 * time.Minute)
	a.Update(esc)
	typeFrom(a, clk, 5)
	wantScreen(t, a, "results")

	runs, _ := st.Runs()
	if len(runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(runs))
	}
	r := st.Profile().Resume
	if r == nil || r.Sprint == nil {
		t.Fatalf("no sprint saved after the round: %+v", r)
	}
	if got := r.Sprint.ActiveMs; got != runs[0].DurationMs || got <= 0 || got >= 2*60*1000 {
		t.Errorf("ActiveMs = %d, run duration %d ms: the 30 minute pause must not count", got, runs[0].DurationMs)
	}
	wantText(t, a, fmt.Sprintf("Sprint %s / 10:00", sprintClock(r.Sprint.ActiveMs)))

	// The pause did not use up the budget: the next planned round is a lesson,
	// not the finale.
	clk.advance(time.Second)
	a.Update(enter)
	wantScreen(t, a, "round")
	if k := ui.RoundSpec(a).Kind; k != play.KindLesson {
		t.Errorf("second round kind = %v, want the planned lesson", k)
	}
}

func TestLeavingSprintSavesPlanAndResumesSameDay(t *testing.T) {
	a, st, clk := playApp(t)
	openEntry(a, homeSprint)
	a.Update(enter) // the default is 20 minutes
	typeKeys(a, clk, -1)
	clk.advance(time.Second)
	a.Update(enter)
	wantScreen(t, a, "round")
	before := st.Profile().Resume
	if before == nil || before.Sprint == nil || before.Kind != "sprint" {
		t.Fatalf("resume after round 1 = %+v", before)
	}
	if before.Sprint.NextIndex != 1 || before.Sprint.Minutes != 20 || before.Day != today || before.Sprint.Seed == 0 {
		t.Errorf("sprint state after round 1 = %+v day %q", *before.Sprint, before.Day)
	}
	active1 := before.Sprint.ActiveMs

	typeKeys(a, clk, 3) // the first keystroke of round 2 has been saved too
	if st.Profile().Resume.Sprint.NextIndex != 1 {
		t.Errorf("NextIndex mid-round = %d, want 1", st.Profile().Resume.Sprint.NextIndex)
	}
	planned := ui.RoundSpec(a)
	a.Update(esc)
	a.Update(down)
	a.Update(down)
	a.Update(enter)
	wantScreen(t, a, "home")
	r := st.Profile().Resume
	if r == nil || r.Sprint == nil || r.Sprint.NextIndex != 1 {
		t.Fatalf("leaving lost the sprint: %+v", r)
	}
	if r.Sprint.ActiveMs <= active1 {
		t.Errorf("ActiveMs %d did not grow by the abandoned part (was %d)", r.Sprint.ActiveMs, active1)
	}
	wantText(t, a, text.MenuSprint+" ("+text.HomeContinue+")")

	// The same day, Sprint continues with the round that was left.
	openEntry(a, homeSprint)
	wantScreen(t, a, "round")
	got := ui.RoundSpec(a)
	if got.Kind != planned.Kind || got.Title != planned.Title || got.Text != planned.Text {
		t.Errorf("resumed round = %v %q %q, want %v %q %q", got.Kind, got.Title, got.Text, planned.Kind, planned.Title, planned.Text)
	}
	wantText(t, a, "Sprint "+sprintClock(r.Sprint.ActiveMs)+" / 20:00")
}

func TestSprintPlanIsDeterministicFromTheSavedState(t *testing.T) {
	a, st, clk := playApp(t)
	openEntry(a, homeSprint)
	a.Update(enter)
	typeKeys(a, clk, 2)
	saved := *st.Profile().Resume.Sprint
	plan := ui.SprintPlan(a, &saved)
	if len(plan) < 5 {
		t.Fatalf("plan has %d rounds", len(plan))
	}

	// The profile moves on while the sprint is played.
	typeFrom(a, clk, 2)
	for _, ch := range a.Curr.Chapters() {
		for _, l := range ch.Lessons {
			st.Profile().Stars[l.ID] = 3
		}
	}
	st.Profile().Stats[a.Layout.ID] = stats.New()
	if again := ui.SprintPlan(a, &saved); !slices.EqualFunc(plan, again, func(x, y recommend.Pick) bool { return fmt.Sprint(x) == fmt.Sprint(y) }) {
		t.Errorf("the plan changed with the profile:\n%v\n%v", plan, again)
	}

	other := saved
	other.Seed++
	if diff := ui.SprintPlan(a, &other); fmt.Sprint(diff) == fmt.Sprint(plan) {
		t.Error("another seed gave the same plan")
	}
}

func TestYesterdaysSprintEndsWithRetroOnCompletedPart(t *testing.T) {
	a, st, clk := playApp(t)
	openEntry(a, homeSprint)
	a.Update(enter)
	typeKeys(a, clk, -1)
	runs, _ := st.Runs()
	if len(runs) != 1 {
		t.Fatalf("runs = %d", len(runs))
	}
	a.Update(esc) // results -> home, the sprint is left
	wantScreen(t, a, "home")

	// The next day the app opens on the Retro of the one round that was played.
	clk.advance(26 * time.Hour)
	b, err := ui.New(st, a.Lib, ui.WithClock(clk.now))
	if err != nil {
		t.Fatal(err)
	}
	b.Update(windowSize(80, 24))
	wantScreen(t, b, "retro")
	wantText(t, b, text.Rounds(1)+" v tomto sprintu")
	wantText(t, b, fmt.Sprintf("%.1f", runs[0].WPM))
	b.Update(enter)
	wantScreen(t, b, "home")
	if st.Profile().Resume != nil {
		t.Errorf("Resume = %+v after the Retro, want nil", st.Profile().Resume)
	}
	if strings.Contains(screen(b), text.HomeContinue) {
		t.Errorf("home still offers the old sprint:\n%s", screen(b))
	}
}

// almostDone saves a 10 minute sprint of drills that ends with the owed boss
// incident, with the given active time and next index.
func almostDone(t *testing.T, a *ui.App, st *store.Store, clk *clock, activeMs int64, next int) {
	t.Helper()
	boss, _ := bossChapter(t, a)
	readyForBoss(a, st, boss)
	p := st.Profile()
	p.Resume = &store.Resume{Kind: "sprint", Day: today, Sprint: &store.SprintState{
		Minutes: 10, ActiveMs: activeMs, NextIndex: next, Seed: 7,
		Started: clk.now().Format(time.RFC3339Nano),
		Chars:   map[string]stats.Entry{}, Stars: maps.Clone(p.Stars), Completed: maps.Clone(p.Completed), IncidentBest: maps.Clone(p.IncidentBest),
	}}
	if plan := ui.SprintPlan(a, p.Resume.Sprint); len(plan) != 8 {
		t.Fatalf("plan has %d rounds, want 8", len(plan))
	}
}

func TestSprintEndsWithIncidentThenRetro(t *testing.T) {
	a, st, clk := playApp(t)
	almostDone(t, a, st, clk, 9*60*1000+55*1000, 1)
	openEntry(a, homeSprint)
	wantScreen(t, a, "round")
	typeKeys(a, clk, -1) // 20+ s of typing: the 10 minute budget is gone
	wantScreen(t, a, "results")
	wantText(t, a, "/ 10:00")

	// Six planned rounds are left, but only the incident finale is run.
	clk.advance(time.Second)
	a.Update(enter)
	wantScreen(t, a, "round")
	_, boss := bossChapter(t, a)
	if spec := ui.RoundSpec(a); spec.Kind != play.KindIncident || spec.Title != boss.Title {
		t.Fatalf("finale = %v %q, want the incident %q", spec.Kind, spec.Title, boss.Title)
	}
	for _, r := range ui.RoundKeys(a) {
		clk.advance(100 * time.Millisecond)
		a.Update(key(r))
	}
	wantScreen(t, a, "incident-results")
	clk.advance(time.Second)
	a.Update(enter)
	wantScreen(t, a, "retro")
	if runs, _ := st.Runs(); len(runs) != 2 {
		t.Errorf("runs = %d, want the round and the incident", len(runs))
	}
	wantText(t, a, text.Rounds(2)+" v tomto sprintu")
	if st.Profile().Resume != nil {
		t.Errorf("Resume = %+v after a finished sprint", st.Profile().Resume)
	}
	a.Update(enter)
	wantScreen(t, a, "home")
}

func TestLostIncidentCountsRealActiveTime(t *testing.T) {
	a, st, clk := playApp(t)
	const budget = 10 * 60 * 1000
	almostDone(t, a, st, clk, budget, 7)
	openEntry(a, homeSprint)
	wantScreen(t, a, "round")
	if k := ui.RoundSpec(a).Kind; k != play.KindIncident {
		t.Fatalf("round kind = %v, want the finale incident", k)
	}
	typeKeys(a, clk, 3)
	breachByErrors(a)
	wantScreen(t, a, "incident-results")
	runs, _ := st.Runs()
	if len(runs) != 1 || runs[0].Incident == nil || runs[0].Incident.Won {
		t.Fatalf("runs = %+v, want one lost incident", runs)
	}
	got := st.Profile().Resume.Sprint.ActiveMs - budget
	if got <= 0 || got != runs[0].DurationMs {
		t.Errorf("ActiveMs grew by %d, run duration %d ms: a lost incident counts its real active time", got, runs[0].DurationMs)
	}
}

func retroRuns(wpm, acc float64, at time.Time, n int) []store.Run {
	var out []store.Run
	for i := range n {
		out = append(out, store.Run{ID: fmt.Sprintf("r%d%d", at.Unix()%1000, i), At: at.Add(time.Duration(i) * time.Minute), WPM: wpm, Accuracy: acc})
	}
	return out
}

func lineWith(v, sub string) string {
	for _, l := range strings.Split(v, "\n") {
		if strings.Contains(l, sub) {
			return l
		}
	}
	return ""
}

func TestRetroListsImprovedAndWorsenedChars(t *testing.T) {
	a, st, clk := playApp(t)
	p := st.Profile()
	p.Resume = &store.Resume{Kind: "sprint", Day: "2026-10-30", Sprint: &store.SprintState{
		Minutes: 20, Started: clk.now().Add(-time.Hour).Format(time.RFC3339Nano),
		Chars: map[string]stats.Entry{
			"q": {ErrorRate: 0.30, LatencyMs: 500, Samples: 20},
			"x": {ErrorRate: 0.02, LatencyMs: 300, Samples: 20},
			"m": {ErrorRate: 0.10, LatencyMs: 300, Samples: 20}, // not typed in the session
			"w": {ErrorRate: 0.10, LatencyMs: 300, Samples: 20}, // barely moved
		},
	}}
	now := stats.New()
	now.Chars["q"] = &stats.Entry{ErrorRate: 0.05, LatencyMs: 400, Samples: 40}
	now.Chars["x"] = &stats.Entry{ErrorRate: 0.20, LatencyMs: 350, Samples: 40}
	now.Chars["m"] = &stats.Entry{ErrorRate: 0.10, LatencyMs: 300, Samples: 20}
	now.Chars["w"] = &stats.Entry{ErrorRate: 0.11, LatencyMs: 300, Samples: 40}
	now.Chars["y"] = &stats.Entry{ErrorRate: 0.50, LatencyMs: 300, Samples: 10} // no baseline: not compared
	p.Stats[a.Layout.ID] = now

	ui.SetScreen(a, ui.NewRetroScreen(a, retroRuns(40, 0.95, clk.now().Add(-30*time.Minute), 3)))
	wantScreen(t, a, "retro")
	v := screen(a)
	better, worse := lineWith(v, "zlepšeno"), lineWith(v, "zhoršeno")
	if !strings.Contains(better, "q") || strings.ContainsAny(better, "xmwy") {
		t.Errorf("improved line = %q, want only q\n%s", better, v)
	}
	if !strings.Contains(worse, "x") || strings.ContainsAny(worse, "qmwy") {
		t.Errorf("worsened line = %q, want only x\n%s", worse, v)
	}
	if !strings.Contains(better, "-25") || !strings.Contains(worse, "+18") {
		t.Errorf("shifts not shown in percentage points: %q / %q", better, worse)
	}
}

func TestRetroComparesWithThePreviousSevenDays(t *testing.T) {
	a, st, clk := playApp(t)
	start := clk.now().Add(-time.Hour)
	for _, r := range append(retroRuns(30, 0.90, start.Add(-3*24*time.Hour), 2), retroRuns(100, 0.50, start.Add(-10*24*time.Hour), 2)...) {
		if err := st.AppendRun(r); err != nil {
			t.Fatal(err)
		}
	}
	session := retroRuns(40, 0.95, start.Add(time.Minute), 2)
	for _, r := range session {
		if err := st.AppendRun(r); err != nil {
			t.Fatal(err)
		}
	}
	ui.SetScreen(a, ui.NewRetroScreen(a, session))
	v := screen(a)
	for _, want := range []string{"40.0", "(+10.0)", "95.0", "(+5.0 p. b.)"} {
		if !strings.Contains(v, want) {
			t.Errorf("retro lacks %q (the 10 day old runs must not count):\n%s", want, v)
		}
	}
	if !strings.Contains(v, text.RetroNoSnapshot) {
		t.Errorf("without a start snapshot the retro must say so:\n%s", v)
	}
}

func TestRetroWithoutEarlierDataSaysSoAndInventsNothing(t *testing.T) {
	a, st, clk := playApp(t)
	session := retroRuns(40, 0.95, clk.now().Add(-time.Hour), 2)
	for _, r := range session {
		if err := st.AppendRun(r); err != nil {
			t.Fatal(err)
		}
	}
	ui.SetScreen(a, ui.NewRetroScreen(a, session))
	v := screen(a)
	if !strings.Contains(v, text.RetroNoPrev) {
		t.Errorf("no sentence about the missing history:\n%s", v)
	}
	if strings.Contains(v, "(+") || strings.Contains(v, "(-") || strings.Contains(v, "zlepšeno") || strings.Contains(v, "zhoršeno") {
		t.Errorf("the retro invents a comparison:\n%s", v)
	}
	// A sprint without a single finished round has nothing to judge either.
	ui.SetScreen(a, ui.NewRetroScreen(a, nil))
	wantText(t, a, text.RetroNoRuns)
}

func TestRetroFitsSixtyByEighteen(t *testing.T) {
	a, st, clk := playApp(t)
	a.Update(windowSize(60, 18))
	p := st.Profile()
	chars := map[string]stats.Entry{}
	now := stats.New()
	for i, c := range "abcdefgh" {
		chars[string(c)] = stats.Entry{ErrorRate: 0.30, Samples: 20}
		rate := 0.01
		if i%2 == 0 {
			rate = 0.60
		}
		now.Chars[string(c)] = &stats.Entry{ErrorRate: rate, Samples: 40}
	}
	p.Resume = &store.Resume{Kind: "sprint", Day: today, Sprint: &store.SprintState{Minutes: 20, Chars: chars}}
	p.Stats[a.Layout.ID] = now
	start := clk.now().Add(-time.Hour)
	if err := st.AppendRun(retroRuns(30, 0.9, start.Add(-24*time.Hour), 1)[0]); err != nil {
		t.Fatal(err)
	}
	ui.SetScreen(a, ui.NewRetroScreen(a, retroRuns(40, 0.95, start, 3)))
	v := screen(a)
	if n := len(strings.Split(v, "\n")); n > 18 {
		t.Errorf("retro has %d rows, want at most 18", n)
	}
	for _, want := range []string{"zlepšeno", "zhoršeno", "(+10.0)"} {
		if !strings.Contains(v, want) {
			t.Errorf("retro at 60x18 lacks %q:\n%s", want, v)
		}
	}
}

func TestSprintFinaleIsPlayedAtSEV3(t *testing.T) {
	a, st, clk := playApp(t)
	almostDone(t, a, st, clk, 10*60*1000, 7)
	openEntry(a, homeSprint)
	if sev := ui.RoundSpec(a).Severity; sev != incident.SEV3.String() {
		t.Errorf("finale severity = %q, want SEV3", sev)
	}
}

// leaveSprintAfterOneRound plays round 1 of a new Sprint, starts round 2 and
// leaves from the pause menu. It returns the run of round 1.
func leaveSprintAfterOneRound(t *testing.T, a *ui.App, st *store.Store, clk *clock) store.Run {
	t.Helper()
	openEntry(a, homeSprint)
	a.Update(enter)
	typeKeys(a, clk, -1)
	runs, _ := st.Runs()
	if len(runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(runs))
	}
	clk.advance(time.Second)
	a.Update(enter)
	typeKeys(a, clk, 3)
	a.Update(esc)
	a.Update(down)
	a.Update(down)
	a.Update(enter)
	wantScreen(t, a, "home")
	if r := st.Profile().Resume; r == nil || r.Sprint == nil || r.Sprint.NextIndex != 1 {
		t.Fatalf("resume after leaving = %+v", r)
	}
	return runs[0]
}

// slowRound types the current round at 400 ms a key.
func slowRound(a *ui.App, clk *clock) {
	for _, r := range ui.RoundKeys(a) {
		clk.advance(400 * time.Millisecond)
		a.Update(key(r))
	}
}

func TestQuickRunAfterLeavingSprintKeepsTheSprint(t *testing.T) {
	a, st, clk := playApp(t)
	leaveSprintAfterOneRound(t, a, st, clk)
	saved := st.Profile().Resume.Sprint
	a.Update(enter) // the quick run
	wantScreen(t, a, "round")
	slowRound(a, clk)
	wantScreen(t, a, "results")
	r := st.Profile().Resume
	if r == nil || r.Kind != "sprint" || r.Sprint == nil || r.Sprint.NextIndex != 1 || len(r.Sprint.Chars) != len(saved.Chars) {
		t.Fatalf("a quick run changed the sprint resume: %+v", r)
	}
	a.Update(esc)
	wantText(t, a, text.MenuSprint+" ("+text.HomeContinue+")")
	openEntry(a, homeSprint)
	wantScreen(t, a, "round")
	wantText(t, a, "Sprint ")
	if st.Profile().Resume.Sprint.NextIndex != 1 {
		t.Errorf("NextIndex = %d, want 1", st.Profile().Resume.Sprint.NextIndex)
	}
}

func TestStandupAfterLeavingSprintKeepsTheSprint(t *testing.T) {
	a, st, clk := playApp(t)
	leaveSprintAfterOneRound(t, a, st, clk)
	openEntry(a, homeStandup)
	wantScreen(t, a, "round")
	slowRound(a, clk)
	a.Update(esc)
	r := st.Profile().Resume
	if r == nil || r.Kind != "sprint" || r.Sprint == nil || r.Sprint.NextIndex != 1 {
		t.Fatalf("a standup changed the sprint resume: %+v", r)
	}
	openEntry(a, homeSprint)
	wantScreen(t, a, "round")
	wantText(t, a, "Sprint ")
}

func TestRetroIgnoresRunsOutsideTheSprintAndSurvivesQuickRuns(t *testing.T) {
	a, st, clk := playApp(t)
	sprintRun := leaveSprintAfterOneRound(t, a, st, clk)
	a.Update(enter) // an outside quick run, slower than the sprint round
	slowRound(a, clk)
	a.Update(esc)
	all, _ := st.Runs()
	if len(all) != 2 || all[1].WPM >= sprintRun.WPM {
		t.Fatalf("runs = %+v, want a slower outside run", all)
	}

	clk.advance(26 * time.Hour)
	b, err := ui.New(st, a.Lib, ui.WithClock(clk.now))
	if err != nil {
		t.Fatal(err)
	}
	b.Update(windowSize(80, 24))
	wantScreen(t, b, "retro")
	wantText(t, b, text.Rounds(1)+" v tomto sprintu")
	wantText(t, b, fmt.Sprintf("%6.1f", sprintRun.WPM))
	if st.Profile().Resume != nil {
		t.Errorf("Resume = %+v after the retro was opened", st.Profile().Resume)
	}
}

func TestEveryHomeEntryOpensAScreen(t *testing.T) {
	a, _, _ := playApp(t)
	for i := range a.Entries {
		ui.SetScreen(a, ui.NewHomeScreen(a))
		openEntry(a, i+1)
		if got := ui.ScreenName(a); got == "home" || got == "other" {
			t.Errorf("entry %d (%s) opens %q, want a screen of its own", i+1, a.Entries[i].Label(), got)
		}
	}
}

// ownerSymbols is the list the owner gave in section 1 of the spec.
const ownerSymbols = "><+@#$~^&*{}°=%'ˇ/[()]\";§!`¨-_.:,?\\|"

// playQuickRuns plays n rounds of the quick run from the home screen, typing
// cleanly, and counts the keys it was asked for.
func playQuickRuns(t *testing.T, a *ui.App, clk *clock, n int, typed map[rune]int) {
	t.Helper()
	for played := 0; played < n; {
		switch ui.ScreenName(a) {
		case "home", "severity", "results":
			a.Update(enter)
		case "incident-results":
			clk.advance(time.Second)
			a.Update(enter)
		case "round":
			for _, r := range ui.RoundKeys(a) {
				typed[r]++
				clk.advance(150 * time.Millisecond)
				a.Update(key(r))
			}
			played++
		default:
			t.Fatalf("stuck on %s:\n%s", ui.ScreenName(a), screen(a))
		}
	}
}

// A user past the end of the curriculum plays a few quick runs a day for three
// months. Rare symbols must keep coming back, however good the rest gets.
func TestNinetyDaysKeepEveryUnlockedSymbolInTraining(t *testing.T) {
	a, st, clk := playApp(t)
	p := st.Profile()
	var all []string
	for _, ch := range a.Curr.Chapters() {
		all = append(all, ch.ID)
	}
	a.Curr.ApplySkip(p, ui.StatsOf(a), all)
	ui.SetScreen(a, ui.NewHomeScreen(a))
	typed := map[rune]int{}
	for range 90 {
		playQuickRuns(t, a, clk, 4, typed)
		ui.SetScreen(a, ui.NewHomeScreen(a))
		clk.advance(24 * time.Hour)
	}
	for _, r := range ownerSymbols {
		if typed[r] < 8 {
			t.Errorf("%q was typed %d times in 90 days, want it to keep coming back", r, typed[r])
		}
	}
}

// typist is someone with a history at wpm, placed before the given chapter.
func typist(t *testing.T, wpm float64, chapter int) (*ui.App, *store.Store, *clock) {
	t.Helper()
	a, st, clk := playApp(t)
	var skipped []string
	for _, ch := range a.Curr.Chapters()[:chapter] {
		skipped = append(skipped, ch.ID)
	}
	a.Curr.ApplySkip(st.Profile(), ui.StatsOf(a), skipped)
	for i := range 20 {
		run := store.Run{ID: fmt.Sprintf("h%d", i), At: clk.now().Add(-time.Hour), Mode: "lesson", WPM: wpm, Accuracy: 0.98}
		if err := st.AppendRun(run); err != nil {
			t.Fatal(err)
		}
	}
	ui.SetScreen(a, ui.NewHomeScreen(a))
	return a, st, clk
}

// playChain types every round of a Standup or Sprint at wpm, without a
// mistake, until the chain ends on the given screen. It returns the active time.
func playChain(t *testing.T, a *ui.App, st *store.Store, clk *clock, wpm float64, end string) time.Duration {
	t.Helper()
	before, _ := st.Runs()
	gap := time.Duration(12 / wpm * float64(time.Second))
	for rounds := 0; ui.ScreenName(a) != end; {
		switch ui.ScreenName(a) {
		case "round":
			if rounds++; rounds > 2000 {
				t.Fatal("the chain does not end")
			}
			for _, r := range ui.RoundKeys(a) {
				clk.advance(gap)
				a.Update(key(r))
			}
		case "results", "incident-results":
			clk.advance(time.Second)
			a.Update(enter)
		default:
			t.Fatalf("stuck on %s:\n%s", ui.ScreenName(a), screen(a))
		}
	}
	after, _ := st.Runs()
	var active time.Duration
	for _, r := range after[len(before):] {
		active += time.Duration(r.DurationMs) * time.Millisecond
	}
	return active
}

func TestSprintLandsNearItsMinutesAtAnySpeed(t *testing.T) {
	for _, wpm := range []float64{25, 40, 60, 80} {
		t.Run(fmt.Sprintf("%.0f wpm", wpm), func(t *testing.T) {
			a, st, clk := typist(t, wpm, 5)
			openEntry(a, homeSprint)
			wantScreen(t, a, "sprint-minutes")
			a.Update(enter) // 20 minutes
			got := playChain(t, a, st, clk, wpm, "retro")
			t.Logf("%.0f wpm: %v of active time", wpm, got.Round(time.Second))
			if got < 17*time.Minute || got > 23*time.Minute {
				t.Errorf("a 20 minute Sprint took %v of active time", got.Round(time.Second))
			}
		})
	}
}

func TestSprintStillLandsForAFastTypistWithoutHistory(t *testing.T) {
	a, st, clk := playApp(t)
	openEntry(a, homeSprint)
	a.Update(up) // 10 minutes
	a.Update(enter)
	if got := playChain(t, a, st, clk, 80, "retro"); got < 8*time.Minute || got > 12*time.Minute {
		t.Errorf("a 10 minute Sprint took %v of active time", got.Round(time.Second))
	}
}

func TestStandupLastsAboutFiveMinutesAtAnySpeed(t *testing.T) {
	for _, wpm := range []float64{25, 40, 60, 80} {
		t.Run(fmt.Sprintf("%.0f wpm", wpm), func(t *testing.T) {
			a, st, clk := typist(t, wpm, 5)
			openEntry(a, homeStandup)
			got := playChain(t, a, st, clk, wpm, "home")
			t.Logf("%.0f wpm: %v of active time", wpm, got.Round(time.Second))
			if got < 4*time.Minute || got > 6*time.Minute {
				t.Errorf("a Standup took %v of active time", got.Round(time.Second))
			}
		})
	}
}

func TestDrillsLastAboutAMinute(t *testing.T) {
	for _, wpm := range []float64{9, 15, 25, 40, 60, 80} {
		a, _, _ := typist(t, wpm, 5)
		for seed := range 10 {
			spec, err := a.SpecFor(recommend.Pick{Kind: recommend.Drill, Focus: []rune("ě§ˇ{q"), Seed: int64(seed + 1)})
			if err != nil {
				t.Fatal(err)
			}
			secs := float64(len([]rune(spec.Text))) / (wpm * 5 / 60)
			if secs < 30 || secs > 90 {
				t.Errorf("%.0f wpm: a drill of %d characters takes %.0f s", wpm, len([]rune(spec.Text)), secs)
			}
		}
	}
}

// Extra drills must come out the same after leaving: they follow from the
// saved state and the index, whatever the profile did meanwhile.
func TestExtraSprintDrillsResumeDeterministically(t *testing.T) {
	a, st, clk := typist(t, 80, 5)
	openEntry(a, homeSprint)
	a.Update(enter)
	s := st.Profile().Resume
	if s != nil {
		t.Fatalf("no keystroke yet, resume = %+v", s)
	}
	typeKeys(a, clk, 1)
	saved := st.Profile().Resume.Sprint
	saved.NextIndex = len(ui.SprintPlan(a, saved)) + 3 // past the planned rounds
	saved.ActiveMs = 60_000
	want := ui.SprintPick(a, saved, saved.NextIndex)
	st.Profile().Stats[a.Layout.ID] = stats.New()
	if got := ui.SprintPick(a, saved, saved.NextIndex); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("the extra drill changed with the profile:\n%v\n%v", want, got)
	}
	a.Update(esc)
	a.Update(down)
	a.Update(down)
	a.Update(enter)
	wantScreen(t, a, "home")
	openEntry(a, homeSprint)
	wantScreen(t, a, "round")
	first := ui.RoundSpec(a)
	if first.Kind != play.KindDrill {
		t.Fatalf("past the plan the Sprint plays a drill, got %v", first.Kind)
	}
	a.Update(esc)
	a.Update(down)
	a.Update(down)
	a.Update(enter)
	openEntry(a, homeSprint)
	if got := ui.RoundSpec(a); got.Text != first.Text {
		t.Errorf("resuming gave another text:\n%q\n%q", first.Text, got.Text)
	}
}

// When the budget is spent the finale comes, at once and only once.
func TestSprintFinaleEndsTheSprintEvenWhenResumedAfterIt(t *testing.T) {
	a, st, clk := playApp(t)
	almostDone(t, a, st, clk, 10*60*1000, 1)
	openEntry(a, homeSprint)
	wantScreen(t, a, "round")
	if k := ui.RoundSpec(a).Kind; k != play.KindIncident {
		t.Fatalf("round kind = %v, want the finale", k)
	}
	for _, r := range ui.RoundKeys(a) {
		clk.advance(100 * time.Millisecond)
		a.Update(key(r))
	}
	wantScreen(t, a, "incident-results")
	clk.advance(time.Second)
	a.Update(esc) // home, without passing the Retro
	wantScreen(t, a, "home")
	openEntry(a, homeSprint)
	wantScreen(t, a, "retro")
}

func TestWeakBigramReachesTheDrillText(t *testing.T) {
	a, st, _ := typist(t, 40, 5)
	s := st.Profile().Stats[a.Layout.ID]
	s.Bigrams["ěq"] = &stats.Entry{Samples: 20, ErrorRate: 0.6, LatencyMs: 900, LastSeen: newClock().now()}
	openEntry(a, homeStandup)
	wantScreen(t, a, "round")
	if spec := ui.RoundSpec(a); !strings.Contains(spec.Text, "ěq") {
		t.Errorf("the drill does not train the weak bigram: %q", spec.Text)
	}
}

func TestSprintPlanKeepsTheBigramsOfItsStart(t *testing.T) {
	a, st, clk := typist(t, 40, 5)
	s := st.Profile().Stats[a.Layout.ID]
	s.Bigrams["st"] = &stats.Entry{Samples: 20, ErrorRate: 0.6, LatencyMs: 900, LastSeen: clk.now()}
	openEntry(a, homeSprint)
	a.Update(enter)
	typeKeys(a, clk, 1)
	saved := st.Profile().Resume.Sprint
	if saved.Bigrams["st"].Samples != 20 {
		t.Fatalf("the snapshot has no bigrams: %+v", saved.Bigrams)
	}
	before := ui.SprintPlan(a, saved)
	st.Profile().Stats[a.Layout.ID].Bigrams = map[string]*stats.Entry{}
	after := ui.SprintPlan(a, saved)
	if fmt.Sprint(before) != fmt.Sprint(after) {
		t.Errorf("the plan moved with the profile:\n%v\n%v", before, after)
	}
	if !slices.Contains(before[0].Bigrams, "st") {
		t.Errorf("first drill = %+v, want it to train st", before[0])
	}
}

func TestLeftSprintCanBeEndedFromHome(t *testing.T) {
	a, st, clk := playApp(t)
	if strings.Contains(screen(a), text.MenuEndSprint) {
		t.Fatalf("nothing to end yet:\n%s", screen(a))
	}
	leaveSprintAfterOneRound(t, a, st, clk)
	wantText(t, a, text.MenuEndSprint)
	openEntry(a, homeSprint+1)
	wantScreen(t, a, "retro")
	wantText(t, a, text.Rounds(1)+" v tomto sprintu")
	if st.Profile().Resume != nil {
		t.Errorf("Resume = %+v after ending the sprint", st.Profile().Resume)
	}
	a.Update(enter)
	wantScreen(t, a, "home")
	if v := screen(a); strings.Contains(v, text.MenuEndSprint) || strings.Contains(v, text.HomeContinue) {
		t.Errorf("home still offers the ended sprint:\n%s", v)
	}
}

func TestEntriesAfterTheEndSprintRowStillOpen(t *testing.T) {
	a, st, clk := playApp(t)
	leaveSprintAfterOneRound(t, a, st, clk)
	openEntry(a, homeCareer+1) // one row lower with the extra row
	wantScreen(t, a, "career")
}

func TestSprintLeftOverMidnightEndsOnTheNextKeyOnHome(t *testing.T) {
	a, st, clk := playApp(t)
	leaveSprintAfterOneRound(t, a, st, clk)
	a.Update(up) // the last row, which is gone once the extra row is
	clk.advance(24 * time.Hour)
	a.Update(enter)
	wantScreen(t, a, "retro")
}

func TestRepeatKeepsOutOfAStandup(t *testing.T) {
	a, _, clk := playApp(t)
	openEntry(a, homeStandup)
	typeKeys(a, clk, -1)
	wantScreen(t, a, "results")
	wantText(t, a, text.HintResults)
	a.Update(key('r'))
	wantScreen(t, a, "results")
}
