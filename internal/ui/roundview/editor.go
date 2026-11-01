package roundview

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"
	"sync"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/charmbracelet/x/ansi"

	"github.com/jiripisa/tuipe/internal/engine"
	"github.com/jiripisa/tuipe/internal/play"
	"github.com/jiripisa/tuipe/internal/ui/style"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

// Colours of the Editor mockup that are not part of the shared style.
var (
	editorBar      color.Color = lipgloss.Color("#16161e") // tab line, status filler
	editorBarHi    color.Color = lipgloss.Color("#292e42") // status items
	editorMode     color.Color = lipgloss.Color("#7aa2f7")
	editorModeText color.Color = lipgloss.Color("#15161e")
	editorTabText  color.Color = lipgloss.Color("#c0caf5")
	editorGutter   color.Color = lipgloss.Color("#3b4261")
	editorCursorLn color.Color = lipgloss.Color("#1f2233")
	editorGhostFg  color.Color = lipgloss.Color("#d6c6ff")
)

// tokenClass is a kind of source token. Classes, not colours, are cached.
type tokenClass uint8

const (
	classPlain tokenClass = iota
	classKeyword
	classType
	classFunction
	classString
	classNumber
	classComment
	classPunctuation
)

var classColours = map[tokenClass]color.Color{
	classKeyword:     lipgloss.Color("#bb9af7"),
	classType:        lipgloss.Color("#2ac3de"),
	classFunction:    lipgloss.Color("#7aa2f7"),
	classString:      lipgloss.Color("#e0af68"),
	classNumber:      lipgloss.Color("#ff9e64"),
	classPunctuation: lipgloss.Color("#89ddff"),
}

// lexerNames maps a Lang to chroma's lexer. It is explicit because lexers.Get
// falls back to guessing from file patterns, and an unknown Lang must stay plain.
var lexerNames = map[string]string{
	"go": "go", "python": "python", "js": "javascript", "ts": "typescript",
	"shell": "bash", "sql": "sql",
}

// regexLexer covers the regex snippets, which chroma has no lexer for.
var regexLexer = chroma.MustNewLexer(&chroma.Config{Name: "regex"}, func() chroma.Rules {
	return chroma.Rules{"root": {
		{Pattern: `^#.*$`, Type: chroma.Comment},
		{Pattern: `\\.`, Type: chroma.LiteralString},
		{Pattern: `\[\^?\]?(?:\\.|[^\]\n])*\]`, Type: chroma.NameClass},
		{Pattern: `[()|]`, Type: chroma.Punctuation},
		{Pattern: `[*+?]|\{\d*,?\d*\}`, Type: chroma.Operator},
		{Pattern: `[\^$.]`, Type: chroma.Keyword},
		{Pattern: `(?s).`, Type: chroma.Text},
	}}
})

var fileNames = map[string]string{
	"go": "main.go", "python": "main.py", "js": "index.js", "ts": "index.ts",
	"shell": "run.sh", "sql": "query.sql", "regex": "pattern.re",
}

func lexerFor(lang string) chroma.Lexer {
	if lang == "regex" {
		return regexLexer
	}
	if name, ok := lexerNames[lang]; ok {
		return lexers.Get(name)
	}
	return nil
}

// syntaxKey names a text. The first cell's address is enough, because a round's
// cells never change, and holding the pointer keeps the address from being reused.
type syntaxKey struct {
	first *engine.Cell
	n     int
	lang  string
}

// syntaxCache holds the classes of the last text, since rendering runs every
// 100 ms and a round shows one text.
var syntaxCache struct {
	sync.Mutex
	key     syntaxKey
	classes []tokenClass
}

// syntaxClasses returns one class per cell, or nil when lang has no colouring.
func syntaxClasses(cells []engine.Cell, lang string) []tokenClass {
	if len(cells) == 0 {
		return nil
	}
	key := syntaxKey{&cells[0], len(cells), lang}
	syntaxCache.Lock()
	defer syntaxCache.Unlock()
	if syntaxCache.key != key {
		syntaxCache.key, syntaxCache.classes = key, tokenise(cells, lang)
	}
	return syntaxCache.classes
}

