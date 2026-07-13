package gen

// Temporary debug for aquifer parity. DELETE BEFORE COMMIT.

import (
	"fmt"
	"math"
	"testing"
)

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
