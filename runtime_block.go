package vanilla

import (
	"math"

	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
)

type runtimeBlock struct {
	name       string
	properties map[string]any
}

func (b runtimeBlock) EncodeBlock() (string, map[string]any) {
	return b.name, b.properties
}

func (runtimeBlock) Hash() (uint64, uint64) {
	return 0, math.MaxUint64
}

func (runtimeBlock) Model() world.BlockModel {
	return runtimeBlockModel{}
}

type runtimeBlockModel struct{}

func (runtimeBlockModel) BBox(cube.Pos, world.BlockSource) []cube.BBox {
	return nil
}

func (runtimeBlockModel) FaceSolid(cube.Pos, cube.Face, world.BlockSource) bool {
	return false
}
