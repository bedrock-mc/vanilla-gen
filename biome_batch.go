package vanilla

import (
	"sync"

	gen "github.com/bedrock-mc/vanilla-gen/gen"
)

type biomeQueryScratch struct {
	points []gen.FunctionContext
	biomes []gen.Biome
}

var biomeQueryPool sync.Pool

func acquireBiomeQuery(size int) *biomeQueryScratch {
	var query *biomeQueryScratch
	if pooled := biomeQueryPool.Get(); pooled != nil {
		query = pooled.(*biomeQueryScratch)
	} else {
		query = &biomeQueryScratch{}
	}
	if cap(query.points) < size {
		query.points = make([]gen.FunctionContext, 0, size)
	} else {
		query.points = query.points[:0]
	}
	if cap(query.biomes) < size {
		query.biomes = make([]gen.Biome, size)
	} else {
		query.biomes = query.biomes[:size]
	}
	return query
}

func releaseBiomeQuery(query *biomeQueryScratch) {
	if query == nil {
		return
	}
	query.points = query.points[:0]
	query.biomes = query.biomes[:0]
	biomeQueryPool.Put(query)
}

func (g Generator) acceleratedBiomeSource() (gen.BatchBiomeSource, bool) {
	if g.acceleration == nil || !g.acceleration.active.Load() {
		return nil, false
	}
	source, ok := g.biomeSource.(gen.BatchBiomeSource)
	return source, ok
}
