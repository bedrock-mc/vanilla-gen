package block

import "github.com/df-mc/dragonfly/server/world"

type PaleMossBlock struct{ base }

func (PaleMossBlock) EncodeBlock() (string, map[string]any) { return "minecraft:pale_moss_block", nil }

type PaleMossCarpetSide struct{ name string }

func PaleMossCarpetNone() PaleMossCarpetSide  { return PaleMossCarpetSide{"none"} }
func PaleMossCarpetShort() PaleMossCarpetSide { return PaleMossCarpetSide{"short"} }
func PaleMossCarpetTall() PaleMossCarpetSide  { return PaleMossCarpetSide{"tall"} }

func PaleMossCarpetSides() []PaleMossCarpetSide {
	return []PaleMossCarpetSide{PaleMossCarpetNone(), PaleMossCarpetShort(), PaleMossCarpetTall()}
}

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
		"pale_moss_carpet_side_north": paleMossCarpetSideName(p.North),
		"pale_moss_carpet_side_east":  paleMossCarpetSideName(p.East),
		"pale_moss_carpet_side_south": paleMossCarpetSideName(p.South),
		"pale_moss_carpet_side_west":  paleMossCarpetSideName(p.West),
	}
}

func paleMossCarpetSideName(s PaleMossCarpetSide) string {
	if s.name == "" {
		return "none"
	}
	return s.name
}

func allPaleMossCarpet() (b []world.Block) {
	for _, north := range PaleMossCarpetSides() {
		for _, east := range PaleMossCarpetSides() {
			for _, south := range PaleMossCarpetSides() {
				for _, west := range PaleMossCarpetSides() {
					b = append(b, PaleMossCarpet{North: north, East: east, South: south, West: west})
					b = append(b, PaleMossCarpet{Upper: true, North: north, East: east, South: south, West: west})
				}
			}
		}
	}
	return
}

type PaleHangingMoss struct {
	base
	Tip bool
}

func (p PaleHangingMoss) EncodeBlock() (string, map[string]any) {
	return "minecraft:pale_hanging_moss", map[string]any{"tip": p.Tip}
}

func allPaleHangingMoss() []world.Block {
	return []world.Block{PaleHangingMoss{}, PaleHangingMoss{Tip: true}}
}
