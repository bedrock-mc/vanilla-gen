package block

import (
	"github.com/bedrock-mc/vanilla-gen/block/model"
	"github.com/df-mc/dragonfly/server/world"
)

// ChorusPlant is a branching End plant.
type ChorusPlant struct{ base }

func (ChorusPlant) Model() world.BlockModel { return model.Chorus{} }

func (ChorusPlant) EncodeItem() (name string, meta int16) {
	return "minecraft:chorus_plant", 0
}

func (ChorusPlant) EncodeBlock() (string, map[string]any) {
	return "minecraft:chorus_plant", nil
}

// ChorusFlower is the flower that grows on chorus plants.
type ChorusFlower struct {
	base
	Age int
}

func (ChorusFlower) Model() world.BlockModel { return model.Chorus{} }

func (ChorusFlower) EncodeItem() (name string, meta int16) {
	return "minecraft:chorus_flower", 0
}

func (c ChorusFlower) EncodeBlock() (string, map[string]any) {
	return "minecraft:chorus_flower", map[string]any{"age": int32(c.Age)}
}

func allChorusFlowers() (b []world.Block) {
	for age := 0; age <= 5; age++ {
		b = append(b, ChorusFlower{Age: age})
	}
	return
}
