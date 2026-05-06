package block

import "github.com/df-mc/dragonfly/server/world"

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
