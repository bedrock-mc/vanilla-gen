package block

import (
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
)

type BambooLeafSize struct{ name string }

func BambooNoLeaves() BambooLeafSize    { return BambooLeafSize{"no_leaves"} }
func BambooSmallLeaves() BambooLeafSize { return BambooLeafSize{"small_leaves"} }
func BambooLargeLeaves() BambooLeafSize { return BambooLeafSize{"large_leaves"} }

func BambooLeafSizes() []BambooLeafSize {
	return []BambooLeafSize{BambooNoLeaves(), BambooSmallLeaves(), BambooLargeLeaves()}
}

type BambooStalkThickness struct{ name string }

func ThinBamboo() BambooStalkThickness  { return BambooStalkThickness{"thin"} }
func ThickBamboo() BambooStalkThickness { return BambooStalkThickness{"thick"} }

func BambooStalkThicknesses() []BambooStalkThickness {
	return []BambooStalkThickness{ThinBamboo(), ThickBamboo()}
}

type Bamboo struct {
	base
	AgeBit    bool
	LeafSize  BambooLeafSize
	Thickness BambooStalkThickness
}

func (b Bamboo) EncodeBlock() (string, map[string]any) {
	leafSize, thickness := b.LeafSize.name, b.Thickness.name
	if leafSize == "" {
		leafSize = BambooNoLeaves().name
	}
	if thickness == "" {
		thickness = ThinBamboo().name
	}
	return "minecraft:bamboo", map[string]any{
		"age_bit":                b.AgeBit,
		"bamboo_leaf_size":       leafSize,
		"bamboo_stalk_thickness": thickness,
	}
}

func allBamboo() (b []world.Block) {
	for _, leafSize := range BambooLeafSizes() {
		for _, thickness := range BambooStalkThicknesses() {
			b = append(b, Bamboo{LeafSize: leafSize, Thickness: thickness})
			b = append(b, Bamboo{AgeBit: true, LeafSize: leafSize, Thickness: thickness})
		}
	}
	return
}

type Azalea struct {
	base
	Flowering bool
}

func (a Azalea) EncodeItem() (name string, meta int16) {
	if a.Flowering {
		return "minecraft:flowering_azalea", 0
	}
	return "minecraft:azalea", 0
}

func (a Azalea) EncodeBlock() (string, map[string]any) {
	if a.Flowering {
		return "minecraft:flowering_azalea", nil
	}
	return "minecraft:azalea", nil
}

func allAzalea() []world.Block {
	return []world.Block{Azalea{}, Azalea{Flowering: true}}
}

type MossBlock struct{ base }

func (MossBlock) EncodeBlock() (string, map[string]any) { return "minecraft:moss_block", nil }

type RootedDirt struct{ base }

func (RootedDirt) EncodeBlock() (string, map[string]any) { return "minecraft:dirt_with_roots", nil }

type MangroveRoots struct{ base }

func (MangroveRoots) EncodeBlock() (string, map[string]any) { return "minecraft:mangrove_roots", nil }

type HangingRoots struct{ base }

func (HangingRoots) EncodeBlock() (string, map[string]any) { return "minecraft:hanging_roots", nil }

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