// tokenise classifies the cells. Colouring is decoration, so any failure in the
// lexer leaves the rest of the text plain instead of reaching the screen.
func tokenise(cells []engine.Cell, lang string) (classes []tokenClass) {
	lexer := lexerFor(lang)
	if lexer == nil {
		return nil
	}
	var src strings.Builder
	for _, c := range cells {
		src.WriteRune(c.R)
	}
	defer func() {
		if recover() != nil {
			classes = nil
		}
	}()
	tokens, err := lexer.Tokenise(nil, src.String())
	if err != nil {
		return nil
	}
	classes = make([]tokenClass, len(cells))
	i := 0
	for token := tokens(); token != chroma.EOF; token = tokens() {
		class := classify(token.Type)
		for _, r := range token.Value {
			if i == len(cells) || cells[i].R != r {
				return classes // the lexer normalised something: stop here
			}
			classes[i] = class
			i++
		}
	}
	return classes
}

func classify(t chroma.TokenType) tokenClass {
	switch {
	case t == chroma.KeywordType || t == chroma.NameClass || t == chroma.NameBuiltin:
		return classType
	case t == chroma.NameFunction:
		return classFunction
	case t.InCategory(chroma.Keyword):
		return classKeyword
	case t.InCategory(chroma.Comment):
		return classComment
	case t.InSubCategory(chroma.LiteralString):
		return classString
	case t.InSubCategory(chroma.LiteralNumber):
		return classNumber
	case t.InCategory(chroma.Operator), t.InCategory(chroma.Punctuation):
		return classPunctuation
	}
	return classPlain
}

// editor is the Neovim look: tab line, numbered buffer, a bottom split with the
// hint or neutral stats, status line and message line. The split is always
// there, so a hint opening never moves the text.
func editor(s style.Style, v View, w, h int) []string {
	f := v.Frame
	gutter := max(len(strconv.Itoa(lineCount(f))), 2) + 1 // 3 cells leave a 56-cell line and its mark at width 60
	rows := wrap(f.Cells, w-gutter)

	prompt := promptLines(v.Prompt, w-gutter, h-1-hintRows-2)
	bufferH := max(1, h-1-len(prompt)-hintRows-2)

	lines := []string{editorTabs(s, v, w)}
	for _, line := range prompt {
		pad := strings.Repeat(" ", gutter)
		lines = append(lines, spread(look{fg: s.Fg, bg: editorBar}.render(pad+line), "", w, look{fg: s.Fg, bg: editorBar}))
	}
	lines = append(lines, editorBuffer(s, v, rows, w, gutter, bufferH)...)
	lines = append(lines, editorHint(s, v, rows, w)...)
	return append(lines, editorStatus(s, f, rows, w), editorMessage(s, f, v.AltHelp))
}

func lineCount(f play.Frame) int {
	n := 1
	for _, c := range f.Cells {
		if c.R == '\n' {
			n++
		}
	}
	return n
}

func editorTabs(s style.Style, v View, w int) string {
	bar := look{fg: s.Dim, bg: editorBar}
	name, ok := fileNames[v.Lang]
	if !ok {
		name = text.RoundProseFile
	}
	left := look{fg: editorTabText, bg: s.Bg}.render(" "+name+" ") + bar.render("  "+v.Title)
	right := bar.render(stars(v.Stars) + " ")
	if v.RightInfo != "" {
		right = look{fg: s.Fg, bg: editorBar}.render(v.RightInfo+"  ") + right
	}
	return spread(left, right, w, bar)
}

