package vanilla

// Temporary debug harness for aquifer parity work. DELETE BEFORE COMMIT.

import (
	"fmt"
	"os"
	"testing"

	"github.com/bedrock-mc/vanilla-gen/parity"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/df-mc/dragonfly/server/world/chunk"
)

func TestDebugAquiferColumn(t *testing.T) {
	dir := os.Getenv("VANILLA_GT_REGION_DIR")
	if dir == "" {
		t.Skip("VANILLA_GT_REGION_DIR not set")
	}
	w, err := parity.OpenRegionDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	h := newChunkParityHarness(1)
	r := h.g.dimension.Range()

	cx, cz := -2, -2
	jc, err := w.Chunk(cx, cz)
	if err != nil {
		t.Fatal(err)
	}

	// Base terrain only: no decoration.
	c := chunk.New(world.DefaultBlockRegistry, r)
	h.g.prepareChunkForDecoration(world.ChunkPos{int32(cx), int32(cz)}, c)

	// Full pipeline for comparison.
	cFull := chunk.New(world.DefaultBlockRegistry, r)
	h2 := newChunkParityHarness(1)
	h2.g.GenerateChunk(world.ChunkPos{int32(cx), int32(cz)}, cFull)

	lx, lz := 0, 0 // block -32, -32
	fmt.Println("y   java                          base  full")
	for y := 20; y <= 50; y++ {
		jname := jc.BlockState(lx, y, lz)
		base := h.classifyLocal(c, lx, y, lz)
		full := h2.classifyLocal(cFull, lx, y, lz)
		mark := ""
		if classifyJava(jname) != full {
			mark = "  <-- full diff"
		}
		if classifyJava(jname) != base {
			mark += "  <-- base diff"
		}
		fmt.Printf("%3d %-30s %d     %d%s\n", y, jname, base, full, mark)
	}

	// Count base-terrain-only mismatches for the whole test area.
	mismatch := map[string]int{}
	byY := map[int]int{}
	for ccx := -2; ccx < 2; ccx++ {
		for ccz := -2; ccz < 2; ccz++ {
			jcc, err := w.Chunk(ccx, ccz)
			if err != nil {
				t.Fatal(err)
			}
			hh := newChunkParityHarness(1)
			cc := chunk.New(world.DefaultBlockRegistry, r)
			hh.g.prepareChunkForDecoration(world.ChunkPos{int32(ccx), int32(ccz)}, cc)
			for x := 0; x < 16; x++ {
				for z := 0; z < 16; z++ {
					for y := r.Min(); y <= r.Max(); y++ {
						want := classifyJava(jcc.BlockState(x, y, z))
						got := hh.classifyLocal(cc, x, y, z)
						if got != want {
							mismatch[fmt.Sprintf("%d->%d", want, got)]++
							byY[y]++
						}
					}
				}
			}
		}
	}
	fmt.Println("base-terrain-only mismatch classes:", mismatch)
	fmt.Println("byY sample:", byY)

	// Print fluid-class mismatches (water/lava involved) below the surface.
	printed := 0
	for ccx := -2; ccx < 2; ccx++ {
		for ccz := -2; ccz < 2; ccz++ {
			jcc, err := w.Chunk(ccx, ccz)
			if err != nil {
				t.Fatal(err)
			}
			hh := newChunkParityHarness(1)
			cc := chunk.New(world.DefaultBlockRegistry, r)
			hh.g.prepareChunkForDecoration(world.ChunkPos{int32(ccx), int32(ccz)}, cc)
			for x := 0; x < 16; x++ {
				for z := 0; z < 16; z++ {
					for y := r.Min(); y <= 30; y++ {
						want := classifyJava(jcc.BlockState(x, y, z))
						got := hh.classifyLocal(cc, x, y, z)
						if got != want && (want == matWater || want == matLava || got == matWater || got == matLava) && printed < 60 {
							fmt.Printf("mismatch %d->%d at (%d,%d,%d) java=%s\n",
								want, got, ccx*16+x, y, ccz*16+z, jcc.BlockState(x, y, z))
							printed++
						}
					}
				}
			}
		}
	}
}
