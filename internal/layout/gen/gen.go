// Package gen writes layout files from dumps of a system's own keyboard layout,
// so that no built-in layout is written by hand.
package gen

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/BurntSushi/toml"

	"github.com/jiripisa/tuipe/internal/layout"
)

// Meta is one [[layout]] of source/layouts.toml.
type Meta struct {
	ID           string `toml:"id"`
	Name         string `toml:"name"`
	OS           string `toml:"os"`
	ModifierName string `toml:"modifier_name"`
	Source       string `toml:"source"` // the dump, a file name in source/
}

// Position is one physical key of the ISO keyboard and the finger that presses it.
type Position struct{ ID, Hand, Finger string }

//go:embed positions.toml
var positionsTOML string

var loadPositions = sync.OnceValues(func() ([][]Position, error) {
	var f struct {
		Rows []struct {
			Keys []struct{ ID, Hand, Finger string } `toml:"keys"`
		} `toml:"row"`
	}
	if _, err := toml.Decode(positionsTOML, &f); err != nil {
		return nil, fmt.Errorf("positions.toml: %w", err)
	}
	var rows [][]Position
	for _, r := range f.Rows {
		var row []Position
		for _, k := range r.Keys {
			row = append(row, Position(k))
		}
		rows = append(rows, row)
	}
	return rows, nil
})

// Positions returns the rows of the ISO keyboard, top row first, as every
// generated layout has them.
func Positions() [][]Position {
	rows, err := loadPositions()
	if err != nil {
		panic(err) // the file is embedded, and a test loads it
	}
	out := make([][]Position, len(rows))
	for i, row := range rows {
		out[i] = slices.Clone(row)
	}
	return out
}

// chars is every character a generated layout routes, in the order of the file.
var chars = func() []rune {
	var out []rune
	for r := 'a'; r <= 'z'; r++ {
		out = append(out, r, r-'a'+'A')
	}
	out = append(out, []rune("áÁčČďĎéÉěĚíÍňŇóÓřŘšŠťŤúÚůŮýÝžŽ")...)
	out = append(out, []rune("0123456789")...)
	for r := rune(0x21); r <= 0x7E; r++ {
		if !('a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || '0' <= r && r <= '9') {
			out = append(out, r)
		}
	}
	return append(out, []rune("§°ˇ¨ \n")...)
}()

// route is a candidate way to type a character: chords of the dump.
type route []chord

// keyIndex is a key's place in position order.
type keyIndex map[string]int

// less reports whether a is the better route of the two: fewer chords, then
// easier modifiers chord by chord, then not on k10 or k50, whose place differs
// between keyboard shapes, then position order chord by chord.
func (idx keyIndex) less(a, b route) bool {
	if len(a) != len(b) {
		return len(a) < len(b)
	}
	for i := range a {
		if ra, rb := levelRank[a[i].level], levelRank[b[i].level]; ra != rb {
			return ra < rb
		}
	}
	if ea, eb := a.usesEdge(), b.usesEdge(); ea != eb {
		return !ea
	}
	for i := range a {
		if pa, pb := idx[a[i].key], idx[b[i].key]; pa != pb {
			return pa < pb
		}
	}
	return false
}

func (r route) usesEdge() bool {
	return r.uses("k10") || r.uses("k50")
}

func (r route) uses(key string) bool {
	return slices.ContainsFunc(r, func(c chord) bool { return c.key == key })
}

// chords renders the route as the chords of a layout file.
func (r route) chords(alt string) []string {
	out := make([]string, len(r))
	for i, c := range r {
		var mods []string
		if strings.Contains(c.level, "shift") {
			mods = append(mods, "shift")
		}
		if strings.Contains(c.level, "alt") {
			mods = append(mods, alt)
		}
		out[i] = strings.Join(append(mods, c.key), "+")
	}
	return out
}

// normalRoute writes a layout route with the modifiers of each chord in the
// order the generator writes them, so a kept route matches however it was written.
func normalRoute(s string) string {
	chords := strings.Fields(s)
	for i, c := range chords {
		parts := strings.Split(c, "+")
		mods := parts[:len(parts)-1] // sorted in place, inside parts
		slices.SortFunc(mods, func(a, b string) int { return modOrder(a) - modOrder(b) })
		chords[i] = strings.Join(parts, "+")
	}
	return strings.Join(chords, " ")
}

func modOrder(m string) int {
	if m == "shift" {
		return 0
	}
	return 1
}

