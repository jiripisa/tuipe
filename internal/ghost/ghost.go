// Package ghost provides the second cursor that races the player: an exact
// replay of a previous attempt or a pace computed from per-character timing.
package ghost

import (
	"math"
	"sort"
	"time"

	"github.com/jiripisa/tuipe/internal/engine"
	"github.com/jiripisa/tuipe/internal/score"
	"github.com/jiripisa/tuipe/internal/stats"
	"github.com/jiripisa/tuipe/internal/store"
)

// paceFactor sets the pace ghost slightly faster than the player's average.
const paceFactor = 0.97

// Ghost holds the cumulative time in ms at which each cell was finished.
type Ghost struct {
	timesMs []int64
}

// FromLog returns the time in ms at which each cell of t was finished, taken
// from the cell's correct keystroke. An Auto cell repeats the previous cell's
// time. The result stops before the first cell that was never typed correctly.
func FromLog(t engine.Text, log []engine.Keystroke) []int64 {
	done := make(map[int]int64, len(log))
	for _, k := range log {
		if k.Correct {
			done[k.Pos] = k.T.Milliseconds()
		}
	}
	times := make([]int64, 0, len(t.Cells))
	var prev int64
	for i, c := range t.Cells {
		if c.Auto {
			times = append(times, prev)
			continue
		}
		ms, ok := done[i]
		if !ok {
			break
		}
		prev = ms
		times = append(times, ms)
	}
	return times
}

// Replay returns a ghost that replays the given per-cell finish times.
func Replay(timesMs []int64) *Ghost {
	return &Ghost{timesMs: append([]int64(nil), timesMs...)}
}

// Pace returns a ghost typing t at 97 % of the player's average latency per
// character. Characters without a latency sample use fallbackMsPerChar; Auto
// cells cost no time. s may be nil.
func Pace(t engine.Text, s *stats.Stats, fallbackMsPerChar float64) *Ghost {
	times := make([]int64, 0, len(t.Cells))
	var total float64
	for _, c := range t.Cells {
		if !c.Auto {
			total += msPerChar(s, c.R, fallbackMsPerChar) * paceFactor
		}
		times = append(times, int64(math.Round(total)))
	}
	return &Ghost{timesMs: times}
}

func msPerChar(s *stats.Stats, r rune, fallback float64) float64 {
	if s != nil {
		if e := s.Chars[string(r)]; e != nil && e.Samples > 0 && e.LatencyMs > 0 {
			return e.LatencyMs
		}
	}
	return fallback
}

// PosAt returns the number of cells the ghost has finished by time d.
func (g *Ghost) PosAt(d time.Duration) int {
	ms := d.Milliseconds()
	return sort.Search(len(g.timesMs), func(i int) bool { return g.timesMs[i] > ms })
}

// Lead is how far ahead of the ghost the player is after finishing pos cells
// at time now. Positive means the player is ahead. A pos beyond the ghost's
// length is compared with the ghost's last cell.
func (g *Ghost) Lead(pos int, now time.Duration) time.Duration {
	if pos <= 0 || len(g.timesMs) == 0 {
		return 0
	}
	pos = min(pos, len(g.timesMs))
	return time.Duration(g.timesMs[pos-1])*time.Millisecond - now
}

// ShouldReplace reports whether a finished run replaces the stored ghost: only
// on a higher score, never after a pause and never for generated text.
func ShouldReplace(old *store.Ghost, m score.Metrics, paused, generatedText bool) bool {
	if paused || generatedText {
		return false
	}
	return old == nil || m.Score > old.Score
}
