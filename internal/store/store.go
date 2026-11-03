// Package store keeps tuipe's data on disk as plain files: a profile, an
// append-only run history, plus the user's config. Files
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
const SchemaVersion = 2

var (
	// ErrNewerData means a file was written by a newer tuipe and is left untouched.
	ErrNewerData = errors.New("data written by a newer tuipe")
	// ErrLocked means another tuipe holds the data directory.
	ErrLocked = errors.New("another tuipe is running")
)

// The JSON tags keep the keys of the files as they are written today, whatever
// a field is called in Go later. Changing one drops the user's data: add a
// migration instead. testdata/profile-v2.json guards them.

// The values of Run.Mode.
const (
	ModeTest      = "test"      // a Test of a lesson
	ModeTraining  = "training"  // a Trénink
	ModePlacement = "placement" // the placement test of the first run
)

// Run is one finished round in the history. Older runs also hold the key Stars,
// which is ignored: a Test is graded from its speed and accuracy.
type Run struct {
	Schema     int       `json:"Schema"`
	ID         string    `json:"ID"`
	At         time.Time `json:"At"`
	Mode       string    `json:"Mode"` // ModeTest, ModeTraining or ModePlacement
	LessonID   string    `json:"LessonID"`
	TextHash   string    `json:"TextHash"`
	LayoutID   string    `json:"LayoutID"`
	WPM        float64   `json:"WPM"`
	Accuracy   float64   `json:"Accuracy"`
	Score      float64   `json:"Score"`
	Length     string    `json:"Length"` // the training length, empty for other parts
	DurationMs int64     `json:"DurationMs"`
	Paused     bool      `json:"Paused"`
	HintUsed   bool      `json:"HintUsed"`
}

// Profile is the mastery and progress of the user. Older profiles also hold the
// key Stars, which is ignored on load and dropped on save: stars are graded from
// the runs.
type Profile struct {
	Schema          int                     `json:"Schema"`
	Stats           map[string]*stats.Stats `json:"Stats"` // by layout id
	FirstRunDone    bool                    `json:"FirstRunDone"`
	FocusTerminals  map[string]bool         `json:"FocusTerminals"` // by terminal key: reports focus changes
	LastUpdateCheck string                  `json:"LastUpdateCheck"`
	LatestSeen      string                  `json:"LatestSeen"`
	SkipNoted       map[string]bool         `json:"SkipNoted"`   // by layout id: told about the content it cannot type
	Recommended     string                  `json:"Recommended"` // lesson id the placement test recommends
	Taught          map[string]bool         `json:"Taught"`      // by lesson id: Výuka finished
	Pending         []string                `json:"Pending"`     // notices the lesson list has not shown yet
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
	if p.FocusTerminals == nil {
		p.FocusTerminals = map[string]bool{}
	}
	if p.SkipNoted == nil {
		p.SkipNoted = map[string]bool{}
	}
	if p.Taught == nil {
		p.Taught = map[string]bool{}
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
// the profile or the run history was written by a newer tuipe. Corrupted files
// and files of an older schema are set aside and reported through Notices.
func Open(configDir, dataDir string) (*Store, error) {
	for _, d := range []string{configDir, dataDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, fmt.Errorf(text.StoreNoDir, d)
		}
	}
	lock, err := takeLock(dataDir)
	if err != nil {
		return nil, err
	}
	s := &Store{configDir: configDir, dataDir: dataDir, lock: lock}
	if err := s.load(); err != nil {
		_ = lock.Unlock()
		return nil, err
	}
	return s, nil
}

// takeLock takes the single-instance lock of dataDir, or returns ErrLocked.
func takeLock(dataDir string) (*flock.Flock, error) {
	lock := flock.New(filepath.Join(dataDir, "tuipe.lock"))
	ok, err := lock.TryLock()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrLocked
	}
	return lock, nil
}

// load reads the files. The schema of the run history is read first, so that a
// history from a newer tuipe stops the opening before any file is touched.
func (s *Store) load() error {
	runsSchema, found, err := s.runsSchema()
	if err != nil {
		return err
	}
	if found && runsSchema > SchemaVersion {
		return fmt.Errorf("%s: %w", runsFile, ErrNewerData)
	}
	if err := s.loadConfig(); err != nil {
		return err
	}
	if err := s.loadProfile(); err != nil {
		return err
	}
	if found && runsSchema < SchemaVersion {
		return s.setAsideOld(s.runsPath(), runsFile)
	}
	return nil
}

// Close releases the lock.
func (s *Store) Close() error { return s.lock.Unlock() }

// Notices lists things the user should be told, such as a file set aside.
func (s *Store) Notices() []string { return append([]string(nil), s.notices...) }

const (
	profileFile = "profile.json"
	runsFile    = "runs.jsonl"
	ghostsDir   = "ghosts" // no longer written; kept so a reset deletes the old folder
)

func (s *Store) profilePath() string { return filepath.Join(s.dataDir, profileFile) }

func (s *Store) loadProfile() error {
	var p Profile
	found, err := s.readJSON(s.profilePath(), profileFile, &p)
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

func (s *Store) runsPath() string { return filepath.Join(s.dataDir, runsFile) }

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
	if head.Schema < SchemaVersion {
		return false, s.setAsideOld(path, name)
	}
	if json.Unmarshal(b, v) != nil {
		return false, s.setAside(path, name)
	}
	return true, nil
}

// runsSchema is the schema of the run history. The first valid line decides,
// so a mix with an old line first counts as old; a missing file or one with no
// valid line reports found=false. It reads only up to that line.
func (s *Store) runsSchema() (schema int, found bool, err error) {
	f, err := os.Open(s.runsPath())
	if errors.Is(err, fs.ErrNotExist) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	defer f.Close()
	br := bufio.NewReader(f)
	for {
		line, rerr := br.ReadBytes('\n')
		var r Run
		if len(line) > 0 && json.Unmarshal(line, &r) == nil && r.ID != "" {
			return r.Schema, true, nil
		}
		if rerr != nil {
			return 0, false, nil
		}
	}
}

// setAside renames a corrupted file to <path>.corrupt-N, with the first free N,
// and records a notice.
func (s *Store) setAside(path, name string) error {
	target, err := moveAside(path, func(n int) string { return fmt.Sprintf("%s.corrupt-%d", path, n) })
	if err != nil {
		return err
	}
	if name == profileFile {
		s.notices = append(s.notices, fmt.Sprintf(text.StoreProfileSetAside, target))
	} else {
		s.notices = append(s.notices, fmt.Sprintf(text.StoreCorruptSetAside, name, target))
	}
	return nil
}

// setAsideOld renames a file of an older schema to <path>.v1, or <path>.v1-N
// when an earlier one is already there, and records a notice.
func (s *Store) setAsideOld(path, name string) error {
	target, err := moveAside(path, func(n int) string {
		if n == 1 {
			return path + ".v1"
		}
		return fmt.Sprintf("%s.v1-%d", path, n)
	})
	if err != nil {
		return err
	}
	s.notices = append(s.notices, fmt.Sprintf(text.OldDataSetAside, name, target))
	return nil
}

// moveAside renames path to the first of target(1), target(2), ... that does
// not exist yet, so nothing set aside earlier is overwritten.
func moveAside(path string, target func(n int) string) (string, error) {
	for n := 1; ; n++ {
		t := target(n)
		if _, err := os.Lstat(t); err == nil {
			continue
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		if err := os.Rename(path, t); err != nil {
			return "", err
		}
		return t, nil
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

// ConfigDir is the folder of config.toml and of the user's layout files.
func (s *Store) ConfigDir() string { return s.configDir }
