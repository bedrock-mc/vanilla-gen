package parity

import (
	"os"
	"strings"
	"testing"
)

func openGroundTruth(t *testing.T) *World {
	t.Helper()
	dir := os.Getenv("VANILLA_GT_REGION_DIR")
	if dir == "" {
		t.Skip("VANILLA_GT_REGION_DIR not set")
	}
	w, err := OpenRegionDir(dir)
	if err != nil {
		t.Fatalf("OpenRegionDir: %v", err)
	}
	return w
}

func TestGroundTruthReadable(t *testing.T) {
	w := openGroundTruth(t)
	c, err := w.Chunk(0, 0)
	if err != nil {
		t.Fatalf("Chunk(0,0): %v", err)
	}
	if c.DataVersion <= 0 {
		t.Errorf("DataVersion = %d, want > 0", c.DataVersion)
	}
	t.Logf("DataVersion = %d", c.DataVersion)
	if c.Status != "minecraft:full" {
		t.Errorf("Status = %q, want minecraft:full", c.Status)
	}
	if bs := c.BlockState(0, -64, 0); bs == "minecraft:air" {
		t.Errorf("BlockState(0,-64,0) = %q, want non-air", bs)
	} else {
		t.Logf("BlockState(0,-64,0) = %s", bs)
	}
	if bs := c.BlockState(8, 20, 8); strings.Contains(bs, "air") {
		t.Logf("BlockState(8,20,8) = %s (cave?)", bs)
	} else {
		t.Logf("BlockState(8,20,8) = %s", bs)
	}
	t.Logf("section y=0 palette: %v", c.SectionBlockPalette(0))
	t.Logf("Biome(0,16,0) = %s", c.Biome(0, 16, 0))
	hm := c.Heightmaps()
	for _, name := range []string{"WORLD_SURFACE", "OCEAN_FLOOR"} {
		vals, ok := hm[name]
		if !ok {
			t.Errorf("heightmap %s missing", name)
			continue
		}
		if len(vals) != 256 {
			t.Errorf("heightmap %s has %d entries, want 256", name, len(vals))
			continue
		}
		t.Logf("%s[0] = %d (worldY %d)", name, vals[0], MinY+vals[0]-1)
	}
}

func TestGroundTruthCoverage(t *testing.T) {
	w := openGroundTruth(t)
	for z := -8; z <= 7; z++ {
		for x := -8; x <= 7; x++ {
			c, err := w.Chunk(x, z)
			if err != nil {
				t.Errorf("Chunk(%d,%d): %v", x, z, err)
				continue
			}
			if c.Status != "minecraft:full" {
				t.Errorf("Chunk(%d,%d): Status = %q, want minecraft:full", x, z, c.Status)
			}
		}
	}
}
