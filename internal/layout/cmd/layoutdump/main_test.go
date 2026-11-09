package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jiripisa/tuipe/internal/layout/gen"
)

func TestUsageErrorsExitTwo(t *testing.T) {
	for _, args := range [][]string{nil, {"amiga", "cz"}} {
		var out, errOut bytes.Buffer
		if code := run(args, &out, &errOut); code != 2 || out.Len() != 0 || errOut.Len() == 0 {
			t.Errorf("%q: exit %d, stdout %q, stderr %q", args, code, out.String(), errOut.String())
		}
	}
}

// TestASystemThisBuildCannotDumpSaysWhereItCan checks the default build, which
// has no dumper for Linux, as on a Mac.
func TestASystemThisBuildCannotDumpSaysWhereItCan(t *testing.T) {
	if _, ok := dumpers["linux"]; ok {
		t.Skip("this build dumps Linux layouts")
	}
	var out, errOut bytes.Buffer
	if code := run([]string{"linux", "cz"}, &out, &errOut); code != 2 {
		t.Errorf("exit %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "-tags xkb") {
		t.Errorf("stderr %q does not say how to build it", errOut.String())
	}
}

func TestWrittenDumpReadsBack(t *testing.T) {
	d := gen.Dump{
		Source: "test \"cz\"",
		System: "linux",
		Keys: map[string]gen.Levels{
			"k10": {"": ";", "shift": "°", "alt": "`", "shift+alt": "~"},
			"k2":  {"": "d", "shift": "D", "alt": "đ", "shift+alt": "Đ"},
			"k24": {"": "", "shift": "", "alt": "", "shift+alt": ""},
			"k36": {"": "\r", "shift": "\r", "alt": "\r", "shift+alt": "\r"},
		},
		Dead: map[string]map[string]string{
			"shift+k24": {"k2": "ď", "shift+k2": "Ď", "k49": "ˇ"},
			"k24":       {"k2": "", "k49": "'"},
		},
	}
	var b bytes.Buffer
	if err := writeDump(&b, d); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "d.json")
	if err := os.WriteFile(path, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := gen.ReadDump(path)
	if err != nil {
		t.Fatalf("%v\n%s", err, b.String())
	}
	if !reflect.DeepEqual(got, d) {
		t.Errorf("read back %+v, want %+v", got, d)
	}
	s := b.String()
	if !(strings.Index(s, `"k2"`) < strings.Index(s, `"k10"`) &&
		strings.Index(s, `"k24": {`) < strings.Index(s, `"shift+k24": {`)) {
		t.Errorf("not in key-code order:\n%s", s)
	}
}

func TestWritingADeadKeyOfNoKeyFails(t *testing.T) {
	d := gen.Dump{Dead: map[string]map[string]string{"k99": {"k0": "x"}}}
	if err := writeDump(&bytes.Buffer{}, d); err == nil {
		t.Error("a dead key k99 was written")
	}
}
