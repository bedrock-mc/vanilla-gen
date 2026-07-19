package vanilla

import (
	"errors"
	"os"
	"sync/atomic"
	"testing"

	gen "github.com/bedrock-mc/vanilla-gen/gen"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/df-mc/dragonfly/server/world/chunk"
)

var benchmarkClimateSink [6]int64
var benchmarkSerialisedSink chunk.SerialisedData

func benchmarkChunkPos(index int) world.ChunkPos {
	// Keep generated 3x3 decoration regions disjoint. This measures cold world
	// generation rather than the proto-chunk and biome-cache hit rate.
	const stride = 64
	return world.ChunkPos{int32(index&31) * stride, int32(index>>5) * stride}
}

func benchmarkAdjacentChunkPos(index int) world.ChunkPos {
	// Model an initial view-distance load: unique chunks packed into adjacent
	// rows around spawn. Their 3x3 decoration regions overlap heavily, just as
	// they do when Dragonfly fans out a player's first chunk requests.
	const width = 32
	return world.ChunkPos{
		int32(index%width - width/2),
		int32(index/width - width/2),
	}
}

func benchmarkClimatePoints() []gen.FunctionContext {
	points := make([]gen.FunctionContext, 0, 9*4*4*97)
	for x := -16; x <= 28; x += 4 {
		for z := -16; z <= 28; z += 4 {
			for y := -64; y <= 320; y += 4 {
				points = append(points, gen.FunctionContext{BlockX: x, BlockY: y, BlockZ: z})
			}
		}
	}
	return points
}

// BenchmarkOverworldDensityCPU isolates the part of overworld generation that
// the GPU backend accelerates. It deliberately varies the chunk position so
// the benchmark does not turn into a cache benchmark.
func BenchmarkOverworldDensityCPU(b *testing.B) {
	noises := gen.NewNoiseRegistry(1)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		chunkX := i & 31
		chunkZ := i >> 5
		flat := gen.OverworldGraph.NewFlatCacheGrid(chunkX, chunkZ, noises)
		_ = gen.NewFinalDensityChunk(
			gen.OverworldGraph,
			chunkX,
			chunkZ,
			-64,
			319,
			noises,
			flat,
		)
	}
}

func BenchmarkOverworldDensityGPU(b *testing.B) {
	const seed = int64(1)
	noises := gen.NewNoiseRegistry(seed)
	accelerator, err := gen.NewOpenCLAccelerator(noises, gen.OpenCLConfig{LibraryPath: os.Getenv("VANILLA_GEN_OPENCL_LIBRARY")})
	if err != nil {
		if errors.Is(err, gen.ErrAcceleratorUnavailable) {
			b.Skip(err)
		}
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = accelerator.Close() })
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		chunkX := i & 31
		chunkZ := i >> 5
		flat := gen.OverworldGraph.NewFlatCacheGrid(chunkX, chunkZ, noises)
		if _, err := accelerator.FinalDensity(chunkX, chunkZ, -64, 319, flat); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkClimateBatchCPU measures the 13,968-point climate query used to
// select possible feature biomes for a decoration region.
func BenchmarkClimateBatchCPU(b *testing.B) {
	source, err := gen.NewBiomeSource(1, gen.NewWorldgenRegistry(), "overworld")
	if err != nil {
		b.Fatal(err)
	}
	points := benchmarkClimatePoints()
	dst := make([][6]int64, len(points))
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		for i, point := range points {
			dst[i] = source.SampleClimate(point.BlockX, point.BlockY, point.BlockZ)
		}
	}
	benchmarkClimateSink = dst[len(dst)-1]
}

func BenchmarkClimateBatchGPU(b *testing.B) {
	accelerator, err := gen.NewOpenCLAccelerator(gen.NewNoiseRegistry(1), gen.OpenCLConfig{
		LibraryPath: os.Getenv("VANILLA_GEN_OPENCL_LIBRARY"),
	})
	if err != nil {
		if errors.Is(err, gen.ErrAcceleratorUnavailable) {
			b.Skip(err)
		}
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = accelerator.Close() })
	points := benchmarkClimatePoints()
	dst := make([][6]int64, len(points))
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if err := accelerator.SampleClimate(points, dst); err != nil {
			b.Fatal(err)
		}
	}
	benchmarkClimateSink = dst[len(dst)-1]
}

