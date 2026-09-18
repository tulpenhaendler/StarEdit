package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tulpenhaendler/StarEdit/internal/config"
	"github.com/tulpenhaendler/StarEdit/internal/save"
	"github.com/tulpenhaendler/StarEdit/internal/save/savetest"
)

func TestFindPlayer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "AutoSave0.sav")
	if err := os.WriteFile(path, savetest.Packed(), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := save.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Players: map[string]string{"alice": "76561198000000001", "carol": "76561198000000003"}}
	for query, want := range map[string]string{
		"alice":             "76561198000000001",
		"Alice":             "76561198000000001",
		"76561198000000002": "76561198000000002",
		"0002":              "76561198000000002",
	} {
		if got, err := findPlayer(cfg, s, query); err != nil || got != want {
			t.Errorf("findPlayer(%q) = %q, %v; want %s", query, got, err, want)
		}
	}
	for _, query := range []string{"carol", "7656", "nobody"} { // not in save, ambiguous, unknown
		if got, err := findPlayer(cfg, s, query); err == nil {
			t.Errorf("findPlayer(%q) = %q, want an error", query, got)
		}
	}
}
