package ui

import (
	"fmt"
	"image/color"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jiripisa/tuipe/internal/content"
	"github.com/jiripisa/tuipe/internal/engine"
	"github.com/jiripisa/tuipe/internal/hint"
	"github.com/jiripisa/tuipe/internal/incident"
	"github.com/jiripisa/tuipe/internal/play"
	"github.com/jiripisa/tuipe/internal/recommend"
	"github.com/jiripisa/tuipe/internal/store"
	"github.com/jiripisa/tuipe/internal/ui/style"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

// severities in the order the picker offers them.
var severities = []incident.Severity{incident.SEV3, incident.SEV2, incident.SEV1}

// story is an incident being played: its steps lie in one text, one newline
// apart, so the position of the cursor tells the step.
type story struct {
	inc    content.Incident
	sev    incident.Severity
	starts []int // index of the first cell of each step
}

func newStory(inc content.Incident, sev incident.Severity) *story {
	s := &story{inc: inc, sev: sev}
	at := 0
	for _, st := range inc.Steps {
		s.starts = append(s.starts, at)
		at += len([]rune(st.Text)) + 1 // the Enter that ends the step
	}
	return s
}

// readingCPS is how fast a prompt is read, in characters a second.
const readingCPS = 20

// graces are the reading times the meter is held back for: at the cell where
// each step but the first begins, the length of its prompt over readingCPS.
func (s *story) graces() map[int]time.Duration {
	out := map[int]time.Duration{}
	for i := 1; i < len(s.starts); i++ {
		n := len([]rune(s.inc.Steps[i].Prompt))
		out[s.starts[i]] = time.Duration(n) * time.Second / readingCPS
	}
	return out
}

func joinSteps(inc content.Incident) string {
	parts := make([]string, len(inc.Steps))
	for i, st := range inc.Steps {
		parts[i] = st.Text
	}
	return strings.Join(parts, "\n")
}

// step is the index of the step the cursor is in. The Enter that ends a step
// still belongs to it.
func (s *story) step(pos int) int {
	i := 0
	for j, start := range s.starts {
		if start <= pos {
			i = j
		}
	}
	return i
}

// view is the prompt, language and right-hand info of the round screen.
func (s *story) view(pos int, active time.Duration) (prompt, lang, info string) {
	i := s.step(pos)
	secs := int(active.Seconds())
	return s.inc.Steps[i].Prompt, s.inc.Steps[i].Lang,
		fmt.Sprintf(text.RoundIncidentInfo, s.inc.ID, i+1, len(s.inc.Steps), secs/60, secs%60)
}

func (a *App) incidentByID(id string) (content.Incident, bool) {
	i := slices.IndexFunc(a.Lib.Incidents, func(inc content.Incident) bool { return inc.ID == id })
	if i < 0 {
		return content.Incident{}, false
	}
	return a.Lib.Incidents[i], true
}

// incidentSpecBase is a Spec holding only the Targets and HintsOff of an
// incident: the targets of the last lesson of its chapter, or of the nearest
// earlier chapter that has lessons.
func (a *App) incidentSpecBase(chapterID string) (targets play.Spec) {
	chs := a.Curr.Chapters()
	i := slices.IndexFunc(chs, func(c content.ChapterDef) bool { return c.ID == chapterID })
	if i < 0 {
		targets.Targets = a.chapterTargets()
		return targets
	}
	targets.HintsOff = chs[i].Number == hintsOffChapter
	for j := i; j >= 0; j-- {
		if n := len(chs[j].Lessons); n > 0 {
			targets.Targets = a.Curr.Targets(chs[j].Lessons[n-1].ID)
			return targets
		}
	}
	return targets
}

// incidentSpec builds the round of an incident: all steps as one text, the
// SLA calibrated to the player's recent pace, the replay ghost of the best win.
func (a *App) incidentSpec(inc content.Incident, sev incident.Severity) (play.Spec, error) {
	if len(inc.Steps) == 0 {
		return play.Spec{}, fmt.Errorf("incident %q has no steps", inc.ID)
	}
	txt := joinSteps(inc)
	keys := 0
	for _, c := range engine.Prepare(txt).Cells {
		if !c.Auto {
			keys++
		}
	}
	runs, _ := a.Store.Runs() // a damaged history only affects the calibration
	params := incident.NewParams(sev, recommend.BaseWPM(runs), keys)
	spec := a.incidentSpecBase(inc.Chapter)
	spec.Kind, spec.Title, spec.Lang, spec.Text = play.KindIncident, inc.Title, inc.Steps[0].Lang, txt
	spec.Incident = &params
	spec.Severity = sev.String()
	spec.Graces = newStory(inc, sev).graces()
	return a.withReplay(spec)
}

// NewIncidentScreen builds the round of an incident at the given severity. When
// it cannot be built it returns the home screen, with a warning on the status line.
func NewIncidentScreen(a *App, inc content.Incident, sev incident.Severity, next func(*App) Screen) Screen {
	spec, err := a.incidentSpec(inc, sev)
	if err != nil {
		a.SaveErr(err)
		return NewHomeScreen(a)
	}
	r := &roundScreen{spec: spec, next: next, story: newStory(inc, sev)}
	r.begin(a)
	return r
}

// severityScreen asks how hard the incident should be.
type severityScreen struct {
	inc  content.Incident
	sel  int
	next func(*App) Screen // after the incident
	back func(*App) Screen // where Esc goes; the home screen when nil
}

// NewSeverityScreen offers the severities of an incident, SEV3 first.
func NewSeverityScreen(a *App, inc content.Incident, next, back func(*App) Screen) Screen {
	return &severityScreen{inc: inc, next: next, back: back}
}

func (s *severityScreen) Update(msg tea.Msg, a *App) (Screen, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return s, nil
	}
	n := len(severities)
	switch k.String() {
	case "up", "k":
		s.sel = (s.sel + n - 1) % n
	case "down", "j":
		s.sel = (s.sel + 1) % n
	case "esc":
		if s.back != nil {
			return s.back(a), nil
		}
		return NewHomeScreen(a), nil
	case "enter":
		round := NewIncidentScreen(a, s.inc, severities[s.sel], s.next)
		if r, ok := round.(*roundScreen); ok && s.back != nil { // picked from the incident list
			r.spec.Session = sessionCareer
		}
		return round, nil
	}
	return s, nil
}

