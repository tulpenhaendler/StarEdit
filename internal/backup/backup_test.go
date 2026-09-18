package backup

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestCreateListRestore(t *testing.T) {
	root := filepath.Join(t.TempDir(), "backups")
	data := filepath.Join(t.TempDir(), "SaveGames")
	write(t, filepath.Join(data, "SaveData.dat"), "index")
	write(t, filepath.Join(data, "Server", "AutoSave0.sav"), "v1")

	first, err := Create(root, data, "before give")
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(data, "Server", "AutoSave0.sav"), "v2")
	write(t, filepath.Join(data, "Server", "stray.tmp"), "junk")
	second, err := Create(root, data, "manual") // same second: must not collide
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID {
		t.Fatalf("both backups got ID %s", first.ID)
	}

	list, err := List(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != second.ID || list[1].ID != first.ID {
		t.Fatalf("List = %v, want newest first [%s %s]", list, second.ID, first.ID)
	}
	if list[1].Reason != "before give" || list[1].DataDir != data {
		t.Errorf("info not kept: %+v", list[1])
	}
	if files, size := list[1].Size(); files != 2 || size != int64(len("index")+len("v1")) {
		t.Errorf("Size = %d files, %d bytes", files, size)
	}

	if err := list[1].Restore(data); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(data, "Server", "AutoSave0.sav")); got != "v1" {
		t.Errorf("restored save = %q, want v1", got)
	}
	if _, err := os.Stat(filepath.Join(data, "Server", "stray.tmp")); !os.IsNotExist(err) {
		t.Error("restore kept a file that was not in the backup")
	}
	leftovers, _ := filepath.Glob(data + ".staredit-*")
	if len(leftovers) != 0 {
		t.Errorf("restore left %v behind", leftovers)
	}
}

func TestListIgnoresIncompleteBackups(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "2026-01", "02", "03-04-05", "data", "AutoSave0.sav"), "x")
	list, err := List(root)
	if err != nil || len(list) != 0 {
		t.Fatalf("List = %v, %v; want empty", list, err)
	}
}
