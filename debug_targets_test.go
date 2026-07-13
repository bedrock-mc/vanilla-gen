package vanilla

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/bedrock-mc/vanilla-gen/parity"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/df-mc/dragonfly/server/world/chunk"
)

// TestDebugTargetPairs prints coordinates for selected java->local mismatch pairs.
func TestDebugTargetPairs(t *testing.T) {
	dir := os.Getenv("VANILLA_GT_REGION_DIR")
	if dir == "" {
		t.Skip("VANILLA_GT_REGION_DIR not set")
	}
	targets := map[string]bool{}
	for _, p := range strings.Split(os.Getenv("DEBUG_PAIRS"), ";") {
		if p != "" {
			targets[p] = true
		}
	}
	w, err := parity.OpenRegionDir(dir)
	if err != nil {
		t.Fatalf("open region dir: %v", err)
	}
	chunkRange := 2
	h := newChunkParityHarness(1)
	r := h.g.dimension.Range()
	count := 0
	for cx := -chunkRange; cx < chunkRange; cx++ {
		for cz := -chunkRange; cz < chunkRange; cz++ {
			jc, err := w.Chunk(cx, cz)
			if err != nil {
				t.Fatalf("chunk: %v", err)
			}
			c := chunk.New(world.DefaultBlockRegistry, r)
			h.g.GenerateChunk(world.ChunkPos{int32(cx), int32(cz)}, c)
			for x := 0; x < 16; x++ {
				for z := 0; z < 16; z++ {
					for y := r.Min(); y <= r.Max(); y++ {
						jname := jc.BlockState(x, y, z)
						if i := strings.IndexByte(jname, '['); i >= 0 {
							jname = jname[:i]
						}
						jname = strings.TrimPrefix(jname, "minecraft:")
						lname := h.g.carverBlockName(c.Block(uint8(x), int16(y), uint8(z), 0))
						if os.Getenv("DEBUG_SPAWNERS") != "" && (jname == "spawner" || lname == "mob_spawner") {
						fmt.Printf("spawner java=%s local=%s @ %d %d %d\n", jname, lname, cx*16+x, y, cz*16+z)
					}
					if !sameBlockName(jname, lname) {
							pair := jname + " -> " + lname
							if targets[pair] {
								fmt.Printf("%s @ %d %d %d\n", pair, cx*16+x, y, cz*16+z)
								count++
							}
						}
					}
				}
			}
		}
	}
	t.Logf("total printed: %d", count)
}
