package block

import (
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
)

type LeafLitter struct {
	base
	Growth int
	Facing cube.Direction
}

func (l LeafLitter) EncodeBlock() (string, map[string]any) {
	return "minecraft:leaf_litter", map[string]any{
		"growth":                       int32(max(0, min(7, l.Growth))),
		"minecraft:cardinal_direction": l.Facing.String(),
	}
}

func allLeafLitter() (b []world.Block) {
	for _, facing := range []cube.Direction{cube.South, cube.West, cube.North, cube.East} {
		for growth := 0; growth <= 7; growth++ {
			b = append(b, LeafLitter{Growth: growth, Facing: facing})
		}
	}
	return
}
