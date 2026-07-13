package vanilla

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/bedrock-mc/vanilla-gen/parity"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/df-mc/dragonfly/server/world/chunk"
)

// Reduced block alphabet: terrain, carvers and aquifers fully determine the
// air/water/lava/solid distribution below the surface, so mismatches in this
// space localize base-terrain deviations without needing a full Bedrock->Java
// block state mapping. Decoration adds some legitimate diffs (vegetation,
// dripstone, geode interiors) until the feature pipeline is exact.
const (
	matAir = iota
	matWater
	matLava
	matSolid
)

func classifyJava(state string) int {
	name := state
	if i := strings.IndexByte(state, '['); i >= 0 {
		name = state[:i]
	}
	switch name {
	case "minecraft:air", "minecraft:cave_air", "minecraft:void_air":
		return matAir
	case "minecraft:water":
		return matWater
	case "minecraft:lava":
		return matLava
	default:
		return matSolid
	}
}

type chunkParityHarness struct {
	g        Generator
	airRID   uint32
	waterRID map[uint32]bool
	lavaRID  map[uint32]bool
}

func newChunkParityHarness(seed int64) *chunkParityHarness {
	g := New(seed)
	h := &chunkParityHarness{
		g:        g,
		airRID:   g.airRID,
		waterRID: map[uint32]bool{},
		lavaRID:  map[uint32]bool{},
	}
	return h
}

func (h *chunkParityHarness) classifyLocal(c *chunk.Chunk, x int, y int, z int) int {
	rid := c.Block(uint8(x), int16(y), uint8(z), 0)
	if rid == h.airRID {
		return matAir
	}
	if h.waterRID[rid] {
		return matWater
	}
	if h.lavaRID[rid] {
		return matLava
	}
	name := h.g.carverBlockName(rid)
	switch name {
	case "water":
		h.waterRID[rid] = true
		return matWater
	case "lava":
		h.lavaRID[rid] = true
		return matLava
	case "air":
		return matAir
	}
	return matSolid
}

// TestChunkParityBaseTerrain diffs generated chunks against the Java 1.21.11
// ground-truth world (seed 1) in the reduced material alphabet, and compares
// stored biomes at quart resolution.
func TestChunkParityBaseTerrain(t *testing.T) {
	dir := os.Getenv("VANILLA_GT_REGION_DIR")
	if dir == "" {
		t.Skip("VANILLA_GT_REGION_DIR not set")
	}
	w, err := parity.OpenRegionDir(dir)
	if err != nil {
		t.Fatalf("open region dir: %v", err)
	}

	chunkRange := 2 // chunks -2..1 by default; VANILLA_GT_FULL=1 for -8..7
	if os.Getenv("VANILLA_GT_FULL") != "" {
		chunkRange = 8
	}

	h := newChunkParityHarness(1)
	r := h.g.dimension.Range()

	totalBlocks := 0
	mismatch := map[string]int{}
	mismatchByY := map[int]int{}
	biomeTotal, biomeBad := 0, 0
	firstDiff := ""

	for cx := -chunkRange; cx < chunkRange; cx++ {
		for cz := -chunkRange; cz < chunkRange; cz++ {
			jc, err := w.Chunk(cx, cz)
			if err != nil {
				t.Fatalf("ground truth chunk %d,%d: %v", cx, cz, err)
			}
			c := chunk.New(world.DefaultBlockRegistry, r)
			h.g.GenerateChunk(world.ChunkPos{int32(cx), int32(cz)}, c)

			for x := 0; x < 16; x++ {
				for z := 0; z < 16; z++ {
					for y := r.Min(); y <= r.Max(); y++ {
						want := classifyJava(jc.BlockState(x, y, z))
						got := h.classifyLocal(c, x, y, z)
						totalBlocks++
						if got != want {
							key := fmt.Sprintf("%d->%d", want, got)
							mismatch[key]++
							mismatchByY[y]++
							if firstDiff == "" {
								firstDiff = fmt.Sprintf("chunk %d,%d block (%d,%d,%d): java %s, local material %d",
									cx, cz, cx*16+x, y, cz*16+z, jc.BlockState(x, y, z), got)
							}
						}
					}
				}
			}

			for qx := 0; qx < 4; qx++ {
				for qz := 0; qz < 4; qz++ {
					for qy := r.Min() >> 2; qy <= r.Max()>>2; qy++ {
						want := strings.TrimPrefix(jc.Biome(qx, qy, qz), "minecraft:")
						switch want {
						case "snowy_taiga":
							want = "cold_taiga"
						case "snowy_plains":
							want = "ice_plains"
						case "stony_shore":
							want = "stone_beach"
						}
						got := biomeKey(h.g.biomeAt(c, qx*4, qy*4, qz*4))
						biomeTotal++
						if got != want {
							biomeBad++
							if biomeBad == 1 {
								t.Logf("first biome diff at chunk %d,%d quart (%d,%d,%d): got %s want %s", cx, cz, qx, qy, qz, got, want)
							}
						}
					}
				}
			}
		}
	}

	bad := 0
	for _, v := range mismatch {
		bad += v
	}
	t.Logf("blocks: %d/%d mismatched (%.4f%%) by class(want->got, 0=air 1=water 2=lava 3=solid): %v",
		bad, totalBlocks, 100*float64(bad)/float64(totalBlocks), mismatch)
	if firstDiff != "" {
		t.Logf("first diff: %s", firstDiff)
	}
	type yc struct{ y, n int }
	var ys []yc
	for y, n := range mismatchByY {
		ys = append(ys, yc{y, n})
	}
	sort.Slice(ys, func(a, b int) bool { return ys[a].n > ys[b].n })
	if len(ys) > 14 {
		ys = ys[:14]
	}
	t.Logf("top mismatch y-levels: %v", ys)
	t.Logf("biomes: %d/%d mismatched", biomeBad, biomeTotal)
	if biomeBad > 0 {
		t.Errorf("biome mismatches: %d", biomeBad)
	}
	// Base terrain target: no threshold yet; log-only until decoration is
	// exact, but fail hard if terrain is grossly off.
	if float64(bad)/float64(totalBlocks) > 0.10 {
		t.Errorf("base terrain mismatch rate too high: %d/%d", bad, totalBlocks)
	}
}
