package vanilla

import (
	"fmt"
	"sync"
	"unsafe"

	gen "github.com/bedrock-mc/vanilla-gen/gen"
	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/df-mc/dragonfly/server/world/chunk"
)

const seaLevel = 63
const featureStepCount = int(gen.GenerationStepTopLayerModification) + 1

var (
	sharedWorldgenOnce      sync.Once
	sharedWorldgenRegistry  *gen.WorldgenRegistry
	sharedStructureRegistry *gen.StructureTemplateRegistry
	sharedStructureResolver *structureResolver
	sharedPlannerCache      sync.Map
)

type Generator struct {
	blockRegistry      world.BlockRegistry
	dimension          world.Dimension
	dimensionName      string
	seed               int64
	biomeZoomSeed      int64
	activeTreeRegion   *treeDecorationRegion
	structureStepOrder [][]structureStepEntry
	graph              *gen.Graph
	graphRoots         map[string]int
	noises             *gen.NoiseRegistry
	worldgen           *gen.WorldgenRegistry
	metadata           gen.DimensionMetadata
	biomeSource        gen.BiomeSource
	carvers            *gen.CarverRegistry
	features           *gen.FeatureRegistry
	biomeGeneration    *biomeGenerationIndex
	featureRegionCache *featureMarginCache
	structureTemplates *gen.StructureTemplateRegistry
	structureResolver  *structureResolver
	structurePlanners  []structurePlanner
	structureStarts    *structureStartCache
	structureRings     *structureRingCache
	surface            *gen.SurfaceRuntime
	surfaceBlockCache  *blockRIDCache
	templateBlockCache *stringRIDCache
	blockNameCache     *runtimeIDNameCache
	featureNoiseCache  *doublePerlinNoiseCache
	protoChunks        *protoChunkCache
	prelimLevels       *xzIntCache
	finalDensityScalar gen.DensityScalarEvaluator
	finalDensityVector gen.DensityVectorEvaluator
	airRID             uint32
	defaultBlockRID    uint32
	deepRID            uint32
	bedrockRID         uint32
	defaultFluidRID    uint32
	waterRID           uint32
	lavaRID            uint32
	forceBottomBedrock bool
}

func New(seed int64) Generator {
	return NewForDimension(seed, world.Overworld)
}

