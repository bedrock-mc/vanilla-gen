package block

import (
	"github.com/bedrock-mc/vanilla-gen/block/model"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
)

// Portal is the active Nether portal block.
type Portal struct {
	base
	Axis cube.Axis
}

func (p Portal) Model() world.BlockModel { return model.Portal{Axis: p.Axis} }

func (Portal) Portal() world.Dimension { return world.Nether }

func (p Portal) EntityInside(pos cube.Pos, _ *world.Tx, e world.Entity) {
	if traveler, ok := e.(interface{ EnterNetherPortal(cube.Pos, cube.Axis) }); ok {
		traveler.EnterNetherPortal(pos, p.Axis)
	}
}

func (Portal) HasLiquidDrops() bool { return false }

func (p Portal) EncodeBlock() (string, map[string]any) {
	return "minecraft:portal", map[string]any{"portal_axis": p.Axis.String()}
}
