package ui_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jiripisa/tuipe/internal/hint"
	"github.com/jiripisa/tuipe/internal/stats"
	"github.com/jiripisa/tuipe/internal/store"
	"github.com/jiripisa/tuipe/internal/ui"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

const homeStats = 6 // downs from the first home entry to Statistiky

var (
	tab    = tea.KeyPressMsg{Code: tea.KeyTab}
	right  = tea.KeyPressMsg{Code: tea.KeyRight}
	left   = tea.KeyPressMsg{Code: tea.KeyLeft}
	pgDown = tea.KeyPressMsg{Code: tea.KeyPgDown}
	pgUp   = tea.KeyPressMsg{Code: tea.KeyPgUp}
)

// statsApp is an app on the fixed clock with a window of the given size.
func statsApp(t *testing.T, w, h int) (*ui.App, *store.Store, *clock) {
	t.Helper()
	a, st, clk := playApp(t)
	a.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return a, st, clk
}

func openStats(t *testing.T, a *ui.App) {
	t.Helper()
	openFromHome(t, a, homeStats, "stats")
}

func addRun(t *testing.T, st *store.Store, r store.Run) {
	t.Helper()
	if err := st.AppendRun(r); err != nil {
		t.Fatal(err)
	}
}

// daysAgo is noon, n days before the fixed clock.
func daysAgo(clk *clock, n int) time.Time { return clk.t.AddDate(0, 0, -n) }

// lessonOf returns the first lesson of the chapter with the given number.
func lessonOf(t *testing.T, a *ui.App, number int) (id, title string) {
	t.Helper()
	for _, ch := range a.Curr.Chapters() {
		if ch.Number == number && len(ch.Lessons) > 0 {
			return ch.Lessons[0].ID, ch.Lessons[0].Title
		}
	}
	t.Fatalf("no chapter %d with lessons", number)
	return "", ""
}

// lineWith returns the n-th (0-based) line of the screen that starts with prefix after trimming.
func nthLine(s, prefix string, n int) string {
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(strings.Trim(strings.TrimSpace(l), "│"))
		if strings.HasPrefix(l, prefix) {
			if n == 0 {
				return l
			}
			n--
		}
	}
	return ""
}

func TestGitLogShowsHashWPMAccuracyStarsAndDelta(t *testing.T) {
	a, st, clk := statsApp(t, 100, 30)
	id, title := lessonOf(t, a, 1)
	delta := int64(3200)
	addRun(t, st, store.Run{ID: "a3f9c1e", At: daysAgo(clk, 0), Mode: "lesson", LessonID: id,
		WPM: 62, Accuracy: 0.981, Stars: 2, GhostDeltaMs: &delta})
	addRun(t, st, store.Run{ID: "b7d20aa", At: daysAgo(clk, 1), Mode: "code", WPM: 40, Accuracy: 0.9, Stars: 0})
	addRun(t, st, store.Run{ID: "c0ffee1", At: daysAgo(clk, 1).Add(-time.Hour), Mode: "incident", WPM: 30, Accuracy: 0.8,
		Incident: &store.IncidentResult{Severity: "SEV2", Won: false}})
	openStats(t, a)
	s := screen(a)

	want := "a3f9c1e  62 wpm  98.1 %  ★★☆  +3.2 s  " + title
	if !strings.Contains(s, want) {
		t.Fatalf("git log lacks the line %q:\n%s", want, s)
	}
	if !strings.Contains(s, "b7d20aa") || !strings.Contains(s, text.StatsModeCode) {
		t.Errorf("a non-lesson run must show its mode:\n%s", s)
	}
	if l := nthLine(s, "c0ffee1", 0); !strings.Contains(l, text.StatsLost) {
		t.Errorf("a lost incident must be marked with %q: %q", text.StatsLost, l)
	}
	if l := nthLine(s, "a3f9c1e", 0); strings.Contains(l, text.StatsLost) {
		t.Errorf("an ordinary run must not be marked lost: %q", l)
	}
	if !(strings.Index(s, "a3f9c1e") < strings.Index(s, "b7d20aa") && strings.Index(s, "b7d20aa") < strings.Index(s, "c0ffee1")) {
		t.Errorf("runs are not newest first:\n%s", s)
	}
	for _, d := range []string{"2026-10-31", "2026-10-30"} {
		if strings.Count(s, d) != 1 {
			t.Errorf("date header %s must appear once:\n%s", d, s)
		}
	}
	if !strings.Contains(s, "up 2 days") {
		t.Errorf("header lacks the uptime:\n%s", s)
	}
}

