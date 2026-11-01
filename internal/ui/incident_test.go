package ui_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jiripisa/tuipe/internal/content"
	"github.com/jiripisa/tuipe/internal/engine"
	"github.com/jiripisa/tuipe/internal/hint"
	"github.com/jiripisa/tuipe/internal/incident"
	"github.com/jiripisa/tuipe/internal/play"
	"github.com/jiripisa/tuipe/internal/recommend"
	"github.com/jiripisa/tuipe/internal/store"
	"github.com/jiripisa/tuipe/internal/ui"
	"github.com/jiripisa/tuipe/internal/ui/style"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

const homeIncident = 5 // downs from the first home entry to the incident list

// bossChapter is the first lettered chapter that ends with an incident.
func bossChapter(t *testing.T, a *ui.App) (content.ChapterDef, content.Incident) {
	t.Helper()
	for _, ch := range a.Curr.Chapters() {
		if inc, ok := a.Curr.IncidentDef(ch.ID); ok && ch.Number > 0 {
			return ch, inc
		}
	}
	t.Fatal("no chapter with an incident")
	return content.ChapterDef{}, content.Incident{}
}

// bossAt is the incident that ends the chapter with the given number.
func bossAt(t *testing.T, a *ui.App, number int) (content.ChapterDef, content.Incident) {
	t.Helper()
	for _, ch := range a.Curr.Chapters() {
		if ch.Number != number {
			continue
		}
		if inc, ok := a.Curr.IncidentDef(ch.ID); ok {
			return ch, inc
		}
	}
	t.Fatalf("chapter %d has no incident", number)
	return content.ChapterDef{}, content.Incident{}
}

// readyForBoss gives every lesson up to and including the boss's chapter a
// star, and wins the bosses before it, so only the incident is owed.
func readyForBoss(a *ui.App, st *store.Store, boss content.ChapterDef) {
	p := st.Profile()
	for _, c := range a.Curr.Chapters() {
		for _, l := range c.Lessons {
			p.Stars[l.ID] = 1
		}
		if inc, ok := a.Curr.IncidentDef(c.ID); ok && c.ID != boss.ID {
			p.IncidentBest[inc.ID] = "SEV3"
			p.Completed[c.ID] = true
		}
		if c.ID == boss.ID {
			return
		}
	}
}

func startIncident(t *testing.T, a *ui.App, inc content.Incident, sev incident.Severity) {
	t.Helper()
	ui.SetScreen(a, ui.NewIncidentScreen(a, inc, sev, nil))
	if ui.ScreenName(a) != "round" {
		t.Fatalf("incident screen is %s, want round", ui.ScreenName(a))
	}
	a.Update(ui.TickMsg(a))
}

func joined(inc content.Incident) string {
	var parts []string
	for _, s := range inc.Steps {
		parts = append(parts, s.Text)
	}
	return strings.Join(parts, "\n")
}

// wrongFor is a key that is not expected.
func wrongFor(expected rune) tea.KeyPressMsg {
	if expected == 'x' {
		return key('y')
	}
	return key('x')
}

// breachByErrors types 12 wrong keys on the first character and 8 on the first
// character that differs from it, which burns the whole error budget. It
// returns the two damaged characters.
func breachByErrors(a *ui.App) (first, second rune) {
	keys := ui.RoundKeys(a)
	first = keys[0]
	for range 12 {
		a.Update(wrongFor(first))
	}
	j := 0
	for keys[j] == first {
		a.Update(key(keys[j]))
		j++
	}
	second = keys[j]
	for range 8 {
		a.Update(wrongFor(second))
	}
	return first, second
}

func firstWords(s string, n int) string {
	w := strings.Fields(s)
	return strings.Join(w[:min(n, len(w))], " ")
}

func chars(txt string) int {
	n := 0
	for _, c := range engine.Prepare(txt).Cells {
		if !c.Auto {
			n++
		}
	}
	return n
}

