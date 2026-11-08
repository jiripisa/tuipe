package layout_test

import (
	"errors"
	"fmt"
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

func TestSpaceBarLabelIsCzech(t *testing.T) {
	k, ok := mustBuiltin(t, "macos-czech-qwerty").Key("k49")
	if !ok || k.Label != "mezera" {
		t.Fatalf("space bar: %+v", k)
	}
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

func TestLoadDirErrorsNameTheirFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.toml")
	if err := os.WriteFile(path, []byte("[[["), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errs := layout.LoadDir(dir)
	var fe *layout.FileError
	if len(errs) != 1 || !errors.As(errs[0], &fe) || fe.Path != path || fe.Err == nil {
		t.Fatalf("errors = %v, want one FileError for %s", errs, path)
	}
}

// shapeFile is a small layout whose US-shaped keyboard moves k50 to k10's place.
const shapeFile = `id = "shape-test"
name = "Shape test"
os = "linux"
modifier_name = "AltGr"

[[row]]
keys = [
  { id = "k10", label = "<", hand = "left", finger = "pinky" },
  { id = "k50", label = "\\", hand = "left", finger = "pinky" },
  { id = "k3", label = "f", hand = "left", finger = "index" },
]

[[char]]
c = "<"
route = ["k10"]

[[char]]
c = "\\"
route = ["k50"]

[[char]]
c = "|"
route = ["shift+k50"]

[[char]]
c = "f"
route = ["k3"]

[ansi]
%s
`

func shapeLayout(t *testing.T, ansi string) (*layout.Layout, error) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "shape.toml"), []byte(fmt.Sprintf(shapeFile, ansi)), 0o644); err != nil {
		t.Fatal(err)
	}
	ls, errs := layout.LoadDir(dir)
	if len(errs) != 0 {
		return nil, errs[0]
	}
	return ls[0], nil
}

func mustShapeLayout(t *testing.T, ansi string) *layout.Layout {
	t.Helper()
	l, err := shapeLayout(t, ansi)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestANSIMovedKeyTakesOverTheLabelAndRoutes(t *testing.T) {
	l := mustShapeLayout(t, `moved = { k50 = "k10" }`)
	if l.WithShape(layout.ISO) != l {
		t.Error("ISO must be the same layout")
	}
	a := l.WithShape(layout.ANSI)
	if a == l || a.Shape() != layout.ANSI || l.Shape() != layout.ISO {
		t.Fatal("ANSI must be a copy in the ANSI shape, and the original must stay")
	}
	if a.ID != l.ID || a.Name != l.Name || a.OS != l.OS || a.ModifierName != l.ModifierName {
		t.Errorf("identity changed: %+v", a)
	}
	if _, ok := a.Route('<'); ok {
		t.Error("< must have no route on ANSI")
	}
	assertRoute(t, a, '\\', "k10")
	assertRoute(t, a, '|', "shift+k10")
	assertRoute(t, a, 'f', "k3")
	if k, _ := a.Key("k10"); k.Label != "\\" {
		t.Errorf("k10 label %q", k.Label)
	}
	if _, ok := a.Key("k50"); ok {
		t.Error("k50 must be gone")
	}
	for _, row := range a.Rows {
		for _, k := range row {
			if k.ID == "k50" {
				t.Error("k50 is still in a row")
			}
		}
	}
	// The ISO layout is untouched.
	assertRoute(t, l, '<', "k10")
	assertRoute(t, l, '\\', "k50")
	if k, _ := l.Key("k10"); k.Label != "<" {
		t.Errorf("ISO k10 label %q", k.Label)
	}
	if a.WithShape(layout.ISO) != l {
		t.Error("ISO of the ANSI copy must be the original")
	}
}

func TestANSIMissingKeyDropsItsCharacters(t *testing.T) {
	a := mustShapeLayout(t, `missing = ["k50"]`).WithShape(layout.ANSI)
	for _, r := range "\\|" {
		if _, ok := a.Route(r); ok {
			t.Errorf("%q must have no route on ANSI", r)
		}
	}
	assertRoute(t, a, '<', "k10")
}

func TestANSICharReplacesARoute(t *testing.T) {
	a := mustShapeLayout(t, `missing = ["k10"]

[[ansi.char]]
c = "<"
route = ["altgr+k3"]`).WithShape(layout.ANSI)
	assertRoute(t, a, '<', "altgr+k3")
}

func TestANSIRejectsUnknownKeys(t *testing.T) {
	for _, ansi := range []string{
		`missing = ["k99"]`,
		`moved = { k99 = "k10" }`,
		`moved = { k50 = "k99" }`,
		"[[ansi.char]]\nc = \"<\"\nroute = [\"k99\"]",
	} {
		if _, err := shapeLayout(t, ansi); err == nil {
			t.Errorf("%q: accepted", ansi)
		}
	}
}

func TestShapeWithoutOverlayStaysISO(t *testing.T) {
	l := mustBuiltin(t, "macos-czech-qwerty")
	if l.HasANSI() {
		t.Skip("the built-in layout has an overlay")
	}
	if l.WithShape(layout.ANSI) != l {
		t.Error("a layout without [ansi] must come back unchanged")
	}
	for in, want := range map[string]layout.Shape{"": layout.ISO, "iso": layout.ISO, "ansi": layout.ANSI, "x": layout.ISO} {
		if got := layout.ParseShape(in); got != want {
			t.Errorf("ParseShape(%q) = %q", in, got)
		}
	}
}

func TestOutputIsTheUnmodifiedKey(t *testing.T) {
	l := mustBuiltin(t, "macos-czech-qwerty")
	if r, ok := l.Output("k16"); !ok || r != 'y' {
		t.Errorf("k16: %q %v, want y", r, ok)
	}
	if r, ok := l.Output("k24"); ok {
		t.Errorf("k24 is a dead key, got %q", r)
	}
}

func TestANSIAppliedTwiceIsTheSameAsOnce(t *testing.T) {
	l := mustShapeLayout(t, `moved = { k50 = "k10" }`)
	once := l.WithShape(layout.ANSI)
	twice := once.WithShape(layout.ANSI)
	if k, _ := twice.Key("k10"); k.Label != "\\" {
		t.Errorf("k10 label %q", k.Label)
	}
	assertRoute(t, twice, '\\', "k10")
	assertRoute(t, twice, '|', "shift+k10")
	if _, ok := twice.Route('<'); ok {
		t.Error("< must have no route")
	}
	if _, ok := twice.Key("k50"); ok {
		t.Error("k50 must be gone")
	}
}
