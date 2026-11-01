// Package stats keeps per-character and per-bigram typing statistics and the
// mastery state derived from them. It never reads the clock; callers pass times in.
package stats

import (
	"cmp"
	"slices"
	"sort"
	"time"
	"unicode/utf8"

	"github.com/jiripisa/tuipe/internal/engine"
)

// Mastery is how well a character is known.
type Mastery int

// Mastery levels, lowest first. NewChar is the unseen level; the name New is
// taken by the constructor.
const (
	NewChar Mastery = iota
	Learning
	Solid
	Automatic
)

const (
	alpha      = 0.1
	staleAfter = 14 * 24 * time.Hour
	fallError  = 0.12

	learningSamples = 10
	solidSamples    = 30
	solidError      = 0.06
	solidLatencyMs  = 600
	autoSamples     = 80
	autoError       = 0.02
	autoLatencyMs   = 350

	weaknessMinSamples = 5
)

// Entry is the running statistics of one character or bigram.
type Entry struct {
	LatencyMs, ErrorRate float64
	Samples              int
	Mastery              Mastery
	LastSeen             time.Time
}

// Stats holds the entries by character and by bigram. Keys are strings so the
// value serialises to JSON.
type Stats struct {
	Chars   map[string]*Entry
	Bigrams map[string]*Entry
}

// New returns empty stats.
func New() *Stats {
	return &Stats{Chars: map[string]*Entry{}, Bigrams: map[string]*Entry{}}
}

func entry(m *map[string]*Entry, key string) *Entry {
	if *m == nil {
		*m = map[string]*Entry{}
	}
	e := (*m)[key]
	if e == nil {
		e = &Entry{}
		(*m)[key] = e
	}
	return e
}

// sample folds one keystroke into e. latencyMs is nil when there is none.
func (e *Entry) sample(wrong bool, latencyMs *float64, now time.Time) {
	stale := e.Samples > 0 && now.Sub(e.LastSeen) > staleAfter
	errv := 0.0
	if wrong {
		errv = 1
	}
	if e.Samples == 0 {
		e.ErrorRate = errv
	} else {
		e.ErrorRate += alpha * (errv - e.ErrorRate)
	}
	if latencyMs != nil {
		if e.LatencyMs == 0 {
			e.LatencyMs = *latencyMs
		} else {
			e.LatencyMs += alpha * (*latencyMs - e.LatencyMs)
		}
	}
	e.Samples++
	e.LastSeen = now
	e.updateMastery(stale)
}

func (e *Entry) target() Mastery {
	switch {
	case e.Samples >= autoSamples && e.ErrorRate < autoError && e.LatencyMs < autoLatencyMs:
		return Automatic
	case e.Samples >= solidSamples && e.ErrorRate < solidError && e.LatencyMs < solidLatencyMs:
		return Solid
	case e.Samples >= learningSamples:
		return Learning
	}
	return NewChar
}

// updateMastery falls one level when the error rate is high or the entry went
// stale; a stale fall caps Samples at the fallen level's minimum so the higher
// level has to be earned again. Otherwise it rises one level toward the target,
// but only while the error rate is at most fallError, so a poorly typed
// character holds one stable level instead of oscillating.
func (e *Entry) updateMastery(stale bool) {
	switch {
	case (stale || e.ErrorRate > fallError) && e.Mastery > NewChar:
		e.Mastery--
		if stale {
			e.Samples = min(e.Samples, minSamples(e.Mastery))
		}
	case e.ErrorRate <= fallError && e.Mastery < e.target():
		e.Mastery++
	}
}

func minSamples(m Mastery) int {
	switch m {
	case Automatic:
		return autoSamples
	case Solid:
		return solidSamples
	case Learning:
		return learningSamples
	}
	return 0
}

// Record folds one finished round into the stats. Auto cells never appear in the log.
func (s *Stats) Record(t engine.Text, log []engine.Keystroke, now time.Time) {
	var prev *engine.Keystroke // previous correct keystroke
	for i := range log {
		k := log[i]
		if k.Pos < 0 || k.Pos >= len(t.Cells) {
			continue
		}
		var lat *float64
		if prev != nil && k.Correct && !k.AfterPause {
			ms := float64(k.T-prev.T) / float64(time.Millisecond)
			lat = &ms
		}
		entry(&s.Chars, string(t.Cells[k.Pos].R)).sample(!k.Correct, lat, now)
		if prev != nil && prev.Pos == k.Pos-1 {
			bg := string([]rune{t.Cells[prev.Pos].R, t.Cells[k.Pos].R})
			entry(&s.Bigrams, bg).sample(!k.Correct, lat, now)
		}
		if k.Correct {
			prev = &log[i]
		}
	}
}

