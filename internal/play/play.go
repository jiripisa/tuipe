// Package play combines the engine, ghost, hints and incident meter into one
// session that the terminal UI drives. It never reads the clock.
package play

import (
	"time"
	"unicode"

	"github.com/jiripisa/tuipe/internal/engine"
	"github.com/jiripisa/tuipe/internal/ghost"
	"github.com/jiripisa/tuipe/internal/incident"
	"github.com/jiripisa/tuipe/internal/layout"
	"github.com/jiripisa/tuipe/internal/score"
	"github.com/jiripisa/tuipe/internal/stats"
)

// Kind is the type of round.
type Kind int

// Kinds of round.
const (
	KindLesson Kind = iota
	KindDrill
	KindCode
	KindIncident
	KindTestOut
	KindPlacement
)

// Spec describes one round.
type Spec struct {
	Kind                  Kind
	Title, Lang, LessonID string
	ChapterID             string // the chapter a test-out would complete
	Practice              bool   // chosen by hand in Practice, for the run's mode
	Text                  string
	Targets               score.Targets
	HintsOff              bool
	Ghost                 *ghost.Ghost
	GhostIsPace           bool
	Incident              *incident.Params
}

// Input is one key press. Alt is set when the terminal reported Alt/Option held.
type Input struct {
	Rune rune
	Alt  bool
}

// gapIsSleep is the longest gap between ticks, or without input, that is still active time.
const gapIsSleep = 5 * time.Second

type pauseKind int

const (
	pauseNone   pauseKind = iota
	pauseAuto             // blur, idle or sleep: the next key resumes
	pauseManual           // Esc: only Resume ends it
)

// Session is one play-through of a Spec.
type Session struct {
	spec        Spec
	layout      *layout.Layout
	stats       *stats.Stats
	focusEvents bool
	focusSeen   bool

	round *engine.Round
	ghost *ghost.Ghost
	meter *incident.Meter

	pause      pauseKind
	pausedFrom time.Time
	lastActive time.Time // last keystroke or resume
	lastTick   time.Time
	hesBase    time.Time // start of the current hesitation, shifted by pauses
	inited     bool

	meterElapsed time.Duration
	logLen       int
	wrong        map[rune]int // wrong presses by expected character
	hintUsed     bool
	lastSeen     time.Time // last Tick or Key: Frame never evaluates a hint later than this
	seen         bool
	altWarning   bool
	lost         bool
	lostElapsed  time.Duration
}

// New starts a session. focusEvents tells whether the terminal reports focus changes.
func New(spec Spec, l *layout.Layout, st *stats.Stats, focusEvents bool) *Session {
	if st == nil {
		st = stats.New()
	}
	s := &Session{
		spec:        spec,
		layout:      l,
		stats:       st,
		focusEvents: focusEvents,
		round:       engine.New(engine.Prepare(spec.Text)),
		ghost:       spec.Ghost,
		wrong:       map[rune]int{},
	}
	if spec.Incident != nil {
		s.meter = incident.NewMeter(*spec.Incident)
	}
	return s
}

// Done reports whether the round is over: finished, or lost to an SLA breach.
func (s *Session) Done() bool { return s.round.Done() || s.lost }

func (s *Session) breached() bool { return s.meter != nil && s.meter.Breached() }

// PartialLog returns the text and the keystrokes made so far, for abandoned rounds.
func (s *Session) PartialLog() (engine.Text, []engine.Keystroke) {
	return s.round.Text(), s.round.Log()
}

func (s *Session) elapsed(at time.Time) time.Duration {
	if s.lost {
		return s.lostElapsed
	}
	return s.round.Elapsed(at)
}

// touch records the first time the session is driven, which starts the hesitation clock.
func (s *Session) touch(at time.Time) {
	if !s.inited {
		s.inited = true
		s.hesBase = at
		s.lastTick = at
	}
}

// hesitation is the active time since the last correct key, or since the
// first tick before the first key. Pauses are not counted.
func (s *Session) hesitation(at time.Time) time.Duration {
	if !s.inited {
		return 0
	}
	if s.pause != pauseNone {
		at = s.pausedFrom
	}
	return max(at.Sub(s.hesBase), 0)
}