func TestUptimeOnHeaderCountsConsecutiveDays(t *testing.T) {
	a, st, clk := statsApp(t, 80, 24)
	for i := range 3 {
		addRun(t, st, store.Run{ID: fmt.Sprintf("r%d", i), At: daysAgo(clk, i), Mode: "drill", WPM: 30, Accuracy: 0.95})
	}
	openStats(t, a)
	if s := screen(a); !strings.Contains(s, "up 3 days") {
		t.Errorf("header lacks %q:\n%s", "up 3 days", s)
	}
}

func TestGitLogReadsRunsOnceOnEntering(t *testing.T) {
	a, st, clk := statsApp(t, 100, 30)
	addRun(t, st, store.Run{ID: "aaaaaaa", At: daysAgo(clk, 0), Mode: "drill", WPM: 30, Accuracy: 0.95})
	openStats(t, a)
	addRun(t, st, store.Run{ID: "bbbbbbb", At: daysAgo(clk, 0), Mode: "drill", WPM: 30, Accuracy: 0.95})
	if s := screen(a); strings.Contains(s, "bbbbbbb") {
		t.Errorf("runs were read again after entering the screen:\n%s", s)
	}
	a.Update(esc)
	openStats(t, a)
	if s := screen(a); !strings.Contains(s, "bbbbbbb") {
		t.Errorf("a new visit must read the runs again:\n%s", s)
	}
}

func TestGitLogScrollsBeyondWindowHeight(t *testing.T) {
	a, st, clk := statsApp(t, 60, 18)
	const n = 50
	for i := range n { // i = 0 is the newest
		addRun(t, st, store.Run{ID: fmt.Sprintf("run%04d", i), At: daysAgo(clk, 0).Add(-time.Duration(i) * time.Minute),
			Mode: "drill", WPM: 30, Accuracy: 0.95})
	}
	openStats(t, a)
	selected := func() string { return nthLine(screen(a), ">", 0) }

	if s := selected(); !strings.Contains(s, "run0000") {
		t.Fatalf("the newest run must be selected first: %q", s)
	}
	for range 30 {
		a.Update(down)
	}
	if s := selected(); !strings.Contains(s, "run0030") {
		t.Fatalf("after 30 downs the selection is %q, want run0030:\n%s", s, screen(a))
	}
	a.Update(up)
	if s := selected(); !strings.Contains(s, "run0029") {
		t.Errorf("after Up the selection is %q, want run0029", s)
	}
	a.Update(pgDown)
	first := selected()
	if first == "" || strings.Contains(first, "run0029") {
		t.Errorf("PgDn did not move the selection: %q", first)
	}
	for range 10 {
		a.Update(pgDown)
	}
	if s := selected(); !strings.Contains(s, "run0049") {
		t.Errorf("PgDn to the end must select the oldest run: %q\n%s", s, screen(a))
	}
	for range 10 {
		a.Update(pgUp)
	}
	if s := selected(); !strings.Contains(s, "run0000") {
		t.Errorf("PgUp to the top must select the newest run: %q\n%s", s, screen(a))
	}
	if got := len(strings.Split(screen(a), "\n")); got != 18 {
		t.Errorf("screen has %d lines, want 18", got)
	}
}

