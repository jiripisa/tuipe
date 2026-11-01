package store

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jiripisa/tuipe/internal/ui/text"
)

// seeded writes what a played tuipe leaves on disk, plus a file of someone else.
func seeded(t *testing.T) (cfg, data string) {
	t.Helper()
	cfg, data = dirs(t)
	files := []string{
		filepath.Join(cfg, "config.toml"),
		filepath.Join(cfg, "config.toml.corrupt-1"),
		filepath.Join(cfg, "layouts", "mine.toml"),
		filepath.Join(cfg, "notes.txt"),
		filepath.Join(data, "profile.json"),
		filepath.Join(data, "profile.json.corrupt-1"),
		filepath.Join(data, "runs.jsonl"),
		filepath.Join(data, "ghosts", "abc.json"),
		filepath.Join(data, "notes.txt"),
	}
	for _, f := range files {
		if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return cfg, data
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func wantFiles(t *testing.T, want bool, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if exists(p) != want {
			t.Errorf("%s: exists = %v, want %v", p, !want, want)
		}
	}
}

func settingsFiles(cfg string) []string {
	return []string{
		filepath.Join(cfg, "config.toml"),
		filepath.Join(cfg, "config.toml.corrupt-1"),
		filepath.Join(cfg, "layouts"),
	}
}

func scoreFiles(data string) []string {
	return []string{
		filepath.Join(data, "profile.json"),
		filepath.Join(data, "profile.json.corrupt-1"),
		filepath.Join(data, "runs.jsonl"),
		filepath.Join(data, "ghosts"),
	}
}

func reset(t *testing.T, cfg, data string, scope ResetScope, answer string) string {
	t.Helper()
	var out bytes.Buffer
	if err := Reset(cfg, data, scope, strings.NewReader(answer), &out); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	return out.String()
}

func TestResetScoreKeepsSettings(t *testing.T) {
	cfg, data := seeded(t)
	reset(t, cfg, data, ResetScope{Score: true}, "ano\n")
	wantFiles(t, false, scoreFiles(data)...)
	wantFiles(t, true, settingsFiles(cfg)...)
}

func TestResetSettingsKeepsScore(t *testing.T) {
	cfg, data := seeded(t)
	reset(t, cfg, data, ResetScope{Settings: true}, "ano\n")
	wantFiles(t, false, settingsFiles(cfg)...)
	wantFiles(t, true, scoreFiles(data)...)
}

func TestResetBothGivesAFreshStart(t *testing.T) {
	cfg, data := dirs(t)
	s := open(t, cfg, data)
	s.Profile().FirstRunDone = true
	if err := s.SaveProfile(); err != nil {
		t.Fatal(err)
	}
	c := s.Config()
	c.Style = "velin"
	if err := s.SaveConfig(c); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	reset(t, cfg, data, ResetScope{Settings: true, Score: true}, "ano\n")

	fresh := open(t, cfg, data)
	defer fresh.Close()
	if fresh.Profile().FirstRunDone {
		t.Error("the profile survived")
	}
	if fresh.Config() != defaultConfig() {
		t.Errorf("config = %+v", fresh.Config())
	}
	if len(fresh.Notices()) != 0 {
		t.Errorf("notices = %v", fresh.Notices())
	}
}

func TestResetDeletesNothingWithoutAYes(t *testing.T) {
	for _, answer := range []string{"", "\n", "ne\n", "a\n", "yes\n", "ano ano\n", "n"} {
		cfg, data := seeded(t)
		out := reset(t, cfg, data, ResetScope{Settings: true, Score: true}, answer)
		wantFiles(t, true, settingsFiles(cfg)...)
		wantFiles(t, true, scoreFiles(data)...)
		if !strings.Contains(out, text.ResetCancelled) {
			t.Errorf("answer %q: output %q", answer, out)
		}
	}
}

func TestResetTakesTheYesAsTyped(t *testing.T) {
	for _, answer := range []string{"ano\n", "ANO\n", " Ano \r\n", "ano"} {
		cfg, data := seeded(t)
		reset(t, cfg, data, ResetScope{Score: true}, answer)
		wantFiles(t, false, scoreFiles(data)...)
	}
}

func TestResetNamesEveryFileBeforeAsking(t *testing.T) {
	cfg, data := seeded(t)
	out := reset(t, cfg, data, ResetScope{Settings: true, Score: true}, "ne\n")
	asked := strings.Index(out, text.ResetQuestion)
	if asked < 0 {
		t.Fatalf("no question in %q", out)
	}
	for _, p := range append(settingsFiles(cfg), scoreFiles(data)...) {
		if i := strings.Index(out, p); i < 0 || i > asked {
			t.Errorf("%s is not named before the question:\n%s", p, out)
		}
	}
}

func TestResetLeavesOtherFilesAlone(t *testing.T) {
	cfg, data := seeded(t)
	out := reset(t, cfg, data, ResetScope{Settings: true, Score: true}, "ano\n")
	wantFiles(t, true, filepath.Join(cfg, "notes.txt"), filepath.Join(data, "notes.txt"))
	if strings.Contains(out, "notes.txt") {
		t.Errorf("a foreign file is listed:\n%s", out)
	}
}

func TestResetWithOneFolderForBothKeepsTheOtherKind(t *testing.T) {
	cfg, data := seeded(t)
	shared := filepath.Join(filepath.Dir(cfg), "shared")
	if err := os.Rename(cfg, shared); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"profile.json", "runs.jsonl", "ghosts"} {
		if err := os.Rename(filepath.Join(data, name), filepath.Join(shared, name)); err != nil {
			t.Fatal(err)
		}
	}
	reset(t, shared, shared, ResetScope{Settings: true}, "ano\n")
	wantFiles(t, false, filepath.Join(shared, "config.toml"), filepath.Join(shared, "layouts"))
	wantFiles(t, true, filepath.Join(shared, "profile.json"), filepath.Join(shared, "runs.jsonl"), filepath.Join(shared, "ghosts"))
}

