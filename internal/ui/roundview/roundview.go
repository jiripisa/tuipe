// Package roundview draws the typing round: the Fosfor, Editor and Velín
// layouts and the compact strip for small windows. Rendering is a pure function
// of the View, so the screen keeps no scroll or layout state between frames.
package roundview

import (
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jiripisa/tuipe/internal/engine"
	"github.com/jiripisa/tuipe/internal/layout"
	"github.com/jiripisa/tuipe/internal/play"
	"github.com/jiripisa/tuipe/internal/ui/style"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

const (
	minW, minH = 60, 18 // below this the compact strip replaces the layouts

	keyboardRows = 5
	hintRows     = keyboardRows + 1 // the hint line above the map
	maxStars     = 3

	overviewLabelW = 11
)

// View is everything the round screen shows.
type View struct {
	Frame       play.Frame
	Title, Lang string
	Layout      *layout.Layout
	Stars       int       // best stars of the lesson; negative when the round has no lesson
	Throughput  []float64 // recent WPM samples, oldest first; may be empty
	AltHelp     string    // how to fix Option on this terminal, when known
}

// Render draws v to fit w × h exactly: the layout of the style, or the compact
// strip below 60 × 18.
func Render(s style.Style, v View, w, h int) string {
	if w <= 0 || h <= 0 {
		return ""
	}
	var lines []string
	switch {
	case w < minW || h < minH:
		lines = compact(s, v, w, h)
	case s.ID == style.Fosfor:
		lines = fosfor(s, v, w, h)
	case s.ID == style.Velin:
		lines = velin(s, v, w, h)
	default:
		lines = editor(s, v, w, h)
	}
	return s.Fill(strings.Join(lines, "\n"), w, h)
}

// look is the appearance of a run of cells. It is comparable, so neighbouring
// cells can share one escape sequence.
type look struct {
	fg, bg          color.Color
	bold, underline bool
}

func (l look) render(s string) string {
	return lipgloss.NewStyle().Foreground(l.fg).Background(l.bg).
		Bold(l.bold).Underline(l.underline).Render(s)
}

// plain is fg on the window background.
func plain(s style.Style, fg color.Color) look { return look{fg: fg, bg: s.Bg} }

// row is one visual line of text: the cells from..to-1.
type row struct {
	from, to int
	line     int  // source line, from 1
	cont     bool // continues a soft-wrapped source line
}

// glyph is what a cell shows. A newline becomes a mark, so the user knows to
// press Enter, and a tab has no width of its own.
func glyph(c engine.Cell) string {
	switch c.R {
	case '\n':
		return "↵"
	case '\t':
		return "    "
	}
	return string(c.R)
}

// wrap splits cells into rows at most width cells wide. It looks only at the
// text and the width, never at the position, so text does not shift while
// typing. Newlines always end a row and take a cell themselves. A row breaks
// after the last space that fits, or inside a word longer than the row.
func wrap(cells []engine.Cell, width int) []row {
	width = max(width, 1)
	var rows []row
	line, start, used, afterSpace, cont := 1, 0, 0, -1, false
	add := func(end int) {
		rows = append(rows, row{from: start, to: end, line: line, cont: cont})
		start = end
	}
	for i, c := range cells {
		cw := ansi.StringWidth(glyph(c))
		if used+cw > width && i > start {
			end := i
			if afterSpace > start {
				end = afterSpace
			}
			add(end)
			cont, afterSpace, used = true, -1, 0
			for _, kept := range cells[start:i] {
				used += ansi.StringWidth(glyph(kept))
			}
		}
		used += cw
		switch {
		case c.R == '\n':
			add(i + 1)
			line, cont, used, afterSpace = line+1, false, 0, -1
		case c.R == ' ' && !c.Auto:
			afterSpace = i + 1
		}
	}
	if start < len(cells) || len(rows) == 0 {
		add(len(cells))
	}
	return rows
}

// WrapSpans breaks cells into rows at most width cells wide, as the round does,
// and returns each row as the half-open range [from, to) of cell indexes.
func WrapSpans(cells []engine.Cell, width int) [][2]int {
	rows := wrap(cells, width)
	out := make([][2]int, len(rows))
	for i, r := range rows {
		out[i] = [2]int{r.from, r.to}
	}
	return out
}

// rowOf returns the index of the row holding cell pos, or the last row when pos
// is past the end.
func rowOf(rows []row, pos int) int {
	for i, r := range rows {
		if pos < r.to {
			return i
		}
	}
	return len(rows) - 1
}

// contextAbove is how far below the top of a window of the given height the
// cursor row sits: a third of the way down, and at least one row when there is
// room for it.
func contextAbove(height int) int {
	return min(height-1, max(1, (height-1)/3))
}

// scrollTop returns the first row of a window of the given height. The cursor
// row keeps its context above, and the window stops at the end of the text.
func scrollTop(total, cursor, height int) int {
	return max(0, min(cursor-contextAbove(height), total-height))
}

// ink colours the cells of a frame. base gives a layout's own colour of a cell,
// and ink adds what all layouts share: indentation, errors and cursor.
type ink struct {
	s    style.Style
	f    play.Frame
	base func(i int) color.Color
}

func (k ink) look(i int, bg color.Color) look {
	c := k.f.Cells[i]
	l := look{fg: k.base(i), bg: bg}
	if c.Auto || c.R == '\n' {
		l.fg = k.s.Dim
	}
	wrong := k.f.WrongPos[i] && !c.Auto
	if wrong {
		l.fg, l.underline = k.s.Bad, true
		if c.R == ' ' {
			l.bg = k.s.Bad
		}
	}
	if i == k.f.Pos && !k.f.Done {
		l = look{fg: k.s.Bg, bg: k.s.Accent, bold: true}
		switch {
		case k.f.Paused:
			l.bg = k.s.Dim
		case wrong:
			l.bg = k.s.Bad
		}
	}
	return l
}

// row draws the cells of r on the background bg.
func (k ink) row(r row, bg color.Color) string {
	var out, run strings.Builder
	var cur look
	for i := r.from; i < r.to; i++ {
		l := k.look(i, bg)
		if run.Len() > 0 && l != cur {
			out.WriteString(cur.render(run.String()))
			run.Reset()
		}
		cur = l
		run.WriteString(glyph(k.f.Cells[i]))
	}
	if run.Len() > 0 {
		out.WriteString(cur.render(run.String()))
	}
	return out.String()
}

// typedThen colours cells typed behind the cursor and untyped after it.
func typedThen(f play.Frame, typed, untyped color.Color) func(int) color.Color {
	return func(i int) color.Color {
		if i < f.Pos {
			return typed
		}
		return untyped
	}
}

// spread puts left and right on one row of w cells. Left gives way when the row
// is too short, so the countdown and the stars stay visible.
func spread(left, right string, w int, fill look) string {
	room := max(0, w-ansi.StringWidth(right))
	if ansi.StringWidth(left) > room {
		left = ansi.Truncate(left, max(0, room-1), "…")
	}
	gap := max(0, w-ansi.StringWidth(left)-ansi.StringWidth(right))
	return left + fill.render(strings.Repeat(" ", gap)) + right
}

// wrapProse breaks a sentence into at most maxRows rows of width cells. What
// does not fit ends with an ellipsis.
func wrapProse(sentence string, width, maxRows int) []string {
	if sentence == "" {
		return nil
	}
	rows := strings.Split(ansi.Wordwrap(sentence, max(width, 1), ""), "\n")
	if len(rows) > maxRows {
		rest := strings.Join(rows[maxRows-1:], " ")
		rows = append(rows[:maxRows-1], ansi.Truncate(rest, width, "…"))
	}
	for i, r := range rows {
		rows[i] = strings.TrimRight(r, " ")
	}
	return rows
}

func stars(n int) string {
	if n < 0 { // a round without a lesson has none to show
		return ""
	}
	n = min(n, maxStars)
	return strings.Repeat("★", n) + strings.Repeat("☆", maxStars-n)
}

func wpmText(v float64) string { return fmt.Sprintf("%.0f %s", v, text.RoundWPM) }

// accuracyText leaves the percentage out until a key has been pressed, because
// 0 % would read as a verdict.
func accuracyText(f play.Frame) string {
	if f.Accuracy == 0 && len(f.WrongPos) == 0 {
		return "-- %"
	}
	return fmt.Sprintf("%.1f %%", f.Accuracy*100)
}

// streak counts the cells typed since the last wrong press.
func streak(f play.Frame) int {
	from := 0
	for p := range f.WrongPos {
		from = max(from, p+1)
	}
	n := 0
	for i := from; i < min(f.Pos, len(f.Cells)); i++ {
		if !f.Cells[i].Auto {
			n++
		}
	}
	return n
}

// progress reports how many typeable cells are done and how many there are.
func progress(f play.Frame) (done, total int) {
	for i, c := range f.Cells {
		if c.Auto {
			continue
		}
		total++
		if i < f.Pos {
			done++
		}
	}
	return done, total
}

func progressShare(f play.Frame) float64 {
	done, total := progress(f)
	if total == 0 {
		return 0
	}
	return float64(done) / float64(total)
}

func percent(share float64) string { return fmt.Sprintf("%.0f %%", share*100) }

// meter draws a bar of width cells, solid up to share and shaded after it.
func meter(s style.Style, share float64, width int, fg color.Color) string {
	if width <= 0 {
		return ""
	}
	n := int(math.Round(max(0, min(share, 1)) * float64(width)))
	return plain(s, fg).render(strings.Repeat("█", n)) +
		plain(s, s.Dim).render(strings.Repeat("░", width-n))
}

// runeLabel shows a character on its own, where a space or newline would vanish.
func runeLabel(r rune) string {
	switch r {
	case ' ':
		return "␣"
	case '\n':
		return "↵"
	case '\t':
		return "⇥"
	}
	return string(r)
}

func weakText(f play.Frame) string {
	if len(f.Weak) == 0 {
		return "—"
	}
	parts := make([]string, len(f.Weak))
	for i, r := range f.Weak {
		parts[i] = runeLabel(r)
	}
	return strings.Join(parts, " ")
}

// hintLine draws a hint such as "{ → levý Option + í (pravý prsteník)" with the
// character as a key cap and the chord brighter than the finger.
func hintLine(s style.Style, h *play.HintInfo) string {
	char, rest, ok := strings.Cut(h.Text, " → ")
	if !ok {
		return plain(s, s.Accent).render(h.Text)
	}
	chord, finger := rest, ""
	if i := strings.LastIndex(rest, " ("); i >= 0 {
		chord, finger = rest[:i], rest[i:]
	}
	if utf8.RuneCountInString(char) > 1 { // "mezera" and "Enter" read as words
		char = plain(s, s.Accent).render(char)
	} else {
		char = look{fg: s.Bg, bg: s.Accent, bold: true}.render(char)
	}
	return char + plain(s, s.Dim).render(" → ") + plain(s, s.Accent).render(chord) +
		plain(s, s.Dim).render(finger)
}

// Highlights lists the keys of a route for the keyboard map: the layout key ids
// and the pseudo ids of the Shift or Option to hold. A modifier is held by the
// hand opposite the key, as the hint text says.
func Highlights(l *layout.Layout, route layout.Route) []string {
	var ids []string
	for _, c := range route {
		ids = append(ids, c.Key)
		key, _ := l.Key(c.Key)
		left := key.Hand == layout.Right
		for _, m := range c.Mods {
			switch {
			case m == "shift" && left:
				ids = append(ids, style.ShiftLeft)
			case m == "shift":
				ids = append(ids, style.ShiftRight)
			case left:
				ids = append(ids, style.OptionLeft)
			default:
				ids = append(ids, style.OptionRight)
			}
		}
	}
	return ids
}

// keyboard draws the map for the active hint, centred in w cells and always
// keyboardRows high. Without a hint or a layout the rows stay empty.
func keyboard(s style.Style, v View, w int) []string {
	rows := make([]string, keyboardRows)
	h := v.Frame.Hint
	if h == nil || v.Layout == nil {
		return rows
	}
	drawn := strings.Split(style.Keyboard(s, v.Layout, Highlights(v.Layout, h.Route), nil, w), "\n")
	widest := 0
	for _, line := range drawn {
		widest = max(widest, ansi.StringWidth(line))
	}
	indent := strings.Repeat(" ", max(0, (w-widest)/2))
	for i := range min(len(rows), len(drawn)) {
		rows[i] = indent + drawn[i]
	}
	return rows
}

// overview is the neutral statistics shown where the map would be: nothing in
// it says how to type the next character.
func overview(s style.Style, f play.Frame, rows []row, w int) []string {
	done, total := progress(f)
	share := progressShare(f)
	bar := meter(s, share, max(0, min(w-overviewLabelW-7, 30)), s.Dim)
	return []string{
		overviewRow(s, text.RoundProgress, "") + bar + plain(s, s.Fg).render("  "+percent(share)),
		overviewRow(s, text.RoundChars, fmt.Sprintf("%d / %d", done, total)),
		overviewRow(s, text.RoundLine, fmt.Sprintf("%d / %d", rows[rowOf(rows, f.Pos)].line, rows[len(rows)-1].line)),
		overviewRow(s, text.RoundErrors, strconv.Itoa(len(f.WrongPos))),
		overviewRow(s, text.RoundWeak, weakText(f)),
	}
}

func overviewRow(s style.Style, name, value string) string {
	pad := strings.Repeat(" ", max(0, overviewLabelW-ansi.StringWidth(name)))
	return plain(s, s.Dim).render(name+pad) + plain(s, s.Fg).render(value)
}

// pausedChip marks the paused state the same way wherever it appears.
func pausedChip(s style.Style) string {
	return look{fg: s.Bg, bg: s.Warn, bold: true}.render(" " + text.RoundPaused + " ")
}

// pausedNote says how to go on. A manual pause has a menu of its own instead.
func pausedNote(s style.Style, f play.Frame) string {
	if f.ManualPause {
		return ""
	}
	return plain(s, s.Dim).render(text.RoundResume)
}

// warning is the line about Option arriving as Alt: the setting of this
// terminal when it is known, or the general advice.
func warning(s style.Style, help string) string {
	if help == "" {
		help = text.RoundAltWarning
	}
	return plain(s, s.Bad).render(help)
}
