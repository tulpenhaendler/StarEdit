package save

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/tulpenhaendler/StarEdit/internal/save/savetest"
)

const quartz = "/Game/Chimera/Items/I_QuartzOre.I_QuartzOre_C"

func unpack(t *testing.T, data []byte) []byte {
	t.Helper()
	zr, err := zlib.NewReader(bytes.NewReader(data[4:]))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// fixtures returns the synthetic save plus any real saves dropped into
// testdata/ (which is not tracked in git).
func fixtures(t *testing.T) map[string][]byte {
	t.Helper()
	out := map[string][]byte{"synthetic": savetest.Packed()}
	real, _ := filepath.Glob("../../testdata/*.sav") // repo root
	for _, path := range real {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		out[filepath.Base(path)] = data
	}
	return out
}

// firstPlayerWithItems keeps the tests independent of the SteamIDs in a save.
func firstPlayerWithItems(t *testing.T, s *Save) *Inventory {
	t.Helper()
	for _, id := range s.PlayerIDs() {
		n, err := s.Inventory(id)
		if err != nil {
			t.Fatal(err)
		}
		if len(n.Items) > 0 {
			return n
		}
	}
	t.Skip("no player with items in this save")
	return nil
}

// An unedited save must serialize back to exactly the bytes the game wrote.
func TestRoundTripIsByteIdentical(t *testing.T) {
	for name, data := range fixtures(t) {
		t.Run(name, func(t *testing.T) {
			s, err := Decode(data)
			if err != nil {
				t.Fatal(err)
			}
			firstPlayerWithItems(t, s).Commit()
			if !bytes.Equal(s.json(), unpack(t, data)) {
				t.Fatal("round trip changed the JSON")
			}
		})
	}
}

func TestGive(t *testing.T) {
	for name, data := range fixtures(t) {
		t.Run(name, func(t *testing.T) {
			s, err := Decode(data)
			if err != nil {
				t.Fatal(err)
			}
			n := firstPlayerWithItems(t, s)
			stacksBefore, freeBefore, quartzBefore := len(n.Items), n.FreeSlots(), 0
			for _, it := range n.Items {
				if it.ItemData == quartz {
					t.Skip("save already holds quartz; stack arithmetic below assumes none")
				}
			}

			if _, created, err := n.Give(quartz, 250, 100); err != nil || created != 3 {
				t.Fatalf("give: created=%d err=%v", created, err)
			}
			// 50 tops up the 50-stack left by the first give; 100 needs one new stack.
			if topped, created, err := n.Give(quartz, 150, 100); err != nil || topped != 1 || created != 1 {
				t.Fatalf("second give: topped=%d created=%d err=%v", topped, created, err)
			}
			n.Commit()

			encoded, err := s.encode()
			if err != nil {
				t.Fatal(err)
			}
			s2, err := Decode(encoded)
			if err != nil {
				t.Fatal(err)
			}
			n2, err := s2.Inventory(n.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got := len(n2.Items); got != stacksBefore+4 {
				t.Errorf("stacks = %d, want %d", got, stacksBefore+4)
			}
			if got := n2.FreeSlots(); got != freeBefore-4 {
				t.Errorf("free slots = %d, want %d", got, freeBefore-4)
			}
			total := 0
			slotted := map[string]bool{}
			for _, sl := range n2.slots {
				slotted[sl.ItemID.Handle] = true
			}
			for _, it := range n2.Items {
				if it.ItemData == quartz {
					total += it.Amount
				}
				if !slotted[it.Handle.Handle] {
					t.Errorf("item %s has no slot", it.Handle.Handle)
				}
			}
			if total != quartzBefore+400 {
				t.Errorf("quartz total = %d, want %d", total, quartzBefore+400)
			}
			if len(n2.ownedRaw) != len(n2.Items) {
				t.Errorf("ownedItems = %d, items = %d", len(n2.ownedRaw), len(n2.Items))
			}

			// Nothing outside the edited player may change.
			var before, after map[string]any
			if err := json.Unmarshal(unpack(t, data), &before); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(unpack(t, encoded), &after); err != nil {
				t.Fatal(err)
			}
			players := func(doc map[string]any) map[string]any {
				m := doc
				for _, k := range playersPath {
					m = m[k].(map[string]any)
				}
				return m
			}
			delete(players(before), n.ID)
			delete(players(after), n.ID)
			if !reflect.DeepEqual(before, after) {
				t.Error("give changed data outside the target player")
			}
		})
	}
}

func TestGiveRefusesWhenFull(t *testing.T) {
	s, err := Decode(savetest.Packed())
	if err != nil {
		t.Fatal(err)
	}
	n := firstPlayerWithItems(t, s)
	before := len(n.itemsRaw)
	if _, _, err := n.Give(quartz, 100*(n.FreeSlots()+1), 100); err == nil {
		t.Fatal("expected an error when the inventory cannot hold the items")
	}
	if len(n.itemsRaw) != before {
		t.Error("failed give modified the inventory")
	}
}
