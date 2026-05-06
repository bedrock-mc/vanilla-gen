package block

import "github.com/df-mc/dragonfly/server/world"

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
