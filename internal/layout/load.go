package layout

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/BurntSushi/toml"
)

//go:embed data/*.toml
var builtinFS embed.FS

type fileLayout struct {
	ID           string `toml:"id"`
	Name         string `toml:"name"`
	OS           string `toml:"os"`
	ModifierName string `toml:"modifier_name"`
	Rows         []struct {
		Keys []struct {
			ID     string `toml:"id"`
			Label  string `toml:"label"`
			Hand   string `toml:"hand"`
			Finger string `toml:"finger"`
		} `toml:"keys"`
	} `toml:"row"`
	Chars []struct {
		C          string   `toml:"c"`
		Route      []string `toml:"route"`
		Alternates []string `toml:"alternates"`
	} `toml:"char"`
}

var (
	hands   = map[string]Hand{"left": Left, "right": Right}
	fingers = map[string]Finger{"pinky": Pinky, "ring": Ring, "middle": Middle, "index": Index, "thumb": Thumb}
	mods    = map[string]bool{"shift": true, "option": true, "altgr": true}
)

// Builtin returns the layouts embedded in the binary.
func Builtin() ([]*Layout, error) {
	entries, err := builtinFS.ReadDir("data")
	if err != nil {
		return nil, err
	}
	var out []*Layout
	for _, e := range entries {
		data, err := builtinFS.ReadFile("data/" + e.Name())
		if err != nil {
			return nil, err
		}
		l, err := parse(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		out = append(out, l)
	}
	return out, nil
}

// FileError is a layout file that could not be read or parsed.
type FileError struct {
	Path string
	Err  error
}

func (e *FileError) Error() string { return e.Path + ": " + e.Err.Error() }

// Unwrap returns the underlying error.
func (e *FileError) Unwrap() error { return e.Err }

// LoadDir loads user layout files (*.toml) from dir. A bad file yields an error and is skipped.
// A missing directory yields no layouts and no errors.
func LoadDir(dir string) ([]*Layout, []error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.toml"))
	if err != nil {
		return nil, []error{err}
	}
	sort.Strings(paths)
	var layouts []*Layout
	var errs []error
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err == nil {
			var l *Layout
			if l, err = parse(data); err == nil {
				layouts = append(layouts, l)
				continue
			}
		}
		errs = append(errs, &FileError{Path: p, Err: err})
	}
	return layouts, errs
}

func parse(data []byte) (*Layout, error) {
	var f fileLayout
	if _, err := toml.Decode(string(data), &f); err != nil {
		return nil, err
	}
	if f.ID == "" || f.Name == "" {
		return nil, fmt.Errorf("layout needs id and name")
	}
	l := &Layout{
		ID: f.ID, Name: f.Name, OS: f.OS, ModifierName: f.ModifierName,
		keys: map[string]Key{}, chars: map[rune]Route{},
	}
	for _, row := range f.Rows {
		var keys []Key
		for _, k := range row.Keys {
			h, ok := hands[k.Hand]
			if !ok {
				return nil, fmt.Errorf("key %q: unknown hand %q", k.ID, k.Hand)
			}
			fi, ok := fingers[k.Finger]
			if !ok {
				return nil, fmt.Errorf("key %q: unknown finger %q", k.ID, k.Finger)
			}
			if _, dup := l.keys[k.ID]; dup || k.ID == "" {
				return nil, fmt.Errorf("key id %q is empty or duplicated", k.ID)
			}
			key := Key{ID: k.ID, Label: k.Label, Hand: h, Finger: fi}
			l.keys[k.ID] = key
			keys = append(keys, key)
		}
		l.Rows = append(l.Rows, keys)
	}
	for _, c := range f.Chars {
		if utf8.RuneCountInString(c.C) != 1 {
			return nil, fmt.Errorf("char %q: must be exactly one character", c.C)
		}
		r, _ := utf8.DecodeRuneInString(c.C)
		if _, dup := l.chars[r]; dup {
			return nil, fmt.Errorf("char %q is defined twice", c.C)
		}
		route, err := l.parseRoute(c.Route)
		if err != nil {
			return nil, fmt.Errorf("char %q: %w", c.C, err)
		}
		l.chars[r] = route
	}
	return l, nil
}

func (l *Layout) parseRoute(chords []string) (Route, error) {
	if len(chords) == 0 {
		return nil, fmt.Errorf("empty route")
	}
	route := make(Route, 0, len(chords))
	for _, s := range chords {
		parts := strings.Split(s, "+")
		key := parts[len(parts)-1]
		if _, ok := l.keys[key]; !ok {
			return nil, fmt.Errorf("route %q names unknown key %q", s, key)
		}
		for _, m := range parts[:len(parts)-1] {
			if !mods[m] {
				return nil, fmt.Errorf("route %q has unknown modifier %q", s, m)
			}
		}
		route = append(route, Chord{Mods: append([]string(nil), parts[:len(parts)-1]...), Key: key})
	}
	return route, nil
}