func openHeatmap(t *testing.T, a *ui.App) {
	t.Helper()
	openStats(t, a)
	a.Update(tab)
}

func setStats(a *ui.App, chars map[rune]stats.Entry) {
	s := stats.New()
	for r, e := range chars {
		e := e
		s.Chars[string(r)] = &e
	}
	a.Store.Profile().Stats[a.Layout.ID] = s
}

// keyStyle returns the escape sequences that precede the five-column label of a key in the raw view.
func keyStyle(t *testing.T, raw, label string) string {
	t.Helper()
	re := regexp.MustCompile(`((?:\x1b\[[0-9;:]*m)+)\s*` + regexp.QuoteMeta(label) + `\s*\x1b\[`)
	m := re.FindStringSubmatch(raw)
	if m == nil {
		t.Fatalf("key %q not found in the raw view", label)
	}
	return m[1]
}

func TestHeatmapColoursByWeakness(t *testing.T) {
	a, st, clk := statsApp(t, 100, 40)
	addRun(t, st, store.Run{ID: "r", At: daysAgo(clk, 0), Mode: "drill", WPM: 30, Accuracy: 0.95})
	setStats(a, map[rune]stats.Entry{
		'a': {LatencyMs: 800, Samples: 10}, // weakness 0.8: the highest
		'd': {LatencyMs: 400, Samples: 10}, // half of it
		'f': {LatencyMs: 800, Samples: 2},  // too few samples: not weak
	})
	openHeatmap(t, a)
	raw := a.View().Content
	sa, sd, sf := keyStyle(t, raw, "a"), keyStyle(t, raw, "d"), keyStyle(t, raw, "f")
	if sa == sd || sa == sf || sd == sf {
		t.Errorf("weak, half-weak and untouched keys must differ:\nA %q\nD %q\nF %q", sa, sd, sf)
	}
	// An untouched key looks the same as on an empty map, which is the dim style.
	setStats(a, map[rune]stats.Entry{'a': {LatencyMs: 800, Samples: 10}})
	a.Update(esc)
	openHeatmap(t, a)
	if got := keyStyle(t, a.View().Content, "s"); got != sf {
		t.Errorf("an untouched key changed colour: %q vs %q", got, sf)
	}
}

func TestHeatmapUsesLastChordKeyOfTheRoute(t *testing.T) {
	a, st, clk := statsApp(t, 100, 40)
	addRun(t, st, store.Run{ID: "r", At: daysAgo(clk, 0), Mode: "drill", WPM: 30, Accuracy: 0.95})
	// "A" is typed with Shift on the key of "a": the key heats, not a Shift key.
	setStats(a, map[rune]stats.Entry{'A': {LatencyMs: 800, Samples: 10}})
	openHeatmap(t, a)
	raw := a.View().Content
	if keyStyle(t, raw, "a") == keyStyle(t, raw, "f") {
		t.Error("the key of a Shift+a character must be heated")
	}
	// "ď" is a dead key followed by "d": the route ends on the key of "d".
	setStats(a, map[rune]stats.Entry{'ď': {LatencyMs: 800, Samples: 10}})
	a.Update(esc)
	openHeatmap(t, a)
	raw = a.View().Content
	if keyStyle(t, raw, "d") == keyStyle(t, raw, "f") {
		t.Error("the last key of a two-chord route must be heated")
	}
}

