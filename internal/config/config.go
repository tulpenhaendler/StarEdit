// Package config loads the optional config.json.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Config is the optional config.json: where the server's data lives, how to
// stop and start the server, and friendly names for the SteamID64s that key
// players in the save. Relative paths are relative to the config file.
type Config struct {
	Save        string            `json:"save"`         // the .sav file to edit
	DataDir     string            `json:"data_dir"`     // backed up before every change; default: the save's folder
	BackupDir   string            `json:"backup_dir"`   // default: backups
	StopScript  string            `json:"stop_script"`  // default: stop_server.sh
	StartScript string            `json:"start_script"` // default: start_server.sh
	Players     map[string]string `json:"players"`      // name -> SteamID64

	dir string // folder of the loaded config file, or "." without one
}

// Path resolves a path from the config (or fallback when it is empty)
// against the config file's folder.
func (c *Config) Path(p, fallback string) string {
	if p == "" {
		p = fallback
	}
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(c.dir, p)
}

// Load reads the file at path. With an empty path it looks for
// $STAREDIT_CONFIG, ./config.json, then config.json beside the executable,
// and a missing file is not an error.
func Load(path string) (*Config, error) {
	candidates := []string{path}
	if path == "" {
		candidates = []string{os.Getenv("STAREDIT_CONFIG"), "config.json"}
		if exe, err := os.Executable(); err == nil {
			candidates = append(candidates, filepath.Join(filepath.Dir(exe), "config.json"))
		}
	}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		data, err := os.ReadFile(c)
		if errors.Is(err, fs.ErrNotExist) && path == "" {
			continue
		}
		if err != nil {
			return nil, err
		}
		cfg := &Config{dir: filepath.Dir(c)}
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("%s: %w", c, err)
		}
		return cfg, nil
	}
	return &Config{dir: "."}, nil
}

// SteamID returns the SteamID64 configured for a player name, if any.
func (c *Config) SteamID(name string) (string, bool) {
	for n, id := range c.Players {
		if strings.EqualFold(n, name) {
			return id, true
		}
	}
	return "", false
}

// Label renders a SteamID64 with its configured name when there is one.
func (c *Config) Label(id string) string {
	for n, pid := range c.Players {
		if pid == id {
			return fmt.Sprintf("%s (%s)", n, id)
		}
	}
	return id
}
