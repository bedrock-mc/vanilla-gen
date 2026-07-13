package gen

// Port of Biome's temperature machinery: the static PerlinSimplexNoise
// instances (fixed legacy seeds independent of the world seed), per-biome base
// temperatures from the vanilla biome definitions, and the frozen-ocean
// temperature modifier.

type perlinSimplexNoise struct {
	levels                 []*SimplexNoise
	highestFreqInputFactor float64
	highestFreqValueFactor float64
}

// newPerlinSimplexNoise mirrors PerlinSimplexNoise for the octave sets Biome
// uses: {0} and {-2,-1,0} (no positive octaves).
func newPerlinSimplexNoise(rng *LegacyRandom, octaves []int) *perlinSimplexNoise {
	contains := func(v int) bool {
		for _, o := range octaves {
			if o == v {
				return true
			}
		}
		return false
	}
	first := octaves[0]
	last := octaves[len(octaves)-1]
	lowFreq := -first
	highFreq := last
	count := lowFreq + highFreq + 1

	zero := NewSimplexNoise(rng)
	levels := make([]*SimplexNoise, count)
	if highFreq >= 0 && highFreq < count && contains(0) {
		levels[highFreq] = &zero
	}
	for i := highFreq + 1; i < count; i++ {
		if i >= 0 && contains(highFreq-i) {
			s := NewSimplexNoise(rng)
			levels[i] = &s
		} else {
			rng.ConsumeCount(262)
		}
	}

	return &perlinSimplexNoise{
		levels:                 levels,
		highestFreqInputFactor: pow2(highFreq),
		highestFreqValueFactor: 1.0 / (pow2(count) - 1.0),
	}
}

func pow2(n int) float64 {
	v := 1.0
	for i := 0; i < n; i++ {
		v *= 2.0
	}
	return v
}

func (p *perlinSimplexNoise) getValue(x, y float64) float64 {
	value := 0.0
	factor := p.highestFreqInputFactor
	valueFactor := p.highestFreqValueFactor
	for _, level := range p.levels {
		if level != nil {
			value += level.Sample2D(x*factor, y*factor) * valueFactor
		}
		factor /= 2.0
		valueFactor *= 2.0
	}
	return value
}

var (
	biomeTemperatureNoise       = newFixedSeedSimplex(1234, []int{0})
	biomeFrozenTemperatureNoise = newFixedSeedSimplex(3456, []int{-2, -1, 0})
	biomeInfoNoise              = newFixedSeedSimplex(2345, []int{0})
)

func newFixedSeedSimplex(seed int64, octaves []int) *perlinSimplexNoise {
	rng := NewLegacyRandom(seed)
	return newPerlinSimplexNoise(&rng, octaves)
}

