package gen

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
)

// ObfuscateSeed mirrors BiomeManager.obfuscateSeed: Guava
// Hashing.sha256().hashLong(seed).asLong(), i.e. SHA-256 over the
// little-endian seed bytes with the first 8 digest bytes read little-endian.
func ObfuscateSeed(seed int64) int64 {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], uint64(seed))
	sum := sha256.Sum256(buf[:])
	return int64(binary.LittleEndian.Uint64(sum[:8]))
}

func lcgNext(rval, c int64) int64 {
	rval *= rval*6364136223846793005 + 1442695040888963407
	return rval + c
}

func biomeFiddle(rval int64) float64 {
	uniform := float64((rval>>24)&1023) / 1024.0
	return (uniform - 0.5) * 0.9
}

func fiddledDistance(seed int64, xRandom, yRandom, zRandom int, distanceX, distanceY, distanceZ float64) float64 {
	rval := lcgNext(seed, int64(xRandom))
	rval = lcgNext(rval, int64(yRandom))
	rval = lcgNext(rval, int64(zRandom))
	rval = lcgNext(rval, int64(xRandom))
	rval = lcgNext(rval, int64(yRandom))
	rval = lcgNext(rval, int64(zRandom))
	fiddleX := biomeFiddle(rval)
	rval = lcgNext(rval, seed)
	fiddleY := biomeFiddle(rval)
	rval = lcgNext(rval, seed)
	fiddleZ := biomeFiddle(rval)
	dz := distanceZ + fiddleZ
	dy := distanceY + fiddleY
	dx := distanceX + fiddleX
	return dz*dz + dy*dy + dx*dx
}

// FuzzyZoomBiome mirrors BiomeManager.getBiome: the fuzzy 4x biome zoom
// applied whenever vanilla resolves a biome at block resolution during
// decoration. getQuart receives quart coordinates.
func FuzzyZoomBiome(zoomSeed int64, x, y, z int, getQuart func(quartX, quartY, quartZ int) Biome) Biome {
	absX := x - 2
	absY := y - 2
	absZ := z - 2
	parentX := absX >> 2
	parentY := absY >> 2
	parentZ := absZ >> 2
	fractX := float64(absX&3) / 4.0
	fractY := float64(absY&3) / 4.0
	fractZ := float64(absZ&3) / 4.0
	minI := 0
	minDistance := math.Inf(1)

	for i := 0; i < 8; i++ {
		xEven := i&4 == 0
		yEven := i&2 == 0
		zEven := i&1 == 0
		cornerX := parentX
		if !xEven {
			cornerX++
		}
		cornerY := parentY
		if !yEven {
			cornerY++
		}
		cornerZ := parentZ
		if !zEven {
			cornerZ++
		}
		distanceX := fractX
		if !xEven {
			distanceX--
		}
		distanceY := fractY
		if !yEven {
			distanceY--
		}
		distanceZ := fractZ
		if !zEven {
			distanceZ--
		}
		next := fiddledDistance(zoomSeed, cornerX, cornerY, cornerZ, distanceX, distanceY, distanceZ)
		if minDistance > next {
			minI = i
			minDistance = next
		}
	}

	biomeX := parentX
	if minI&4 != 0 {
		biomeX++
	}
	biomeY := parentY
	if minI&2 != 0 {
		biomeY++
	}
	biomeZ := parentZ
	if minI&1 != 0 {
		biomeZ++
	}
	return getQuart(biomeX, biomeY, biomeZ)
}
