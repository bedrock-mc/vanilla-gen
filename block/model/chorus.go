package model

import (
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
)

// Chorus is a simplified model for chorus plants and flowers.
type Chorus struct{}

func (Chorus) BBox(cube.Pos, world.BlockSource) []cube.BBox {
	return []cube.BBox{cube.Box(0.1875, 0, 0.1875, 0.8125, 1, 0.8125)}
}

func (Chorus) FaceSolid(cube.Pos, cube.Face, world.BlockSource) bool {
	return false
}
