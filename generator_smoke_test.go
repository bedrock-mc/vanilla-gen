package vanilla_test

import (
	"testing"

	vanilla "github.com/bedrock-mc/vanilla-gen"
	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/df-mc/dragonfly/server/world/chunk"
)

func TestGenerateChunkWithUpstreamDragonfly(t *testing.T) {
	g := vanilla.New(1)
	air := runtimeID(t, block.Air{})
	c := chunk.New(world.DefaultBlockRegistry, cube.Range{-64, 319})

	g.GenerateChunk(world.ChunkPos{0, 0}, c)

	if c.Block(0, 0, 0, 0) == air {
		t.Fatal("expected generated chunk to contain terrain at y=0")
	}
}

func runtimeID(t *testing.T, b world.Block) uint32 {
	t.Helper()
	name, properties := b.EncodeBlock()
	return world.BlockRuntimeID(testRuntimeBlock{name: name, properties: properties})
}

type testRuntimeBlock struct {
	name       string
	properties map[string]any
}

func (b testRuntimeBlock) EncodeBlock() (string, map[string]any) {
	return b.name, b.properties
}

func (testRuntimeBlock) Hash() (uint64, uint64) {
	return 0, ^uint64(0)
}

func (testRuntimeBlock) Model() world.BlockModel {
	return testRuntimeBlockModel{}
}

type testRuntimeBlockModel struct{}

func (testRuntimeBlockModel) BBox(cube.Pos, world.BlockSource) []cube.BBox {
	return nil
}

func (testRuntimeBlockModel) FaceSolid(cube.Pos, cube.Face, world.BlockSource) bool {
	return false
}