func (s *severityScreen) View(a *App, w, h int) string {
	st := a.Style
	runs, _ := a.Store.Runs()
	labels := []string{text.IncidentSev3, text.IncidentSev2, text.IncidentSev1}
	lines := []string{
		st.Base().Foreground(st.Fg).Bold(true).Render(s.inc.ID + "  " + s.inc.Title),
		st.Base().Foreground(st.Dim).Render(fmt.Sprintf(text.IncidentBaseWPM, recommend.BaseWPM(runs))),
		"",
	}
	for i, l := range labels {
		if i == s.sel {
			lines = append(lines, st.Base().Foreground(st.Accent).Bold(true).Render("> "+l))
		} else {
			lines = append(lines, st.Base().Foreground(st.Fg).Render("  "+l))
		}
	}
	return style.Frame(st, w, h, text.AppName+" · "+text.IncidentSeverity, strings.Join(lines, "\n"), text.HintIncident)
}

// incidentListScreen lists the incidents the player may start: chapter bosses
// and free-play extras of unlocked chapters.
type incidentListScreen struct{ sel int }

// NewIncidentListScreen builds the list opened by the home menu.
func NewIncidentListScreen(a *App) Screen { return &incidentListScreen{} }

func (l *incidentListScreen) entries(a *App) []content.Incident {
	p := a.Store.Profile()
	var out []content.Incident
	for _, inc := range a.Lib.Incidents {
		if a.Curr.IsUnlocked(p, inc.Chapter) && len(a.Layout.Unresolved(joinSteps(inc))) == 0 {
			out = append(out, inc)
		}
	}
	return out
}

