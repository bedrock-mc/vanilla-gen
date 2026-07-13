package vanilla

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/bedrock-mc/vanilla-gen/parity"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/df-mc/dragonfly/server/world/chunk"
)

// TestDebugBiomes compares java stored biomes vs local biome source at quart
// resolution. DEBUG_BIOME_BOX="qx0,qy,qz0,qx1,qz1" (quart coords).
func TestDebugBiomes(t *testing.T) {
	dir := os.Getenv("VANILLA_GT_REGION_DIR")
	spec := os.Getenv("DEBUG_BIOME_BOX")
	if dir == "" || spec == "" {
		t.Skip("VANILLA_GT_REGION_DIR / DEBUG_BIOME_BOX not set")
	}
	var v [5]int
	for i, part := range strings.Split(spec, ",") {
		n, _ := strconv.Atoi(strings.TrimSpace(part))
		v[i] = n
	}
	w, err := parity.OpenRegionDir(dir)
	if err != nil {
		t.Fatalf("open region dir: %v", err)
	}
	h := newChunkParityHarness(parityWorldSeed(t))
	bad := 0
	for qx := v[0]; qx <= v[3]; qx++ {
		for qz := v[2]; qz <= v[4]; qz++ {
			cx, cz := floorDiv(qx, 4), floorDiv(qz, 4)
			jc, err := w.Chunk(cx, cz)
			if err != nil {
				t.Fatalf("chunk: %v", err)
			}
			jb := strings.TrimPrefix(jc.Biome(qx&3, v[1], qz&3), "minecraft:")
			lb := biomeKey(h.g.biomeSource.GetBiome(qx<<2, v[1]<<2, qz<<2))
			mark := "  "
			if jb != strings.TrimPrefix(lb, "minecraft:") {
				mark = "!!"
				bad++
			}
			fmt.Printf("%s q %4d %3d %4d java=%-30s local=%s\n", mark, qx, v[1], qz, jb, lb)
		}
	}
	t.Logf("bad=%d", bad)
}

// TestDebugBox prints java vs local blocks in a box. DEBUG_BOX="x0,y0,z0,x1,y1,z1"
func TestDebugBox(t *testing.T) {
	dir := os.Getenv("VANILLA_GT_REGION_DIR")
	spec := os.Getenv("DEBUG_BOX")
	if dir == "" || spec == "" {
		t.Skip("VANILLA_GT_REGION_DIR / DEBUG_BOX not set")
	}
	var v [6]int
	for i, part := range strings.Split(spec, ",") {
		n, _ := strconv.Atoi(strings.TrimSpace(part))
		v[i] = n
	}
	w, err := parity.OpenRegionDir(dir)
	if err != nil {
		t.Fatalf("open region dir: %v", err)
	}
	h := newChunkParityHarness(parityWorldSeed(t))
	r := h.g.dimension.Range()
	chunks := map[[2]int]*chunk.Chunk{}
	for x := v[0]; x <= v[3]; x++ {
		for z := v[2]; z <= v[5]; z++ {
			cx, cz := floorDiv(x, 16), floorDiv(z, 16)
			key := [2]int{cx, cz}
			c, ok := chunks[key]
			if !ok {
				c = chunk.New(world.DefaultBlockRegistry, r)
				h.g.GenerateChunk(world.ChunkPos{int32(cx), int32(cz)}, c)
				chunks[key] = c
			}
			jc, err := w.Chunk(cx, cz)
			if err != nil {
				t.Fatalf("chunk: %v", err)
			}
			for y := v[1]; y <= v[4]; y++ {
				jname := jc.BlockState(x&15, y, z&15)
				jfull := jname
				if i := strings.IndexByte(jname, '['); i >= 0 {
					jname = jname[:i]
				}
				jname = strings.TrimPrefix(jname, "minecraft:")
				lname := h.g.carverBlockName(c.Block(uint8(x&15), int16(y), uint8(z&15), 0))
				mark := "  "
				if !sameBlockName(jname, lname) {
					mark = "!!"
				}
				if jname == "air" && lname == "air" {
					continue
				}
				fmt.Printf("%s %4d %3d %4d  java=%-40s local=%s\n", mark, x, y, z, jfull, lname)
			}
		}
	}
}
