package gen

// Temporary debug for aquifer parity. DELETE BEFORE COMMIT.

import (
	"fmt"
	"math"
	"os"
	"testing"
)

// TestDebugAquiferProbeParity diffs ComputeSubstance(ctx, 0.0) for every block
// in chunks [-2,2)x[-2,2), y in [-64,100], against the vanilla dump produced by
// driver/AquiferProbe.java.
func TestDebugAquiferProbeParity(t *testing.T) {
	path := os.Getenv("AQUIFER_PROBE_FIXTURE")
	if path == "" {
		t.Skip("AQUIFER_PROBE_FIXTURE not set")
	}
	density := 0.0
	if d := os.Getenv("AQUIFER_PROBE_DENSITY"); d != "" {
		fmt.Sscanf(d, "%f", &density)
	}
	realDensity := os.Getenv("AQUIFER_PROBE_REAL_DENSITY") != ""
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := make([]string, 0, 4096)
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, string(data[start:i]))
			start = i + 1
		}
	}

	chunkRange := 2
	yMax := 100
	if os.Getenv("AQUIFER_PROBE_BIG") != "" {
		chunkRange = 8
		yMax = 319
	}
	seed := int64(1)
	noises := NewNoiseRegistry(seed)
	idx := 0
	total := 0
	mismatches := 0
	classes := map[string]int{}
	for cx := -chunkRange; cx < chunkRange; cx++ {
		for cz := -chunkRange; cz < chunkRange; cz++ {
			flat := OverworldGraph.NewFlatCacheGrid(cx, cz, noises)
			a := NewNoiseBasedAquifer(OverworldGraph, cx, cz, -64, 319, noises, flat, seed, OverworldFluidPicker{SeaLevel: 63})
			for lx := 0; lx < 16; lx++ {
				for lz := 0; lz < 16; lz++ {
					line := lines[idx]
					idx++
					for y := -64; y <= yMax; y++ {
						total++
						want := line[y+64]
						ctx := FunctionContext{BlockX: cx*16 + lx, BlockY: y, BlockZ: cz*16 + lz}
						d := density
						if realDensity {
							d = OverworldGraph.Eval(OverworldRootFinalDensity, ctx, noises, nil, nil, nil)
						}
						got := byte('0') + byte(a.ComputeSubstance(ctx, d))
						if got != want {
							mismatches++
							classes[fmt.Sprintf("%c->%c", want, got)]++
							if mismatches <= 20 {
								t.Logf("aquifer diff at (%d,%d,%d): java %c, go %c", cx*16+lx, y, cz*16+lz, want, got)
							}
						}
					}
				}
			}
		}
	}
	t.Logf("aquifer probe mismatches: %d / %d, classes %v", mismatches, total, classes)
}

func TestDebugAquiferInternals(t *testing.T) {
	seed := int64(1)
	noises := NewNoiseRegistry(seed)
	chunkX, chunkZ := -2, -1
	flat := OverworldGraph.NewFlatCacheGrid(chunkX, chunkZ, noises)
	a := NewNoiseBasedAquifer(OverworldGraph, chunkX, chunkZ, -64, 319, noises, flat, seed, OverworldFluidPicker{SeaLevel: 63})
	dc := NewFinalDensityChunkWithEvaluator(OverworldGraph, OverworldRootFinalDensity, chunkX, chunkZ, -64, 319, noises, flat, ComputeFinalDensity, ComputeFinalDensity4)

	fmt.Println("skipSamplingAboveY:", a.skipSamplingAboveY)

	blocks := [][3]int{
		{-24, -1, -16},
	}
	for _, b := range blocks {
		ctx := FunctionContext{BlockX: b[0], BlockY: b[1], BlockZ: b[2]}
		density := dc.Density(b[0]-chunkX*16, b[1], b[2]-chunkZ*16)
		sub := a.ComputeSubstance(ctx, density)
		fmt.Printf("\nblock %v density=%v -> substance %d (0=air 1=water 2=lava 3=barrier)\n", b, density, sub)

		gx := aquiferGridX(ctx.BlockX + aquiferSampleOffsetX)
		gy := aquiferGridY(ctx.BlockY + aquiferSampleOffsetY)
		gz := aquiferGridZ(ctx.BlockZ + aquiferSampleOffsetZ)
		var keys [3]aquiferCellKey
		dists := [3]int{math.MaxInt32, math.MaxInt32, math.MaxInt32}
		for dx := 0; dx <= 1; dx++ {
			for dy := -1; dy <= 1; dy++ {
				for dz := 0; dz <= 1; dz++ {
					key := aquiferCellKey{x: gx + dx, y: gy + dy, z: gz + dz}
					pos := a.getAquiferLocation(key)
					d := squaredDistance(pos.x-ctx.BlockX, pos.y-ctx.BlockY, pos.z-ctx.BlockZ)
					insertAquiferNearest(&keys, &dists, key, d)
				}
			}
		}
		for i := 0; i < 3; i++ {
			pos := a.getAquiferLocation(keys[i])
			st := a.getAquiferStatus(keys[i])
			fmt.Printf("  closest%d cell=%v jitter=(%d,%d,%d) distSq=%d status={level:%d type:%d}\n",
				i+1, keys[i], pos.x, pos.y, pos.z, dists[i], st.FluidLevel, st.FluidType)
		}
		sim12 := aquiferSimilarity(dists[0], dists[1])
		sim13 := aquiferSimilarity(dists[0], dists[2])
		sim23 := aquiferSimilarity(dists[1], dists[2])
		fmt.Printf("  sim12=%v sim13=%v sim23=%v\n", sim12, sim13, sim23)
		bn := math.NaN()
		f1 := a.getAquiferStatus(keys[0])
		f2 := a.getAquiferStatus(keys[1])
		f3 := a.getAquiferStatus(keys[2])
		p12 := a.calculatePressure(ctx, &bn, f1, f2)
		p13 := a.calculatePressure(ctx, &bn, f1, f3)
		p23 := a.calculatePressure(ctx, &bn, f2, f3)
		fmt.Printf("  pressure12=%v pressure13=%v pressure23=%v barrierNoise=%v\n", p12, p13, p23, bn)
	}
}
