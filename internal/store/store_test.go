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
	p.FirstRunDone = true
	p.Recommended = "l2"
	p.Taught["l1"] = true
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
	r1 := Run{ID: "1", At: time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC), Mode: "lesson", WPM: 40, Accuracy: 97}
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
	if s.Profile() == nil || s.Profile().Taught == nil || s.Profile().Schema != SchemaVersion {
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
	s.Profile().Recommended = "l1"
	if err := s.SaveProfile(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(data, "profile.json")
	before, _ := os.ReadFile(path)

	if err := os.Chmod(data, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(data, 0o755)
	s.Profile().Recommended = "l3"
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
	os.WriteFile(filepath.Join(data, "profile.json"), []byte(`{"Schema":2,"Stats":{"x":null}}`), 0o644)
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
	b, err := os.ReadFile(filepath.Join("testdata", "profile-v2.json"))
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

// The fixture is what schema 2 writes. If this test fails, a field was
// renamed or moved and users would lose that data: add a migration instead.
func TestGoldenV2ProfileLoads(t *testing.T) {
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
	if !p.FirstRunDone || !p.FocusTerminals["ghostty|tmux=false"] ||
		p.LastUpdateCheck != "2026-10-30" || p.LatestSeen != "v1.0.0" || !p.SkipNoted["user-layout"] {
		t.Errorf("flags = %+v", p)
	}
	if p.Recommended != "ch02-01" || !p.Taught["ch01-01"] {
		t.Errorf("course = %q %+v", p.Recommended, p.Taught)
	}
	if len(p.Pending) != 1 || p.Pending[0] != "Soubor profile.json je ze starší verze tuipe." {
		t.Errorf("pending = %q", p.Pending)
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
	for _, v := range []any{Profile{}, Run{}, stats.Stats{}, stats.Entry{}} {
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

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestOldProfileIsSetAside(t *testing.T) {
	cfg, data := dirs(t)
	old := `{"Schema":1,"FirstRunDone":true,"Stars":{"ch01-01":3}}`
	writeFile(t, filepath.Join(data, "profile.json"), old)
	s := open(t, cfg, data)
	defer s.Close()
	if p := s.Profile(); p.FirstRunDone || p.Schema != SchemaVersion {
		t.Fatalf("expected a fresh profile, got %+v", p)
	}
	if got := readFile(t, filepath.Join(data, "profile.json.v1")); got != old {
		t.Fatalf("set-aside content = %q", got)
	}
	if exists(filepath.Join(data, "profile.json")) {
		t.Fatal("the old profile is still in place")
	}
}

func TestOldRunsAreSetAside(t *testing.T) {
	cfg, data := dirs(t)
	old := `{"Schema":1,"ID":"a","Mode":"lesson"}` + "\n"
	writeFile(t, filepath.Join(data, "runs.jsonl"), old)
	s := open(t, cfg, data)
	defer s.Close()
	if runs, _ := s.Runs(); len(runs) != 0 {
		t.Fatalf("runs = %+v", runs)
	}
	if got := readFile(t, filepath.Join(data, "runs.jsonl.v1")); got != old {
		t.Fatalf("set-aside content = %q", got)
	}
	if err := s.AppendRun(Run{ID: "b", Mode: ModeTest}); err != nil {
		t.Fatal(err)
	}
	if runs, _ := s.Runs(); len(runs) != 1 || runs[0].Schema != SchemaVersion {
		t.Fatalf("runs = %+v", runs)
	}
}

func TestRunsFileSchemaIsJudgedByTheFirstValidLine(t *testing.T) {
	for name, tc := range map[string]struct {
		body    string
		wantOld bool
	}{
		"torn first line is skipped": {"{not json\n{\"Schema\":2,\"ID\":\"a\"}\n", false},
		"old line first is a mix":    {"{\"Schema\":1,\"ID\":\"a\"}\n{\"Schema\":2,\"ID\":\"b\"}\n", true},
		"no schema counts as old":    {"{\"ID\":\"a\"}\n", true},
		"empty file stays":           {"", false},
		"no valid line stays":        {"garbage\n{}\n", false},
	} {
		t.Run(name, func(t *testing.T) {
			cfg, data := dirs(t)
			writeFile(t, filepath.Join(data, "runs.jsonl"), tc.body)
			open(t, cfg, data).Close()
			if got := exists(filepath.Join(data, "runs.jsonl.v1")); got != tc.wantOld {
				t.Fatalf("set aside = %v, want %v", got, tc.wantOld)
			}
			if exists(filepath.Join(data, "runs.jsonl")) == tc.wantOld {
				t.Fatalf("runs.jsonl present = %v", !tc.wantOld)
			}
		})
	}
}

// A run history from a newer tuipe is refused like a newer profile: lines of
// this version must never be appended to it, and nothing else is touched either.
func TestNewerRunsAreRefusedAndNotTouched(t *testing.T) {
	for name, profile := range map[string]string{
		"no profile":     "",
		"an old profile": `{"Schema":1}`,
	} {
		t.Run(name, func(t *testing.T) {
			cfg, data := dirs(t)
			runs := "{torn\n" + `{"Schema":3,"ID":"a"}` + "\n" + `{"Schema":2,"ID":"b"}` + "\n"
			writeFile(t, filepath.Join(data, "runs.jsonl"), runs)
			if profile != "" {
				writeFile(t, filepath.Join(data, "profile.json"), profile)
			}
			for range 2 { // the lock is released again
				if s, err := Open(cfg, data); !errors.Is(err, ErrNewerData) {
					t.Fatalf("err = %v, s = %v", err, s)
				}
			}
			if got := readFile(t, filepath.Join(data, "runs.jsonl")); got != runs {
				t.Fatalf("runs.jsonl changed: %q", got)
			}
			if m, _ := filepath.Glob(filepath.Join(data, "*.v1*")); len(m) != 0 {
				t.Fatalf("set aside: %v", m)
			}
			if profile != "" && readFile(t, filepath.Join(data, "profile.json")) != profile {
				t.Fatal("the profile changed")
			}
			if profile == "" && exists(filepath.Join(data, "profile.json")) {
				t.Fatal("a profile was written")
			}
		})
	}
}

func TestEachFileIsJudgedOnItsOwn(t *testing.T) {
	cfg, data := dirs(t)
	// A schema 1 profile with no runs file.
	writeFile(t, filepath.Join(data, "profile.json"), `{"Schema":1}`)
	open(t, cfg, data).Close()
	if !exists(filepath.Join(data, "profile.json.v1")) || exists(filepath.Join(data, "runs.jsonl.v1")) {
		t.Fatal("only the profile should be set aside")
	}

	// A schema 2 profile next to schema 1 runs.
	cfg, data = dirs(t)
	writeFile(t, filepath.Join(data, "profile.json"), `{"Schema":2,"FirstRunDone":true}`)
	writeFile(t, filepath.Join(data, "runs.jsonl"), `{"Schema":1,"ID":"a"}`+"\n")
	s := open(t, cfg, data)
	defer s.Close()
	if !s.Profile().FirstRunDone || exists(filepath.Join(data, "profile.json.v1")) {
		t.Fatal("the new profile must stay")
	}
	if !exists(filepath.Join(data, "runs.jsonl.v1")) {
		t.Fatal("the old runs should be set aside")
	}
}

func TestSecondSetAsideKeepsTheFirst(t *testing.T) {
	cfg, data := dirs(t)
	writeFile(t, filepath.Join(data, "profile.json.v1"), "first")
	writeFile(t, filepath.Join(data, "profile.json"), `{"Schema":1}`)
	open(t, cfg, data).Close()
	if got := readFile(t, filepath.Join(data, "profile.json.v1")); got != "first" {
		t.Fatalf("the first set-aside file was overwritten: %q", got)
	}
	if got := readFile(t, filepath.Join(data, "profile.json.v1-2")); got != `{"Schema":1}` {
		t.Fatalf("second set-aside = %q", got)
	}
}

func TestSetAsideShowsANotice(t *testing.T) {
	cfg, data := dirs(t)
	writeFile(t, filepath.Join(data, "profile.json"), `{"Schema":1}`)
	writeFile(t, filepath.Join(data, "runs.jsonl"), `{"Schema":1,"ID":"a"}`+"\n")
	s := open(t, cfg, data)
	defer s.Close()
	n := s.Notices()
	if len(n) != 2 {
		t.Fatalf("notices = %q", n)
	}
	for i, f := range []string{"profile.json", "runs.jsonl"} {
		if !strings.Contains(n[i], "starší verze") || !strings.Contains(n[i], filepath.Join(data, f+".v1")) {
			t.Errorf("notice %d = %q", i, n[i])
		}
	}
}

func TestNewProfileKeepsTaughtAndRecommended(t *testing.T) {
	cfg, data := dirs(t)
	s := open(t, cfg, data)
	s.Profile().Taught["ch01-01"] = true
	s.Profile().Recommended = "ch02-01"
	if err := s.SaveProfile(); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s = open(t, cfg, data)
	defer s.Close()
	if p := s.Profile(); !p.Taught["ch01-01"] || p.Recommended != "ch02-01" || len(s.Notices()) != 0 {
		t.Fatalf("profile = %+v notices = %q", p, s.Notices())
	}
	// A profile saved without the new keys gets a usable map.
	writeFile(t, filepath.Join(data, "profile.json"), `{"Schema":2,"Taught":null}`)
	s.Close()
	s = open(t, cfg, data)
	defer s.Close()
	if s.Profile().Taught == nil {
		t.Fatal("Taught is nil")
	}
}

// Profiles and runs of v0.2 stored stars. Stars are graded from the runs now,
// so the old keys load without error, are not kept, and the runs stay whole.
func TestOldStarsKeysAreIgnoredOnLoad(t *testing.T) {
	cfg, data := dirs(t)
	writeFile(t, filepath.Join(data, "profile.json"), `{"Schema":2,"FirstRunDone":true,"Stars":{"ch01-01":3},"Recommended":"ch01-02"}`)
	writeFile(t, filepath.Join(data, "runs.jsonl"),
		`{"Schema":2,"ID":"a3f9c1e","At":"2026-10-30T12:00:00Z","Mode":"test","LessonID":"ch01-01","WPM":7,"Accuracy":0.98,"Score":6.5,"Stars":3}`+"\n")
	s := open(t, cfg, data)
	defer s.Close()
	if n := s.Notices(); len(n) != 0 {
		t.Fatalf("the old files were set aside: %q", n)
	}
	if p := s.Profile(); !p.FirstRunDone || p.Recommended != "ch01-02" {
		t.Errorf("profile = %+v", p)
	}
	runs, err := s.Runs()
	if err != nil || len(runs) != 1 || runs[0].ID != "a3f9c1e" || runs[0].WPM != 7 || runs[0].Accuracy != 0.98 {
		t.Fatalf("runs = %+v, %v", runs, err)
	}
	if err := s.SaveProfile(); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(data, "profile.json")); strings.Contains(got, `"Stars"`) {
		t.Errorf("saving kept the old stars: %s", got)
	}
}

func TestKeyboardShapeRoundTrips(t *testing.T) {
	cfg, data := dirs(t)
	s := open(t, cfg, data)
	if got := s.Config().KeyboardShape; got != "" {
		t.Fatalf("default shape %q, want empty", got)
	}
	want := Config{Layout: "x", KeyboardShape: "ansi", UpdateCheck: true}
	if err := s.SaveConfig(want); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if got := open(t, cfg, data).Config(); got != want {
		t.Fatalf("reloaded config = %+v", got)
	}
}
