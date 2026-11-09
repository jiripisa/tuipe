// Command layoutdump prints what a system's own keyboard layout types, key by
// key, as the JSON dump that layoutgen reads (see internal/layout/gen). It only
// reads the system's layout definition; it changes no setting.
//
//	go run -tags xkb ./internal/layout/cmd/layoutdump linux cz         # on Linux, with libxkbcommon
//	go run -tags xkb ./internal/layout/cmd/layoutdump linux cz qwerty
//	go run ./internal/layout/cmd/layoutdump windows 00000405           # on Windows
//	go run ./internal/layout/cmd/layoutdump windows 00010405
//
// Each system's dumper lives in its own file, built only where that system's
// libraries are. The Windows one calls user32.dll without cgo; the Linux one
// needs cgo and the xkb tag, so the default build uses no cgo anywhere.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/jiripisa/tuipe/internal/layout/gen"
)

// dumper reads one layout of a system; args are what follows the system's name.
type dumper struct {
	usage string // the arguments, as "<layout> [<variant>]"
	min   int    // how many arguments it needs at least
	max   int    // and at most
	dump  func(args []string) (gen.Dump, error)
}

// dumpers are the systems this build can dump; a system's file adds itself.
var dumpers = map[string]dumper{}

// systems are every system there is a dumper for, and where it builds.
var systems = map[string]string{
	"linux":   "on Linux, built with -tags xkb and libxkbcommon",
	"windows": "on Windows",
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: layoutdump <system> <layout>...; systems:", strings.Join(sortedKeys(systems), ", "))
		return 2
	}
	system, rest := args[0], args[1:]
	d, ok := dumpers[system]
	if !ok {
		if where, known := systems[system]; known {
			fmt.Fprintf(stderr, "layoutdump: this build cannot dump %s layouts; run it %s\n", system, where)
		} else {
			fmt.Fprintf(stderr, "layoutdump: unknown system %q; systems: %s\n", system, strings.Join(sortedKeys(systems), ", "))
		}
		return 2
	}
	if len(rest) < d.min || len(rest) > d.max {
		fmt.Fprintf(stderr, "usage: layoutdump %s %s\n", system, d.usage)
		return 2
	}
	dump, err := d.dump(rest)
	if err != nil {
		fmt.Fprintln(stderr, "layoutdump:", err)
		return 1
	}
	if err := writeDump(stdout, dump); err != nil {
		fmt.Fprintln(stderr, "layoutdump:", err)
		return 1
	}
	return 0
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// levels are the levels of a dump, from the easiest to type.
var levels = []string{"", "shift", "alt", "shift+alt"}

// chordName joins a level and a key id as a dump writes a chord.
func chordName(level, id string) string {
	if level == "" {
		return id
	}
	return level + "+" + id
}

// writeDump writes d as the macOS dumper does: keys in key-code order, then
// the dead keys in that order and by level, each with its followers in the same
// order, so that a dump made again can be compared with git diff.
func writeDump(w io.Writer, d gen.Dump) error {
	var b strings.Builder
	fmt.Fprintf(&b, "{\n  \"source\": %s,\n  \"system\": %s,\n  \"keys\": {\n", quote(d.Source), quote(d.System))
	var lines []string
	for _, p := range positions {
		ls, ok := d.Keys[p.ID]
		if !ok {
			continue
		}
		var fields []string
		for _, level := range levels {
			fields = append(fields, quote(level)+": "+quote(ls[level]))
		}
		lines = append(lines, fmt.Sprintf("    %s: { %s }", quote(p.ID), strings.Join(fields, ", ")))
	}
	b.WriteString(strings.Join(lines, ",\n"))
	b.WriteString("\n  },\n  \"dead\": {\n")
	var blocks []string
	written := 0
	for _, p := range positions {
		for _, level := range levels {
			name := chordName(level, p.ID)
			followers, ok := d.Dead[name]
			if !ok {
				continue
			}
			written++
			var fl []string
			for _, f := range positions {
				for _, flevel := range levels[:2] {
					fname := chordName(flevel, f.ID)
					if out, ok := followers[fname]; ok {
						fl = append(fl, fmt.Sprintf("      %s: %s", quote(fname), quote(out)))
					}
				}
			}
			if len(fl) != len(followers) {
				return fmt.Errorf("a follower of the dead key %s is not a key, or a key with Shift", name)
			}
			blocks = append(blocks, fmt.Sprintf("    %s: {\n%s\n    }", quote(name), strings.Join(fl, ",\n")))
		}
	}
	if written != len(d.Dead) {
		return errors.New("a dead key is not a chord of a known key")
	}
	b.WriteString(strings.Join(blocks, ",\n"))
	b.WriteString("\n  }\n}\n")
	_, err := io.WriteString(w, b.String())
	return err
}

// quote writes s as a JSON string, escaping as the macOS dumper does.
func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20:
			fmt.Fprintf(&b, `\u%04X`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
