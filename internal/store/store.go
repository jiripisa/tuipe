// Package store keeps tuipe's data on disk as plain files: a profile, an
// append-only run history and one file per ghost, plus the user's config. Files
// are written atomically, a corrupted file is set aside instead of crashing the
// app, and a lock keeps two instances from corrupting each other's data. It
// never reads the clock.
package store

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"

	"github.com/jiripisa/tuipe/internal/stats"
	"github.com/jiripisa/tuipe/internal/ui/text"
)

// SchemaVersion is the version written into every JSON file.
const SchemaVersion = 1

var (
	// ErrNewerData means a file was written by a newer tuipe and is left untouched.
	ErrNewerData = errors.New("data written by a newer tuipe")
	// ErrLocked means another tuipe holds the data directory.
	ErrLocked = errors.New("another tuipe is running")
)

// IncidentResult is the outcome of an incident round.
type IncidentResult struct {
	Severity string
	Won      bool
	SLA      float64
	MTTRMs   int64
}

// Run is one finished round in the history.
type Run struct {
	Schema                             int
	ID                                 string
	At                                 time.Time
	Mode, LessonID, TextHash, LayoutID string
	Session                            string // how the round was started: quick, standup, sprint, career or practice; empty in old runs
	WPM, Accuracy, Score               float64
	Stars                              int
	DurationMs                         int64
	Paused, HintUsed                   bool
	GhostDeltaMs                       *int64
	Incident                           *IncidentResult
}

// Ghost is the best recorded replay of one text.
type Ghost struct {
	Schema   int
	TextHash string
	Score    float64
	TimesMs  []int64
}

// SprintState is the saved state of an interrupted sprint. Seed and the
// snapshot taken at the start (Chars, Stars, Completed, IncidentBest) fix the
// plan, so a resumed sprint regenerates the same rounds. Chars is also what the
// retro compares the final statistics with.
type SprintState struct {
	Minutes      int
	ActiveMs     int64
	NextIndex    int
	Seed         int64
	Started      string // RFC 3339
	Chars        map[string]stats.Entry
	Stars        map[string]int
	Completed    map[string]bool
	IncidentBest map[string]string
	RunIDs       []string // the runs of this Sprint, as its rounds finish
}

// Resume is what to offer the user when they come back.
type Resume struct {
	Kind, LessonID string
	Sprint         *SprintState
	Day            string
}

// Profile is the mastery and progress of the user.
type Profile struct {
	Schema          int
	Stats           map[string]*stats.Stats // by layout id
	Stars           map[string]int          // by lesson id
	Completed       map[string]bool         // by chapter id
	IncidentBest    map[string]string       // by incident id: best severity won
	FirstRunDone    bool
	FocusTerminals  map[string]bool // by terminal key: reports focus changes
	StandupDay      string
	LastUpdateCheck string
	LatestSeen      string
	Resume          *Resume
}

func newProfile() *Profile {
	p := &Profile{Schema: SchemaVersion}
	p.fillMaps()
	return p
}

// fillMaps replaces nil maps, which JSON null or a missing key produces.
func (p *Profile) fillMaps() {
	if p.Stats == nil {
		p.Stats = map[string]*stats.Stats{}
	}
	for k, v := range p.Stats {
		if v == nil {
			delete(p.Stats, k)
		}
	}
	if p.Stars == nil {
		p.Stars = map[string]int{}
	}
	if p.Completed == nil {
		p.Completed = map[string]bool{}
	}
	if p.FocusTerminals == nil {
		p.FocusTerminals = map[string]bool{}
	}
	if p.IncidentBest == nil {
		p.IncidentBest = map[string]string{}
	}
}

// Store is an open data directory. It holds the single-instance lock until Close.
type Store struct {
	configDir, dataDir string
	lock               *flock.Flock
	config             Config
	profile            *Profile
	notices            []string
}

// Paths returns the config and data directories, honouring XDG_CONFIG_HOME and
// XDG_DATA_HOME on every platform.
func Paths() (configDir, dataDir string, err error) {
	cfgBase, dataBase := os.Getenv("XDG_CONFIG_HOME"), os.Getenv("XDG_DATA_HOME")
	if cfgBase == "" || dataBase == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", "", err
		}
		if cfgBase == "" {
			cfgBase = filepath.Join(home, ".config")
		}
		if dataBase == "" {
			dataBase = filepath.Join(home, ".local", "share")
		}
	}
	return filepath.Join(cfgBase, "tuipe"), filepath.Join(dataBase, "tuipe"), nil
}

// Open creates the directories, takes the lock and loads config and profile.
// It returns ErrLocked when another instance is running and ErrNewerData when
// the profile was written by a newer tuipe. Corrupted files are set aside and
// reported through Notices.
func Open(configDir, dataDir string) (*Store, error) {
	for _, d := range []string{configDir, dataDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, err
		}
	}
	lock := flock.New(filepath.Join(dataDir, "tuipe.lock"))
	ok, err := lock.TryLock()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrLocked
	}
	s := &Store{configDir: configDir, dataDir: dataDir, lock: lock}
	if err := s.load(); err != nil {
		_ = lock.Unlock()
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	if err := s.loadConfig(); err != nil {
		return err
	}
	return s.loadProfile()
}

