package vanilla

import (
	"sync"

	gen "github.com/bedrock-mc/vanilla-gen/gen"
	"github.com/df-mc/dragonfly/server/world"
)

const featureBlockCacheShardCount = 32

type featureStateKey struct {
	name       string
	properties uintptr
}

func makeFeatureStateKey(state gen.BlockState) (featureStateKey, bool) {
	return featureStateKey{name: state.Name, properties: mapIdentity(state.Properties)}, true
}

type featureBlockCacheEntry struct {
	block world.Block
	rid   uint32
	ok    bool
}

type featureBlockCacheShard struct {
	mu     sync.RWMutex
	cap    int
	values map[featureStateKey]featureBlockCacheEntry
	order  []featureStateKey
}

type featureBlockCache [featureBlockCacheShardCount]featureBlockCacheShard

func newFeatureBlockCache(capacity int) *featureBlockCache {
	c := &featureBlockCache{}
	perShard := max(1, capacity/featureBlockCacheShardCount)
	for i := range c {
		c[i].cap = perShard
	}
	return c
}

func (c *featureBlockCache) shard(key featureStateKey) *featureBlockCacheShard {
	var hash uint64 = 1469598103934665603
	add := func(value string) {
		for i := 0; i < len(value); i++ {
			hash ^= uint64(value[i])
			hash *= 1099511628211
		}
	}
	add(key.name)
	hash ^= uint64(key.properties)
	hash *= 1099511628211
	return &c[hash&(featureBlockCacheShardCount-1)]
}

func (c *featureBlockCache) Load(key featureStateKey) (featureBlockCacheEntry, bool) {
	if c == nil {
		return featureBlockCacheEntry{}, false
	}
	shard := c.shard(key)
	shard.mu.RLock()
	entry, ok := shard.values[key]
	shard.mu.RUnlock()
	return entry, ok
}

func (c *featureBlockCache) Store(key featureStateKey, entry featureBlockCacheEntry) {
	if c == nil {
		return
	}
	shard := c.shard(key)
	shard.mu.Lock()
	if shard.values == nil {
		shard.values = make(map[featureStateKey]featureBlockCacheEntry, shard.cap)
	}
	if _, exists := shard.values[key]; exists {
		shard.mu.Unlock()
		return
	}
	shard.values[key] = entry
	shard.order = append(shard.order, key)
	for len(shard.order) > shard.cap {
		oldest := shard.order[0]
		shard.order = shard.order[1:]
		delete(shard.values, oldest)
	}
	shard.mu.Unlock()
}
