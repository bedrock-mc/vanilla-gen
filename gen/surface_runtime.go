package gen

import (
	"math"
	"sync"
)

const surfaceNoWaterHeight = -1 << 31

type SurfaceContext struct {
	BlockX int
	BlockY int
	BlockZ int

	SurfaceDepth     int
	SurfaceSecondary float64
	WaterHeight      int
	StoneDepthAbove  int
	StoneDepthBelow  int
	Steep            bool
	Biome            Biome
	// BiomeFunc lazily resolves the zoomed biome like vanilla's memoized
	// Context.biome supplier; when set it takes precedence over Biome.
	BiomeFunc       func() Biome
	MinSurfaceLevel int
	MinY            int
	MaxY            int
}

type SurfaceRuntime struct {
	seed              int64
	seaLevel          int
	noises            *NoiseRegistry
	biomeSource       BiomeSource
	noiseRandom       PositionalRandomFactory
	gradientMu        sync.Mutex
	gradientFactories map[string]PositionalRandomFactory
	rule              *surfaceRule
	useBandlands      bool
	bandlands         surfaceBandlands
}

// NoiseRandomAt exposes the SurfaceSystem noiseRandom positional factory.
func (s *SurfaceRuntime) NoiseRandomAt(x, y, z int) Xoroshiro128 {
	return s.noiseRandom.At(x, y, z)
}

// namedRandomFactory mirrors RandomState.getOrCreateRandomFactory.
func (s *SurfaceRuntime) namedRandomFactory(name string) PositionalRandomFactory {
	s.gradientMu.Lock()
	defer s.gradientMu.Unlock()
	if factory, ok := s.gradientFactories[name]; ok {
		return factory
	}
	rng := s.noiseRandom.FromHashOf(name)
	factory := NewPositionalRandomFactoryFromSeeds(int64(rng.NextLong()), int64(rng.NextLong()))
	s.gradientFactories[name] = factory
	return factory
}

// gradientRandomName recovers the vanilla random_name for the vertical
// gradient conditions; the generated rule data dropped the name but the three
// vanilla gradients are uniquely identified by their anchor kinds.
func gradientRandomName(condition *surfaceCondition) string {
	switch condition.trueAtAndBelow.kind {
	case surfaceAnchorAboveBottom:
		return "minecraft:bedrock_floor"
	case surfaceAnchorBelowTop:
		return "minecraft:bedrock_roof"
	default:
		return "minecraft:deepslate"
	}
}

type surfaceRuntimeLookup func(name string, properties map[string]string) uint32

type surfaceRuleKind uint8

const (
	surfaceRuleSequence surfaceRuleKind = iota
	surfaceRuleCondition
	surfaceRuleBlock
	surfaceRuleBandlands
)

type surfaceConditionKind uint8

const (
	surfaceConditionBiome surfaceConditionKind = iota
	surfaceConditionStoneDepth
	surfaceConditionYAbove
	surfaceConditionWater
	surfaceConditionNoiseThreshold
	surfaceConditionVerticalGradient
	surfaceConditionSteep
	surfaceConditionHole
	surfaceConditionAbovePreliminarySurface
	surfaceConditionTemperature
	surfaceConditionNot
)

type surfaceCaveSurface uint8

const (
	surfaceFloor surfaceCaveSurface = iota
	surfaceCeiling
)

type surfaceAnchorKind uint8

const (
	surfaceAnchorAbsolute surfaceAnchorKind = iota
	surfaceAnchorAboveBottom
	surfaceAnchorBelowTop
)

type surfaceVerticalAnchor struct {
	kind  surfaceAnchorKind
	value int
}

func (a surfaceVerticalAnchor) resolve(minY, maxY int) int {
	switch a.kind {
	case surfaceAnchorAboveBottom:
		return minY + a.value
	case surfaceAnchorBelowTop:
		return maxY - a.value
	default:
		return a.value
	}
}

type surfaceBlockState struct {
	name       string
	properties map[string]string
}

type surfaceCondition struct {
	kind surfaceConditionKind

	biomes []Biome

	offset              int
	addSurfaceDepth     bool
	secondaryDepthRange int
	caveSurface         surfaceCaveSurface

	anchor                 surfaceVerticalAnchor
	surfaceDepthMultiplier int
	addStoneDepth          bool

	noise        NoiseRef
	minThreshold float64
	maxThreshold float64

	trueAtAndBelow  surfaceVerticalAnchor
	falseAtAndAbove surfaceVerticalAnchor

	inner *surfaceCondition
}

type surfaceRule struct {
	kind surfaceRuleKind

	sequence []*surfaceRule
	ifTrue   *surfaceCondition
	thenRun  *surfaceRule
	block    surfaceBlockState
}

type surfaceBandlands struct {
	bands [192]surfaceBlockState
}

