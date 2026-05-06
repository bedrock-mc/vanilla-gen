package block

// Fungus is a non-solid Nether plant that grows on nylium.
type Fungus struct {
	base
	Warped bool
}

func (Fungus) HasLiquidDrops() bool { return false }

func (Fungus) CompostChance() float64 { return 0.65 }

func (f Fungus) EncodeItem() (name string, meta int16) {
	if f.Warped {
		return "minecraft:warped_fungus", 0
	}
	return "minecraft:crimson_fungus", 0
}

func (f Fungus) EncodeBlock() (string, map[string]any) {
	if f.Warped {
		return "minecraft:warped_fungus", nil
	}
	return "minecraft:crimson_fungus", nil
}
