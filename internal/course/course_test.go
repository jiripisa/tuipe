package course_test

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/jiripisa/tuipe/internal/content"
	"github.com/jiripisa/tuipe/internal/course"
	"github.com/jiripisa/tuipe/internal/layout"
	"github.com/jiripisa/tuipe/internal/score"
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

func newCourse(t *testing.T) *course.Course {
	t.Helper()
	return course.New(loadLib(t), czech(t))
}

// courseCharacters is space and every focus character of the course.
func courseCharacters(lib *content.Library) map[rune]bool {
	chars := map[rune]bool{' ': true}
	for _, ch := range lib.Course {
		for _, ls := range ch.Lessons {
			for _, r := range ls.Focus {
				chars[r] = true
			}
		}
	}
	return chars
}

// layoutWithout types space and every focus character of the course except the
// ones in missing.
func layoutWithout(t *testing.T, lib *content.Library, missing string) *layout.Layout {
	t.Helper()
	return layoutOf(t, courseCharacters(lib), missing)
}

// layoutOf types chars except the ones in missing.
func layoutOf(t *testing.T, chars map[rune]bool, missing string) *layout.Layout {
	t.Helper()
	var runes []rune
	for r := range chars {
		if !strings.ContainsRune(missing, r) {
			runes = append(runes, r)
		}
	}
	sort.Slice(runes, func(i, j int) bool { return runes[i] < runes[j] })
	var sb strings.Builder
	sb.WriteString("id = \"stub\"\nname = \"Stub\"\nos = \"linux\"\nmodifier_name = \"AltGr\"\n[[row]]\nkeys = [\n")
	for _, r := range runes {
		fmt.Fprintf(&sb, "  { id = \"k%d\", label = \"k\", hand = \"left\", finger = \"index\" },\n", r)
	}
	sb.WriteString("]\n")
	for _, r := range runes {
		fmt.Fprintf(&sb, "[[char]]\nc = %q\nroute = [\"k%d\"]\n", string(r), r)
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

// oldCharacters is every character v0.1.0 taught, except
// Enter, which no lesson of the course has a focus for.
const oldCharacters = "asdfjklů gh" + "qwertyuiop" + "zxcvbnm,." +
	"ABCDEFGHIJKLMNOPQRSTUVWXYZ-_:?!" +
	"ěščřžýáíéúóďťňĚŠČŘŽÝÁÍÉÚÓĎŤŇŮ" + "0123456789" + "+=%/()\"`" +
	"@#${}[];'" + "~^&*<>\\|" + "°ˇ¨§"

func TestEveryOldCharacterIsTaught(t *testing.T) {
	c := newCourse(t)
	if c.Skipped() != 0 {
		t.Fatalf("the built-in layout skips %d lessons", c.Skipped())
	}
	taught := map[rune]bool{}
	for _, ls := range c.Lessons() {
		for _, r := range ls.Focus {
			taught[r] = true
		}
	}
	for _, r := range oldCharacters {
		if !taught[r] {
			t.Errorf("no lesson teaches %q", r)
		}
	}
	last := c.Chapters()[len(c.Chapters())-1].ID
	allowed := c.ChapterAllowed(last)
	for _, r := range oldCharacters + "\n" {
		if !allowed(r) {
			t.Errorf("%q is not allowed at the end of the course", r)
		}
	}
}

func TestLessonIDsAreUnique(t *testing.T) {
	c := newCourse(t)
	seen := map[string]bool{}
	for i, ls := range c.Lessons() {
		if seen[ls.ID] {
			t.Errorf("duplicate lesson id %q", ls.ID)
		}
		seen[ls.ID] = true
		if ls.Index != i {
			t.Errorf("%s: Index = %d, want %d", ls.ID, ls.Index, i)
		}
		got, ok := c.Lesson(ls.ID)
		if !ok || got.ID != ls.ID {
			t.Errorf("Lesson(%q) = %v, %v", ls.ID, got, ok)
		}
	}
	if _, ok := c.Lesson("nope"); ok {
		t.Error("an unknown lesson was found")
	}
}

func TestAllowedGrowsWithTheLesson(t *testing.T) {
	c := newCourse(t)
	first := c.AllowedUpTo("ch01-01")
	for _, r := range "fj " {
		if !first(r) {
			t.Errorf("ch01-01 must allow %q", r)
		}
	}
	for _, r := range "dkě\n" {
		if first(r) {
			t.Errorf("ch01-01 must not allow %q", r)
		}
	}
	if !c.AllowedUpTo("ch01-02")('d') {
		t.Error("ch01-02 must allow d")
	}
	if c.AllowedUpTo("ch05-01")('ě') || !c.AllowedUpTo("ch05-02")('ě') {
		t.Error("ě is allowed from ch05-02 and not before")
	}
	if c.AllowedUpTo("ch10-03")('\n') || c.ChapterAllowed("ch10")('\n') {
		t.Error("Enter is not allowed before the code chapter")
	}
	if !c.AllowedUpTo("ch11-01")('\n') || !c.ChapterAllowed("ch11")('\n') {
		t.Error("Enter is allowed from the code chapter on")
	}
	if !c.ChapterAllowed("ch12")('\n') {
		t.Error("Enter stays allowed after the code chapter")
	}
	if !c.ChapterAllowed("ch01")('g') || c.ChapterAllowed("ch01")('q') {
		t.Error("ch01 allows its own keys and nothing from ch02")
	}
	if c.AllowedUpTo("nope")('a') {
		t.Error("an unknown lesson allows no letter")
	}
}

// gradesFor hands out got stars in order, six to a lesson: accuracy first, then speed.
func gradesFor(c *course.Course, got int) map[string]score.Grade {
	grades := map[string]score.Grade{}
	for _, ls := range c.Lessons() {
		n := min(got, 6)
		if n == 0 {
			break
		}
		grades[ls.ID] = score.Grade{Accuracy: min(n, 3), Speed: max(n-3, 0)}
		got -= n
	}
	return grades
}

func TestStarsCountSixPerLesson(t *testing.T) {
	c := newCourse(t)
	_, total := c.Stars(nil)
	if total != 6*len(c.Lessons()) {
		t.Fatalf("total = %d for %d lessons", total, len(c.Lessons()))
	}
	ls := c.Lessons()
	got, _ := c.Stars(map[string]score.Grade{ls[0].ID: {Accuracy: 3, Speed: 1}, ls[1].ID: {Speed: 2}})
	if got != 6 {
		t.Errorf("got = %d, want 6", got)
	}
}

func TestStarsIgnoreUnknownLessonsAndClamp(t *testing.T) {
	c := newCourse(t)
	got, _ := c.Stars(map[string]score.Grade{
		"ch01-01": {Accuracy: 7, Speed: 9}, "nope": {Accuracy: 3, Speed: 3}, "ch01-02": {Accuracy: -1, Speed: 1},
	})
	if got != 6+1 {
		t.Errorf("got = %d, want 7", got)
	}
}

func TestTargetsAreTheLessonsSpeed(t *testing.T) {
	c := newCourse(t)
	ls := c.Lessons()[0]
	if got := c.Targets(ls.ID); got.Speed != ls.Speed {
		t.Errorf("targets %v, lesson %v", got.Speed, ls.Speed)
	}
	if got := c.Targets("nope"); got != (score.Targets{}) {
		t.Errorf("an unknown lesson has targets %v", got)
	}
}

func TestRankFollowsShareOfSix(t *testing.T) {
	c := newCourse(t)
	_, total := c.Stars(nil)
	for _, tc := range []struct {
		pct  int
		want course.Rank
	}{
		{0, course.Intern}, {9, course.Intern}, {10, course.Junior}, {29, course.Junior},
		{30, course.Medior}, {54, course.Medior}, {55, course.Senior}, {79, course.Senior},
		{80, course.Staff}, {94, course.Staff}, {95, course.Principal}, {100, course.Principal},
	} {
		// The smallest star count that reaches the percentage for the lower edge
		// of a band, the largest one below it for the upper edge.
		got := (tc.pct*total + 99) / 100
		if tc.pct == 9 || tc.pct == 29 || tc.pct == 54 || tc.pct == 79 || tc.pct == 94 {
			got = tc.pct * total / 100
		}
		grades := gradesFor(c, got)
		if g, _ := c.Stars(grades); g != got {
			t.Fatalf("%d%%: got %d stars, want %d", tc.pct, g, got)
		}
		if r := c.Rank(grades); r != tc.want {
			t.Errorf("%d%% (%d/%d): rank %v, want %v", tc.pct, got, total, r, tc.want)
		}
	}
	if s := course.Principal.String(); s != "Principal" {
		t.Errorf("Principal.String() = %q", s)
	}
	if s := course.Rank(99).String(); s != "Intern" {
		t.Errorf("an invalid rank prints %q", s)
	}
}

func TestNextWantsALessonWithNoStar(t *testing.T) {
	c := newCourse(t)
	ls := c.Lessons()
	grades := map[string]score.Grade{ls[0].ID: {Accuracy: 1}, ls[1].ID: {Speed: 3}, ls[2].ID: {}}
	next, ok := c.Next(grades, "")
	if !ok || next.ID != ls[2].ID { // {0,1} and {1,0} count as started, {0,0} does not
		t.Errorf("Next = %v, %v; want %s", next.ID, ok, ls[2].ID)
	}
}

func TestNextStartsAtRecommendation(t *testing.T) {
	c := newCourse(t)
	ls := c.Lessons()
	grades := map[string]score.Grade{ls[5].ID: {Speed: 2}}
	next, ok := c.Next(grades, ls[5].ID)
	if !ok || next.ID != ls[6].ID {
		t.Errorf("Next = %v, %v; want %s", next.ID, ok, ls[6].ID)
	}
	// Past the recommendation nothing is left: wrap to the first lesson without a star.
	grades = map[string]score.Grade{}
	for _, l := range ls[5:] {
		grades[l.ID] = score.Grade{Accuracy: 1}
	}
	next, ok = c.Next(grades, ls[5].ID)
	if !ok || next.ID != ls[0].ID {
		t.Errorf("Next = %v, %v; want %s", next.ID, ok, ls[0].ID)
	}
}

func TestNextWithAllStarred(t *testing.T) {
	c := newCourse(t)
	grades := map[string]score.Grade{}
	for _, l := range c.Lessons() {
		grades[l.ID] = score.Grade{Speed: 1}
	}
	if next, ok := c.Next(grades, c.Lessons()[3].ID); ok {
		t.Errorf("Next = %v, want none", next.ID)
	}
}

func TestNextIgnoresUnknownRecommendation(t *testing.T) {
	c := newCourse(t)
	ls := c.Lessons()
	next, ok := c.Next(map[string]score.Grade{ls[0].ID: {Accuracy: 1}}, "gone-after-layout-change")
	if !ok || next.ID != ls[1].ID {
		t.Errorf("Next = %v, %v; want %s", next.ID, ok, ls[1].ID)
	}
}

func TestUntypableLessonIsLeftOutAndCounted(t *testing.T) {
	lib := loadLib(t)
	c := course.New(lib, layoutWithout(t, lib, "ě"))
	if c.Skipped() == 0 {
		t.Fatal("Skipped() = 0")
	}
	if _, ok := c.Lesson("ch05-02"); ok {
		t.Error("ch05-02 needs ě and must be left out")
	}
	if _, ok := c.Lesson("ch05-03"); !ok {
		t.Fatal("ch05-03 must stay")
	}
	allowed := c.AllowedUpTo("ch05-03")
	if allowed('ě') {
		t.Error("ě must not be allowed on a layout without it")
	}
	if !allowed('č') || !allowed('š') {
		t.Error("č and š are taught by kept lessons")
	}
	ls := c.Lessons()
	for i, l := range ls {
		if l.Index != i {
			t.Errorf("%s: Index = %d, want %d", l.ID, l.Index, i)
		}
	}
}

func TestLanguageLessonNeedsATypableSnippet(t *testing.T) {
	lib := loadLib(t)
	c := course.New(lib, czech(t))
	ls, ok := c.Lesson("ch11-01")
	if !ok || ls.Lang != "go" || len(ls.Focus) != 0 {
		t.Errorf("ch11-01 = %+v, %v", ls, ok)
	}
	bare := course.New(lib, layoutWithout(t, lib, ""))
	if _, ok := bare.Lesson("ch11-01"); ok {
		t.Error("a layout that cannot type any snippet must leave out the code lessons")
	}
	if bare.Skipped() < 7 {
		t.Errorf("Skipped() = %d, want at least the 7 code lessons", bare.Skipped())
	}
}

var allLengths = []course.Length{course.Short, course.Medium, course.Long}

var trainingSeconds = map[course.Length]float64{course.Short: 30, course.Medium: 60, course.Long: 120}

// targetChars is the characters typed in seconds at the two-star speed of ls.
func targetChars(ls course.Lesson, seconds float64) float64 {
	return seconds * ls.Speed[1] * 5 / 60
}

func training(t *testing.T, c *course.Course, id string, n course.Length, seed int64) string {
	t.Helper()
	text, err := c.Training(id, n, seed)
	if err != nil {
		t.Fatalf("Training(%s, %v, %d): %v", id, n, seed, err)
	}
	return text
}

func testText(t *testing.T, c *course.Course, id string, seed int64) string {
	t.Helper()
	text, _, err := c.Test(id, seed)
	if err != nil {
		t.Fatalf("Test(%s, %d): %v", id, seed, err)
	}
	return text
}

func TestLengthNames(t *testing.T) {
	for n, want := range map[course.Length]string{course.Short: "short", course.Medium: "medium", course.Long: "long"} {
		if got := n.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", int(n), got, want)
		}
	}
	if got := course.Length(9).String(); got != "short" {
		t.Errorf("an invalid length prints %q", got)
	}
}

func TestTrainingLengths(t *testing.T) {
	c := newCourse(t)
	for _, ls := range c.Lessons() {
		if ls.Lang != "" {
			continue
		}
		for seed := int64(1); seed <= 5; seed++ {
			last := 0
			for _, n := range allLengths {
				got := utf8.RuneCountInString(training(t, c, ls.ID, n, seed))
				if float64(got) < targetChars(ls, trainingSeconds[n]) {
					t.Errorf("%s %v seed %d: %d characters, want at least %.1f", ls.ID, n, seed, got, targetChars(ls, trainingSeconds[n]))
				}
				if got <= last {
					t.Errorf("%s %v seed %d: %d characters, not longer than the shorter length's %d", ls.ID, n, seed, got, last)
				}
				last = got
			}
		}
	}
}

// focusShare is the share of the non-space characters of text that are focus
// characters.
func focusShare(text string, focus []rune) float64 {
	hits, total := 0, 0
	for _, r := range text {
		if r == ' ' {
			continue
		}
		total++
		if slices.Contains(focus, r) {
			hits++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(hits) / float64(total)
}

func TestTrainingFocusesTheLesson(t *testing.T) {
	c := newCourse(t)
	lowest, where := 1.0, ""
	for _, ls := range c.Lessons() {
		if ls.Lang != "" || ls.Review {
			continue
		}
		for _, n := range allLengths {
			for seed := int64(1); seed <= 20; seed++ {
				text := training(t, c, ls.ID, n, seed)
				share := focusShare(text, ls.Focus)
				if share < lowest {
					lowest, where = share, fmt.Sprintf("%s %v seed %d", ls.ID, n, seed)
				}
				if share < 0.25 {
					t.Errorf("%s %v seed %d: %.1f %% focus characters, want at least 25 %%: %q", ls.ID, n, seed, share*100, text)
				}
			}
		}
	}
	t.Logf("lowest focus share: %.1f %% at %s", lowest*100, where)
}

func TestTrainingHoldsEveryFocusCharacter(t *testing.T) {
	c := newCourse(t)
	for _, ls := range c.Lessons() {
		for _, n := range allLengths {
			for seed := int64(1); seed <= 5; seed++ {
				text := training(t, c, ls.ID, n, seed)
				for _, r := range ls.Focus {
					if !strings.ContainsRune(text, r) {
						t.Errorf("%s %v seed %d: %q is missing from %q", ls.ID, n, seed, r, text)
					}
				}
			}
		}
	}
}

func TestTrainingIsFixedBySeed(t *testing.T) {
	c := newCourse(t)
	for _, id := range []string{"ch03-03", "ch11-01"} {
		if training(t, c, id, course.Medium, 7) != training(t, c, id, course.Medium, 7) {
			t.Errorf("%s: the same seed gave different text", id)
		}
	}
	if training(t, c, "ch03-03", course.Medium, 7) == training(t, c, "ch03-03", course.Medium, 8) {
		t.Error("another seed gave the same text")
	}
}

func TestUnknownLessonIsAnError(t *testing.T) {
	c := newCourse(t)
	if _, err := c.Training("nope", course.Short, 1); err == nil {
		t.Error("Training accepted an unknown lesson")
	}
	if _, err := c.Training("ch01-01", course.Length(9), 1); err == nil {
		t.Error("Training accepted an unknown length")
	}
	if _, _, err := c.Test("nope", 1); err == nil {
		t.Error("Test accepted an unknown lesson")
	}
}

func TestLessonWithoutTextIsAnError(t *testing.T) {
	lib := *loadLib(t)
	lib.Course = []content.CourseChapter{{ID: "x", Lessons: []content.CourseLesson{
		{ID: "x-1", Focus: " ", Speed: [3]float64{10, 15, 20}},
	}}}
	c := course.New(&lib, nil)
	if text, err := c.Training("x-1", course.Short, 1); err == nil {
		t.Errorf("Training gave %q and no error", text)
	}
	if text, _, err := c.Test("x-1", 1); err == nil {
		t.Errorf("Test gave %q and no error", text)
	}
}

func TestTestIsFreshOnEachSeed(t *testing.T) {
	c := newCourse(t)
	for _, id := range []string{"ch01-01", "ch04-08", "ch11-01"} {
		a, b := testText(t, c, id, 1), testText(t, c, id, 2)
		if a == b {
			t.Errorf("%s: seeds 1 and 2 gave the same text %q", id, a)
		}
		if a != testText(t, c, id, 1) {
			t.Errorf("%s: the same seed gave different text", id)
		}
	}
}

func TestTestCarriesTheLessonsTargets(t *testing.T) {
	c := newCourse(t)
	for _, id := range []string{"ch02-01", "ch11-01"} {
		ls, _ := c.Lesson(id)
		_, targets, err := c.Test(id, 1)
		if err != nil {
			t.Fatal(err)
		}
		if targets != (score.Targets{Speed: ls.Speed}) {
			t.Errorf("%s: targets %v, want %v", id, targets, ls.Speed)
		}
	}
}

func TestTestHoldsTheLessonsCharacters(t *testing.T) {
	c := newCourse(t)
	for _, ls := range c.Lessons() {
		if ls.Lang != "" {
			continue
		}
		want := 3
		if ls.Review {
			want = 1 // a long focus takes the whole text at one each
		}
		for seed := int64(1); seed <= 10; seed++ {
			text := testText(t, c, ls.ID, seed)
			for _, r := range ls.Focus {
				if r == ' ' {
					continue
				}
				if got := strings.Count(text, string(r)); got < want {
					t.Errorf("%s seed %d: %q appears %d times, want at least %d: %q", ls.ID, seed, r, got, want, text)
				}
			}
		}
	}
}

func TestTestDrawsFromEarlierLessons(t *testing.T) {
	c := newCourse(t)
	seen := map[rune]bool{}
	for seed := int64(1); seed <= 20; seed++ {
		for _, r := range testText(t, c, "ch02-01", seed) {
			seen[r] = true
		}
	}
	for _, r := range "asdfghjklů" {
		if !seen[r] {
			t.Errorf("%q never appears in the tests of ch02-01", r)
		}
	}
}

// Late in the course a text cannot hold every earlier character, so each attempt
// samples them, and over attempts the sample reaches them all.
func TestTestSamplesEarlierCharactersBySeed(t *testing.T) {
	c := newCourse(t)
	last := lastDrillLesson(c)
	seen := map[rune]bool{}
	for seed := int64(1); seed <= 60; seed++ {
		for _, r := range testText(t, c, last.ID, seed) {
			seen[r] = true
		}
	}
	for _, ls := range c.Lessons() {
		for _, r := range ls.Focus {
			if !seen[r] {
				t.Errorf("%q never appears in 60 tests of %s", r, last.ID)
			}
		}
	}
}

func lastDrillLesson(c *course.Course) course.Lesson {
	ls := c.Lessons()
	for i := len(ls) - 1; i >= 0; i-- {
		if ls[i].Lang == "" {
			return ls[i]
		}
	}
	return course.Lesson{}
}

func TestTestStaysShortLateInTheCourse(t *testing.T) {
	c := newCourse(t)
	for _, ls := range c.Lessons() {
		if ls.Lang != "" {
			continue
		}
		for seed := int64(1); seed <= 20; seed++ {
			limit := 1.5 * math.Ceil(targetChars(ls, 90))
			if n := float64(utf8.RuneCountInString(testText(t, c, ls.ID, seed))); n > limit {
				t.Errorf("%s seed %d: %.0f characters, want at most %.1f", ls.ID, seed, n, limit)
			}
		}
	}
}

// The first lesson allows f, j and space and nothing else, and there is no real
// word in that alphabet.
func TestFirstLessonHasFullLengthText(t *testing.T) {
	c := newCourse(t)
	first := c.Lessons()[0]
	check := func(name, text string, want float64) {
		t.Helper()
		if float64(utf8.RuneCountInString(text)) < want {
			t.Errorf("%s: %d characters, want at least %.1f: %q", name, utf8.RuneCountInString(text), want, text)
		}
		if strings.Trim(text, "fj ") != "" {
			t.Errorf("%s: only f, j and space are allowed: %q", name, text)
		}
		if !strings.Contains(text, "f") || !strings.Contains(text, "j") {
			t.Errorf("%s: f or j is missing: %q", name, text)
		}
	}
	for seed := int64(1); seed <= 20; seed++ {
		for _, n := range allLengths {
			check(n.String(), training(t, c, first.ID, n, seed), targetChars(first, trainingSeconds[n]))
		}
		check("test", testText(t, c, first.ID, seed), targetChars(first, 90))
	}
}

// splitSnippets finds the ids of the snippets that text joins with newlines, each
// used once, and reports whether there is such a split.
func splitSnippets(text string, pool []content.Snippet, used []string) ([]string, bool) {
	for _, s := range pool {
		if slices.Contains(used, s.ID) || !strings.HasPrefix(text, s.Text) {
			continue
		}
		rest := strings.TrimPrefix(text, s.Text)
		if rest == "" {
			return append(used, s.ID), true
		}
		if after, ok := strings.CutPrefix(rest, "\n"); ok {
			if ids, ok := splitSnippets(after, pool, append(used, s.ID)); ok {
				return ids, true
			}
		}
	}
	return nil, false
}

func snippetsOf(lib *content.Library, lang string) []content.Snippet {
	var out []content.Snippet
	for _, s := range lib.Snippets {
		if s.Lang == lang {
			out = append(out, s)
		}
	}
	return out
}

func TestCodeTextIsWholeSnippets(t *testing.T) {
	lib := loadLib(t)
	c := course.New(lib, czech(t))
	for _, ls := range c.Lessons() {
		if ls.Lang == "" {
			continue
		}
		pool := snippetsOf(lib, ls.Lang)
		for _, n := range allLengths {
			for seed := int64(1); seed <= 10; seed++ {
				text := training(t, c, ls.ID, n, seed)
				ids, ok := splitSnippets(text, pool, nil)
				if !ok {
					t.Fatalf("%s %v seed %d: not whole snippets of %s: %q", ls.ID, n, seed, ls.Lang, text)
				}
				if want := int(n) + 1; len(ids) != want {
					t.Errorf("%s %v seed %d: %d snippets, want %d", ls.ID, n, seed, len(ids), want)
				}
			}
		}
	}
}

func TestCodeTestUsesItsLanguageOnly(t *testing.T) {
	lib := loadLib(t)
	c := course.New(lib, czech(t))
	for _, ls := range c.Lessons() {
		if ls.Lang == "" {
			continue
		}
		pool := snippetsOf(lib, ls.Lang)
		texts := map[string]bool{}
		for seed := int64(1); seed <= 10; seed++ {
			text := testText(t, c, ls.ID, seed)
			texts[text] = true
			ids, ok := splitSnippets(text, pool, nil)
			if !ok {
				t.Fatalf("%s seed %d: not whole snippets of %s: %q", ls.ID, seed, ls.Lang, text)
			}
			if len(ids) != 3 {
				t.Errorf("%s seed %d: %d snippets, want 3", ls.ID, seed, len(ids))
			}
		}
		if len(texts) < 2 {
			t.Errorf("%s: 10 seeds gave one text", ls.ID)
		}
	}
}

func TestCodeUsesWhatExistsWithoutRepeating(t *testing.T) {
	lib := *loadLib(t)
	pool := snippetsOf(&lib, "ts")[:2]
	lib.Snippets = pool
	c := course.New(&lib, czech(t))
	for seed := int64(1); seed <= 10; seed++ {
		for name, text := range map[string]string{
			"long": training(t, c, "ch11-04", course.Long, seed),
			"test": testText(t, c, "ch11-04", seed),
		} {
			if ids, ok := splitSnippets(text, pool, nil); !ok || len(ids) != 2 {
				t.Errorf("%s seed %d: %v, %v from a pool of 2", name, seed, ids, ok)
			}
		}
	}
}

// A snippet may be typable without Enter, but snippets joined by Enter are not.
func TestCodeTextStaysTypableWithoutEnter(t *testing.T) {
	lib := *loadLib(t)
	pool := []content.Snippet{{ID: "a", Lang: "go", Text: "fj"}, {ID: "b", Lang: "go", Text: "jf"}}
	lib.Snippets = pool
	l := layoutWithout(t, &lib, "")
	c := course.New(&lib, l)
	if c.CanType('\n') {
		t.Fatal("the stub layout has Enter")
	}
	for seed := int64(1); seed <= 5; seed++ {
		for name, text := range map[string]string{
			"long": training(t, c, "ch11-01", course.Long, seed),
			"test": testText(t, c, "ch11-01", seed),
		} {
			if bad := l.Unresolved(text); len(bad) != 0 {
				t.Errorf("%s seed %d: %q cannot be typed in %q", name, seed, bad, text)
			}
			if _, ok := splitSnippets(text, pool, nil); !ok {
				t.Errorf("%s seed %d: not whole snippets: %q", name, seed, text)
			}
		}
	}
}

func TestTextsAreTypable(t *testing.T) {
	lib := loadLib(t)
	chars := courseCharacters(lib)
	chars['\n'] = true
	for _, s := range lib.Snippets {
		for _, r := range s.Text {
			chars[r] = true
		}
	}
	for name, l := range map[string]*layout.Layout{"macOS": czech(t), "without ě": layoutOf(t, chars, "ě")} {
		c := course.New(lib, l)
		if name == "without ě" && c.Skipped() == 0 {
			t.Fatal("the copy without ě leaves nothing out")
		}
		for _, ls := range c.Lessons() {
			allowed := c.AllowedUpTo(ls.ID)
			var texts []string
			for seed := int64(1); seed <= 3; seed++ {
				for _, n := range allLengths {
					texts = append(texts, training(t, c, ls.ID, n, seed))
				}
				texts = append(texts, testText(t, c, ls.ID, seed))
			}
			for _, text := range texts {
				if text == "" {
					t.Errorf("%s %s: empty text", name, ls.ID)
				}
				if bad := l.Unresolved(text); len(bad) != 0 {
					t.Errorf("%s %s: %q cannot be typed in %q", name, ls.ID, bad, text)
				}
				for _, r := range text {
					if ok := (ls.Lang != "" && c.CanType(r)) || (ls.Lang == "" && allowed(r)); !ok {
						t.Errorf("%s %s: %q is not allowed in %q", name, ls.ID, r, text)
						break
					}
				}
			}
		}
	}
}

// qwertzLayout is the shipped QWERTY layout with y and z exchanged, as a QWERTZ
// keyboard has them.
func qwertzLayout(t *testing.T) *layout.Layout {
	t.Helper()
	b, err := os.ReadFile("../layout/data/macos-czech-qwerty.toml")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, p := range [][2]string{
		{`{ id = "k16", label = "y"`, `{ id = "k16", label = "z"`},
		{`{ id = "k6", label = "z"`, `{ id = "k6", label = "y"`},
		{"c = \"y\"\nroute = [\"k16\"]", "c = \"y\"\nroute = [\"k6\"]"},
		{"route = [\"shift+k16\"]", "route = [\"shift+k6\"]"},
		{"c = \"z\"\nroute = [\"k6\"]", "c = \"z\"\nroute = [\"k16\"]"},
	} {
		if !strings.Contains(s, p[0]) {
			t.Fatalf("fixture no longer applies: %q", p[0])
		}
	}
	// Exchange the routes through a placeholder so the two swaps do not meet.
	s = strings.ReplaceAll(s, `route = ["k16"]`, `route = ["@@"]`)
	s = strings.ReplaceAll(s, `route = ["k6"]`, `route = ["k16"]`)
	s = strings.ReplaceAll(s, `route = ["@@"]`, `route = ["k6"]`)
	s = strings.ReplaceAll(s, `route = ["shift+k16"]`, `route = ["@@"]`)
	s = strings.ReplaceAll(s, `route = ["shift+k6"]`, `route = ["shift+k16"]`)
	s = strings.ReplaceAll(s, `route = ["@@"]`, `route = ["shift+k6"]`)
	s = strings.Replace(s, `id = "macos-czech-qwerty"`, `id = "test-qwertz"`, 1)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "qwertz.toml"), []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
	ls, errs := layout.LoadDir(dir)
	if len(errs) != 0 || len(ls) != 1 {
		t.Fatalf("load: %v %v", ls, errs)
	}
	return ls[0]
}

func TestQWERTZSwapsYAndZInLessons(t *testing.T) {
	lib := loadLib(t)
	lay := qwertzLayout(t)
	if !lay.QWERTZ() {
		t.Fatal("fixture is not QWERTZ")
	}
	c := course.New(lib, lay)
	for id, title := range map[string]string{"ch02-03": "t z", "ch03-05": "y", "ch04-05": "Q P A Y"} {
		ls, ok := c.Lesson(id)
		if !ok || ls.Title != title {
			t.Errorf("%s: title %q, want %q", id, ls.Title, title)
		}
	}
	ls, _ := c.Lesson("ch02-03")
	if string(ls.Focus) != "tz" {
		t.Errorf("ch02-03 focus %q", string(ls.Focus))
	}
	ls, _ = c.Lesson("ch02-06")
	if string(ls.Focus) != "qwertzuiop" {
		t.Errorf("ch02-06 focus %q", string(ls.Focus))
	}
	ls, _ = c.Lesson("ch03-06")
	if ls.Title != "celá dolní řada" || string(ls.Focus) != "yxcvbnm,." {
		t.Errorf("ch03-06 %q %q", ls.Title, string(ls.Focus))
	}
	if !c.AllowedUpTo("ch03-05")('y') || !c.AllowedUpTo("ch02-03")('z') {
		t.Error("allowed sets do not follow the swapped focus")
	}
	if c.AllowedUpTo("ch02-03")('y') {
		t.Error("y is allowed before it is taught")
	}
}

func TestQWERTZKeepsWordsInTitles(t *testing.T) {
	c := course.New(loadLib(t), qwertzLayout(t))
	found := false
	for _, ls := range c.Lessons() {
		if ls.Title == "řada bez g a h" {
			found = true
		}
	}
	if !found {
		t.Error(`title "řada bez g a h" was changed`)
	}
}

func TestQWERTYKeepsLessonsAsWritten(t *testing.T) {
	lib := loadLib(t)
	_ = course.New(lib, qwertzLayout(t)) // must not change what the library holds
	c := course.New(lib, czech(t))
	for id, title := range map[string]string{"ch02-03": "t y", "ch03-05": "z", "ch04-05": "Q P A Z"} {
		if ls, ok := c.Lesson(id); !ok || ls.Title != title {
			t.Errorf("%s: title %q, want %q", id, ls.Title, title)
		}
	}
	if ls, _ := c.Lesson("ch02-06"); string(ls.Focus) != "qwertyuiop" {
		t.Errorf("ch02-06 focus %q", string(ls.Focus))
	}
}