func TestIncidentRunsAllStepsWithSharedMeter(t *testing.T) {
	a, st, clk := playApp(t)
	_, inc := bossChapter(t, a)
	if len(inc.Steps) < 3 {
		t.Fatalf("setup: incident has %d steps", len(inc.Steps))
	}
	startIncident(t, a, inc, incident.SEV3)

	spec := ui.RoundSpec(a)
	if spec.Kind != play.KindIncident || spec.Text != joined(inc) {
		t.Fatalf("spec: kind %v text %q, want one session over %q", spec.Kind, spec.Text, joined(inc))
	}
	runs, _ := st.Runs()
	want := incident.NewParams(incident.SEV3, recommend.BaseWPM(runs), chars(spec.Text))
	if spec.Incident == nil || *spec.Incident != want {
		t.Fatalf("spec.Incident = %+v, want %+v", spec.Incident, want)
	}
	if spec.Severity != "SEV3" || len(spec.Graces) != len(inc.Steps)-1 {
		t.Errorf("severity %q graces %v, want SEV3 and one grace per step boundary", spec.Severity, spec.Graces)
	}
	at2 := len([]rune(inc.Steps[0].Text)) + 1
	if want := time.Duration(len([]rune(inc.Steps[1].Prompt))) * time.Second / 20; spec.Graces[at2] != want {
		t.Errorf("grace for step 2 = %v, want its prompt over 20 characters a second = %v", spec.Graces[at2], want)
	}
	if v := screen(a); !strings.Contains(v, firstWords(inc.Steps[0].Prompt, 3)) || !strings.Contains(v, fmt.Sprintf("%s  1/%d", inc.ID, len(inc.Steps))) {
		t.Errorf("step 1 prompt or counter missing:\n%s", v)
	}

	// Step 1 including the Enter that ends it; the next step's prompt appears.
	n1 := len([]rune(inc.Steps[0].Text)) + 1
	typeKeys(a, clk, n1)
	sla1 := ui.RoundFrame(a).SLA
	v := screen(a)
	if !strings.Contains(v, firstWords(inc.Steps[1].Prompt, 3)) || !strings.Contains(v, fmt.Sprintf("%s  2/%d", inc.ID, len(inc.Steps))) {
		t.Errorf("step 2 prompt or counter missing:\n%s", v)
	}
	if strings.Contains(v, firstWords(inc.Steps[0].Prompt, 4)) {
		t.Errorf("step 1 prompt still shown:\n%s", v)
	}
	if sla1 >= 100 {
		t.Fatalf("SLA after step 1 = %.3f, want it burnt", sla1)
	}

	// The meter carries over: the rest of the keys burn it further.
	for _, r := range ui.RoundKeys(a)[n1 : n1+3] {
		clk.advance(200 * time.Millisecond)
		a.Update(key(r))
	}
	if sla2 := ui.RoundFrame(a).SLA; sla2 >= sla1 {
		t.Errorf("SLA in step 2 = %.3f, want below %.3f from step 1", sla2, sla1)
	}
	if got := len(ui.RoundKeys(a)); got != chars(joined(inc)) {
		t.Errorf("one session must hold every step: %d keys, want %d", got, chars(joined(inc)))
	}
}

func TestIncidentTakesTheLanguageOfTheCurrentStep(t *testing.T) {
	a, _, clk := playApp(t)
	_, inc := bossChapter(t, a)
	if inc.Steps[0].Lang != "shell" || inc.Steps[2].Lang != "" {
		t.Fatalf("setup: step languages %q and %q", inc.Steps[0].Lang, inc.Steps[2].Lang)
	}
	startIncident(t, a, inc, incident.SEV3)
	if ui.RoundSpec(a).Lang != "shell" || !strings.Contains(screen(a), "run.sh") {
		t.Errorf("step 1 is shell: spec lang %q\n%s", ui.RoundSpec(a).Lang, screen(a))
	}
	typeKeys(a, clk, len([]rune(inc.Steps[0].Text+"\n"+inc.Steps[1].Text))+2) // into step 3, which is prose
	if v := screen(a); !strings.Contains(v, text.RoundProseFile) {
		t.Errorf("step 3 has no language, so the tab is the prose file:\n%s", v)
	}
}

func TestBreachShowsBlamelessPostmortemWithDrillKey(t *testing.T) {
	a, st, clk := playApp(t)
	ch, inc := bossChapter(t, a)
	readyForBoss(a, st, ch)
	startIncident(t, a, inc, incident.SEV3)
	first, second := breachByErrors(a)

	if got := ui.ScreenName(a); got != "incident-results" {
		t.Fatalf("after the breach: %s, want the postmortem at once", got)
	}
	v := screen(a)
	for _, want := range []string{text.PostmortemTitle, text.PostmortemSummary, text.PostmortemCause,
		hint.Text(a.Layout, first), hint.Text(a.Layout, second)} {
		if !strings.Contains(v, want) {
			t.Errorf("postmortem lacks %q:\n%s", want, v)
		}
	}
	if strings.Index(v, hint.Text(a.Layout, first)) > strings.Index(v, hint.Text(a.Layout, second)) {
		t.Errorf("the most damaged character must come first:\n%s", v)
	}
	if strings.Contains(v, text.IncidentWon) {
		t.Errorf("a lost incident shows the win:\n%s", v)
	}
	runs, _ := st.Runs()
	if len(runs) != 1 || runs[0].Incident == nil || runs[0].Incident.Won {
		t.Fatalf("a lost incident must be one lost run: %+v", runs)
	}
	if g, _ := st.Ghost(engine.Prepare(joined(inc)).Hash); g != nil {
		t.Error("a lost incident stored a ghost")
	}
	if st.Profile().IncidentBest[inc.ID] != "" || st.Profile().Completed[ch.ID] {
		t.Error("a lost incident recorded a win")
	}

	// One key starts a drill on the damaged characters.
	clk.advance(time.Second)
	a.Update(key('d'))
	if ui.ScreenName(a) != "round" {
		t.Fatalf("after d: %s, want a round", ui.ScreenName(a))
	}
	spec := ui.RoundSpec(a)
	if spec.Kind != play.KindDrill || !strings.ContainsRune(spec.Text, first) || !strings.ContainsRune(spec.Text, second) {
		t.Errorf("drill: kind %v text %q, want a drill containing %q and %q", spec.Kind, spec.Text, first, second)
	}
}

