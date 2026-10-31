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
	raceLabelW    = 7  // "HEAD~1 "
	raceLeadW     = 20 // the lead text beside the ghost's bar
)

// afterglow is typed text on the rows above the focus: cooling phosphor, paler
// than the fresh green of text still to come.
var afterglow color.Color = lipgloss.Color("#54855f")

// eighths are the partial blocks at the end of the player's bar.
var eighths = []rune("▏▎▍▌▋▊▉")

// fosfor is the CRT: one lit line that stays on the same row while the text
// moves under it, the race below, and a dark screen until a hint needs the map.
func fosfor(s style.Style, v View, w, h int) []string {
	f := v.Frame
	col := min(w-3, fosforColumn) // room for a 56-cell line and its mark at width 60
	margin := strings.Repeat(" ", (w-col)/2)

	prompt := wrapProse(v.Prompt, col, promptMaxRows)
	below := fosforBelow(s, f, col)
	gap := 1
	if h < 20 {
		gap = 0
	}
	rows := wrap(f.Cells, col)
	room := max(1, h-1-gap-len(prompt)-len(below)-hintRows)
	textH := min(room, fosforMaxText)
	above := min(contextAbove(textH), len(rows)-1)
	textH = min(textH, above+len(rows)) // a short text keeps the race close to it
	spare := room - textH

	lines := []string{fosforHeader(s, v, w)}
	lines = append(lines, make([]string, gap+spare/2)...)
	for _, row := range prompt {
		lines = append(lines, margin+plain(s, s.Fg).render(row))
	}
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
	if v.RightInfo != "" {
		right = plain(s, s.Fg).render(v.RightInfo) + "  " + right
	}
	return " " + spread(plain(s, s.Dim).render(left), right+" ", w-1, plain(s, s.Dim))
}

// fosforText draws the window: the focus row stays above rows down the window
// through the whole round, with blank rows where the text has not reached yet or
// has run out.
func fosforText(s style.Style, f play.Frame, rows []row, above, height int, margin string) []string {
	cursor := rowOf(rows, f.Pos)
	focus := rows[cursor]
	paint := ink{s: s, f: f, ghostFg: s.Accent}
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

// fosforBelow is what follows the text: the race, the numbers and a message row.
func fosforBelow(s style.Style, f play.Frame, col int) []string {
	lines := []string{""}
	if f.HasGhost {
		lines = append(lines, fosforRace(s, f, col)...)
		lines = append(lines, "")
	}
	numbers := []string{
		plain(s, s.Accent).render(wpmText(f.WPM)),
		plain(s, s.Accent).render(accuracyText(f)),
		plain(s, s.Fg).render(text.RoundStreak + " " + strconv.Itoa(streak(f))),
	}
	if f.HasSLA {
		fg := slaColour(s, f)
		numbers = append(numbers, plain(s, fg).render(text.RoundSLA+" "+slaText(f.SLA)+" ")+
			meter(s, slaLeft(f.SLA), 8, fg))
	}
	if f.Paused {
		numbers = append([]string{pausedChip(s)}, numbers...)
	}
	lines = append(lines, strings.Join(numbers, "   "))

	switch {
	case f.AltWarning:
		return append(lines, warning(s))
	case f.Paused:
		return append(lines, pausedNote(s, f))
	}
	return append(lines, "")
}

// fosforRace draws the two bars on one scale. The ghost's bar stays in the ghost
// colour, as its marker does in the text.
func fosforRace(s style.Style, f play.Frame, col int) []string {
	track := max(1, col-raceLabelW-raceLeadW)
	total := max(1, len(f.Cells))
	label := func(name string, fg color.Color) string {
		return plain(s, fg).render(name + strings.Repeat(" ", raceLabelW-len(name)))
	}

	ghostCells := min(track, f.GhostPos*track/total)
	ghostBar := look{fg: s.Dim, bg: s.Ghost}.render(strings.Repeat("░", ghostCells)) +
		look{fg: s.Accent, bg: s.Ghost}.render("▒")
	rest := strings.Repeat(" ", track-ghostCells)

	word := text.RoundAhead
	if leadText(f.Lead)[0] == '-' {
		word = text.RoundBehind
	}
	lead := plain(s, leadColour(s, f.Lead)).render(leadText(f.Lead)) +
		plain(s, s.Dim).render(" "+word)

	return []string{
		label(text.RoundGhost, s.Dim) + ghostBar + rest + lead,
		label(text.RoundYou, s.Accent) + plain(s, s.Accent).render(blocks(float64(f.Pos)/float64(total), track)),
	}
}

// blocks draws a bar of up to width cells to an eighth of a cell.
func blocks(share float64, width int) string {
	n := int(max(0, min(share, 1)) * float64(width) * 8)
	bar := strings.Repeat("█", n/8)
	if n%8 > 0 {
		bar += string(eighths[n%8-1])
	}
	return bar
}
