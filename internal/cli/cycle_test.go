package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tulpenhaendler/StarEdit/internal/backup"
	"github.com/tulpenhaendler/StarEdit/internal/save"
	"github.com/tulpenhaendler/StarEdit/internal/save/savetest"
)

// setup builds a fake install: config, data dir with a synthetic save, and
// server scripts that append to a log so the test can check the call order.
func setup(t *testing.T) (dir string) {
	t.Helper()
	dir = t.TempDir()
	files := map[string]string{
		"config.json": `{"save":"data/Server/AutoSave0.sav","data_dir":"data",
			"players":{"alice":"76561198000000001"}}`,
		"data/SaveData.dat":         "index",
		"data/Server/AutoSave0.sav": string(savetest.Packed()),
		"stop_server.sh":            "#!/bin/sh\necho stop >> calls.log\n",
		"start_server.sh":           "#!/bin/sh\necho start >> calls.log\n",
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	running := serverRunning
	serverRunning = func() bool { return false }
	t.Cleanup(func() { serverRunning = running })
	return dir
}

func staredit(dir string, args ...string) error {
	return Run(append([]string{"-config", filepath.Join(dir, "config.json")}, args...))
}

func calls(t *testing.T, dir string) string {
	t.Helper()
	b, _ := os.ReadFile(filepath.Join(dir, "calls.log"))
	return strings.Join(strings.Fields(string(b)), " ")
}

func quartzOf(t *testing.T, dir string) int {
	t.Helper()
	s, err := save.Read(filepath.Join(dir, "data/Server/AutoSave0.sav"))
	if err != nil {
		t.Fatal(err)
	}
	n, err := s.Inventory("76561198000000001")
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, it := range n.Items {
		if strings.Contains(it.ItemData, "I_QuartzOre") {
			total += it.Amount
		}
	}
	return total
}

func TestGiveBackupRestoreCycle(t *testing.T) {
	dir := setup(t)

	if err := staredit(dir, "give", "alice", "quartz", "250"); err != nil {
		t.Fatal(err)
	}
	if got := calls(t, dir); got != "stop start" {
		t.Errorf("script calls = %q, want \"stop start\"", got)
	}
	if got := quartzOf(t, dir); got != 250 {
		t.Errorf("quartz after give = %d, want 250", got)
	}
	list, err := backup.List(filepath.Join(dir, "backups"))
	if err != nil || len(list) != 1 {
		t.Fatalf("backups = %v, %v; want exactly one", list, err)
	}
	if want := "before: give alice quartz 250"; list[0].Reason != want {
		t.Errorf("reason = %q, want %q", list[0].Reason, want)
	}
	if _, err := os.Stat(filepath.Join(list[0].Dir, "data", "SaveData.dat")); err != nil {
		t.Errorf("backup does not hold the whole data dir: %v", err)
	}

	// Restoring backup 1 brings back the pre-give state and is itself backed up.
	if err := staredit(dir, "restore", "1"); err != nil {
		t.Fatal(err)
	}
	if got := quartzOf(t, dir); got != 0 {
		t.Errorf("quartz after restore = %d, want 0", got)
	}
	if got := calls(t, dir); got != "stop start stop start" {
		t.Errorf("script calls = %q", got)
	}
	if list, _ := backup.List(filepath.Join(dir, "backups")); len(list) != 2 {
		t.Errorf("backups after restore = %d, want 2", len(list))
	}
}

// A give that cannot work must not take the server down at all.
func TestBadGiveLeavesServerAlone(t *testing.T) {
	dir := setup(t)
	for _, args := range [][]string{
		{"give", "nobody", "quartz", "5"},
		{"give", "alice", "quartz", "100000"}, // no room
		{"give", "alice", "quartz", "5", "-dry-run"},
	} {
		staredit(dir, args...)
	}
	if got := calls(t, dir); got != "" {
		t.Errorf("script calls = %q, want none", got)
	}
	if list, _ := backup.List(filepath.Join(dir, "backups")); len(list) != 0 {
		t.Errorf("backups = %d, want 0", len(list))
	}
}

func TestServerStillRunningAbortsButRestarts(t *testing.T) {
	dir := setup(t)
	serverRunning = func() bool { return true }
	if err := staredit(dir, "give", "alice", "quartz", "5"); err == nil {
		t.Fatal("expected an error while the server is still running")
	}
	if got := calls(t, dir); got != "stop start" {
		t.Errorf("script calls = %q, want \"stop start\"", got)
	}
	if got := quartzOf(t, dir); got != 0 {
		t.Errorf("save was edited (%d quartz) although the server was running", got)
	}
}

func TestMissingScriptsIsAnError(t *testing.T) {
	dir := setup(t)
	os.Remove(filepath.Join(dir, "start_server.sh"))
	if err := staredit(dir, "give", "alice", "quartz", "5"); err == nil {
		t.Fatal("expected an error without start_server.sh")
	}
	if got := calls(t, dir); got != "" {
		t.Errorf("server was stopped (%q) although it could not be started again", got)
	}
}