func TestBreachByTimeAloneBlamesNobodyAndOffersNoDrill(t *testing.T) {
	a, st, clk := playApp(t)
	ch, inc := bossChapter(t, a)
	readyForBoss(a, st, ch)
	a.Update(tea.FocusMsg{}) // the terminal reports focus, so silence is not an idle pause
	startIncident(t, a, inc, incident.SEV1)
	typeKeys(a, clk, 1)
	for i := 0; i < 300 && ui.ScreenName(a) == "round"; i++ {
		clk.advance(time.Second)
		a.Update(ui.TickMsg(a))
	}
	if ui.ScreenName(a) != "incident-results" {
		t.Fatalf("the SLA never ran out: %s", ui.ScreenName(a))
	}
	v := screen(a)
	if strings.Contains(v, "d drill") {
		t.Errorf("no damage, so no drill key:\n%s", v)
	}
	if !strings.Contains(v, text.PostmortemSlow) || strings.Contains(v, text.PostmortemImpact) {
		t.Errorf("a breach by time must say so and list no characters:\n%s", v)
	}
	clk.advance(time.Second)
	a.Update(key('d'))
	if ui.ScreenName(a) != "incident-results" {
		t.Errorf("d without damaged characters left the postmortem: %s", ui.ScreenName(a))
	}
}

func TestWinShowsMTTRAndSLA(t *testing.T) {
	a, st, clk := playApp(t)
	ch, inc := bossChapter(t, a)
	readyForBoss(a, st, ch)
	startIncident(t, a, inc, incident.SEV2)
	typeKeys(a, clk, -1)
	if ui.ScreenName(a) != "incident-results" {
		t.Fatalf("after the last key: %s", ui.ScreenName(a))
	}
	runs, _ := st.Runs()
	if len(runs) != 1 || runs[0].Incident == nil || !runs[0].Incident.Won {
		t.Fatalf("runs: %+v", runs)
	}
	r := runs[0].Incident
	if r.SLA <= 99 || r.SLA >= 100 {
		t.Fatalf("setup: SLA %.3f should be burnt but not breached", r.SLA)
	}
	v := screen(a)
	for _, want := range []string{
		text.IncidentWon,
		fmt.Sprintf(text.IncidentMTTR, float64(r.MTTRMs)/1000),
		fmt.Sprintf(text.IncidentSLALeft, r.SLA),
		fmt.Sprintf(text.IncidentSevShown, "SEV2"),
		text.IncidentFirst,
	} {
		if !strings.Contains(v, want) {
			t.Errorf("win screen lacks %q:\n%s", want, v)
		}
	}
	if g, _ := st.Ghost(engine.Prepare(joined(inc)).Hash); g == nil {
		t.Error("a won incident must store its ghost")
	}
}

func TestWinComparesWithTheGhostAndIgnoresLostRuns(t *testing.T) {
	a, st, clk := playApp(t)
	ch, inc := bossChapter(t, a)
	readyForBoss(a, st, ch)

	startIncident(t, a, inc, incident.SEV3) // slow win: this is the ghost
	for _, r := range ui.RoundKeys(a) {
		clk.advance(400 * time.Millisecond)
		a.Update(key(r))
	}
	g1, _ := st.Ghost(engine.Prepare(joined(inc)).Hash)
	if g1 == nil {
		t.Fatal("setup: no ghost after the first win")
	}

	startIncident(t, a, inc, incident.SEV3) // a loss, however fast
	breachByErrors(a)
	if ui.ScreenName(a) != "incident-results" {
		t.Fatal("setup: no loss")
	}
	if g, _ := st.Ghost(engine.Prepare(joined(inc)).Hash); g == nil || g.Score != g1.Score {
		t.Errorf("a lost incident replaced the ghost: %+v", g)
	}

	startIncident(t, a, inc, incident.SEV3) // a faster win
	typeKeys(a, clk, -1)
	runs, _ := st.Runs()
	first := runs[0].Incident
	v := screen(a)
	if want := fmt.Sprintf(text.IncidentPrev, "SEV3", float64(first.MTTRMs)/1000, first.SLA); !strings.Contains(v, want) {
		t.Errorf("the comparison must be against the first win, not the loss; want %q:\n%s", want, v)
	}
	if !strings.Contains(v, "duch: o ") || !strings.Contains(v, "před minulým řešením") {
		t.Errorf("no ghost comparison:\n%s", v)
	}
	if g, _ := st.Ghost(engine.Prepare(joined(inc)).Hash); g == nil || g.Score <= g1.Score {
		t.Errorf("a faster win must replace the ghost: %+v vs %+v", g, g1)
	}
}

func TestWinningChapterBossCompletesChapterAndPromotes(t *testing.T) {
	a, st, clk := playApp(t)
	ch, inc := bossAt(t, a, 4) // the first Junior chapter
	readyForBoss(a, st, ch)
	p := st.Profile()
	before := a.Curr.RankOf(p)
	if p.Completed[ch.ID] {
		t.Fatal("setup: chapter already completed")
	}
	startIncident(t, a, inc, incident.SEV2)
	typeKeys(a, clk, -1)

	after := a.Curr.RankOf(p)
	if !p.Completed[ch.ID] || p.IncidentBest[inc.ID] != "SEV2" || after <= before {
		t.Fatalf("completed %v best %q rank %v -> %v", p.Completed[ch.ID], p.IncidentBest[inc.ID], before, after)
	}
	v := screen(a)
	for _, want := range []string{
		fmt.Sprintf(text.IncidentPromoted, ch.Title),
		fmt.Sprintf(text.IncidentRank, before, after),
	} {
		if !strings.Contains(v, want) {
			t.Errorf("win screen lacks %q:\n%s", want, v)
		}
	}
}