func TestFingerSummaryNamesWeakestFinger(t *testing.T) {
	a, st, clk := statsApp(t, 100, 40)
	addRun(t, st, store.Run{ID: "r", At: daysAgo(clk, 0), Mode: "drill", WPM: 30, Accuracy: 0.95})
	// The left ring finger has the weakest single key (s), but the left pinky
	// has the larger sum (a + q).
	setStats(a, map[rune]stats.Entry{
		's': {LatencyMs: 600, Samples: 10},
		'a': {LatencyMs: 400, Samples: 10},
		'q': {LatencyMs: 400, Samples: 10},
	})
	openHeatmap(t, a)
	s := screen(a)
	want := fmt.Sprintf(text.StatsWeakestFinger, "levý malíček")
	if !strings.Contains(s, want) {
		t.Errorf("finger summary lacks %q:\n%s", want, s)
	}
	for _, r := range []rune{'s', 'a', 'q'} {
		h := hint.Text(a.Layout, r)
		if !strings.Contains(s, h) {
			t.Errorf("weakest characters lack the hint %q:\n%s", h, s)
		}
	}
	if strings.Index(s, hint.Text(a.Layout, 's')) > strings.Index(s, hint.Text(a.Layout, 'a')) {
		t.Errorf("the weakest character must come first:\n%s", s)
	}
}

func TestHeatmapListsAtMostEightCharsAndFitsMinimumWindow(t *testing.T) {
	a, st, clk := statsApp(t, 60, 18)
	addRun(t, st, store.Run{ID: "r", At: daysAgo(clk, 0), Mode: "drill", WPM: 30, Accuracy: 0.95})
	chars := map[rune]stats.Entry{}
	for i, r := range "asdfjklqwerty" {
		chars[r] = stats.Entry{LatencyMs: float64(1000 - 50*i), Samples: 10}
	}
	setStats(a, chars)
	openHeatmap(t, a)
	s := screen(a)
	shown := 0
	for _, r := range "asdfjklqwerty" {
		if strings.Contains(s, hint.Text(a.Layout, r)) {
			shown++
		}
	}
	if shown != 8 {
		t.Errorf("shows %d weakest characters, want 8:\n%s", shown, s)
	}
	if got := len(strings.Split(s, "\n")); got != 18 {
		t.Errorf("screen has %d lines, want 18", got)
	}
}

func TestHeatmapOfEmptyStatsShowsMessage(t *testing.T) {
	a, st, clk := statsApp(t, 80, 24)
	addRun(t, st, store.Run{ID: "r", At: daysAgo(clk, 0), Mode: "drill", WPM: 30, Accuracy: 0.95})
	openHeatmap(t, a)
	if s := screen(a); !strings.Contains(s, text.StatsHeatEmpty) {
		t.Errorf("empty heatmap lacks %q:\n%s", text.StatsHeatEmpty, s)
	}
}

func TestShowingAHintHereIsNotHintUse(t *testing.T) {
	a, st, clk := statsApp(t, 100, 40)
	addRun(t, st, store.Run{ID: "r", At: daysAgo(clk, 0), Mode: "drill", WPM: 30, Accuracy: 0.95})
	setStats(a, map[rune]stats.Entry{'a': {LatencyMs: 800, Samples: 10, Mastery: stats.Learning}})
	before := *a.Store.Profile().Stats[a.Layout.ID].Chars["a"]
	openHeatmap(t, a)
	_ = screen(a)
	if after := *a.Store.Profile().Stats[a.Layout.ID].Chars["a"]; after != before {
		t.Errorf("viewing the heatmap changed the statistics: %+v -> %+v", before, after)
	}
	if runs, _ := st.Runs(); len(runs) != 1 {
		t.Errorf("viewing the heatmap wrote runs: %d", len(runs))
	}
}

func openTrends(t *testing.T, a *ui.App) {
	t.Helper()
	openStats(t, a)
	a.Update(tab)
	a.Update(tab)
}

