package vanilla

import (
	"sync"

	gen "github.com/bedrock-mc/vanilla-gen/gen"
	"github.com/df-mc/dragonfly/server/world/chunk"
)

const biomeCellSize = 4

type sourceBiomeVolume struct {
	startY int
	cellsY int
	data   []gen.Biome
}

type chunkBiomeVolumeEntry struct {
	volume sourceBiomeVolume
	set    biomeSet
}

type chunkBiomeVolumeFlight struct {
	done  chan struct{}
	entry chunkBiomeVolumeEntry
	ok    bool
}

type chunkBiomeVolumeCacheShard struct {
	mu      sync.Mutex
	cap     int
	values  map[chunkBiomeSetKey]chunkBiomeVolumeEntry
	order   []chunkBiomeSetKey
	flights map[chunkBiomeSetKey]*chunkBiomeVolumeFlight
}

// chunkBiomeVolumeCache stores immutable quart-resolution biome volumes.
// Feature selection and proto-chunk construction request the same coordinates;
// sharing the compact result avoids evaluating every climate point twice.
type chunkBiomeVolumeCache [chunkBiomeSetCacheShardCount]chunkBiomeVolumeCacheShard

func newChunkBiomeVolumeCache(capacity int) *chunkBiomeVolumeCache {
	c := &chunkBiomeVolumeCache{}
	perShard := max(1, capacity/chunkBiomeSetCacheShardCount)
	for i := range c {
		c[i].cap = perShard
	}
	return c
}

func (c *chunkBiomeVolumeCache) shard(key chunkBiomeSetKey) *chunkBiomeVolumeCacheShard {
	h := uint64(int64(key.chunkX))*0x9e3779b185ebca87 ^ uint64(int64(key.chunkZ))*0xc2b2ae3d27d4eb4f
	h ^= uint64(key.minY)*0x165667b19e3779f9 ^ uint64(key.maxY)
	h ^= h >> 29
	return &c[h&(chunkBiomeSetCacheShardCount-1)]
}

func (c *chunkBiomeVolumeCache) LoadOrCompute(key chunkBiomeSetKey, compute func() chunkBiomeVolumeEntry) (result chunkBiomeVolumeEntry) {
	if c == nil {
		return compute()
	}
	shard := c.shard(key)
	shard.mu.Lock()
	if entry, ok := shard.values[key]; ok {
		shard.mu.Unlock()
		return entry
	}
	if flight := shard.flights[key]; flight != nil {
		shard.mu.Unlock()
		<-flight.done
		if flight.ok {
			return flight.entry
		}
		return c.LoadOrCompute(key, compute)
	}
	if shard.flights == nil {
		shard.flights = make(map[chunkBiomeSetKey]*chunkBiomeVolumeFlight)
	}
	flight := &chunkBiomeVolumeFlight{done: make(chan struct{})}
	shard.flights[key] = flight
	shard.mu.Unlock()

	completed := false
	defer func() {
		if completed {
			return
		}
		shard.mu.Lock()
		delete(shard.flights, key)
		close(flight.done)
		shard.mu.Unlock()
	}()

	result = compute()
	shard.mu.Lock()
	if shard.values == nil {
		shard.values = make(map[chunkBiomeSetKey]chunkBiomeVolumeEntry, shard.cap)
	}
	if stored, ok := shard.values[key]; ok {
		result = stored
	} else {
		shard.values[key] = result
		shard.order = append(shard.order, key)
		for len(shard.order) > shard.cap {
			oldest := shard.order[0]
			shard.order = shard.order[1:]
			delete(shard.values, oldest)
		}
	}
	flight.entry = result
	flight.ok = true
	delete(shard.flights, key)
	close(flight.done)
	completed = true
	shard.mu.Unlock()
	return result
}

type biomeSet [4]uint64

func (s *biomeSet) add(biome gen.Biome) {
	s[biome>>6] |= 1 << (biome & 63)
}

func (s biomeSet) contains(biome gen.Biome) bool {
	return s[biome>>6]&(1<<(biome&63)) != 0
}

func (s *biomeSet) merge(other biomeSet) {
	for i := range s {
		s[i] |= other[i]
	}
}

const chunkBiomeSetCacheShardCount = 32

type chunkBiomeSetKey struct {
	chunkX int
	chunkZ int
	minY   int
	maxY   int
}

type chunkBiomeSetFlight struct {
	done chan struct{}
	set  biomeSet
	ok   bool
}

type chunkBiomeSetCacheShard struct {
	mu      sync.Mutex
	cap     int
	values  map[chunkBiomeSetKey]biomeSet
	order   []chunkBiomeSetKey
	flights map[chunkBiomeSetKey]*chunkBiomeSetFlight
}

// chunkBiomeSetCache stores the compact union of quart-resolution biomes in
// each chunk. Feature selection asks for overlapping 3x3 unions repeatedly;
// caching the 32-byte leaves lets adjacent requests OR nine values instead of
// performing thousands of repeated biome-source lookups.
type chunkBiomeSetCache [chunkBiomeSetCacheShardCount]chunkBiomeSetCacheShard

func newChunkBiomeSetCache(capacity int) *chunkBiomeSetCache {
	c := &chunkBiomeSetCache{}
	perShard := max(1, capacity/chunkBiomeSetCacheShardCount)
	for i := range c {
		c[i].cap = perShard
	}
	return c
}

func (c *chunkBiomeSetCache) shard(key chunkBiomeSetKey) *chunkBiomeSetCacheShard {
	h := uint64(int64(key.chunkX))*0x9e3779b185ebca87 ^ uint64(int64(key.chunkZ))*0xc2b2ae3d27d4eb4f
	h ^= uint64(key.minY)*0x165667b19e3779f9 ^ uint64(key.maxY)
	h ^= h >> 29
	return &c[h&(chunkBiomeSetCacheShardCount-1)]
}

