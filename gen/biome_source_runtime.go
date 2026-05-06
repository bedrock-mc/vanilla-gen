package gen

import (
	"fmt"
	"sync"
)

type BiomeSource interface {
	SampleClimate(x, y, z int) [6]int64
	GetBiome(x, y, z int) Biome
}

type presetBiomeSource struct {
	preset string
	noise  BiomeNoise
	cache  sync.Map
}

type endBiomeSource struct {
	erosion EndIslandDensity
	cache   sync.Map
}

type biomeCoordKey struct {
	x int
	y int
	z int
}

func biomeCacheKey(x, y, z int) biomeCoordKey {
	return biomeCoordKey{x: x >> 2, y: y >> 2, z: z >> 2}
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
		return &presetBiomeSource{preset: def.Preset, noise: NewBiomeNoise(seed)}, nil
	case "nether":
		return &presetBiomeSource{preset: def.Preset, noise: NewBiomeNoise(seed)}, nil
	default:
		return nil, fmt.Errorf("unsupported biome source preset %q", def.Preset)
	}
}

func (s *presetBiomeSource) SampleClimate(x, y, z int) [6]int64 {
	return s.noise.SampleClimate(x, y, z)
}

func (s *presetBiomeSource) GetBiome(x, y, z int) Biome {
	key := biomeCacheKey(x, y, z)
	if cached, ok := s.cache.Load(key); ok {
		return cached.(Biome)
	}
	climate := s.SampleClimate(x, y, z)
	var biome Biome
	switch s.preset {
	case "overworld":
		biome = lookupOverworldPresetBiome(climate)
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
		return cached.(Biome)
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
