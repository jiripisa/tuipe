// Package content holds the text embedded in the binary: word lists, Czech
// commit messages, code snippets, incident scripts and the curriculum.
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

// Step is one prompt and the text typed in answer to it.
type Step struct{ Prompt, Text, Lang string }

// Incident is a scripted on-call scenario.
type Incident struct {
	ID, Title, Chapter string
	Steps              []Step
}

// LessonDef describes one lesson of a chapter. A lesson either drills Focus
// with generated text of at least Length characters, or types the Snippets.
type LessonDef struct {
	ID       string     `toml:"id"`
	Title    string     `toml:"title"`
	Focus    string     `toml:"focus"`
	Seed     int64      `toml:"seed"`
	Length   int        `toml:"length"`
	Speed    [3]float64 `toml:"speed"` // WPM for one, two and three stars
	Snippets []string   `toml:"snippets"`
}

// ChapterDef describes one chapter of the curriculum. A test-out is generated
// text like a lesson, so it is identified by its seed. Allowed lists characters
// the chapter may use without introducing them (only the onboarding chapter
// needs it).
type ChapterDef struct {
	ID          string      `toml:"id"`
	Number      int         `toml:"number"`
	Title       string      `toml:"title"`
	Rank        string      `toml:"rank"`
	Introduces  string      `toml:"introduces"`
	Allowed     string      `toml:"allowed"`
	IncidentID  string      `toml:"incident"`
	TestOutSeed int64       `toml:"test_out_seed"`
	Lessons     []LessonDef `toml:"lesson"`
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
	chapters, err := loadChapters()
	if err != nil {
		return nil, err
	}
	lib.Chapters = chapters
	if lib.Snippets, err = loadSnippets(); err != nil {
		return nil, err
	}
	if lib.Incidents, err = loadIncidents(); err != nil {
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

func loadIncidents() ([]Incident, error) {
	data, err := dataFS.ReadFile("data/incidents.toml")
	if err != nil {
		return nil, fmt.Errorf("content: %w", err)
	}
	type step struct {
		Prompt string `toml:"prompt"`
		Text   string `toml:"text"`
		Lang   string `toml:"lang"`
	}
	var f struct {
		Incident []struct {
			ID      string `toml:"id"`
			Title   string `toml:"title"`
			Chapter string `toml:"chapter"`
			Step    []step `toml:"step"`
		} `toml:"incident"`
	}
	md, err := toml.Decode(string(data), &f)
	if err != nil {
		return nil, fmt.Errorf("content: incidents.toml: %w", err)
	}
	if u := md.Undecoded(); len(u) > 0 {
		return nil, fmt.Errorf("content: incidents.toml: unknown keys %v", u)
	}
	out := make([]Incident, 0, len(f.Incident))
	for _, in := range f.Incident {
		inc := Incident{ID: in.ID, Title: in.Title, Chapter: in.Chapter}
		for _, s := range in.Step {
			inc.Steps = append(inc.Steps, Step(s))
		}
		out = append(out, inc)
	}
	return out, nil
}

func loadChapters() ([]ChapterDef, error) {
	data, err := dataFS.ReadFile("data/curriculum.toml")
	if err != nil {
		return nil, fmt.Errorf("content: %w", err)
	}
	var f struct {
		Chapter []ChapterDef `toml:"chapter"`
	}
	md, err := toml.Decode(string(data), &f)
	if err != nil {
		return nil, fmt.Errorf("content: curriculum.toml: %w", err)
	}
	if u := md.Undecoded(); len(u) > 0 {
		return nil, fmt.Errorf("content: curriculum.toml: unknown keys %v", u)
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
