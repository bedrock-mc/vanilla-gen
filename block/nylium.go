package block

import "github.com/df-mc/dragonfly/server/world"

// Nylium is a fungal grass-like block found in the Nether.
type Nylium struct {
	base
	Warped bool
}

func (n Nylium) SoilFor(b world.Block) bool {
	switch b.(type) {
	case Roots, Fungus:
		return true
	default:
		return false
	}
}

func (n Nylium) EncodeItem() (name string, meta int16) {
	if n.Warped {
		return "minecraft:warped_nylium", 0
	}
	return "minecraft:crimson_nylium", 0
}

func (n Nylium) EncodeBlock() (string, map[string]any) {
	if n.Warped {
		return "minecraft:warped_nylium", nil
	}
	return "minecraft:crimson_nylium", nil
}