func NewForDimension(seed int64, dim world.Dimension) Generator {
	world.DefaultBlockRegistry.Finalize()
	noises := gen.NewNoiseRegistry(seed)
	worldgen, structureTemplates, structureResolver := sharedStructureResources()
	dimensionName, graph, roots, surfaceRuntime, forceBottomBedrock, scalar, vector := dimensionRuntime(seed, dim, noises, worldgen)
	biomeSource, err := gen.NewBiomeSource(seed, worldgen, dimensionName)
	if err != nil {
		panic(err)
	}
	if surfaceRuntime == nil {
		surfaceRuntime = dimensionSurfaceRuntime(seed, dim, noises, biomeSource)
	}
	metadata, err := worldgen.DimensionMetadata("minecraft:" + dimensionName)
	if err != nil {
		panic(err)
	}
	structurePlanners := sharedStructurePlanners(worldgen, structureTemplates, dim)
	structureStepOrder := buildStructureStepOrder(structurePlanners)
	carvers := gen.NewCarverRegistry()
	features := gen.NewFeatureRegistry()
	biomeGeneration := newBiomeGenerationIndex(features, carvers, gen.PossibleBiomes(dimensionName))
	// Prewarm static structure pool/template data so chunk generation doesn't pay first-use decode costs.
	structureResolver.prewarmJigsawCandidates(structurePlanners)
	defaultBlockRID := runtimeIDForDimensionState(metadata.DefaultBlock)
	defaultFluidRID := runtimeIDForDimensionState(metadata.DefaultFluid)
	return Generator{
		blockRegistry:      world.DefaultBlockRegistry,
		dimension:          dim,
		dimensionName:      dimensionName,
		seed:               seed,
		biomeZoomSeed:      gen.ObfuscateSeed(seed),
		structureStepOrder: structureStepOrder,
		graph:              graph,
		graphRoots:         roots,
		noises:             noises,
		worldgen:           worldgen,
		metadata:           metadata,
		biomeSource:        biomeSource,
		carvers:            carvers,
		features:           features,
		biomeGeneration:    biomeGeneration,
		featureRegionCache: newFeatureMarginCache(),
		structureTemplates: structureTemplates,
		structureResolver:  structureResolver,
		structurePlanners:  structurePlanners,
		structureStarts:    newStructureStartCache(),
		structureRings:     newStructureRingCache(),
		surface:            surfaceRuntime,
		surfaceBlockCache:  newBlockRIDCache(),
		templateBlockCache: newStringRIDCache(),
		blockNameCache:     newRuntimeIDNameCache(),
		featureNoiseCache:  newDoublePerlinNoiseCache(),
		protoChunks:        newProtoChunkCache(320),
		prelimLevels:       newXZIntCache(),
		finalDensityScalar: scalar,
		finalDensityVector: vector,
		airRID:             runtimeIDForBlock(block.Air{}),
		defaultBlockRID:    defaultBlockRID,
		deepRID:            runtimeIDForBlock(block.Deepslate{Type: block.NormalDeepslate(), Axis: cube.Y}),
		bedrockRID:         runtimeIDForBlock(block.Bedrock{}),
		defaultFluidRID:    defaultFluidRID,
		waterRID:           runtimeIDForBlock(block.Water{Still: true, Depth: 8}),
		lavaRID:            runtimeIDForBlock(block.Lava{Still: true, Depth: 8}),
		forceBottomBedrock: forceBottomBedrock,
	}
}

// Seed returns the seed configured for the generator.
func (g Generator) Seed() int64 {
	return g.seed
}

func sharedStructureResources() (*gen.WorldgenRegistry, *gen.StructureTemplateRegistry, *structureResolver) {
	sharedWorldgenOnce.Do(func() {
		sharedWorldgenRegistry = gen.NewWorldgenRegistry()
		sharedStructureRegistry = gen.NewStructureTemplateRegistry(sharedWorldgenRegistry)
		sharedStructureResolver = newStructureResolver(sharedWorldgenRegistry, sharedStructureRegistry)
	})
	return sharedWorldgenRegistry, sharedStructureRegistry, sharedStructureResolver
}

func sharedStructurePlanners(worldgen *gen.WorldgenRegistry, templates *gen.StructureTemplateRegistry, dim world.Dimension) []structurePlanner {
	if planners, ok := sharedPlannerCache.Load(dim); ok {
		return planners.([]structurePlanner)
	}
	planners := buildStructurePlanners(worldgen, templates, dim)
	actual, _ := sharedPlannerCache.LoadOrStore(dim, planners)
	return actual.([]structurePlanner)
}

func (g Generator) GenerateChunk(pos world.ChunkPos, c *chunk.Chunk) {
	chunkX := int(pos[0])
	chunkZ := int(pos[1])
	minY := c.Range().Min()
	maxY := c.Range().Max()
	biomes, _, _, _, _ := g.prepareChunkForDecoration(pos, c)
	g.decorateFeaturesAndStructures(c, biomes, chunkX, chunkZ, minY, maxY)
}

func (g Generator) prepareChunkForDecoration(pos world.ChunkPos, c *chunk.Chunk) (sourceBiomeVolume, int, int, int, int) {
	chunkX := int(pos[0])
	chunkZ := int(pos[1])
	minY := c.Range().Min()
	maxY := c.Range().Max()

	// The pre-decoration state of a chunk is deterministic and needed by every
	// neighbouring chunk's region replay, so it is cached and copied.
	if g.protoChunks != nil {
		if cached, biomes, ok := g.protoChunks.get(chunkX, chunkZ); ok {
			copyChunkInto(c, cached, minY, maxY)
			return biomes, chunkX, chunkZ, minY, maxY
		}
	}
	biomes := g.prepareChunkUncached(c, chunkX, chunkZ, minY, maxY)
	if g.protoChunks != nil {
		g.protoChunks.store(chunkX, chunkZ, c, biomes)
	}
	return biomes, chunkX, chunkZ, minY, maxY
}

