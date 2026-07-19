package vanilla

import (
	"sync"

	gen "github.com/bedrock-mc/vanilla-gen/gen"
	"github.com/df-mc/dragonfly/server/block/cube"
)

const orePlanCacheShardCount = 32

type orePlanKey struct {
	state gen.WorldgenRandomState
	pos   cube.Pos
	size  int
	minY  int
	maxY  int
}

type oreGeometryPlan struct {
	positions []orePositionOffset
	endState  gen.WorldgenRandomState
}

// Ore veins are at most a few dozen blocks across. Store positions relative
// to the placed-feature origin so cached plans use 6 bytes per candidate
// instead of a 24-byte cube.Pos on 64-bit systems.
type orePositionOffset struct {
	x int16
	y int16
	z int16
}

type orePlanFlight struct {
	done chan struct{}
	plan oreGeometryPlan
	ok   bool
}

type orePlanCacheShard struct {
	mu      sync.Mutex
	cap     int
	values  map[orePlanKey]oreGeometryPlan
	order   []orePlanKey
	flights map[orePlanKey]*orePlanFlight
}

type orePlanCache [orePlanCacheShardCount]orePlanCacheShard

func newOrePlanCache(capacity int) *orePlanCache {
	c := &orePlanCache{}
	perShard := max(1, capacity/orePlanCacheShardCount)
	for i := range c {
		c[i].cap = perShard
	}
	return c
}

func (c *orePlanCache) shard(key orePlanKey) *orePlanCacheShard {
	h := key.state.XoroshiroLow ^ key.state.XoroshiroHigh
	h ^= uint64(int64(key.pos[0])) * 0x9e3779b185ebca87
	h ^= uint64(int64(key.pos[1])) * 0xc2b2ae3d27d4eb4f
	h ^= uint64(int64(key.pos[2])) * 0x165667b19e3779f9
	h ^= uint64(key.size) * 0x85ebca77c2b2ae63
	h ^= uint64(int64(key.minY))<<32 ^ uint64(int64(key.maxY))
	h ^= h >> 29
	return &c[h&(orePlanCacheShardCount-1)]
}

func (c *orePlanCache) LoadOrCompute(key orePlanKey, compute func() oreGeometryPlan) (result oreGeometryPlan) {
	if c == nil {
		return compute()
	}
	shard := c.shard(key)
	shard.mu.Lock()
	if plan, ok := shard.values[key]; ok {
		shard.mu.Unlock()
		return plan
	}
	if flight := shard.flights[key]; flight != nil {
		shard.mu.Unlock()
		<-flight.done
		if flight.ok {
			return flight.plan
		}
		return c.LoadOrCompute(key, compute)
	}
	if shard.flights == nil {
		shard.flights = make(map[orePlanKey]*orePlanFlight)
	}
	flight := &orePlanFlight{done: make(chan struct{})}
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
		shard.values = make(map[orePlanKey]oreGeometryPlan, shard.cap)
	}
	shard.values[key] = result
	shard.order = append(shard.order, key)
	for len(shard.order) > shard.cap {
		oldest := shard.order[0]
		shard.order = shard.order[1:]
		delete(shard.values, oldest)
	}
	flight.plan = result
	flight.ok = true
	delete(shard.flights, key)
	close(flight.done)
	completed = true
	shard.mu.Unlock()
	return result
}

var orePositionScratchPool sync.Pool

func acquireOrePositionScratch(capacity int) []orePositionOffset {
	if pooled := orePositionScratchPool.Get(); pooled != nil {
		positions := pooled.([]orePositionOffset)
		if cap(positions) >= capacity {
			return positions[:0]
		}
	}
	return make([]orePositionOffset, 0, capacity)
}

func releaseOrePositionScratch(positions []orePositionOffset) {
	if cap(positions) <= 4096 {
		orePositionScratchPool.Put(positions[:0])
	}
}
