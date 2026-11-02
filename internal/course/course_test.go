package course_test

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jiripisa/tuipe/internal/content"
	"github.com/jiripisa/tuipe/internal/course"
	"github.com/jiripisa/tuipe/internal/layout"
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

// layoutWithout types space and every focus character of the course except the
// ones in missing.
func layoutWithout(t *testing.T, lib *content.Library, missing string) *layout.Layout {
	t.Helper()
	chars := map[rune]bool{' ': true}
	for _, ch := range lib.Course {
		for _, ls := range ch.Lessons {
			for _, r := range ls.Focus {
				chars[r] = true
			}
		}
	}
	for _, r := range missing {
		delete(chars, r)
	}
	var runes []rune
	for r := range chars {
		runes = append(runes, r)
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

// oldCharacters is every character the previous curriculum introduced, except
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

// starsFor stars lessons in order until got stars are handed out, three each.
func starsFor(c *course.Course, got int) map[string]int {
	stars := map[string]int{}
	for _, ls := range c.Lessons() {
		n := min(got, 3)
		if n == 0 {
			break
		}
		stars[ls.ID] = n
		got -= n
	}
	return stars
}

func TestRankFollowsStarShare(t *testing.T) {
	c := newCourse(t)
	_, total := c.Stars(nil)
	if total != 3*len(c.Lessons()) {
		t.Fatalf("total = %d for %d lessons", total, len(c.Lessons()))
	}
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
		stars := starsFor(c, got)
		if g, _ := c.Stars(stars); g != got {
			t.Fatalf("%d%%: got %d stars, want %d", tc.pct, g, got)
		}
		if r := c.Rank(stars); r != tc.want {
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

func TestStarsIgnoreUnknownLessonsAndClamp(t *testing.T) {
	c := newCourse(t)
	got, _ := c.Stars(map[string]int{"ch01-01": 7, "nope": 3, "ch01-02": -1})
	if got != 3 {
		t.Errorf("got = %d, want 3", got)
	}
}

func TestNextSkipsStarredLessons(t *testing.T) {
	c := newCourse(t)
	ls := c.Lessons()
	stars := map[string]int{ls[0].ID: 1, ls[1].ID: 3}
	next, ok := c.Next(stars, "")
	if !ok || next.ID != ls[2].ID {
		t.Errorf("Next = %v, %v; want %s", next.ID, ok, ls[2].ID)
	}
}

func TestNextStartsAtRecommendation(t *testing.T) {
	c := newCourse(t)
	ls := c.Lessons()
	stars := map[string]int{ls[5].ID: 2}
	next, ok := c.Next(stars, ls[5].ID)
	if !ok || next.ID != ls[6].ID {
		t.Errorf("Next = %v, %v; want %s", next.ID, ok, ls[6].ID)
	}
	// Past the recommendation nothing is left: wrap to the first lesson without a star.
	stars = map[string]int{}
	for _, l := range ls[5:] {
		stars[l.ID] = 1
	}
	next, ok = c.Next(stars, ls[5].ID)
	if !ok || next.ID != ls[0].ID {
		t.Errorf("Next = %v, %v; want %s", next.ID, ok, ls[0].ID)
	}
}

func TestNextWithAllStarred(t *testing.T) {
	c := newCourse(t)
	stars := map[string]int{}
	for _, l := range c.Lessons() {
		stars[l.ID] = 1
	}
	if next, ok := c.Next(stars, c.Lessons()[3].ID); ok {
		t.Errorf("Next = %v, want none", next.ID)
	}
}

func TestNextIgnoresUnknownRecommendation(t *testing.T) {
	c := newCourse(t)
	ls := c.Lessons()
	next, ok := c.Next(map[string]int{ls[0].ID: 1}, "gone-after-layout-change")
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
