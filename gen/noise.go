package gen

import (
	"math"
	"strconv"
)

func itoaSigned(v int) string { return strconv.Itoa(v) }

type NoiseRegistry struct {
	noises       []DoublePerlinNoise
	blendedNoise BlendedNoise
	endIslands   EndIslandDensity
}

// noiseIDs maps NoiseRef ordinals to the vanilla worldgen/noise registry ids,
// in the same alphabetical order as the generated NoiseParams table.
var noiseIDs = [...]string{
	"aquifer_barrier",
	"aquifer_fluid_level_floodedness",
	"aquifer_fluid_level_spread",
	"aquifer_lava",
	"badlands_pillar",
	"badlands_pillar_roof",
	"badlands_surface",
	"calcite",
	"cave_cheese",
	"cave_entrance",
	"cave_layer",
	"clay_bands_offset",
	"continentalness",
	"continentalness_large",
	"erosion",
	"erosion_large",
	"gravel",
	"gravel_layer",
	"ice",
	"iceberg_pillar",
	"iceberg_pillar_roof",
	"iceberg_surface",
	"jagged",
	"nether_state_selector",
	"nether_wart",
	"netherrack",
	"noodle",
	"noodle_ridge_a",
	"noodle_ridge_b",
	"noodle_thickness",
	"offset",
	"ore_gap",
	"ore_vein_a",
	"ore_vein_b",
	"ore_veininess",
	"packed_ice",
	"patch",
	"pillar",
	"pillar_rareness",
	"pillar_thickness",
	"powder_snow",
	"ridge",
	"soul_sand_layer",
	"spaghetti_2d",
	"spaghetti_2d_elevation",
	"spaghetti_2d_modulator",
	"spaghetti_2d_thickness",
	"spaghetti_3d_1",
	"spaghetti_3d_2",
	"spaghetti_3d_rarity",
	"spaghetti_3d_thickness",
	"spaghetti_roughness",
	"spaghetti_roughness_modulator",
	"surface",
	"surface_secondary",
	"surface_swamp",
	"temperature",
	"temperature_large",
	"vegetation",
	"vegetation_large",
}

// NewNoiseRegistry mirrors RandomState: a positional random factory forked
// from XoroshiroRandomSource(seed), with every noise instantiated from
// factory.fromHashOf("minecraft:<id>") and the legacy blended terrain noise
// from factory.fromHashOf("minecraft:terrain").
func NewNoiseRegistry(seed int64) *NoiseRegistry {
	factory := NewPositionalRandomFactory(seed)

	noises := make([]DoublePerlinNoise, len(NoiseParams))
	for i, params := range NoiseParams {
		rng := factory.FromHashOf("minecraft:" + noiseIDs[i])
		noises[i] = NewDoublePerlinNoise(&rng, params.Amplitudes, params.FirstOctave)
	}

	blendedRng := factory.FromHashOf("minecraft:terrain")
	return &NoiseRegistry{
		noises: noises,
		blendedNoise: NewBlendedNoise(
			&blendedRng,
			0.25,
			0.125,
			80.0,
			160.0,
			8.0,
		),
		endIslands: NewEndIslandDensity(seed),
	}
}

func (n *NoiseRegistry) Sample(noise NoiseRef, x, y, z float64) float64 {
	idx := int(noise)
	if idx < 0 || idx >= len(n.noises) {
		return 0
	}
	return n.noises[idx].Sample(x, y, z)
}

func (n *NoiseRegistry) SampleBlendedNoise(x, y, z, _xzScale, _yScale, _xzFactor, _yFactor, _smearScaleMultiplier float64) float64 {
	return n.blendedNoise.Sample(x, y, z)
}

func (n *NoiseRegistry) SampleEndIslands(blockX, blockZ int) float64 {
	return n.endIslands.Sample(blockX, blockZ)
}

type PerlinNoise struct {
	d           [257]int32
	a           float64
	b           float64
	c           float64
	amplitude   float64
	valueFactor float64
	lacunarity  float64
	h2          int32
	d2          float64
	t2          float64
}