func (l *incidentListScreen) Update(msg tea.Msg, a *App) (Screen, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return l, nil
	}
	list := l.entries(a)
	n := len(list)
	switch k.String() {
	case "up", "k":
		l.sel--
	case "down", "j":
		l.sel++
	case "esc":
		return NewHomeScreen(a), nil
	case "enter":
		if n > 0 {
			again := func(*App) Screen { return l }
			return NewSeverityScreen(a, list[(l.sel%n+n)%n], again, again), nil
		}
	}
	if n > 0 {
		l.sel = (l.sel%n + n) % n
	}
	return l, nil
}

func (l *incidentListScreen) View(a *App, w, h int) string {
	st := a.Style
	p := a.Store.Profile()
	list := l.entries(a)
	if len(list) == 0 {
		body := st.Base().Foreground(st.Dim).Render(text.IncidentListEmpty)
		return style.Frame(st, w, h, text.AppName+" · "+text.IncidentTitle, body, text.HintIncident)
	}
	var lines []string
	for i, inc := range list {
		best := text.IncidentListNone
		if b := p.IncidentBest[inc.ID]; b != "" {
			best = fmt.Sprintf(text.IncidentListBest, b)
		}
		row := ansi.Truncate(inc.ID+"  "+inc.Title, max(w-4-len([]rune(best))-1, 1), "…")
		row += strings.Repeat(" ", max(w-4-ansi.StringWidth(row)-len([]rune(best)), 1)) + best
		if i == l.sel {
			lines = append(lines, st.Base().Foreground(st.Accent).Bold(true).Render("> "+row))
		} else {
			lines = append(lines, st.Base().Foreground(st.Fg).Render("  "+row))
		}
	}
	if avail := h - 3; avail > 0 && len(lines) > avail {
		top := min(max(l.sel-avail/2, 0), len(lines)-avail)
		lines = lines[top : top+avail]
	}
	return style.Frame(st, w, h, text.AppName+" · "+text.IncidentTitle, strings.Join(lines, "\n"), text.HintIncident)
}

// parseSeverity reads a stored severity.
func parseSeverity(name string) (incident.Severity, bool) {
	for _, s := range severities {
		if s.String() == name {
			return s, true
		}
	}
	return 0, false
}

// harderOf is the stored severity to keep: the harder of the stored one and sev.
func harderOf(stored string, sev incident.Severity) string {
	if old, ok := parseSeverity(stored); ok && old > sev {
		return stored
	}
	return sev.String()
}

// incidentResultsScreen is the win screen, or the postmortem of a lost incident.
type incidentResultsScreen struct {
	res      play.Result
	spec     play.Spec
	story    *story
	prev     *store.Run
	next     func(*App) Screen
	promoted []string // dry promotion lines, when the chapter just completed
	focus    []rune   // damaged characters a drill can train
	shownAt  time.Time
}

// endingLock is how long keys are ignored after the ending appears, so that
// typing that runs on does not skip the postmortem.
const endingLock = 700 * time.Millisecond

// NewIncidentResultsScreen records the finished incident and shows it. A win
// records the best severity and completes the chapter whose boss it is; a loss
// records only the run and the keystrokes.
func NewIncidentResultsScreen(a *App, r play.Result, spec play.Spec, st *story, next func(*App) Screen) Screen {
	if r.Incident == nil { // not an incident result: treat it as lost without damage
		r.Incident = &play.IncidentOutcome{}
	}
	s := &incidentResultsScreen{res: r, spec: spec, story: st, next: next, shownAt: a.Now()}
	s.prev = a.previousBest(r.Text.Hash) // before Finish adds this run
	if r.Incident != nil && r.Incident.Won {
		s.promoted = a.recordIncidentWin(st)
	} else {
		for _, c := range r.Incident.DamageChars {
			if c != '\n' && c != ' ' {
				s.focus = append(s.focus, c)
			}
		}
	}
	a.Finish(spec, r) // Finish has shown any save error
	return s
}

