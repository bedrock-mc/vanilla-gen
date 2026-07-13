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
	if os.Getenv("DEBUG_FULL") != "" {
		chunkRange = 8
	}
	h := newChunkParityHarness(parityWorldSeed(t))
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

// TestDebugSurface compares the surface specifically: per-column top height,
// top block, and the band of blocks around the java surface.
func TestDebugSurface(t *testing.T) {
	dir := os.Getenv("VANILLA_GT_REGION_DIR")
	if dir == "" {
		t.Skip("VANILLA_GT_REGION_DIR not set")
	}
	w, err := parity.OpenRegionDir(dir)
	if err != nil {
		t.Fatalf("open region dir: %v", err)
	}
	h := newChunkParityHarness(parityWorldSeed(t))
	r := h.g.dimension.Range()
	isAir := func(n string) bool { return n == "air" || n == "cave_air" || n == "void_air" }
	norm := func(n string) string {
		if i := strings.IndexByte(n, '['); i >= 0 {
			n = n[:i]
		}
		return strings.TrimPrefix(n, "minecraft:")
	}
	columns, heightDiff, topDiff, bandBad, bandTotal := 0, 0, 0, 0, 0
	topPairs := map[string]int{}
	bandPairs := map[string]int{}
	chunkRange := 2
	if os.Getenv("DEBUG_FULL") != "" {
		chunkRange = 8
	}
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
					columns++
					jy, ly := r.Min()-1, r.Min()-1
					var jtop, ltop string
					for y := r.Max(); y >= r.Min(); y-- {
						if jy < r.Min() {
							if n := norm(jc.BlockState(x, y, z)); !isAir(n) {
								jy, jtop = y, n
							}
						}
						if ly < r.Min() {
							if n := h.g.carverBlockName(c.Block(uint8(x), int16(y), uint8(z), 0)); !isAir(n) {
								ly, ltop = y, n
							}
						}
						if jy >= r.Min() && ly >= r.Min() {
							break
						}
					}
					if jy != ly {
						heightDiff++
						if heightDiff <= 12 {
							fmt.Printf("height diff @ %d %d: java=%d(%s) local=%d(%s)\n", cx*16+x, cz*16+z, jy, jtop, ly, ltop)
						}
					}
					if !sameBlockName(jtop, ltop) {
						topDiff++
						topPairs[jtop+" -> "+ltop]++
					}
					// Compare the band surface-4 .. surface+1 around java's surface.
					for y := jy - 4; y <= jy+1; y++ {
						if y < r.Min() || y > r.Max() {
							continue
						}
						bandTotal++
						jn := norm(jc.BlockState(x, y, z))
						ln := h.g.carverBlockName(c.Block(uint8(x), int16(y), uint8(z), 0))
						if !sameBlockName(jn, ln) {
							bandBad++
							bandPairs[jn+" -> "+ln]++
						}
					}
				}
			}
		}
	}
	fmt.Printf("columns=%d heightDiff=%d topBlockDiff=%d band(bad/total)=%d/%d\n", columns, heightDiff, topDiff, bandBad, bandTotal)
	for p, n := range topPairs {
		fmt.Printf("top: %4d  %s\n", n, p)
	}
	for p, n := range bandPairs {
		fmt.Printf("band: %4d  %s\n", n, p)
	}
}

// TestDebugTreePairs counts every mismatch pair involving tree blocks across
// the full ground-truth range.
func TestDebugTreePairs(t *testing.T) {
	dir := os.Getenv("VANILLA_GT_REGION_DIR")
	if dir == "" {
		t.Skip("VANILLA_GT_REGION_DIR not set")
	}
	w, err := parity.OpenRegionDir(dir)
	if err != nil {
		t.Fatalf("open region dir: %v", err)
	}
	h := newChunkParityHarness(parityWorldSeed(t))
	r := h.g.dimension.Range()
	treeish := func(n string) bool {
		return strings.Contains(n, "log") || strings.Contains(n, "leaves") || strings.Contains(n, "sapling") ||
			strings.Contains(n, "birch") || strings.Contains(n, "bee_nest") || strings.Contains(n, "mushroom_block") ||
			strings.Contains(n, "mushroom_stem")
	}
	pairs := map[string]int{}
	treeBlocks := 0
	for cx := -8; cx < 8; cx++ {
		for cz := -8; cz < 8; cz++ {
			jc, err := w.Chunk(cx, cz)
			if err != nil {
				t.Fatalf("chunk: %v", err)
			}
			c := chunk.New(world.DefaultBlockRegistry, r)
			h.g.GenerateChunk(world.ChunkPos{int32(cx), int32(cz)}, c)
			for x := 0; x < 16; x++ {
				for z := 0; z < 16; z++ {
					for y := 40; y <= 200; y++ {
						jname := jc.BlockState(x, y, z)
						if i := strings.IndexByte(jname, '['); i >= 0 {
							jname = jname[:i]
						}
						jname = strings.TrimPrefix(jname, "minecraft:")
						lname := h.g.carverBlockName(c.Block(uint8(x), int16(y), uint8(z), 0))
						if treeish(jname) {
							treeBlocks++
						}
						if (treeish(jname) || treeish(lname)) && !sameBlockName(jname, lname) {
							pairs[jname+" -> "+lname]++
						}
					}
				}
			}
		}
	}
	fmt.Printf("java tree-ish blocks in range: %d, mismatched pairs involving trees:\n", treeBlocks)
	if len(pairs) == 0 {
		fmt.Println("  NONE - all trees identical")
	}
	for p, n := range pairs {
		fmt.Printf("  %5d  %s\n", n, p)
	}
}