func TestIncidentBestKeepsTheHardestSeverity(t *testing.T) {
	a, st, clk := playApp(t)
	ch, inc := bossChapter(t, a)
	readyForBoss(a, st, ch)
	for _, sev := range []incident.Severity{incident.SEV3, incident.SEV1, incident.SEV2} {
		startIncident(t, a, inc, sev)
		typeKeys(a, clk, -1)
		if ui.ScreenName(a) != "incident-results" {
			t.Fatalf("%v: screen %s", sev, ui.ScreenName(a))
		}
	}
	if got := st.Profile().IncidentBest[inc.ID]; got != "SEV1" {
		t.Errorf("IncidentBest = %q, want SEV1 after SEV3, SEV1, SEV2", got)
	}
}

func TestSecondWinOfABossDoesNotPromoteAgain(t *testing.T) {
	a, st, clk := playApp(t)
	ch, inc := bossChapter(t, a)
	readyForBoss(a, st, ch)
	startIncident(t, a, inc, incident.SEV3)
	typeKeys(a, clk, -1)
	startIncident(t, a, inc, incident.SEV3)
	typeKeys(a, clk, -1)
	if v := screen(a); strings.Contains(v, fmt.Sprintf(text.IncidentPromoted, ch.Title)) {
		t.Errorf("a repeated win promoted again:\n%s", v)
	}
}

func TestLosingDoesNotLowerRank(t *testing.T) {
	a, st, clk := playApp(t)
	ch, inc := bossChapter(t, a)
	readyForBoss(a, st, ch)
	p := st.Profile()

	// Lost before the first win: nothing moves.
	rank := a.Curr.RankOf(p)
	starsBefore := fmt.Sprint(p.Stars)
	startIncident(t, a, inc, incident.SEV1)
	breachByErrors(a)
	if ui.ScreenName(a) != "incident-results" {
		t.Fatal("setup: no loss")
	}
	if a.Curr.RankOf(p) != rank || p.Completed[ch.ID] || p.IncidentBest[inc.ID] != "" || fmt.Sprint(p.Stars) != starsBefore {
		t.Errorf("a loss changed progress: rank %v completed %v best %q", a.Curr.RankOf(p), p.Completed[ch.ID], p.IncidentBest[inc.ID])
	}
	if cur := a.Curr.CurrentChapter(p); cur.ID != ch.ID {
		t.Errorf("current chapter = %s, want %s", cur.ID, ch.ID)
	}

	// Lost after a win: the rank and the best severity stay.
	startIncident(t, a, inc, incident.SEV2)
	typeKeys(a, clk, -1)
	won := a.Curr.RankOf(p)
	startIncident(t, a, inc, incident.SEV1)
	breachByErrors(a)
	if a.Curr.RankOf(p) != won || p.IncidentBest[inc.ID] != "SEV2" || !p.Completed[ch.ID] {
		t.Errorf("a loss after a win changed progress: rank %v best %q", a.Curr.RankOf(p), p.IncidentBest[inc.ID])
	}
}

func TestIncidentPathFromHomeToNextChapter(t *testing.T) {
	a, st, clk := playApp(t)
	ch, inc := bossChapter(t, a)
	readyForBoss(a, st, ch)
	p := st.Profile()
	ui.SetScreen(a, ui.NewHomeScreen(a))

	// The owed incident is one Enter away, and so is SEV3.
	a.Update(enter)
	if ui.ScreenName(a) != "severity" {
		t.Fatalf("the recommended incident opens %s, want the severity picker", ui.ScreenName(a))
	}
	a.Update(enter)
	if ui.ScreenName(a) != "round" || ui.RoundSpec(a).Kind != play.KindIncident {
		t.Fatalf("after the picker: %s %v", ui.ScreenName(a), ui.RoundSpec(a).Kind)
	}
	if ui.RoundSpec(a).Title != inc.Title {
		t.Errorf("title = %q, want %q", ui.RoundSpec(a).Title, inc.Title)
	}
	typeKeys(a, clk, -1)
	if ui.ScreenName(a) != "incident-results" {
		t.Fatalf("after the incident: %s", ui.ScreenName(a))
	}
	if v := screen(a); !strings.Contains(v, fmt.Sprintf(text.IncidentPromoted, ch.Title)) || strings.Contains(v, "Hodnost:") {
		t.Errorf("the chapter is closed, but the rank did not change, so the rank line stays out:\n%s", v)
	}
	cur := a.Curr.CurrentChapter(p)
	if cur.ID == ch.ID || cur.Number <= ch.Number {
		t.Fatalf("current chapter = %s, want one after %s", cur.ID, ch.ID)
	}
	next, ok := a.Curr.NextLesson(p)
	if !ok {
		t.Fatal("the next chapter has no lesson to offer")
	}
	clk.advance(time.Second)
	a.Update(enter) // the next pick of the quick run
	if ui.ScreenName(a) != "round" || ui.RoundLessonID(a) != next.ID {
		t.Errorf("next pick: %s lesson %q, want round on %q", ui.ScreenName(a), ui.RoundLessonID(a), next.ID)
	}
}