func TestTrendsOverallAndPerGroup(t *testing.T) {
	a, st, clk := statsApp(t, 60, 18)
	letters, _ := lessonOf(t, a, 1)
	diacritics, _ := lessonOf(t, a, 5)
	digits, _ := lessonOf(t, a, 6)
	shift, _ := lessonOf(t, a, 4)
	option, _ := lessonOf(t, a, 8)
	n := 0
	run := func(day int, lesson string, wpm, acc float64) {
		n++
		addRun(t, st, store.Run{ID: fmt.Sprintf("t%d", n), At: daysAgo(clk, day), Mode: "lesson", LessonID: lesson, WPM: wpm, Accuracy: acc})
	}
	// Diacritics rise 10, 20, 30; letters fall 30, 20, 10; the others have one day.
	for i, w := range []float64{10, 20, 30} {
		run(2-i, diacritics, w, 0.90+0.02*float64(i))
		run(2-i, letters, 40-w, 0.99-0.02*float64(i))
	}
	run(0, digits, 25, 0.95)
	run(0, shift, 26, 0.96)
	run(0, option, 27, 0.97)
	// A drill has no chapter: overall only. A lost incident is no WPM sample.
	addRun(t, st, store.Run{ID: "drill", At: daysAgo(clk, 3), Mode: "drill", WPM: 20, Accuracy: 0.9})
	addRun(t, st, store.Run{ID: "lost", At: daysAgo(clk, 4), Mode: "incident", WPM: 99, Accuracy: 0.5,
		Incident: &store.IncidentResult{Severity: "SEV1", Won: false}})
	openTrends(t, a)
	s := screen(a)

	glyphs := func(l string) string {
		var g []rune
		for _, r := range l {
			if r >= '▁' && r <= '█' {
				g = append(g, r)
			}
		}
		return string(g)
	}
	for label, want := range map[string]string{
		text.StatsGroupDiacritics: "▁▅█",
		text.StatsGroupLetters:    "█▅▁",
		text.StatsGroupDigits:     "▄",
		text.StatsGroupShift:      "▄",
		text.StatsGroupOption:     "▄",
	} {
		if got := glyphs(nthLine(s, label, 0)); got != want {
			t.Errorf("WPM trend of %s = %q, want %q:\n%s", label, got, want, s)
		}
		if nthLine(s, label, 1) == "" {
			t.Errorf("accuracy trend of %s is missing:\n%s", label, s)
		}
	}
	// Overall: the drill day and the three lesson days; the lost incident is no WPM sample.
	if got := glyphs(nthLine(s, text.StatsGroupOverall, 0)); len([]rune(got)) != 4 {
		t.Errorf("overall WPM trend = %q, want 4 days (drill included, lost incident excluded):\n%s", got, s)
	}
	// Accuracy does take the lost incident into account: five days.
	if got := glyphs(nthLine(s, text.StatsGroupOverall, 1)); len([]rune(got)) != 5 {
		t.Errorf("overall accuracy trend = %q, want 5 days:\n%s", got, s)
	}
	if !strings.Contains(nthLine(s, text.StatsGroupDiacritics, 0), "30") {
		t.Errorf("the latest WPM should be shown: %q", nthLine(s, text.StatsGroupDiacritics, 0))
	}
}

func TestTrendsUseDailyMediansOfTheLast30ActiveDays(t *testing.T) {
	a, st, clk := statsApp(t, 80, 24)
	// 35 active days; the sparkline keeps the newest 30. A day of three runs
	// with 10, 20 and 90 WPM has the median 20.
	for d := range 35 {
		addRun(t, st, store.Run{ID: fmt.Sprintf("d%02d", d), At: daysAgo(clk, d*2), Mode: "drill", WPM: 50, Accuracy: 0.95})
	}
	for i, w := range []float64{10, 90, 20} {
		addRun(t, st, store.Run{ID: fmt.Sprintf("m%d", i), At: daysAgo(clk, 1).Add(time.Duration(i) * time.Minute), Mode: "drill", WPM: w, Accuracy: 0.95})
	}
	openTrends(t, a)
	l := nthLine(screen(a), text.StatsGroupOverall, 0)
	count := 0
	for _, r := range l {
		if r >= '▁' && r <= '█' {
			count++
		}
	}
	if count != 30 {
		t.Errorf("sparkline has %d points, want 30: %q", count, l)
	}
}

