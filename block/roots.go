package block

// Roots are non-solid Nether plants found on nylium and soul soil.
type Roots struct {
	base
	Warped bool
}

func (Roots) HasLiquidDrops() bool { return false }

func (Roots) CompostChance() float64 { return 0.65 }

func (r Roots) EncodeItem() (name string, meta int16) {
	if r.Warped {
		return "minecraft:warped_roots", 0
	}
	return "minecraft:crimson_roots", 0
}

func (r Roots) EncodeBlock() (string, map[string]any) {
	if r.Warped {
		return "minecraft:warped_roots", nil
	}
	return "minecraft:crimson_roots", nil
}