func BenchmarkPrepareChunkCPU(b *testing.B) {
	g := New(1)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pos := benchmarkChunkPos(i)
		c := chunk.New(world.DefaultBlockRegistry, cube.Range{-64, 319})
		g.prepareChunkUncached(c, int(pos[0]), int(pos[1]), -64, 319)
	}
}

func BenchmarkPrepareChunkGPU(b *testing.B) {
	g := newGPUGeneratorForTest(b, 1)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pos := benchmarkChunkPos(i)
		c := chunk.New(world.DefaultBlockRegistry, cube.Range{-64, 319})
		g.prepareChunkUncached(c, int(pos[0]), int(pos[1]), -64, 319)
	}
}

// BenchmarkPopulateBiomeVolume isolates expanding the quart-resolution biome
// volume into Dragonfly's block-resolution paletted storages. The climate
// result is warm: this benchmark intentionally measures storage population,
// not biome selection.
func BenchmarkPopulateBiomeVolume(b *testing.B) {
	g := New(1)
	const chunkX, chunkZ = 0, 0
	_ = g.loadChunkBiomeVolume(chunkX, chunkZ, -64, 319)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		c := chunk.New(world.DefaultBlockRegistry, cube.Range{-64, 319})
		b.StartTimer()
		g.populateBiomeVolume(c, chunkX, chunkZ, -64, 319)
	}
}

// BenchmarkProtoChunkCacheHit measures the path used when overlapping 3x3
// decoration regions ask for terrain that an adjacent request already built.
func BenchmarkProtoChunkCacheHit(b *testing.B) {
	g := New(1)
	pos := world.ChunkPos{0, 0}
	warm := chunk.New(world.DefaultBlockRegistry, cube.Range{-64, 319})
	g.prepareChunkForDecoration(pos, warm)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c := chunk.New(world.DefaultBlockRegistry, cube.Range{-64, 319})
		g.prepareChunkForDecoration(pos, c)
	}
}

type benchmarkPreparedChunk struct {
	chunk  *chunk.Chunk
	biomes sourceBiomeVolume
	pos    world.ChunkPos
}

func benchmarkPreparedDecorationArea(b *testing.B, g Generator) []benchmarkPreparedChunk {
	b.Helper()
	// Preload the 10x10 proto area needed by 64 adjacent target chunks. This
	// removes terrain generation while retaining all neighbour clones and all
	// feature/structure work performed by decoration.
	for z := -1; z <= 8; z++ {
		for x := -1; x <= 8; x++ {
			pos := world.ChunkPos{int32(x), int32(z)}
			c := chunk.New(world.DefaultBlockRegistry, cube.Range{-64, 319})
			g.prepareChunkForDecoration(pos, c)
		}
	}
	prepared := make([]benchmarkPreparedChunk, 0, 64)
	for z := 0; z < 8; z++ {
		for x := 0; x < 8; x++ {
			pos := world.ChunkPos{int32(x), int32(z)}
			c, biomes, ok := g.protoChunks.get(x, z)
			if !ok {
				b.Fatalf("missing prepared chunk %v", pos)
			}
			prepared = append(prepared, benchmarkPreparedChunk{chunk: c, biomes: biomes, pos: pos})
		}
	}
	return prepared
}

func BenchmarkDecorateChunkCPU(b *testing.B) {
	g := New(1)
	prepared := benchmarkPreparedDecorationArea(b, g)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		base := prepared[i%len(prepared)]
		b.StopTimer()
		c := base.chunk.Clone()
		b.StartTimer()
		g.decorateFeaturesAndStructures(c, base.biomes, int(base.pos[0]), int(base.pos[1]), -64, 319)
	}
}

func BenchmarkDecorateChunkGPU(b *testing.B) {
	g := newGPUGeneratorForTest(b, 1)
	prepared := benchmarkPreparedDecorationArea(b, g)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		base := prepared[i%len(prepared)]
		b.StopTimer()
		c := base.chunk.Clone()
		b.StartTimer()
		g.decorateFeaturesAndStructures(c, base.biomes, int(base.pos[0]), int(base.pos[1]), -64, 319)
	}
}

