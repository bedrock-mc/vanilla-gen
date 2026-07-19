package gen

import (
	"fmt"
	"sync"
)

type BiomeSource interface {
	SampleClimate(x, y, z int) [6]int64
	GetBiome(x, y, z int) Biome
}

// BatchBiomeSource can resolve many independent biome samples in one call.
// GPU-backed sources use this to amortize dispatch overhead; callers should
// retain the scalar BiomeSource path for small queries.
type BatchBiomeSource interface {
	BiomeSource
	GetBiomes(points []FunctionContext, dst []Biome)
}

type presetBiomeSource struct {
	preset      string
	noise       BiomeNoise
	graph       *Graph
	graphRoots  [6]int
	noises      *NoiseRegistry
	rtree       *climateRTree
	cache       biomeCache
	scratchPool *sync.Pool
	accelerator ComputeAccelerator
	batchPool   sync.Pool
}

var (
	overworldRTreeOnce sync.Once
	overworldRTree     *climateRTree
)

func overworldClimateRTree() *climateRTree {
	overworldRTreeOnce.Do(func() {
		overworldRTree = newClimateRTree(overworldBiomePoints)
	})
	return overworldRTree
}

type endBiomeSource struct {
	erosion EndIslandDensity
	cache   biomeCache
}

const biomeCacheShardCount = 64

type biomeCacheShard struct {
	mu     sync.RWMutex
	values map[int64]Biome
}

type biomeCache [biomeCacheShardCount]biomeCacheShard

func (c *biomeCache) shard(key int64) *biomeCacheShard {
	hash := uint64(key)
	hash ^= hash >> 21
	hash ^= hash >> 42
	return &c[hash&(biomeCacheShardCount-1)]
}

func (c *biomeCache) Load(key int64) (Biome, bool) {
	shard := c.shard(key)
	shard.mu.RLock()
	biome, ok := shard.values[key]
	shard.mu.RUnlock()
	return biome, ok
}

func (c *biomeCache) Store(key int64, biome Biome) {
	shard := c.shard(key)
	shard.mu.Lock()
	if shard.values == nil {
		shard.values = make(map[int64]Biome)
	}
	shard.values[key] = biome
	shard.mu.Unlock()
}

// biomeCacheKey packs quart coordinates into one int64 for cheap hashing on
// the hot biome lookup path.
func biomeCacheKey(x, y, z int) int64 {
	return int64(x>>2)&0x1FFFFF<<42 | int64(y>>2)&0x1FFFFF<<21 | int64(z>>2)&0x1FFFFF
}

type climateParameter struct {
	min int64
	max int64
}

type climateParameterPoint struct {
	params [6]climateParameter
	offset int64
	biome  Biome
}

func NewBiomeSource(seed int64, registry *WorldgenRegistry, name string) (BiomeSource, error) {
	return NewBiomeSourceWithAccelerator(seed, registry, name, nil)
}

// NewBiomeSourceWithAccelerator creates a biome source that batches overworld
// climate samples through accelerator. It falls back to the scalar CPU path
// if a runtime submission fails.
func NewBiomeSourceWithAccelerator(seed int64, registry *WorldgenRegistry, name string, accelerator ComputeAccelerator) (BiomeSource, error) {
	if registry == nil {
		registry = NewWorldgenRegistry()
	}
	if normalizeIdentifier(name) == "end" {
		return &endBiomeSource{erosion: NewEndIslandDensity(seed)}, nil
	}

	def, err := registry.BiomeSourceParameterList(name)
	if err != nil {
		return nil, err
	}

	switch def.Preset {
	case "overworld":
		// Vanilla Climate.Sampler samples the noise router's climate density
		// functions, so the overworld source evaluates the graph roots.
		noises := NewNoiseRegistry(seed)
		var roots [6]int
		for i, slot := range [6]string{"temperature", "vegetation", "continents", "erosion", "depth", "ridges"} {
			idx, ok := OverworldRoots[slot]
			if !ok {
				return nil, fmt.Errorf("overworld graph missing climate root %q", slot)
			}
			roots[i] = idx
		}
		graph := OverworldGraph
		return &presetBiomeSource{
			preset: def.Preset, noise: NewBiomeNoise(seed), graph: graph, graphRoots: roots,
			noises: noises, rtree: overworldClimateRTree(),
			scratchPool: &sync.Pool{New: func() any { return NewEvalScratch(graph) }},
			accelerator: accelerator,
		}, nil
	case "nether":
		return &presetBiomeSource{preset: def.Preset, noise: NewBiomeNoise(seed)}, nil
	default:
		return nil, fmt.Errorf("unsupported biome source preset %q", def.Preset)
	}
}

type biomeBatchScratch struct {
	points  []FunctionContext
	indexes []int
	biomes  []Biome
}