func copyChunkInto(dst, src *chunk.Chunk, minY, maxY int) {
	for x := uint8(0); x < 16; x++ {
		for z := uint8(0); z < 16; z++ {
			for y := minY; y <= maxY; y++ {
				dst.SetBlock(x, int16(y), z, 0, src.Block(x, int16(y), z, 0))
				dst.SetBiome(x, int16(y), z, src.Biome(x, int16(y), z))
			}
		}
	}
}

func (g Generator) prepareChunkUncached(c *chunk.Chunk, chunkX, chunkZ, minY, maxY int) sourceBiomeVolume {
	flat := g.graph.NewFlatCacheGrid(chunkX, chunkZ, g.noises)
	finalDensityRoot := g.rootIndex("final_density")
	densityScalar := g.finalDensityScalar
	densityVector := g.finalDensityVector
	if terrainSampler := newStructureTerrainSampler(g, chunkX, chunkZ, minY, maxY); terrainSampler != nil {
		densityScalar = terrainSampler.scalarEvaluator(g, densityScalar)
		densityVector = terrainSampler.vectorEvaluator(g, g.finalDensityScalar, densityVector)
	}
	densityChunk := gen.NewFinalDensityChunkWithEvaluator(
		g.graph,
		finalDensityRoot,
		chunkX,
		chunkZ,
		minY,
		maxY,
		g.noises,
		flat,
		densityScalar,
		densityVector,
	)
	var aquifer *gen.NoiseBasedAquifer
	if g.metadata.AquifersEnabled {
		aquifer = gen.NewNoiseBasedAquifer(
			g.graph,
			chunkX,
			chunkZ,
			minY,
			maxY,
			g.noises,
			flat,
			g.seed,
			gen.OverworldFluidPicker{SeaLevel: g.metadata.SeaLevel},
		)
	}

	for localX := 0; localX < 16; localX++ {
		for localZ := 0; localZ < 16; localZ++ {
			worldX := chunkX*16 + localX
			worldZ := chunkZ*16 + localZ

			for y := minY + 1; y <= maxY; y++ {
				density := densityChunk.Density(localX, y, localZ)

				if density > 0 {
					rid := g.baseRuntimeID(y)
					c.SetBlock(uint8(localX), int16(y), uint8(localZ), 0, rid)
					continue
				}

				if aquifer != nil {
					switch aquifer.ComputeSubstance(
						gen.FunctionContext{BlockX: worldX, BlockY: y, BlockZ: worldZ},
						density,
					) {
					case gen.AquiferBarrier:
						c.SetBlock(uint8(localX), int16(y), uint8(localZ), 0, g.baseRuntimeID(y))
					case gen.AquiferWater:
						c.SetBlock(uint8(localX), int16(y), uint8(localZ), 0, g.waterRID)
					case gen.AquiferLava:
						c.SetBlock(uint8(localX), int16(y), uint8(localZ), 0, g.lavaRID)
					}
					continue
				}

				if y <= g.metadata.SeaLevel && g.defaultFluidRID != g.airRID {
					c.SetBlock(uint8(localX), int16(y), uint8(localZ), 0, g.defaultFluidRID)
				}
			}

			if g.forceBottomBedrock {
				c.SetBlock(uint8(localX), int16(minY), uint8(localZ), 0, g.bedrockRID)
			}
		}
	}

	biomes := g.populateBiomeVolume(c, chunkX, chunkZ, minY, maxY)
	// Vanilla builds the surface before carving; carvers patch exposed dirt
	// via the topMaterial fixup instead.
	g.applySurfaceAndBiomes(c, biomes, chunkX, chunkZ, minY, maxY)
	g.carveTerrain(c, biomes, chunkX, chunkZ, minY, maxY, aquifer)
	g.decorateEndMainIsland(c, chunkX, chunkZ, minY, maxY)
	return biomes
}

