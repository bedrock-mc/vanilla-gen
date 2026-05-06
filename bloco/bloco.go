// Package bloco registers Dragonfly block states that do not yet have typed
// block implementations.
package bloco

import (
	"math"
	"sync"

	_ "github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
)

var registerOnce sync.Once

func init() {
	RegisterMissing()
}

// RegisterMissing registers generic implementations for every block state that
// Dragonfly knows about but has not registered as a typed block yet.
func RegisterMissing() {
	registerOnce.Do(func() {
		for _, b := range world.Blocks() {
			name, properties := b.EncodeBlock()
			if _, hash := b.Hash(); hash != math.MaxUint64 {
				continue
			}
			world.RegisterBlock(Block{Name: name, Properties: properties})
		}
	})
}

// Block is a generic implementation for a Dragonfly block state that is present
// in the runtime palette but missing a typed block implementation upstream.
type Block struct {
	Name       string
	Properties map[string]any
}

// EncodeBlock encodes the generic block state.
func (b Block) EncodeBlock() (string, map[string]any) {
	return b.Name, b.Properties
}

// Hash marks Block as runtime-state backed, so world.BlockRuntimeID resolves it
// by name and properties instead of a typed Dragonfly block hash.
func (Block) Hash() (uint64, uint64) {
	return 0, math.MaxUint64
}

// Model returns a non-solid placeholder model.
func (Block) Model() world.BlockModel {
	return model{}
}

type model struct{}

func (model) BBox(cube.Pos, world.BlockSource) []cube.BBox {
	return nil
}

func (model) FaceSolid(cube.Pos, cube.Face, world.BlockSource) bool {
	return false
}
