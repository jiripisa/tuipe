package gen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Dump is what a system says its layout types, key by key.
//
// A chord in Dead is a level and a key id joined with "+", as "shift+k24" or
// "shift+alt+k27"; the level "" is the bare key id, as "k24". A key that is a
// dead key at some level has "" at that level in Keys and its results in Dead.
// Keys that are not positions of the ISO keyboard are ignored.
type Dump struct {
	Source string                       `json:"source"` // e.g. "macOS AppleKeyboardLayouts: com.apple.keylayout.Czech"
	System string                       `json:"system"` // darwin | windows | linux
	Keys   map[string]Levels            `json:"keys"`   // by key id
	Dead   map[string]map[string]string `json:"dead"`   // dead chord ("shift+k24") -> follower chord ("k2", "shift+k8", "k49") -> result
}

// Levels maps "", "shift", "alt", "shift+alt" to the output; "" when nothing.
type Levels map[string]string

// levelRank orders the levels from the easiest to type.
var levelRank = map[string]int{"": 0, "shift": 1, "alt": 2, "shift+alt": 3}

// ReadDump reads a dump written as JSON. Unknown fields are an error, so a
// misspelt field in a dumper does not silently drop data.
func ReadDump(path string) (Dump, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Dump{}, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var d Dump
	if err := dec.Decode(&d); err != nil {
		return Dump{}, fmt.Errorf("%s: %w", path, err)
	}
	return d, nil
}

// chord is one key press of a dump: a level and a key id.
type chord struct{ level, key string }

// parseChord reads a dump chord. The modifiers may come in any order.
func parseChord(s string) (chord, error) {
	parts := strings.Split(s, "+")
	key := parts[len(parts)-1]
	if key == "" {
		return chord{}, fmt.Errorf("chord %q has no key", s)
	}
	var shift, alt bool
	for _, m := range parts[:len(parts)-1] {
		switch {
		case m == "shift" && !shift:
			shift = true
		case m == "alt" && !alt:
			alt = true
		default:
			return chord{}, fmt.Errorf("chord %q has an unknown or repeated modifier %q", s, m)
		}
	}
	level := ""
	switch {
	case shift && alt:
		level = "shift+alt"
	case shift:
		level = "shift"
	case alt:
		level = "alt"
	}
	return chord{level: level, key: key}, nil
}
