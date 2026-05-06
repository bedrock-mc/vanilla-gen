// Package block registers vanilla block implementations that are needed by the
// generator but are not available as typed blocks in upstream Dragonfly yet.
package block

import (
	"math"

	_ "github.com/df-mc/dragonfly/server/block"
	dfcube "github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
)

func init() {
	Register()
}

// Register registers all missing block states. States already implemented by
// upstream Dragonfly are left alone.
func Register() {
	for _, b := range world.Blocks() {
		name, properties := b.EncodeBlock()
		if _, hash := b.Hash(); hash != math.MaxUint64 {
			continue
		}
		world.RegisterBlock(StateBlock{Name: name, Properties: properties})
	}
}

// StateBlock is the registered implementation used for missing block states.
type StateBlock struct {
	base
	Name       string
	Properties map[string]any
}

func (b StateBlock) EncodeBlock() (string, map[string]any) {
	return b.Name, b.Properties
}

type base struct{}

func (base) Hash() (uint64, uint64) { return 0, math.MaxUint64 }

func (base) Model() world.BlockModel { return model{} }

type model struct{}

func (model) BBox(dfcube.Pos, world.BlockSource) []dfcube.BBox { return nil }

func (model) FaceSolid(dfcube.Pos, dfcube.Face, world.BlockSource) bool { return false }

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
