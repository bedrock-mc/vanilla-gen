package vanilla

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	gen "github.com/bedrock-mc/vanilla-gen/gen"
)

type climateFixture struct {
	Seed    int64 `json:"seed"`
	Samples []struct {
		X      int `json:"x"`
		Y      int `json:"y"`
		Z      int `json:"z"`
		Target struct {
			Temperature     int64 `json:"temperature"`
			Humidity        int64 `json:"humidity"`
			Continentalness int64 `json:"continentalness"`
			Erosion         int64 `json:"erosion"`
			Depth           int64 `json:"depth"`
			Weirdness       int64 `json:"weirdness"`
		} `json:"target"`
		Biome string `json:"biome"`
	} `json:"samples"`
}

// TestClimateParitySeed1 compares the six climate parameters and the selected
// biome against values dumped from the real 1.21.11 Climate.Sampler and
// MultiNoiseBiomeSource at quart coordinates.
func TestClimateParitySeed1(t *testing.T) {
	data, err := os.ReadFile("gen/testdata/climate_seed1.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fx climateFixture
	if err := json.Unmarshal(data, &fx); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}

	noises := gen.NewNoiseRegistry(fx.Seed)
	graph := gen.OverworldGraph
	roots := gen.OverworldRoots
	slots := []string{"temperature", "vegetation", "continents", "erosion", "depth", "ridges"}
	rootIdx := make([]int, len(slots))
	for i, name := range slots {
		idx, ok := roots[name]
		if !ok {
			t.Fatalf("missing root %s", name)
		}
		rootIdx[i] = idx
	}

	worldgen := gen.NewWorldgenRegistry()
	source, err := gen.NewBiomeSource(fx.Seed, worldgen, "overworld")
	if err != nil {
		t.Fatalf("biome source: %v", err)
	}

	climateBad := 0
	biomeBad := 0
	firstClimate := ""
	firstBiome := ""
	for _, s := range fx.Samples {
		bx, by, bz := s.X<<2, s.Y<<2, s.Z<<2
		ctx := gen.FunctionContext{BlockX: bx, BlockY: by, BlockZ: bz}
		want := [6]int64{s.Target.Temperature, s.Target.Humidity, s.Target.Continentalness, s.Target.Erosion, s.Target.Depth, s.Target.Weirdness}
		var got [6]int64
		ok := true
		for i, idx := range rootIdx {
			got[i] = int64(float32(graph.Eval(idx, ctx, noises, nil, nil, nil)) * 10000.0)
			if got[i] != want[i] {
				ok = false
			}
		}
		if !ok {
			climateBad++
			if firstClimate == "" {
				firstClimate = strings.Join([]string{"at quart", itoa3(s.X, s.Y, s.Z), "got", jsonNums(got[:]), "want", jsonNums(want[:])}, " ")
			}
		}

		wantBiome := strings.TrimPrefix(s.Biome, "minecraft:")
		gotBiome := biomeKey(source.GetBiome(bx, by, bz))
		if gotBiome != wantBiome {
			biomeBad++
			if firstBiome == "" {
				firstBiome = "at quart " + itoa3(s.X, s.Y, s.Z) + " got " + gotBiome + " want " + wantBiome
			}
		}
	}

	if climateBad > 0 {
		t.Errorf("climate: %d/%d samples mismatched; first: %s", climateBad, len(fx.Samples), firstClimate)
	}
	if biomeBad > 0 {
		t.Errorf("biome: %d/%d samples mismatched; first: %s", biomeBad, len(fx.Samples), firstBiome)
	}
}

func itoa3(x, y, z int) string {
	b, _ := json.Marshal([3]int{x, y, z})
	return string(b)
}

func jsonNums(v []int64) string {
	b, _ := json.Marshal(v)
	return string(b)
}
