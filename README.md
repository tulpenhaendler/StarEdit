# staredit

Save editor for a StarRupture dedicated server: gives items to a player's inventory.

```
cp config.example.json config.json   # then edit it; check the two server scripts

go run main.go items ore
go build -o staredit .               # the live save usually needs root

sudo ./staredit players
sudo ./staredit inv alice
sudo ./staredit give alice quartz 500 ignitium 200
sudo ./staredit backups
sudo ./staredit restore 1
```

## Layout

```
main.go                  entry point
internal/cli             commands and flags
internal/save            save container, byte-preserving JSON, inventory edits
internal/save/savetest   synthetic save used by the tests
internal/items           item names (items.txt, generated from the game paks)
internal/config          config.json
internal/backup          timestamped copies of the data directory
tools/extract_items.py   regenerates items.txt
stop_server.sh           run before every change
start_server.sh          run after every change
```

Items go by name (`quartz`, `ignitium`, `I_TitaniumBar`, `titaniumbar`, ...);
`staredit items` lists all of them. Ignitium is `I_FireWaveOre` internally.

## How a change is made

`give`, `restore` and `backup` all go through the same cycle:

1. `stop_server.sh`
2. copy the whole data directory to `backups/<YYYY-MM>/<DD>/<HH-MM-SS>/`
3. make the change
4. `start_server.sh` (also when step 2 or 3 failed)

The server keeps the world in memory and rewrites the save file, so it has to
be down while files change. **`stop_server.sh` must only return once the
server has saved and exited.** The shipped scripts use `systemctl stop/start
starrupture`; replace them with whatever runs your server (docker, screen,
...). If the server process is still alive after the stop script, staredit
changes nothing.

`give` rehearses the edit on the current save first, so an unknown player, a
typo or a full inventory is reported without taking the server down.
`-dry-run` stops after that rehearsal.

`give` tops up existing stacks, then fills empty slots with stacks of at most
`-stack` (default 100, the ore stack size).

## Backups

```
$ staredit backups
  1  2026-09/18/14-31-07  3 files, 1.3 MB  before: restore 2026-09/18/14-05-32
  2  2026-09/18/14-05-32  3 files, 1.3 MB  before: give alice quartz 500
$ staredit restore 2            # or: staredit restore 2026-09/18/14-05-32
$ staredit backup "before the patch"
```

Every backup is a plain copy in `data/` plus an `info.json` saying when and
why it was made, so you can also inspect or copy one by hand. A restore swaps
the data directory for the backup's copy (files that are not in the backup are
gone afterwards) and, like any change, is preceded by a backup of its own, so
it can be undone. File owners and modes are kept when running as root.
Backups are never deleted automatically.

## Config

The save only knows players by SteamID64, so `config.json` maps names to IDs.
It also says where things are; relative paths are relative to the config file.

| key | meaning | default |
| --- | --- | --- |
| `save` | the `.sav` file to edit | the usual `/opt/starrupture/...` path |
| `data_dir` | folder that is backed up and restored | the folder of `save` |
| `backup_dir` | where backups go | `backups` |
| `stop_script`, `start_script` | server control | `stop_server.sh`, `start_server.sh` |
| `players` | name -> SteamID64 | none; use the ID, or any unique part of it |

Pointing `data_dir` at `SaveGames` (one level above the session folder) also
covers `SaveData.dat`.

The file is looked up as `-config <path>`, `$STAREDIT_CONFIG`, `./config.json`,
then `config.json` beside the executable. `config.json` and `backups/` are
gitignored; commit only `config.example.json`.

`-save <path>` works on a copy of a save instead: no server scripts are run,
and the backup covers that file's folder.

## Save format

`AutoSave0.sav` is a `uint32` little-endian uncompressed size followed by a
zlib stream containing one JSON document. Players live under
`itemData.GameStateData.allCharactersBaseSaveData.allPlayersSaveData.<SteamID64>`.
An inventory item is three linked things:

- a record in `itemsStoreState.itemsArray` (`handle`, `amount`, `itemData` asset path),
- a slot in `inventoryState.slots` whose `itemId.handle` points at it,
- an entry in `inventoryState.ownedItems`.

The editor re-serializes only the objects on that path; the rest of the file is
written back byte for byte.

## Tests

`go test ./...` runs against a synthetic save. Drop real saves into
`testdata/*.sav` (gitignored) to run the same checks against them.

## After a game update

New items need a fresh `internal/items/items.txt`:

```
sudo python3 tools/extract_items.py \
  /opt/starrupture/server/StarRupture/Content/Paks/pakchunk0-WindowsServer.utoc > internal/items/items.txt
go build -o staredit .
```

## License

MIT, see [LICENSE](LICENSE). Not affiliated with the makers of StarRupture.
