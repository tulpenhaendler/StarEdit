// Package backup keeps timestamped copies of the server's data directory.
//
// Layout: <root>/<YYYY-MM>/<DD>/<HH-MM-SS>/{info.json, data/...}
package backup

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"time"
)

const (
	infoFile = "info.json"
	dataDir  = "data"
)

type Backup struct {
	ID      string    `json:"-"` // path below the root, e.g. "2026-09/18/14-05-32"
	Dir     string    `json:"-"`
	Time    time.Time `json:"time"`
	Reason  string    `json:"reason"`
	DataDir string    `json:"data_dir"` // where the data was copied from
}

// Create copies src into a new backup under root.
func Create(root, src, reason string) (*Backup, error) {
	if st, err := os.Stat(src); err != nil {
		return nil, err
	} else if !st.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", src)
	}
	abs, err := filepath.Abs(src)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	b := &Backup{Time: now.Truncate(time.Second), Reason: reason, DataDir: abs}

	day := filepath.Join(now.Format("2006-01"), now.Format("02"))
	if err := os.MkdirAll(filepath.Join(root, day), 0o755); err != nil {
		return nil, err
	}
	// Two backups in the same second get a numeric suffix.
	for n := 1; ; n++ {
		name := now.Format("15-04-05")
		if n > 1 {
			name = fmt.Sprintf("%s-%d", name, n)
		}
		b.ID = filepath.ToSlash(filepath.Join(day, name))
		b.Dir = filepath.Join(root, day, name)
		if err := os.Mkdir(b.Dir, 0o755); err == nil {
			break
		} else if !os.IsExist(err) {
			return nil, err
		}
	}

	if err := copyTree(src, filepath.Join(b.Dir, dataDir)); err != nil {
		os.RemoveAll(b.Dir)
		return nil, fmt.Errorf("copy %s: %w", src, err)
	}
	// info.json is written last: a backup without it is incomplete and not listed.
	info, _ := json.MarshalIndent(b, "", "  ")
	if err := os.WriteFile(filepath.Join(b.Dir, infoFile), append(info, '\n'), 0o644); err != nil {
		os.RemoveAll(b.Dir)
		return nil, err
	}
	return b, nil
}

// List returns all complete backups under root, newest first.
func List(root string) ([]*Backup, error) {
	infos, err := filepath.Glob(filepath.Join(root, "*", "*", "*", infoFile))
	if err != nil {
		return nil, err
	}
	var out []*Backup
	for _, p := range infos {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		b := &Backup{Dir: filepath.Dir(p)}
		if err := json.Unmarshal(data, b); err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		rel, err := filepath.Rel(root, b.Dir)
		if err != nil {
			return nil, err
		}
		b.ID = filepath.ToSlash(rel)
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out, nil
}

// Restore replaces dst with the backup's data. The swap is done with renames
// so dst is never left half-copied.
func (b *Backup) Restore(dst string) error {
	dst = filepath.Clean(dst)
	incoming, outgoing := dst+".staredit-new", dst+".staredit-old"
	for _, p := range []string{incoming, outgoing} {
		if err := os.RemoveAll(p); err != nil {
			return err
		}
	}
	if err := copyTree(filepath.Join(b.Dir, dataDir), incoming); err != nil {
		os.RemoveAll(incoming)
		return err
	}
	if err := os.Rename(dst, outgoing); err != nil && !os.IsNotExist(err) {
		os.RemoveAll(incoming)
		return err
	}
	if err := os.Rename(incoming, dst); err != nil {
		os.Rename(outgoing, dst) // put the original back
		return err
	}
	return os.RemoveAll(outgoing)
}

// Size returns the number of files in the backup and their total size.
func (b *Backup) Size() (files int, bytes int64) {
	filepath.WalkDir(filepath.Join(b.Dir, dataDir), func(_ string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			if st, err := d.Info(); err == nil {
				files++
				bytes += st.Size()
			}
		}
		return nil
	})
	return files, bytes
}

// copyTree copies a directory, keeping modes, mtimes and (when running as
// root) owners, so a restored tree is still writable by the server's user.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		st, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			if err := os.Mkdir(target, 0o700); err != nil {
				return err
			}
		case d.Type().IsRegular():
			if err := copyFile(path, target); err != nil {
				return err
			}
		default:
			return nil // sockets, symlinks: nothing a save directory should hold
		}
		if sys, ok := st.Sys().(*syscall.Stat_t); ok && os.Geteuid() == 0 {
			if err := os.Chown(target, int(sys.Uid), int(sys.Gid)); err != nil {
				return err
			}
		}
		if err := os.Chmod(target, st.Mode().Perm()); err != nil {
			return err
		}
		return os.Chtimes(target, st.ModTime(), st.ModTime())
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
