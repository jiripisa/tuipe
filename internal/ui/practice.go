package ui

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/jiripisa/tuipe/internal/content"
	"github.com/jiripisa/tuipe/internal/engine"
	"github.com/jiripisa/tuipe/internal/generate"
	"github.com/jiripisa/tuipe/internal/ghost"
	"github.com/jiripisa/tuipe/internal/play"
	"github.com/jiripisa/tuipe/internal/score"
	"github.com/jiripisa/tuipe/internal/ui/style"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

const maxPracticeChars = 12

// practiceLangs are the languages that can be picked, as named in the snippets.
var practiceLangs = []string{"go", "python", "js", "ts", "shell", "sql", "regex"}

type practiceStage int

const (
	stageMenu practiceStage = iota
	stageLang
	stageChars
)

// practiceScreen lets the user pick what to train by hand: a chapter, a
// lesson, a language or a set of characters.
type practiceScreen struct {
	stage practiceStage
	sel   int
	chars []rune
	note  string
}

// NewPracticeScreen builds the Practice menu.
func NewPracticeScreen(a *App) Screen { return &practiceScreen{} }

func (p *practiceScreen) menuLabels() []string {
	return []string{text.PracticeChapter, text.PracticeLesson, text.PracticeLanguage, text.PracticeChars}
}

func (p *practiceScreen) Update(msg tea.Msg, a *App) (Screen, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return p, nil
	}
	p.note = ""
	switch p.stage {
	case stageChars:
		return p.updateChars(k, a)
	case stageLang:
		return p.updateList(k, a, len(practiceLangs))
	}
	return p.updateList(k, a, len(p.menuLabels()))
}

// updateList moves the selection of the menu or the language list.
func (p *practiceScreen) updateList(k tea.KeyPressMsg, a *App, n int) (Screen, tea.Cmd) {
	switch k.String() {
	case "up", "k":
		p.sel = (p.sel + n - 1) % n
	case "down", "j":
		p.sel = (p.sel + 1) % n
	case "esc":
		if p.stage == stageMenu {
			return NewHomeScreen(a), nil
		}
		p.stage, p.sel = stageMenu, 0
	case "enter":
		if p.stage == stageLang {
			return p.startLanguage(a, practiceLangs[p.sel])
		}
		return p.open(a)
	}
	return p, nil
}

// open acts on the chosen way in.
func (p *practiceScreen) open(a *App) (Screen, tea.Cmd) {
	back := func(*App) Screen { return p }
	switch p.sel {
	case 0:
		return newChapterList(listPracticeChapter, back, a), nil
	case 1:
		return newChapterList(listPracticeLesson, back, a), nil
	case 2:
		p.stage, p.sel = stageLang, 0
	default:
		p.stage = stageChars
	}
	return p, nil
}

// updateChars edits the field of characters. It reads KeyPressMsg.Text, so a
// dead-key result and an Option symbol arrive as the character they make.
func (p *practiceScreen) updateChars(k tea.KeyPressMsg, a *App) (Screen, tea.Cmd) {
	switch {
	case k.Code == tea.KeyEscape:
		p.stage, p.sel = stageMenu, 3
	case k.Code == tea.KeyBackspace:
		if n := len(p.chars); n > 0 {
			p.chars = p.chars[:n-1]
		}
	case k.Code == tea.KeyEnter:
		if len(p.chars) == 0 {
			p.note = text.PracticeCharsEmpty
			return p, nil
		}
		return p.startChars(a)
	case k.Mod.Contains(tea.ModCtrl) || k.Mod.Contains(tea.ModMeta):
	default:
		in := k.Text
		if in == "" && k.Mod.Contains(tea.ModAlt) && unicode.IsPrint(k.Code) {
			in = string(k.Code) // the terminal decoder drops Text for Alt chords
		}
		for _, r := range in {
			p.add(a, r)
		}
	}
	return p, nil
}

// add appends r unless it is a repeat, cannot be typed on the layout, or the
// field is full.
func (p *practiceScreen) add(a *App, r rune) {
	if len(p.chars) >= maxPracticeChars || !unicode.IsPrint(r) || r == ' ' || !a.Curr.CanType(r) {
		return
	}
	for _, c := range p.chars {
		if c == r {
			return
		}
	}
	p.chars = append(p.chars, r)
}