func BenchmarkGeneratedChunkClone(b *testing.B) {
	g := New(1)
	c := chunk.New(world.DefaultBlockRegistry, cube.Range{-64, 319})
	g.GenerateChunk(world.ChunkPos{0, 0}, c)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.Clone()
	}
}

func BenchmarkGeneratedChunkLightFill(b *testing.B) {
	g := New(1)
	pos := world.ChunkPos{0, 0}
	base := chunk.New(world.DefaultBlockRegistry, cube.Range{-64, 319})
	g.GenerateChunk(pos, base)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		c := base.Clone()
		b.StartTimer()
		chunk.LightArea([]*chunk.Chunk{c}, int(pos[0]), int(pos[1])).Fill()
	}
}

func BenchmarkGeneratedChunkNetworkEncode(b *testing.B) {
	g := New(1)
	c := chunk.New(world.DefaultBlockRegistry, cube.Range{-64, 319})
	g.GenerateChunk(world.ChunkPos{0, 0}, c)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkSerialisedSink = chunk.Encode(c, chunk.NetworkEncoding)
	}
}

// BenchmarkGenerateChunkCPU measures the public, complete world-generation
// path. GPU benchmarks use the same chunk-position sequence.
func BenchmarkGenerateChunkCPU(b *testing.B) {
	g := New(1)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pos := benchmarkChunkPos(i)
		c := chunk.New(world.DefaultBlockRegistry, cube.Range{-64, 319})
		g.GenerateChunk(pos, c)
	}
}

func BenchmarkGenerateChunkGPU(b *testing.B) {
	g := newGPUGeneratorForTest(b, 1)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pos := benchmarkChunkPos(i)
		c := chunk.New(world.DefaultBlockRegistry, cube.Range{-64, 319})
		g.GenerateChunk(pos, c)
	}
}

func BenchmarkGenerateChunkAdjacentCPU(b *testing.B) {
	g := New(1)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pos := benchmarkAdjacentChunkPos(i)
		c := chunk.New(world.DefaultBlockRegistry, cube.Range{-64, 319})
		g.GenerateChunk(pos, c)
	}
}

func BenchmarkGenerateChunkAdjacentGPU(b *testing.B) {
	g := newGPUGeneratorForTest(b, 1)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pos := benchmarkAdjacentChunkPos(i)
		c := chunk.New(world.DefaultBlockRegistry, cube.Range{-64, 319})
		g.GenerateChunk(pos, c)
	}
}

func BenchmarkGenerateChunkParallelCPU(b *testing.B) {
	g := New(1)
	var next atomic.Uint64
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			pos := benchmarkChunkPos(int(next.Add(1) - 1))
			c := chunk.New(world.DefaultBlockRegistry, cube.Range{-64, 319})
			g.GenerateChunk(pos, c)
		}
	})
}

func BenchmarkGenerateChunkParallelGPU(b *testing.B) {
	g := newGPUGeneratorForTest(b, 1)
	var next atomic.Uint64
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			pos := benchmarkChunkPos(int(next.Add(1) - 1))
			c := chunk.New(world.DefaultBlockRegistry, cube.Range{-64, 319})
			g.GenerateChunk(pos, c)
		}
	})
}

func BenchmarkGenerateChunkAdjacentParallelCPU(b *testing.B) {
	g := New(1)
	var next atomic.Uint64
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			pos := benchmarkAdjacentChunkPos(int(next.Add(1) - 1))
			c := chunk.New(world.DefaultBlockRegistry, cube.Range{-64, 319})
			g.GenerateChunk(pos, c)
		}
	})
}

func BenchmarkGenerateChunkAdjacentParallelGPU(b *testing.B) {
	g := newGPUGeneratorForTest(b, 1)
	var next atomic.Uint64
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			pos := benchmarkAdjacentChunkPos(int(next.Add(1) - 1))
			c := chunk.New(world.DefaultBlockRegistry, cube.Range{-64, 319})
			g.GenerateChunk(pos, c)
		}
	})
}