func NewSurfaceRuntime(seed int64, seaLevel int, noises *NoiseRegistry, biomeSource BiomeSource, rule *surfaceRule, useBandlands bool) *SurfaceRuntime {
	// Mirrors SurfaceSystem: the surface noises come from the shared noise
	// registry and noiseRandom is RandomState's master positional factory.
	factory := NewPositionalRandomFactory(seed)
	return &SurfaceRuntime{
		seed:              seed,
		seaLevel:          seaLevel,
		noises:            noises,
		biomeSource:       biomeSource,
		noiseRandom:       factory,
		gradientFactories: map[string]PositionalRandomFactory{},
		rule:              rule,
		useBandlands:      useBandlands,
		bandlands:         newSurfaceBandlands(factory),
	}
}

func NewOverworldSurfaceRuntime(seed int64, noises *NoiseRegistry, biomeSource BiomeSource) *SurfaceRuntime {
	return NewSurfaceRuntime(seed, 63, noises, biomeSource, overworldSurfaceRule, true)
}

func NewNetherSurfaceRuntime(seed int64, noises *NoiseRegistry, biomeSource BiomeSource) *SurfaceRuntime {
	return NewSurfaceRuntime(seed, 32, noises, biomeSource, netherSurfaceRule, false)
}

func NewEndSurfaceRuntime(seed int64, noises *NoiseRegistry, biomeSource BiomeSource) *SurfaceRuntime {
	return NewSurfaceRuntime(seed, 0, noises, biomeSource, endSurfaceRule, false)
}

// SurfaceDepth mirrors SurfaceSystem.getSurfaceDepth.
func (s *SurfaceRuntime) SurfaceDepth(x, z int) int {
	noiseValue := s.noises.Sample(NoiseSurface, float64(x), 0.0, float64(z))
	rng := s.noiseRandom.At(x, 0, z)
	return int(noiseValue*2.75 + 3.0 + rng.NextDouble()*0.25)
}

// SurfaceSecondary mirrors SurfaceSystem.getSurfaceSecondary (raw noise).
func (s *SurfaceRuntime) SurfaceSecondary(x, z int) float64 {
	return s.noises.Sample(NoiseSurfaceSecondary, float64(x), 0.0, float64(z))
}

func (s *SurfaceRuntime) TryApply(ctx SurfaceContext, lookup func(name string, properties map[string]string) uint32) (uint32, bool) {
	if lookup == nil {
		return 0, false
	}
	return s.evalRule(s.rule, ctx, lookup)
}

func (s *SurfaceRuntime) evalRule(rule *surfaceRule, ctx SurfaceContext, lookup surfaceRuntimeLookup) (uint32, bool) {
	if rule == nil {
		return 0, false
	}
	switch rule.kind {
	case surfaceRuleSequence:
		for _, child := range rule.sequence {
			if rid, ok := s.evalRule(child, ctx, lookup); ok {
				return rid, true
			}
		}
		return 0, false
	case surfaceRuleCondition:
		if s.evalCondition(rule.ifTrue, ctx) {
			return s.evalRule(rule.thenRun, ctx, lookup)
		}
		return 0, false
	case surfaceRuleBlock:
		return lookup(rule.block.name, rule.block.properties), true
	case surfaceRuleBandlands:
		if !s.useBandlands {
			return 0, false
		}
		state := s.bandlandsState(ctx)
		return lookup(state.name, state.properties), true
	default:
		return 0, false
	}
}