func (a *App) seed() int64 { return a.Now().UnixNano() }

// startChars drills the chosen characters with everything the layout can type.
func (p *practiceScreen) startChars(a *App) (Screen, tea.Cmd) { return p.charsRound(a), nil }

func (p *practiceScreen) charsRound(a *App) Screen {
	spec := a.generatedSpec(fmt.Sprintf(text.PracticeDrill, string(p.chars)), p.chars, a.Curr.CanType, a.chapterTargets())
	spec.Practice = true
	return newRound(a, spec, func(a *App) Screen { return p.charsRound(a) })
}

// generatedSpec is a drill on focus, with a pace ghost from the user's timing.
func (a *App) generatedSpec(title string, focus []rune, allowed func(rune) bool, targets score.Targets) play.Spec {
	txt := generate.Drill(a.Lib, generate.Input{Focus: focus, Allowed: allowed, Seed: a.seed()})
	return play.Spec{
		Kind: play.KindDrill, Title: title, Text: txt, Targets: targets,
		Ghost:       ghost.Pace(engine.Prepare(txt), a.stats(), paceFallbackMs),
		GhostIsPace: true,
	}
}

// chapterDrillSpec drills the characters a chapter introduces, with what it has unlocked.
func (a *App) chapterDrillSpec(ch content.ChapterDef) (play.Spec, error) {
	focus := []rune(strings.TrimSpace(ch.Introduces))
	if len(focus) == 0 {
		focus = []rune(strings.TrimSpace(ch.Allowed))
	}
	unlocked := a.Curr.AllowedAt(ch.ID)
	targets := a.chapterTargets()
	if n := len(ch.Lessons); n > 0 {
		targets = a.Curr.Targets(ch.Lessons[n-1].ID)
	}
	spec := a.generatedSpec(fmt.Sprintf(text.PracticeDrill, ch.Title), focus,
		func(r rune) bool { return unlocked(r) && a.Curr.CanType(r) }, targets)
	if spec.Text == "" {
		return play.Spec{}, errors.New(text.TestOutNotReady)
	}
	return spec, nil
}

// startLanguage plays a random snippet of lang that the layout can type.
func (p *practiceScreen) startLanguage(a *App, lang string) (Screen, tea.Cmd) {
	var ids []string
	for _, s := range a.Lib.Snippets {
		if s.Lang == lang && len(a.Layout.Unresolved(s.Text)) == 0 {
			ids = append(ids, s.ID)
		}
	}
	if len(ids) == 0 {
		p.note = text.PracticeNoSnippet
		return p, nil
	}
	var round func(*App) Screen
	round = func(a *App) Screen {
		r := rand.New(rand.NewPCG(uint64(a.seed()), 0x9e3779b97f4a7c15))
		spec, err := a.codeSpec(ids[r.IntN(len(ids))])
		if err != nil {
			a.SaveErr(err)
			return p
		}
		spec.Practice = true
		return newRound(a, spec, round)
	}
	return round(a), nil
}

func (p *practiceScreen) View(a *App, w, h int) string {
	st := a.Style
	paint := func(s string, bold bool, sel bool) string {
		col := st.Fg
		if sel {
			col = st.Accent
		}
		return st.Base().Foreground(col).Bold(bold).Render(s)
	}
	var lines []string
	hint := text.HintPractice
	switch p.stage {
	case stageChars:
		hint = text.HintPracticeChars
		lines = []string{
			paint(fmt.Sprintf(text.PracticeCharsAsk, maxPracticeChars), false, false), "",
			paint("> "+string(p.chars)+"▏", true, true),
		}
	default:
		labels := p.menuLabels()
		if p.stage == stageLang {
			labels = practiceLangs
		}
		for i, l := range labels {
			if i == p.sel {
				lines = append(lines, paint("> "+l, true, true))
			} else {
				lines = append(lines, paint("  "+l, false, false))
			}
		}
	}
	if p.note != "" {
		lines = append(lines, "", st.Base().Foreground(st.Dim).Render(p.note))
	}
	return style.Frame(st, w, h, text.AppName+" · "+text.PracticeTitle, strings.Join(lines, "\n"), hint)
}
