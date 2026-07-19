package vanilla

import (
	"errors"
	"os"
	"sync"
	"testing"

	gen "github.com/bedrock-mc/vanilla-gen/gen"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/df-mc/dragonfly/server/world/chunk"
)

func gpuTestConfig() GeneratorConfig {
	return GeneratorConfig{Acceleration: AccelerationConfig{
		Mode: AccelerationOpenCL,
		OpenCL: gen.OpenCLConfig{
			LibraryPath: os.Getenv("VANILLA_GEN_OPENCL_LIBRARY"),
		},
	}}
}

func newGPUGeneratorForTest(t testing.TB, seed int64) Generator {
	t.Helper()
	g, err := NewWithConfig(seed, gpuTestConfig())
	if err != nil {
		if os.Getenv("VANILLA_GEN_REQUIRE_GPU") == "1" || !errors.Is(err, gen.ErrAcceleratorUnavailable) {
			t.Fatal(err)
		}
		t.Skipf("OpenCL GPU unavailable: %v", err)
	}
	t.Cleanup(func() { _ = g.Close() })
	return g
}

func TestOpenCLGeneratedChunkParity(t *testing.T) {
	const seed = int64(1)
	cpu := New(seed)
	gpu := newGPUGeneratorForTest(t, seed)
	status := gpu.AccelerationStatus()
	if !status.Active || status.Backend != "opencl" {
		t.Fatalf("unexpected acceleration status: %+v", status)
	}
	for _, pos := range []world.ChunkPos{{0, 0}, {17, -31}, {-127, 255}} {
		cpuChunk := chunk.New(world.DefaultBlockRegistry, cube.Range{-64, 319})
		gpuChunk := chunk.New(world.DefaultBlockRegistry, cube.Range{-64, 319})
		cpu.GenerateChunk(pos, cpuChunk)
		gpu.GenerateChunk(pos, gpuChunk)
		assertChunkBlocksAndBiomesEqual(t, pos, cpuChunk, gpuChunk)
	}
}

func TestOpenCLConcurrentGenerationParity(t *testing.T) {
	const seed = int64(1)
	positions := []world.ChunkPos{{-256, -256}, {-64, 192}, {128, -96}, {320, 256}}
	gpu := newGPUGeneratorForTest(t, seed)
	cpu := New(seed)
	want := make([]*chunk.Chunk, len(positions))
	for i, pos := range positions {
		want[i] = chunk.New(world.DefaultBlockRegistry, cube.Range{-64, 319})
		cpu.GenerateChunk(pos, want[i])
	}

	got := make([]*chunk.Chunk, len(positions))
	var workers sync.WaitGroup
	workers.Add(len(positions))
	for i, pos := range positions {
		go func() {
			defer workers.Done()
			got[i] = chunk.New(world.DefaultBlockRegistry, cube.Range{-64, 319})
			gpu.GenerateChunk(pos, got[i])
		}()
	}
	workers.Wait()
	for i, pos := range positions {
		assertChunkBlocksAndBiomesEqual(t, pos, want[i], got[i])
	}
}

func TestOpenCLAdjacentGenerationParity(t *testing.T) {
	const seed = int64(1)
	cpu := New(seed)
	gpu := newGPUGeneratorForTest(t, seed)
	for _, pos := range []world.ChunkPos{{-1, -1}, {0, -1}, {-1, 0}, {0, 0}} {
		cpuChunk := chunk.New(world.DefaultBlockRegistry, cube.Range{-64, 319})
		gpuChunk := chunk.New(world.DefaultBlockRegistry, cube.Range{-64, 319})
		cpu.GenerateChunk(pos, cpuChunk)
		gpu.GenerateChunk(pos, gpuChunk)
		assertChunkBlocksAndBiomesEqual(t, pos, cpuChunk, gpuChunk)
	}
}

func TestAccelerationConfiguration(t *testing.T) {
	if _, err := NewWithConfig(1, GeneratorConfig{Acceleration: AccelerationConfig{Mode: "invalid"}}); err == nil {
		t.Fatal("expected invalid acceleration mode to fail")
	}
	g, err := NewWithConfig(1, GeneratorConfig{Acceleration: AccelerationConfig{Mode: AccelerationCPU}})
	if err != nil {
		t.Fatal(err)
	}
	if status := g.AccelerationStatus(); status.Active || status.Backend != "cpu" {
		t.Fatalf("unexpected CPU status: %+v", status)
	}

	auto, err := NewWithConfig(1, GeneratorConfig{Acceleration: AccelerationConfig{
		Mode: AccelerationAuto,
		OpenCL: gen.OpenCLConfig{
			PlatformIndex: 1 << 30,
		},
	}})
	if err != nil {
		t.Fatalf("automatic acceleration must fall back: %v", err)
	}
	if status := auto.AccelerationStatus(); status.Active || status.Backend != "cpu" || status.Fallback == "" {
		t.Fatalf("unexpected automatic fallback status: %+v", status)
	}

	_, err = NewWithConfig(1, GeneratorConfig{Acceleration: AccelerationConfig{
		Mode: AccelerationOpenCL,
		OpenCL: OpenCLConfig{
			PlatformIndex: 1 << 30,
		},
	}})
	if err == nil || !IsAcceleratorUnavailable(err) {
		t.Fatalf("required acceleration should report an unavailable device, got %v", err)
	}
}

func assertChunkBlocksAndBiomesEqual(t *testing.T, pos world.ChunkPos, want, got *chunk.Chunk) {
	t.Helper()
	for x := uint8(0); x < 16; x++ {
		for z := uint8(0); z < 16; z++ {
			for y := want.Range().Min(); y <= want.Range().Max(); y++ {
				for layer := uint8(0); layer < 2; layer++ {
					wantRID := want.Block(x, int16(y), z, layer)
					gotRID := got.Block(x, int16(y), z, layer)
					if gotRID != wantRID {
						t.Fatalf("chunk %v block (%d,%d,%d) layer %d: gpu=%d cpu=%d", pos, x, y, z, layer, gotRID, wantRID)
					}
				}
				wantBiome := want.Biome(x, int16(y), z)
				gotBiome := got.Biome(x, int16(y), z)
				if gotBiome != wantBiome {
					t.Fatalf("chunk %v biome (%d,%d,%d): gpu=%d cpu=%d", pos, x, y, z, gotBiome, wantBiome)
				}
			}
		}
	}
}
