package gen

import (
	"encoding/json"
	"math"
	"os"
	"strconv"
	"testing"
)

type routerFixture struct {
	Seed   int64 `json:"seed"`
	Points []struct {
		X      int               `json:"x"`
		Y      int               `json:"y"`
		Z      int               `json:"z"`
		Values map[string]string `json:"values"`
	} `json:"points"`
}

// TestRouterParitySeed1 compares every overworld noise router slot against
// values dumped from the real 1.21.11 server (RandomState seed 1).
func TestRouterParitySeed1(t *testing.T) {
	data, err := os.ReadFile("testdata/router_seed1.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fx routerFixture
	if err := json.Unmarshal(data, &fx); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}

	noises := NewNoiseRegistry(fx.Seed)
	graph := OverworldGraph

	mismatches := map[string]int{}
	total := map[string]int{}
	var firstMismatch map[string]string

	for _, p := range fx.Points {
		// The Java fixture evaluated router slots at single points with cache
		// markers acting as pass-through, so evaluate without caches here.
		ctx := FunctionContext{BlockX: p.X, BlockY: p.Y, BlockZ: p.Z}

		for slot, wantStr := range p.Values {
			root, ok := OverworldRoots[slot]
			if !ok {
				t.Fatalf("no graph root for router slot %q", slot)
			}
			want, err := strconv.ParseFloat(wantStr, 64)
			if err != nil {
				t.Fatalf("parse %q: %v", wantStr, err)
			}
			got := graph.Eval(root, ctx, noises, nil, nil, nil)
			total[slot]++
			if got != want && !(math.IsNaN(got) && math.IsNaN(want)) {
				mismatches[slot]++
				if firstMismatch == nil {
					firstMismatch = map[string]string{}
				}
				if _, seen := firstMismatch[slot]; !seen {
					firstMismatch[slot] = "at (" + itoa(p.X) + "," + itoa(p.Y) + "," + itoa(p.Z) + ") got " +
						strconv.FormatFloat(got, 'g', 17, 64) + " want " + wantStr
				}
			}
		}
	}

	for slot, count := range total {
		if bad := mismatches[slot]; bad > 0 {
			t.Errorf("slot %s: %d/%d mismatched; first: %s", slot, bad, count, firstMismatch[slot])
		}
	}
}

func itoa(v int) string { return strconv.Itoa(v) }
