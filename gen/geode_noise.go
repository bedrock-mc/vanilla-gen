package gen

import "math"

// GeodeNoise mirrors the NormalNoise GeodeFeature builds per placement:
// NormalNoise.create(new WorldgenRandom(new LegacyRandomSource(level.getSeed())),
// -4, 1.0). The two PerlinNoise halves are seeded through the legacy
// positional fork (nextLong per PerlinNoise, then hash of "octave_-4").
type GeodeNoise struct {
	first  PerlinNoise
	second PerlinNoise
}

const (
	geodeNoiseInputFactor = 1.0181268882175227
	// firstOctave -4 with a single non-zero amplitude: octave span 0, so
	// valueFactor = (1/6) / (0.1 * (1 + 1/(0+1))).
	geodeNoiseValueFactor = 0.16666666666666666 / 0.2
	// 2^firstOctave for firstOctave = -4.
	geodeNoiseFreqFactor = 0.0625
)

// javaStringHash mirrors java.lang.String.hashCode.
func javaStringHash(s string) int32 {
	var h int32
	for _, c := range []byte(s) {
		h = 31*h + int32(c)
	}
	return h
}

// newLegacyImprovedNoise mirrors ImprovedNoise(RandomSource) with a legacy
// (java.util.Random) source: three nextDouble offsets and a forward swap
// shuffle of the permutation table.
func newLegacyImprovedNoise(rng *LegacyRandom) PerlinNoise {
	a := rng.NextDouble() * 256
	b := rng.NextDouble() * 256
	c := rng.NextDouble() * 256

	var d [257]int32
	for i := 0; i < 256; i++ {
		d[i] = int32(i)
	}
	for i := 0; i < 256; i++ {
		j := rng.NextInt(256-i) + i
		d[i], d[j] = d[j], d[i]
	}
	d[256] = d[0]

	// Precompute the y==0 fast path exactly like NewPerlinNoise.
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

// NewGeodeNoise builds the geode noise for a world seed.
func NewGeodeNoise(seed int64) GeodeNoise {
	parent := NewLegacyRandom(seed)
	hash := int64(javaStringHash("octave_-4"))

	// PerlinNoise.create -> random.forkPositional() consumes one nextLong
	// per PerlinNoise; fromHashOf xors the string hash into that fork seed.
	fork1 := NewLegacyRandom(hash ^ parent.NextLong())
	first := newLegacyImprovedNoise(&fork1)
	fork2 := NewLegacyRandom(hash ^ parent.NextLong())
	second := newLegacyImprovedNoise(&fork2)

	return GeodeNoise{first: first, second: second}
}

// GetValue mirrors NormalNoise.getValue for firstOctave -4, amplitudes [1.0].
func (n *GeodeNoise) GetValue(x, y, z float64) float64 {
	x2 := x * geodeNoiseInputFactor
	y2 := y * geodeNoiseInputFactor
	z2 := z * geodeNoiseInputFactor
	v1 := n.first.Sample(wrapCoord(x*geodeNoiseFreqFactor), wrapCoord(y*geodeNoiseFreqFactor), wrapCoord(z*geodeNoiseFreqFactor))
	v2 := n.second.Sample(wrapCoord(x2*geodeNoiseFreqFactor), wrapCoord(y2*geodeNoiseFreqFactor), wrapCoord(z2*geodeNoiseFreqFactor))
	return (v1 + v2) * geodeNoiseValueFactor
}