func (s *Session) startPause(kind pauseKind, from time.Time) {
	if s.Done() {
		return
	}
	if s.pause != pauseNone {
		if kind == pauseManual {
			s.pause = pauseManual // Esc during an automatic pause keeps its start
		}
		return
	}
	if from.Before(s.lastActive) {
		from = s.lastActive
	}
	s.pause = kind
	s.pausedFrom = from
	s.round.PauseFrom(from)
}

func (s *Session) endPause(at time.Time) {
	if s.pause == pauseNone {
		return
	}
	if s.inited {
		s.hesBase = s.hesBase.Add(max(at.Sub(s.pausedFrom), 0))
	}
	s.pause = pauseNone
	s.round.Resume(at)
	s.lastActive = at
}

// checkGaps starts an automatic pause after a machine sleep or, without focus
// reporting, after 5 seconds without input.
func (s *Session) checkGaps(at time.Time) {
	if s.pause != pauseNone || s.Done() {
		return
	}
	if !s.focusEvents && !s.focusSeen && s.round.Started() && at.Sub(s.lastActive) >= gapIsSleep {
		s.startPause(pauseAuto, s.lastActive)
		return
	}
	if at.Sub(s.lastTick) > gapIsSleep {
		s.startPause(pauseAuto, s.lastTick)
	}
}

// syncMeter burns SLA for the active time since the last sync.
func (s *Session) syncMeter(at time.Time) {
	if s.meter == nil || s.lost {
		return
	}
	e := s.round.Elapsed(at)
	if d := e - s.meterElapsed; d != 0 { // negative after a retroactive pause
		s.meter.Advance(d)
		s.meterElapsed = e
	}
	if s.meter.Breached() {
		s.lost = true
		s.lostElapsed = e
	}
}

// noteHint records that a hint is shown now.
func (s *Session) noteHint(at time.Time) {
	if !s.hintUsed && s.hintAt(at) != nil {
		s.hintUsed = true
	}
}

// Tick advances time without input.
func (s *Session) Tick(at time.Time) {
	s.touch(at)
	s.checkGaps(at)
	s.lastTick = at
	s.syncMeter(at)
	s.see(at)
	s.noteHint(at)
}

// see marks the time of the last hint evaluation.
func (s *Session) see(at time.Time) {
	s.lastSeen = at
	s.seen = true
}

// Key handles one key press. A key during an automatic pause resumes the round
// and counts as input; a key during an Esc pause is ignored.
func (s *Session) Key(in Input, at time.Time) {
	s.touch(at)
	s.checkGaps(at)
	s.lastTick = at
	if s.Done() || s.pause == pauseManual {
		return
	}
	if in.Rune != '\n' && !unicode.IsPrint(in.Rune) {
		return // same rule as the engine: such keys are not input at all
	}
	if in.Alt && !s.expects(in.Rune) {
		s.altWarning = true
		return
	}
	s.altWarning = false
	s.endPause(at)
	s.syncMeter(at)
	if s.lost {
		return
	}
	pos := s.round.Pos()
	expected := s.round.Text().Cells[pos].R
	correct := s.round.Press(in.Rune, at)
	s.lastActive = at
	if n := len(s.round.Log()); n > s.logLen {
		s.logLen = n
		if correct {
			s.hesBase = at
		} else {
			s.wrong[expected]++
			if s.meter != nil {
				s.meter.Error()
			}
		}
	}
	s.syncMeter(at)
	s.see(at)
	s.noteHint(at)
}

// expects reports whether r is exactly the rune the round waits for.
func (s *Session) expects(r rune) bool {
	cells := s.round.Text().Cells
	pos := s.round.Pos()
	return pos < len(cells) && cells[pos].R == r
}

// Focus reports a terminal focus change. Losing focus pauses the round; gaining
// it does not resume, the next key does.
func (s *Session) Focus(focused bool, at time.Time) {
	s.touch(at)
	s.checkGaps(at)
	s.focusSeen = true
	s.lastTick = at
	s.syncMeter(at)
	if !focused {
		s.startPause(pauseAuto, at)
	}
}

// Pause is the explicit (Esc) pause. An automatic pause stays automatic.
func (s *Session) Pause(at time.Time) {
	s.touch(at)
	s.checkGaps(at)
	s.lastTick = at
	s.syncMeter(at)
	s.startPause(pauseManual, at)
}

// Resume ends any pause.
func (s *Session) Resume(at time.Time) {
	s.touch(at)
	s.lastTick = at
	s.endPause(at)
}
