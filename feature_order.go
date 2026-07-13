package vanilla

import (
	"fmt"
	"sort"

	gen "github.com/bedrock-mc/vanilla-gen/gen"
)

type featureNode struct {
	step         int
	featureIndex int
	name         string
}

type stepFeatureData struct {
	features    []string
	indexByName map[string]int
}

func (d stepFeatureData) index(name string) (int, bool) {
	index, ok := d.indexByName[name]
	return index, ok
}

// buildStepFeatureData mirrors FeatureSorter.buildFeaturesPerStep over the
// generator's possible biomes in vanilla order; the resulting indices feed
// setFeatureSeed.
func buildStepFeatureData(featureSteps [256][featureStepCount][]string, possibleBiomes []gen.Biome) [featureStepCount]stepFeatureData {
	featureIndexByName := make(map[string]int)
	nextFeatureIndex := 0
	edges := make(map[featureNode]map[featureNode]struct{})
	nodes := make([]featureNode, 0, 512)
	seenNodes := make(map[featureNode]struct{})

	for _, biome := range possibleBiomes {
		var sourceFeatures []featureNode
		for step := 0; step < featureStepCount; step++ {
			for _, name := range featureSteps[biome][step] {
				featureIndex, ok := featureIndexByName[name]
				if !ok {
					featureIndex = nextFeatureIndex
					nextFeatureIndex++
					featureIndexByName[name] = featureIndex
				}
				node := featureNode{step: step, featureIndex: featureIndex, name: name}
				sourceFeatures = append(sourceFeatures, node)
				if _, ok := seenNodes[node]; !ok {
					nodes = append(nodes, node)
					seenNodes[node] = struct{}{}
				}
				if _, ok := edges[node]; !ok {
					edges[node] = make(map[featureNode]struct{})
				}
			}
		}
		for i := 0; i < len(sourceFeatures)-1; i++ {
			edges[sourceFeatures[i]][sourceFeatures[i+1]] = struct{}{}
		}
	}

	sort.Slice(nodes, func(i, j int) bool {
		return compareFeatureNode(nodes[i], nodes[j]) < 0
	})

	visiting := make(map[featureNode]bool, len(nodes))
	visited := make(map[featureNode]bool, len(nodes))
	sortedNodes := make([]featureNode, 0, len(nodes))

	var visit func(featureNode)
	visit = func(node featureNode) {
		if visited[node] {
			return
		}
		if visiting[node] {
			panic(fmt.Sprintf("feature order cycle found at step=%d feature=%s", node.step, node.name))
		}
		visiting[node] = true

		neighbors := make([]featureNode, 0, len(edges[node]))
		for neighbor := range edges[node] {
			neighbors = append(neighbors, neighbor)
		}
		sort.Slice(neighbors, func(i, j int) bool {
			return compareFeatureNode(neighbors[i], neighbors[j]) < 0
		})
		for _, neighbor := range neighbors {
			visit(neighbor)
		}

		visiting[node] = false
		visited[node] = true
		sortedNodes = append(sortedNodes, node)
	}

	for _, node := range nodes {
		visit(node)
	}

	for left, right := 0, len(sortedNodes)-1; left < right; left, right = left+1, right-1 {
		sortedNodes[left], sortedNodes[right] = sortedNodes[right], sortedNodes[left]
	}

	var out [featureStepCount]stepFeatureData
	for step := 0; step < featureStepCount; step++ {
		names := make([]string, 0, 64)
		indexByName := make(map[string]int)
		for _, node := range sortedNodes {
			if node.step != step {
				continue
			}
			indexByName[node.name] = len(names)
			names = append(names, node.name)
		}
		out[step] = stepFeatureData{features: names, indexByName: indexByName}
	}
	return out
}

func compareFeatureNode(left, right featureNode) int {
	switch {
	case left.step < right.step:
		return -1
	case left.step > right.step:
		return 1
	case left.featureIndex < right.featureIndex:
		return -1
	case left.featureIndex > right.featureIndex:
		return 1
	default:
		return 0
	}
}

func (idx *biomeGenerationIndex) featureIndexesForStep(biomes []gen.Biome, step gen.GenerationStep) []int {
	if idx == nil {
		return nil
	}
	maxIndex := len(idx.stepFeatures[int(step)].features)
	if maxIndex == 0 {
		return nil
	}

	seen := make([]bool, maxIndex)
	out := make([]int, 0, maxIndex)
	for _, biome := range biomes {
		for _, index := range idx.featureIndexes[biome][int(step)] {
			if index < 0 || index >= len(seen) || seen[index] {
				continue
			}
			seen[index] = true
			out = append(out, index)
		}
	}
	sort.Ints(out)
	return out
}

func (idx *biomeGenerationIndex) featureIndex(step gen.GenerationStep, featureName string) (int, bool) {
	if idx == nil {
		return 0, false
	}
	return idx.stepFeatures[int(step)].index(featureName)
}

func (idx *biomeGenerationIndex) biomeHasFeature(biome gen.Biome, featureName string) bool {
	if idx == nil {
		return false
	}
	_, ok := idx.featureMembership[biome][featureName]
	return ok
}
