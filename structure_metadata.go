package vanilla

import (
	"github.com/df-mc/dragonfly/server/world"
	"github.com/df-mc/dragonfly/server/world/chunk"
)

func (g Generator) GenerateColumn(pos world.ChunkPos, col *chunk.Column) {
	if col == nil || col.Chunk == nil {
		return
	}
	g.GenerateChunk(pos, col.Chunk)
}
