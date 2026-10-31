// Package style holds the three visual styles and the drawing helpers every
// screen shares.
package style

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// ID names a style in the config.
type ID string

// The three styles.
const (
	Fosfor ID = "fosfor"
	Editor ID = "editor"
	Velin  ID = "velin"
)

// Style is a colour scheme with a border character.
type Style struct {
	ID                                        ID
	Name                                      string
	Bg, Fg, Dim, Accent, OK, Warn, Bad, Ghost color.Color
	Border                                    lipgloss.Border
}

func c(hex string) color.Color { return lipgloss.Color(hex) }

var all = []Style{
	{
		ID: Fosfor, Name: "Fosfor",
		Bg: c("#050805"), Fg: c("#3fae55"), Dim: c("#1e5a2c"), Accent: c("#9dffb0"),
		OK: c("#9dffb0"), Warn: c("#3fae55"), Bad: c("#ff5a4a"), Ghost: c("#143f20"),
		Border: lipgloss.NormalBorder(),
	},
	{
		ID: Editor, Name: "Editor",
		Bg: c("#1a1b26"), Fg: c("#a9b1d6"), Dim: c("#565f89"), Accent: c("#e0af68"),
		OK: c("#9ece6a"), Warn: c("#e0af68"), Bad: c("#f7768e"), Ghost: c("#3d2f5c"),
		Border: lipgloss.NormalBorder(),
	},
	{
		ID: Velin, Name: "Velín",
		Bg: c("#0b0d10"), Fg: c("#c9d1d9"), Dim: c("#545d68"), Accent: c("#e0823d"),
		OK: c("#57ab5a"), Warn: c("#e0823d"), Bad: c("#e5534b"), Ghost: c("#3a2a55"),
		Border: lipgloss.RoundedBorder(),
	},
}

// All returns the styles in cycling order.
func All() []Style { return append([]Style(nil), all...) }

// ByID returns the style with the given id. An unknown id gives Editor.
func ByID(id ID) Style {
	for _, s := range all {
		if s.ID == id {
			return s
		}
	}
	return all[1]
}

// Next returns the id after id in the cycle.
func Next(id ID) ID {
	for i, s := range all {
		if s.ID == id {
			return all[(i+1)%len(all)].ID
		}
	}
	return all[0].ID
}

// Base is the style of ordinary text: Fg on Bg.
func (s Style) Base() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(s.Fg).Background(s.Bg)
}

// Paint pads or cuts line to exactly w columns and paints Bg behind it, also
// behind the parts that were reset by styled text inside.
func (s Style) Paint(line string, w int) string {
	if w <= 0 {
		return ""
	}
	base := s.Base()
	probe := base.Render("X")
	prefix := probe[:strings.Index(probe, "X")]
	line = ansi.Truncate(line, w, "")
	if pad := w - ansi.StringWidth(line); pad > 0 {
		line += strings.Repeat(" ", pad)
	}
	line = strings.ReplaceAll(line, "\x1b[0m", "\x1b[0m"+prefix)
	line = strings.ReplaceAll(line, "\x1b[m", "\x1b[m"+prefix)
	return prefix + line + "\x1b[0m"
}

// Fill paints content over a w × h window, cutting or padding every line.
func (s Style) Fill(content string, w, h int) string {
	lines := strings.Split(content, "\n")
	out := make([]string, h)
	for i := range out {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		out[i] = s.Paint(line, w)
	}
	return strings.Join(out, "\n")
}

// ClipHook, when set, is called by Frame with a description each time it
// discards a body row or cuts a line that did not fit. It is nil in production;
// tests set it to make clipping visible.
var ClipHook func(what string)

func clipped(what string) {
	if ClipHook != nil {
		ClipHook(what)
	}
}

// Frame draws the structure shared by all screens in a w × h box: a border with
// the title in its top edge, the body, and the footer on the last inner line.
func Frame(s Style, w, h int, title, body, footer string) string {
	if w < 4 || h < 3 {
		return s.Fill(body, w, h)
	}
	b := s.Border
	edge := lipgloss.NewStyle().Foreground(s.Dim).Background(s.Bg)
	head := lipgloss.NewStyle().Foreground(s.Accent).Background(s.Bg).Bold(true)
	iw, ih := w-2, h-2

	titleText := ""
	if title != "" && iw >= 5 { // narrower, the title would push the corner out of the box
		titleText = " " + ansi.Truncate(title, iw-4, "…") + " "
	}
	rest := max(0, iw-1-ansi.StringWidth(titleText))
	top := edge.Render(b.TopLeft+b.Top) + head.Render(titleText) +
		edge.Render(strings.Repeat(b.Top, rest)+b.TopRight)
	lines := []string{top}

	bodyRows := ih
	if footer != "" {
		bodyRows--
	}
	bl := strings.Split(body, "\n")
	for i, l := range bl {
		switch {
		case i >= bodyRows && strings.TrimSpace(ansi.Strip(l)) != "":
			clipped("body row " + ansi.Strip(l))
		case i < bodyRows && ansi.StringWidth(l) > iw:
			clipped("body line " + ansi.Strip(l))
		}
	}
	if ansi.StringWidth(footer) > iw {
		clipped("footer " + footer)
	}
	for i := 0; i < ih; i++ {
		var inner string
		switch {
		case i < bodyRows && i < len(bl):
			inner = bl[i]
		case i == ih-1 && footer != "":
			inner = lipgloss.NewStyle().Foreground(s.Dim).Render(footer)
		}
		lines = append(lines, edge.Render(b.Left)+s.Paint(inner, iw)+edge.Render(b.Right))
	}
	lines = append(lines, edge.Render(b.BottomLeft+strings.Repeat(b.Bottom, iw)+b.BottomRight))
	return strings.Join(lines, "\n")
}
