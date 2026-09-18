// Package cli implements the staredit command line.
package cli

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/tulpenhaendler/StarEdit/internal/backup"
	"github.com/tulpenhaendler/StarEdit/internal/config"
	"github.com/tulpenhaendler/StarEdit/internal/items"
	"github.com/tulpenhaendler/StarEdit/internal/save"
)

const defaultSave = "/opt/starrupture/server/StarRupture/Saved/SaveGames/StarRuptureServer/AutoSave0.sav"

const usage = `staredit - StarRupture save editor

Usage:
  staredit players                              list players in the save
  staredit inv <player>                         show a player's inventory
  staredit items [filter]                       list known item names
  staredit give <player> <item> <amount> [<item> <amount>...]
  staredit backup [note]                        back up the data directory now
  staredit backups                              list backups, newest first
  staredit restore <number|id>                  put a backup back in place

<player> is a name from config.json, a SteamID64, or any unique part of one.
<item> is a name like quartz, ignitium, I_TitaniumBar, or any unique part of one.
<number|id> is the number or the id shown by "staredit backups".

give, backup and restore always run the same cycle:

  stop_server.sh  ->  copy the data directory to backups/<month>/<day>/<time>
                  ->  make the change  ->  start_server.sh

The server keeps the world in memory and overwrites the save, so it has to be
down while the files change. stop_server.sh must only return once the server
has saved and exited. start_server.sh runs even when the change failed.

Flags (any position):
  -config <path>   config file (default: $STAREDIT_CONFIG, ./config.json,
                   or config.json beside the executable)
  -save <path>     work on a copy of a save instead of the configured one:
                   the server scripts are not run, and that file's folder is
                   what gets backed up and restored
  -stack <n>       give: max items per stack (default 100)
  -dry-run         give: show what would change; touch nothing
  -force           skip the safety checks (server still running after
                   stop_server.sh; restoring a backup of another directory)
`

type options struct {
	cfg         *config.Config
	save        string
	dataDir     string
	backupDir   string
	stopScript  string
	startScript string
	offline     bool // -save given: a copy, no server involved
	stack       int
	dryRun      bool
	force       bool
}

// Run executes one staredit invocation; args excludes the program name.
func Run(args []string) error {
	var opt options
	fs := flag.NewFlagSet("staredit", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	var configPath, saveFlag string
	fs.StringVar(&configPath, "config", "", "")
	fs.StringVar(&saveFlag, "save", "", "")
	fs.IntVar(&opt.stack, "stack", 100, "")
	fs.BoolVar(&opt.dryRun, "dry-run", false, "")
	fs.BoolVar(&opt.force, "force", false, "")

	// Allow flags before, between and after positional arguments.
	var pos []string
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			if err == flag.ErrHelp {
				return nil
			}
			return err
		}
		if args = fs.Args(); len(args) > 0 {
			pos = append(pos, args[0])
			args = args[1:]
		}
	}
	if len(pos) == 0 {
		fs.Usage()
		return nil
	}
	if opt.stack < 1 {
		return fmt.Errorf("-stack must be at least 1")
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	opt.cfg = cfg
	opt.backupDir = cfg.Path(cfg.BackupDir, "backups")
	opt.stopScript = cfg.Path(cfg.StopScript, "stop_server.sh")
	opt.startScript = cfg.Path(cfg.StartScript, "start_server.sh")
	if saveFlag != "" {
		opt.offline = true
		opt.save = saveFlag
		opt.dataDir = filepath.Dir(saveFlag)
	} else {
		opt.save = cfg.Path(cfg.Save, defaultSave)
		opt.dataDir = cfg.Path(cfg.DataDir, filepath.Dir(opt.save))
	}

	switch cmd, rest := pos[0], pos[1:]; cmd {
	case "players":
		return cmdPlayers(opt)
	case "inv":
		if len(rest) != 1 {
			return fmt.Errorf("usage: staredit inv <player>")
		}
		return cmdInv(opt, rest[0])
	case "items":
		return cmdItems(strings.Join(rest, " "))
	case "give":
		if len(rest) < 3 || len(rest)%2 != 1 {
			return fmt.Errorf("usage: staredit give <player> <item> <amount> [<item> <amount>...]")
		}
		return cmdGive(opt, rest[0], rest[1:])
	case "backup":
		return cmdBackup(opt, strings.Join(rest, " "))
	case "backups":
		return cmdBackups(opt)
	case "restore":
		if len(rest) != 1 {
			return fmt.Errorf("usage: staredit restore <number|id>   (see: staredit backups)")
		}
		return cmdRestore(opt, rest[0])
	case "help":
		fs.Usage()
		return nil
	default:
		return fmt.Errorf("unknown command %q (see staredit help)", cmd)
	}
}