var biomeBaseTemperature = map[Biome]float32{
	BiomeBadlands: 2.0, BiomeBambooJungle: 0.95, BiomeBasaltDeltas: 2.0,
	BiomeBeach: 0.8, BiomeBirchForest: 0.6, BiomeCherryGrove: 0.5,
	BiomeColdOcean: 0.5, BiomeCrimsonForest: 2.0, BiomeDarkForest: 0.7,
	BiomeDeepColdOcean: 0.5, BiomeDeepDark: 0.8, BiomeDeepFrozenOcean: 0.5,
	BiomeDeepLukewarmOcean: 0.5, BiomeDeepOcean: 0.5, BiomeDesert: 2.0,
	BiomeDripstoneCaves: 0.8, BiomeEndBarrens: 0.5, BiomeEndHighlands: 0.5,
	BiomeEndMidlands: 0.5, BiomeErodedBadlands: 2.0, BiomeFlowerForest: 0.7,
	BiomeForest: 0.7, BiomeFrozenOcean: 0.0, BiomeFrozenPeaks: -0.7,
	BiomeFrozenRiver: 0.0, BiomeGrove: -0.2, BiomeIceSpikes: 0.0,
	BiomeJaggedPeaks: -0.7, BiomeJungle: 0.95, BiomeLukewarmOcean: 0.5,
	BiomeLushCaves: 0.5, BiomeMangroveSwamp: 0.8, BiomeMeadow: 0.5,
	BiomeMushroomFields: 0.9, BiomeNetherWastes: 2.0, BiomeOcean: 0.5,
	BiomeTallBirchForest: 0.6, BiomeOldGrowthPineTaiga: 0.3,
	BiomeOldGrowthSpruceTaiga: 0.25, BiomePaleGarden: 0.7, BiomePlains: 0.8,
	BiomeRiver: 0.5, BiomeSavanna: 2.0, BiomeSavannaPlateau: 2.0,
	BiomeSmallEndIslands: 0.5, BiomeSnowyBeach: 0.05, BiomeSnowyPlains: 0.0,
	BiomeSnowySlopes: -0.3, BiomeSnowyTaiga: -0.5, BiomeSoulSandValley: 2.0,
	BiomeSparseJungle: 0.95, BiomeStonyPeaks: 1.0, BiomeStonyShore: 0.2,
	BiomeSunflowerPlains: 0.8, BiomeSwamp: 0.8, BiomeTaiga: 0.25,
	BiomeTheEnd: 0.5, BiomeTheVoid: 0.5, BiomeWarmOcean: 0.5,
	BiomeWarpedForest: 2.0, BiomeWindsweptForest: 0.2,
	BiomeGravellyMountains: 0.2, BiomeWindsweptHills: 0.2,
	BiomeWindsweptSavanna: 2.0, BiomeWoodedBadlands: 2.0,
}

func biomeHasFrozenModifier(biome Biome) bool {
	return biome == BiomeFrozenOcean || biome == BiomeDeepFrozenOcean
}

// biomeModifiedTemperature mirrors TemperatureModifier.modifyTemperature.
func biomeModifiedTemperature(biome Biome, x, z int) float32 {
	base, ok := biomeBaseTemperature[biome]
	if !ok {
		base = 0.5
	}
	if !biomeHasFrozenModifier(biome) {
		return base
	}
	largeVariation := biomeFrozenTemperatureNoise.getValue(float64(x)*0.05, float64(z)*0.05) * 7.0
	edgeVariation := biomeInfoNoise.getValue(float64(x)*0.2, float64(z)*0.2)
	if largeVariation+edgeVariation < 0.3 {
		smallVariation := biomeInfoNoise.getValue(float64(x)*0.09, float64(z)*0.09)
		if smallVariation < 0.8 {
			return 0.2
		}
	}
	return base
}

// BiomeTemperature mirrors Biome.getHeightAdjustedTemperature.
func BiomeTemperature(biome Biome, x, y, z, seaLevel int) float32 {
	adjusted := biomeModifiedTemperature(biome, x, z)
	snowLevel := seaLevel + 17
	if y > snowLevel {
		v := float32(biomeTemperatureNoise.getValue(float64(float32(x)/8.0), float64(float32(z)/8.0)) * 8.0)
		return adjusted - (v+float32(y)-float32(snowLevel))*0.05/40.0
	}
	return adjusted
}

// BiomeColdEnoughToSnow mirrors Biome.coldEnoughToSnow.
func BiomeColdEnoughToSnow(biome Biome, x, y, z, seaLevel int) bool {
	return BiomeTemperature(biome, x, y, z, seaLevel) < 0.15
}

// BiomeShouldMeltIceberg mirrors Biome.shouldMeltFrozenOceanIcebergSlightly.
func BiomeShouldMeltIceberg(biome Biome, x, y, z, seaLevel int) bool {
	return BiomeTemperature(biome, x, y, z, seaLevel) > 0.1
}

// BiomeInfoNoise exposes Biome.BIOME_INFO_NOISE for placement modifiers.
func BiomeInfoNoise(x, z float64) float64 {
	return biomeInfoNoise.getValue(x, z)
}
