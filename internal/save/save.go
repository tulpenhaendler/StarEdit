// Package save reads and edits StarRupture AutoSave0.sav files.
package save

import (
	"bytes"
	"compress/zlib"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

// File layout of AutoSave0.sav: uint32 LE uncompressed size, then a zlib
// stream holding a single JSON document.

const emptyHandle = "00000000000000000000000000000000"

// Path from the document root to the per-player save data.
var playersPath = []string{"itemData", "GameStateData", "allCharactersBaseSaveData", "allPlayersSaveData"}

type Save struct {
	// chain[0] is the root object, chain[len-1] is allPlayersSaveData.
	chain []*object
}

func Decode(data []byte) (*Save, error) {
	if len(data) < 6 {
		return nil, fmt.Errorf("file too short to be a save")
	}
	want := binary.LittleEndian.Uint32(data[:4])
	zr, err := zlib.NewReader(bytes.NewReader(data[4:]))
	if err != nil {
		return nil, fmt.Errorf("not a zlib-compressed save: %w", err)
	}
	raw, err := io.ReadAll(zr)
	if err != nil {
		return nil, fmt.Errorf("decompress: %w", err)
	}
	if uint32(len(raw)) != want {
		return nil, fmt.Errorf("size header says %d bytes, decompressed %d", want, len(raw))
	}
	return parseSave(raw)
}

func parseSave(raw []byte) (*Save, error) {
	root, err := parseObject(raw)
	if err != nil {
		return nil, err
	}
	s := &Save{chain: []*object{root}}
	for _, key := range playersPath {
		next, err := s.chain[len(s.chain)-1].child(key)
		if err != nil {
			return nil, fmt.Errorf("unexpected save layout: %w", err)
		}
		s.chain = append(s.chain, next)
	}
	return s, nil
}

func (s *Save) players() *object { return s.chain[len(s.chain)-1] }

// json reassembles the document, re-serializing only the objects on playersPath.
func (s *Save) json() []byte {
	for i := len(s.chain) - 1; i > 0; i-- {
		s.chain[i-1].set(playersPath[i-1], s.chain[i].bytes())
	}
	return s.chain[0].bytes()
}

func (s *Save) encode() ([]byte, error) {
	raw := s.json()
	if !json.Valid(raw) {
		return nil, fmt.Errorf("internal error: edited save is not valid JSON, refusing to write")
	}
	var buf bytes.Buffer
	var hdr [4]byte
	binary.LittleEndian.PutUint32(hdr[:], uint32(len(raw)))
	buf.Write(hdr[:])
	zw := zlib.NewWriter(&buf)
	if _, err := zw.Write(raw); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// HasPlayer reports whether the save holds data for this exact SteamID64.
func (s *Save) HasPlayer(id string) bool {
	_, ok := s.players().get(id)
	return ok
}

func (s *Save) PlayerIDs() []string {
	ids := append([]string(nil), s.players().keys...)
	sort.Strings(ids)
	return ids
}

// FindPlayer resolves a full SteamID64 or any unambiguous part of one.
func (s *Save) FindPlayer(query string) (string, error) {
	var matches []string
	for _, id := range s.PlayerIDs() {
		if id == query {
			return id, nil
		}
		if strings.Contains(id, query) {
			matches = append(matches, id)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return "", fmt.Errorf("no player matches %q (known: %s)", query, strings.Join(s.PlayerIDs(), ", "))
	default:
		return "", fmt.Errorf("%q is ambiguous: %s", query, strings.Join(matches, ", "))
	}
}

type handle struct {
	Handle string `json:"handle"`
}

type slot struct {
	ItemID handle `json:"itemId"`
	SlotID struct {
		ID int `json:"iD"`
	} `json:"slotId"`
	Column int `json:"column"`
	Row    int `json:"row"`
}

type ItemRecord struct {
	Handle   handle `json:"handle"`
	Amount   int    `json:"amount"`
	ItemData string `json:"itemData"`
}

// Inventory is a player's main inventory: item records live in
// itemsStoreState.itemsArray, and inventoryState references them by handle
// from both its slot grid and its ownedItems list.
type Inventory struct {
	save   *Save
	ID     string
	player *object
	store  *object
	inv    *object

	itemsRaw [][]byte
	Items    []ItemRecord
	slotsRaw [][]byte
	slots    []slot
	ownedRaw [][]byte
}

func (s *Save) Inventory(id string) (*Inventory, error) {
	player, err := s.players().child(id)
	if err != nil {
		return nil, err
	}
	n := &Inventory{save: s, ID: id, player: player}
	if n.store, err = player.child("itemsStoreState"); err != nil {
		return nil, err
	}
	if n.inv, err = player.child("inventoryState"); err != nil {
		return nil, err
	}
	if n.itemsRaw, err = rawArray(n.store, "itemsArray"); err != nil {
		return nil, err
	}
	if n.slotsRaw, err = rawArray(n.inv, "slots"); err != nil {
		return nil, err
	}
	if n.ownedRaw, err = rawArray(n.inv, "ownedItems"); err != nil {
		return nil, err
	}
	n.Items = make([]ItemRecord, len(n.itemsRaw))
	for i, raw := range n.itemsRaw {
		if err := json.Unmarshal(raw, &n.Items[i]); err != nil {
			return nil, fmt.Errorf("itemsArray[%d]: %w", i, err)
		}
	}
	n.slots = make([]slot, len(n.slotsRaw))
	for i, raw := range n.slotsRaw {
		if err := json.Unmarshal(raw, &n.slots[i]); err != nil {
			return nil, fmt.Errorf("slots[%d]: %w", i, err)
		}
	}
	return n, nil
}

func rawArray(o *object, key string) ([][]byte, error) {
	raw, ok := o.get(key)
	if !ok {
		return nil, fmt.Errorf("key %q not found", key)
	}
	var msgs []json.RawMessage
	if err := json.Unmarshal(raw, &msgs); err != nil {
		return nil, fmt.Errorf("%s: %w", key, err)
	}
	out := make([][]byte, len(msgs))
	for i, m := range msgs {
		out[i] = m
	}
	return out, nil
}

func joinArray(elems [][]byte) []byte {
	return append(append([]byte{'['}, bytes.Join(elems, []byte{','})...), ']')
}

func (n *Inventory) SlotCount() int { return len(n.slots) }

func (n *Inventory) FreeSlots() int {
	free := 0
	for _, s := range n.slots {
		if s.ItemID.Handle == emptyHandle {
			free++
		}
	}
	return free
}

// Give adds amount of the item, topping up existing stacks first and then
// filling empty slots with new stacks of at most stackSize. It changes
// nothing unless everything fits.
func (n *Inventory) Give(itemData string, amount, stackSize int) (topped, created int, err error) {
	remaining := amount
	topUps := map[int]int{}
	for i, it := range n.Items {
		if remaining == 0 {
			break
		}
		if it.ItemData == itemData && it.Amount < stackSize {
			add := min(stackSize-it.Amount, remaining)
			topUps[i] = add
			remaining -= add
		}
	}
	newStacks := (remaining + stackSize - 1) / stackSize
	if free := n.FreeSlots(); newStacks > free {
		return 0, 0, fmt.Errorf("not enough room: need %d free slots, player has %d (give less, or raise -stack)", newStacks, free)
	}

	for i, add := range topUps {
		rec, err := parseObject(n.itemsRaw[i])
		if err != nil {
			return 0, 0, err
		}
		n.Items[i].Amount += add
		rec.set("amount", marshal(n.Items[i].Amount))
		n.itemsRaw[i] = rec.bytes()
	}
	for si := range n.slots {
		if remaining == 0 {
			break
		}
		if n.slots[si].ItemID.Handle != emptyHandle {
			continue
		}
		h := newHandle()
		count := min(stackSize, remaining)
		remaining -= count

		n.itemsRaw = append(n.itemsRaw, newItemRecord(h, itemData, count))
		n.Items = append(n.Items, ItemRecord{Handle: handle{h}, Amount: count, ItemData: itemData})
		n.ownedRaw = append(n.ownedRaw, marshal(map[string]handle{"itemId": {h}}))

		so, err := parseObject(n.slotsRaw[si])
		if err != nil {
			return 0, 0, err
		}
		so.set("itemId", marshal(handle{h}))
		n.slotsRaw[si] = so.bytes()
		n.slots[si].ItemID.Handle = h
	}
	return len(topUps), newStacks, nil
}

// Commit writes the inventory back into the save's object tree.
func (n *Inventory) Commit() {
	n.store.set("itemsArray", joinArray(n.itemsRaw))
	n.inv.set("slots", joinArray(n.slotsRaw))
	n.inv.set("ownedItems", joinArray(n.ownedRaw))
	n.player.set("itemsStoreState", n.store.bytes())
	n.player.set("inventoryState", n.inv.bytes())
	n.save.players().set(n.ID, n.player.bytes())
}

// newItemRecord mirrors the field order of a plain stackable item as the game writes it.
func newItemRecord(h, itemData string, amount int) []byte {
	return []byte(fmt.Sprintf(`{"handle":{"handle":"%s"},"ownerId":{"handle":"%s"},"amount":%d,`+
		`"componentHandles":[],"componentsToAdd":[],"itemStats":[],"statsToGive":[],`+
		`"randomItemStats":{"pool":[]},"randomAttributes":{"pool":[]},`+
		`"generatedGrantedEffects":[],"generatedGrantedPassiveAbilities":[],"itemData":%s}`,
		h, h, amount, marshal(itemData)))
}

// newHandle returns a random GUID in the game's format: 32 uppercase hex digits.
func newHandle() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return fmt.Sprintf("%X", b[:])
}

func Read(path string) (*Save, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Decode(data)
}

// Write replaces the file atomically, keeping the original owner and mode.
func Write(path string, s *Save) error {
	data, err := s.encode()
	if err != nil {
		return err
	}
	if _, err := Decode(data); err != nil {
		return fmt.Errorf("internal error: edited save does not read back: %w", err)
	}
	st, err := os.Stat(path)
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".staredit-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), st.Mode().Perm()); err != nil {
		return err
	}
	if sys, ok := st.Sys().(*syscall.Stat_t); ok {
		// Only root can hand the file back to the server's user; for
		// everyone else the file stays ours, which is fine for copies.
		if err := os.Chown(tmp.Name(), int(sys.Uid), int(sys.Gid)); err != nil && os.Geteuid() == 0 {
			return err
		}
	}
	return os.Rename(tmp.Name(), path)
}
