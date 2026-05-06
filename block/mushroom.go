package block

import "github.com/df-mc/dragonfly/server/world"

type BrownMushroomBlock struct{ base }

func (BrownMushroomBlock) EncodeBlock() (string, map[string]any) {
	return "minecraft:brown_mushroom_block", nil
}

func allBrownMushroomBlock() []world.Block {
	return []world.Block{BrownMushroomBlock{}}
}

type RedMushroomBlock struct{ base }

func (RedMushroomBlock) EncodeBlock() (string, map[string]any) {
	return "minecraft:red_mushroom_block", nil
}

func allRedMushroomBlock() []world.Block {
	return []world.Block{RedMushroomBlock{}}
}

type MushroomStem struct{ base }

func (MushroomStem) EncodeBlock() (string, map[string]any) {
	return "minecraft:mushroom_stem", nil
}

func allMushroomStem() []world.Block {
	return []world.Block{MushroomStem{}}
}
