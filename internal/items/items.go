// Package items knows the item classes shipped with the game.
package items

import (
	_ "embed"
	"fmt"
	"sort"
	"strings"
)

// items.txt lists every item class shipped in the game's pak files, as the
// asset path the save uses in "itemData". Regenerate it after a game update
// that adds items (see README).
//
//go:embed items.txt
var itemsTxt string

// aliases map in-game display names to internal ones where they differ.
var aliases = map[string]string{
	"quartz":   "I_QuartzOre",
	"ignitium": "I_FireWaveOre", // renamed in-game, asset kept its old name
	"ignitum":  "I_FireWaveOre",
	"helium":   "I_HeliumOre",
	"helium-3": "I_HeliumOre",
	"wolfram":  "I_WolframOre",
	"titanium": "I_TitaniumOre",
	"calcium":  "I_CalciumOre",
	"sulphur":  "I_SulphurOre",
	"sulfur":   "I_SulphurOre",
	"coal":     "I_CoalOre",
	"copper":   "I_CopperOre",
	"gold":     "I_GoldOre",
	"goethite": "I_GoethiteOre",
	"pyrite":   "I_PyriteOre",
}

// Paths returns every known item as the asset path the save uses.
func Paths() []string {
	return strings.Fields(itemsTxt)
}

// ShortName turns "/Game/Chimera/Items/I_QuartzOre.I_QuartzOre_C" into "I_QuartzOre".
func ShortName(path string) string {
	name := path[strings.LastIndexByte(path, '/')+1:]
	if i := strings.IndexByte(name, '.'); i >= 0 {
		name = name[:i]
	}
	return name
}

// IsPlain reports whether the item is a simple stackable. Weapons, mods
// and gems carry component and stat data that a bare stack record lacks.
func IsPlain(path string) bool {
	return strings.HasPrefix(path, "/Game/Chimera/Items/") ||
		strings.HasPrefix(path, "/Game/Chimera/Weapons/AmmoTypes/")
}

// Resolve accepts an alias, a full asset path, an exact name with or
// without the "I_" prefix, or any unambiguous part of a name.
func Resolve(query string) (string, error) {
	q := strings.ToLower(query)
	if a, ok := aliases[q]; ok {
		q = strings.ToLower(a)
	}
	var partial []string
	for _, p := range Paths() {
		name := strings.ToLower(ShortName(p))
		if strings.ToLower(p) == q || name == q || name == "i_"+q {
			return p, nil
		}
		if strings.Contains(name, q) {
			partial = append(partial, p)
		}
	}
	switch len(partial) {
	case 1:
		return partial[0], nil
	case 0:
		return "", fmt.Errorf("unknown item %q (try: staredit items <filter>)", query)
	default:
		names := make([]string, len(partial))
		for i, p := range partial {
			names[i] = ShortName(p)
		}
		sort.Strings(names)
		if len(names) > 12 {
			names = append(names[:12], "...")
		}
		return "", fmt.Errorf("item %q is ambiguous: %s", query, strings.Join(names, ", "))
	}
}
