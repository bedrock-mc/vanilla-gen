package block

import "github.com/df-mc/dragonfly/server/world"

// EndStone is a block found in The End.
type EndStone struct{ base }

func (e EndStone) SoilFor(b world.Block) bool {
	switch b.(type) {
	case ChorusPlant, ChorusFlower:
		return true
	default:
		return false
	}
}

func (EndStone) EncodeItem() (name string, meta int16) {
	return "minecraft:end_stone", 0
}

func (EndStone) EncodeBlock() (string, map[string]any) {
	return "minecraft:end_stone", nil
}