func NewPerlinNoise(rng *Xoroshiro128) PerlinNoise {
	a := rng.NextDouble() * 256
	b := rng.NextDouble() * 256
	c := rng.NextDouble() * 256

	var d [257]int32
	for i := 0; i < 256; i++ {
		d[i] = int32(i)
	}
	for i := 0; i < 256; i++ {
		j := int(rng.NextInt(uint32(256-i))) + i
		d[i], d[j] = d[j], d[i]
	}
	d[256] = d[0]

	i2 := math.Floor(b)
	d2 := b - i2
	h2 := int32(i2) & 255
	t2 := smoothstep(d2)

	return PerlinNoise{
		d:          d,
		a:          a,
		b:          b,
		c:          c,
		amplitude:  1,
		lacunarity: 1,
		h2:         h2,
		d2:         d2,
		t2:         t2,
	}
}

func (p *PerlinNoise) Sample(x, y, z float64) float64 {
	var d2, t2 float64
	var h2 int32
	if y == 0 {
		d2, h2, t2 = p.d2, p.h2, p.t2
	} else {
		yv := y + p.b
		i2 := math.Floor(yv)
		d2 = yv - i2
		h2 = int32(i2) & 255
		t2 = smoothstep(d2)
	}

	d1 := x + p.a
	d3 := z + p.c
	i1 := math.Floor(d1)
	i3 := math.Floor(d3)
	d1 -= i1
	d3 -= i3

	h1 := int32(i1) & 255
	h3 := int32(i3) & 255
	t1 := smoothstep(d1)
	t3 := smoothstep(d3)

	idx := p.d[:]
	a1 := (idx[h1] + h2) & 255
	b1 := (idx[(h1+1)&255] + h2) & 255
	a2 := (idx[a1] + h3) & 255
	a3 := (idx[(a1+1)&255] + h3) & 255
	b2 := (idx[b1] + h3) & 255
	b3 := (idx[(b1+1)&255] + h3) & 255

	l1 := indexedLerp(idx[a2]&15, d1, d2, d3)
	l2 := indexedLerp(idx[b2]&15, d1-1, d2, d3)
	l3 := indexedLerp(idx[a3]&15, d1, d2-1, d3)
	l4 := indexedLerp(idx[b3]&15, d1-1, d2-1, d3)
	l5 := indexedLerp(idx[(a2+1)&255]&15, d1, d2, d3-1)
	l6 := indexedLerp(idx[(b2+1)&255]&15, d1-1, d2, d3-1)
	l7 := indexedLerp(idx[(a3+1)&255]&15, d1, d2-1, d3-1)
	l8 := indexedLerp(idx[(b3+1)&255]&15, d1-1, d2-1, d3-1)

	l1 = lerp(t1, l1, l2)
	l3 = lerp(t1, l3, l4)
	l5 = lerp(t1, l5, l6)
	l7 = lerp(t1, l7, l8)
	l1 = lerp(t2, l1, l3)
	l5 = lerp(t2, l5, l7)
	return lerp(t3, l1, l5)
}

func (p *PerlinNoise) SampleSmeared(x, y, z, yScale, yOrig float64) float64 {
	d1 := x + p.a
	d2Raw := y + p.b
	d3 := z + p.c

	i1 := math.Floor(d1)
	i2 := math.Floor(d2Raw)
	i3 := math.Floor(d3)

	d1 -= i1
	d2 := d2Raw - i2
	d3 -= i3

	s := 0.0
	if yScale != 0 {
		r := d2
		if yOrig >= 0 && yOrig < d2 {
			r = yOrig
		}
		s = math.Floor(r/yScale+1.0e-7) * yScale
	}
	d2Smeared := d2 - s

	h1 := int32(i1) & 255
	h2 := int32(i2) & 255
	h3 := int32(i3) & 255

	t1 := smoothstep(d1)
	t2 := smoothstep(d2)
	t3 := smoothstep(d3)

	idx := p.d[:]
	a1 := (idx[h1] + h2) & 255
	b1 := (idx[(h1+1)&255] + h2) & 255
	a2 := (idx[a1] + h3) & 255
	a3 := (idx[(a1+1)&255] + h3) & 255
	b2 := (idx[b1] + h3) & 255
	b3 := (idx[(b1+1)&255] + h3) & 255

	l1 := indexedLerp(idx[a2]&15, d1, d2Smeared, d3)
	l2 := indexedLerp(idx[b2]&15, d1-1, d2Smeared, d3)
	l3 := indexedLerp(idx[a3]&15, d1, d2Smeared-1, d3)
	l4 := indexedLerp(idx[b3]&15, d1-1, d2Smeared-1, d3)
	l5 := indexedLerp(idx[(a2+1)&255]&15, d1, d2Smeared, d3-1)
	l6 := indexedLerp(idx[(b2+1)&255]&15, d1-1, d2Smeared, d3-1)
	l7 := indexedLerp(idx[(a3+1)&255]&15, d1, d2Smeared-1, d3-1)
	l8 := indexedLerp(idx[(b3+1)&255]&15, d1-1, d2Smeared-1, d3-1)

	l1 = lerp(t1, l1, l2)
	l3 = lerp(t1, l3, l4)
	l5 = lerp(t1, l5, l6)
	l7 = lerp(t1, l7, l8)
	l1 = lerp(t2, l1, l3)
	l5 = lerp(t2, l5, l7)
	return lerp(t3, l1, l5)
}

