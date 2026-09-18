// Package savetest provides a synthetic save for tests, so they need no real
// player data.
package savetest

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"strings"
)

const (
	emptyHandle = "00000000000000000000000000000000"
	wolfram     = "/Game/Chimera/Items/I_WolframOre.I_WolframOre_C"
)

// item is a plain stackable item record, copied from a real save.
func item(h, itemData string, amount int) string {
	return fmt.Sprintf(`{"handle":{"handle":"%s"},"ownerId":{"handle":"%s"},"amount":%d,`+
		`"componentHandles":[],"componentsToAdd":[],"itemStats":[],"statsToGive":[],`+
		`"randomItemStats":{"pool":[]},"randomAttributes":{"pool":[]},`+
		`"generatedGrantedEffects":[],"generatedGrantedPassiveAbilities":[],"itemData":"%s"}`,
		h, h, amount, itemData)
}

// JSON builds a small document shaped like a real save: one player
// with a 2x3 inventory holding one stack, one player with an empty inventory,
// and some world data formatted the way the game writes it.
func JSON() []byte {
	inventory := func(firstSlot int, handles ...string) string {
		var slots, owned []string
		for i := 0; i < 6; i++ {
			h := emptyHandle
			if i < len(handles) {
				h = handles[i]
				owned = append(owned, fmt.Sprintf(`{"itemId":{"handle":"%s"}}`, h))
			}
			slots = append(slots, fmt.Sprintf(`{"itemId":{"handle":"%s"},"slotId":{"iD":%d},"column":%d,"row":%d,"ownerSlot":{"iD":-1}}`,
				h, firstSlot+i, i%3, i/3))
		}
		return fmt.Sprintf(`{"gridColumns":3,"gridRows":2,"slots":[%s],"ownedItems":[%s],"slottedItems":[]}`,
			strings.Join(slots, ","), strings.Join(owned, ","))
	}
	const h1 = "5378B33F4D6ECE0E3B041F926B4A97A7"
	alice := fmt.Sprintf(`{"survivalData":{"health":{"current":100,"min":0,"max":100}},`+
		`"itemsStoreState":{"itemsArray":[%s],"itemsInstancesSaveData":{},"itemsComponents":[]},`+
		`"inventoryState":%s,"profession":"None"}`,
		item(h1, wolfram, 68), inventory(100, h1))
	bob := fmt.Sprintf(`{"itemsStoreState":{"itemsArray":[],"itemsInstancesSaveData":{},"itemsComponents":[]},"inventoryState":%s}`,
		inventory(200))
	return []byte(fmt.Sprintf(`{"itemData":{"Mass":{"entities":{"(ID=1)":{"location":{"x":-317808.89691976301,"y":0,"z":6265.5693851925153}}},`+
		`"note":"quotes \" braces }{ and <html> & stay as written"},`+
		`"GameStateData":{"playtimeDuration":95038.265625,"allCharactersBaseSaveData":{"allPlayersSaveData":{`+
		`"76561198000000001":%s,"76561198000000002":%s}},"hintsData":[]}},"level":"/Game/Chimera/Maps/ChimeraMain/ChimeraMain.ChimeraMain","timestamp":"20260101000000"}`,
		alice, bob))
}

// Packed returns JSON in the on-disk container: uint32 size, then zlib.
func Packed() []byte {
	raw := JSON()
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, uint32(len(raw)))
	zw := zlib.NewWriter(&buf)
	zw.Write(raw)
	zw.Close()
	return buf.Bytes()
}