// ConcurrentChunkGeneration returns true because Generator guards its shared
// caches and registries internally.
func (g Generator) ConcurrentChunkGeneration() bool { return true }

// baseRuntimeID returns the dimension default block; vanilla fills the whole
// noise terrain with it and deepslate comes from the surface rule gradient.
func (g Generator) baseRuntimeID(int) uint32 {
	return g.defaultBlockRID
}

func (g Generator) isSolidRID(rid uint32) bool {
	return rid != g.airRID && rid != g.waterRID && rid != g.lavaRID
}

func (g Generator) rootIndex(name string) int {
	if g.graphRoots == nil {
		return -1
	}
	if root, ok := g.graphRoots[name]; ok {
		return root
	}
	return -1
}

func dimensionRuntime(_ int64, dim world.Dimension, _ *gen.NoiseRegistry, _ *gen.WorldgenRegistry) (string, *gen.Graph, map[string]int, *gen.SurfaceRuntime, bool, gen.DensityScalarEvaluator, gen.DensityVectorEvaluator) {
	switch dim {
	case world.Overworld:
		return "overworld", gen.OverworldGraph, gen.OverworldRoots, nil, true, gen.ComputeFinalDensity, gen.ComputeFinalDensity4
	case world.Nether:
		return "nether", gen.NetherGraph, gen.NetherRoots, nil, true, nil, nil
	case world.End:
		return "end", gen.EndGraph, gen.EndRoots, nil, false, nil, nil
	default:
		panic(fmt.Sprintf("unsupported dimension %v", dim))
	}
}

func dimensionSurfaceRuntime(seed int64, dim world.Dimension, noises *gen.NoiseRegistry, biomeSource gen.BiomeSource) *gen.SurfaceRuntime {
	switch dim {
	case world.Overworld:
		return gen.NewOverworldSurfaceRuntime(seed, noises, biomeSource)
	case world.Nether:
		return gen.NewNetherSurfaceRuntime(seed, noises, biomeSource)
	case world.End:
		return gen.NewEndSurfaceRuntime(seed, noises, biomeSource)
	default:
		return nil
	}
}

func runtimeIDForBlock(b world.Block) uint32 {
	if b == nil {
		b = block.Air{}
	}
	name, properties := b.EncodeBlock()
	return world.BlockRuntimeID(runtimeBlock{name: name, properties: properties})
}

func runtimeIDForDimensionState(state gen.DimensionBlockState) uint32 {
	switch state.Name {
	case "minecraft:air":
		return runtimeIDForBlock(block.Air{})
	case "minecraft:stone":
		return runtimeIDForBlock(block.Stone{})
	case "minecraft:netherrack":
		return runtimeIDForBlock(block.Netherrack{})
	case "minecraft:end_stone":
		return runtimeIDForBlock(block.EndStone{})
	case "minecraft:water":
		return runtimeIDForBlock(block.Water{Still: true, Depth: 8})
	case "minecraft:lava":
		return runtimeIDForBlock(block.Lava{Still: true, Depth: 8})
	}

	properties := make(map[string]any, len(state.Properties))
	for key, value := range state.Properties {
		properties[key] = value
	}
	if b, ok := world.BlockByName(state.Name, properties); ok {
		return runtimeIDForBlock(b)
	}
	return runtimeIDForBlock(block.Air{})
}

type blockRIDCache struct {
	mu    sync.RWMutex
	byKey map[blockStateKey]uint32
}

func newBlockRIDCache() *blockRIDCache {
	return &blockRIDCache{byKey: make(map[blockStateKey]uint32)}
}

