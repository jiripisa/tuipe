package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jiripisa/tuipe/internal/stats"
)

func dirs(t *testing.T) (cfg, data string) {
	t.Helper()
	root := t.TempDir()
	return filepath.Join(root, "config"), filepath.Join(root, "data")
}

func open(t *testing.T, cfg, data string) *Store {
	t.Helper()
	s, err := Open(cfg, data)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return s
}

func TestProfileRoundTrip(t *testing.T) {
	cfg, data := dirs(t)
	s := open(t, cfg, data)
	p := s.Profile()
	st := stats.New()
	st.Chars["a"] = &stats.Entry{LatencyMs: 210, ErrorRate: 0.05, Samples: 12, Mastery: stats.Learning, LastSeen: time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)}
	p.Stats["mac-cs"] = st
	p.Stars["l1"] = 3
	p.Completed["c1"] = true
	p.FirstRunDone = true
	p.Resume = &Resume{Kind: "lesson", LessonID: "l1", Day: "2026-10-31"}
	if err := s.SaveProfile(); err != nil {
		t.Fatal(err)
	}
	want := *p
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s2 := open(t, cfg, data)
	defer s2.Close()
	if !reflect.DeepEqual(*s2.Profile(), want) {
		t.Fatalf("profile differs after reopen:\n got %+v\nwant %+v", *s2.Profile(), want)
	}
	if s2.Profile().Schema != SchemaVersion {
		t.Fatalf("schema = %d", s2.Profile().Schema)
	}
}