func TestSeverityPicker(t *testing.T) {
	for _, tc := range []struct {
		downs int
		sev   incident.Severity
	}{{0, incident.SEV3}, {1, incident.SEV2}, {2, incident.SEV1}} {
		t.Run(tc.sev.String(), func(t *testing.T) {
			a, st, _ := playApp(t)
			ch, _ := bossChapter(t, a)
			readyForBoss(a, st, ch)
			for i := range 4 {
				if err := st.AppendRun(store.Run{ID: fmt.Sprint("r", i), At: time.Now(), WPM: 40}); err != nil {
					t.Fatal(err)
				}
			}
			ui.SetScreen(a, ui.NewHomeScreen(a))
			a.Update(enter)
			v := screen(a)
			for _, want := range []string{text.IncidentSeverity, fmt.Sprintf(text.IncidentBaseWPM, 40.0), text.IncidentSev3, text.IncidentSev2, text.IncidentSev1} {
				if !strings.Contains(v, want) {
					t.Errorf("picker lacks %q:\n%s", want, v)
				}
			}
			if !strings.Contains(v, "> "+text.IncidentSev3) {
				t.Errorf("SEV3 must be selected first:\n%s", v)
			}
			for range tc.downs {
				a.Update(down)
			}
			a.Update(enter)
			spec := ui.RoundSpec(a)
			runs, _ := st.Runs()
			want := incident.NewParams(tc.sev, recommend.BaseWPM(runs), chars(spec.Text))
			if spec.Incident == nil || *spec.Incident != want {
				t.Errorf("params = %+v, want %+v", spec.Incident, want)
			}
		})
	}
}

func TestSeverityPickerEscGoesBack(t *testing.T) {
	a, st, _ := playApp(t)
	ch, _ := bossChapter(t, a)
	readyForBoss(a, st, ch)
	ui.SetScreen(a, ui.NewHomeScreen(a))
	a.Update(enter)
	a.Update(esc)
	if ui.ScreenName(a) != "home" {
		t.Errorf("Esc in the picker: %s, want home", ui.ScreenName(a))
	}
}

func TestIncidentPickerListsOnlyUnlocked(t *testing.T) {
	a, st, _ := playApp(t)
	var first, locked content.Incident
	chapters := a.Curr.Chapters()
	for _, ch := range chapters {
		if inc, ok := a.Curr.IncidentDef(ch.ID); ok && ch.Number > 0 {
			if first.ID == "" {
				first = inc
			} else if locked.ID == "" {
				locked = inc
			}
		}
	}
	openFromHome(t, a, homeIncident, "incident-list")
	v := screen(a)
	if !strings.Contains(v, first.ID) || !strings.Contains(v, first.Title) {
		t.Errorf("the first chapter's incident is missing:\n%s", v)
	}
	if strings.Contains(v, locked.ID) || strings.Contains(v, "INC-2001") {
		t.Errorf("a locked incident is listed:\n%s", v)
	}

	// With chapter 11 reached, the free-play extras of that chapter appear.
	for _, ch := range chapters {
		if ch.Number < 11 {
			a.Curr.CompleteChapter(st.Profile(), ch.ID)
		}
	}
	a.Update(esc)
	openFromHome(t, a, homeIncident, "incident-list")
	v = screen(a)
	for _, id := range []string{"INC-1011", "INC-2001", "INC-2002", "INC-2003"} {
		if !strings.Contains(v, id) {
			t.Errorf("%s should be unlocked:\n%s", id, v)
		}
	}
	for _, id := range []string{"INC-1012", "INC-2004"} {
		if strings.Contains(v, id) {
			t.Errorf("%s belongs to a locked chapter:\n%s", id, v)
		}
	}
}

func TestIncidentListScrollsAndOpensThePicker(t *testing.T) {
	a, st, _ := playApp(t)
	for _, ch := range a.Curr.Chapters() {
		a.Curr.CompleteChapter(st.Profile(), ch.ID)
	}
	st.Profile().IncidentBest["INC-2006"] = "SEV1"
	a.Update(tea.WindowSizeMsg{Width: 60, Height: 18})
	openFromHome(t, a, homeIncident, "incident-list")
	a.Update(tea.KeyPressMsg{Code: tea.KeyUp}) // wraps to the last entry
	v := screen(a)
	if !strings.Contains(v, "> INC-2006") || !strings.Contains(v, "SEV1") {
		t.Errorf("the last entry must be visible and selected at 60 x 18:\n%s", v)
	}
	if lines := strings.Count(v, "\n") + 1; lines > 18 {
		t.Errorf("the list overflows the window: %d lines", lines)
	}
	a.Update(enter)
	if ui.ScreenName(a) != "severity" {
		t.Fatalf("Enter on a list entry: %s, want the severity picker", ui.ScreenName(a))
	}
	a.Update(enter)
	if spec := ui.RoundSpec(a); spec.Kind != play.KindIncident || spec.Title != "undefined is not a function" {
		t.Fatalf("the picked incident did not start: kind %v title %q", spec.Kind, spec.Title)
	}
}

func TestIncidentPausedByFocusLoss(t *testing.T) {
	a, _, clk := playApp(t)
	_, inc := bossChapter(t, a)
	startIncident(t, a, inc, incident.SEV3)
	typeKeys(a, clk, 3)
	before := ui.RoundFrame(a).SLA
	a.Update(tea.BlurMsg{})
	clk.advance(10 * time.Minute)
	a.Update(ui.TickMsg(a))
	f := ui.RoundFrame(a)
	if !f.Paused || f.Breached || f.SLA != before {
		t.Fatalf("paused %v breached %v SLA %.4f, want paused, alive and %.4f", f.Paused, f.Breached, f.SLA, before)
	}
	a.Update(key(ui.RoundKeys(a)[3])) // the next key resumes
	clk.advance(2 * time.Second)
	a.Update(ui.TickMsg(a))
	if after := ui.RoundFrame(a).SLA; after >= before {
		t.Errorf("SLA after resuming = %.4f, want it burning again below %.4f", after, before)
	}
}

