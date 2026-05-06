package block

import (
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
)

type CreakingHeartState struct{ name string }

func UprootedCreakingHeart() CreakingHeartState { return CreakingHeartState{"uprooted"} }
func DormantCreakingHeart() CreakingHeartState  { return CreakingHeartState{"dormant"} }
func AwakeCreakingHeart() CreakingHeartState    { return CreakingHeartState{"awake"} }

func CreakingHeartStates() []CreakingHeartState {
	return []CreakingHeartState{UprootedCreakingHeart(), DormantCreakingHeart(), AwakeCreakingHeart()}
}

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

func allCreakingHeart() (b []world.Block) {
	for _, axis := range cube.Axes() {
		for _, state := range CreakingHeartStates() {
			b = append(b, CreakingHeart{Axis: axis, State: state})
			b = append(b, CreakingHeart{Axis: axis, Natural: true, State: state})
		}
	}
	return
}