func (s *presetBiomeSource) GetBiomes(points []FunctionContext, dst []Biome) {
	if len(dst) < len(points) {
		panic("gen: biome batch destination too small")
	}
	if s.accelerator == nil || s.preset != "overworld" || len(points) < 64 {
		for i, point := range points {
			dst[i] = s.GetBiome(point.BlockX, point.BlockY, point.BlockZ)
		}
		return
	}

	var scratch *biomeBatchScratch
	if pooled := s.batchPool.Get(); pooled != nil {
		scratch = pooled.(*biomeBatchScratch)
	} else {
		scratch = &biomeBatchScratch{}
	}
	defer func() {
		scratch.points = scratch.points[:0]
		scratch.indexes = scratch.indexes[:0]
		scratch.biomes = scratch.biomes[:0]
		s.batchPool.Put(scratch)
	}()
	for i, point := range points {
		key := biomeCacheKey(point.BlockX, point.BlockY, point.BlockZ)
		if cached, ok := s.cache.Load(key); ok {
			dst[i] = cached
			continue
		}
		scratch.points = append(scratch.points, point)
		scratch.indexes = append(scratch.indexes, i)
	}
	if len(scratch.points) == 0 {
		return
	}
	if cap(scratch.biomes) < len(scratch.points) {
		scratch.biomes = make([]Biome, len(scratch.points))
	} else {
		scratch.biomes = scratch.biomes[:len(scratch.points)]
	}
	if err := s.accelerator.SampleBiomes(scratch.points, scratch.biomes); err != nil {
		for _, index := range scratch.indexes {
			point := points[index]
			dst[index] = s.GetBiome(point.BlockX, point.BlockY, point.BlockZ)
		}
		return
	}
	for i, index := range scratch.indexes {
		biome := scratch.biomes[i]
		point := points[index]
		s.cache.Store(biomeCacheKey(point.BlockX, point.BlockY, point.BlockZ), biome)
		dst[index] = biome
	}
}

func (s *presetBiomeSource) SampleClimate(x, y, z int) [6]int64 {
	if s.graph != nil {
		ctx := FunctionContext{BlockX: x, BlockY: y, BlockZ: z}
		var scratch *EvalScratch
		if s.scratchPool != nil {
			scratch = s.scratchPool.Get().(*EvalScratch)
			defer s.scratchPool.Put(scratch)
		} else {
			scratch = NewEvalScratch(s.graph)
		}
		scratch.reset()
		var climate [6]int64
		for i, root := range s.graphRoots {
			// Climate.target casts to float and quantizeCoord multiplies in
			// float32 before truncating. Keep one memoization generation for all
			// six roots: their shared shift noises and splines are pure at ctx.
			climate[i] = int64(float32(s.graph.evalNormal(root, ctx, s.noises, nil, nil, scratch)) * 10000.0)
		}
		return climate
	}
	return s.noise.SampleClimate(x, y, z)
}

func (s *presetBiomeSource) GetBiome(x, y, z int) Biome {
	key := biomeCacheKey(x, y, z)
	if cached, ok := s.cache.Load(key); ok {
		return cached
	}
	climate := s.SampleClimate(x, y, z)
	var biome Biome
	switch s.preset {
	case "overworld":
		biome = s.rtree.Lookup(climate)
	case "nether":
		biome = lookupPresetBiome(climate, netherPresetPoints)
	default:
		biome = BiomePlains
	}
	s.cache.Store(key, biome)
	return biome
}

func (s *endBiomeSource) SampleClimate(x, y, z int) [6]int64 {
	var climate [6]int64
	climate[erosionIdx] = int64(s.erosion.Sample(x, z) * 10000.0)
	return climate
}

func (s *endBiomeSource) GetBiome(x, y, z int) Biome {
	key := biomeCacheKey(x, y, z)
	if cached, ok := s.cache.Load(key); ok {
		return cached
	}
	chunkX := x >> 4
	chunkZ := z >> 4
	if int64(chunkX)*int64(chunkX)+int64(chunkZ)*int64(chunkZ) <= 4096 {
		s.cache.Store(key, BiomeTheEnd)
		return BiomeTheEnd
	}

	weirdBlockX := ((x>>4)*2 + 1) * 8
	weirdBlockZ := ((z>>4)*2 + 1) * 8
	heightValue := s.erosion.Sample(weirdBlockX, weirdBlockZ)
	var biome Biome
	switch {
	case heightValue > 0.25:
		biome = BiomeEndHighlands
	case heightValue >= -0.0625:
		biome = BiomeEndMidlands
	case heightValue < -0.21875:
		biome = BiomeSmallEndIslands
	default:
		biome = BiomeEndBarrens
	}
	s.cache.Store(key, biome)
	return biome
}

func climateSpan(min, max float64) climateParameter {
	return climateParameter{min: int64(min * 10000.0), max: int64(max * 10000.0)}
}

func climatePoint(value float64) climateParameter {
	return climateSpan(value, value)
}

func (p climateParameter) distance(value int64) int64 {
	if value < p.min {
		return p.min - value
	}
	if value > p.max {
		return value - p.max
	}
	return 0
}

