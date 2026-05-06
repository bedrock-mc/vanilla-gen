package block

import "github.com/df-mc/dragonfly/server/block/cube"

type DripleafTilt struct{ name string }

func DripleafTiltNone() DripleafTilt     { return DripleafTilt{"none"} }
func DripleafTiltUnstable() DripleafTilt { return DripleafTilt{"unstable"} }
func DripleafTiltPartial() DripleafTilt  { return DripleafTilt{"partial_tilt"} }
func DripleafTiltFull() DripleafTilt     { return DripleafTilt{"full_tilt"} }

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

type Fungus struct {
	base
	Type string
}

func (f Fungus) EncodeBlock() (string, map[string]any) {
	if f.Type == "warped" {
		return "minecraft:warped_fungus", nil
	}
	return "minecraft:crimson_fungus", nil
}

type Roots struct {
	base
	Type string
}

func (r Roots) EncodeBlock() (string, map[string]any) {
	if r.Type == "warped" {
		return "minecraft:warped_roots", nil
	}
	return "minecraft:crimson_roots", nil
}

type NetherVines struct {
	base
	Twisting bool
	Age      int
}

func (n NetherVines) EncodeBlock() (string, map[string]any) {
	name := "minecraft:weeping_vines"
	if n.Twisting {
		name = "minecraft:twisting_vines"
	}
	return name, map[string]any{"twisting_vines_age": int32(max(0, min(25, n.Age)))}
}

type Nylium struct {
	base
	Type string
}

func (n Nylium) EncodeBlock() (string, map[string]any) {
	if n.Type == "warped" {
		return "minecraft:warped_nylium", nil
	}
	return "minecraft:crimson_nylium", nil
}

type ChorusPlant struct{ base }

func (ChorusPlant) EncodeBlock() (string, map[string]any) { return "minecraft:chorus_plant", nil }

type ChorusFlower struct {
	base
	Age int
}

func (c ChorusFlower) EncodeBlock() (string, map[string]any) {
	return "minecraft:chorus_flower", map[string]any{"age": int32(max(0, min(5, c.Age)))}
}

type EndPortal struct{ base }

func (EndPortal) EncodeBlock() (string, map[string]any) { return "minecraft:end_portal", nil }

type EndPortalFrame struct {
	base
	Facing cube.Direction
	Eye    bool
}

func (e EndPortalFrame) EncodeBlock() (string, map[string]any) {
	return "minecraft:end_portal_frame", map[string]any{
		"minecraft:cardinal_direction": e.Facing.String(),
		"end_portal_eye_bit":           e.Eye,
	}
}

type Spawner struct{ base }

func (Spawner) EncodeBlock() (string, map[string]any) { return "minecraft:mob_spawner", nil }

type MushroomBlock struct {
	base
	Name string
}

func (m MushroomBlock) EncodeBlock() (string, map[string]any) {
	name := m.Name
	if name == "" {
		name = "minecraft:mushroom_stem"
	}
	return name, nil
}

type BrownMushroomBlock struct{ base }

func (BrownMushroomBlock) EncodeBlock() (string, map[string]any) {
	return "minecraft:brown_mushroom_block", nil
}

type RedMushroomBlock struct{ base }

func (RedMushroomBlock) EncodeBlock() (string, map[string]any) {
	return "minecraft:red_mushroom_block", nil
}

type MushroomStem struct{ base }

func (MushroomStem) EncodeBlock() (string, map[string]any) { return "minecraft:mushroom_stem", nil }

type BuddingAmethyst struct{ base }

func (BuddingAmethyst) EncodeBlock() (string, map[string]any) {
	return "minecraft:budding_amethyst", nil
}

type AmethystBud struct {
	base
	Size string
	Face cube.Face
}

func (a AmethystBud) EncodeBlock() (string, map[string]any) {
	name := "minecraft:amethyst_cluster"
	switch a.Size {
	case "small":
		name = "minecraft:small_amethyst_bud"
	case "medium":
		name = "minecraft:medium_amethyst_bud"
	case "large":
		name = "minecraft:large_amethyst_bud"
	}
	return name, map[string]any{"minecraft:block_face": a.Face.String()}
}

type SmallAmethystBud struct {
	base
	Face cube.Face
}

func (s SmallAmethystBud) EncodeBlock() (string, map[string]any) {
	return AmethystBud{Size: "small", Face: s.Face}.EncodeBlock()
}

type MediumAmethystBud struct {
	base
	Face cube.Face
}

func (m MediumAmethystBud) EncodeBlock() (string, map[string]any) {
	return AmethystBud{Size: "medium", Face: m.Face}.EncodeBlock()
}

type LargeAmethystBud struct {
	base
	Face cube.Face
}

func (l LargeAmethystBud) EncodeBlock() (string, map[string]any) {
	return AmethystBud{Size: "large", Face: l.Face}.EncodeBlock()
}

type AmethystCluster struct {
	base
	Face cube.Face
}

func (a AmethystCluster) EncodeBlock() (string, map[string]any) {
	return AmethystBud{Face: a.Face}.EncodeBlock()
}
