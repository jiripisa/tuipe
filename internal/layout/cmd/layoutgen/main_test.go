package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tree makes an internal/layout folder with one layout in its manifest and no
// generated file yet.
func tree(t *testing.T) (dir, data string) {
	t.Helper()
	dir = t.TempDir()
	for _, sub := range []string{"source", "data"} {
		if err := os.Mkdir(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"source/layouts.toml": "[[layout]]\nid = \"t\"\nname = \"T\"\nos = \"linux\"\nmodifier_name = \"AltGr\"\nsource = \"t.json\"\n",
		"source/t.json":       `{"system": "linux", "keys": {"k0": {"": "a", "shift": "A"}}}`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir, filepath.Join(dir, "data", "t.toml")
}

func TestCheckNamesStaleFilesAndExitsOne(t *testing.T) {
	dir, data := tree(t)
	var out, errOut bytes.Buffer
	if code := run([]string{"-check", "-dir", dir}, &out, &errOut); code != 1 {
		t.Errorf("exit %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), data) {
		t.Errorf("stderr %q does not name %s", errOut.String(), data)
	}
	if _, err := os.Stat(data); !os.IsNotExist(err) {
		t.Errorf("-check wrote %s", data)
	}
}

func TestWriteThenCheckPasses(t *testing.T) {
	dir, data := tree(t)
	var out, errOut bytes.Buffer
	if code := run([]string{"-dir", dir}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), data) {
		t.Errorf("stdout %q does not name %s", out.String(), data)
	}
	got, err := os.ReadFile(data)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "c = \"A\"\nroute = [\"shift+k0\"]\n") {
		t.Errorf("unexpected file:\n%s", got)
	}
	out.Reset()
	if code := run([]string{"-check", "-dir", dir}, &out, &errOut); code != 0 {
		t.Errorf("check after write: exit %d: %s", code, errOut.String())
	}
}

func TestEmptyManifestIsCurrent(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "source"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "source", "layouts.toml"), []byte("# nothing yet\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := run([]string{"-check", "-dir", dir}, &out, &errOut); code != 0 {
		t.Errorf("exit %d: %s", code, errOut.String())
	}
}

func TestArgumentsAreRejected(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"extra"}, &out, &errOut); code != 2 {
		t.Errorf("exit %d, want 2", code)
	}
}
