package block

import (
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
)

// EndPortal is the active End portal block.
type EndPortal struct{ base }

func (EndPortal) Portal() world.Dimension { return world.End }

func (EndPortal) EntityInside(pos cube.Pos, _ *world.Tx, e world.Entity) {
	if traveler, ok := e.(interface{ EnterEndPortal(cube.Pos) }); ok {
		traveler.EnterEndPortal(pos)
	}
}

func (EndPortal) HasLiquidDrops() bool { return false }

func (EndPortal) LightEmissionLevel() uint8 { return 15 }

func (EndPortal) EncodeBlock() (string, map[string]any) {
	return "minecraft:end_portal", nil
}