type OctaveNoise struct {
	octaves []PerlinNoise
}

func NewOctaveNoise(rng *Xoroshiro128, amplitudes []float64, omin int) OctaveNoise {
	// Mirrors PerlinNoise: lowestFreqInputFactor = 2^firstOctave,
	// lowestFreqValueFactor = 2^(n-1) / (2^n - 1).
	lacuna := math.Pow(2.0, float64(omin))
	persist := math.Pow(2.0, float64(len(amplitudes)-1)) / (math.Pow(2.0, float64(len(amplitudes))) - 1.0)

	xLo := rng.NextLong()
	xHi := rng.NextLong()
	octaves := make([]PerlinNoise, 0, len(amplitudes))

	for i, amp := range amplitudes {
		if amp != 0 {
			lo, hi := seedFromHashOf("octave_" + itoaSigned(omin+i))
			pxr := NewXoroshiro128FromState(xLo^lo, xHi^hi)
			noise := NewPerlinNoise(&pxr)
			noise.amplitude = amp
			noise.valueFactor = persist
			noise.lacunarity = lacuna
			octaves = append(octaves, noise)
		}
		lacuna *= 2.0
		persist /= 2.0
	}

	return OctaveNoise{octaves: octaves}
}

// Sample mirrors PerlinNoise.getValue: value += amplitude * noise * valueFactor
// with coordinates wrapped after the frequency multiply.
func (o *OctaveNoise) Sample(x, y, z float64) float64 {
	value := 0.0
	for i := range o.octaves {
		octave := &o.octaves[i]
		lf := octave.lacunarity
		noiseVal := octave.Sample(wrapCoord(x*lf), wrapCoord(y*lf), wrapCoord(z*lf))
		value += octave.amplitude * noiseVal * octave.valueFactor
	}
	return value
}

type DoublePerlinNoise struct {
	amplitude float64
	octA      OctaveNoise
	octB      OctaveNoise
}

func NewDoublePerlinNoise(rng *Xoroshiro128, amplitudes []float64, omin int) DoublePerlinNoise {
	octA := NewOctaveNoise(rng, amplitudes, omin)
	octB := NewOctaveNoise(rng, amplitudes, omin)

	// Mirrors NormalNoise: valueFactor = 1/6 / expectedDeviation(span) where
	// span is the index distance between the first and last non-zero
	// amplitudes.
	minOctave := len(amplitudes)
	maxOctave := -1
	for i, amp := range amplitudes {
		if amp != 0.0 {
			if i < minOctave {
				minOctave = i
			}
			if i > maxOctave {
				maxOctave = i
			}
		}
	}
	span := maxOctave - minOctave
	expectedDeviation := 0.1 * (1.0 + 1.0/float64(span+1))

	return DoublePerlinNoise{
		amplitude: 0.16666666666666666 / expectedDeviation,
		octA:      octA,
		octB:      octB,
	}
}

// Sample mirrors NormalNoise.getValue; the per-octave frequency (including
// 2^firstOctave) is applied inside OctaveNoise.Sample like vanilla.
func (d *DoublePerlinNoise) Sample(x, y, z float64) float64 {
	const inputFactor = 1.0181268882175227
	x2 := x * inputFactor
	y2 := y * inputFactor
	z2 := z * inputFactor
	return (d.octA.Sample(x, y, z) + d.octB.Sample(x2, y2, z2)) * d.amplitude
}

