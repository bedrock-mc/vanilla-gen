package gen

import (
	"errors"
	"math"
	"os"
	"testing"
)

func testOpenCLAccelerator(t testing.TB, seed int64) ComputeAccelerator {
	t.Helper()
	accelerator, err := NewOpenCLAccelerator(NewNoiseRegistry(seed), OpenCLConfig{
		LibraryPath: os.Getenv("VANILLA_GEN_OPENCL_LIBRARY"),
	})
	if err != nil {
		if os.Getenv("VANILLA_GEN_REQUIRE_GPU") == "1" || !errors.Is(err, ErrAcceleratorUnavailable) {
			t.Fatal(err)
		}
		t.Skipf("OpenCL GPU unavailable: %v", err)
	}
	t.Cleanup(func() { _ = accelerator.Close() })
	return accelerator
}

func openCLBenchmarkPoints() []FunctionContext {
	points := make([]FunctionContext, 0, 12*12*97)
	for x := -16; x <= 28; x += 4 {
		for z := -16; z <= 28; z += 4 {
			for y := -64; y <= 320; y += 4 {
				points = append(points, FunctionContext{BlockX: x, BlockY: y, BlockZ: z})
			}
		}
	}
	return points
}

func TestOpenCLFinalDensityParity(t *testing.T) {
	const seed = int64(1)
	noises := NewNoiseRegistry(seed)
	accelerator := testOpenCLAccelerator(t, seed)
	for _, position := range [][2]int{{0, 0}, {17, -31}, {-1000, 2048}} {
		chunkX, chunkZ := position[0], position[1]
		flat := OverworldGraph.NewFlatCacheGrid(chunkX, chunkZ, noises)
		cpu := NewFinalDensityChunk(OverworldGraph, chunkX, chunkZ, -64, 319, noises, flat)
		gpu, err := accelerator.FinalDensity(chunkX, chunkZ, -64, 319, flat)
		if err != nil {
			t.Fatalf("chunk %v: %v", position, err)
		}
		maxDelta := 0.0
		for x := 0; x <= 4; x++ {
			for z := 0; z <= 4; z++ {
				for y := 0; y <= cpu.cellCountY; y++ {
					want, got := cpu.corners[x][z][y], gpu.corners[x][z][y]
					delta := math.Abs(want - got)
					maxDelta = max(maxDelta, delta)
					if delta > 1e-12 {
						t.Fatalf("chunk %v corner (%d,%d,%d): gpu=%0.17g cpu=%0.17g delta=%g", position, x, y, z, got, want, delta)
					}
				}
			}
		}
		t.Logf("chunk %v maximum density delta %.3g", position, maxDelta)
	}
}

func TestOpenCLClimateParity(t *testing.T) {
	const seed = int64(1)
	accelerator := testOpenCLAccelerator(t, seed)
	source, err := NewBiomeSource(seed, NewWorldgenRegistry(), "overworld")
	if err != nil {
		t.Fatal(err)
	}
	points := make([]FunctionContext, 0, 4096)
	for x := -128; x < 128; x += 8 {
		for z := -128; z < 128; z += 8 {
			for y := -64; y <= 320; y += 64 {
				points = append(points, FunctionContext{BlockX: x, BlockY: y, BlockZ: z})
			}
		}
	}
	got := make([][6]int64, len(points))
	if err := accelerator.SampleClimate(points, got); err != nil {
		t.Fatal(err)
	}
	for i, point := range points {
		want := source.SampleClimate(point.BlockX, point.BlockY, point.BlockZ)
		if got[i] != want {
			t.Fatalf("point %+v: gpu=%v cpu=%v", point, got[i], want)
		}
	}
}

func TestOpenCLBiomeParity(t *testing.T) {
	const seed = int64(1)
	accelerator := testOpenCLAccelerator(t, seed)
	source, err := NewBiomeSource(seed, NewWorldgenRegistry(), "overworld")
	if err != nil {
		t.Fatal(err)
	}
	points := openCLBenchmarkPoints()
	got := make([]Biome, len(points))
	if err := accelerator.SampleBiomes(points, got); err != nil {
		t.Fatal(err)
	}
	for i, point := range points {
		want := source.GetBiome(point.BlockX, point.BlockY, point.BlockZ)
		if got[i] != want {
			t.Fatalf("point %+v: gpu=%v cpu=%v", point, got[i], want)
		}
	}
}

func BenchmarkBiomeBatchCPU(b *testing.B) {
	source, err := NewBiomeSource(1, NewWorldgenRegistry(), "overworld")
	if err != nil {
		b.Fatal(err)
	}
	preset := source.(*presetBiomeSource)
	points := openCLBenchmarkPoints()
	dst := make([]Biome, len(points))
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		for i, point := range points {
			dst[i] = preset.rtree.Lookup(preset.SampleClimate(point.BlockX, point.BlockY, point.BlockZ))
		}
	}
}

func BenchmarkBiomeBatchGPU(b *testing.B) {
	accelerator := testOpenCLAccelerator(b, 1)
	points := openCLBenchmarkPoints()
	dst := make([]Biome, len(points))
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if err := accelerator.SampleBiomes(points, dst); err != nil {
			b.Fatal(err)
		}
	}
}
