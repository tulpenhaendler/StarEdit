package items

import "testing"

func TestResolve(t *testing.T) {
	for query, want := range map[string]string{
		"quartz":        "I_QuartzOre",
		"Ignitium":      "I_FireWaveOre",
		"I_TitaniumBar": "I_TitaniumBar",
		"titaniumbar":   "I_TitaniumBar",
		"StandardAmmo":  "I_StandardAmmoItem",
	} {
		got, err := Resolve(query)
		if err != nil || ShortName(got) != want {
			t.Errorf("Resolve(%q) = %q, %v; want %s", query, got, err, want)
		}
	}
	if _, err := Resolve("ore"); err == nil {
		t.Error("expected ambiguity error for \"ore\"")
	}
}
