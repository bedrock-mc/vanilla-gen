package block

import (
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
)

type BuddingAmethyst struct{ base }

func (BuddingAmethyst) EncodeBlock() (string, map[string]any) {
	return "minecraft:budding_amethyst", nil
}

type SmallAmethystBud struct {
	base
	Face cube.Face
}

func (s SmallAmethystBud) EncodeBlock() (string, map[string]any) {
	return "minecraft:small_amethyst_bud", map[string]any{"minecraft:block_face": s.Face.String()}
}

func allSmallAmethystBud() (b []world.Block) {
	for _, face := range cube.Faces() {
		b = append(b, SmallAmethystBud{Face: face})
	}
	return
}

type MediumAmethystBud struct {
	base
	Face cube.Face
}

func (m MediumAmethystBud) EncodeBlock() (string, map[string]any) {
	return "minecraft:medium_amethyst_bud", map[string]any{"minecraft:block_face": m.Face.String()}
}

func allMediumAmethystBud() (b []world.Block) {
	for _, face := range cube.Faces() {
		b = append(b, MediumAmethystBud{Face: face})
	}
	return
}

type LargeAmethystBud struct {
	base
	Face cube.Face
}

func (l LargeAmethystBud) EncodeBlock() (string, map[string]any) {
	return "minecraft:large_amethyst_bud", map[string]any{"minecraft:block_face": l.Face.String()}
}

func allLargeAmethystBud() (b []world.Block) {
	for _, face := range cube.Faces() {
		b = append(b, LargeAmethystBud{Face: face})
	}
	return
}

type AmethystCluster struct {
	base
	Face cube.Face
}

func (a AmethystCluster) EncodeBlock() (string, map[string]any) {
	return "minecraft:amethyst_cluster", map[string]any{"minecraft:block_face": a.Face.String()}
}

func allAmethystCluster() (b []world.Block) {
	for _, face := range cube.Faces() {
		b = append(b, AmethystCluster{Face: face})
	}
	return
}
