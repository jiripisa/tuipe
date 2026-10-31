package ui

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jiripisa/tuipe/internal/store"
	"github.com/jiripisa/tuipe/internal/ui/style"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

const (
	retroWindow   = 7 * 24 * time.Hour
	retroMinShift = 0.02 // error rate change that counts as better or worse
	retroTop      = 3    // characters named on each side
	percent       = 100
)

// charShift is the change of a character's error rate over the Sprint.
type charShift struct {
	char  string
	shift float64 // fraction, negative is better
}

// retroScreen compares a Sprint with the previous 7 days.
type retroScreen struct {
	runs               int
	wpm, acc           float64
	hasPrev            bool
	prevWPM, prevAcc   float64
	hasSnap            bool
	improved, worsened []charShift
}

// NewRetroScreen sums up the given runs of a Sprint against the 7 days before
// the first of them. The characters are compared with the snapshot saved when
// the Sprint started, which this call takes from Profile.Resume and clears.
func NewRetroScreen(a *App, runs []store.Run) Screen {
	s := &retroScreen{runs: len(runs)}
	p := a.Store.Profile()
	var snap *store.SprintState
	if r := p.Resume; r != nil && r.Kind == resumeKindSprint {
		snap = r.Sprint
		p.Resume = nil
		a.SaveErr(a.Store.SaveProfile())
	}
	if len(runs) == 0 {
		return s
	}
	s.wpm, s.acc = means(runs)
	start := runs[0].At
	for _, r := range runs {
		if r.At.Before(start) {
			start = r.At
		}
	}
	all, _ := a.Store.Runs()
	var before []store.Run
	for _, r := range all {
		if r.At.Before(start) && !r.At.Before(start.Add(-retroWindow)) {
			before = append(before, r)
		}
	}
	if len(before) > 0 {
		s.hasPrev = true
		s.prevWPM, s.prevAcc = means(before)
	}
	if snap != nil && len(snap.Chars) > 0 {
		s.hasSnap = true
		s.improved, s.worsened = a.charShifts(snap)
	}
	return s
}

func means(runs []store.Run) (wpm, acc float64) {
	for _, r := range runs {
		wpm += r.WPM
		acc += r.Accuracy
	}
	n := float64(len(runs))
	return wpm / n, acc / n * percent
}

// charShifts lists the characters typed during the Sprint whose error rate
// moved by at least retroMinShift, the biggest moves first. A character with
// no entry at the start has nothing to be compared with.
func (a *App) charShifts(snap *store.SprintState) (better, worse []charShift) {
	for k, end := range a.stats().Chars {
		start, ok := snap.Chars[k]
		if !ok || end.Samples <= start.Samples {
			continue
		}
		d := end.ErrorRate - start.ErrorRate
		switch {
		case d <= -retroMinShift:
			better = append(better, charShift{k, d})
		case d >= retroMinShift:
			worse = append(worse, charShift{k, d})
		}
	}
	order := func(x, y charShift) int {
		return cmp.Or(cmp.Compare(math.Abs(y.shift), math.Abs(x.shift)), cmp.Compare(x.char, y.char))
	}
	slices.SortFunc(better, order)
	slices.SortFunc(worse, order)
	return better[:min(len(better), retroTop)], worse[:min(len(worse), retroTop)]
}

func (s *retroScreen) Update(msg tea.Msg, a *App) (Screen, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok && (k.String() == "enter" || k.String() == "esc") {
		return NewHomeScreen(a), nil
	}
	return s, nil
}

func shown(c string) string {
	switch c {
	case " ":
		return "␣"
	case "\n":
		return "⏎"
	}
	return c
}

func (s *retroScreen) shifts(list []charShift) string {
	parts := make([]string, len(list))
	for i, c := range list {
		parts[i] = fmt.Sprintf(text.RetroCharShift, shown(c.char), c.shift*percent)
	}
	return strings.Join(parts, ", ")
}

func (s *retroScreen) View(a *App, w, h int) string {
	st := a.Style
	width := w - 4
	var lines []string
	para := func(line string, fg, bold bool) {
		col := st.Dim
		if fg {
			col = st.Fg
		}
		lines = append(lines, painted(st, col, bold, line, width)...)
	}
	paint := func(line string, ok bool) {
		col := st.Bad
		if ok {
			col = st.OK
		}
		lines = append(lines, painted(st, col, false, line, width)...)
	}
	para(text.RetroHead, false, false)
	if s.runs == 0 {
		para("", true, false)
		para(text.RetroNoRuns, true, false)
		return s.frame(a, w, h, lines)
	}
	para(fmt.Sprintf(text.RetroRuns, s.runs), true, true)
	para("", true, false)
	if s.hasPrev {
		para(fmt.Sprintf(text.RetroWPM, s.wpm, s.wpm-s.prevWPM), true, false)
		para(fmt.Sprintf(text.RetroAccuracy, s.acc, s.acc-s.prevAcc), true, false)
	} else {
		para(fmt.Sprintf(text.RetroWPMNew, s.wpm), true, false)
		para(fmt.Sprintf(text.RetroAccNew, s.acc), true, false)
		para(text.RetroNoPrev, false, false)
	}
	para("", true, false)
	switch {
	case !s.hasSnap:
		para(text.RetroNoSnapshot, false, false)
	case len(s.improved) == 0 && len(s.worsened) == 0:
		para(text.RetroNoShift, false, false)
	}
	if len(s.improved) > 0 {
		paint(fmt.Sprintf(text.RetroImproved, s.shifts(s.improved)), true)
	}
	if len(s.worsened) > 0 {
		paint(fmt.Sprintf(text.RetroWorsened, s.shifts(s.worsened)), false)
	}
	return s.frame(a, w, h, lines)
}

func (s *retroScreen) frame(a *App, w, h int, lines []string) string {
	if rows := h - 3; len(lines) > rows {
		lines = lines[:max(rows, 0)]
	}
	return style.Frame(a.Style, w, h, text.AppName+" · "+text.RetroTitle, strings.Join(lines, "\n"), text.HintRetro)
}