func TestResetWithNothingToDeleteAsksNothing(t *testing.T) {
	cfg, data := dirs(t)
	out := reset(t, cfg, data, ResetScope{Settings: true, Score: true}, "ano\n")
	if !strings.Contains(out, text.ResetNothing) || strings.Contains(out, text.ResetQuestion) {
		t.Errorf("output %q", out)
	}
	wantFiles(t, false, cfg, data) // looking creates nothing
}

func TestResetRefusesWhileTuipeRuns(t *testing.T) {
	cfg, data := seeded(t)
	s := open(t, cfg, data)
	defer s.Close()
	var out bytes.Buffer
	err := Reset(cfg, data, ResetScope{Score: true}, strings.NewReader("ano\n"), &out)
	if err == nil || err.Error() != text.SecondInstance {
		t.Fatalf("err = %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("it asked anyway: %q", out.String())
	}
	wantFiles(t, true, filepath.Join(data, "runs.jsonl"))
}

func TestResetFreesTheLockForTheNextStart(t *testing.T) {
	cfg, data := seeded(t)
	reset(t, cfg, data, ResetScope{Score: true}, "ano\n")
	s, err := Open(cfg, data)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
}

func TestResetTellsInCzechWhatItCouldNotDelete(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod does not make a directory read-only on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	cfg, data := seeded(t)
	if err := os.Chmod(cfg, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(cfg, 0o755)
	var out bytes.Buffer
	err := Reset(cfg, data, ResetScope{Settings: true, Score: true}, strings.NewReader("ano\n"), &out)
	if err == nil || !strings.HasPrefix(err.Error(), text.ResetFailedPrefix) || !strings.Contains(err.Error(), "config.toml") {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "permission") {
		t.Errorf("English in %q", err)
	}
	wantFiles(t, false, scoreFiles(data)...) // the rest still goes
}