func cmdPlayers(opt options) error {
	s, err := save.Read(opt.save)
	if err != nil {
		return err
	}
	for _, id := range s.PlayerIDs() {
		n, err := s.Inventory(id)
		if err != nil {
			return fmt.Errorf("player %s: %w", id, err)
		}
		fmt.Printf("%s  %d stacks, %d of %d slots free\n", opt.cfg.Label(id), len(n.Items), n.FreeSlots(), n.SlotCount())
	}
	return nil
}

func cmdInv(opt options, player string) error {
	s, err := save.Read(opt.save)
	if err != nil {
		return err
	}
	id, err := findPlayer(opt.cfg, s, player)
	if err != nil {
		return err
	}
	n, err := s.Inventory(id)
	if err != nil {
		return err
	}
	printInventory(opt.cfg, n)
	return nil
}

func printInventory(cfg *config.Config, n *save.Inventory) {
	totals := map[string]int{}
	stacks := map[string]int{}
	for _, it := range n.Items {
		totals[items.ShortName(it.ItemData)] += it.Amount
		stacks[items.ShortName(it.ItemData)]++
	}
	names := make([]string, 0, len(totals))
	for name := range totals {
		names = append(names, name)
	}
	sort.Strings(names)
	fmt.Printf("player %s: %d of %d slots free\n", cfg.Label(n.ID), n.FreeSlots(), n.SlotCount())
	for _, name := range names {
		fmt.Printf("  %-34s %6d  (%d stacks)\n", name, totals[name], stacks[name])
	}
}

func cmdItems(filter string) error {
	filter = strings.ToLower(filter)
	for _, p := range items.Paths() {
		if strings.Contains(strings.ToLower(p), filter) {
			fmt.Printf("%-40s %s\n", items.ShortName(p), p)
		}
	}
	return nil
}

type grant struct {
	path   string
	amount int
}

func cmdGive(opt options, player string, pairs []string) error {
	var grants []grant
	for i := 0; i < len(pairs); i += 2 {
		path, err := items.Resolve(pairs[i])
		if err != nil {
			return err
		}
		amount, err := strconv.Atoi(pairs[i+1])
		if err != nil || amount < 1 {
			return fmt.Errorf("bad amount %q for %s", pairs[i+1], pairs[i])
		}
		if !items.IsPlain(path) {
			fmt.Fprintf(os.Stderr, "warning: %s is not a plain stackable item (weapon, mod, gem...); it may not work in game\n", items.ShortName(path))
		}
		grants = append(grants, grant{path, amount})
	}

	// apply reads the save fresh each time: the server rewrites it on shutdown.
	apply := func(verbose bool) (*save.Save, error) {
		s, err := save.Read(opt.save)
		if err != nil {
			return nil, err
		}
		id, err := findPlayer(opt.cfg, s, player)
		if err != nil {
			return nil, err
		}
		n, err := s.Inventory(id)
		if err != nil {
			return nil, err
		}
		for _, g := range grants {
			topped, created, err := n.Give(g.path, g.amount, opt.stack)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", items.ShortName(g.path), err)
			}
			if verbose {
				fmt.Printf("+%d %s (%d stacks topped up, %d new)\n", g.amount, items.ShortName(g.path), topped, created)
			}
		}
		n.Commit()
		if verbose {
			printInventory(opt.cfg, n)
		}
		return s, nil
	}

	// Rehearse against the current save so a typo or a full inventory is
	// caught before the server is taken down.
	if _, err := apply(opt.dryRun); err != nil {
		return err
	}
	if opt.dryRun {
		fmt.Println("dry run: nothing written, server not touched")
		return nil
	}

	reason := "before: give " + player + " " + strings.Join(pairs, " ")
	return withServerStopped(opt, reason, func() error {
		s, err := apply(true)
		if err != nil {
			return err
		}
		if err := save.Write(opt.save, s); err != nil {
			return err
		}
		fmt.Printf("wrote %s\n", opt.save)
		return nil
	})
}

func cmdBackup(opt options, note string) error {
	reason := "manual"
	if note != "" {
		reason += ": " + note
	}
	return withServerStopped(opt, reason, func() error { return nil })
}

