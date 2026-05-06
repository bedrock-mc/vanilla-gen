package block

import (
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
)

// EndGateway is the portal block found on the outer End islands.
type EndGateway struct{ base }

func (EndGateway) EntityInside(pos cube.Pos, _ *world.Tx, e world.Entity) {
	if traveler, ok := e.(interface{ EnterEndGateway(cube.Pos) }); ok {
		traveler.EnterEndGateway(pos)
	}
}

func (EndGateway) LightEmissionLevel() uint8 { return 15 }

func (EndGateway) EncodeBlock() (string, map[string]any) {
	return "minecraft:end_gateway", nil
}