// Close releases the lock.
func (s *Store) Close() error { return s.lock.Unlock() }

// Notices lists things the user should be told, such as a file set aside.
func (s *Store) Notices() []string { return append([]string(nil), s.notices...) }

func (s *Store) profilePath() string { return filepath.Join(s.dataDir, "profile.json") }

func (s *Store) loadProfile() error {
	var p Profile
	found, err := s.readJSON(s.profilePath(), "profile.json", &p)
	if err != nil {
		return err
	}
	if !found {
		s.profile = newProfile()
		return nil
	}
	p.fillMaps()
	s.profile = &p
	return nil
}

// Profile returns the live profile. Change it, then call SaveProfile.
func (s *Store) Profile() *Profile { return s.profile }

// SaveProfile writes the profile atomically.
func (s *Store) SaveProfile() error {
	s.profile.Schema = SchemaVersion
	return writeJSON(s.profilePath(), s.profile)
}

func (s *Store) runsPath() string { return filepath.Join(s.dataDir, "runs.jsonl") }

// AppendRun adds one line to the run history.
func (s *Store) AppendRun(r Run) error {
	r.Schema = SchemaVersion
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(s.runsPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	// A torn last line has no newline; start a fresh line so this run survives.
	line := append(b, '\n')
	if tornTail(s.runsPath()) {
		line = append([]byte{'\n'}, line...)
	}
	// One Write call keeps the line whole even if another process appends.
	if _, err := f.Write(line); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// tornTail reports whether the file is non-empty and does not end in a newline.
func tornTail(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || fi.Size() == 0 {
		return false
	}
	var last [1]byte
	if _, err := f.ReadAt(last[:], fi.Size()-1); err != nil {
		return false
	}
	return last[0] != '\n'
}

// Runs returns the run history in file order. Lines that do not parse are skipped.
func (s *Store) Runs() ([]Run, error) {
	f, err := os.Open(s.runsPath())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var runs []Run
	br := bufio.NewReader(f)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			var r Run
			if json.Unmarshal(line, &r) == nil && r.ID != "" {
				runs = append(runs, r)
			}
		}
		if errors.Is(err, io.EOF) {
			return runs, nil
		}
		if err != nil {
			return runs, err
		}
	}
}

func (s *Store) ghostPath(hash string) (string, error) {
	if hash == "" || hash != filepath.Base(hash) || hash == "." || hash == ".." {
		return "", fmt.Errorf("invalid ghost hash %q", hash)
	}
	return filepath.Join(s.dataDir, "ghosts", hash+".json"), nil
}

// Ghost returns the ghost for a text hash, or nil, nil when there is none. A
// corrupted file is set aside and treated as absent.
func (s *Store) Ghost(hash string) (*Ghost, error) {
	path, err := s.ghostPath(hash)
	if err != nil {
		return nil, err
	}
	var g Ghost
	found, err := s.readJSON(path, "ghosts/"+hash+".json", &g)
	if err != nil || !found {
		return nil, err
	}
	return &g, nil
}

// SaveGhost writes a ghost atomically.
func (s *Store) SaveGhost(g Ghost) error {
	path, err := s.ghostPath(g.TextHash)
	if err != nil {
		return err
	}
	if b, err := os.ReadFile(path); err == nil {
		var head struct{ Schema int }
		if json.Unmarshal(b, &head) == nil && head.Schema > SchemaVersion {
			return fmt.Errorf("ghosts/%s.json: %w", g.TextHash, ErrNewerData)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	g.Schema = SchemaVersion
	return writeJSON(path, g)
}

// readJSON decodes the file at path into v. It reports found=false for a missing
// file and for a corrupted one, which it sets aside. A Schema newer than
// SchemaVersion gives ErrNewerData and leaves the file alone.
func (s *Store) readJSON(path, name string, v any) (found bool, err error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var head struct{ Schema int }
	if json.Unmarshal(b, &head) != nil {
		return false, s.setAside(path, name)
	}
	if head.Schema > SchemaVersion {
		return false, fmt.Errorf("%s: %w", name, ErrNewerData)
	}
	if json.Unmarshal(b, v) != nil {
		return false, s.setAside(path, name)
	}
	return true, nil
}

// setAside renames a corrupted file to <path>.corrupt-N, with the first free N,
// and records a notice.
func (s *Store) setAside(path, name string) error {
	for n := 1; ; n++ {
		target := fmt.Sprintf("%s.corrupt-%d", path, n)
		if _, err := os.Lstat(target); err == nil {
			continue
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if err := os.Rename(path, target); err != nil {
			return err
		}
		s.notices = append(s.notices, fmt.Sprintf(text.StoreCorruptSetAside, name, filepath.Base(target)))
		return nil
	}
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, b)
}

// writeAtomic writes data to a temp file in the same directory, syncs it and
// renames it over path, so a failure leaves the old file intact.
func writeAtomic(path string, data []byte) (err error) {
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = f.Close()
			_ = os.Remove(f.Name())
		}
	}()
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
