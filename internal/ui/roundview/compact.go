package roundview

import "github.com/jiripisa/tuipe/internal/ui/style"

// compact is the strip for small windows: core numbers on top, then the text.
// It keeps one column free at each edge.
func compact(s style.Style, v View, w, h int) []string {
	f := v.Frame
	numbers := plain(s, s.Accent).render(wpmText(f.WPM)) + "  " +
		plain(s, s.Fg).render(accuracyText(f))
	if f.HasGhost {
		numbers += "  " + plain(s, leadColour(s, f.Lead)).render(leadText(f.Lead))
	}
	if f.Paused {
		numbers = pausedChip(s) + " " + numbers
	}
	lines := []string{" " + numbers}

	textH := h - 1
	warn := f.AltWarning && textH > 1
	if warn {
		textH--
	}
	rows := wrap(f.Cells, w-2)
	paint := ink{s: s, f: f, base: typedThen(f, s.OK, s.Fg), ghostFg: s.Fg}
	top := scrollTop(len(rows), rowOf(rows, f.Pos), textH)
	for i := top; i < min(top+textH, len(rows)); i++ {
		lines = append(lines, " "+paint.row(rows[i], s.Bg))
	}
	if warn {
		lines = append(lines, make([]string, h-1-len(lines))...)
		lines = append(lines, " "+warning(s))
	}
	return lines
}