// candidates collects every route of the dump that types a character of chars.
func candidates(d Dump, idx keyIndex) (map[rune][]route, error) {
	wanted := map[rune]bool{}
	for _, r := range chars {
		wanted[r] = true
	}
	out := map[rune][]route{}
	add := func(s string, rt route) {
		r, size := utf8.DecodeRuneInString(s)
		if size > 0 && size == len(s) && r != utf8.RuneError && wanted[r] {
			out[r] = append(out[r], rt)
		}
	}
	dead := map[chord]bool{}
	for s := range d.Dead {
		c, err := parseChord(s)
		if err != nil {
			return nil, fmt.Errorf("dead key: %w", err)
		}
		dead[c] = true
	}
	for id, levels := range d.Keys {
		for level, s := range levels {
			if _, ok := levelRank[level]; !ok {
				return nil, fmt.Errorf("key %s: unknown level %q", id, level)
			}
			c := chord{level: level, key: id}
			if _, ok := idx[id]; ok && !dead[c] {
				add(s, route{c})
			}
		}
	}
	for s, followers := range d.Dead {
		dc, _ := parseChord(s)
		for f, result := range followers {
			fc, err := parseChord(f)
			if err != nil {
				return nil, fmt.Errorf("dead key %s: %w", s, err)
			}
			_, okDead := idx[dc.key]
			_, okFollower := idx[fc.key]
			if okDead && okFollower {
				add(result, route{dc, fc})
			}
		}
	}
	out['\n'] = []route{{{key: "k36"}}} // Enter types a newline, whatever a system reports for it
	return out, nil
}

// best returns the preferred route among rs, and false when there is none.
func (idx keyIndex) best(rs []route) (route, bool) {
	if len(rs) == 0 {
		return nil, false
	}
	return slices.MinFunc(rs, func(a, b route) int {
		switch {
		case idx.less(a, b):
			return -1
		case idx.less(b, a):
			return 1
		}
		return 0
	}), true
}

// ansi is how the US-shaped keyboard differs on a system: on macOS the key
// left of 1 types what the key right of the left Shift typed on ISO, elsewhere
// that key is just missing.
type ansi struct {
	darwin bool
}

// apply returns the route as it is on the US-shaped keyboard, and false when
// the overlay drops it. It follows the rules of the loader.
func (a ansi) apply(r route) (route, bool) {
	if a.darwin {
		if r.uses("k10") {
			return nil, false
		}
		out := slices.Clone(r)
		for i := range out {
			if out[i].key == "k50" {
				out[i].key = "k10"
			}
		}
		return out, true
	}
	if r.uses("k50") {
		return nil, false
	}
	return r, true
}

// Generate writes the layout file for a dump. keep maps a character to the
// route it should keep, as a layout file writes it ("shift+k24 k2"), when the
// dump still types it that way.
func Generate(d Dump, m Meta, keep map[rune]string) ([]byte, error) {
	if m.ID == "" || m.Name == "" {
		return nil, fmt.Errorf("layout needs id and name")
	}
	rows, err := loadPositions()
	if err != nil {
		return nil, err
	}
	idx := keyIndex{}
	for _, row := range rows {
		for _, p := range row {
			idx[p.ID] = len(idx)
		}
	}
	cands, err := candidates(d, idx)
	if err != nil {
		return nil, err
	}
	alt := "altgr"
	if m.OS == "darwin" {
		alt = "option"
	}
	shape := ansi{darwin: m.OS == "darwin"}

	var b bytes.Buffer
	fmt.Fprintf(&b, "# Generated by layoutgen from internal/layout/source/%s. Do not edit.\n", m.Source)
	if d.Source != "" {
		fmt.Fprintf(&b, "# The system's definition: %s.\n", d.Source)
	}
	fmt.Fprintf(&b, "\nid = %s\nname = %s\n", quote(m.ID), quote(m.Name))
	if m.OS != "" {
		fmt.Fprintf(&b, "os = %s\n", quote(m.OS))
	}
	if m.ModifierName != "" {
		fmt.Fprintf(&b, "modifier_name = %s\n", quote(m.ModifierName))
	}
	for _, row := range rows {
		b.WriteString("\n[[row]]\nkeys = [\n")
		for _, p := range row {
			fmt.Fprintf(&b, "  { id = %s, label = %s, hand = %s, finger = %s },\n",
				quote(p.ID), quote(label(d, p.ID)), quote(p.Hand), quote(p.Finger))
		}
		b.WriteString("]\n")
	}

	var dropped []rune
	for _, r := range chars {
		rt, ok := idx.best(cands[r])
		if want, kept := keep[r]; kept {
			for _, c := range cands[r] {
				if strings.Join(c.chords(alt), " ") == normalRoute(want) {
					rt, ok = c, true
					break
				}
			}
		}
		if !ok {
			continue
		}
		writeChar(&b, "char", r, rt.chords(alt))
		if _, survives := shape.apply(rt); !survives {
			dropped = append(dropped, r)
		}
	}

	if shape.darwin {
		b.WriteString("\n[ansi]\nmoved = { k50 = \"k10\" }\n")
	} else {
		b.WriteString("\n[ansi]\nmissing = [\"k50\"]\n")
	}
	for _, r := range dropped {
		var others []route
		for _, c := range cands[r] {
			if us, ok := shape.apply(c); ok {
				others = append(others, us)
			}
		}
		if rt, ok := idx.best(others); ok {
			writeChar(&b, "ansi.char", r, rt.chords(alt))
		}
	}
	return b.Bytes(), nil
}