func (s *SurfaceRuntime) evalCondition(condition *surfaceCondition, ctx SurfaceContext) bool {
	if condition == nil {
		return false
	}
	switch condition.kind {
	case surfaceConditionBiome:
		ctxBiome := ctx.Biome
		if ctx.BiomeFunc != nil {
			ctxBiome = ctx.BiomeFunc()
		}
		for _, biome := range condition.biomes {
			if biome == ctxBiome {
				return true
			}
		}
		return false
	case surfaceConditionStoneDepth:
		depth := ctx.StoneDepthAbove
		if condition.caveSurface == surfaceCeiling {
			depth = ctx.StoneDepthBelow
		}
		threshold := 1 + condition.offset
		if condition.addSurfaceDepth {
			threshold += ctx.SurfaceDepth
		}
		if condition.secondaryDepthRange != 0 {
			// Mth.map(surfaceSecondary, -1, 1, 0, range) over the raw noise.
			t := (ctx.SurfaceSecondary - (-1.0)) / (1.0 - (-1.0))
			threshold += int(t * float64(condition.secondaryDepthRange))
		}
		return depth <= threshold
	case surfaceConditionYAbove:
		targetY := condition.anchor.resolve(ctx.MinY, ctx.MaxY)
		blockY := ctx.BlockY
		if condition.addStoneDepth {
			blockY += ctx.StoneDepthAbove
		}
		return blockY >= targetY+ctx.SurfaceDepth*condition.surfaceDepthMultiplier
	case surfaceConditionWater:
		if ctx.WaterHeight == surfaceNoWaterHeight {
			return true
		}
		blockY := ctx.BlockY
		if condition.addStoneDepth {
			blockY += ctx.StoneDepthAbove
		}
		return blockY >= ctx.WaterHeight+condition.offset+ctx.SurfaceDepth*condition.surfaceDepthMultiplier
	case surfaceConditionNoiseThreshold:
		if s.noises == nil {
			return false
		}
		value := s.noises.Sample(condition.noise, float64(ctx.BlockX), 0.0, float64(ctx.BlockZ))
		return value >= condition.minThreshold && value <= condition.maxThreshold
	case surfaceConditionVerticalGradient:
		trueY := condition.trueAtAndBelow.resolve(ctx.MinY, ctx.MaxY)
		falseY := condition.falseAtAndAbove.resolve(ctx.MinY, ctx.MaxY)
		if ctx.BlockY <= trueY {
			return true
		}
		if ctx.BlockY >= falseY {
			return false
		}
		// Mth.map(y, trueY, falseY, 1.0, 0.0) with a positional random forked
		// from the gradient's random_name, like VerticalGradientConditionSource.
		t := (float64(ctx.BlockY) - float64(trueY)) / (float64(falseY) - float64(trueY))
		probability := 1.0 + t*(0.0-1.0)
		rng := s.namedRandomFactory(gradientRandomName(condition)).At(ctx.BlockX, ctx.BlockY, ctx.BlockZ)
		return float64(rng.NextFloat()) < probability
	case surfaceConditionSteep:
		return ctx.Steep
	case surfaceConditionHole:
		return ctx.SurfaceDepth <= 0
	case surfaceConditionAbovePreliminarySurface:
		return ctx.BlockY >= ctx.MinSurfaceLevel
	case surfaceConditionTemperature:
		// Mirrors TemperatureHelperCondition: Biome.coldEnoughToSnow.
		tempBiome := ctx.Biome
		if ctx.BiomeFunc != nil {
			tempBiome = ctx.BiomeFunc()
		}
		return BiomeColdEnoughToSnow(tempBiome, ctx.BlockX, ctx.BlockY, ctx.BlockZ, s.seaLevel)
	case surfaceConditionNot:
		return !s.evalCondition(condition.inner, ctx)
	default:
		return false
	}
}

func (s *SurfaceRuntime) temperatureValue(ctx SurfaceContext) float64 {
	climate := s.biomeSource.SampleClimate(ctx.BlockX, ctx.BlockY, ctx.BlockZ)
	return float64(climate[temperatureIdx]) / 10000.0
}

// newSurfaceBandlands mirrors SurfaceSystem.generateBands seeded from
// noiseRandom.fromHashOf("minecraft:clay_bands").
func newSurfaceBandlands(factory PositionalRandomFactory) surfaceBandlands {
	rng := factory.FromHashOf("minecraft:clay_bands")
	var bands [192]surfaceBlockState
	for i := range bands {
		bands[i] = surfaceBlockState{name: "minecraft:terracotta"}
	}

	for i := 0; i < len(bands); i++ {
		i += int(rng.NextInt(5)) + 1
		if i < len(bands) {
			bands[i] = surfaceBlockState{name: "minecraft:orange_terracotta"}
		}
	}

	makeClayBands(&rng, &bands, 1, "minecraft:yellow_terracotta")
	makeClayBands(&rng, &bands, 2, "minecraft:brown_terracotta")
	makeClayBands(&rng, &bands, 1, "minecraft:red_terracotta")

	whiteBandCount := int(rng.NextInt(15-9+1)) + 9
	placed := 0
	for start := 0; placed < whiteBandCount && start < len(bands); start += int(rng.NextInt(16)) + 4 {
		bands[start] = surfaceBlockState{name: "minecraft:white_terracotta"}
		if start-1 > 0 && rng.NextBool() {
			bands[start-1] = surfaceBlockState{name: "minecraft:light_gray_terracotta"}
		}
		if start+1 < len(bands) && rng.NextBool() {
			bands[start+1] = surfaceBlockState{name: "minecraft:light_gray_terracotta"}
		}
		placed++
	}

	return surfaceBandlands{bands: bands}
}

func makeClayBands(rng *Xoroshiro128, bands *[192]surfaceBlockState, baseWidth int, name string) {
	bandCount := int(rng.NextInt(15-6+1)) + 6
	for i := 0; i < bandCount; i++ {
		width := baseWidth + int(rng.NextInt(3))
		start := int(rng.NextInt(uint32(len(bands))))
		for p := 0; start+p < len(bands) && p < width; p++ {
			bands[start+p] = surfaceBlockState{name: name}
		}
	}
}

// bandlandsState mirrors SurfaceSystem.getBand, including Java's
// floor(x+0.5) rounding.
func (s *SurfaceRuntime) bandlandsState(ctx SurfaceContext) surfaceBlockState {
	offsetNoise := s.noises.Sample(NoiseClayBandsOffset, float64(ctx.BlockX), 0.0, float64(ctx.BlockZ))
	offset := int(math.Floor(offsetNoise*4.0 + 0.5))
	index := (ctx.BlockY + offset + len(s.bandlands.bands)) % len(s.bandlands.bands)
	if index < 0 {
		index += len(s.bandlands.bands)
	}
	return s.bandlands.bands[index]
}
