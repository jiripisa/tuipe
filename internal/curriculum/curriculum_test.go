package curriculum_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/jiripisa/tuipe/internal/content"
	"github.com/jiripisa/tuipe/internal/curriculum"
	"github.com/jiripisa/tuipe/internal/layout"
	"github.com/jiripisa/tuipe/internal/score"
	"github.com/jiripisa/tuipe/internal/stats"
	"github.com/jiripisa/tuipe/internal/store"
)

func loadLib(t *testing.T) *content.Library {
	t.Helper()
	lib, err := content.Load()
	if err != nil {
		t.Fatal(err)
	}
	return lib
}

func czech(t *testing.T) *layout.Layout {
	t.Helper()
	all, err := layout.Builtin()
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range all {
		if l.ID == "macos-czech-qwerty" {
			return l
		}
	}
	t.Fatal("no Czech layout")
	return nil
}

func curr(t *testing.T) *curriculum.Curriculum {
	t.Helper()
	return curriculum.New(loadLib(t), czech(t))
}

func newProfile() *store.Profile {
	return &store.Profile{
		Stats:        map[string]*stats.Stats{},
		Stars:        map[string]int{},
		Completed:    map[string]bool{},
		IncidentBest: map[string]string{},
	}
}

// stubLayout types the letters a-z and space, and nothing else: no braces.
func stubLayout(t *testing.T) *layout.Layout {
	t.Helper()
	var sb strings.Builder
	sb.WriteString("id = \"stub\"\nname = \"Stub\"\nos = \"linux\"\nmodifier_name = \"AltGr\"\n[[row]]\nkeys = [\n")
	for _, r := range "abcdefghijklmnopqrstuvwxyz " {
		fmt.Fprintf(&sb, "  { id = \"k%d\", label = \"%c\", hand = \"left\", finger = \"index\" },\n", r, r)
	}
	sb.WriteString("]\n")
	for _, r := range "abcdefghijklmnopqrstuvwxyz " {
		fmt.Fprintf(&sb, "[[char]]\nc = \"%c\"\nroute = [\"k%d\"]\n", r, r)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "stub.toml"), []byte(sb.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	ls, errs := layout.LoadDir(dir)
	if len(ls) != 1 || len(errs) != 0 {
		t.Fatalf("stub layout: %v %v", ls, errs)
	}
	return ls[0]
}

func TestChaptersMatchSpecTable(t *testing.T) {
	c := curr(t)
	want := []string{
		"Intern", "Intern", "Intern", "Intern", "Junior", "Junior", "Medior", "Medior",
		"Senior", "Senior", "Senior", "Staff", "Principal",
	}
	chs := loadLib(t).Chapters
	if len(chs) != len(want) {
		t.Fatalf("%d chapters, want %d", len(chs), len(want))
	}
	for i, ch := range chs {
		if ch.Number != i || ch.Rank != want[i] || ch.ID != fmt.Sprintf("ch%02d", i) {
			t.Errorf("chapter %d: got id %q number %d rank %q, want rank %q", i, ch.ID, ch.Number, ch.Rank, want[i])
		}
	}
	if got := len(c.Chapters()); got != len(want) {
		t.Errorf("Chapters() has %d chapters, want %d", got, len(want))
	}
}

func TestOwnerListIsCoveredByLessonsOrIntroduces(t *testing.T) {
	const owner = `><+@#$~^&*{}°=%'ˇ/[()]";'§!` + "`" + `¨-_.:,?\|` +
		"abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789" +
		"áčďéěíňóřšťúůýžÁČĎÉĚÍŇÓŘŠŤÚŮÝŽ \n"
	taught := map[rune]bool{}
	for _, ch := range loadLib(t).Chapters {
		for _, r := range ch.Introduces {
			taught[r] = true
		}
	}
	for _, r := range owner {
		if !taught[r] {
			t.Errorf("%q is not introduced by any chapter", r)
		}
	}
}

func TestEachCharTaughtOnce(t *testing.T) {
	where := map[rune]string{}
	for _, ch := range loadLib(t).Chapters {
		if ch.Number == 0 && ch.Introduces != "" {
			t.Errorf("chapter 0 must introduce nothing, has %q", ch.Introduces)
		}
		for _, r := range ch.Introduces {
			if prev, dup := where[r]; dup {
				t.Errorf("%q is introduced by both %s and %s", r, prev, ch.ID)
			}
			where[r] = ch.ID
		}
	}
}

func TestAllowedAtIsCumulative(t *testing.T) {
	c := curr(t)
	cases := []struct {
		chapter string
		r       rune
		want    bool
	}{
		{"ch00", 'f', true}, {"ch00", ' ', true}, {"ch00", 'a', false},
		{"ch01", 'a', true}, {"ch01", 'q', false}, {"ch01", 'ů', true},
		{"ch02", 'q', true}, {"ch02", 'a', true}, {"ch02", 'z', false},
		{"ch04", 'Q', true}, {"ch04", '!', true}, {"ch04", 'ě', false},
		{"ch05", 'Ď', true}, {"ch05", '1', false},
		{"ch10", '§', true}, {"ch10", '\n', false},
		{"ch11", '\n', true}, {"ch12", '\n', true},
		{"nope", 'a', false},
	}
	for _, tc := range cases {
		if got := c.AllowedAt(tc.chapter)(tc.r); got != tc.want {
			t.Errorf("AllowedAt(%s)(%q) = %v, want %v", tc.chapter, tc.r, got, tc.want)
		}
	}
}

func TestLessonUsesOnlyUnlockedChars(t *testing.T) {
	c := curr(t)
	for _, ch := range c.Chapters() {
		allowed := c.AllowedAt(ch.ID)
		for _, l := range ch.Lessons {
			text, err := c.LessonText(l.ID)
			if err != nil {
				t.Errorf("%s: %v", l.ID, err)
				continue
			}
			for _, r := range text {
				if !allowed(r) {
					t.Errorf("%s: %q is not unlocked in %s", l.ID, r, ch.ID)
					break
				}
			}
		}
	}
}

func TestEarlyLessonsOnlyUseCharsTaughtSoFar(t *testing.T) {
	c := curr(t)
	chs := c.Chapters()
	text, err := c.LessonText(chs[1].Lessons[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range text {
		if !strings.ContainsRune("fjdk ", r) {
			t.Fatalf("first home-row lesson uses %q: %q", r, text)
		}
	}
}

func TestLessonTextIsStable(t *testing.T) {
	c := curr(t)
	for _, ch := range c.Chapters() {
		for _, l := range ch.Lessons {
			a, err1 := c.LessonText(l.ID)
			b, err2 := c.LessonText(l.ID)
			if err1 != nil || err2 != nil || a != b || a == "" {
				t.Errorf("%s: unstable or empty text (%v, %v)", l.ID, err1, err2)
			}
		}
	}
	if _, err := c.LessonText("no-such-lesson"); err == nil {
		t.Error("unknown lesson should be an error")
	}
}

func TestLessonDurationIs60To90SecondsAtTarget(t *testing.T) {
	c := curr(t)
	for _, ch := range c.Chapters() {
		for _, l := range ch.Lessons {
			if len(l.Snippets) > 0 {
				continue
			}
			text, _ := c.LessonText(l.ID)
			secs := float64(utf8.RuneCountInString(text)) / (l.Speed[1] * 5 / 60)
			if secs < 60 || secs > 90 {
				t.Errorf("%s: %.0f s at target %v WPM (%d chars)", l.ID, secs, l.Speed[1], utf8.RuneCountInString(text))
			}
		}
	}
}

func TestSnippetLessonsResolve(t *testing.T) {
	c := curr(t)
	var found int
	for _, ch := range c.Chapters() {
		if ch.Number == 11 {
			found = len(ch.Lessons)
		}
	}
	if found < 6 || found > 9 {
		t.Errorf("chapter 11 has %d resolvable lessons, want 6 to 9", found)
	}
}

func TestTargets(t *testing.T) {
	c := curr(t)
	l := c.Chapters()[1].Lessons[0]
	if got := c.Targets(l.ID); got != (score.Targets{Speed: l.Speed}) {
		t.Errorf("Targets = %v, want %v", got, l.Speed)
	}
	if got := c.Targets("nope"); got != (score.Targets{}) {
		t.Errorf("unknown lesson targets = %v", got)
	}
}

func TestSkipCompletesWithoutStars(t *testing.T) {
	c := curr(t)
	p := newProfile()
	s := stats.New()
	s.Chars["a"] = &stats.Entry{Samples: 3, ErrorRate: 0.3} // weak evidence
	s.Chars["s"] = &stats.Entry{Samples: 4, ErrorRate: 0.05}
	s.Chars["d"] = &stats.Entry{Samples: 40, ErrorRate: 0.01, Mastery: stats.Automatic}
	s.Chars["q"] = &stats.Entry{Samples: 2, ErrorRate: 0.9} // too few samples to judge

	c.ApplySkip(p, s, []string{"ch01", "ch02", "ch03"})

	for _, id := range []string{"ch01", "ch02", "ch03"} {
		if !p.Completed[id] {
			t.Errorf("%s not completed", id)
		}
	}
	if len(p.Stars) != 0 {
		t.Errorf("skip granted stars: %v", p.Stars)
	}
	want := map[rune]stats.Mastery{
		'a': stats.Learning, 's': stats.Solid, 'd': stats.Automatic, 'q': stats.Solid,
		'z': stats.Solid, 'ů': stats.Solid, ' ': stats.Solid,
		'A': stats.NewChar, '1': stats.NewChar, // not in a skipped chapter
	}
	for r, m := range want {
		if got := s.MasteryOf(r); got != m {
			t.Errorf("mastery of %q = %v, want %v", r, got, m)
		}
	}
	if p.Completed["ch04"] {
		t.Error("ch04 must stay open")
	}
}

func TestSkipIgnoresUnknownChapterAndNilStats(t *testing.T) {
	c := curr(t)
	p := newProfile()
	c.ApplySkip(p, nil, []string{"nope", "ch01"})
	if p.Completed["nope"] || !p.Completed["ch01"] {
		t.Errorf("completed = %v", p.Completed)
	}
}

func TestRankString(t *testing.T) {
	want := []string{"Intern", "Junior", "Medior", "Senior", "Staff", "Principal"}
	for i, w := range want {
		if got := curriculum.Rank(i).String(); got != w {
			t.Errorf("Rank(%d) = %q, want %q", i, got, w)
		}
	}
}

func TestRankOfHighestCompleted(t *testing.T) {
	c := curr(t)
	p := newProfile()
	if got := c.RankOf(p); got != curriculum.Intern {
		t.Errorf("fresh profile rank = %v", got)
	}
	c.CompleteChapter(p, "ch04")
	if got := c.RankOf(p); got != curriculum.Junior {
		t.Errorf("after ch04 rank = %v, want Junior", got)
	}
	c.CompleteChapter(p, "ch08")
	if got := c.RankOf(p); got != curriculum.Senior {
		t.Errorf("after ch08 rank = %v, want Senior", got)
	}
	c.CompleteChapter(p, "ch12")
	if got := c.RankOf(p); got != curriculum.Principal {
		t.Errorf("after ch12 rank = %v, want Principal", got)
	}
}

func TestRankNeverFalls(t *testing.T) {
	c := curr(t)
	p := newProfile()
	s := stats.New()
	p.Stats["macos-czech-qwerty"] = s
	c.ApplySkip(p, s, []string{"ch01", "ch02", "ch03", "ch04"})
	before := c.RankOf(p)
	for _, r := range "asdfjkl" {
		s.SetMastery(r, stats.NewChar) // mastery decay
	}
	if after := c.RankOf(p); after != before || after != curriculum.Junior {
		t.Errorf("rank changed from %v to %v", before, after)
	}
}

func TestCompleteChapterGivesNoStars(t *testing.T) {
	c := curr(t)
	p := newProfile()
	c.CompleteChapter(p, "ch03")
	c.CompleteChapter(p, "nope")
	if !p.Completed["ch03"] || p.Completed["nope"] || len(p.Stars) != 0 {
		t.Errorf("completed %v stars %v", p.Completed, p.Stars)
	}
}

func TestNextLessonFollowsStarsAndChapters(t *testing.T) {
	c := curr(t)
	p := newProfile()
	chs := c.Chapters()

	l, ok := c.NextLesson(p)
	if !ok || l.ID != chs[0].Lessons[0].ID {
		t.Fatalf("fresh profile next = %q %v, want first onboarding lesson", l.ID, ok)
	}
	if got := c.CurrentChapter(p); got.ID != "ch00" {
		t.Errorf("current chapter = %s", got.ID)
	}

	p.Stars[chs[0].Lessons[0].ID] = 1 // chapter 0 has no incident: done
	l, _ = c.NextLesson(p)
	if l.ID != chs[1].Lessons[0].ID {
		t.Errorf("after onboarding next = %q, want %q", l.ID, chs[1].Lessons[0].ID)
	}
	p.Stars[chs[1].Lessons[0].ID] = 2
	l, _ = c.NextLesson(p)
	if l.ID != chs[1].Lessons[1].ID {
		t.Errorf("next = %q, want second lesson of ch01", l.ID)
	}
}

func TestSkippedChaptersAreNotOffered(t *testing.T) {
	c := curr(t)
	p := newProfile()
	c.ApplySkip(p, stats.New(), []string{"ch00", "ch01", "ch02"})
	l, ok := c.NextLesson(p)
	if !ok || l.ID != c.Chapters()[3].Lessons[0].ID {
		t.Errorf("next = %q %v", l.ID, ok)
	}
	if got := c.CurrentChapter(p); got.ID != "ch03" {
		t.Errorf("current = %s", got.ID)
	}
}

func TestIsUnlocked(t *testing.T) {
	c := curr(t)
	p := newProfile()
	for id, want := range map[string]bool{"ch00": true, "ch01": true, "ch02": false, "ch12": false, "nope": false} {
		if got := c.IsUnlocked(p, id); got != want {
			t.Errorf("fresh: IsUnlocked(%s) = %v, want %v", id, got, want)
		}
	}
	c.ApplySkip(p, stats.New(), []string{"ch01", "ch02"})
	for id, want := range map[string]bool{"ch03": true, "ch04": false} {
		if got := c.IsUnlocked(p, id); got != want {
			t.Errorf("after skip: IsUnlocked(%s) = %v, want %v", id, got, want)
		}
	}
}

func TestTestOutText(t *testing.T) {
	c := curr(t)
	text, targets := c.TestOutText("ch03")
	again, _ := c.TestOutText("ch03")
	if text != again {
		t.Error("test-out text is not fixed by its seed")
	}
	if n := utf8.RuneCountInString(text); n < 120 || n > 150 {
		t.Errorf("test-out has %d characters", n)
	}
	allowed := c.AllowedAt("ch03")
	for _, r := range text {
		if !allowed(r) {
			t.Fatalf("test-out uses locked character %q", r)
		}
	}
	ch := c.Chapters()[3]
	if want := ch.Lessons[len(ch.Lessons)-1].Speed; targets.Speed != want {
		t.Errorf("targets %v, want last lesson's %v", targets.Speed, want)
	}
	if other, _ := c.TestOutText("ch04"); other == text {
		t.Error("different chapters share a test-out")
	}
	if none, tg := c.TestOutText("nope"); none != "" || tg != (score.Targets{}) {
		t.Errorf("unknown chapter gave %q %v", none, tg)
	}
}

func TestIncidentDefIsFalseWhenIncidentDoesNotExist(t *testing.T) {
	lib := loadLib(t)
	lib.Chapters[1].IncidentID = "INC-9999" // no such incident
	c := curriculum.New(lib, czech(t))
	if _, ok := c.IncidentDef("ch01"); ok {
		t.Error("an incident that does not exist was resolved")
	}
	if _, ok := c.IncidentDef("ch00"); ok {
		t.Error("chapter 0 has no incident")
	}
	if _, ok := c.IncidentDef("nope"); ok {
		t.Error("unknown chapter has no incident")
	}
}

// incidentLib is a two-chapter library whose first chapter has an incident
// that needs a brace.
func incidentLib(t *testing.T) *content.Library {
	t.Helper()
	lib := loadLib(t)
	lesson := func(id string) content.LessonDef {
		return content.LessonDef{ID: id, Title: id, Focus: "ab", Seed: 7, Length: 60, Speed: [3]float64{10, 15, 20}}
	}
	lib.Chapters = []content.ChapterDef{
		{
			ID: "c1", Number: 1, Rank: "Intern", Introduces: "abcdefghijklmnopqrstuvwxyz ", IncidentID: "INC-1001",
			Lessons: []content.LessonDef{lesson("c1-1"), lesson("c1-2")},
		},
		{ID: "c2", Number: 2, Rank: "Junior", Lessons: []content.LessonDef{lesson("c2-1")}},
	}
	lib.Incidents = []content.Incident{{
		ID: "INC-1001", Title: "brace", Chapter: "c1",
		Steps: []content.Step{{Prompt: "fix", Text: "ab {cd}", Lang: "go"}},
	}}
	return lib
}

func TestChapterWithUntypableIncidentCompletesThroughLessons(t *testing.T) {
	c := curriculum.New(incidentLib(t), stubLayout(t))
	if _, ok := c.IncidentDef("c1"); ok {
		t.Fatal("incident needing '{' must be dropped on a layout without it")
	}
	p := newProfile()
	if l, _ := c.NextLesson(p); l.ID != "c1-1" {
		t.Fatalf("next = %q", l.ID)
	}
	p.Stars["c1-1"] = 3
	if l, _ := c.NextLesson(p); l.ID != "c1-2" {
		t.Fatalf("next = %q, want the last lesson", l.ID)
	}
	p.Stars["c1-2"] = 1
	l, ok := c.NextLesson(p)
	if !ok || l.ID != "c2-1" {
		t.Fatalf("next = %q %v, want c2-1", l.ID, ok)
	}
	if got := c.RankOf(p); got != curriculum.Intern {
		t.Errorf("rank = %v", got)
	}
	if !c.IsUnlocked(p, "c2") {
		t.Error("c2 should be unlocked")
	}
}

func TestChapterWithIncidentNeedsTheIncident(t *testing.T) {
	c := curriculum.New(incidentLib(t), czech(t))
	inc, ok := c.IncidentDef("c1")
	if !ok || inc.ID != "INC-1001" {
		t.Fatalf("IncidentDef = %v %v", inc, ok)
	}
	p := newProfile()
	p.Stars["c1-1"], p.Stars["c1-2"] = 3, 3
	if l, ok := c.NextLesson(p); ok {
		t.Errorf("lessons done but incident owed: next = %q", l.ID)
	}
	if got := c.CurrentChapter(p); got.ID != "c1" {
		t.Errorf("current = %s, want c1", got.ID)
	}
	if c.IsUnlocked(p, "c2") {
		t.Error("c2 must stay locked until the incident is won")
	}
	p.IncidentBest["INC-1001"] = "SEV3"
	if l, ok := c.NextLesson(p); !ok || l.ID != "c2-1" {
		t.Errorf("after the win next = %q %v", l.ID, ok)
	}
	if got := c.RankOf(p); got != curriculum.Intern {
		t.Errorf("rank = %v", got)
	}
}

func TestIncidentWonBeforeTheLessonsDoesNotCloseTheChapter(t *testing.T) {
	c := curriculum.New(incidentLib(t), czech(t))
	p := newProfile()
	p.IncidentBest["INC-1001"] = "SEV3" // a free-play win with lessons unplayed
	if c.IsDone(p, "c1") || c.IsUnlocked(p, "c2") {
		t.Error("the chapter closed with its lessons unplayed")
	}
	if l, ok := c.NextLesson(p); !ok || l.ID != "c1-1" {
		t.Errorf("next = %q %v, want the first lesson of c1", l.ID, ok)
	}
	p.Stars["c1-1"], p.Stars["c1-2"] = 1, 1
	if !c.IsDone(p, "c1") || !c.IsUnlocked(p, "c2") {
		t.Error("lessons and incident are done, so the chapter is done")
	}
}

func TestChaptersDropsUnresolvableLessons(t *testing.T) {
	lib := incidentLib(t)
	lib.Chapters[0].Lessons = append(lib.Chapters[0].Lessons,
		content.LessonDef{ID: "c1-snip", Title: "snip", Snippets: []string{"missing"}, Speed: [3]float64{10, 15, 20}})
	c := curriculum.New(lib, czech(t))
	if got := len(c.Chapters()[0].Lessons); got != 2 {
		t.Errorf("%d lessons, want 2 (snippet lesson dropped)", got)
	}
	if _, err := c.LessonText("c1-snip"); err == nil {
		t.Error("unresolvable lesson should have no text")
	}
}

func TestSnippetLessonJoinsSnippets(t *testing.T) {
	lib := incidentLib(t)
	lib.Snippets = []content.Snippet{{ID: "s1", Lang: "go", Text: "a := 1"}, {ID: "s2", Lang: "go", Text: "b := 2"}}
	lib.Chapters[0].Introduces += ":=1\n2"
	lib.Chapters[0].Lessons = []content.LessonDef{
		{ID: "c1-code", Title: "code", Snippets: []string{"s1", "s2"}, Speed: [3]float64{10, 15, 20}},
	}
	c := curriculum.New(lib, czech(t))
	text, err := c.LessonText("c1-code")
	if err != nil || text != "a := 1\nb := 2" {
		t.Errorf("text %q err %v", text, err)
	}
}

func TestEmptyChapterNeverBlocksProgress(t *testing.T) {
	lib := incidentLib(t)
	lib.Chapters = append(lib.Chapters[:1], content.ChapterDef{ID: "c-empty", Number: 2, Rank: "Staff"},
		content.ChapterDef{ID: "c3", Number: 3, Rank: "Principal", Lessons: lib.Chapters[1].Lessons})
	c := curriculum.New(lib, czech(t))
	p := newProfile()
	p.Stars["c1-1"], p.Stars["c1-2"] = 1, 1
	p.IncidentBest["INC-1001"] = "SEV3"
	if l, ok := c.NextLesson(p); !ok || l.ID != "c2-1" {
		t.Fatalf("next = %q %v, want to skip the empty chapter", l.ID, ok)
	}
	if !c.IsUnlocked(p, "c3") {
		t.Error("c3 must not wait on an empty chapter")
	}
	if got := c.RankOf(p); got != curriculum.Intern {
		t.Errorf("an empty chapter earned rank %v", got)
	}
}

// walk plays a fresh user through the whole curriculum and checks that every
// step moves forward.
func walk(t *testing.T, c *curriculum.Curriculum) {
	t.Helper()
	p := newProfile()
	last := -1
	for step := 0; step < 1000; step++ {
		cur := c.CurrentChapter(p)
		if cur.Number < last {
			t.Fatalf("step %d: moved back from chapter %d to %d", step, last, cur.Number)
		}
		last = cur.Number
		if !c.IsUnlocked(p, cur.ID) {
			t.Fatalf("step %d: current chapter %s is locked", step, cur.ID)
		}
		if l, ok := c.NextLesson(p); ok {
			if p.Stars[l.ID] != 0 {
				t.Fatalf("step %d: %s offered again", step, l.ID)
			}
			p.Stars[l.ID] = 1
			continue
		}
		if inc, ok := c.IncidentDef(cur.ID); ok && p.IncidentBest[inc.ID] == "" {
			p.IncidentBest[inc.ID] = "SEV3"
			continue
		}
		// Nothing left to do: everything must be finished.
		if last != 12 && last != len(c.Chapters())-1 {
			t.Fatalf("stuck in chapter %d", last)
		}
		for _, ch := range c.Chapters() {
			if !c.IsUnlocked(p, ch.ID) {
				t.Errorf("%s still locked at the end", ch.ID)
			}
		}
		return
	}
	t.Fatal("walk did not end")
}

func TestWalkFromOnboardingToTheEnd(t *testing.T) {
	walk(t, curr(t))
}

func TestWalkWithIncidents(t *testing.T) {
	lib := loadLib(t)
	for _, ch := range lib.Chapters {
		if ch.IncidentID != "" {
			lib.Incidents = append(lib.Incidents, content.Incident{
				ID: ch.IncidentID, Chapter: ch.ID, Steps: []content.Step{{Prompt: "p", Text: "ab", Lang: "sh"}},
			})
		}
	}
	c := curriculum.New(lib, czech(t))
	if _, ok := c.IncidentDef("ch05"); !ok {
		t.Fatal("test incidents were not picked up")
	}
	walk(t, c)
	p := newProfile()
	for _, ch := range c.Chapters() {
		for _, l := range ch.Lessons {
			p.Stars[l.ID] = 1
		}
		if ch.IncidentID != "" {
			p.IncidentBest[ch.IncidentID] = "SEV3"
		}
	}
	if got := c.RankOf(p); got != curriculum.Principal {
		t.Errorf("rank after every incident = %v", got)
	}
}

func TestTestOutWorksForEveryChapter(t *testing.T) {
	c := curr(t)
	for _, ch := range c.Chapters()[1:] {
		text, tg := c.TestOutText(ch.ID)
		if text == "" || tg == (score.Targets{}) {
			t.Errorf("%s: test-out %q targets %v", ch.ID, text, tg)
		}
		if bad := czech(t).Unresolved(text); len(bad) > 0 {
			t.Errorf("%s: untypable %q", ch.ID, string(bad))
		}
		allowed := c.AllowedAt(ch.ID)
		for _, r := range text {
			if !allowed(r) {
				t.Errorf("%s: locked character %q", ch.ID, r)
			}
		}
	}
}

func TestTestOutTargetsFallBackToEarlierChapter(t *testing.T) {
	lib := incidentLib(t)
	lib.Chapters = append(lib.Chapters, content.ChapterDef{ID: "c3", Number: 3, Rank: "Junior", Introduces: "x", TestOutSeed: 5})
	c := curriculum.New(lib, czech(t))
	_, tg := c.TestOutText("c3")
	if tg.Speed != [3]float64{10, 15, 20} {
		t.Errorf("targets = %v", tg.Speed)
	}
}

func TestIsDoneFollowsCompletion(t *testing.T) {
	c := curr(t)
	p := newProfile()
	chs := c.Chapters()
	ch := chs[2] // Horní řada: lessons, no cheap shortcut
	if c.IsDone(p, ch.ID) {
		t.Fatal("a fresh profile has no chapter done, apart from one with nothing to play")
	}
	c.CompleteChapter(p, ch.ID)
	if !c.IsDone(p, ch.ID) {
		t.Error("a completed chapter is done")
	}
	if c.IsDone(p, "nope") {
		t.Error("an unknown chapter is not done")
	}
	other := chs[3]
	if _, hasInc := c.IncidentDef(other.ID); !hasInc {
		t.Skip("chapter 3 has no incident")
	}
	n := len(other.Lessons)
	p.Stars[other.Lessons[n-1].ID] = 3
	if c.IsDone(p, other.ID) {
		t.Error("a star on the last lesson does not finish a chapter whose incident is owed")
	}
}

// runesOf is the set of characters of s.
func runesOf(s string) map[rune]bool {
	set := map[rune]bool{}
	for _, r := range s {
		set[r] = true
	}
	return set
}

// distinctOf counts the characters of chars that occur in seen.
func distinctOf(chars string, seen map[rune]bool) int {
	n := 0
	for r := range runesOf(chars) {
		if seen[r] {
			n++
		}
	}
	return n
}

func TestChapterZeroHasNoTestOut(t *testing.T) {
	text, tg := curr(t).TestOutText("ch00")
	if text != "" || tg != (score.Targets{}) {
		t.Errorf("chapter 0 has a test-out: %q %v", text, tg)
	}
}

// A pass proves what it closes: the text holds every character the chapter
// introduces and a fair sample of every chapter before it.
func TestTestOutProvesItsChapterAndEveryEarlierOne(t *testing.T) {
	c := curr(t)
	chs := c.Chapters()
	for i, ch := range chs {
		if ch.Number == 0 {
			continue
		}
		text, _ := c.TestOutText(ch.ID)
		seen := runesOf(text)
		for _, r := range ch.Introduces {
			if !seen[r] {
				t.Errorf("%s: test-out lacks %q, which the chapter introduces", ch.ID, r)
			}
		}
		for _, e := range chs[:i] {
			n := len([]rune(e.Introduces))
			if n == 0 {
				continue
			}
			if got, want := distinctOf(e.Introduces, seen), max(n/4, 1); got < want {
				t.Errorf("%s: only %d of the %d characters of %s, want at least %d", ch.ID, got, n, e.ID, want)
			}
		}
	}
}

func TestChapterElevenTestOutIsRealSnippets(t *testing.T) {
	c := curr(t)
	text, _ := c.TestOutText("ch11")
	langs := map[string]bool{}
	rest := text
	for rest != "" {
		found := false
		for _, sn := range loadLib(t).Snippets {
			if strings.HasPrefix(rest, sn.Text) {
				rest = strings.TrimPrefix(strings.TrimPrefix(rest, sn.Text), "\n")
				langs[sn.Lang] = true
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("not made of snippets from: %q", rest)
		}
	}
	if len(langs) < 2 {
		t.Errorf("test-out uses only %v, want more than one language", langs)
	}
}

func TestChapterTwelveTestOutIsSymbolRichAcrossAllChapters(t *testing.T) {
	c := curr(t)
	text, _ := c.TestOutText("ch12")
	seen := runesOf(text)
	symbols, all := 0, 0
	for _, r := range text {
		if r == ' ' || r == '\n' {
			continue
		}
		all++
		if !unicode.IsUpper(r) && !unicode.IsLower(r) && !unicode.IsDigit(r) {
			symbols++
		}
	}
	if symbols*5 < all {
		t.Errorf("%d symbols among %d characters, want at least a fifth: %q", symbols, all, text)
	}
	for _, ch := range c.Chapters()[7:11] {
		for _, r := range ch.Introduces {
			if !seen[r] {
				t.Errorf("the test-out of everything lacks %q of %s", r, ch.ID)
			}
		}
	}
}

func TestTestOutOnALayoutWithoutACharacterLeavesItOut(t *testing.T) {
	lib := loadLib(t)
	c := curriculum.New(lib, stubLayout(t))
	text, _ := c.TestOutText("ch03")
	if bad := stubLayout(t).Unresolved(text); len(bad) > 0 {
		t.Errorf("untypable %q in %q", string(bad), text)
	}
}
