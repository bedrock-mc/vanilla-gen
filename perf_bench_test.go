package vanilla

import (
	"os"
	"testing"
	"time"

	"github.com/df-mc/dragonfly/server/world"
	"github.com/df-mc/dragonfly/server/world/chunk"
)

func TestPerfColdArea(t *testing.T) {
	if os.Getenv("VANILLA_PERF") == "" {
		t.Skip("VANILLA_PERF not set")
	}
	g := New(1)
	r := g.dimension.Range()
	start := time.Now()
	n := 0
	for cx := 30; cx < 40; cx++ {
		for cz := 30; cz < 40; cz++ {
			c := chunk.New(world.DefaultBlockRegistry, r)
			g.GenerateChunk(world.ChunkPos{int32(cx), int32(cz)}, c)
			n++
		}
	}
	d := time.Since(start)
	t.Logf("%d chunks in %s (%.0f ms/chunk)", n, d, float64(d.Milliseconds())/float64(n))
}
