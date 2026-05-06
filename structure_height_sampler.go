package vanilla

import (
	"math"

	gen "github.com/bedrock-mc/vanilla-gen/gen"
	"github.com/df-mc/dragonfly/server/world"
)

type structureHeightSampler struct {
	g                Generator
	minY             int
	maxY             int
	finalDensityRoot int

	preliminary map[uint64]*structurePreliminaryChunk
	world       map[uint64]*structureWorldSurfaceChunk
}

type structurePreliminaryChunk struct {
	flat    *gen.FlatCacheGrid
	columns [16 * 16]*gen.ColumnContext
	heights [16 * 16]int16
	loaded  [16 * 16]bool
}

type structureWorldSurfaceChunk struct {
	density *gen.FinalDensityChunk
	aquifer *gen.NoiseBasedAquifer
	heights [16 * 16]int16
	loaded  [16 * 16]bool
}

func newStructureHeightSampler(g Generator, minY, maxY int) *structureHeightSampler {
	return &structureHeightSampler{
		g:                g,
		minY:             minY,
		maxY:             maxY,
		finalDensityRoot: g.rootIndex("final_density"),
		preliminary:      make(map[uint64]*structurePreliminaryChunk, 4),
		world:            make(map[uint64]*structureWorldSurfaceChunk, 4),
	}
}

func (s *structureHeightSampler) preliminarySurfaceLevelAt(blockX, blockZ int) int {
	chunkX, chunkZ, localX, localZ := structureHeightChunkCoords(blockX, blockZ)
	key := structureHeightSamplerKey(chunkX, chunkZ)
	entry := s.preliminary[key]
	if entry == nil {
		entry = &structurePreliminaryChunk{
			flat: s.g.graph.NewFlatCacheGrid(chunkX, chunkZ, s.g.noises),
		}
		s.preliminary[key] = entry
	}

	index := localZ*16 + localX
	if entry.loaded[index] {
		return int(entry.heights[index])
	}

	column := entry.columns[index]
	if column == nil {
		column = s.g.graph.NewColumnContext(blockX, blockZ, s.g.noises, entry.flat)
		entry.columns[index] = column
	}

	ctx := gen.FunctionContext{BlockX: blockX, BlockY: 0, BlockZ: blockZ}
	value := 0.0
	if s.g.dimension == world.Overworld {
		value = gen.ComputePreliminarySurfaceLevel(ctx, s.g.noises, entry.flat, column)
	} else {
		value = s.g.graph.Eval(s.g.rootIndex("preliminary_surface_level"), ctx, s.g.noises, entry.flat, column, nil)
	}
	y := clamp(int(math.Floor(value)), s.minY, s.maxY)
	entry.heights[index] = int16(y)
	entry.loaded[index] = true
	return y
}

func (s *structureHeightSampler) worldSurfaceLevelAt(blockX, blockZ int) int {
	if s.finalDensityRoot < 0 {
		return s.preliminarySurfaceLevelAt(blockX, blockZ)
	}

	chunkX, chunkZ, localX, localZ := structureHeightChunkCoords(blockX, blockZ)
	key := structureHeightSamplerKey(chunkX, chunkZ)
	entry := s.world[key]
	if entry == nil {
		flat := s.g.graph.NewFlatCacheGrid(chunkX, chunkZ, s.g.noises)
		entry = &structureWorldSurfaceChunk{
			density: gen.NewFinalDensityChunkWithEvaluator(
				s.g.graph,
				s.finalDensityRoot,
				chunkX,
				chunkZ,
				s.minY,
				s.maxY,
				s.g.noises,
				flat,
				s.g.finalDensityScalar,
				s.g.finalDensityVector,
			),
		}
		if s.g.metadata.AquifersEnabled {
			entry.aquifer = gen.NewNoiseBasedAquifer(
				s.g.graph,
				chunkX,
				chunkZ,
				s.minY,
				s.maxY,
				s.g.noises,
				flat,
				s.g.seed,
				gen.OverworldFluidPicker{SeaLevel: s.g.metadata.SeaLevel},
			)
		}
		s.world[key] = entry
	}

	index := localZ*16 + localX
	if entry.loaded[index] {
		return int(entry.heights[index])
	}

	y := s.computeWorldSurfaceLevel(entry, blockX, blockZ, localX, localZ)
	entry.heights[index] = int16(y)
	entry.loaded[index] = true
	return y
}

func (s *structureHeightSampler) computeWorldSurfaceLevel(entry *structureWorldSurfaceChunk, blockX, blockZ, localX, localZ int) int {
	for y := s.maxY; y >= s.minY; y-- {
		density := entry.density.Density(localX, y, localZ)
		if density > 0 {
			return min(y+1, s.maxY)
		}
		if entry.aquifer != nil {
			switch entry.aquifer.ComputeSubstance(gen.FunctionContext{BlockX: blockX, BlockY: y, BlockZ: blockZ}, density) {
			case gen.AquiferBarrier, gen.AquiferWater, gen.AquiferLava:
				return min(y+1, s.maxY)
			}
			continue
		}
		if y <= s.g.metadata.SeaLevel && s.g.defaultFluidRID != s.g.airRID {
			return min(y+1, s.maxY)
		}
	}
	return s.minY
}

func structureHeightChunkCoords(blockX, blockZ int) (chunkX, chunkZ, localX, localZ int) {
	chunkX = floorDiv(blockX, 16)
	chunkZ = floorDiv(blockZ, 16)
	localX = blockX - chunkX*16
	localZ = blockZ - chunkZ*16
	return chunkX, chunkZ, localX, localZ
}

func structureHeightSamplerKey(x, z int) uint64 {
	return uint64(uint32(x))<<32 | uint64(uint32(z))
}
