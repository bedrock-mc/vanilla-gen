package vanilla

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestFeatureOrderParity compares the per-step feature index assignment with
// vanilla FeatureSorter.buildFeaturesPerStep output: the indices feed
// setFeatureSeed, so the order must match exactly.
func TestFeatureOrderParity(t *testing.T) {
	data, err := os.ReadFile("gen/testdata/feature_order.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fx struct {
		Steps [][]string `json:"steps"`
	}
	if err := json.Unmarshal(data, &fx); err != nil {
		t.Fatalf("parse: %v", err)
	}

	g := New(1)
	for step, want := range fx.Steps {
		if step >= len(g.biomeGeneration.stepFeatures) {
			t.Fatalf("missing step %d locally", step)
		}
		got := g.biomeGeneration.stepFeatures[step].features
		bad := 0
		for i := range want {
			wantName := strings.TrimPrefix(want[i], "minecraft:")
			var gotName string
			if i < len(got) {
				gotName = strings.TrimPrefix(got[i], "minecraft:")
			}
			if gotName != wantName {
				bad++
				if bad == 1 {
					t.Errorf("step %d index %d: got %q want %q", step, i, gotName, wantName)
				}
			}
		}
		if len(got) != len(want) {
			t.Errorf("step %d: %d features locally, %d in vanilla", step, len(got), len(want))
		} else if bad > 0 {
			t.Errorf("step %d: %d/%d indices mismatched", step, bad, len(want))
		}
	}
}