func TestIncidentLeaveSavesNothingAsARun(t *testing.T) {
	a, st, clk := playApp(t)
	_, inc := bossChapter(t, a)
	startIncident(t, a, inc, incident.SEV3)
	typeKeys(a, clk, 3)
	a.Update(esc)
	a.Update(down)
	a.Update(down)
	a.Update(enter) // leave
	if ui.ScreenName(a) != "home" {
		t.Fatalf("after leaving: %s", ui.ScreenName(a))
	}
	if runs, _ := st.Runs(); len(runs) != 0 {
		t.Errorf("an abandoned incident became a run: %+v", runs)
	}
}

func TestPreviousBestIgnoresLostIncidentRuns(t *testing.T) {
	a, st, clk := playApp(t)
	ch, inc := bossChapter(t, a)
	readyForBoss(a, st, ch)
	// A lost run with a dazzling score on the very same text.
	if err := st.AppendRun(store.Run{
		ID: "lost001", At: clk.t, TextHash: engine.Prepare(joined(inc)).Hash, Score: 9999, WPM: 20,
		Incident: &store.IncidentResult{Won: false, SLA: 99, MTTRMs: 1000},
	}); err != nil {
		t.Fatal(err)
	}
	startIncident(t, a, inc, incident.SEV3)
	typeKeys(a, clk, -1)
	if v := screen(a); !strings.Contains(v, text.IncidentFirst) || strings.Contains(v, "minule") {
		t.Errorf("a lost run was taken for the previous best:\n%s", v)
	}
}

// prompt of step i, shortened to what must survive on screen
func lastWords(s string, n int) string {
	w := strings.Fields(s)
	return strings.Join(w[max(0, len(w)-n):], " ")
}

// readStep lets the player read the prompt of the step at pos before typing:
// 20 characters a second, with a tick every half second.
func readStep(a *ui.App, clk *clock, prompt string) {
	for left := time.Duration(len([]rune(prompt))) * time.Second / 20; left > 0; left -= 500 * time.Millisecond {
		clk.advance(min(left, 500*time.Millisecond))
		a.Update(ui.TickMsg(a))
	}
}

// playReading types the incident at pace, reading every prompt but the first.
func playReading(a *ui.App, clk *clock, inc content.Incident, perKey time.Duration) {
	keys := ui.RoundKeys(a)
	starts := map[int]string{}
	at := 0
	for i, st := range inc.Steps {
		if i > 0 {
			starts[at] = st.Prompt
		}
		at += len([]rune(st.Text)) + 1
	}
	for i, r := range keys {
		if p, ok := starts[i]; ok {
			readStep(a, clk, p)
		}
		clk.advance(perKey)
		a.Update(key(r))
	}
}

