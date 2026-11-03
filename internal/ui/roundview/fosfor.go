package roundview

import (
	"image/color"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/jiripisa/tuipe/internal/play"
	"github.com/jiripisa/tuipe/internal/ui/style"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

const (
	fosforColumn  = 66 // widest text column, as in the mockup
	fosforMaxText = 17 // text rows at most; the rest of a tall window is margin
)

// afterglow is typed text on the rows above the focus: cooling phosphor, paler
// than the fresh green of text still to come.
var afterglow color.Color = lipgloss.Color("#54855f")

// fosfor is the CRT: one lit line that stays on the same row while the text
// moves under it, the numbers below, and a dark screen until a hint needs the map.
func fosfor(s style.Style, v View, w, h int) []string {
	f := v.Frame
	col := min(w-3, fosforColumn) // room for a 56-cell line and its mark at width 60
	margin := strings.Repeat(" ", (w-col)/2)

	below := fosforBelow(s, f, col, v.AltHelp)
	gap := 1
	if h < 20 {
		gap = 0
	}
	rows := wrap(f.Cells, col)
	room := max(1, h-1-gap-len(below)-hintRows)
	textH := min(room, fosforMaxText)
	above := min(contextAbove(textH), len(rows)-1)
	textH = min(textH, above+len(rows)) // a short text keeps the numbers close to it
	spare := room - textH

	lines := []string{fosforHeader(s, v, w)}
	lines = append(lines, make([]string, gap+spare/2)...)
	lines = append(lines, fosforText(s, f, rows, above, textH, margin)...)
	for _, row := range below {
		lines = append(lines, margin+row)
	}
	lines = append(lines, make([]string, h-hintRows-len(lines))...)

	if f.Hint != nil {
		lines = append(lines, margin+hintLine(s, f.Hint))
	} else {
		lines = append(lines, "")
	}
	return append(lines, keyboard(s, v, w-2)...)
}

func fosforHeader(s style.Style, v View, w int) string {
	left := text.AppName
	if v.Title != "" {
		left += "  ::  " + v.Title
	}
	right := plain(s, s.Accent).render(stars(v.Stars))
	return " " + spread(plain(s, s.Dim).render(left), right+" ", w-1, plain(s, s.Dim))
}

// fosforText draws the window: the focus row stays above rows down the window
// through the whole round, with blank rows where the text has not reached yet or
// has run out.
func fosforText(s style.Style, f play.Frame, rows []row, above, height int, margin string) []string {
	cursor := rowOf(rows, f.Pos)
	focus := rows[cursor]
	paint := ink{s: s, f: f}
	paint.base = func(i int) color.Color {
		switch {
		case i < f.Pos && i >= focus.from:
			return s.Accent
		case i < f.Pos:
			return afterglow
		case i < focus.to:
			return s.Fg
		}
		return s.Dim
	}
	lines := make([]string, height)
	first := cursor - above
	for i := range lines {
		if n := first + i; n >= 0 && n < len(rows) {
			lines[i] = margin + paint.row(rows[n], s.Bg)
		}
	}
	return lines
}

// fosforBelow is what follows the text: the numbers and a message row.
func fosforBelow(s style.Style, f play.Frame, col int, altHelp string) []string {
	lines := []string{""}
	numbers := []string{
		plain(s, s.Accent).render(speedText(f.WPM)),
		plain(s, s.Accent).render(accuracyText(f)),
		plain(s, s.Fg).render(text.RoundStreak + " " + strconv.Itoa(streak(f))),
	}
	if f.Paused {
		numbers = append([]string{pausedChip(s)}, numbers...)
	}
	lines = append(lines, strings.Join(numbers, "   "))

	switch {
	case f.AltWarning:
		return append(lines, warning(s, altHelp))
	case f.Paused:
		return append(lines, pausedNote(s, f))
	}
	return append(lines, "")
}
