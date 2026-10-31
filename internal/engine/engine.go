// Package engine implements the typing round: stop-on-error input, active-time
// accounting and the keystroke log. It never reads the clock; callers pass times in.
package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
	"unicode"
)

// Cell is one character of the text to type.
type Cell struct {
	R    rune
	Auto bool // indentation skipped by the engine
}

// Text is prepared source text.
type Text struct {
	Cells []Cell
	Hash  string // sha256 hex of the source
}

// Prepare converts source into cells, marking the leading spaces and tabs
// after each newline as Auto.
func Prepare(s string) Text {
	sum := sha256.Sum256([]byte(s))
	t := Text{Hash: hex.EncodeToString(sum[:])}
	atLineStart := false
	for _, r := range s {
		auto := atLineStart && (r == ' ' || r == '\t')
		t.Cells = append(t.Cells, Cell{R: r, Auto: auto})
		atLineStart = auto || r == '\n'
	}
	return t
}

// Keystroke is one recorded keypress.
type Keystroke struct {
	Pos     int
	Typed   rune
	Correct bool
	T       time.Duration // active time since the first keystroke
	// AfterPause marks the first keystroke after a pause; its T-gap to the
	// previous key is not a real latency.
	AfterPause bool
}

// Round is one typing round.
type Round struct {
	text       Text
	pos        int
	wrong      int
	log        []Keystroke
	started    bool
	start      time.Time
	paused     bool
	pausedAt   time.Time
	pausedSum  time.Duration
	wasPaused  bool
	afterPause bool
	done       bool
	end        time.Duration
}

// New starts a round on t, skipping any Auto cells at the cursor.
func New(t Text) *Round {
	r := &Round{text: t}
	r.skipAuto()
	r.done = r.pos >= len(t.Cells)
	return r
}

func (r *Round) skipAuto() {
	for r.pos < len(r.text.Cells) && r.text.Cells[r.pos].Auto {
		r.pos++
	}
}

// Press handles a typed rune. Runes other than printable ones and newline,
// presses while paused and presses after the end are ignored.
func (r *Round) Press(ch rune, at time.Time) bool {
	if r.done || r.paused || (ch != '\n' && !unicode.IsPrint(ch)) {
		return false
	}
	if !r.started {
		r.started = true
		r.start = at
	}
	correct := ch == r.text.Cells[r.pos].R
	t := r.Elapsed(at)
	r.log = append(r.log, Keystroke{Pos: r.pos, Typed: ch, Correct: correct, T: t, AfterPause: r.afterPause})
	r.afterPause = false
	if !correct {
		r.wrong++
		return false
	}
	r.wrong = 0
	r.pos++
	r.skipAuto()
	if r.pos >= len(r.text.Cells) {
		r.done = true
		r.end = t
	}
	return true
}

// Pause stops the active clock at the given time.
func (r *Round) Pause(at time.Time) { r.PauseFrom(at) }

// PauseFrom stops the active clock retroactively from the given time.
func (r *Round) PauseFrom(from time.Time) {
	if !r.started || r.paused || r.done {
		return
	}
	if from.Before(r.start) {
		from = r.start
	}
	r.paused = true
	r.wasPaused = true
	r.pausedAt = from
}

// Resume restarts the active clock; the paused span is not counted.
func (r *Round) Resume(at time.Time) {
	if !r.paused {
		return
	}
	if d := at.Sub(r.pausedAt); d > 0 {
		r.pausedSum += d
	}
	r.paused = false
	r.afterPause = true
}

func (r *Round) Pos() int        { return r.pos }
func (r *Round) Started() bool   { return r.started }
func (r *Round) Paused() bool    { return r.paused }
func (r *Round) WasPaused() bool { return r.wasPaused }
func (r *Round) Done() bool      { return r.done }

// Elapsed is the active time, excluding pauses. It is frozen once the round is done.
func (r *Round) Elapsed(at time.Time) time.Duration {
	if !r.started {
		return 0
	}
	if r.done {
		return r.end
	}
	if r.paused {
		at = r.pausedAt
	}
	return at.Sub(r.start) - r.pausedSum
}

// Log returns a copy of the keystroke log.
func (r *Round) Log() []Keystroke { return append([]Keystroke(nil), r.log...) }

// WrongAtPos is the number of wrong presses at the current position.
func (r *Round) WrongAtPos() int { return r.wrong }

// Text returns the prepared text.
func (r *Round) Text() Text { return r.text }
