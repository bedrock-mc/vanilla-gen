package vanilla

import (
	"testing"

	"github.com/df-mc/dragonfly/server/world"
	"github.com/df-mc/dragonfly/server/world/chunk"
)

func BenchmarkGenerateChunks(b *testing.B) {
	g := New(1)
	r := g.dimension.Range()
	b.ResetTimer()
	n := 0
	for i := 0; i < b.N; i++ {
		cx := n % 8
		cz := (n / 8) % 8
		n++
		c := chunk.New(world.DefaultBlockRegistry, r)
		g.GenerateChunk(world.ChunkPos{int32(cx), int32(cz)}, c)
	}
}
