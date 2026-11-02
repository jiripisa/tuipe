package roundview

import (
	"fmt"
	"image/color"
	"math"
	"slices"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jiripisa/tuipe/internal/play"
	"github.com/jiripisa/tuipe/internal/ui/style"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

const (
	velinBottomH = keyboardRows + 2
	velinTopMax  = 6 // inner rows of the top panels in a tall window
)

// Colours of the Velín mockup that are not part of the shared style.
var (
	velinLine  color.Color = lipgloss.Color("#2d333b")
	velinLabel color.Color = lipgloss.Color("#768390")
)

// sparkLevels are the eight heights of one sparkline cell.
var sparkLevels = []rune("▁▂▃▄▅▆▇█")

// velin is the control room: a grid of panels with the numbers on top, the text
// in the middle, and the map with the next character at the bottom. The grid
// never changes while typing. A tall window with a short text gives the spare
// rows to the top panels, so the charts grow instead of the emptiness.
func velin(s style.Style, v View, w, h int) []string {
	f := v.Frame
	right := 20 + (w-minW)/10 // narrow enough to leave the map its 3-cell keys at 80
	left := (w - right) / 2
	mid := w - right - left

	textW := w - 3
	rows := wrap(f.Cells, textW)

	wanted := len(rows) + 2
	topInner := 2
	if h >= 30 {
		topInner = 4
	}
	if spare := h - velinBottomH - 2 - topInner - wanted; spare > 0 {
		topInner = min(topInner+spare, velinTopMax)
	}
	topH := topInner + 2
	textH := h - topH - velinBottomH

	lines := zip(
		velinFirst(s, f, left, topH),
		velinThroughput(s, v, mid, topH),
		velinStats(s, f, right, topH),
	)
	lines = append(lines, velinText(s, v, rows, w, textH)...)
	return append(lines, zip(
		velinMap(s, v, rows, w-right),
		velinNext(s, f, right),
	)...)
}

// zip puts panels of the same height side by side.
func zip(panels ...[]string) []string {
	out := make([]string, len(panels[0]))
	for _, panel := range panels {
		for i, line := range panel {
			out[i] += line
		}
	}
	return out
}

// box draws a panel w × h with labels in its top and bottom edges. The body has
// one column of padding on the left only: at width 60 the text panel needs all
// the room it can get.
func box(s style.Style, w, h int, title, right, footer string, body []string) []string {
	b := s.Border
	edge := plain(s, velinLine)
	inner := w - 3
	titled := func(start, line, end, label, tail string) string {
		head := start + line
		if label != "" {
			label = " " + label + " "
		}
		if tail != "" {
			tail = " " + tail + " "
		}
		room := max(0, w-4-ansi.StringWidth(tail))
		if ansi.StringWidth(label) > room {
			label = ansi.Truncate(label, max(0, room-1), "…")
		}
		fill := max(0, room-ansi.StringWidth(label))
		return edge.render(head) + label + edge.render(strings.Repeat(line, fill)) + tail +
			edge.render(line+end)
	}
	lines := []string{titled(b.TopLeft, b.Top, b.TopRight, title, right)}
	for i := range max(0, h-2) {
		row := ""
		if i < len(body) {
			row = body[i]
		}
		lines = append(lines, edge.render(b.Left)+" "+s.Paint(row, inner)+edge.render(b.Right))
	}
	return append(lines, titled(b.BottomLeft, b.Bottom, b.BottomRight, footer, ""))
}

func caption(s style.Style, name string) string { return plain(s, velinLabel).render(name) }

// velinFirst is the progress panel.
func velinFirst(s style.Style, f play.Frame, w, h int) []string {
	done, total := progress(f)
	value, note := percent(progressShare(f)), fmt.Sprintf("%d / %d", done, total)
	inner := w - 4
	body := []string{spread(look{fg: s.OK, bg: s.Bg, bold: true}.render(value), caption(s, note), inner, plain(s, s.Fg))}
	for len(body) < h-2 {
		body = append(body, meter(s, progressShare(f), inner, s.OK))
	}
	return box(s, w, h, caption(s, text.RoundPanelProgress), "", "", body)
}

// velinThroughput is the current speed over its recent history.
func velinThroughput(s style.Style, v View, w, h int) []string {
	body := []string{plain(s, s.Fg).render(wpmText(v.Frame.WPM))}
	for _, row := range sparkline(v.Throughput, w-4, h-3) {
		body = append(body, plain(s, s.OK).render(row))
	}
	return box(s, w, h, caption(s, text.RoundThroughput), "", "", body)
}

// sparkline draws the newest values that fit on the given number of rows, with
// the newest at the right edge. It scales to the values' own range, because a
// typing speed moves little against zero and the movement is the point.
func sparkline(values []float64, width, height int) []string {
	if len(values) > width {
		values = values[len(values)-width:]
	}
	lines := make([]string, height)
	if len(values) == 0 {
		return lines
	}
	low, high := slices.Min(values), slices.Max(values)
	levels := height * len(sparkLevels)
	indent := strings.Repeat(" ", width-len(values))
	for row := range lines {
		var b strings.Builder
		b.WriteString(indent)
		floor := (height - 1 - row) * len(sparkLevels)
		for _, x := range values {
			n := 1
			if high > low {
				n = max(n, int(math.Ceil((x-low)/(high-low)*float64(levels))))
			}
			switch {
			case n <= floor:
				b.WriteRune(' ')
			case n-floor >= len(sparkLevels):
				b.WriteRune('█')
			default:
				b.WriteRune(sparkLevels[n-floor-1])
			}
		}
		lines[row] = b.String()
	}
	return lines
}

// velinStats is the panel of plain statistics.
func velinStats(s style.Style, f play.Frame, w, h int) []string {
	series := caption(s, text.RoundStreak+" ") + plain(s, s.Fg).render(strconv.Itoa(streak(f)))
	accuracy := caption(s, text.RoundAccuracy+" ") + plain(s, s.Fg).render(accuracyText(f))
	errors := caption(s, text.RoundErrors+" ") + plain(s, s.Fg).render(strconv.Itoa(len(f.WrongPos)))
	return box(s, w, h, caption(s, text.RoundPanelStats), "", "", []string{series, accuracy, errors})
}

// velinText is the wide panel in the middle.
func velinText(s style.Style, v View, rows []row, w, h int) []string {
	f := v.Frame
	inner := h - 2
	textH := max(1, inner)

	cursor := rowOf(rows, f.Pos)
	focus := rows[cursor]
	paint := ink{s: s, f: f}
	paint.base = func(i int) color.Color {
		switch {
		case i < f.Pos:
			return s.OK
		case i < focus.to:
			return s.Fg
		}
		return velinLabel
	}
	// A short text floats a little down the panel, nearer the map than the edge.
	body := make([]string, max(0, textH-len(rows))/3)
	top := scrollTop(len(rows), cursor, textH)
	for i := top; i < min(top+textH, len(rows)); i++ {
		body = append(body, paint.row(rows[i], s.Bg))
	}

	right := plain(s, s.Dim).render(stars(v.Stars))
	if f.Paused {
		right = pausedChip(s) + " " + right
	}
	footer := ""
	switch {
	case f.AltWarning:
		footer = warning(s, v.AltHelp)
	case f.Paused && !f.ManualPause:
		footer = plain(s, s.Warn).render(text.RoundResume)
	}
	return box(s, w, h, caption(s, v.Title), right, footer, body)
}

// velinMap holds the keyboard map during a hint and neutral stats otherwise.
func velinMap(s style.Style, v View, rows []row, w int) []string {
	if v.Frame.Hint != nil {
		return box(s, w, velinBottomH, caption(s, text.RoundPanelKeyboard), "", "", keyboard(s, v, w-4))
	}
	body := overview(s, v.Frame, rows, w-4)
	body[len(body)-1] = overviewRow(s, text.RoundStreak, strconv.Itoa(streak(v.Frame))) // weakest has a panel here
	return box(s, w, velinBottomH, caption(s, text.RoundPanelOverview), "", "", body)
}

// velinNext shows how to type the next character, only during a hint, and the
// weakest characters, which reveal nothing about it.
func velinNext(s style.Style, f play.Frame, w int) []string {
	inner := w - 4
	body := make([]string, keyboardRows)
	if f.Hint != nil {
		char, rest, ok := strings.Cut(f.Hint.Text, " → ")
		if !ok {
			char, rest = "", f.Hint.Text
		}
		chord, finger := rest, ""
		if i := strings.LastIndex(rest, " ("); i >= 0 {
			chord, finger = rest[:i], strings.Trim(rest[i+1:], "()")
		}
		rows := wrapProse(char+" "+chord, inner, keyboardRows-2)
		for i, row := range rows {
			body[i] = plain(s, s.Accent).render(row)
		}
		body[len(rows)] = caption(s, finger)
	}
	body[keyboardRows-1] = caption(s, text.RoundWeakest+" ") + plain(s, s.Bad).render(weakText(f))
	return box(s, w, velinBottomH, caption(s, text.RoundNextChar), "", "", body)
}