func (c *blockRIDCache) Lookup(key blockStateKey) (uint32, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	rid, ok := c.byKey[key]
	return rid, ok
}

func (c *blockRIDCache) Store(key blockStateKey, rid uint32) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.byKey[key] = rid
}

type runtimeIDNameCache struct {
	mu    sync.RWMutex
	byRID map[uint32]string
}

func newRuntimeIDNameCache() *runtimeIDNameCache {
	return &runtimeIDNameCache{byRID: make(map[uint32]string)}
}

func (c *runtimeIDNameCache) Lookup(rid uint32) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	name, ok := c.byRID[rid]
	return name, ok
}

func (c *runtimeIDNameCache) Store(rid uint32, name string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.byRID[rid] = name
}

type stringRIDCache struct {
	mu    sync.RWMutex
	byKey map[string]uint32
}

func newStringRIDCache() *stringRIDCache {
	return &stringRIDCache{byKey: make(map[string]uint32)}
}

func (c *stringRIDCache) Lookup(key string) (uint32, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	rid, ok := c.byKey[key]
	return rid, ok
}

func (c *stringRIDCache) Store(key string, rid uint32) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.byKey[key] = rid
}

type blockStateKey struct {
	name       string
	properties uintptr
}

func blockStateCacheKey(name string, properties map[string]string) blockStateKey {
	if len(properties) == 0 {
		return blockStateKey{name: name}
	}
	return blockStateKey{name: name, properties: mapIdentity(properties)}
}

func mapIdentity(m map[string]string) uintptr {
	return *(*uintptr)(unsafe.Pointer(&m))
}

type doublePerlinNoiseCache struct {
	mu    sync.RWMutex
	byKey map[string]gen.DoublePerlinNoise
}

func newDoublePerlinNoiseCache() *doublePerlinNoiseCache {
	return &doublePerlinNoiseCache{byKey: make(map[string]gen.DoublePerlinNoise)}
}

func (c *doublePerlinNoiseCache) Lookup(key string) (gen.DoublePerlinNoise, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	noise, ok := c.byKey[key]
	return noise, ok
}

func (c *doublePerlinNoiseCache) Store(key string, noise gen.DoublePerlinNoise) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.byKey[key] = noise
}

type biomeGenerationIndex struct {
	featureSteps      [256][featureStepCount][]string
	featureIndexes    [256][featureStepCount][]int
	featureMembership [256]map[string]struct{}
	stepFeatures      [featureStepCount]stepFeatureData
	carverNames       [256][]string
}

func newBiomeGenerationIndex(features *gen.FeatureRegistry, carvers *gen.CarverRegistry, possibleBiomes []gen.Biome) *biomeGenerationIndex {
	idx := &biomeGenerationIndex{}
	// Carvers stay populated for every biome; the feature order below only
	// considers the generator's possible biomes like vanilla FeatureSorter.
	for _, biome := range sortedBiomesByKey {
		biomeID := int(biome)
		key := biomeKey(biome)
		if key == "" {
			continue
		}
		for step := 0; step < featureStepCount; step++ {
			idx.featureSteps[biomeID][step] = features.BiomePlacedFeatures(key, gen.GenerationStep(step))
		}
		idx.carverNames[biomeID] = carvers.BiomeCarvers(key)
	}
	idx.stepFeatures = buildStepFeatureData(idx.featureSteps, possibleBiomes)
	for _, biome := range sortedBiomesByKey {
		biomeID := int(biome)
		membership := make(map[string]struct{})
		for step := 0; step < featureStepCount; step++ {
			names := idx.featureSteps[biomeID][step]
			indexes := make([]int, 0, len(names))
			for _, name := range names {
				membership[name] = struct{}{}
				if index, ok := idx.stepFeatures[step].index(name); ok {
					indexes = append(indexes, index)
				}
			}
			idx.featureIndexes[biomeID][step] = indexes
		}
		idx.featureMembership[biomeID] = membership
	}
	return idx
}
