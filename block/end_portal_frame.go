package block

import (
	"github.com/bedrock-mc/vanilla-gen/block/model"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
)

// EndPortalFrame is the frame block used to create End portals.
type EndPortalFrame struct {
	base
	Facing cube.Direction
	Eye    bool
}

func (f EndPortalFrame) Model() world.BlockModel {
	return model.EndPortalFrame{Eye: f.Eye}
}

func (EndPortalFrame) LightEmissionLevel() uint8 { return 1 }

func (EndPortalFrame) EncodeItem() (name string, meta int16) {
	return "minecraft:end_portal_frame", 0
}

func (f EndPortalFrame) EncodeBlock() (string, map[string]any) {
	return "minecraft:end_portal_frame", map[string]any{
		"minecraft:cardinal_direction": f.Facing.String(),
		"end_portal_eye_bit":           f.Eye,
	}
}

func allEndPortalFrames() (b []world.Block) {
	for _, facing := range []cube.Direction{cube.North, cube.East, cube.South, cube.West} {
		b = append(b, EndPortalFrame{Facing: facing})
		b = append(b, EndPortalFrame{Facing: facing, Eye: true})
	}
	return
}
