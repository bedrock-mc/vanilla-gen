package block

import "github.com/df-mc/dragonfly/server/block/cube"

type BambooLeafSize struct{ name string }

func BambooNoLeaves() BambooLeafSize    { return BambooLeafSize{"no_leaves"} }
func BambooSmallLeaves() BambooLeafSize { return BambooLeafSize{"small_leaves"} }
func BambooLargeLeaves() BambooLeafSize { return BambooLeafSize{"large_leaves"} }

type BambooStalkThickness struct{ name string }

func ThinBamboo() BambooStalkThickness  { return BambooStalkThickness{"thin"} }
func ThickBamboo() BambooStalkThickness { return BambooStalkThickness{"thick"} }

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

type MossBlock struct{ base }

func (MossBlock) EncodeBlock() (string, map[string]any) { return "minecraft:moss_block", nil }

type RootedDirt struct{ base }

func (RootedDirt) EncodeBlock() (string, map[string]any) { return "minecraft:dirt_with_roots", nil }

type MangroveRoots struct{ base }

func (MangroveRoots) EncodeBlock() (string, map[string]any) { return "minecraft:mangrove_roots", nil }

type LeafLitter struct {
	base
	Growth int
	Facing cube.Direction
}

func (l LeafLitter) EncodeBlock() (string, map[string]any) {
	growth := max(0, min(3, l.Growth))
	return "minecraft:leaf_litter", map[string]any{
		"growth":                       int32(growth),
		"minecraft:cardinal_direction": l.Facing.String(),
	}
}

type PaleMossBlock struct{ base }

func (PaleMossBlock) EncodeBlock() (string, map[string]any) { return "minecraft:pale_moss_block", nil }

type PaleMossCarpetSide struct{ name string }

func PaleMossCarpetNone() PaleMossCarpetSide  { return PaleMossCarpetSide{"none"} }
func PaleMossCarpetShort() PaleMossCarpetSide { return PaleMossCarpetSide{"short"} }
func PaleMossCarpetTall() PaleMossCarpetSide  { return PaleMossCarpetSide{"tall"} }

type PaleMossCarpet struct {
	base
	Upper bool
	North PaleMossCarpetSide
	East  PaleMossCarpetSide
	South PaleMossCarpetSide
	West  PaleMossCarpetSide
}

func (p PaleMossCarpet) EncodeBlock() (string, map[string]any) {
	return "minecraft:pale_moss_carpet", map[string]any{
		"upper_block_bit":             p.Upper,
		"pale_moss_carpet_side_north": sideName(p.North),
		"pale_moss_carpet_side_east":  sideName(p.East),
		"pale_moss_carpet_side_south": sideName(p.South),
		"pale_moss_carpet_side_west":  sideName(p.West),
	}
}

func sideName(s PaleMossCarpetSide) string {
	if s.name == "" {
		return "none"
	}
	return s.name
}

type PaleHangingMoss struct {
	base
	Tip bool
}

func (p PaleHangingMoss) EncodeBlock() (string, map[string]any) {
	return "minecraft:pale_hanging_moss", map[string]any{"tip": p.Tip}
}

type HangingRoots struct{ base }

func (HangingRoots) EncodeBlock() (string, map[string]any) { return "minecraft:hanging_roots", nil }

type CreakingHeartState struct{ name string }

func UprootedCreakingHeart() CreakingHeartState { return CreakingHeartState{"uprooted"} }
func DormantCreakingHeart() CreakingHeartState  { return CreakingHeartState{"dormant"} }
func AwakeCreakingHeart() CreakingHeartState    { return CreakingHeartState{"awake"} }

type CreakingHeart struct {
	base
	Axis    cube.Axis
	Natural bool
	State   CreakingHeartState
}

func (c CreakingHeart) EncodeBlock() (string, map[string]any) {
	state := c.State.name
	if state == "" {
		state = UprootedCreakingHeart().name
	}
	return "minecraft:creaking_heart", map[string]any{
		"pillar_axis":          c.Axis.String(),
		"natural":              c.Natural,
		"creaking_heart_state": state,
	}
}

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

type Azalea struct {
	base
	Flowering bool
}

func (a Azalea) EncodeBlock() (string, map[string]any) {
	if a.Flowering {
		return "minecraft:flowering_azalea", nil
	}
	return "minecraft:azalea", nil
}
