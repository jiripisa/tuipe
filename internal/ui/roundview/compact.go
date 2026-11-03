package roundview

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/jiripisa/tuipe/internal/play"
	"github.com/jiripisa/tuipe/internal/ui/style"
)

const (
	compactNotes   = 2 // rows below the text for the hint and the warning
	compactMinText = 2 // rows of text that come before those notes
)

// compact is the strip for small windows: core numbers on top, the text, then a
// hint and a warning when there are any. A hint is one line without the map,
// so that even a small window shows it. It keeps one column free at each edge.
//
// The height alone decides the rows, so a hint opening never moves the text. In
// the smallest windows the text and its cursor come first, then the numbers,
// and the notes only take what is left.
func compact(s style.Style, v View, w, h int) []string {
	f := v.Frame
	var lines []string
	if h > 1 {
		lines = append(lines, " "+compactNumbers(s, f, w-2))
	}
	notes := max(0, min(compactNotes, h-1-compactMinText))
	textH := h - len(lines) - notes

	rows := wrap(f.Cells, w-2)
	paint := ink{s: s, f: f, base: typedThen(f, s.OK, s.Fg)}
	top := scrollTop(len(rows), rowOf(rows, f.Pos), textH)
	for i := top; i < min(top+textH, len(rows)); i++ {
		lines = append(lines, " "+paint.row(rows[i], s.Bg))
	}

	var shown []string
	if f.Hint != nil {
		shown = append(shown, " "+ansi.Truncate(hintLine(s, f.Hint), w-2, "…"))
	}
	if f.AltWarning {
		shown = append(shown, " "+warning(s, v.AltHelp))
	}
	shown = shown[:min(len(shown), notes)]
	lines = append(lines, make([]string, h-len(shown)-len(lines))...)
	return append(lines, shown...)
}

// compactNumbers joins the numbers on one row of at most w cells. The paused
// mark stays, and in a narrow window the numbers go from the left, so the
// speed, the first number, is the last to leave.
func compactNumbers(s style.Style, f play.Frame, w int) string {
	numbers := []string{
		plain(s, s.Accent).render(speedText(f.WPM)),
		plain(s, s.Fg).render(accuracyText(f)),
	}
	if f.Paused {
		w -= ansi.StringWidth(pausedChip(s)) + 1
	}
	for len(numbers) > 1 && ansi.StringWidth(strings.Join(numbers, "  ")) > w {
		numbers = numbers[1:]
	}
	row := strings.Join(numbers, "  ")
	if f.Paused {
		return pausedChip(s) + " " + row
	}
	return row
}
