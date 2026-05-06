package block

import (
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
)

// NetherVines are climbable non-solid Nether plants.
type NetherVines struct {
	base
	Twisting bool
	Age      int
}

func (NetherVines) EntityInside(_ cube.Pos, _ *world.Tx, e world.Entity) {
	if fallEntity, ok := e.(interface{ ResetFallDistance() }); ok {
		fallEntity.ResetFallDistance()
	}
}

func (NetherVines) HasLiquidDrops() bool { return false }

func (NetherVines) CompostChance() float64 { return 0.5 }

func (v NetherVines) EncodeItem() (name string, meta int16) {
	if v.Twisting {
		return "minecraft:twisting_vines", 0
	}
	return "minecraft:weeping_vines", 0
}

func (v NetherVines) EncodeBlock() (string, map[string]any) {
	if v.Twisting {
		return "minecraft:twisting_vines", map[string]any{"twisting_vines_age": int32(v.Age)}
	}
	return "minecraft:weeping_vines", map[string]any{"weeping_vines_age": int32(v.Age)}
}

func allNetherVines() (b []world.Block) {
	for age := 0; age <= 25; age++ {
		b = append(b, NetherVines{Twisting: true, Age: age})
		b = append(b, NetherVines{Twisting: false, Age: age})
	}
	return
}
