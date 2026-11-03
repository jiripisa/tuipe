// Package content holds the text embedded in the binary: word lists, Czech
// commit messages, code snippets and the course.
package content

import (
	"embed"
	"fmt"
	"path"
	"strings"

	"github.com/BurntSushi/toml"
)

//go:embed data/*.txt data/*.toml data/snippets/*/*.txt
var dataFS embed.FS

// Snippet is a piece of code to type.
type Snippet struct{ ID, Lang, Text string }

// CourseLesson is one lesson of the course. A lesson with Focus drills those
// characters, a Review one practises earlier characters only, and one with
// Lang types real code of that language.
type CourseLesson struct {
	ID     string     `toml:"id"`
	Title  string     `toml:"title"`
	Focus  string     `toml:"focus"`
	Lang   string     `toml:"lang"`
	Intro  string     `toml:"intro"`
	Review bool       `toml:"review"`
	Speed  [3]float64 `toml:"speed"` // WPM for one, two and three stars
}

// CourseChapter is a group of course lessons.
type CourseChapter struct {
	ID      string         `toml:"id"`
	Title   string         `toml:"title"`
	Lessons []CourseLesson `toml:"lesson"`
}

// Library is all embedded typing material.
type Library struct {
	WordsCS, WordsEN, CommitsCS []string
	Snippets                    []Snippet
	Course                      []CourseChapter
}

// Load reads the embedded material.
func Load() (*Library, error) {
	lib := &Library{}
	for _, f := range []struct {
		name string
		dst  *[]string
	}{
		{"words-cs.txt", &lib.WordsCS},
		{"words-en.txt", &lib.WordsEN},
		{"commits-cs.txt", &lib.CommitsCS},
	} {
		data, err := dataFS.ReadFile("data/" + f.name)
		if err != nil {
			return nil, fmt.Errorf("content: %w", err)
		}
		*f.dst = lines(string(data))
	}
	var err error
	if lib.Course, err = loadCourse(); err != nil {
		return nil, err
	}
	if lib.Snippets, err = loadSnippets(); err != nil {
		return nil, err
	}
	return lib, nil
}

// loadSnippets reads data/snippets/<lang>/<id>.txt. The language is the
// directory and the id is the file name without its extension. The one newline
// that ends a file is dropped, so a round does not end on an extra Enter.
func loadSnippets() ([]Snippet, error) {
	dirs, err := dataFS.ReadDir("data/snippets")
	if err != nil {
		return nil, fmt.Errorf("content: %w", err)
	}
	var out []Snippet
	for _, dir := range dirs {
		files, err := dataFS.ReadDir(path.Join("data/snippets", dir.Name()))
		if err != nil {
			return nil, fmt.Errorf("content: %w", err)
		}
		for _, f := range files {
			data, err := dataFS.ReadFile(path.Join("data/snippets", dir.Name(), f.Name()))
			if err != nil {
				return nil, fmt.Errorf("content: %w", err)
			}
			out = append(out, Snippet{
				ID:   strings.TrimSuffix(f.Name(), path.Ext(f.Name())),
				Lang: dir.Name(),
				Text: strings.TrimSuffix(string(data), "\n"),
			})
		}
	}
	return out, nil
}

func loadCourse() ([]CourseChapter, error) {
	data, err := dataFS.ReadFile("data/course.toml")
	if err != nil {
		return nil, fmt.Errorf("content: %w", err)
	}
	var f struct {
		Chapter []CourseChapter `toml:"chapter"`
	}
	md, err := toml.Decode(string(data), &f)
	if err != nil {
		return nil, fmt.Errorf("content: course.toml: %w", err)
	}
	if u := md.Undecoded(); len(u) > 0 {
		return nil, fmt.Errorf("content: course.toml: unknown keys %v", u)
	}
	return f.Chapter, nil
}

func lines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}
