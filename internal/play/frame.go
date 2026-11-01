package play

import (
	"slices"
	"time"

	"github.com/jiripisa/tuipe/internal/engine"
	"github.com/jiripisa/tuipe/internal/hint"
	"github.com/jiripisa/tuipe/internal/layout"
	"github.com/jiripisa/tuipe/internal/score"
)

// HintInfo describes how to type the current character.
type HintInfo struct {
	Rune  rune
	Text  string
	Route layout.Route
}

// Frame is everything the screen needs to draw one moment of the round.
type Frame struct {
	Cells         []engine.Cell
	Pos, GhostPos int
	WrongPos      map[int]bool // positions that have had a wrong press
	WPM, Accuracy float64
	Lead          time.Duration
	HasGhost      bool
	Hint          *HintInfo
	Paused        bool
	ManualPause   bool // Esc pause: only Resume ends it
	AltWarning    bool
	SLA, SLO      float64
	HasSLA        bool
	Breached      bool
	Done          bool
	Weak          []rune // up to weakCount characters with the most wrong presses
}

// IncidentOutcome is the result of an incident scenario.
type IncidentOutcome struct {
	Won         bool
	SLA         float64
	MTTR        time.Duration
	DamageChars []rune
}

// Result is the outcome of a finished (or inspected) round.
type Result struct {
	Metrics    score.Metrics
	Stars      int
	HintUsed   bool
	Paused     bool
	Log        []engine.Keystroke
	Text       engine.Text
	GhostDelta *time.Duration
	Incident   *IncidentOutcome
}

const (
	weakCount   = 4
	damageCount = 5
)

// Frame reports the state at the given time. It never changes the session.
func (s *Session) Frame(at time.Time) Frame {
	elapsed := s.elapsed(at)
	m := score.Compute(s.round.Text(), s.round.Log(), elapsed)
	f := Frame{
		Cells:       s.round.Text().Cells,
		Pos:         s.round.Pos(),
		WrongPos:    map[int]bool{},
		WPM:         m.WPM,
		Accuracy:    m.Accuracy,
		Paused:      s.pause != pauseNone,
		ManualPause: s.pause == pauseManual,
		AltWarning:  s.altWarning,
		Done:        s.Done(),
		Breached:    s.breached(),
		Weak:        topWrong(s.wrong, weakCount),
	}
	for _, k := range s.round.Log() {
		if !k.Correct {
			f.WrongPos[k.Pos] = true
		}
	}
	if s.ghost != nil {
		f.HasGhost = true
		f.GhostPos = s.ghost.PosAt(elapsed)
		f.Lead = s.ghost.Lead(f.Pos, elapsed)
	}
	if s.meter != nil {
		f.HasSLA = true
		f.SLA = s.meter.SLA()
		f.SLO = s.meter.SLO()
	}
	// Never evaluate later than the last Tick or Key, so the hint shown and
	// HintUsed always agree.
	if s.seen {
		f.Hint = s.hintAt(minTime(at, s.lastSeen))
	}
	return f
}

// hintAt returns the hint shown at the given time, or nil.
func (s *Session) hintAt(at time.Time) *HintInfo {
	if s.Done() || s.round.Pos() >= len(s.round.Text().Cells) {
		return nil
	}
	r := s.round.Text().Cells[s.round.Pos()].R
	if !hint.Show(hint.Input{
		Mastery:    s.stats.MasteryAt(r, at),
		Hesitation: s.hesitation(at),
		WrongAtPos: s.round.WrongAtPos(),
		Disabled:   s.spec.HintsOff,
	}) {
		return nil
	}
	text := hint.Text(s.layout, r)
	if text == "" {
		return nil
	}
	route, _ := s.layout.Route(r)
	return &HintInfo{Rune: r, Text: text, Route: route}
}

// Result summarises the round at the given time.
func (s *Session) Result(at time.Time) Result {
	elapsed := s.elapsed(at)
	m := score.Compute(s.round.Text(), s.round.Log(), elapsed)
	r := Result{
		Metrics:  m,
		HintUsed: s.hintUsed,
		Paused:   s.round.WasPaused(),
		Log:      s.round.Log(),
		Text:     s.round.Text(),
	}
	if s.ghost != nil {
		d := s.ghost.Lead(s.round.Pos(), elapsed)
		r.GhostDelta = &d
	}
	won := true
	if s.meter != nil {
		won = !s.breached() && s.round.Done()
		out := &IncidentOutcome{Won: won, SLA: s.meter.SLA(), MTTR: elapsed}
		if !won {
			out.DamageChars = topWrong(s.wrong, damageCount)
		}
		r.Incident = out
	}
	if won {
		r.Stars = score.Stars(m, s.spec.Targets, s.hintUsed)
	}
	return r
}

// topWrong returns up to n characters with the most wrong presses, most first.
// Ties are broken by rune value so the order is stable.
func topWrong(counts map[rune]int, n int) []rune {
	var out []rune
	for r := range counts {
		out = append(out, r)
	}
	slices.SortFunc(out, func(a, b rune) int {
		if counts[a] != counts[b] {
			return counts[b] - counts[a]
		}
		return int(a - b)
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

func minTime(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}
