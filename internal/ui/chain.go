package ui

import (
	"fmt"
	"maps"
	"math/rand/v2"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jiripisa/tuipe/internal/incident"
	"github.com/jiripisa/tuipe/internal/play"
	"github.com/jiripisa/tuipe/internal/recommend"
	"github.com/jiripisa/tuipe/internal/stats"
	"github.com/jiripisa/tuipe/internal/store"
	"github.com/jiripisa/tuipe/internal/ui/style"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

// Kinds of chain.
const (
	kindStandup      = "standup"
	kindSprint       = "sprint"
	resumeKindSprint = "sprint"
)

// sprintMinutes are the lengths offered by the picker; the default is the middle one.
var sprintMinutes = []int{10, 20, 30}

const (
	sprintDefault = 1
	msPerMinute   = 60_000
	seedMix       = 0x9e3779b97f4a7c15
	finaleRound   = -1 // the round of a Sprint that plays its finale
)

// chain drives a series of planned rounds: the daily Standup or a Sprint. The
// rounds are the ordinary round screens; the chain is what their next function
// returns to. The App holds the running chain so that finishing, abandoning and
// the first keystroke of a round can keep its state, see chainFinished.
type chain struct {
	kind  string
	picks []recommend.Pick
	state *store.SprintState // set for a Sprint, shared with Profile.Resume
	idx   int                // the round on screen, or finaleRound
	done  int                // rounds finished: the index to continue at
}

func seededRand(seed int64) *rand.Rand {
	s := uint64(seed)
	return rand.New(rand.NewPCG(s, s^seedMix))
}

func (a *App) today() string { return a.Now().Local().Format(dateLayout) }

// NewChain starts a Standup or a Sprint of the given minutes and returns its
// first round. A nil picks gets the plan the recommender makes; Sprint plans
// are made from the saved state, so a resumed Sprint gets the same rounds.
func NewChain(a *App, kind string, picks []recommend.Pick, minutes int) Screen {
	c := &chain{kind: kind, picks: picks}
	switch kind {
	case kindStandup:
		if c.picks == nil {
			c.picks = recommend.Standup(a.liveState(seededRand(a.Now().UnixNano())))
		}
	case kindSprint:
		c.state = a.newSprintState(minutes)
		if c.picks == nil {
			c.picks = a.sprintPlan(c.state)
		}
	default:
		return NewHomeScreen(a)
	}
	if len(c.picks) == 0 {
		return NewHomeScreen(a)
	}
	a.chain = c
	return c.screen(a)
}

// liveState is what the recommender looks at now.
func (a *App) liveState(rng *rand.Rand) recommend.State {
	runs, _ := a.Store.Runs() // a damaged history only affects the drill length
	return recommend.State{
		Curr: a.Curr, Lib: a.Lib, Profile: a.Store.Profile(), Stats: a.stats(),
		RunCount: a.runCadence(), Rand: rng, Now: a.Now(), WPM: recommend.PaceWPM(runs),
	}
}

// sprintPlanState is what the recommender looks at for a Sprint: the snapshot
// of its start and nothing that has moved since.
func (a *App) sprintPlanState(s *store.SprintState, rng *rand.Rand) recommend.State {
	st := stats.New()
	for k, e := range s.Chars {
		st.Chars[k] = &e
	}
	for k, e := range s.Bigrams {
		st.Bigrams[k] = &e
	}
	started, _ := time.Parse(time.RFC3339Nano, s.Started)
	return recommend.State{
		Curr: a.Curr, Lib: a.Lib, Stats: st, RunCount: a.runCadence(), Rand: rng, Now: started, WPM: s.WPM,
		Profile: &store.Profile{Stars: s.Stars, Completed: s.Completed, IncidentBest: s.IncidentBest},
	}
}

// newSprintState snapshots what the plan and the Retro need: the character and
// bigram statistics, the stars and the completed chapters at the start.
func (a *App) newSprintState(minutes int) *store.SprintState {
	p := a.Store.Profile()
	now := a.Now()
	s := &store.SprintState{
		Minutes: minutes, Seed: now.UnixNano(), Started: now.Format(time.RFC3339Nano),
		WPM:          a.liveState(nil).WPM,
		Chars:        map[string]stats.Entry{},
		Bigrams:      map[string]stats.Entry{},
		Stars:        maps.Clone(p.Stars),
		Completed:    maps.Clone(p.Completed),
		IncidentBest: maps.Clone(p.IncidentBest),
	}
	for k, e := range a.stats().Chars {
		s.Chars[k] = *e
	}
	for k, e := range a.stats().Bigrams {
		s.Bigrams[k] = *e
	}
	return s
}

// sprintPlan lays out a Sprint from its saved state alone: the same seed and
// snapshot give the same rounds, however far the profile has moved on since.
func (a *App) sprintPlan(s *store.SprintState) []recommend.Pick {
	return recommend.Sprint(a.sprintPlanState(s, seededRand(s.Seed)), s.Minutes)
}

// current is the round to play. Once the budget of a Sprint is spent it is the
// finale, whatever else was left.
func (c *chain) current() int {
	if c.state != nil && c.state.ActiveMs >= int64(c.state.Minutes)*msPerMinute {
		return finaleRound
	}
	return c.done
}

// pickAt is the pick of a round. A Sprint with minutes left after its planned
// rounds goes on with extra drills, which follow from its state and the index.
func (c *chain) pickAt(a *App, i int) recommend.Pick {
	last := len(c.picks) - 1
	switch {
	case i == finaleRound:
		return c.picks[last]
	case c.state == nil || i < last:
		return c.picks[i]
	}
	return recommend.SprintExtra(a.sprintPlanState(c.state, seededRand(c.state.Seed+int64(i))), i)
}

// screen builds the round to play now.
func (c *chain) screen(a *App) Screen {
	c.idx = c.current()
	pick := c.pickAt(a, c.idx)
	if pick.Kind == recommend.Incident {
		inc, ok := a.incidentByID(pick.IncidentID)
		if !ok {
			a.SaveErr(fmt.Errorf("unknown incident %q", pick.IncidentID))
			return NewHomeScreen(a)
		}
		return NewIncidentScreen(a, inc, incident.SEV3, c.next)
	}
	return NewRoundScreen(a, pick, c.next)
}

// over reports a chain with nothing left to play.
func (c *chain) over() bool {
	if c.state != nil {
		return c.state.FinaleDone
	}
	return c.done >= len(c.picks)
}

// next follows a results screen: the next round, or the end of the chain.
func (c *chain) next(a *App) Screen {
	if !c.over() {
		return c.screen(a)
	}
	a.chain = nil
	if c.state == nil {
		return NewHomeScreen(a)
	}
	return a.endSprint(c.state)
}

// sprintResume reports a Resume that holds a Sprint.
func sprintResume(r *store.Resume) bool { return r != nil && r.Kind == resumeKindSprint }

// endSprint opens the Retro of a Sprint and drops the Sprint to resume.
func (a *App) endSprint(s *store.SprintState) Screen {
	retro := newRetro(a, a.sessionRuns(s), s)
	a.Store.Profile().Resume = nil
	a.SaveErr(a.Store.SaveProfile())
	return retro
}

// sessionRuns are the runs that belong to the Sprint.
func (a *App) sessionRuns(s *store.SprintState) []store.Run {
	if s == nil {
		return nil
	}
	all, _ := a.Store.Runs()
	var out []store.Run
	for _, r := range all {
		if slices.Contains(s.RunIDs, r.ID) {
			out = append(out, r)
		}
	}
	return out
}

// activeOf is the active time of a finished round. A lost incident has only
// the time it ran before the breach, which is also in its metrics.
func activeOf(r play.Result) time.Duration {
	d := r.Metrics.Duration
	if r.Incident != nil {
		d = max(d, r.Incident.MTTR)
	}
	return d
}

// finished is called by Finish, before it saves the profile. It counts the
// round and keeps the Sprint to resume, or marks the day of a finished Standup.
func (c *chain) finished(a *App, r play.Result, runID string) {
	if c == nil {
		return
	}
	if c.idx != finaleRound {
		c.done = c.idx + 1
	}
	switch {
	case c.state != nil:
		c.state.ActiveMs += activeOf(r).Milliseconds()
		c.state.NextIndex = c.done
		c.state.FinaleDone = c.idx == finaleRound
		c.state.RunIDs = append(c.state.RunIDs, runID)
		c.remember(a)
	case c.done >= len(c.picks):
		a.Store.Profile().StandupDay = a.today()
	}
}

// partial counts the active time of a round that was left or restarted.
func (c *chain) partial(a *App, d time.Duration) {
	if c == nil || c.state == nil {
		return
	}
	c.state.ActiveMs += d.Milliseconds()
	c.remember(a)
}

// started saves the Sprint at the first keystroke of a round, so that a closed
// window still offers to continue.
func (c *chain) started(a *App) {
	if c == nil || c.state == nil {
		return
	}
	c.remember(a)
	a.SaveErr(a.Store.SaveProfile())
}

func (c *chain) remember(a *App) {
	a.Store.Profile().Resume = &store.Resume{Kind: resumeKindSprint, Day: a.today(), Sprint: c.state}
}

// label is the progress of the chain: the round of a Standup, or the active
// time of a Sprint against its budget, with extra for the round being typed.
func (c *chain) label(extra time.Duration) string {
	if c == nil {
		return ""
	}
	if c.state == nil {
		return fmt.Sprintf(text.ChainStandup, c.idx+1, len(c.picks))
	}
	s := int((time.Duration(c.state.ActiveMs)*time.Millisecond + extra).Seconds())
	return fmt.Sprintf(text.ChainSprint, s/60, s%60, c.state.Minutes)
}

// sprintToContinue is the Sprint left earlier today.
func (a *App) sprintToContinue() *store.SprintState {
	r := a.Store.Profile().Resume
	if r == nil || r.Kind != resumeKindSprint || r.Sprint == nil || r.Day != a.today() {
		return nil
	}
	return r.Sprint
}

// openSprint is the home entry: continue today's Sprint, or ask how long.
func openSprint(a *App) (Screen, tea.Cmd) {
	s := a.sprintToContinue()
	if s == nil {
		return &sprintPickerScreen{sel: sprintDefault}, nil
	}
	c := &chain{kind: kindSprint, state: s, picks: a.sprintPlan(s), done: s.NextIndex}
	a.chain = c
	if len(c.picks) == 0 {
		return NewHomeScreen(a), nil
	}
	return c.next(a), nil
}

func openStandup(a *App) (Screen, tea.Cmd) { return NewChain(a, kindStandup, nil, 0), nil }

// sprintPickerScreen asks for the length of a new Sprint.
type sprintPickerScreen struct{ sel int }

func (s *sprintPickerScreen) Update(msg tea.Msg, a *App) (Screen, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return s, nil
	}
	n := len(sprintMinutes)
	switch k.String() {
	case "up", "k":
		s.sel = (s.sel + n - 1) % n
	case "down", "j":
		s.sel = (s.sel + 1) % n
	case "esc":
		return NewHomeScreen(a), nil
	case "enter":
		return NewChain(a, kindSprint, nil, sprintMinutes[s.sel]), nil
	}
	return s, nil
}

func (s *sprintPickerScreen) View(a *App, w, h int) string {
	st := a.Style
	lines := []string{st.Base().Foreground(st.Fg).Bold(true).Render(text.SprintAsk), ""}
	for i, m := range sprintMinutes {
		label := fmt.Sprintf(text.SprintOption, m)
		if i == s.sel {
			lines = append(lines, st.Base().Foreground(st.Accent).Bold(true).Render("> "+label))
		} else {
			lines = append(lines, st.Base().Foreground(st.Fg).Render("  "+label))
		}
	}
	return style.Frame(st, w, h, text.AppName+" · "+text.SprintTitle, strings.Join(lines, "\n"), text.HintSprint)
}
