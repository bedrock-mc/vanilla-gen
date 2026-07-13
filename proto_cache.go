package vanilla

import (
	"sync"

	"github.com/df-mc/dragonfly/server/world/chunk"
)

// protoChunkCache stores pristine pre-decoration chunks (terrain, biomes,
// surface, carving) so that region replay and adjacent chunk generation do
// not regenerate the same base chunk repeatedly. Entries are immutable:
// consumers receive clones.
type protoChunkCache struct {
	mu    sync.Mutex
	cap   int
	byPos map[[2]int]protoChunkEntry
	order [][2]int
}

type protoChunkEntry struct {
	c      *chunk.Chunk
	biomes sourceBiomeVolume
}

func newProtoChunkCache(capacity int) *protoChunkCache {
	return &protoChunkCache{cap: capacity, byPos: make(map[[2]int]protoChunkEntry, capacity)}
}

// get returns a mutable clone of the cached pre-decoration chunk.
func (p *protoChunkCache) get(chunkX, chunkZ int) (*chunk.Chunk, sourceBiomeVolume, bool) {
	p.mu.Lock()
	entry, ok := p.byPos[[2]int{chunkX, chunkZ}]
	p.mu.Unlock()
	if !ok {
		return nil, sourceBiomeVolume{}, false
	}
	return entry.c.Clone(), entry.biomes, true
}

// store keeps a pristine clone of c.
func (p *protoChunkCache) store(chunkX, chunkZ int, c *chunk.Chunk, biomes sourceBiomeVolume) {
	key := [2]int{chunkX, chunkZ}
	clone := c.Clone()
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.byPos[key]; ok {
		return
	}
	p.byPos[key] = protoChunkEntry{c: clone, biomes: biomes}
	p.order = append(p.order, key)
	for len(p.order) > p.cap {
		oldest := p.order[0]
		p.order = p.order[1:]
		delete(p.byPos, oldest)
	}
}

// xzIntCache memoizes pure functions of (x, z) such as the preliminary
// surface level corners.
type xzIntCache struct {
	mu    sync.RWMutex
	byPos map[[2]int]int
}

func newXZIntCache() *xzIntCache {
	return &xzIntCache{byPos: make(map[[2]int]int)}
}

func (c *xzIntCache) lookup(x, z int) (int, bool) {
	c.mu.RLock()
	v, ok := c.byPos[[2]int{x, z}]
	c.mu.RUnlock()
	return v, ok
}

func (c *xzIntCache) store(x, z, v int) {
	c.mu.Lock()
	c.byPos[[2]int{x, z}] = v
	c.mu.Unlock()
}
