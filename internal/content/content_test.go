package content_test

import (
	"regexp"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/jiripisa/tuipe/internal/content"
	"github.com/jiripisa/tuipe/internal/layout"
)

func load(t *testing.T) *content.Library {
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

func checkList(t *testing.T, name string, entries []string, minCount int, lowercase bool) {
	t.Helper()
	l := czech(t)
	if len(entries) < minCount {
		t.Errorf("%s: %d entries, want at least %d", name, len(entries), minCount)
	}
	seen := map[string]bool{}
	for _, e := range entries {
		if e == "" || e != strings.TrimSpace(e) || strings.Contains(e, "  ") {
			t.Errorf("%s: badly spaced entry %q", name, e)
		}
		if seen[e] {
			t.Errorf("%s: duplicate %q", name, e)
		}
		seen[e] = true
		if lowercase && e != strings.ToLower(e) {
			t.Errorf("%s: %q is not lowercase", name, e)
		}
		if !utf8.ValidString(e) {
			t.Errorf("%s: %q is not valid UTF-8", name, e)
		}
		for _, r := range e {
			if r != ' ' && !unicode.IsPrint(r) {
				t.Errorf("%s: %q has non-printable %U", name, e, r)
			}
		}
		if bad := l.Unresolved(e); len(bad) > 0 {
			t.Errorf("%s: %q has characters with no route: %q", name, e, string(bad))
		}
	}
}

func TestWordListsAreCleanAndLargeEnough(t *testing.T) {
	lib := load(t)
	checkList(t, "WordsCS", lib.WordsCS, 1500, true)
	checkList(t, "WordsEN", lib.WordsEN, 500, true)
	checkList(t, "CommitsCS", lib.CommitsCS, 150, false)
	for _, w := range append(append([]string{}, lib.WordsCS...), lib.WordsEN...) {
		if strings.Contains(w, " ") {
			t.Errorf("word %q contains a space", w)
		}
	}
	for _, c := range lib.CommitsCS {
		if r, _ := utf8.DecodeRuneInString(c); !unicode.IsLower(r) {
			t.Errorf("commit %q does not start with a lowercase letter", c)
		}
		if n := utf8.RuneCountInString(c); n > 56 {
			t.Errorf("commit %q has %d characters, want at most 56", c, n)
		}
		if strings.HasSuffix(c, ".") {
			t.Errorf("commit %q ends with a period", c)
		}
	}
}

func TestLoadFillsCurriculumOnly(t *testing.T) {
	lib := load(t)
	if lib.Snippets != nil || lib.Incidents != nil {
		t.Error("snippets and incidents are filled by task 11")
	}
	if len(lib.Chapters) != 13 {
		t.Errorf("%d chapters, want 13", len(lib.Chapters))
	}
}

func TestCurriculumDataIsWellFormed(t *testing.T) {
	incidentID := regexp.MustCompile(`^INC-\d{4}$`)
	chapterIDs, lessonIDs, incidentIDs := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for i, ch := range load(t).Chapters {
		if ch.Number != i || chapterIDs[ch.ID] || ch.ID == "" || ch.Title == "" {
			t.Errorf("chapter %d: bad number, id or title (%q)", i, ch.ID)
		}
		chapterIDs[ch.ID] = true
		if ch.Number == 0 {
			if ch.IncidentID != "" || len(ch.Lessons) != 1 || ch.Allowed != "fj " {
				t.Errorf("chapter 0: incident %q, %d lessons, allowed %q", ch.IncidentID, len(ch.Lessons), ch.Allowed)
			}
		} else {
			if !incidentID.MatchString(ch.IncidentID) || incidentIDs[ch.IncidentID] {
				t.Errorf("%s: bad or repeated incident id %q", ch.ID, ch.IncidentID)
			}
			incidentIDs[ch.IncidentID] = true
			if n := len(ch.Lessons); n < 6 || n > 9 {
				t.Errorf("%s: %d lessons, want 6 to 9", ch.ID, n)
			}
		}
		if ch.TestOutSeed == 0 && ch.Number > 0 {
			t.Errorf("%s: no test-out seed", ch.ID)
		}
		for _, l := range ch.Lessons {
			if l.ID == "" || l.Title == "" || lessonIDs[l.ID] || !strings.HasPrefix(l.ID, ch.ID+"-") {
				t.Errorf("%s: bad lesson id or title %q", ch.ID, l.ID)
			}
			lessonIDs[l.ID] = true
			if l.Speed[0] <= 0 || l.Speed[0] >= l.Speed[1] || l.Speed[1] >= l.Speed[2] {
				t.Errorf("%s: speeds %v not rising", l.ID, l.Speed)
			}
			if len(l.Snippets) == 0 && (l.Length <= 0 || l.Seed == 0) {
				t.Errorf("%s: drill lesson needs a length and a seed", l.ID)
			}
			if len(l.Snippets) > 0 && ch.Number != 11 {
				t.Errorf("%s: snippets outside chapter 11", l.ID)
			}
		}
	}
}