func slowTypist(t *testing.T, st *store.Store, clk *clock, wpm float64) {
	t.Helper()
	for i := range 4 {
		if err := st.AppendRun(store.Run{ID: fmt.Sprint("w", i), At: clk.t, WPM: wpm}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReadingThePromptsDoesNotCostTheIncident(t *testing.T) {
	a, st, clk := playApp(t)
	ch, inc := bossAt(t, a, 2)
	readyForBoss(a, st, ch)
	slowTypist(t, st, clk, 30)
	a.Update(tea.FocusMsg{})
	startIncident(t, a, inc, incident.SEV3)
	playReading(a, clk, inc, 400*time.Millisecond) // 30 wpm
	if ui.ScreenName(a) != "incident-results" || !strings.Contains(screen(a), text.IncidentWon) {
		t.Fatalf("a 30 wpm typist who reads lost INC-1002:\n%s", screen(a))
	}
	runs, _ := st.Runs()
	last := runs[len(runs)-1]
	if realMs := last.DurationMs; realMs < 20000 {
		t.Errorf("the MTTR must stay real active time, got %d ms", realMs)
	}
}

func TestLongPromptReadingDoesNotTriggerIdlePause(t *testing.T) {
	a, st, clk := playApp(t) // no focus reporting
	ch, inc := bossAt(t, a, 2)
	readyForBoss(a, st, ch)
	slowTypist(t, st, clk, 30)
	if len([]rune(inc.Steps[2].Prompt)) < 100 {
		t.Fatalf("setup: prompt is %d characters", len([]rune(inc.Steps[2].Prompt)))
	}
	startIncident(t, a, inc, incident.SEV3)
	playReading(a, clk, inc, 400*time.Millisecond)
	runs, _ := st.Runs()
	if len(runs) == 0 {
		t.Fatalf("no run recorded:\n%s", screen(a))
	}
	if last := runs[len(runs)-1]; last.Paused {
		t.Error("reading a prompt marked the run paused")
	}
	if g, _ := st.Ghost(engine.Prepare(joined(inc)).Hash); g == nil {
		t.Error("the win lost its ghost to a retroactive idle pause")
	}
}

func TestKeysAreIgnoredRightAfterTheEnding(t *testing.T) {
	for _, name := range []string{"loss", "win"} {
		t.Run(name, func(t *testing.T) {
			a, st, clk := playApp(t)
			ch, inc := bossChapter(t, a)
			readyForBoss(a, st, ch)
			startIncident(t, a, inc, incident.SEV3)
			if name == "loss" {
				breachByErrors(a)
			} else {
				typeKeys(a, clk, -1)
			}
			if ui.ScreenName(a) != "incident-results" {
				t.Fatalf("setup: %s", ui.ScreenName(a))
			}
			clk.advance(600 * time.Millisecond)
			a.Update(key('d'))
			a.Update(enter)
			a.Update(esc)
			if ui.ScreenName(a) != "incident-results" {
				t.Fatalf("typing ran on into the ending: %s after 600 ms", ui.ScreenName(a))
			}
			clk.advance(200 * time.Millisecond) // 800 ms in
			a.Update(esc)
			if ui.ScreenName(a) != "home" {
				t.Errorf("after 800 ms Esc must work: %s", ui.ScreenName(a))
			}
		})
	}
}

func TestRunRecordsTheSeverity(t *testing.T) {
	for _, won := range []bool{true, false} {
		a, st, clk := playApp(t)
		ch, inc := bossChapter(t, a)
		readyForBoss(a, st, ch)
		startIncident(t, a, inc, incident.SEV2)
		if won {
			typeKeys(a, clk, -1)
		} else {
			breachByErrors(a)
		}
		runs, _ := st.Runs()
		if len(runs) != 1 || runs[0].Incident == nil || runs[0].Incident.Severity != "SEV2" || runs[0].Incident.Won != won {
			t.Errorf("won=%v: run %+v", won, runs)
		}
	}
}

func TestFreePlayWinBeforeTheLessonsClosesNothing(t *testing.T) {
	a, st, clk := playApp(t)
	ch, inc := bossAt(t, a, 4)
	p := st.Profile()
	before := a.Curr.RankOf(p)
	startIncident(t, a, inc, incident.SEV3) // lessons of the chapter are unplayed
	typeKeys(a, clk, -1)
	if ui.ScreenName(a) != "incident-results" || !strings.Contains(screen(a), text.IncidentWon) {
		t.Fatalf("setup: %s", screen(a))
	}
	if p.Completed[ch.ID] || a.Curr.IsDone(p, ch.ID) || a.Curr.RankOf(p) != before {
		t.Errorf("a free-play win closed the chapter: completed %v rank %v", p.Completed[ch.ID], a.Curr.RankOf(p))
	}
	if strings.Contains(screen(a), "uzavřena") {
		t.Errorf("a promotion line for a chapter that stays open:\n%s", screen(a))
	}
	if p.IncidentBest[inc.ID] != "SEV3" {
		t.Errorf("IncidentBest = %q, want it recorded", p.IncidentBest[inc.ID])
	}
	if runs, _ := st.Runs(); len(runs) != 1 {
		t.Errorf("runs = %d, want the win counted", len(runs))
	}
}

func TestWinFitsASixtyByEighteenWindow(t *testing.T) {
	a, st, clk := playApp(t)
	ch, inc := bossAt(t, a, 4)
	readyForBoss(a, st, ch)
	a.Update(tea.WindowSizeMsg{Width: 60, Height: 18})
	startIncident(t, a, inc, incident.SEV3)
	typeKeys(a, clk, -1)
	v := screen(a)
	for _, want := range []string{"nepanikaří.", "HR byla informována."} {
		if !strings.Contains(v, want) {
			t.Errorf("the win screen cuts its text at 60 x 18; lacks %q:\n%s", want, v)
		}
	}
}

func TestPostmortemInATinyWindowStillNamesADamagedCharacter(t *testing.T) {
	a, st, _ := playApp(t)
	ch, inc := bossChapter(t, a)
	readyForBoss(a, st, ch)
	a.Update(tea.WindowSizeMsg{Width: 40, Height: 8})
	startIncident(t, a, inc, incident.SEV3)
	a.Update(tea.WindowSizeMsg{Width: 80, Height: 24}) // typing needs room
	first, _ := breachByErrors(a)
	a.Update(tea.WindowSizeMsg{Width: 40, Height: 8})
	if v := screen(a); !strings.Contains(v, hint.Text(a.Layout, first)) || !strings.Contains(v, "d drill") {
		t.Errorf("at 40 x 8 the damaged character %q and the drill key must show:\n%s", first, v)
	}
}

func TestLongPromptIsNotCutAtSixtyByEighteen(t *testing.T) {
	for _, id := range []style.ID{style.Editor, style.Fosfor, style.Velin} {
		t.Run(string(id), func(t *testing.T) {
			a, st, clk := playApp(t)
			ch, inc := bossAt(t, a, 2)
			readyForBoss(a, st, ch)
			a.Style = style.ByID(id)
			a.Update(tea.WindowSizeMsg{Width: 60, Height: 18})
			startIncident(t, a, inc, incident.SEV3)
			typeKeys(a, clk, len([]rune(inc.Steps[0].Text+"\n"+inc.Steps[1].Text))+2) // into step 3
			p := inc.Steps[2].Prompt
			if len([]rune(p)) < 100 {
				t.Fatalf("setup: prompt is %d characters", len([]rune(p)))
			}
			v := screen(a)
			if !strings.Contains(v, lastWords(p, 1)) {
				t.Errorf("the prompt is cut (want its last word %q):\n%s", lastWords(p, 1), v)
			}
		})
	}
}

func TestLostBossLeadsToADrillThenTheBossAgain(t *testing.T) {
	a, st, clk := playApp(t)
	boss, inc := bossChapter(t, a)
	readyForBoss(a, st, boss)
	ui.SetScreen(a, ui.NewHomeScreen(a))
	a.Update(enter)
	wantScreen(t, a, "severity")
	a.Update(enter)
	wantScreen(t, a, "round")
	typeKeys(a, clk, 3)
	breachByErrors(a)
	wantScreen(t, a, "incident-results")
	clk.advance(time.Second)
	a.Update(enter)
	wantScreen(t, a, "round")
	if k := ui.RoundSpec(a).Kind; k != play.KindDrill {
		t.Fatalf("after the loss: round kind %v, want a drill on the weakest characters", k)
	}
	typeKeys(a, clk, -1)
	wantScreen(t, a, "results")
	a.Update(enter)
	wantScreen(t, a, "severity")
	wantText(t, a, inc.ID)
}

// The typist the tuning is for: 92 %, the accuracy of one star, at the usual
// pace, on the longest boss.
func TestTypistAtTheOneStarAccuracyWinsTheLongestBoss(t *testing.T) {
	a, st, clk := typist(t, 40, 12)
	_, inc := bossAt(t, a, 12)
	startIncident(t, a, inc, incident.SEV3)
	keys := ui.RoundKeys(a)
	gap := time.Duration(12.0 / 40 * float64(time.Second))
	misses := 0
	for i, r := range keys {
		clk.advance(gap)
		if i%25 == 12 || i%25 == 24 { // 2 keys in 25 are wrong first: 92 %
			misses++
			a.Update(wrongFor(r))
		}
		a.Update(key(r))
	}
	wantScreen(t, a, "incident-results")
	runs, _ := st.Runs()
	last := runs[len(runs)-1]
	if last.Incident == nil || !last.Incident.Won {
		t.Fatalf("%d misses in %d keys lost the incident: %+v", misses, len(keys), last.Incident)
	}
	if last.Accuracy > 0.93 {
		t.Errorf("accuracy = %.3f, the test is about 92 %%", last.Accuracy)
	}
}

// The boss is won in free play first, and the last lesson's star closes the
// chapter later. That must be as loud as winning last.
func TestLastLessonAfterAnEarlierWinClosesTheChapterAloud(t *testing.T) {
	a, st, clk := playApp(t)
	boss, inc := bossAt(t, a, 4) // the first chapter that earns Junior
	readyForBoss(a, st, boss)
	p := st.Profile()
	p.IncidentBest[inc.ID] = "SEV3"
	last := boss.Lessons[len(boss.Lessons)-1]
	delete(p.Stars, last.ID)
	if a.Curr.IsDone(p, boss.ID) {
		t.Fatal("precondition: the chapter is still open")
	}
	ui.SetScreen(a, ui.NewRoundScreen(a, recommend.Pick{Kind: recommend.Lesson, LessonID: last.ID}, nil))
	typeKeys(a, clk, -1)
	wantScreen(t, a, "results")
	wantText(t, a, fmt.Sprintf(text.IncidentPromoted, boss.Title))
	wantText(t, a, fmt.Sprintf(text.IncidentRank, "Intern", "Junior"))
	if !p.Completed[boss.ID] {
		t.Error("the chapter is done but Completed is not set")
	}
}

func TestWinShowsNoStarsThatNothingKeeps(t *testing.T) {
	a, _, clk := playApp(t)
	_, inc := bossChapter(t, a)
	startIncident(t, a, inc, incident.SEV3)
	for _, r := range ui.RoundKeys(a) {
		clk.advance(100 * time.Millisecond)
		a.Update(key(r))
	}
	wantScreen(t, a, "incident-results")
	if v := screen(a); strings.ContainsAny(v, "★☆") {
		t.Errorf("an incident has no lesson to keep stars:\n%s", v)
	}
}

func TestCareerRowForTheOwedBoss(t *testing.T) {
	a, st, clk := playApp(t)
	tall(a)
	boss, inc := bossChapter(t, a)
	openFromHome(t, a, homeCareer, "career")
	if strings.Contains(view(a), inc.ID) {
		t.Fatalf("the boss is not owed yet:\n%s", view(a))
	}
	a.Update(esc)
	readyForBoss(a, st, boss)
	openFromHome(t, a, homeCareer, "career")
	row := selectedLine(a)
	if !strings.Contains(row, inc.ID) || !strings.Contains(row, inc.Title) {
		t.Fatalf("the owed boss is not the selected row, got %q:\n%s", row, view(a))
	}
	a.Update(enter)
	wantScreen(t, a, "severity")
	a.Update(enter)
	wantScreen(t, a, "round")
	if got := ui.RoundSpec(a); got.Title != inc.Title {
		t.Fatalf("round %q, want the boss", got.Title)
	}
	for _, r := range ui.RoundKeys(a) {
		clk.advance(100 * time.Millisecond)
		a.Update(key(r))
	}
	wantScreen(t, a, "incident-results")
	clk.advance(time.Second)
	a.Update(enter)
	wantScreen(t, a, "career")
	if strings.Contains(view(a), inc.ID) {
		t.Errorf("a won boss has no row:\n%s", view(a))
	}
}

func TestPracticeListsNoBossRow(t *testing.T) {
	a, st, _ := playApp(t)
	tall(a)
	boss, inc := bossChapter(t, a)
	readyForBoss(a, st, boss)
	openPractice(t, a, practiceLesson)
	if strings.Contains(view(a), inc.ID) {
		t.Errorf("the practice list offers the boss:\n%s", view(a))
	}
}

func TestHomeNamesTheOwedBoss(t *testing.T) {
	a, st, _ := playApp(t)
	boss, inc := bossChapter(t, a)
	readyForBoss(a, st, boss)
	ui.SetScreen(a, ui.NewHomeScreen(a))
	wantText(t, a, inc.ID)
}