// label is what the keyboard map shows on a key.
func label(d Dump, id string) string {
	switch id {
	case "k49":
		return "mezera"
	case "k36":
		return "↵"
	}
	if results, dead := d.Dead[id]; dead {
		for _, f := range []string{"k0", "shift+k0"} {
			if accent, ok := accents[results[f]]; ok {
				return accent
			}
		}
		return results["k49"]
	}
	return d.Keys[id][""]
}

// accents maps an a with an accent to the accent on its own. A dead key is
// labelled with the accent it adds to a, as the key cap shows it, because what
// it types before space can be another character: macOS types ' for the acute.
var accents = map[string]string{
	"á": "´", "ǎ": "ˇ", "ä": "¨", "å": "°", "à": "`", "â": "^", "ã": "~",
	"Á": "´", "Ǎ": "ˇ", "Ä": "¨", "Å": "°", "À": "`", "Â": "^", "Ã": "~",
}

func writeChar(b *bytes.Buffer, table string, r rune, chords []string) {
	quoted := make([]string, len(chords))
	for i, c := range chords {
		quoted[i] = quote(c)
	}
	fmt.Fprintf(b, "\n[[%s]]\nc = %s\nroute = [%s]\n", table, quote(string(r)), strings.Join(quoted, ", "))
}

// quote writes s as a TOML basic string.
func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7F:
			fmt.Fprintf(&b, `\u%04X`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// KeepFrom reads the routes of an existing layout file, to pass to Generate as keep.
func KeepFrom(data []byte) (map[rune]string, error) {
	l, err := layout.Parse(data)
	if err != nil {
		return nil, err
	}
	keep := map[rune]string{}
	for _, r := range chars {
		if rt, ok := l.Route(r); ok {
			keep[r] = rt.String()
		}
	}
	return keep, nil
}

// File is one generated layout file.
type File struct {
	Path string
	Data []byte
}

// Build generates every layout listed in dir/source/layouts.toml, where dir is
// the internal/layout folder. Each layout keeps the routes of the file in
// dir/data that it replaces. Nothing is written.
func Build(dir string) ([]File, error) {
	manifest := filepath.Join(dir, "source", "layouts.toml")
	var f struct {
		Layouts []Meta `toml:"layout"`
	}
	md, err := toml.DecodeFile(manifest, &f)
	if err != nil {
		return nil, err
	}
	if extra := md.Undecoded(); len(extra) > 0 {
		return nil, fmt.Errorf("%s: unknown fields %v", manifest, extra)
	}
	var files []File
	seen := map[string]bool{}
	for _, m := range f.Layouts {
		if m.ID == "" || m.Source == "" || seen[m.ID] {
			return nil, fmt.Errorf("%s: layout %q needs a unique id and a source", manifest, m.ID)
		}
		seen[m.ID] = true
		d, err := ReadDump(filepath.Join(dir, "source", m.Source))
		if err != nil {
			return nil, err
		}
		path := filepath.Join(dir, "data", m.ID+".toml")
		var keep map[rune]string
		old, err := os.ReadFile(path)
		switch {
		case err == nil:
			if keep, err = KeepFrom(old); err != nil {
				return nil, fmt.Errorf("%s: %w", path, err)
			}
		case !errors.Is(err, fs.ErrNotExist):
			return nil, err
		}
		data, err := Generate(d, m, keep)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", m.Source, err)
		}
		files = append(files, File{Path: path, Data: data})
	}
	return files, nil
}

// Stale returns the paths of the files that are missing or differ on disk.
func Stale(files []File) ([]string, error) {
	var stale []string
	for _, f := range files {
		old, err := os.ReadFile(f.Path)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		if err != nil || !bytes.Equal(old, f.Data) {
			stale = append(stale, f.Path)
		}
	}
	return stale, nil
}
