package gen

import (
	"encoding/json"
	"math"
	"os"
	"slices"
	"testing"
)

type randomFixture struct {
	XoroshiroNextLong   []int64 `json:"xoroshiro_seed1_nextLong_x8"`
	XoroshiroPositional []int64 `json:"xoroshiro_seed1_forkPositional_at_1_2_3_nextLong_x4"`
	XoroshiroAquifer    []int64 `json:"xoroshiro_seed1_forkPositional_fromHashOf_minecraft_aquifer_nextLong_x4"`
	LegacyNextLong      []int64 `json:"legacy_seed1_nextLong_x4"`
	WorldgenLegacy      struct {
		DecorationSeed int64   `json:"setDecorationSeed_1_16_32"`
		FeatureNextInt []int64 `json:"after_setFeatureSeed_dec_5_3_nextInt16_x4"`
	} `json:"worldgenrandom_legacy_backed"`
	WorldgenXoroshiro struct {
		DecorationSeed int64   `json:"setDecorationSeed_1_16_32"`
		FeatureNextInt []int64 `json:"after_setFeatureSeed_dec_5_3_nextInt16_x4"`
	} `json:"worldgenrandom_xoroshiro_backed"`
}

func loadRandomFixture(t *testing.T) randomFixture {
	t.Helper()
	data, err := os.ReadFile("testdata/random_seed1.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fx randomFixture
	if err := json.Unmarshal(data, &fx); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return fx
}

func TestXoroshiroMatchesJava(t *testing.T) {
	fx := loadRandomFixture(t)

	rng := NewXoroshiro128FromSeed(1)
	for i, want := range fx.XoroshiroNextLong {
		if got := int64(rng.NextLong()); got != want {
			t.Fatalf("nextLong[%d] = %d, want %d", i, got, want)
		}
	}

	factory := NewPositionalRandomFactory(1)
	at := factory.At(1, 2, 3)
	for i, want := range fx.XoroshiroPositional {
		if got := int64(at.NextLong()); got != want {
			t.Fatalf("positional nextLong[%d] = %d, want %d", i, got, want)
		}
	}

	aquifer := factory.ForkAquiferRandom()
	// ForkAquiferRandom forks a new factory; the fixture captured the RNG
	// fromHashOf("minecraft:aquifer") directly, which is the factory's source.
	_ = aquifer
	hashRng := factory.fromHashOf(aquiferHashLo, aquiferHashHi)
	for i, want := range fx.XoroshiroAquifer {
		if got := int64(hashRng.NextLong()); got != want {
			t.Fatalf("aquifer hash nextLong[%d] = %d, want %d", i, got, want)
		}
	}
}

func TestLegacyRandomMatchesJava(t *testing.T) {
	fx := loadRandomFixture(t)

	rng := NewLegacyRandom(1)
	for i, want := range fx.LegacyNextLong {
		if got := rng.NextLong(); got != want {
			t.Fatalf("legacy nextLong[%d] = %d, want %d", i, got, want)
		}
	}
}

func TestWorldgenRandomLegacyMatchesJava(t *testing.T) {
	fx := loadRandomFixture(t)

	rng := NewWorldgenRandomLegacy(0)
	dec := rng.SetDecorationSeed(1, 16, 32)
	if dec != fx.WorldgenLegacy.DecorationSeed {
		t.Fatalf("decoration seed = %d, want %d", dec, fx.WorldgenLegacy.DecorationSeed)
	}
	rng.SetFeatureSeed(dec, 5, 3)
	for i, want := range fx.WorldgenLegacy.FeatureNextInt {
		if got := int64(rng.NextInt(16)); got != want {
			t.Fatalf("feature nextInt[%d] = %d, want %d", i, got, want)
		}
	}
}

func TestWorldgenRandomXoroshiroMatchesJava(t *testing.T) {
	fx := loadRandomFixture(t)

	rng := NewWorldgenRandomXoroshiro(0)
	dec := rng.SetDecorationSeed(1, 16, 32)
	if dec != fx.WorldgenXoroshiro.DecorationSeed {
		t.Fatalf("decoration seed = %d, want %d", dec, fx.WorldgenXoroshiro.DecorationSeed)
	}
	rng.SetFeatureSeed(dec, 5, 3)
	for i, want := range fx.WorldgenXoroshiro.FeatureNextInt {
		if got := int64(rng.NextInt(16)); got != want {
			t.Fatalf("feature nextInt[%d] = %d, want %d", i, got, want)
		}
	}
}

func TestWorldgenRandomSnapshotRestore(t *testing.T) {
	for _, test := range []struct {
		name string
		new  func() *WorldgenRandom
	}{
		{name: "xoroshiro", new: func() *WorldgenRandom { return NewWorldgenRandomXoroshiro(0x12345678) }},
		{name: "legacy", new: func() *WorldgenRandom { return NewWorldgenRandomLegacy(0x12345678) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			rng := test.new()
			_ = rng.NextInt(37)
			_ = rng.NextGaussian()
			state := rng.Snapshot()
			want := []uint64{
				rng.NextLong(),
				uint64(rng.NextInt(1_000_003)),
				math.Float64bits(rng.NextDouble()),
				math.Float64bits(rng.NextGaussian()),
				math.Float64bits(rng.NextGaussian()),
			}
			rng.Restore(state)
			got := []uint64{
				rng.NextLong(),
				uint64(rng.NextInt(1_000_003)),
				math.Float64bits(rng.NextDouble()),
				math.Float64bits(rng.NextGaussian()),
				math.Float64bits(rng.NextGaussian()),
			}
			if !slices.Equal(got, want) {
				t.Fatalf("restored stream mismatch:\n got %x\nwant %x", got, want)
			}
		})
	}
}