type BlendedNoise struct {
	minLimit        OctaveNoise
	maxLimit        OctaveNoise
	main            OctaveNoise
	xzMultiplier    float64
	yMultiplier     float64
	xzFactor        float64
	yFactor         float64
	limitSmearScale float64
	mainSmearScale  float64
}

func NewBlendedNoise(rng *Xoroshiro128, xzScale, yScale, xzFactor, yFactor, smearScaleMultiplier float64) BlendedNoise {
	const baseScale = 684.412
	minLimit := createLegacyOctaves(rng, 16)
	maxLimit := createLegacyOctaves(rng, 16)
	main := createLegacyOctaves(rng, 8)
	limitSmearScale := baseScale * yScale * smearScaleMultiplier

	return BlendedNoise{
		minLimit:        minLimit,
		maxLimit:        maxLimit,
		main:            main,
		xzMultiplier:    baseScale * xzScale,
		yMultiplier:     baseScale * yScale,
		xzFactor:        xzFactor,
		yFactor:         yFactor,
		limitSmearScale: limitSmearScale,
		mainSmearScale:  limitSmearScale / yFactor,
	}
}

func createLegacyOctaves(rng *Xoroshiro128, count int) OctaveNoise {
	octaves := make([]PerlinNoise, 0, count)
	for i := 0; i < count; i++ {
		noise := NewPerlinNoise(rng)
		octaves = append(octaves, noise)
	}
	return OctaveNoise{octaves: octaves}
}

func (b *BlendedNoise) Sample(x, y, z float64) float64 {
	dx := x * b.xzMultiplier
	dy := y * b.yMultiplier
	dz := z * b.xzMultiplier

	gx := dx / b.xzFactor
	gy := dy / b.yFactor
	gz := dz / b.xzFactor

	n := 0.0
	o := 1.0
	for i := 0; i < 8; i++ {
		if i < len(b.main.octaves) {
			n += b.main.octaves[i].SampleSmeared(
				wrapCoord(gx*o),
				wrapCoord(gy*o),
				wrapCoord(gz*o),
				b.mainSmearScale*o,
				gy*o,
			) / o
		}
		o /= 2
	}

	q := (n/10.0 + 1.0) / 2.0
	useMaxOnly := q >= 1
	useMinOnly := q <= 0

	l := 0.0
	m := 0.0
	o = 1.0
	for i := 0; i < 16; i++ {
		if !useMaxOnly && i < len(b.minLimit.octaves) {
			l += b.minLimit.octaves[i].SampleSmeared(
				wrapCoord(dx*o),
				wrapCoord(dy*o),
				wrapCoord(dz*o),
				b.limitSmearScale*o,
				dy*o,
			) / o
		}
		if !useMinOnly && i < len(b.maxLimit.octaves) {
			m += b.maxLimit.octaves[i].SampleSmeared(
				wrapCoord(dx*o),
				wrapCoord(dy*o),
				wrapCoord(dz*o),
				b.limitSmearScale*o,
				dy*o,
			) / o
		}
		o /= 2
	}

	return clampedLerp(q, l/512.0, m/512.0) / 128.0
}

// wrapCoord mirrors PerlinNoise.wrap: round-to-nearest multiple of 2^25.
func wrapCoord(value float64) float64 {
	const coordRange = 3.3554432e7
	return value - math.Floor(value/coordRange+0.5)*coordRange
}

func clampedLerp(t, a, b float64) float64 {
	t = clampFloat(t, 0, 1)
	return a + t*(b-a)
}

func smoothstep(d float64) float64 {
	return d * d * d * (d*(d*6-15) + 10)
}

// perlinGradients is SimplexNoise.GRADIENT; gradDot computes the exact
// vanilla dot product (branchless, including the +0*coordinate term).
var perlinGradients = [16][3]float64{
	{1, 1, 0}, {-1, 1, 0}, {1, -1, 0}, {-1, -1, 0},
	{1, 0, 1}, {-1, 0, 1}, {1, 0, -1}, {-1, 0, -1},
	{0, 1, 1}, {0, -1, 1}, {0, 1, -1}, {0, -1, -1},
	{1, 1, 0}, {0, -1, 1}, {-1, 1, 0}, {0, -1, -1},
}

func indexedLerp(idx int32, x, y, z float64) float64 {
	g := &perlinGradients[idx&15]
	return g[0]*x + g[1]*y + g[2]*z
}

func lerp(t, a, b float64) float64 {
	return a + t*(b-a)
}
