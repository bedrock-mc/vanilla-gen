package vanilla

import (
	gen "github.com/bedrock-mc/vanilla-gen/gen"
	"github.com/df-mc/dragonfly/server/world/chunk"
)

const biomeCellSize = 4

type sourceBiomeVolume struct {
	startY int
	cellsY int
	data   []gen.Biome
}

type biomeSet [4]uint64

func (s *biomeSet) add(biome gen.Biome) {
	s[biome>>6] |= 1 << (biome & 63)
}

func (s biomeSet) contains(biome gen.Biome) bool {
	return s[biome>>6]&(1<<(biome&63)) != 0
}

func newSourceBiomeVolume(minY, maxY int) sourceBiomeVolume {
	startY := alignDown(minY, biomeCellSize)
	cellsY := (maxY-startY)/biomeCellSize + 1
	return sourceBiomeVolume{
		startY: startY,
		cellsY: cellsY,
		data:   make([]gen.Biome, 4*4*cellsY),
	}
}

func (v sourceBiomeVolume) cellIndex(localX, y, localZ int) int {
	cellX := clamp(localX>>2, 0, 3)
	cellZ := clamp(localZ>>2, 0, 3)
	cellY := clamp((y-v.startY)/biomeCellSize, 0, v.cellsY-1)
	return (cellY*4+cellZ)*4 + cellX
}

func (v sourceBiomeVolume) set(localX, y, localZ int, biome gen.Biome) {
	v.data[v.cellIndex(localX, y, localZ)] = biome
}

func (v sourceBiomeVolume) biomeAt(localX, y, localZ int) gen.Biome {
	return v.data[v.cellIndex(localX, y, localZ)]
}

func (g Generator) populateBiomeVolume(c *chunk.Chunk, chunkX, chunkZ, minY, maxY int) sourceBiomeVolume {
	startY := alignDown(minY, biomeCellSize)
	volume := newSourceBiomeVolume(minY, maxY)

	for baseX := 0; baseX < 16; baseX += biomeCellSize {
		worldX := chunkX*16 + baseX

		for baseZ := 0; baseZ < 16; baseZ += biomeCellSize {
			worldZ := chunkZ*16 + baseZ

			for baseY := startY; baseY <= maxY; baseY += biomeCellSize {
				biome := g.biomeSource.GetBiome(worldX, baseY, worldZ)
				volume.set(baseX, baseY, baseZ, biome)
				biomeRID := biomeRuntimeID(biome)

				fillFromY := baseY
				if fillFromY < minY {
					fillFromY = minY
				}
				fillToY := baseY + biomeCellSize - 1
				if fillToY > maxY {
					fillToY = maxY
				}

				for localY := fillFromY; localY <= fillToY; localY++ {
					for localX := baseX; localX < baseX+biomeCellSize; localX++ {
						for localZ := baseZ; localZ < baseZ+biomeCellSize; localZ++ {
							c.SetBiome(uint8(localX), int16(localY), uint8(localZ), biomeRID)
						}
					}
				}
			}
		}
	}
	return volume
}

func (g Generator) biomeAt(c *chunk.Chunk, localX, y, localZ int) gen.Biome {
	return biomeFromRuntimeID(c.Biome(uint8(localX), int16(y), uint8(localZ)))
}

// zoomedBiomeAt mirrors vanilla level.getBiome(pos) during decoration: the
// fuzzy 4x zoom over quart-resolution noise biomes. Quart Y is clamped to the
// generated range like ChunkAccess.getNoiseBiome.
func (g Generator) zoomedBiomeAt(x, y, z int) gen.Biome {
	minQuartY := g.metadata.MinY >> 2
	maxQuartY := minQuartY + (g.metadata.Height >> 2) - 1
	return gen.FuzzyZoomBiome(g.biomeZoomSeed, x, y, z, func(quartX, quartY, quartZ int) gen.Biome {
		if quartY < minQuartY {
			quartY = minQuartY
		}
		if quartY > maxQuartY {
			quartY = maxQuartY
		}
		return g.biomeSource.GetBiome(quartX<<2, quartY<<2, quartZ<<2)
	})
}

func alignDown(value, multiple int) int {
	remainder := value % multiple
	if remainder < 0 {
		remainder += multiple
	}
	return value - remainder
}
