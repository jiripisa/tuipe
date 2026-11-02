package course_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/jiripisa/tuipe/internal/content"
	"github.com/jiripisa/tuipe/internal/course"
	"github.com/jiripisa/tuipe/internal/hint"
	"github.com/jiripisa/tuipe/internal/layout"
)

func tutorial(t *testing.T, c *course.Course, id string, seed int64) course.Tutorial {
	t.Helper()
	tu, err := c.Tutorial(id, seed)
	if err != nil {
		t.Fatalf("Tutorial(%s): %v", id, err)
	}
	return tu
}

func TestPairLessonLines(t *testing.T) {
	tu := tutorial(t, newCourse(t), "ch01-01", 1)
	want := []string{"f f f f f f f f", "j j j j j j j j", "fj jf fj jf fj jf"}
	if len(tu.Lines) != 4 || !slices.Equal(tu.Lines[:3], want) {
		t.Fatalf("lines = %q, want %q and a closing line", tu.Lines, want)
	}
	if strings.TrimSpace(tu.Lines[3]) == "" {
		t.Error("closing line is empty")
	}
}

func TestLeftHandComesFirst(t *testing.T) {
	lib := *loadLib(t)
	lib.Course = []content.CourseChapter{{ID: "x", Lessons: []content.CourseLesson{
		{ID: "x-1", Focus: "kd", Speed: [3]float64{10, 15, 20}},
	}}}
	c := course.New(&lib, czech(t))
	tu := tutorial(t, c, "x-1", 1)
	if tu.Lines[0] != "d d d d d d d d" || tu.Lines[1] != "k k k k k k k k" || tu.Lines[2] != "dk kd dk kd dk kd" {
		t.Errorf("lines = %q, want d before k", tu.Lines)
	}
	if !strings.HasPrefix(tu.Explain[1], "d ") {
		t.Errorf("explanation = %q, want d first", tu.Explain)
	}
}

func TestExplainNamesTheFinger(t *testing.T) {
	l := czech(t)
	c := course.New(loadLib(t), l)
	for _, id := range []string{"ch01-01", "ch01-02"} {
		ls, _ := c.Lesson(id)
		tu := tutorial(t, c, id, 1)
		for _, r := range ls.Focus {
			if want := hint.Text(l, r); !slices.Contains(tu.Explain, want) {
				t.Errorf("%s: explanation lacks %q: %q", id, want, tu.Explain)
			}
		}
	}
}

func TestKeysAreTheLastKeysOfTheRoutes(t *testing.T) {
	l := czech(t)
	tu := tutorial(t, course.New(loadLib(t), l), "ch01-01", 1)
	var want []string
	for _, r := range "fj " {
		route, _ := l.Route(r)
		want = append(want, route[len(route)-1].Key)
	}
	slices.Sort(want)
	got := slices.Clone(tu.Keys)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("keys = %v, want %v", got, want)
	}
}

func lessonFocusing(t *testing.T, c *course.Course, focus string) course.Lesson {
	t.Helper()
	for _, ls := range c.Lessons() {
		if string(ls.Focus) == focus && !ls.Review {
			return ls
		}
	}
	t.Fatalf("no lesson focusing %q", focus)
	return course.Lesson{}
}

func TestDeadKeyLesson(t *testing.T) {
	l := czech(t)
	c := course.New(loadLib(t), l)
	ls := lessonFocusing(t, c, "ďťň")
	tu := tutorial(t, c, ls.ID, 1)
	if !slices.Contains(tu.Explain, hint.Text(l, 'ď')) {
		t.Fatalf("no explanation of ď: %q", tu.Explain)
	}
	if h := hint.Text(l, 'ď'); !strings.Contains(h, "Shift") || !strings.Contains(h, ", pak ") {
		t.Errorf("hint %q does not name both presses", h)
	}
	if tu.Lines[0] != "ď ď ď ď ď ď ď ď" && !slices.Contains(tu.Lines, "ď ď ď ď ď ď ď ď") {
		t.Errorf("no line of ď: %q", tu.Lines)
	}
	if bad := l.Unresolved(strings.Join(tu.Lines, "")); len(bad) > 0 {
		t.Errorf("lines hold untypable %q", string(bad))
	}
}

func TestReviewLessonHasOneLine(t *testing.T) {
	c := newCourse(t)
	tu := tutorial(t, c, "ch01-05", 1)
	ls, _ := c.Lesson("ch01-05")
	if len(tu.Lines) != 1 || strings.Contains(tu.Lines[0], "\n") {
		t.Errorf("lines = %q, want one line", tu.Lines)
	}
	if len(tu.Explain) != 1 || tu.Explain[0] != ls.Intro {
		t.Errorf("explain = %q, want the intro", tu.Explain)
	}
	for _, r := range ls.Focus {
		if !strings.ContainsRune(tu.Lines[0], r) {
			t.Errorf("line lacks %q", r)
		}
	}
}

func TestCodeLessonIsOneSnippet(t *testing.T) {
	c := newCourse(t)
	tu := tutorial(t, c, "ch11-01", 1)
	if len(tu.Lines) != 1 || len(tu.Keys) != 0 {
		t.Errorf("lines %q keys %q, want one snippet and no keys", tu.Lines, tu.Keys)
	}
	ls, _ := c.Lesson("ch11-01")
	if len(tu.Explain) != 1 || tu.Explain[0] != ls.Intro {
		t.Errorf("explain = %q, want the intro", tu.Explain)
	}
}

func TestTutorialIsFixedBySeedAndVariesWithIt(t *testing.T) {
	c := newCourse(t)
	a, b := tutorial(t, c, "ch01-02", 3), tutorial(t, c, "ch01-02", 3)
	if !slices.Equal(a.Lines, b.Lines) {
		t.Error("the same seed gave different lines")
	}
	varies := false
	for seed := int64(4); seed < 20 && !varies; seed++ {
		varies = !slices.Equal(a.Lines, tutorial(t, c, "ch01-02", seed).Lines)
	}
	if !varies {
		t.Error("the seed never changes the lines")
	}
}

func TestUnknownLessonHasNoTutorial(t *testing.T) {
	if _, err := newCourse(t).Tutorial("nope", 1); err == nil {
		t.Error("no error for an unknown lesson")
	}
}

func TestEveryLessonHasATutorial(t *testing.T) {
	lib := loadLib(t)
	all, err := layout.Builtin()
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range all {
		c := course.New(lib, l)
		for _, ls := range c.Lessons() {
			tu := tutorial(t, c, ls.ID, 7)
			if len(tu.Explain) == 0 || len(tu.Lines) == 0 {
				t.Errorf("%s/%s: empty tutorial %+v", l.ID, ls.ID, tu)
			}
			for _, s := range append(slices.Clone(tu.Explain), tu.Lines...) {
				if strings.TrimSpace(s) == "" {
					t.Errorf("%s/%s: blank line in %+v", l.ID, ls.ID, tu)
				}
			}
			if bad := l.Unresolved(strings.Join(tu.Lines, "\n")); len(bad) > 0 && ls.Lang == "" {
				t.Errorf("%s/%s: untypable %q", l.ID, ls.ID, string(bad))
			}
			if ls.Lang != "" && len(tu.Keys) != 0 {
				t.Errorf("%s/%s: code lesson has keys", l.ID, ls.ID)
			}
		}
	}
}