// recordIncidentWin keeps the hardest severity won and completes the chapter
// the incident ends. It returns the promotion lines when the chapter has just
// been completed. The caller saves the profile.
func (a *App) recordIncidentWin(st *story) []string {
	p := a.Store.Profile()
	boss, isBoss := a.Curr.IncidentDef(st.inc.Chapter)
	isBoss = isBoss && boss.ID == st.inc.ID
	firstWin := isBoss && !p.Completed[st.inc.Chapter] && p.IncidentBest[st.inc.ID] == ""
	before := a.Curr.RankOf(p)
	p.IncidentBest[st.inc.ID] = harderOf(p.IncidentBest[st.inc.ID], st.sev)
	if !isBoss || !a.lessonsDone(st.inc.Chapter) {
		return nil // a free-play win is recorded but closes nothing
	}
	a.Curr.CompleteChapter(p, st.inc.Chapter)
	if !firstWin {
		return nil
	}
	lines := []string{fmt.Sprintf(text.IncidentPromoted, a.chapterTitle(st.inc.Chapter))}
	if after := a.Curr.RankOf(p); after != before {
		lines = append(lines, fmt.Sprintf(text.IncidentRank, before, after))
	}
	return lines
}

// lessonsDone reports whether every lesson of a chapter has a star.
func (a *App) lessonsDone(chapterID string) bool {
	p := a.Store.Profile()
	for _, ch := range a.Curr.Chapters() {
		if ch.ID != chapterID {
			continue
		}
		for _, l := range ch.Lessons {
			if p.Stars[l.ID] == 0 {
				return false
			}
		}
	}
	return true
}

func (a *App) chapterTitle(id string) string {
	for _, ch := range a.Curr.Chapters() {
		if ch.ID == id {
			return ch.Title
		}
	}
	return id
}

func (s *incidentResultsScreen) won() bool { return s.res.Incident.Won }

func (s *incidentResultsScreen) Update(msg tea.Msg, a *App) (Screen, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok || a.Now().Sub(s.shownAt) < endingLock {
		return s, nil
	}
	switch k.String() {
	case "enter":
		if s.next == nil {
			return NewHomeScreen(a), nil
		}
		return s.next(a), nil
	case "esc":
		return NewHomeScreen(a), nil
	case "d":
		if !s.won() && len(s.focus) > 0 {
			return s.drill(a), nil
		}
	}
	return s, nil
}

// drill trains the damaged characters with everything their chapter unlocked.
func (s *incidentResultsScreen) drill(a *App) Screen {
	unlocked := a.Curr.AllowedAt(s.story.inc.Chapter)
	allowed := func(r rune) bool {
		return (unlocked(r) || slices.Contains(s.focus, r)) && a.Curr.CanType(r)
	}
	spec := a.generatedSpec(fmt.Sprintf(text.PostmortemDrill, string(s.focus)), s.focus, allowed, s.spec.Targets)
	return newRound(a, spec, s.next)
}

func (s *incidentResultsScreen) View(a *App, w, h int) string {
	var body []string
	hintLine := text.HintIncidentWon
	if s.won() {
		body = s.winLines(a, w-4, h-3)
	} else {
		body = s.postmortemLines(a, w-4, h-3)
		hintLine = text.HintPostmortemBare
		if len(s.focus) > 0 {
			hintLine = text.HintPostmortem
		}
	}
	return style.Frame(a.Style, w, h, text.AppName+" · "+s.story.inc.ID, strings.Join(body, "\n"), hintLine)
}

// painted wraps a line to width and paints every row.
func painted(st style.Style, col color.Color, bold bool, line string, width int) []string {
	rows := strings.Split(ansi.Wordwrap(line, max(width, 10), ""), "\n")
	for i, r := range rows {
		rows[i] = st.Base().Foreground(col).Bold(bold).Render(r)
	}
	return rows
}

