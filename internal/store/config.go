package store

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config is the user's settings, kept in config.toml.
type Config struct {
	Style       string `toml:"style"`
	Layout      string `toml:"layout"`
	UpdateCheck bool   `toml:"update_check"`
}

func defaultConfig() Config { return Config{UpdateCheck: true} }

func (s *Store) configPath() string { return filepath.Join(s.configDir, "config.toml") }

// loadConfig reads config.toml. A missing file gives the defaults; a corrupted
// one is set aside and also gives the defaults.
func (s *Store) loadConfig() error {
	path := s.configPath()
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		s.config = defaultConfig()
		return nil
	}
	if err != nil {
		return err
	}
	c := defaultConfig()
	if err := toml.Unmarshal(b, &c); err != nil {
		s.config = defaultConfig()
		return s.setAside(path, "config.toml")
	}
	s.config = c
	return nil
}

// Config returns the current settings.
func (s *Store) Config() Config { return s.config }

// SaveConfig writes the settings atomically and makes them current.
func (s *Store) SaveConfig(c Config) error {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(c); err != nil {
		return err
	}
	if err := writeAtomic(s.configPath(), buf.Bytes()); err != nil {
		return err
	}
	s.config = c
	return nil
}
