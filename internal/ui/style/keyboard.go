package style

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jiripisa/tuipe/internal/layout"
)

// Pseudo key ids for the modifier row. Modifiers are not layout keys.
const (
	ShiftLeft   = "shift-left"
	ShiftRight  = "shift-right"
	OptionLeft  = "option-left"
	OptionRight = "option-right"
)

// stagger is the indent of each letter row in quarter key widths: the number
// row, the QWERTY row, the home row and the bottom row.
var stagger = []int{0, 2, 3, 1}

// spaceID is the layout key of the space bar.
const spaceID = "k49"

type cell struct {
	id, label string
	width     int
}

// Keyboard draws the keyboard map of l no wider than w columns. highlight lists
// key ids to mark (layout ids and the pseudo ids Shift/Option left/right) and
// heat maps key ids to 0..1 weakness. Keys get narrower rather than wrapping.
func Keyboard(s Style, l *layout.Layout, highlight []string, heat map[string]float64, w int) string {
	mark := map[string]bool{}
	for _, id := range highlight {
		mark[id] = true
	}
	var kw int
	rows := [][]cell{}
	for _, kw = range []int{5, 3, 1} {
		rows = buildRows(l, kw)
		if rowsWidth(rows, kw) <= w {
			break
		}
	}

	var out []string
	for i, row := range rows {
		indent := 0
		if i < len(stagger) && i < len(rows)-1 {
			indent = stagger[i] * (kw + 1) / 4
		}
		line := strings.Repeat(" ", indent)
		for _, k := range row {
			line += renderKey(s, k, mark[k.id], heat[k.id]) + " "
		}
		out = append(out, ansi.Truncate(strings.TrimRight(line, " "), w, ""))
	}
	return strings.Join(out, "\n")
}

func rowsWidth(rows [][]cell, kw int) int {
	widest := 0
	for i, row := range rows {
		n := 0
		if i < len(stagger) && i < len(rows)-1 {
			n = stagger[i] * (kw + 1) / 4
		}
		for _, k := range row {
			n += k.width + 1
		}
		widest = max(widest, n-1)
	}
	return widest
}

// buildRows lays out every row but the last as letter keys and ends with the
// modifier row around the space bar.
func buildRows(l *layout.Layout, kw int) [][]cell {
	var rows [][]cell
	var space *layout.Key
	for _, r := range l.Rows {
		var row []cell
		for _, k := range r {
			if k.ID == spaceID {
				k := k
				space = &k
				continue
			}
			row = append(row, cell{k.ID, k.Label, kw})
		}
		if len(row) > 0 {
			rows = append(rows, row)
		}
	}
	mod := "⌥"
	if l.ModifierName != "" && l.ModifierName != "Option" {
		mod = "Alt"
	}
	wide := kw + 2
	bar := 5*(kw+1) - 1
	row := []cell{{ShiftLeft, "⇧", wide}, {OptionLeft, mod, kw}}
	if space != nil {
		row = append(row, cell{space.ID, space.Label, bar})
	}
	row = append(row, cell{OptionRight, mod, kw}, cell{ShiftRight, "⇧", wide})
	return append(rows, row)
}

func renderKey(s Style, k cell, marked bool, heat float64) string {
	label := ansi.Truncate(k.label, k.width, "")
	pad := k.width - ansi.StringWidth(label)
	label = strings.Repeat(" ", pad/2) + label + strings.Repeat(" ", pad-pad/2)
	st := lipgloss.NewStyle().Foreground(s.Dim).Background(s.Bg)
	switch {
	case marked:
		st = lipgloss.NewStyle().Foreground(s.Bg).Background(s.Accent).Bold(true)
	case heat > 0:
		st = lipgloss.NewStyle().Foreground(s.Fg).Background(blend(s.Bg, s.Bad, min(heat, 1)))
	}
	return st.Render(label)
}

// blend mixes from towards to by t in 0..1.
func blend(from, to color.Color, t float64) color.Color {
	fr, fg, fb, _ := from.RGBA()
	tr, tg, tb, _ := to.RGBA()
	mix := func(a, b uint32) uint8 {
		return uint8((float64(a)*(1-t) + float64(b)*t) / 257)
	}
	return color.RGBA{mix(fr, tr), mix(fg, tg), mix(fb, tb), 255}
}
