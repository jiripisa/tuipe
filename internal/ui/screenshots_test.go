package ui_test

import (
	"fmt"
	"html"
	"math/rand/v2"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jiripisa/tuipe/internal/content"
	"github.com/jiripisa/tuipe/internal/incident"
	"github.com/jiripisa/tuipe/internal/recommend"
	"github.com/jiripisa/tuipe/internal/stats"
	"github.com/jiripisa/tuipe/internal/store"
	"github.com/jiripisa/tuipe/internal/ui"
	"github.com/jiripisa/tuipe/internal/ui/style"
)

// TestScreenshots writes the pictures of the website and the manual as HTML
// files, one per screen, into the folder named by TUIPE_SHOTS. It is not a test
// of anything: scripts/render-screenshots.sh runs it and turns the files into
// PNGs. Without the variable it is skipped.
//
// Nothing here is drawn by hand. A simulated typist plays about four weeks of
// real rounds through the real app on a fake clock, so the ghost, the SLA, the
// heatmap and the history on the pictures are what the app computes.
func TestScreenshots(t *testing.T) {
	dir := os.Getenv("TUIPE_SHOTS")
	if dir == "" {
		t.Skip("set TUIPE_SHOTS to a folder to render the screenshots")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	defer ui.SetGOOS("darwin")()

	s := newSession(t, dir)
	s.placedThroughChapter(7)
	s.playWeeks()
	s.shootRound()
	s.shootHint()
	s.shootIncident()
	s.shootShared()
	newPlacementShots(t, dir)
}

const (
	shotW, shotH = 80, 24
	shotLayout   = "macos-czech-qwerty"
	firstDay     = 8  // October 2026
	skippedDay   = 29 // a Thursday: the error budget of the last week
	lastDay      = 32 // 1 November
	startWPM     = 34.0
	wpmPerDay    = 0.9
	roundsADay   = 3
	endingWait   = time.Second // longer than the lock on an incident's ending
)

// session is the app of the simulated user.
type session struct {
	t   *testing.T
	dir string
	a   *ui.App
	st  *store.Store
	clk *clock
	rng *rand.Rand
	wpm float64 // the typist's speed today
	bad float64 // multiplies the chance of a wrong key
}

func newSession(t *testing.T, dir string) *session {
	t.Helper()
	tmp := t.TempDir()
	st, err := store.Open(filepath.Join(tmp, "cfg"), filepath.Join(tmp, "data"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	lib, err := content.Load()
	if err != nil {
		t.Fatal(err)
	}
	st.Profile().FirstRunDone = true
	clk := &clock{t: dayAt(firstDay)}
	a, err := ui.New(st, lib, ui.WithClock(clk.now))
	if err != nil {
		t.Fatal(err)
	}
	a.Update(tea.WindowSizeMsg{Width: shotW, Height: shotH})
	return &session{t: t, dir: dir, a: a, st: st, clk: clk, rng: rand.New(rand.NewPCG(7, 11)), wpm: startWPM, bad: 1}
}

// dayAt is 9:00 on a day of October 2026; 32 is 1 November.
func dayAt(day int) time.Time {
	return time.Date(2026, 10, day, 9, 0, 0, 0, time.Local)
}

// placedThroughChapter skips the chapters up to last, as a placement test does.
func (s *session) placedThroughChapter(last int) {
	var ids []string
	for _, ch := range s.a.Curr.Chapters() {
		if ch.Number <= last {
			ids = append(ids, ch.ID)
		}
	}
	p := s.st.Profile()
	p.Stats[shotLayout] = stats.New()
	s.a.Curr.ApplySkip(p, p.Stats[shotLayout], ids)
	p.Stats[shotLayout].SetMastery('\n', stats.Solid)
}

// playWeeks plays every day but one: quick runs, some Standups and a Sprint.
func (s *session) playWeeks() {
	for day := firstDay; day < lastDay; day++ {
		if day == skippedDay {
			continue
		}
		s.clk.t = dayAt(day)
		s.wpm = startWPM + wpmPerDay*float64(day-firstDay) + s.rng.Float64()*3
		for range roundsADay {
			s.quickRun()
		}
		switch {
		case day == lastDay-1:
			s.chain("sprint", 10)
		case day%3 == 0:
			s.chain("standup", 0)
		}
	}
	s.loseIncident()
	s.start(recommend.Pick{Kind: recommend.Code, SnippetID: "go-1"}) // the ghost of shootRound
	s.playRound()
	s.clk.advance(endingWait)
	s.a.Update(esc)
	s.clk.t = dayAt(lastDay)
	s.wpm += 2
}

// quickRun takes the recommended round from the home screen, as a user does.
func (s *session) quickRun() {
	ui.SetScreen(s.a, ui.NewHomeScreen(s.a))
	s.a.Update(enter)
	if ui.ScreenName(s.a) == "severity" {
		s.a.Update(enter)
	}
	s.playRound()
	s.clk.advance(endingWait)
	s.a.Update(esc)
}

// chain plays a whole Standup or Sprint and leaves it on its last screen.
func (s *session) chain(kind string, minutes int) {
	ui.SetScreen(s.a, ui.NewChain(s.a, kind, nil, minutes))
	for ui.ScreenName(s.a) == "round" {
		s.playRound()
		s.clk.advance(endingWait)
		s.a.Update(enter)
	}
	if ui.ScreenName(s.a) == "retro" {
		s.shoot("retro", style.Editor)
		s.a.Update(enter)
	}
}

// playRound types the rest of the round, or until an incident is lost.
func (s *session) playRound() {
	s.a.Update(ui.TickMsg(s.a))
	for _, r := range ui.RoundKeys(s.a) {
		if ui.ScreenName(s.a) != "round" {
			return
		}
		s.press(r)
	}
}

// press types one character the way a person does: symbols behind Option are
// slower and go wrong more often. A wrong key is followed by the right one.
func (s *session) press(r rune) {
	slow, miss := 1.0, 0.012
	if route, ok := s.a.Layout.Route(r); ok {
		for _, chord := range route {
			for _, mod := range chord.Mods {
				if mod == "option" {
					slow, miss = 1.9, 0.07
				} else if slow < 1.25 {
					slow, miss = 1.25, 0.025
				}
			}
		}
		if len(route) > 1 {
			slow, miss = slow*1.5, miss*1.5
		}
	}
	gap := time.Duration(float64(12*time.Second) / s.wpm * slow * (0.7 + 0.6*s.rng.Float64()))
	if s.rng.Float64() < miss*s.bad {
		s.clk.advance(gap)
		s.a.Update(wrongFor(r))
		if ui.ScreenName(s.a) != "round" {
			return
		}
	}
	s.clk.advance(gap)
	s.a.Update(key(r))
}

// typeUpTo types until the cursor stands on the first occurrence of mark in
// the keys of the round.
func (s *session) typeUpTo(mark string) {
	keys := ui.RoundKeys(s.a)
	at := strings.Index(string(keys), mark)
	if at < 0 {
		s.t.Fatalf("no %q in the round", mark)
	}
	for _, r := range []rune(string(keys)[:at]) {
		s.press(r)
	}
}

func (s *session) start(pick recommend.Pick) {
	ui.SetScreen(s.a, ui.NewRoundScreen(s.a, pick, nil))
	s.a.Update(ui.TickMsg(s.a))
}

// shootRound is a Go lesson in the middle of a round against its replay ghost.
func (s *session) shootRound() {
	s.start(recommend.Pick{Kind: recommend.Lesson, LessonID: "ch11-01"})
	s.typeUpTo("return 0, fmt.Errorf")
	s.a.Update(ui.TickMsg(s.a))
	for _, st := range []style.ID{style.Editor, style.Fosfor, style.Velin} {
		s.shoot("round-"+string(st), st)
	}
	for ui.ScreenName(s.a) == "round" {
		keys := ui.RoundKeys(s.a)
		s.press(keys[len(keys)-roundKeysLeft(s.a)])
	}
	s.shoot("results", style.Editor)
	s.clk.advance(endingWait)
	s.a.Update(esc)
}

// roundKeysLeft counts the keys still to type.
func roundKeysLeft(a *ui.App) int {
	f := ui.RoundFrame(a)
	left := 0
	for i := f.Pos; i < len(f.Cells); i++ {
		if !f.Cells[i].Auto {
			left++
		}
	}
	return left
}

// shootHint waits on a { until the hint and the keyboard map appear.
func (s *session) shootHint() {
	s.start(recommend.Pick{Kind: recommend.Code, SnippetID: "go-5"})
	s.typeUpTo("{\nid := r.URL")
	for range 25 { // 2.5 s of hesitation, in the steps of the real tick
		s.clk.advance(100 * time.Millisecond)
		s.a.Update(ui.TickMsg(s.a))
	}
	if ui.RoundHint(s.a) == nil {
		s.t.Fatal("no hint after 2.5 s")
	}
	for _, st := range []style.ID{style.Editor, style.Fosfor, style.Velin} {
		s.shoot("hint-"+string(st), st)
	}
	s.a.Update(esc)
	s.a.Update(down)
	s.a.Update(down)
	s.a.Update(enter) // the pause menu: leave
}

// shootIncident is a boss incident in its third step, with the SLA burning.
func (s *session) shootIncident() {
	inc := s.incident("INC-1011")
	ui.SetScreen(s.a, ui.NewIncidentScreen(s.a, inc, incident.SEV2, nil))
	s.a.Update(ui.TickMsg(s.a))
	s.bad = 2.5
	s.typeUpTo("http.Error(w,")
	s.bad = 1
	s.a.Update(ui.TickMsg(s.a))
	for _, st := range []style.ID{style.Editor, style.Fosfor, style.Velin} {
		s.shoot("incident-"+string(st), st)
	}
	s.a.Update(esc)
	s.a.Update(down)
	s.a.Update(down)
	s.a.Update(enter)
}

// loseIncident lets the SLA run out, for the postmortem and a lost run in the
// history.
func (s *session) loseIncident() {
	ui.SetScreen(s.a, ui.NewIncidentScreen(s.a, s.incident("INC-1008"), incident.SEV1, nil))
	s.wpm, s.bad = s.wpm/2.5, 4
	s.playRound()
	s.wpm, s.bad = s.wpm*2.5, 1
	s.clk.advance(endingWait)
	if ui.ScreenName(s.a) != "incident-results" {
		s.t.Fatalf("after the lost incident: %s", ui.ScreenName(s.a))
	}
	s.shoot("postmortem", style.Editor)
	s.a.Update(esc)
}

func (s *session) incident(id string) content.Incident {
	for _, inc := range s.a.Lib.Incidents {
		if inc.ID == id {
			return inc
		}
	}
	s.t.Fatalf("no incident %s", id)
	return content.Incident{}
}

// shootShared draws the screens every style shares.
func (s *session) shootShared() {
	ui.SetScreen(s.a, ui.NewHomeScreen(s.a))
	s.shoot("home", style.Editor)

	ui.SetScreen(s.a, ui.NewCareerScreen(s.a))
	s.shoot("career", style.Editor)
	s.a.Update(esc)

	ui.SetScreen(s.a, ui.NewStatsScreen(s.a))
	right := tea.KeyPressMsg{Code: tea.KeyRight}
	for _, name := range []string{"gitlog", "heatmap", "trends", "records"} {
		s.shoot(name, style.Editor)
		s.a.Update(right)
	}
	ui.SetScreen(s.a, ui.NewHomeScreen(s.a))
}

// newPlacementShots plays a first run in a new profile up to the job offer.
func newPlacementShots(t *testing.T, dir string) {
	s := newSession(t, dir)
	s.st.Profile().FirstRunDone = false
	s.clk.t = dayAt(lastDay)
	s.wpm = 42
	ui.SetScreen(s.a, ui.NewFirstRunScreen(s.a))
	s.shoot("firstrun-style", style.Editor)
	s.a.Update(enter)
	for _, r := range "ě{@" {
		s.a.Update(key(r))
	}
	s.a.Update(down)
	s.a.Update(enter) // the placement test
	for ui.ScreenName(s.a) == "placement" && strings.Contains(screen(s.a), "Enter začít") {
		if strings.Contains(screen(s.a), "Fáze 5") {
			s.bad = 40 // Option is where this user stops
		}
		s.a.Update(enter)
		s.playRound()
	}
	s.shoot("offer", style.Editor)
}

// shoot writes the current screen in the given style as an HTML page.
func (s *session) shoot(name string, id style.ID) {
	s.t.Helper()
	s.a.Style = style.ByID(id)
	body := renderCells(s.a.View().Content, s.a.Style)
	path := filepath.Join(s.dir, name+".html")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		s.t.Fatal(err)
	}
}

// sgr matches a colour or weight sequence, the only escapes a screen contains.
var sgr = regexp.MustCompile(`\x1b\[[0-9;:]*m`)

// cellStyle is the appearance of the text that follows an SGR sequence.
type cellStyle struct {
	fg, bg          string
	bold, underline bool
}

// apply folds the parameters of one SGR sequence into c.
func (c cellStyle) apply(seq string) cellStyle {
	params := strings.Split(strings.TrimSuffix(strings.TrimPrefix(seq, "\x1b["), "m"), ";")
	for i := 0; i < len(params); i++ {
		switch params[i] {
		case "", "0":
			c = cellStyle{}
		case "1":
			c.bold = true
		case "4", "4:1":
			c.underline = true
		case "22":
			c.bold = false
		case "24":
			c.underline = false
		case "39":
			c.fg = ""
		case "49":
			c.bg = ""
		case "38", "48":
			if i+4 >= len(params) || params[i+1] != "2" {
				panic("unexpected colour in " + strconv.Quote(seq))
			}
			hex := "#"
			for _, p := range params[i+2 : i+5] {
				v, _ := strconv.Atoi(p)
				hex += fmt.Sprintf("%02x", v)
			}
			if params[i] == "38" {
				c.fg = hex
			} else {
				c.bg = hex
			}
			i += 4
		default:
			panic("unexpected SGR parameter in " + strconv.Quote(seq))
		}
	}
	return c
}

func (c cellStyle) css() string {
	var b strings.Builder
	if c.fg != "" {
		b.WriteString("color:" + c.fg + ";")
	}
	if c.bg != "" {
		b.WriteString("background:" + c.bg + ";")
	}
	if c.bold {
		b.WriteString("font-weight:700;")
	}
	if c.underline {
		b.WriteString("text-decoration:underline;")
	}
	return b.String()
}

// renderCells turns a screen into a page where each character sits in a cell
// of its own, so glyphs from fallback fonts cannot push a row out of the grid.
func renderCells(screen string, st style.Style) string {
	var b strings.Builder
	fg, bg := ansiHex(st.Fg), ansiHex(st.Bg)
	fmt.Fprintf(&b, `<!doctype html><meta charset="utf-8"><style>
html,body{margin:0;background:%s;color:%s}
#s{padding:10px 12px;font:15px/18px Menlo,monospace;-webkit-font-smoothing:antialiased}
#s div{height:18px;white-space:pre;display:flex}
#s i{display:block;width:9px;height:18px;font-style:normal;text-align:center;flex:none}
</style><div id="s">`, bg, fg)
	for _, line := range strings.Split(screen, "\n") {
		b.WriteString("<div>")
		var cur cellStyle
		rest := line
		for rest != "" {
			if loc := sgr.FindStringIndex(rest); loc != nil && loc[0] == 0 {
				cur = cur.apply(rest[:loc[1]])
				rest = rest[loc[1]:]
				continue
			}
			end := len(rest)
			if loc := sgr.FindStringIndex(rest); loc != nil {
				end = loc[0]
			}
			for _, r := range rest[:end] {
				if ansi.StringWidth(string(r)) == 0 {
					continue
				}
				fmt.Fprintf(&b, `<i style="%s">%s</i>`, cur.css(), html.EscapeString(string(r)))
			}
			rest = rest[end:]
		}
		b.WriteString("</div>")
	}
	b.WriteString("</div>")
	return b.String()
}

func ansiHex(c interface{ RGBA() (r, g, b, a uint32) }) string {
	r, g, bl, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, bl>>8)
}
