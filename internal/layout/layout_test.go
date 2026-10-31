package layout_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jiripisa/tuipe/internal/layout"
)

func mustBuiltin(t *testing.T, id string) *layout.Layout {
	t.Helper()
	all, err := layout.Builtin()
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range all {
		if l.ID == id {
			return l
		}
	}
	t.Fatalf("no builtin layout %q", id)
	return nil
}

func assertRoute(t *testing.T, l *layout.Layout, r rune, want string) {
	t.Helper()
	got, ok := l.Route(r)
	if !ok {
		t.Errorf("%q: no route", r)
		return
	}
	if got.String() != want {
		t.Errorf("%q: route %q, want %q", r, got.String(), want)
	}
}

func assertFinger(t *testing.T, l *layout.Layout, id string, h layout.Hand, f layout.Finger) {
	t.Helper()
	k, ok := l.Key(id)
	if !ok {
		t.Fatalf("no key %s", id)
	}
	if k.Hand != h || k.Finger != f {
		t.Errorf("%s: hand %v finger %v, want %v %v", id, k.Hand, k.Finger, h, f)
	}
}

func TestCzechRoutes(t *testing.T) {
	l := mustBuiltin(t, "macos-czech-qwerty")
	cases := map[rune]string{
		'{': "option+k25", '}': "option+k29", '@': "option+k19", '#': "option+k20",
		'$': "option+k21", '~': "option+k23", '^': "option+k22", '&': "option+k26",
		'*': "option+k28", '[': "option+k33", ']': "option+k30", ';': "option+k41",
		'\'': "option+k39", '<': "option+k43", '>': "option+k47", '`': "shift+k42",
		'\\': "k50", '|': "shift+k50", '/': "shift+k33", '(': "shift+k30", ')': "k30",
		'"': "shift+k41", '!': "shift+k39", '§': "k39", '+': "k18", '=': "k27",
		'%': "shift+k27", '1': "shift+k18", 'ě': "k19", 'ž': "k22", 'ď': "shift+k24 k2",
		'ó': "k24 k31", 'Č': "shift+k24 shift+k8", 'Á': "k24 shift+k0",
		'°': "option+k27 k49", 'ˇ': "shift+k24 k49", '¨': "k42 k49", '\n': "k36", ' ': "k49",
	}
	for r, want := range cases {
		assertRoute(t, l, r, want)
	}
}

func TestOwnerCharacterListResolves(t *testing.T) {
	l := mustBuiltin(t, "macos-czech-qwerty")
	const list = `><+@#$~^&*{}°=%'ˇ/[()]";'§!` + "`" + `¨-_.:,?\|`
	if u := l.Unresolved(list); len(u) != 0 {
		t.Fatalf("unresolved: %q", u)
	}
}

func TestEveryPrintableAndCzechCharacterResolves(t *testing.T) {
	l := mustBuiltin(t, "macos-czech-qwerty")
	all := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789" +
		"áčďéěíňóřšťúůýžÁČĎÉĚÍŇÓŘŠŤÚŮÝŽ"
	if u := l.Unresolved(all); len(u) != 0 {
		t.Fatalf("unresolved: %q", u)
	}
}

func TestEveryRouteKeyHasAFinger(t *testing.T) {
	l := mustBuiltin(t, "macos-czech-qwerty")
	for _, r := range "abcxyzÁĎ{°\n \\" {
		route, _ := l.Route(r)
		for _, c := range route {
			if _, ok := l.Key(c.Key); !ok {
				t.Errorf("%q: unknown key %s", r, c.Key)
			}
		}
	}
}

func TestFingers(t *testing.T) {
	l := mustBuiltin(t, "macos-czech-qwerty")
	assertFinger(t, l, "k25", layout.Right, layout.Ring)  // í
	assertFinger(t, l, "k22", layout.Left, layout.Index)  // ž
	assertFinger(t, l, "k3", layout.Left, layout.Index)   // f
	assertFinger(t, l, "k38", layout.Right, layout.Index) // j
	assertFinger(t, l, "k49", layout.Right, layout.Thumb) // space
}

func TestLoadDirSkipsBrokenFile(t *testing.T) {
	dir := t.TempDir()
	good := `id = "g"
name = "Good"
os = "linux"
modifier_name = "AltGr"
[[row]]
keys = [{ id = "a", label = "a", hand = "left", finger = "pinky" }]
[[char]]
c = "a"
route = ["a"]
`
	bad := good + "[[char]]\nc = \"b\"\nroute = [\"nokey\"]\n"
	for name, body := range map[string]string{"good.toml": good, "bad.toml": bad} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	layouts, errs := layout.LoadDir(dir)
	if len(layouts) != 1 || len(errs) != 1 {
		t.Fatalf("got %d layouts, %d errors; want 1 and 1", len(layouts), len(errs))
	}
	if layouts[0].ID != "g" {
		t.Errorf("loaded %q", layouts[0].ID)
	}
}

func TestLoadDirMissingDirIsEmpty(t *testing.T) {
	layouts, errs := layout.LoadDir(filepath.Join(t.TempDir(), "absent"))
	if len(layouts) != 0 || len(errs) != 0 {
		t.Fatalf("got %d layouts, %d errors", len(layouts), len(errs))
	}
}
