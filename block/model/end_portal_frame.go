package model

import (
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
)

// EndPortalFrame is the model used by end portal frame blocks.
type EndPortalFrame struct {
	Eye bool
}

func (m EndPortalFrame) BBox(cube.Pos, world.BlockSource) []cube.BBox {
	maxY := 0.8125
	if m.Eye {
		maxY = 1
	}
	return []cube.BBox{cube.Box(0, 0, 0, 1, maxY, 1)}
}

func (EndPortalFrame) FaceSolid(cube.Pos, cube.Face, world.BlockSource) bool {
	return true
}