func (s *incidentResultsScreen) winLines(a *App, width, rows int) []string {
	if lines := s.buildWin(a, width, true); len(lines) <= rows {
		return lines
	}
	return s.buildWin(a, width, false) // the blank rows go first
}

func (s *incidentResultsScreen) buildWin(a *App, width int, blank bool) []string {
	st := a.Style
	var lines []string
	add := func(col color.Color, line string) {
		if line == "" && !blank {
			return
		}
		lines = append(lines, painted(st, col, false, line, width)...)
	}
	o := s.res.Incident
	lines = painted(st, st.OK, true, text.IncidentWon, width)
	add(st.Dim, s.story.inc.ID+"  "+s.story.inc.Title)
	add(st.Fg, "")
	add(st.Fg, fmt.Sprintf(text.IncidentMTTR, o.MTTR.Seconds()))
	add(st.Fg, fmt.Sprintf(text.IncidentSLALeft, o.SLA))
	add(st.Fg, fmt.Sprintf(text.IncidentSevShown, s.story.sev))
	add(st.Fg, "")
	if s.prev != nil && s.prev.Incident != nil {
		add(st.Dim, fmt.Sprintf(text.IncidentPrev, s.prev.Incident.Severity, float64(s.prev.Incident.MTTRMs)/1000, s.prev.Incident.SLA))
	} else {
		add(st.Dim, text.IncidentFirst)
	}
	if d := s.res.GhostDelta; d != nil {
		if *d >= 0 {
			add(st.OK, fmt.Sprintf(text.IncidentAhead, d.Seconds()))
		} else {
			add(st.Bad, fmt.Sprintf(text.IncidentBehind, -d.Seconds()))
		}
	}
	n := max(0, min(s.res.Stars, maxStars))
	add(st.Fg, "")
	add(st.Accent, text.ResultsStars+"  "+starBar(n))
	if s.res.Paused {
		add(st.Dim, text.ResultsPaused)
	}
	if s.res.HintUsed {
		add(st.Dim, text.ResultsHintUsed)
	}
	if len(s.promoted) > 0 {
		add(st.Fg, "")
		for _, l := range s.promoted {
			add(st.Accent, l)
		}
	}
	return lines
}

// postmortemLines is the blameless postmortem: what happened, which characters
// were damaged and how to type them, the cause and the follow-up. In a short
// window it gives up context first (blank rows, the incident, the summary,
// the cause, the heading, the follow-up), never the damaged characters.
func (s *incidentResultsScreen) postmortemLines(a *App, width, rows int) []string {
	st := a.Style
	para := func(col color.Color, line string) []string { return painted(st, col, false, line, width) }
	title := painted(st, st.Bad, true, text.PostmortemTitle, width)
	id := para(st.Dim, s.story.inc.ID+"  "+s.story.inc.Title)
	summary := para(st.Fg, text.PostmortemSummary)
	var header, chars, cause, action []string
	if damaged := s.res.Incident.DamageChars; len(damaged) > 0 {
		header = para(st.Fg, text.PostmortemImpact)
		for _, r := range damaged {
			t := hint.Text(a.Layout, r)
			if t == "" {
				t = string(r)
			}
			chars = append(chars, para(st.Accent, "  "+t)...)
		}
		cause = para(st.Fg, text.PostmortemCause)
	} else {
		cause = para(st.Fg, text.PostmortemSlow)
	}
	if len(s.focus) > 0 {
		action = para(st.Fg, text.PostmortemAction)
	}
	blank := []string{""}
	join := func(parts ...[]string) []string {
		var out []string
		for _, p := range parts {
			out = append(out, p...)
		}
		return out
	}
	options := [][]string{
		join(title, id, blank, summary, blank, header, chars, blank, cause, action),
		join(title, id, summary, header, chars, cause, action),
		join(title, summary, header, chars, cause, action),
		join(title, header, chars, cause, action),
		join(title, header, chars, action),
		join(title, chars, action),
		join(title, chars),
	}
	for _, o := range options {
		if len(o) <= rows {
			return o
		}
	}
	return options[len(options)-1]
}
