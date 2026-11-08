// Command layoutgen writes the built-in layout files in internal/layout/data
// from the system dumps listed in internal/layout/source/layouts.toml. Run it
// from the root of the repository:
//
//	go run ./internal/layout/cmd/layoutgen         # write the files that changed
//	go run ./internal/layout/cmd/layoutgen -check  # exit 1 and name the stale files
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/jiripisa/tuipe/internal/layout/gen"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("layoutgen", flag.ContinueOnError)
	flags.SetOutput(stderr)
	check := flags.Bool("check", false, "write nothing; exit 1 and name the files that are stale")
	dir := flags.String("dir", filepath.Join("internal", "layout"), "the folder that holds source/ and data/")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: layoutgen [-check] [-dir internal/layout]")
		return 2
	}
	files, err := gen.Build(*dir)
	if err != nil {
		fmt.Fprintln(stderr, "layoutgen:", err)
		return 1
	}
	stale, err := gen.Stale(files)
	if err != nil {
		fmt.Fprintln(stderr, "layoutgen:", err)
		return 1
	}
	if *check {
		for _, path := range stale {
			fmt.Fprintln(stderr, "stale:", path)
		}
		if len(stale) > 0 {
			fmt.Fprintln(stderr, "Run go run ./internal/layout/cmd/layoutgen and commit the result.")
			return 1
		}
		return 0
	}
	for _, f := range files {
		if !slices.Contains(stale, f.Path) {
			continue
		}
		if err := os.WriteFile(f.Path, f.Data, 0o644); err != nil {
			fmt.Fprintln(stderr, "layoutgen:", err)
			return 1
		}
		fmt.Fprintln(stdout, "wrote", f.Path)
	}
	return 0
}