func TestEmptyHistoryShowsFriendlyMessage(t *testing.T) {
	a, _, _ := statsApp(t, 80, 24)
	openStats(t, a)
	for i, name := range []string{"git log", "heatmap", "trendy", "rekordy"} {
		if i > 0 {
			a.Update(tab)
		}
		s := screen(a)
		if !strings.Contains(s, text.StatsEmpty) {
			t.Errorf("tab %s lacks %q:\n%s", name, text.StatsEmpty, s)
		}
		if !strings.Contains(s, name) {
			t.Errorf("tab bar lacks %q:\n%s", name, s)
		}
	}
}

func TestPersonalBestsPerMode(t *testing.T) {
	a, st, clk := statsApp(t, 80, 24)
	id, _ := lessonOf(t, a, 1)
	add := func(r store.Run) {
		r.At = daysAgo(clk, 0)
		addRun(t, st, r)
	}
	add(store.Run{ID: "l1", Mode: "lesson", LessonID: id, WPM: 50, Accuracy: 0.97, Score: 44})
	add(store.Run{ID: "l2", Mode: "lesson", LessonID: id, WPM: 55, Accuracy: 0.96, Score: 46.7}) // best
	add(store.Run{ID: "l3", Mode: "lesson", LessonID: id, WPM: 90, Accuracy: 0.99, Score: 86, Paused: true})
	add(store.Run{ID: "i1", Mode: "incident", WPM: 45, Accuracy: 0.95, Score: 36.7,
		Incident: &store.IncidentResult{Severity: "SEV3", Won: true}})
	add(store.Run{ID: "i2", Mode: "incident", WPM: 80, Accuracy: 0.99, Score: 76.8,
		Incident: &store.IncidentResult{Severity: "SEV1", Won: false}})
	add(store.Run{ID: "c1", Mode: "code", WPM: 33, Accuracy: 0.931, Score: 24.8})
	openStats(t, a)
	for range 3 {
		a.Update(tab)
	}
	s := screen(a)
	for mode, want := range map[string]string{
		text.StatsModeLesson:   "55 wpm  96.0 %",
		text.StatsModeIncident: "45 wpm  95.0 %",
		text.StatsModeCode:     "33 wpm  93.1 %",
	} {
		if l := nthLine(s, mode, 0); !strings.Contains(l, want) {
			t.Errorf("record of %s = %q, want it to contain %q:\n%s", mode, l, want, s)
		}
	}
	if strings.Contains(s, "90 wpm") || strings.Contains(s, "80 wpm") {
		t.Errorf("a paused run or a lost incident counted as a record:\n%s", s)
	}
	if l := nthLine(s, text.StatsSessionStandup, 0); l != "" {
		t.Errorf("a mode without runs must not be listed: %q", l)
	}
}