func editorBuffer(s style.Style, v View, rows []row, w, gutter, height int) []string {
	f := v.Frame
	classes := syntaxClasses(f.Cells, v.Lang)
	paint := ink{s: s, f: f, ghostFg: editorGhostFg}
	paint.base = func(i int) color.Color {
		switch {
		case i < f.Pos:
			return s.OK
		case classes == nil:
			return s.Fg
		case classes[i] == classComment:
			return s.Dim
		}
		if c, ok := classColours[classes[i]]; ok {
			return c
		}
		return s.Fg
	}

	cursor := rowOf(rows, f.Pos)
	top := scrollTop(len(rows), cursor, height)
	lines := make([]string, height)
	for i := range lines {
		n := top + i
		if n >= len(rows) {
			lines[i] = plain(s, editorGutter).render(fmt.Sprintf("%*s ", gutter-1, "~"))
			continue
		}
		bg, fg := s.Bg, editorGutter
		if n == cursor {
			bg, fg = editorCursorLn, s.Accent
		}
		number := ""
		if !rows[n].cont {
			number = strconv.Itoa(rows[n].line)
		}
		line := look{fg: fg, bg: bg}.render(fmt.Sprintf("%*s ", gutter-1, number)) + paint.row(rows[n], bg)
		if pad := w - ansi.StringWidth(line); pad > 0 {
			line += look{fg: s.Fg, bg: bg}.render(strings.Repeat(" ", pad))
		}
		lines[i] = line
	}
	return lines
}

// editorHint is the bottom split. Its first row is a rule that carries the hint.
func editorHint(s style.Style, v View, rows []row, w int) []string {
	f := v.Frame
	rule := func(title string) string {
		left := plain(s, editorGutter).render("── ") + title + " "
		fill := max(0, w-ansi.StringWidth(left))
		return left + plain(s, editorGutter).render(strings.Repeat("─", fill))
	}
	if f.Hint != nil {
		return append([]string{rule(hintLine(s, f.Hint))}, keyboard(s, v, w-2)...)
	}
	lines := []string{rule(plain(s, s.Dim).render(text.RoundOverview))}
	for _, line := range overview(s, f, rows, w-3) {
		lines = append(lines, "   "+line)
	}
	return lines
}

func editorStatus(s style.Style, f play.Frame, rows []row, w int) string {
	item := func(fg, bg color.Color, value string) string {
		return look{fg: fg, bg: bg}.render(" " + value + " ")
	}
	mode := look{fg: editorModeText, bg: editorMode, bold: true}.render(" " + text.RoundTyping + " ")
	if f.Paused {
		mode = look{fg: s.Bg, bg: s.Warn, bold: true}.render(" " + text.RoundPaused + " ")
	}
	left := mode + item(s.Fg, editorBarHi, wpmText(f.WPM)) + item(s.Fg, editorBarHi, accuracyText(f))
	bar := look{fg: s.Dim, bg: editorBar}
	if f.HasGhost {
		left += bar.render(" "+text.RoundGhost+" ") +
			look{fg: leadColour(s, f.Lead), bg: editorBar}.render(leadText(f.Lead)+" ")
	}
	right := ""
	if f.HasSLA {
		right = look{fg: s.Bg, bg: slaColour(s, f), bold: true}.
			render(" " + text.RoundSLA + " " + slaText(f.SLA) + " ")
	}
	for _, optional := range []string{
		item(s.Fg, editorBarHi, ruler(f, rows)),
		item(s.Fg, editorBarHi, text.RoundStreak+" "+strconv.Itoa(streak(f))),
	} {
		if ansi.StringWidth(left)+ansi.StringWidth(right)+ansi.StringWidth(optional) <= w {
			right = optional + right
		}
	}
	return spread(left, right, w, bar)
}

// ruler is the line:column of the cursor, as Neovim shows it.
func ruler(f play.Frame, rows []row) string {
	n := rowOf(rows, f.Pos)
	first := n
	for first > 0 && rows[first].cont {
		first--
	}
	return fmt.Sprintf("%d:%d", rows[n].line, f.Pos-rows[first].from+1)
}

// editorMessage is the line under the status bar, where Neovim talks to you.
func editorMessage(s style.Style, f play.Frame, altHelp string) string {
	switch {
	case f.AltWarning:
		return " " + warning(s, altHelp)
	case f.Paused:
		return " " + plain(s, s.Warn).render("-- "+text.RoundPaused+" -- ") + pausedNote(s, f)
	}
	return ""
}
