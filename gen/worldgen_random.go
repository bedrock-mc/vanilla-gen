package gen

// WorldgenRandom mirrors vanilla WorldgenRandom: java.util.Random-style
// value derivation over next(bits), where next(bits) comes either from a
// legacy LCG or from the high bits of a Xoroshiro128++ nextLong. Vanilla
// hands this wrapper to every feature and structure, so the bounded-int and
// double derivations below must be used instead of the raw Xoroshiro ones.
type WorldgenRandom struct {
	xoro     *Xoroshiro128
	legacy   LegacyRandom
	isLegacy bool
	gaussian marsagliaGaussian
}

func NewWorldgenRandomXoroshiro(seed int64) *WorldgenRandom {
	x := NewXoroshiro128FromSeed(seed)
	return &WorldgenRandom{xoro: &x}
}

// NewWorldgenRandomFromXoroshiro wraps an existing Xoroshiro state (e.g. a
// positional fork) in WorldgenRandom semantics.
func NewWorldgenRandomFromXoroshiro(x Xoroshiro128) *WorldgenRandom {
	return &WorldgenRandom{xoro: &x}
}

func NewWorldgenRandomLegacy(seed int64) *WorldgenRandom {
	return &WorldgenRandom{legacy: NewLegacyRandom(seed), isLegacy: true}
}

func (w *WorldgenRandom) next(bits int) int32 {
	if w.isLegacy {
		return w.legacy.next(bits)
	}
	return int32(w.xoro.NextLong() >> (64 - uint(bits)))
}

// SetSeed mirrors WorldgenRandom.setSeed: reseeds the wrapped source. Like
// vanilla it does not reset the cached gaussian.
func (w *WorldgenRandom) SetSeed(seed int64) {
	if w.isLegacy {
		w.legacy.seed = (uint64(seed) ^ legacyMultiplier) & legacyMask
		w.legacy.gaussian.reset()
		return
	}
	*w.xoro = NewXoroshiro128FromSeed(seed)
}

// NextInt mirrors java.util.Random.nextInt(bound); the signature matches the
// raw Xoroshiro method so feature code can use either interchangeably.
func (w *WorldgenRandom) NextInt(bound uint32) uint32 {
	b := int32(bound)
	if b <= 0 {
		return 0
	}
	if b&-b == b {
		return uint32(int32((int64(b) * int64(w.next(31))) >> 31))
	}
	for {
		bits := w.next(31)
		value := bits % b
		if bits-value+(b-1) >= 0 {
			return uint32(value)
		}
	}
}

func (w *WorldgenRandom) NextIntUnbounded() int32 {
	return w.next(32)
}

func (w *WorldgenRandom) NextLong() uint64 {
	hi := int64(w.next(32))
	lo := int64(w.next(32))
	return uint64((hi << 32) + lo)
}

func (w *WorldgenRandom) NextBool() bool {
	return w.next(1) != 0
}

func (w *WorldgenRandom) NextFloat() float32 {
	return float32(w.next(24)) * 5.9604645e-8
}

func (w *WorldgenRandom) NextDouble() float64 {
	return float64((int64(w.next(26))<<27)+int64(w.next(27))) * 1.1102230246251565e-16
}

func (w *WorldgenRandom) NextGaussian() float64 {
	return w.gaussian.sample(w)
}

func (w *WorldgenRandom) ConsumeCount(rounds int) {
	for i := 0; i < rounds; i++ {
		w.next(32)
	}
}

// SetDecorationSeed mirrors WorldgenRandom.setDecorationSeed and returns the
// population seed.
func (w *WorldgenRandom) SetDecorationSeed(levelSeed int64, minBlockX, minBlockZ int) int64 {
	w.SetSeed(levelSeed)
	xScale := int64(w.NextLong()) | 1
	zScale := int64(w.NextLong()) | 1
	seed := int64(minBlockX)*xScale + int64(minBlockZ)*zScale ^ levelSeed
	w.SetSeed(seed)
	return seed
}

func (w *WorldgenRandom) SetFeatureSeed(decorationSeed int64, index, step int) {
	w.SetSeed(decorationSeed + int64(index) + int64(10000*step))
}

func (w *WorldgenRandom) SetLargeFeatureSeed(seed int64, chunkX, chunkZ int) {
	w.SetSeed(seed)
	xScale := int64(w.NextLong())
	zScale := int64(w.NextLong())
	w.SetSeed(int64(chunkX)*xScale ^ int64(chunkZ)*zScale ^ seed)
}

func (w *WorldgenRandom) SetLargeFeatureWithSalt(seed int64, x, z, salt int) {
	w.SetSeed(int64(x)*341873128712 + int64(z)*132897987541 + seed + int64(salt))
}
