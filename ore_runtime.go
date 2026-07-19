package vanilla

import (
	"strings"
	"sync"

	gen "github.com/bedrock-mc/vanilla-gen/gen"
	"github.com/df-mc/dragonfly/server/world"
)

const (
	oreTagStone uint8 = 1 << iota
	oreTagDeepslate
	oreTagNether
)

type oreRuntimeData struct {
	tagMasks  []uint8
	javaSolid []bool
	blockRIDs map[string]uint32
}

var (
	sharedOreRuntimeOnce sync.Once
	sharedOreRuntime     *oreRuntimeData
)

func sharedOreRuntimeData() *oreRuntimeData {
	sharedOreRuntimeOnce.Do(func() {
		blocks := world.Blocks()
		data := &oreRuntimeData{
			tagMasks:  make([]uint8, len(blocks)),
			javaSolid: make([]bool, len(blocks)),
			blockRIDs: make(map[string]uint32),
		}
		for rid, b := range blocks {
			if b == nil {
				continue
			}
			name := featureBlockName(b)
			name = strings.TrimPrefix(name, "minecraft:")
			data.blockRIDs[name] = uint32(rid)
			data.javaSolid[rid] = (Generator{}).javaSolidName(name)
			switch name {
			case "stone", "granite", "diorite", "andesite":
				data.tagMasks[rid] |= oreTagStone
			case "deepslate", "tuff":
				data.tagMasks[rid] |= oreTagDeepslate
			case "netherrack", "basalt", "blackstone":
				data.tagMasks[rid] |= oreTagNether
			}
		}
		sharedOreRuntime = data
	})
	return sharedOreRuntime
}

func (g Generator) javaSolidRuntimeID(rid uint32) bool {
	data := g.oreRuntime
	if data == nil {
		data = sharedOreRuntimeData()
	}
	if int(rid) < len(data.javaSolid) {
		return data.javaSolid[rid]
	}
	return g.javaSolidName(g.blockNameForRuntimeID(rid))
}

func (g Generator) oreTargetMatchesRuntimeID(rid uint32, target gen.OreTargetConfig) bool {
	data := g.oreRuntime
	if data == nil {
		data = sharedOreRuntimeData()
	}
	switch target.Target.PredicateType {
	case "tag_match":
		if int(rid) < len(data.tagMasks) {
			mask := data.tagMasks[rid]
			switch target.Target.Tag {
			case "stone_ore_replaceables":
				return mask&oreTagStone != 0
			case "deepslate_ore_replaceables":
				return mask&oreTagDeepslate != 0
			case "base_stone_overworld":
				return mask&(oreTagStone|oreTagDeepslate) != 0
			case "base_stone_nether":
				return mask&oreTagNether != 0
			}
		}
	case "block_match":
		if expected, ok := data.blockRIDs[target.Target.Block]; ok {
			return rid == expected
		}
	}
	// Keep uncommon/future data-pack predicates on the general exact path.
	return g.oreTargetMatches(g.blockNameForRuntimeID(rid), target)
}
