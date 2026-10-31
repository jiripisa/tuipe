// Package content holds the text embedded in the binary: word lists, Czech
// commit messages, code snippets, incident scripts and the curriculum.
package content

import (
	"embed"
	"fmt"
	"strings"
)

//go:embed data/*.txt
var dataFS embed.FS

// Snippet is a piece of code to type.
type Snippet struct{ ID, Lang, Text string }

// Step is one prompt and the text typed in answer to it.
type Step struct{ Prompt, Text, Lang string }

// Incident is a scripted on-call scenario.
type Incident struct {
	ID, Title, Chapter string
	Steps              []Step
}

// LessonDef describes one lesson of a chapter.
type LessonDef struct {
	ID, Title, Focus string
	Seed             int64
	Length           int
	Speed            [3]float64
	Snippets         []string
}

// ChapterDef describes one chapter of the curriculum. A test-out is generated
// text like a lesson, so it is identified by its seed.
type ChapterDef struct {
	ID, Title, Rank, Introduces, IncidentID string
	TestOutSeed                             int64
	Number                                  int
	Lessons                                 []LessonDef
}

// Library is all embedded typing material.
type Library struct {
	WordsCS, WordsEN, CommitsCS []string
	Snippets                    []Snippet
	Incidents                   []Incident
	Chapters                    []ChapterDef
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
	return lib, nil
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
