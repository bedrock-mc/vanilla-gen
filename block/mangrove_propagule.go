package block

import (
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
)

type MangrovePropagule struct {
	base
	Hanging bool
	Stage   int
}

func (m MangrovePropagule) EncodeBlock() (string, map[string]any) {
	return "minecraft:mangrove_propagule", map[string]any{
		"hanging":                      m.Hanging,
		"propagule_stage":              int32(max(0, min(4, m.Stage))),
		"age_bit":                      false,
		"growth":                       int32(0),
		"minecraft:cardinal_direction": cube.South.String(),
	}
}

func allMangrovePropagule() (b []world.Block) {
	for stage := 0; stage <= 4; stage++ {
		b = append(b, MangrovePropagule{Stage: stage})
		b = append(b, MangrovePropagule{Hanging: true, Stage: stage})
	}
	return
}