func lookupPresetBiome(climate [6]int64, points []climateParameterPoint) Biome {
	if len(points) == 0 {
		return BiomePlains
	}
	best := points[0]
	bestFitness := climatePointFitness(climate, points[0])
	for _, point := range points[1:] {
		fitness, better := climatePointFitnessBelow(climate, point, bestFitness)
		if better {
			best = point
			bestFitness = fitness
		}
	}
	return best.biome
}

func climatePointFitness(climate [6]int64, point climateParameterPoint) int64 {
	total := point.offset * point.offset
	total += climateDeltaSquared(climate[continentalnessIdx], point.params[continentalnessIdx])
	total += climateDeltaSquared(climate[erosionIdx], point.params[erosionIdx])
	total += climateDeltaSquared(climate[weirdnessIdx], point.params[weirdnessIdx])
	total += climateDeltaSquared(climate[temperatureIdx], point.params[temperatureIdx])
	total += climateDeltaSquared(climate[humidityIdx], point.params[humidityIdx])
	total += climateDeltaSquared(climate[depthIdx], point.params[depthIdx])
	return total
}

func climatePointFitnessBelow(climate [6]int64, point climateParameterPoint, limit int64) (int64, bool) {
	total := point.offset * point.offset
	if total >= limit {
		return total, false
	}
	total += climateDeltaSquared(climate[continentalnessIdx], point.params[continentalnessIdx])
	if total >= limit {
		return total, false
	}
	total += climateDeltaSquared(climate[erosionIdx], point.params[erosionIdx])
	if total >= limit {
		return total, false
	}
	total += climateDeltaSquared(climate[weirdnessIdx], point.params[weirdnessIdx])
	if total >= limit {
		return total, false
	}
	total += climateDeltaSquared(climate[temperatureIdx], point.params[temperatureIdx])
	if total >= limit {
		return total, false
	}
	total += climateDeltaSquared(climate[humidityIdx], point.params[humidityIdx])
	if total >= limit {
		return total, false
	}
	total += climateDeltaSquared(climate[depthIdx], point.params[depthIdx])
	return total, total < limit
}

func climateDeltaSquared(value int64, parameter climateParameter) int64 {
	if value < parameter.min {
		delta := parameter.min - value
		return delta * delta
	}
	if value > parameter.max {
		delta := value - parameter.max
		return delta * delta
	}
	return 0
}

var netherPresetPoints = []climateParameterPoint{
	{
		params: [6]climateParameter{
			climatePoint(0.0),
			climatePoint(0.0),
			climatePoint(0.0),
			climatePoint(0.0),
			climatePoint(0.0),
			climatePoint(0.0),
		},
		biome: BiomeNetherWastes,
	},
	{
		params: [6]climateParameter{
			climatePoint(0.0),
			climatePoint(-0.5),
			climatePoint(0.0),
			climatePoint(0.0),
			climatePoint(0.0),
			climatePoint(0.0),
		},
		biome: BiomeSoulSandValley,
	},
	{
		params: [6]climateParameter{
			climatePoint(0.4),
			climatePoint(0.0),
			climatePoint(0.0),
			climatePoint(0.0),
			climatePoint(0.0),
			climatePoint(0.0),
		},
		biome: BiomeCrimsonForest,
	},
	{
		params: [6]climateParameter{
			climatePoint(0.0),
			climatePoint(0.5),
			climatePoint(0.0),
			climatePoint(0.0),
			climatePoint(0.0),
			climatePoint(0.0),
		},
		offset: int64(0.375 * 10000.0),
		biome:  BiomeWarpedForest,
	},
	{
		params: [6]climateParameter{
			climatePoint(-0.5),
			climatePoint(0.0),
			climatePoint(0.0),
			climatePoint(0.0),
			climatePoint(0.0),
			climatePoint(0.0),
		},
		offset: int64(0.175 * 10000.0),
		biome:  BiomeBasaltDeltas,
	},
}

// PossibleBiomes mirrors BiomeSource.possibleBiomes: the distinct biomes in
// parameter-list order (first appearance), which feeds FeatureSorter and
// therefore the feature seed indices.
func PossibleBiomes(dimension string) []Biome {
	switch normalizeIdentifier(dimension) {
	case "overworld":
		return distinctPointBiomes(overworldBiomePoints)
	case "nether":
		return distinctPointBiomes(netherPresetPoints)
	case "end":
		// TheEndBiomeSource constructor order.
		return []Biome{BiomeTheEnd, BiomeEndHighlands, BiomeEndMidlands, BiomeSmallEndIslands, BiomeEndBarrens}
	default:
		return nil
	}
}

func distinctPointBiomes(points []climateParameterPoint) []Biome {
	seen := make(map[Biome]struct{}, 64)
	out := make([]Biome, 0, 64)
	for _, p := range points {
		if _, ok := seen[p.biome]; ok {
			continue
		}
		seen[p.biome] = struct{}{}
		out = append(out, p.biome)
	}
	return out
}