func TestRunsAppendAndSkipBrokenLine(t *testing.T) {
	cfg, data := dirs(t)
	s := open(t, cfg, data)
	defer s.Close()
	r1 := Run{ID: "1", At: time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC), Mode: "lesson", WPM: 40, Accuracy: 97, Stars: 2}
	r2 := Run{ID: "2", At: time.Date(2026, 10, 31, 10, 0, 0, 0, time.UTC), Mode: "drill"}
	if err := s.AppendRun(r1); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(filepath.Join(data, "runs.jsonl"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("{not json\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err := s.AppendRun(r2); err != nil {
		t.Fatal(err)
	}
	got, err := s.Runs()
	if err != nil {
		t.Fatal(err)
	}
	r1.Schema, r2.Schema = SchemaVersion, SchemaVersion
	if !reflect.DeepEqual(got, []Run{r1, r2}) {
		t.Fatalf("runs = %+v", got)
	}
}

func TestRunsEmptyWhenNoFile(t *testing.T) {
	cfg, data := dirs(t)
	s := open(t, cfg, data)
	defer s.Close()
	got, err := s.Runs()
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestCorruptProfileIsSetAside(t *testing.T) {
	cfg, data := dirs(t)
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, "profile.json"), []byte("garbage{{"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := open(t, cfg, data)
	defer s.Close()
	if s.Profile() == nil || s.Profile().Stars == nil || s.Profile().Schema != SchemaVersion {
		t.Fatalf("expected a fresh profile, got %+v", s.Profile())
	}
	m, _ := filepath.Glob(filepath.Join(data, "profile.json.corrupt-*"))
	if len(m) != 1 {
		t.Fatalf("expected one set-aside file, got %v", m)
	}
	b, _ := os.ReadFile(m[0])
	if string(b) != "garbage{{" {
		t.Fatalf("set-aside content = %q", b)
	}
	if len(s.Notices()) == 0 {
		t.Fatal("expected a notice")
	}
}

func TestSecondCorruptionDoesNotOverwriteFirst(t *testing.T) {
	cfg, data := dirs(t)
	for i := 0; i < 2; i++ {
		if err := os.MkdirAll(data, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(data, "profile.json"), []byte("bad"), 0o644); err != nil {
			t.Fatal(err)
		}
		open(t, cfg, data).Close()
	}
	m, _ := filepath.Glob(filepath.Join(data, "profile.json.corrupt-*"))
	if len(m) != 2 {
		t.Fatalf("expected two set-aside files, got %v", m)
	}
}

func TestNewerSchemaIsNotTouched(t *testing.T) {
	cfg, data := dirs(t)
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(data, "profile.json")
	orig := []byte(`{"Schema": 999}`)
	if err := os.WriteFile(path, orig, 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Open(cfg, data)
	if !errors.Is(err, ErrNewerData) {
		t.Fatalf("err = %v, s = %v", err, s)
	}
	b, _ := os.ReadFile(path)
	if string(b) != string(orig) {
		t.Fatalf("file changed: %q", b)
	}
	if m, _ := filepath.Glob(path + ".corrupt-*"); len(m) != 0 {
		t.Fatalf("file was set aside: %v", m)
	}
	// The lock must have been released again.
	if _, err := Open(cfg, data); !errors.Is(err, ErrNewerData) {
		t.Fatalf("second open err = %v", err)
	}
}

func TestSecondOpenIsLocked(t *testing.T) {
	cfg, data := dirs(t)
	s := open(t, cfg, data)
	if _, err := Open(cfg, data); !errors.Is(err, ErrLocked) {
		t.Fatalf("err = %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(cfg, data)
	if err != nil {
		t.Fatalf("open after close: %v", err)
	}
	s2.Close()
}

func skipIfCannotMakeReadOnly(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("chmod does not make a directory read-only on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
}

func TestSaveFailureReturnsErrorAndKeepsOldFile(t *testing.T) {
	skipIfCannotMakeReadOnly(t)
	cfg, data := dirs(t)
	s := open(t, cfg, data)
	defer s.Close()
	s.Profile().Stars["l1"] = 1
	if err := s.SaveProfile(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(data, "profile.json")
	before, _ := os.ReadFile(path)

	if err := os.Chmod(data, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(data, 0o755)
	s.Profile().Stars["l1"] = 3
	if err := s.SaveProfile(); err == nil {
		t.Fatal("expected an error")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("old file was modified")
	}
}

func TestPathsHonourXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/x/cfg")
	t.Setenv("XDG_DATA_HOME", "/x/data")
	c, d, err := Paths()
	if err != nil {
		t.Fatal(err)
	}
	if c != filepath.Join("/x/cfg", "tuipe") || d != filepath.Join("/x/data", "tuipe") {
		t.Fatalf("got %q, %q", c, d)
	}
}

func TestPathsDefaultsUnderHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	c, d, err := Paths()
	if err != nil {
		t.Fatal(err)
	}
	if c != filepath.Join(home, ".config", "tuipe") || d != filepath.Join(home, ".local", "share", "tuipe") {
		t.Fatalf("got %q, %q", c, d)
	}
}

func TestOpenCreatesDirectories(t *testing.T) {
	cfg, data := dirs(t)
	open(t, cfg, data).Close()
	for _, d := range []string{cfg, data} {
		if fi, err := os.Stat(d); err != nil || !fi.IsDir() {
			t.Fatalf("%s not created: %v", d, err)
		}
	}
}

func TestConfigDefaultsAndRoundTrip(t *testing.T) {
	cfg, data := dirs(t)
	s := open(t, cfg, data)
	if got := s.Config(); got != (Config{UpdateCheck: true}) {
		t.Fatalf("default config = %+v", got)
	}
	want := Config{Style: "terminal", Layout: "mac-cs", UpdateCheck: false}
	if err := s.SaveConfig(want); err != nil {
		t.Fatal(err)
	}
	if s.Config() != want {
		t.Fatalf("in-memory config = %+v", s.Config())
	}
	s.Close()
	s2 := open(t, cfg, data)
	defer s2.Close()
	if s2.Config() != want {
		t.Fatalf("reloaded config = %+v", s2.Config())
	}
	if _, err := os.Stat(filepath.Join(cfg, "config.toml")); err != nil {
		t.Fatal(err)
	}
}

func TestCorruptConfigIsSetAside(t *testing.T) {
	cfg, data := dirs(t)
	if err := os.MkdirAll(cfg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg, "config.toml"), []byte("= = ="), 0o644); err != nil {
		t.Fatal(err)
	}
	s := open(t, cfg, data)
	defer s.Close()
	if s.Config() != (Config{UpdateCheck: true}) {
		t.Fatalf("config = %+v", s.Config())
	}
	if m, _ := filepath.Glob(filepath.Join(cfg, "config.toml.corrupt-*")); len(m) != 1 {
		t.Fatalf("set-aside files = %v", m)
	}
	if len(s.Notices()) == 0 {
		t.Fatal("expected a notice")
	}
}

func TestAppendAfterTornTailKeepsNewRun(t *testing.T) {
	cfg, data := dirs(t)
	s := open(t, cfg, data)
	defer s.Close()
	if err := s.AppendRun(Run{ID: "1"}); err != nil {
		t.Fatal(err)
	}
	f, _ := os.OpenFile(filepath.Join(data, "runs.jsonl"), os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString(`{"ID":"torn","WP`)
	f.Close()
	if err := s.AppendRun(Run{ID: "2"}); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Runs()
	if len(got) != 2 || got[0].ID != "1" || got[1].ID != "2" {
		t.Fatalf("runs = %+v", got)
	}
}

func TestRunsSkipEmptyIDLines(t *testing.T) {
	cfg, data := dirs(t)
	s := open(t, cfg, data)
	defer s.Close()
	os.WriteFile(filepath.Join(data, "runs.jsonl"), []byte("{}\nnull\n{\"ID\":\"ok\"}\n"), 0o644)
	got, _ := s.Runs()
	if len(got) != 1 || got[0].ID != "ok" {
		t.Fatalf("runs = %+v", got)
	}
}

func TestNilStatsEntriesAreDropped(t *testing.T) {
	cfg, data := dirs(t)
	os.MkdirAll(data, 0o755)
	os.WriteFile(filepath.Join(data, "profile.json"), []byte(`{"Schema":1,"Stats":{"x":null}}`), 0o644)
	s := open(t, cfg, data)
	defer s.Close()
	if len(s.Profile().Stats) != 0 {
		t.Fatalf("stats = %v", s.Profile().Stats)
	}
}

func TestCorruptedProfileNoticeSaysWhatHappenedAndWhere(t *testing.T) {
	cfg, data := dirs(t)
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, "profile.json"), []byte("garbage{{"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := open(t, cfg, data)
	defer s.Close()
	n := s.Notices()
	if len(n) != 1 || !strings.Contains(n[0], "Profil byl poškozený") || !strings.Contains(n[0], filepath.Join(data, "profile.json.corrupt-1")) {
		t.Errorf("notices = %q", n)
	}
}

func copyFixture(t *testing.T, data string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "profile-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, "profile.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// The fixture is what a first release writes. If this test fails, a field was
// renamed or moved and users would lose that data: add a migration instead.
func TestGoldenV1ProfileLoads(t *testing.T) {
	cfg, data := dirs(t)
	copyFixture(t, data)
	s := open(t, cfg, data)
	defer s.Close()
	if n := s.Notices(); len(n) != 0 {
		t.Fatalf("the fixture was set aside: %q", n)
	}
	p := s.Profile()
	e := p.Stats["macos-czech-qwerty"].Chars["{"]
	if e == nil || e.Samples != 44 || e.Mastery != stats.Solid || e.LatencyMs != 412.5 {
		t.Errorf("char entry = %+v", e)
	}
	if b := p.Stats["macos-czech-qwerty"].Bigrams["st"]; b == nil || b.Samples != 12 {
		t.Errorf("bigram entry = %+v", b)
	}
	if p.Stars["ch01-01"] != 2 || !p.Completed["ch01"] {
		t.Errorf("progress = %+v %+v", p.Stars, p.Completed)
	}
	if !p.FirstRunDone || !p.FocusTerminals["ghostty|tmux=false"] ||
		p.LastUpdateCheck != "2026-10-30" || p.LatestSeen != "v1.0.0" || !p.SkipNoted["user-layout"] {
		t.Errorf("flags = %+v", p)
	}
	r := p.Resume
	if r == nil || r.Kind != "lesson" || r.LessonID != "ch01-01" || r.Day != "2026-10-30" {
		t.Fatalf("resume = %+v", r)
	}
}

// keysOf lists the object keys of a JSON document, with their paths. Map keys
// are data, so what is under a map is walked with a wildcard.
func keysOf(t *testing.T, raw []byte) []string {
	t.Helper()
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	var out []string
	var walk func(path string, v any)
	walk = func(path string, v any) {
		if m, ok := v.(map[string]any); ok {
			for k, sub := range m {
				out = append(out, path+"/"+k)
				walk(path+"/"+k, sub)
			}
		}
	}
	walk("", v)
	slices.Sort(out)
	return out
}

func TestSavingKeepsEveryKeyOfTheGoldenProfile(t *testing.T) {
	cfg, data := dirs(t)
	copyFixture(t, data)
	want, err := os.ReadFile(filepath.Join(data, "profile.json"))
	if err != nil {
		t.Fatal(err)
	}
	s := open(t, cfg, data)
	defer s.Close()
	if err := s.SaveProfile(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(data, "profile.json"))
	if err != nil {
		t.Fatal(err)
	}
	have := keysOf(t, got)
	for _, k := range keysOf(t, want) {
		if !slices.Contains(have, k) {
			t.Errorf("saving dropped the key %s", k)
		}
	}
}

// Every field carries a JSON tag, so that a later rename in Go cannot drop data.
func TestPersistedStructsHaveJSONTags(t *testing.T) {
	for _, v := range []any{Profile{}, Resume{}, Run{}, stats.Stats{}, stats.Entry{}} {
		ty := reflect.TypeOf(v)
		for i := range ty.NumField() {
			f := ty.Field(i)
			if tag := f.Tag.Get("json"); tag != f.Name {
				t.Errorf("%s.%s has tag %q, want the key %q as it always was", ty.Name(), f.Name, tag, f.Name)
			}
		}
	}
}

func TestDirectoryThatCannotBeMadeIsToldInCzech(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "file")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Open(filepath.Join(blocker, "cfg"), filepath.Join(dir, "data"))
	if err == nil || !strings.Contains(err.Error(), "nelze vytvořit") || strings.Contains(err.Error(), "mkdir") {
		t.Errorf("error = %v", err)
	}
}
