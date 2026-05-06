package block_test

import (
	"testing"

	"github.com/bedrock-mc/vanilla-gen/block"
	"github.com/df-mc/dragonfly/server/world"
)

func TestImportRegistersMissingBlockStates(t *testing.T) {
	b, ok := world.BlockByName("minecraft:bamboo", map[string]any{
		"age_bit":                false,
		"bamboo_leaf_size":       "no_leaves",
		"bamboo_stalk_thickness": "thin",
	})
	if !ok {
		t.Fatal("expected minecraft:bamboo to exist in Dragonfly block states")
	}
	if _, ok := b.(block.Bamboo); !ok {
		t.Fatalf("expected minecraft:bamboo to be registered by vanilla-gen/block, got %T", b)
	}
}

func TestTypedMissingBlocksEncode(t *testing.T) {
	b := block.Azalea{Flowering: true}
	name, properties := b.EncodeBlock()
	if name != "minecraft:flowering_azalea" || properties != nil {
		t.Fatalf("unexpected azalea encoding: %s %#v", name, properties)
	}
}
