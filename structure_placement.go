package vanilla

import (
	"math"
	"sync"

	"github.com/df-mc/dragonfly/server/world"
)

type structureRingCache struct {
	mu    sync.RWMutex
	bySet map[string][]world.ChunkPos
}

func newStructureRingCache() *structureRingCache {
	return &structureRingCache{bySet: make(map[string][]world.ChunkPos)}
}

func (c *structureRingCache) Lookup(setName string) ([]world.ChunkPos, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	positions, ok := c.bySet[setName]
	return positions, ok
}

func (c *structureRingCache) Store(setName string, positions []world.ChunkPos) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.bySet[setName] = positions
}

func (g Generator) ringPositionsForPlanner(planner structurePlanner) []world.ChunkPos {
	if g.structureRings == nil || planner.placementType != "concentric_rings" || planner.concentricPlacement.Count <= 0 {
		return nil
	}
	if positions, ok := g.structureRings.Lookup(planner.setName); ok {
		return positions
	}

	placement := planner.concentricPlacement
	preferredTag := normalizeStructureTag(placement.PreferredBiomes)
	rng := newLegacyRandom(g.seed + int64(placement.Salt))
	angle := rng.NextDouble() * math.Pi * 2.0
	spread := max(placement.Spread, 1)
	positionInCircle := 0
	circle := 0
	positions := make([]world.ChunkPos, 0, placement.Count)

	for i := 0; i < placement.Count; i++ {
		dist := float64(4*placement.Distance+placement.Distance*circle*6) + (rng.NextDouble()-0.5)*float64(placement.Distance*5)/2.0
		initialX := int(math.Round(math.Cos(angle) * dist))
		initialZ := int(math.Round(math.Sin(angle) * dist))
		positions = append(positions, g.findPreferredRingChunk(initialX, initialZ, preferredTag))

		angle += (math.Pi * 2.0) / float64(spread)
		positionInCircle++
		if positionInCircle == spread {
			circle++
			positionInCircle = 0
			spread += (2 * spread) / (circle + 1)
			remaining := placement.Count - i - 1
			if spread > remaining {
				spread = max(remaining, 1)
			}
			angle += rng.NextDouble() * math.Pi * 2.0
		}
	}
	g.structureRings.Store(planner.setName, positions)
	return positions
}

func (g Generator) findPreferredRingChunk(initialX, initialZ int, preferredTag string) world.ChunkPos {
	if preferredTag == "" {
		return world.ChunkPos{int32(initialX), int32(initialZ)}
	}

	bestChunk := world.ChunkPos{int32(initialX), int32(initialZ)}
	bestDistance := math.MaxInt
	radiusChunks := 7
	for dz := -radiusChunks; dz <= radiusChunks; dz++ {
		for dx := -radiusChunks; dx <= radiusChunks; dx++ {
			distSq := dx*dx + dz*dz
			if distSq > radiusChunks*radiusChunks {
				continue
			}
			chunkX := initialX + dx
			chunkZ := initialZ + dz
			biome := g.biomeSource.GetBiome(chunkX*16+8, 0, chunkZ*16+8)
			if !structureBiomeTagAllows(preferredTag, biome) {
				continue
			}
			if distSq < bestDistance {
				bestDistance = distSq
				bestChunk = world.ChunkPos{int32(chunkX), int32(chunkZ)}
			}
		}
	}
	return bestChunk
}

func (g Generator) plannerPotentialStartChunksNearChunk(planner structurePlanner, chunkX, chunkZ, extraX, extraZ int) []world.ChunkPos {
	startMinChunkX := chunkX - planner.maxBackreachX - extraX
	startMaxChunkX := chunkX + planner.maxBackreachX + extraX
	startMinChunkZ := chunkZ - planner.maxBackreachZ - extraZ
	startMaxChunkZ := chunkZ + planner.maxBackreachZ + extraZ

	switch planner.placementType {
	case "concentric_rings":
		positions := g.ringPositionsForPlanner(planner)
		if len(positions) == 0 {
			return nil
		}
		out := make([]world.ChunkPos, 0, len(positions))
		for _, startChunk := range positions {
			if int(startChunk[0]) < startMinChunkX || int(startChunk[0]) > startMaxChunkX || int(startChunk[1]) < startMinChunkZ || int(startChunk[1]) > startMaxChunkZ {
				continue
			}
			out = append(out, startChunk)
		}
		return out
	default:
		minGridX := randomSpreadMinGrid(startMinChunkX, planner.randomPlacement.Spacing, planner.randomPlacement.Separation)
		maxGridX := floorDiv(startMaxChunkX, planner.randomPlacement.Spacing)
		minGridZ := randomSpreadMinGrid(startMinChunkZ, planner.randomPlacement.Spacing, planner.randomPlacement.Separation)
		maxGridZ := floorDiv(startMaxChunkZ, planner.randomPlacement.Spacing)

		out := make([]world.ChunkPos, 0, max(0, (maxGridX-minGridX+1)*(maxGridZ-minGridZ+1)))
		for gridX := minGridX; gridX <= maxGridX; gridX++ {
			for gridZ := minGridZ; gridZ <= maxGridZ; gridZ++ {
				startChunk := randomSpreadPotentialChunk(g.seed, planner.randomPlacement, gridX, gridZ)
				if int(startChunk[0]) < startMinChunkX || int(startChunk[0]) > startMaxChunkX || int(startChunk[1]) < startMinChunkZ || int(startChunk[1]) > startMaxChunkZ {
					continue
				}
				out = append(out, startChunk)
			}
		}
		return out
	}
}
