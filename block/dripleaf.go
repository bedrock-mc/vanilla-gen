package block

import (
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
)

type DripleafTilt struct{ name string }

func DripleafTiltNone() DripleafTilt     { return DripleafTilt{"none"} }
func DripleafTiltUnstable() DripleafTilt { return DripleafTilt{"unstable"} }
func DripleafTiltPartial() DripleafTilt  { return DripleafTilt{"partial_tilt"} }
func DripleafTiltFull() DripleafTilt     { return DripleafTilt{"full_tilt"} }

func DripleafTilts() []DripleafTilt {
	return []DripleafTilt{DripleafTiltNone(), DripleafTiltUnstable(), DripleafTiltPartial(), DripleafTiltFull()}
}

type BigDripleaf struct {
	base
	Head   bool
	Tilt   DripleafTilt
	Facing cube.Direction
}

func (b BigDripleaf) EncodeBlock() (string, map[string]any) {
	tilt := b.Tilt.name
	if tilt == "" {
		tilt = DripleafTiltNone().name
	}
	name := "minecraft:big_dripleaf"
	if !b.Head {
		name = "minecraft:big_dripleaf_stem"
	}
	return name, map[string]any{
		"big_dripleaf_tilt":            tilt,
		"minecraft:cardinal_direction": b.Facing.String(),
	}
}

func allBigDripleaf() (b []world.Block) {
	for _, facing := range []cube.Direction{cube.North, cube.East, cube.South, cube.West} {
		for _, tilt := range DripleafTilts() {
			b = append(b, BigDripleaf{Head: true, Facing: facing, Tilt: tilt})
			b = append(b, BigDripleaf{Facing: facing, Tilt: tilt})
		}
	}
	return
}

type SmallDripleaf struct {
	base
	Upper  bool
	Facing cube.Direction
}

func (s SmallDripleaf) EncodeBlock() (string, map[string]any) {
	return "minecraft:small_dripleaf_block", map[string]any{
		"upper_block_bit":              s.Upper,
		"minecraft:cardinal_direction": s.Facing.String(),
	}
}

func allSmallDripleaf() (b []world.Block) {
	for _, facing := range []cube.Direction{cube.North, cube.East, cube.South, cube.West} {
		b = append(b, SmallDripleaf{Facing: facing})
		b = append(b, SmallDripleaf{Upper: true, Facing: facing})
	}
	return
}
