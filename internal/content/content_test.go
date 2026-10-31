package content_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/jiripisa/tuipe/internal/content"
	"github.com/jiripisa/tuipe/internal/curriculum"
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

func TestLoadFillsTheLibrary(t *testing.T) {
	lib := load(t)
	if len(lib.Snippets) == 0 || len(lib.Incidents) == 0 {
		t.Error("snippets and incidents are not loaded")
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

// snippetPrefix maps a snippet language to the prefix of its ids.
var snippetPrefix = map[string]string{
	"go": "go", "python": "py", "js": "js", "ts": "ts", "shell": "sh", "sql": "sql", "regex": "re",
}

func countLang(lib *content.Library, langs ...string) int {
	n := 0
	for _, s := range lib.Snippets {
		for _, l := range langs {
			if s.Lang == l {
				n++
			}
		}
	}
	return n
}

func TestSnippetsAreWideEnoughCoverage(t *testing.T) {
	lib := load(t)
	for _, tc := range []struct {
		name  string
		langs []string
		min   int
	}{
		{"go", []string{"go"}, 15},
		{"python", []string{"python"}, 15},
		{"js and ts", []string{"js", "ts"}, 15},
		{"ts", []string{"ts"}, 5},
		{"shell", []string{"shell"}, 15},
		{"sql", []string{"sql"}, 8},
		{"regex", []string{"regex"}, 8},
	} {
		if got := countLang(lib, tc.langs...); got < tc.min {
			t.Errorf("%s: %d snippets, want at least %d", tc.name, got, tc.min)
		}
	}
	for _, id := range []string{"go-1", "go-2", "go-3", "py-1", "py-2", "js-1", "js-2", "ts-1", "sh-1", "sql-1", "re-1"} {
		found := false
		for _, s := range lib.Snippets {
			found = found || s.ID == id
		}
		if !found {
			t.Errorf("snippet %s, referenced by the curriculum, does not exist", id)
		}
	}
}

func TestSnippetsAreWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range load(t).Snippets {
		prefix, ok := snippetPrefix[s.Lang]
		if !ok || !regexp.MustCompile(`^`+prefix+`-\d+$`).MatchString(s.ID) {
			t.Errorf("snippet %q in language %q: bad id or language", s.ID, s.Lang)
		}
		if seen[s.ID] {
			t.Errorf("duplicate snippet id %s", s.ID)
		}
		seen[s.ID] = true
		if n := utf8.RuneCountInString(s.Text); n < 100 || n > 500 {
			t.Errorf("%s: %d characters, want 100 to 500", s.ID, n)
		}
		lines := strings.Split(s.Text, "\n")
		if len(lines) < 4 || len(lines) > 12 {
			t.Errorf("%s: %d lines, want 4 to 12", s.ID, len(lines))
		}
		if strings.HasPrefix(s.Text, "\n") || strings.HasSuffix(s.Text, "\n") || strings.Contains(s.Text, "\n\n\n") {
			t.Errorf("%s: stray blank lines at an edge or three in a row", s.ID)
		}
		if strings.ContainsAny(s.Text, "\t\r") {
			t.Errorf("%s: contains a tab or carriage return", s.ID)
		}
		for i, l := range lines {
			if l != strings.TrimRight(l, " ") {
				t.Errorf("%s:%d: trailing space", s.ID, i+1)
			}
		}
	}
}

func TestSnippetsFitWidth(t *testing.T) {
	for _, s := range load(t).Snippets {
		for i, l := range strings.Split(s.Text, "\n") {
			if n := utf8.RuneCountInString(l); n > 56 {
				t.Errorf("%s:%d: %d runes, want at most 56", s.ID, i+1, n)
			}
		}
	}
}

func TestSnippetsResolve(t *testing.T) {
	l := czech(t)
	for _, s := range load(t).Snippets {
		if bad := l.Unresolved(s.Text); len(bad) > 0 {
			t.Errorf("%s: characters with no route: %q", s.ID, string(bad))
		}
	}
}

func TestIncidentsHaveStableUniqueIDs(t *testing.T) {
	idPattern := regexp.MustCompile(`^INC-\d{4}$`)
	seen := map[string]bool{}
	for _, inc := range load(t).Incidents {
		if !idPattern.MatchString(inc.ID) || seen[inc.ID] {
			t.Errorf("bad or repeated incident id %q", inc.ID)
		}
		seen[inc.ID] = true
	}
	for n := 1001; n <= 1012; n++ {
		if id := fmt.Sprintf("INC-%d", n); !seen[id] {
			t.Errorf("chapter incident %s is missing", id)
		}
	}
}

func TestIncidentsPerChapter(t *testing.T) {
	lib := load(t)
	lay := czech(t)
	curr := curriculum.New(lib, lay)
	byID := map[string]content.Incident{}
	extras := map[string]int{}
	for _, inc := range lib.Incidents {
		byID[inc.ID] = inc
		if strings.HasPrefix(inc.ID, "INC-2") {
			extras[inc.Chapter]++
		}
		if inc.Title == "" {
			t.Errorf("%s: no title", inc.ID)
		}
		if n := len(inc.Steps); n < 3 || n > 5 {
			t.Errorf("%s: %d steps, want 3 to 5", inc.ID, n)
		}
		allowed := curr.AllowedAt(inc.Chapter)
		for i, st := range inc.Steps {
			where := fmt.Sprintf("%s step %d", inc.ID, i+1)
			if st.Prompt == "" || st.Text == "" {
				t.Errorf("%s: empty prompt or text", where)
			}
			if st.Text != strings.TrimSpace(st.Text) || strings.Contains(st.Text, "\t") {
				t.Errorf("%s: badly spaced text %q", where, st.Text)
			}
			for _, line := range strings.Split(st.Text, "\n") {
				if body := strings.TrimLeft(line, " "); strings.Contains(body, "  ") || line != strings.TrimRight(line, " ") {
					t.Errorf("%s: badly spaced line %q", where, line)
				}
				if n := utf8.RuneCountInString(line); n > 56 {
					t.Errorf("%s: line of %d runes, want at most 56", where, n)
				}
			}
			for _, r := range st.Text {
				if !allowed(r) {
					t.Errorf("%s: %q is not unlocked in %s", where, r, inc.Chapter)
					break
				}
			}
			if bad := lay.Unresolved(st.Text); len(bad) > 0 {
				t.Errorf("%s: characters with no route: %q", where, string(bad))
			}
		}
	}
	for _, ch := range lib.Chapters {
		if ch.Number == 0 {
			continue
		}
		inc, ok := byID[ch.IncidentID]
		if !ok {
			t.Errorf("%s: incident %s does not exist", ch.ID, ch.IncidentID)
			continue
		}
		if inc.Chapter != ch.ID {
			t.Errorf("%s: belongs to chapter %q, want %q", inc.ID, inc.Chapter, ch.ID)
		}
		if _, ok := curr.IncidentDef(ch.ID); !ok {
			t.Errorf("%s: curriculum does not resolve its incident", ch.ID)
		}
	}
	for _, ch := range []string{"ch11", "ch12"} {
		if extras[ch] < 3 {
			t.Errorf("%s: %d free-play incidents, want at least 3", ch, extras[ch])
		}
	}
}
