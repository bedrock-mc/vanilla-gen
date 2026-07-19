package vanilla

import (
	"reflect"
	"sync"
	"unsafe"

	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/df-mc/dragonfly/server/world/chunk"
)

// protoChunkCache stores pristine pre-decoration chunks (terrain, biomes,
// surface, carving) so that region replay and adjacent chunk generation do
// not regenerate the same base chunk repeatedly. Entries are immutable:
// consumers receive clones.
type protoChunkCache struct {
	mu       sync.Mutex
	cap      int
	byPos    map[[2]int]protoChunkEntry
	order    [][2]int
	inFlight map[[2]int]*protoChunkFlight
}

type protoChunkEntry struct {
	c      *chunk.Chunk
	biomes sourceBiomeVolume
}

type protoChunkFlight struct {
	done  chan struct{}
	entry protoChunkEntry
	ok    bool
}

func newProtoChunkCache(capacity int) *protoChunkCache {
	return &protoChunkCache{cap: capacity, byPos: make(map[[2]int]protoChunkEntry, capacity)}
}

// get returns a mutable clone of the cached pre-decoration chunk.
func (p *protoChunkCache) get(chunkX, chunkZ int) (*chunk.Chunk, sourceBiomeVolume, bool) {
	key := [2]int{chunkX, chunkZ}
	p.mu.Lock()
	entry, ok := p.byPos[key]
	flight := p.inFlight[key]
	p.mu.Unlock()
	if ok {
		return entry.c.Clone(), entry.biomes, true
	}
	if flight == nil {
		return nil, sourceBiomeVolume{}, false
	}
	<-flight.done
	if !flight.ok {
		return nil, sourceBiomeVolume{}, false
	}
	return flight.entry.c.Clone(), flight.entry.biomes, true
}

// reserve returns a cached entry, an existing computation to wait for, or a
// new flight owned by the caller. It closes the check/compute race that would
// otherwise let adjacent generation workers build the same base chunk.
func (p *protoChunkCache) reserve(chunkX, chunkZ int) (protoChunkEntry, *protoChunkFlight, bool) {
	key := [2]int{chunkX, chunkZ}
	p.mu.Lock()
	defer p.mu.Unlock()
	if entry, ok := p.byPos[key]; ok {
		return entry, nil, false
	}
	if flight := p.inFlight[key]; flight != nil {
		return protoChunkEntry{}, flight, false
	}
	if p.inFlight == nil {
		p.inFlight = make(map[[2]int]*protoChunkFlight)
	}
	flight := &protoChunkFlight{done: make(chan struct{})}
	p.inFlight[key] = flight
	return protoChunkEntry{}, flight, true
}

func (p *protoChunkCache) complete(chunkX, chunkZ int, flight *protoChunkFlight, c *chunk.Chunk, biomes sourceBiomeVolume) {
	key := [2]int{chunkX, chunkZ}
	entry := protoChunkEntry{c: c.Clone(), biomes: biomes}
	p.mu.Lock()
	if stored, ok := p.byPos[key]; ok {
		entry = stored
	} else {
		p.byPos[key] = entry
		p.order = append(p.order, key)
	}
	for len(p.order) > p.cap {
		oldest := p.order[0]
		p.order = p.order[1:]
		delete(p.byPos, oldest)
	}
	flight.entry = entry
	flight.ok = true
	delete(p.inFlight, key)
	close(flight.done)
	p.mu.Unlock()
}

func (p *protoChunkCache) abort(chunkX, chunkZ int, flight *protoChunkFlight) {
	key := [2]int{chunkX, chunkZ}
	p.mu.Lock()
	if p.inFlight[key] == flight {
		delete(p.inFlight, key)
		close(flight.done)
	}
	p.mu.Unlock()
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

// adoptChunkContents transplants the block sub-chunks and biome storage of a
// throwaway clone into dst, avoiding ~100k palette writes per cache hit. The
// biome slice is unexported in Dragonfly, so it is swapped via a
// reflect-derived offset that is verified once at startup; on verification
// failure we fall back to block-by-block copying.
func adoptChunkContents(dst, src *chunk.Chunk, minY, maxY int) {
	copy(dst.Sub(), src.Sub())
	if biomeSwapOK {
		srcBiomes := chunkBiomesSlice(src)
		dstBiomes := chunkBiomesSlice(dst)
		copy(*dstBiomes, *srcBiomes)
		return
	}
	for x := uint8(0); x < 16; x++ {
		for z := uint8(0); z < 16; z++ {
			for y := minY; y <= maxY; y++ {
				dst.SetBiome(x, int16(y), z, src.Biome(x, int16(y), z))
			}
		}
	}
}

var (
	biomesFieldOffset uintptr
	biomeSwapOK       bool
)

func chunkBiomesSlice(c *chunk.Chunk) *[]*chunk.PalettedStorage {
	return (*[]*chunk.PalettedStorage)(unsafe.Pointer(uintptr(unsafe.Pointer(c)) + biomesFieldOffset))
}

func initBiomeSwap() {
	t := reflect.TypeOf(chunk.Chunk{})
	field, ok := t.FieldByName("biomes")
	if !ok || field.Type != reflect.TypeOf([]*chunk.PalettedStorage(nil)) {
		return
	}
	biomesFieldOffset = field.Offset

	// Verify against the real accessors before trusting the offset.
	a := chunk.New(world.DefaultBlockRegistry, cube.Range{-64, 319})
	b := chunk.New(world.DefaultBlockRegistry, cube.Range{-64, 319})
	a.SetBiome(3, 100, 5, 7)
	copy(*chunkBiomesSlice(b), *chunkBiomesSlice(a))
	biomeSwapOK = b.Biome(3, 100, 5) == 7
}