func (c *chunkBiomeSetCache) Store(key chunkBiomeSetKey, set biomeSet) {
	if c == nil {
		return
	}
	shard := c.shard(key)
	shard.mu.Lock()
	shard.storeLocked(key, set)
	shard.mu.Unlock()
}

func (s *chunkBiomeSetCacheShard) storeLocked(key chunkBiomeSetKey, set biomeSet) {
	if s.values == nil {
		s.values = make(map[chunkBiomeSetKey]biomeSet, s.cap)
	}
	if _, exists := s.values[key]; exists {
		return
	}
	s.values[key] = set
	s.order = append(s.order, key)
	for len(s.order) > s.cap {
		oldest := s.order[0]
		s.order = s.order[1:]
		delete(s.values, oldest)
	}
}

func (c *chunkBiomeSetCache) LoadOrCompute(key chunkBiomeSetKey, compute func() biomeSet) (result biomeSet) {
	if c == nil {
		return compute()
	}
	shard := c.shard(key)
	shard.mu.Lock()
	if set, ok := shard.values[key]; ok {
		shard.mu.Unlock()
		return set
	}
	if flight := shard.flights[key]; flight != nil {
		shard.mu.Unlock()
		<-flight.done
		if flight.ok {
			return flight.set
		}
		return c.LoadOrCompute(key, compute)
	}
	if shard.flights == nil {
		shard.flights = make(map[chunkBiomeSetKey]*chunkBiomeSetFlight)
	}
	flight := &chunkBiomeSetFlight{done: make(chan struct{})}
	shard.flights[key] = flight
	shard.mu.Unlock()

	completed := false
	defer func() {
		if completed {
			return
		}
		shard.mu.Lock()
		delete(shard.flights, key)
		close(flight.done)
		shard.mu.Unlock()
	}()

	result = compute()
	shard.mu.Lock()
	if stored, ok := shard.values[key]; ok {
		result = stored
	} else {
		shard.storeLocked(key, result)
	}
	flight.set = result
	flight.ok = true
	delete(shard.flights, key)
	close(flight.done)
	completed = true
	shard.mu.Unlock()
	return result
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

func (g Generator) loadChunkBiomeVolume(chunkX, chunkZ, minY, maxY int) chunkBiomeVolumeEntry {
	key := chunkBiomeSetKey{chunkX: chunkX, chunkZ: chunkZ, minY: minY, maxY: maxY}
	return g.biomeVolumes.LoadOrCompute(key, func() chunkBiomeVolumeEntry {
		return g.sampleChunkBiomeVolume(chunkX, chunkZ, minY, maxY)
	})
}

func (g Generator) sampleChunkBiomeVolume(chunkX, chunkZ, minY, maxY int) chunkBiomeVolumeEntry {
	startY := alignDown(minY, biomeCellSize)
	volume := newSourceBiomeVolume(minY, maxY)
	var (
		query      *biomeQueryScratch
		queryIndex int
		seen       biomeSet
	)
	if source, ok := g.acceleratedBiomeSource(); ok {
		yCount := (maxY-startY)/biomeCellSize + 1
		query = acquireBiomeQuery(4 * 4 * yCount)
		defer releaseBiomeQuery(query)
		for baseX := 0; baseX < 16; baseX += biomeCellSize {
			worldX := chunkX*16 + baseX
			for baseZ := 0; baseZ < 16; baseZ += biomeCellSize {
				worldZ := chunkZ*16 + baseZ
				for baseY := startY; baseY <= maxY; baseY += biomeCellSize {
					query.points = append(query.points, gen.FunctionContext{BlockX: worldX, BlockY: baseY, BlockZ: worldZ})
				}
			}
		}
		source.GetBiomes(query.points, query.biomes)
	}

	for baseX := 0; baseX < 16; baseX += biomeCellSize {
		worldX := chunkX*16 + baseX

		for baseZ := 0; baseZ < 16; baseZ += biomeCellSize {
			worldZ := chunkZ*16 + baseZ

			for baseY := startY; baseY <= maxY; baseY += biomeCellSize {
				var biome gen.Biome
				if query != nil {
					biome = query.biomes[queryIndex]
					queryIndex++
				} else {
					biome = g.biomeSource.GetBiome(worldX, baseY, worldZ)
				}
				volume.set(baseX, baseY, baseZ, biome)
				seen.add(biome)
			}
		}
	}
	return chunkBiomeVolumeEntry{volume: volume, set: seen}
}

func (g Generator) populateBiomeVolume(c *chunk.Chunk, chunkX, chunkZ, minY, maxY int) sourceBiomeVolume {
	entry := g.loadChunkBiomeVolume(chunkX, chunkZ, minY, maxY)
	volume := entry.volume
	startY := volume.startY
	for baseX := 0; baseX < 16; baseX += biomeCellSize {
		for baseZ := 0; baseZ < 16; baseZ += biomeCellSize {
			for baseY := startY; baseY <= maxY; baseY += biomeCellSize {
				biomeRID := biomeRuntimeID(volume.biomeAt(baseX, baseY, baseZ))
				fillFromY := max(baseY, minY)
				fillToY := min(baseY+biomeCellSize-1, maxY)
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
	if g.featureBiomeSets != nil {
		g.featureBiomeSets.Store(chunkBiomeSetKey{chunkX: chunkX, chunkZ: chunkZ, minY: minY, maxY: maxY}, entry.set)
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
