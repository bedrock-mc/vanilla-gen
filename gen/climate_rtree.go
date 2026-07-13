package gen

import (
	"math"
	"sort"
)

// climateRTree ports Climate.RTree: the 7-dimensional bounding-interval tree
// vanilla uses to find the closest biome parameter point. Build order (stable
// sorts, bucket sizes, dimension choice) matches vanilla exactly because the
// traversal order decides exact-fitness ties.
//
// Vanilla additionally seeds each search with the thread's previous result
// (Climate.RTree.lastResult), which makes tie outcomes depend on query order.
// This port searches without a seed, which matches vanilla's behaviour for a
// fresh thread and keeps results deterministic.
type climateRTree struct {
	root *climateRTreeNode
}

type climateRTreeNode struct {
	space    [7]climateParameter
	children []*climateRTreeNode // nil for leaves
	biome    Biome
}

func newClimateRTree(points []climateParameterPoint) *climateRTree {
	leaves := make([]*climateRTreeNode, len(points))
	for i, p := range points {
		leaves[i] = &climateRTreeNode{
			space: [7]climateParameter{
				p.params[0], p.params[1], p.params[2],
				p.params[3], p.params[4], p.params[5],
				{min: p.offset, max: p.offset},
			},
			biome: p.biome,
		}
	}
	return &climateRTree{root: buildClimateRTreeNode(leaves)}
}

func buildClimateRTreeNode(children []*climateRTreeNode) *climateRTreeNode {
	const dimensions = 7
	if len(children) == 1 {
		return children[0]
	}
	if len(children) <= 6 {
		sort.SliceStable(children, func(a, b int) bool {
			return climateNodeMagnitude(children[a]) < climateNodeMagnitude(children[b])
		})
		return newClimateSubTree(children)
	}

	minCost := int64(math.MaxInt64)
	minDimension := -1
	var minBuckets []*climateRTreeNode
	for d := 0; d < dimensions; d++ {
		sortClimateNodes(children, d, false)
		buckets := bucketizeClimateNodes(children)
		totalCost := int64(0)
		for _, bucket := range buckets {
			totalCost += climateSpaceCost(&bucket.space)
		}
		if minCost > totalCost {
			minCost = totalCost
			minDimension = d
			minBuckets = buckets
		}
	}

	sortClimateNodes(minBuckets, minDimension, true)
	rebuilt := make([]*climateRTreeNode, len(minBuckets))
	for i, bucket := range minBuckets {
		rebuilt[i] = buildClimateRTreeNode(bucket.children)
	}
	return newClimateSubTree(rebuilt)
}

func newClimateSubTree(children []*climateRTreeNode) *climateRTreeNode {
	node := &climateRTreeNode{children: append([]*climateRTreeNode(nil), children...)}
	for d := 0; d < 7; d++ {
		first := true
		for _, child := range node.children {
			if first {
				node.space[d] = child.space[d]
				first = false
				continue
			}
			if child.space[d].min < node.space[d].min {
				node.space[d].min = child.space[d].min
			}
			if child.space[d].max > node.space[d].max {
				node.space[d].max = child.space[d].max
			}
		}
	}
	return node
}

func climateNodeMagnitude(node *climateRTreeNode) int64 {
	total := int64(0)
	for d := 0; d < 7; d++ {
		p := node.space[d]
		mid := (p.min + p.max) / 2
		if mid < 0 {
			mid = -mid
		}
		total += mid
	}
	return total
}

// sortClimateNodes mirrors RTree.sort: a stable sort by the (possibly
// absolute) interval centers of dimension, then each following dimension in
// cyclic order.
func sortClimateNodes(children []*climateRTreeNode, dimension int, absolute bool) {
	const dimensions = 7
	center := func(node *climateRTreeNode, d int) int64 {
		p := node.space[d]
		mid := (p.min + p.max) / 2
		if absolute && mid < 0 {
			mid = -mid
		}
		return mid
	}
	sort.SliceStable(children, func(a, b int) bool {
		for i := 0; i < dimensions; i++ {
			d := (dimension + i) % dimensions
			ca := center(children[a], d)
			cb := center(children[b], d)
			if ca != cb {
				return ca < cb
			}
		}
		return false
	})
}

func bucketizeClimateNodes(nodes []*climateRTreeNode) []*climateRTreeNode {
	expected := int(math.Pow(6.0, math.Floor(math.Log(float64(len(nodes))-0.01)/math.Log(6.0))))
	var buckets []*climateRTreeNode
	var children []*climateRTreeNode
	for _, child := range nodes {
		children = append(children, child)
		if len(children) >= expected {
			buckets = append(buckets, newClimateSubTree(children))
			children = nil
		}
	}
	if len(children) > 0 {
		buckets = append(buckets, newClimateSubTree(children))
	}
	return buckets
}

func climateSpaceCost(space *[7]climateParameter) int64 {
	total := int64(0)
	for _, p := range space {
		d := p.max - p.min
		if d < 0 {
			d = -d
		}
		total += d
	}
	return total
}

func (n *climateRTreeNode) distance(target *[7]int64) int64 {
	total := int64(0)
	for i := 0; i < 7; i++ {
		d := n.space[i].distance(target[i])
		total += d * d
	}
	return total
}

func (n *climateRTreeNode) search(target *[7]int64, candidate *climateRTreeNode) *climateRTreeNode {
	if n.children == nil {
		return n
	}
	minDistance := int64(math.MaxInt64)
	if candidate != nil {
		minDistance = candidate.distance(target)
	}
	closest := candidate
	for _, child := range n.children {
		childDistance := child.distance(target)
		if minDistance > childDistance {
			leaf := child.search(target, closest)
			leafDistance := childDistance
			if leaf != child {
				leafDistance = leaf.distance(target)
			}
			if minDistance > leafDistance {
				minDistance = leafDistance
				closest = leaf
			}
		}
	}
	return closest
}

func (t *climateRTree) Lookup(climate [6]int64) Biome {
	target := [7]int64{climate[0], climate[1], climate[2], climate[3], climate[4], climate[5], 0}
	leaf := t.root.search(&target, nil)
	if leaf == nil {
		return BiomePlains
	}
	return leaf.biome
}
