package bloco_test

import (
	"testing"

	"github.com/bedrock-mc/vanilla-gen/bloco"
	"github.com/df-mc/dragonfly/server/world"
)

func TestBlankImportRegistersMissingBlocks(t *testing.T) {
	b, ok := world.BlockByName("minecraft:bamboo", map[string]any{
		"age_bit":                false,
		"bamboo_leaf_size":       "no_leaves",
		"bamboo_stalk_thickness": "thin",
	})
	if !ok {
		t.Fatal("expected minecraft:bamboo to exist in Dragonfly block states")
	}
	if _, ok := b.(bloco.Block); !ok {
		t.Fatal("expected minecraft:bamboo to be registered by bloco")
	}
}
