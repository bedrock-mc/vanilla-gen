package gen

import "math"

// LegacyRandom mirrors java.util.Random / net.minecraft LegacyRandomSource,
// including exact 32-bit overflow semantics and the Marsaglia polar gaussian
// with its cached second sample.
type LegacyRandom struct {
	seed     uint64
	gaussian marsagliaGaussian
}

const (
	legacyMask       uint64 = (1 << 48) - 1
	legacyMultiplier uint64 = 25214903917
	legacyIncrement  uint64 = 11
)

func NewLegacyRandom(seed int64) LegacyRandom {
	return LegacyRandom{seed: (uint64(seed) ^ legacyMultiplier) & legacyMask}
}

func (r *LegacyRandom) SetSeed(seed int64) {
	r.seed = (uint64(seed) ^ legacyMultiplier) & legacyMask
	r.gaussian.reset()
}

func (r *LegacyRandom) next(bits int) int32 {
	r.seed = (r.seed*legacyMultiplier + legacyIncrement) & legacyMask
	return int32(r.seed >> (48 - bits))
}

// NextInt mirrors java.util.Random.nextInt(bound). The rejection test relies
// on 32-bit overflow, so all arithmetic stays in int32.
func (r *LegacyRandom) NextInt(bound int) int {
	b := int32(bound)
	if b <= 0 {
		return 0
	}
	if b&-b == b {
		return int(int32((int64(b) * int64(r.next(31))) >> 31))
	}
	for {
		bits := r.next(31)
		value := bits % b
		if bits-value+(b-1) >= 0 {
			return int(value)
		}
	}
}

// NextIntUnbounded mirrors java.util.Random.nextInt().
func (r *LegacyRandom) NextIntUnbounded() int32 {
	return r.next(32)
}

// NextLong mirrors java.util.Random.nextLong(): ((long)next(32) << 32) + next(32).
func (r *LegacyRandom) NextLong() int64 {
	hi := int64(r.next(32))
	lo := int64(r.next(32))
	return (hi << 32) + lo
}

func (r *LegacyRandom) NextBool() bool {
	return r.next(1) != 0
}

func (r *LegacyRandom) NextFloat() float32 {
	return float32(r.next(24)) * 5.9604645e-8
}

func (r *LegacyRandom) NextDouble() float64 {
	return float64((int64(r.next(26))<<27)+int64(r.next(27))) * 1.1102230246251565e-16
}

func (r *LegacyRandom) NextGaussian() float64 {
	return r.gaussian.sample(r)
}

func (r *LegacyRandom) ConsumeCount(rounds int) {
	for i := 0; i < rounds; i++ {
		r.next(32)
	}
}

// marsagliaGaussian mirrors net.minecraft MarsagliaPolarGaussian.
type marsagliaGaussian struct {
	hasNext bool
	next    float64
}

type doubleSource interface {
	NextDouble() float64
}

func (g *marsagliaGaussian) reset() {
	g.hasNext = false
}

func (g *marsagliaGaussian) sample(r doubleSource) float64 {
	if g.hasNext {
		g.hasNext = false
		return g.next
	}
	for {
		d0 := 2.0*r.NextDouble() - 1.0
		d1 := 2.0*r.NextDouble() - 1.0
		d2 := d0*d0 + d1*d1
		if d2 < 1.0 && d2 != 0.0 {
			d3 := math.Sqrt(-2.0 * math.Log(d2) / d2)
			g.next = d1 * d3
			g.hasNext = true
			return d0 * d3
		}
	}
}

// SetLargeFeatureSeed mirrors WorldgenRandom.setLargeFeatureSeed: used with a
// per-carver index added to the world seed and the origin chunk coordinates.
func (r *LegacyRandom) SetLargeFeatureSeed(seed int64, chunkX, chunkZ int) {
	r.SetSeed(seed)
	a := r.NextLong()
	b := r.NextLong()
	r.SetSeed(int64(chunkX)*a ^ int64(chunkZ)*b ^ seed)
}

// SetLargeFeatureWithSalt mirrors WorldgenRandom.setLargeFeatureWithSalt.
func (r *LegacyRandom) SetLargeFeatureWithSalt(seed int64, x, z, salt int) {
	r.SetSeed(int64(x)*341873128712 + int64(z)*132897987541 + seed + int64(salt))
}