func cmdBackups(opt options) error {
	list, err := backup.List(opt.backupDir)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		fmt.Printf("no backups in %s\n", opt.backupDir)
		return nil
	}
	for i, b := range list {
		files, size := b.Size()
		fmt.Printf("%3d  %s  %d files, %s  %s\n", i+1, b.ID, files, humanSize(size), b.Reason)
	}
	fmt.Printf("restore with: staredit restore <number|id>   (backups are in %s)\n", opt.backupDir)
	return nil
}

func cmdRestore(opt options, which string) error {
	list, err := backup.List(opt.backupDir)
	if err != nil {
		return err
	}
	b, err := pickBackup(list, which)
	if err != nil {
		return err
	}
	if abs, _ := filepath.Abs(opt.dataDir); abs != b.DataDir && !opt.force {
		return fmt.Errorf("backup %s was taken from %s, not %s; use -force to restore it there anyway", b.ID, b.DataDir, abs)
	}
	// The pre-restore backup makes a restore itself undoable.
	return withServerStopped(opt, "before: restore "+b.ID, func() error {
		if err := b.Restore(opt.dataDir); err != nil {
			return err
		}
		fmt.Printf("restored %s (%s) to %s\n", b.ID, b.Reason, opt.dataDir)
		return nil
	})
}

// pickBackup accepts the 1-based number from "staredit backups" or a backup id.
func pickBackup(list []*backup.Backup, which string) (*backup.Backup, error) {
	if n, err := strconv.Atoi(which); err == nil {
		if n < 1 || n > len(list) {
			return nil, fmt.Errorf("no backup number %d (there are %d)", n, len(list))
		}
		return list[n-1], nil
	}
	for _, b := range list {
		if b.ID == strings.Trim(which, "/") {
			return b, nil
		}
	}
	return nil, fmt.Errorf("no backup with id %q (see: staredit backups)", which)
}

// withServerStopped is the cycle every change goes through: stop the server,
// back up the data directory, run change, start the server again.
func withServerStopped(opt options, reason string, change func() error) (err error) {
	if !opt.offline {
		for _, script := range []string{opt.stopScript, opt.startScript} {
			if _, err := os.Stat(script); err != nil {
				return fmt.Errorf("%w\nstaredit stops the server around every change and needs both stop_server.sh and start_server.sh (see README)", err)
			}
		}
		fmt.Printf("stopping server: %s\n", opt.stopScript)
		// Once a stop was attempted, always try to bring the server back.
		defer func() {
			fmt.Printf("starting server: %s\n", opt.startScript)
			if startErr := runScript(opt.startScript); startErr != nil {
				if err == nil {
					err = startErr
				} else {
					fmt.Fprintln(os.Stderr, "staredit:", startErr)
				}
			}
		}()
		if err := runScript(opt.stopScript); err != nil {
			return err
		}
		if serverRunning() && !opt.force {
			return fmt.Errorf("the server is still running after %s; it would overwrite the change, so nothing was touched", opt.stopScript)
		}
	}

	b, err := backup.Create(opt.backupDir, opt.dataDir, reason)
	if err != nil {
		return fmt.Errorf("backup failed, nothing was changed: %w", err)
	}
	fmt.Printf("backup %s  <- %s\n", b.ID, opt.dataDir)
	return change()
}

func runScript(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	cmd := exec.Command(abs)
	cmd.Dir = filepath.Dir(abs)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w (is it executable? chmod +x)", path, err)
	}
	return nil
}

// findPlayer resolves a configured name first, then falls back to SteamID matching.
func findPlayer(cfg *config.Config, s *save.Save, query string) (string, error) {
	if id, ok := cfg.SteamID(query); ok {
		if !s.HasPlayer(id) {
			return "", fmt.Errorf("%s (%s) is not in this save; players appear once they have joined the server", query, id)
		}
		return id, nil
	}
	return s.FindPlayer(query)
}

// serverRunning looks for the game's server binary among all processes.
// A variable so tests can run on a machine that hosts a real server.
var serverRunning = func() bool {
	procs, _ := filepath.Glob("/proc/[0-9]*/cmdline")
	for _, p := range procs {
		// Match the game binary, not our own -save path or a sudo wrapping it.
		if b, err := os.ReadFile(p); err == nil && strings.Contains(string(b), "StarRuptureServer") && strings.Contains(string(b), "Shipping.exe") {
			return true
		}
	}
	return false
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}