func TestTabsSwitchWithTabAndArrowsAndEscGoesHome(t *testing.T) {
	a, st, clk := statsApp(t, 80, 24)
	addRun(t, st, store.Run{ID: "r", At: daysAgo(clk, 0), Mode: "drill", WPM: 30, Accuracy: 0.95})
	openStats(t, a)
	bracket := regexp.MustCompile(`\[[^\]]+\]`)
	marker := func() string { // the active tab is bracketed
		for _, l := range strings.Split(screen(a), "\n") {
			if strings.Contains(l, "rekordy") {
				return bracket.FindString(l)
			}
		}
		return ""
	}
	if m := marker(); !strings.Contains(m, "[git log]") {
		t.Fatalf("active tab = %q, want [git log]", m)
	}
	a.Update(right)
	if m := marker(); !strings.Contains(m, "[heatmap]") {
		t.Errorf("after Right: %q", m)
	}
	a.Update(tab)
	if m := marker(); !strings.Contains(m, "[trendy]") {
		t.Errorf("after Tab: %q", m)
	}
	a.Update(left)
	a.Update(left)
	if m := marker(); !strings.Contains(m, "[git log]") {
		t.Errorf("after two Left: %q", m)
	}
	a.Update(left)
	if m := marker(); !strings.Contains(m, "[rekordy]") {
		t.Errorf("Left from the first tab must wrap: %q", m)
	}
	a.Update(esc)
	if ui.ScreenName(a) != "home" {
		t.Errorf("Esc leads to %s, want home", ui.ScreenName(a))
	}
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

func TestFinishWritesTheSessionOfEveryWayToStartARound(t *testing.T) {
	t.Run("quick", func(t *testing.T) {
		a, st, clk := playApp(t)
		a.Update(enter) // the recommended round
		typeKeys(a, clk, -1)
		if got := onlyRun(t, st).Session; got != "quick" {
			t.Errorf("Session = %q, want quick", got)
		}
	})
	t.Run("career", func(t *testing.T) {
		a, st, clk := playApp(t)
		tall(a)
		openFromHome(t, a, homeCareer, "career")
		a.Update(enter)
		typeKeys(a, clk, -1)
		if got := onlyRun(t, st).Session; got != "career" {
			t.Errorf("Session = %q, want career", got)
		}
	})
	t.Run("practice", func(t *testing.T) {
		a, st, clk := playApp(t)
		tall(a)
		openPractice(t, a, practiceLesson)
		a.Update(enter)
		typeKeys(a, clk, -1)
		if got := onlyRun(t, st).Session; got != "practice" {
			t.Errorf("Session = %q, want practice", got)
		}
	})
	t.Run("standup", func(t *testing.T) {
		a, st, clk := playApp(t)
		openEntry(a, homeStandup)
		typeKeys(a, clk, -1)
		if got := onlyRun(t, st).Session; got != "standup" {
			t.Errorf("Session = %q, want standup", got)
		}
	})
	t.Run("sprint", func(t *testing.T) {
		a, st, clk := playApp(t)
		openEntry(a, homeSprint)
		a.Update(enter)
		typeKeys(a, clk, -1)
		if got := onlyRun(t, st).Session; got != "sprint" {
			t.Errorf("Session = %q, want sprint", got)
		}
	})
	t.Run("incident from the list", func(t *testing.T) {
		a, st, clk := playApp(t)
		openFromHome(t, a, homeIncident, "incident-list")
		a.Update(enter)
		a.Update(enter)
		typeKeys(a, clk, -1)
		if got := onlyRun(t, st).Session; got != "career" {
			t.Errorf("Session = %q, want career", got)
		}
	})
}

func TestRecordsPerSessionAndMode(t *testing.T) {
	a, st, clk := statsApp(t, 80, 24)
	id, _ := lessonOf(t, a, 1)
	add := func(r store.Run) {
		r.At = daysAgo(clk, 0)
		addRun(t, st, r)
	}
	add(store.Run{ID: "q1", Session: "quick", Mode: "drill", WPM: 41, Accuracy: 0.95, Score: 33})
	add(store.Run{ID: "q0", Mode: "drill", WPM: 44, Accuracy: 0.96, Score: 37}) // no session: quick
	add(store.Run{ID: "s1", Session: "standup", Mode: "drill", WPM: 52, Accuracy: 0.97, Score: 46})
	add(store.Run{ID: "p1", Session: "sprint", Mode: "lesson", LessonID: id, WPM: 58, Accuracy: 0.98, Score: 53})
	add(store.Run{ID: "p2", Session: "sprint", Mode: "code", WPM: 60, Accuracy: 0.99, Score: 57, Paused: true})
	add(store.Run{ID: "l1", Session: "career", Mode: "lesson", LessonID: id, WPM: 47, Accuracy: 0.94, Score: 36})
	openStats(t, a)
	for range 3 {
		a.Update(tab)
	}
	s := screen(a)
	for label, want := range map[string]string{
		text.StatsSessionQuick:   "44 wpm  96.0 %",
		text.StatsSessionStandup: "52 wpm  97.0 %",
		text.StatsSessionSprint:  "58 wpm  98.0 %",
		text.StatsModeLesson:     "58 wpm  98.0 %",
	} {
		if l := nthLine(s, label, 0); !strings.Contains(l, want) {
			t.Errorf("record %q = %q, want it to contain %q:\n%s", label, l, want, s)
		}
	}
	if l := nthLine(s, text.StatsModeCode, 0); l != "" {
		t.Errorf("a paused run set a code record: %q", l)
	}
}

func TestGitLogNamesTheSessionAndAlignsAccuracy(t *testing.T) {
	a, st, clk := statsApp(t, 100, 30)
	addRun(t, st, store.Run{ID: "aaaaaaa", At: daysAgo(clk, 0), Session: "standup", Mode: "drill", WPM: 70, Accuracy: 1, Stars: 3})
	addRun(t, st, store.Run{ID: "bbbbbbb", At: daysAgo(clk, 0).Add(-time.Minute), Session: "sprint", Mode: "code", WPM: 5, Accuracy: 0.5})
	addRun(t, st, store.Run{ID: "ccccccc", At: daysAgo(clk, 0).Add(-2 * time.Minute), Session: "quick", Mode: "drill", WPM: 30, Accuracy: 0.9})
	openStats(t, a)
	s := screen(a)
	stars := func(id string) int { // the offset of the stars from the hash
		for _, l := range strings.Split(s, "\n") {
			if i := strings.Index(l, id); i >= 0 {
				return strings.IndexAny(l, "★☆") - i
			}
		}
		return -1
	}
	if stars("aaaaaaa") != stars("bbbbbbb") || stars("bbbbbbb") != stars("ccccccc") {
		t.Errorf("the stars column is not aligned (100.0 %% pushes it):\n%s", s)
	}
	for id, want := range map[string]string{
		"aaaaaaa": text.StatsSessionStandup, "bbbbbbb": text.StatsSessionSprint, "ccccccc": text.StatsSessionQuick,
	} {
		if l := nthLine(s, id, 0); !strings.Contains(l, want) {
			if l = nthLine(s, "> "+id, 0); !strings.Contains(l, want) {
				t.Errorf("line of %s lacks the session %q: %q", id, want, l)
			}
		}
	}
}

func TestRecordLabelsAreCapitalised(t *testing.T) {
	a, st, clk := statsApp(t, 80, 24)
	id, _ := lessonOf(t, a, 1)
	inc := &store.IncidentResult{Severity: "SEV3", Won: true}
	for i, r := range []store.Run{
		{Session: "quick", Mode: "drill"},
		{Session: "career", Mode: "lesson", LessonID: id},
		{Session: "career", Mode: "code"},
		{Session: "career", Mode: "incident", Incident: inc},
		{Session: "standup", Mode: "drill"},
		{Session: "sprint", Mode: "drill"},
	} {
		r.ID, r.At, r.WPM, r.Accuracy, r.Score = fmt.Sprintf("x%d", i), daysAgo(clk, 0), 40, 0.95, 30
		addRun(t, st, r)
	}
	openStats(t, a)
	for range 3 {
		a.Update(tab)
	}
	s := screen(a)
	for _, row := range []string{"Rychlovka", "Lekce", "Kód", "Incident", "Standup", "Sprint"} {
		if !strings.Contains(s, "│"+row+" ") && !strings.Contains(s, row+"  ") {
			t.Errorf("no record row %q:\n%s", row, s)
		}
	}
	for _, bad := range []string{"lekce", "kód", "incident ", "drill"} {
		if strings.Contains(s, bad) {
			t.Errorf("record rows must be capitalised, found %q:\n%s", bad, s)
		}
	}
	a.Update(left)
	a.Update(left)
	a.Update(left)
	if !strings.Contains(screen(a), "Drill · Rychlovka") {
		t.Errorf("git log lacks %q:\n%s", "Drill · Rychlovka", screen(a))
	}
}