// MasteryOf is the mastery of r; unseen characters are NewChar.
func (s *Stats) MasteryOf(r rune) Mastery {
	if e := s.Chars[string(r)]; e != nil {
		return e.Mastery
	}
	return NewChar
}

// MasteryAt is the mastery of r as it reads at now: one level lower for every
// 14 days the character has not been typed, so hints come back while it fades.
// It only reads. A character that was never typed, such as one skipped by the
// placement test, has nothing to fade from and keeps its level.
func (s *Stats) MasteryAt(r rune, now time.Time) Mastery {
	e := s.Chars[string(r)]
	if e == nil {
		return NewChar
	}
	if e.LastSeen.IsZero() {
		return e.Mastery
	}
	age := now.Sub(e.LastSeen)
	if age <= staleAfter {
		return e.Mastery
	}
	levels := int((age - 1) / staleAfter)
	return max(e.Mastery-Mastery(levels), NewChar)
}

// SetMastery overrides the mastery of r.
func (s *Stats) SetMastery(r rune, m Mastery) {
	entry(&s.Chars, string(r)).Mastery = m
}

func weakness(e *Entry) float64 {
	if e == nil || e.Samples < weaknessMinSamples {
		return 0
	}
	return e.ErrorRate*3 + e.LatencyMs/1000
}

// Weakness combines error rate and latency; characters with few samples score 0.
func (s *Stats) Weakness(r rune) float64 { return weakness(s.Chars[string(r)]) }

func weakest(m map[string]*Entry, n int, allowed func(string) bool) []string {
	var keys []string
	for k, e := range m {
		if weakness(e) > 0 && allowed(k) {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		wi, wj := weakness(m[keys[i]]), weakness(m[keys[j]])
		if wi != wj {
			return wi > wj
		}
		return keys[i] < keys[j]
	})
	if len(keys) > n {
		keys = keys[:n]
	}
	return keys
}

// WeakestChars returns up to n allowed characters, weakest first. A nil allowed permits all.
func (s *Stats) WeakestChars(n int, allowed func(rune) bool) []rune {
	if n <= 0 {
		return nil
	}
	keys := weakest(s.Chars, n, func(k string) bool {
		r, _ := utf8.DecodeRuneInString(k)
		return allowed == nil || allowed(r)
	})
	out := make([]rune, len(keys))
	for i, k := range keys {
		out[i], _ = utf8.DecodeRuneInString(k)
	}
	return out
}

// StalestChars returns up to n characters of allowed, the longest unseen first.
// Characters that were never typed come first of all, in the order of allowed.
// A last-seen date after now, from a clock that was wrong, counts as now.
func (s *Stats) StalestChars(n int, allowed []rune, now time.Time) []rune {
	if n <= 0 {
		return nil
	}
	out := slices.Clone(allowed)
	age := func(r rune) (never bool, d time.Duration) {
		e := s.Chars[string(r)]
		if e == nil || e.LastSeen.IsZero() {
			return true, 0
		}
		return false, max(now.Sub(e.LastSeen), 0)
	}
	slices.SortStableFunc(out, func(a, b rune) int {
		neverA, ageA := age(a)
		neverB, ageB := age(b)
		switch {
		case neverA != neverB && neverA:
			return -1
		case neverA != neverB:
			return 1
		}
		return cmp.Compare(ageB, ageA)
	})
	return out[:min(n, len(out))]
}

// WeakestBigrams returns up to n bigrams whose both characters are allowed, weakest first.
func (s *Stats) WeakestBigrams(n int, allowed func(rune) bool) []string {
	if n <= 0 {
		return nil
	}
	return weakest(s.Bigrams, n, func(k string) bool {
		for _, r := range k {
			if allowed != nil && !allowed(r) {
				return false
			}
		}
		return true
	})
}

// AverageLatencyMs is the sample-weighted mean latency over characters that have one.
func (s *Stats) AverageLatencyMs() float64 {
	var sum, n float64
	for _, e := range s.Chars {
		if e.LatencyMs > 0 {
			sum += e.LatencyMs * float64(e.Samples)
			n += float64(e.Samples)
		}
	}
	if n == 0 {
		return 0
	}
	return sum / n
}
