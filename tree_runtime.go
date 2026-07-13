package vanilla

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"slices"
	"sort"
	"strings"

	gen "github.com/bedrock-mc/vanilla-gen/gen"
	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/df-mc/dragonfly/server/world/chunk"
)

type treeDirection int

const (
	treeEast treeDirection = iota
	treeWest
	treeSouth
	treeNorth
)

type treePlacementRecord struct {
	set   map[cube.Pos]struct{}
	order []cube.Pos
}

type treeFoliageAttachment struct {
	pos          cube.Pos
	radiusOffset int
	doubleTrunk  bool
}

type treeRuntime struct {
	g      Generator
	c      *chunk.Chunk
	cfg    gen.TreeConfig
	rng    *gen.WorldgenRandom
	minY   int
	maxY   int
	chunkX int
	chunkZ int
	origin cube.Pos
	biomes sourceBiomeVolume

	trunkCanGrowThroughTag string

	roots       treePlacementRecord
	trunks      treePlacementRecord
	foliage     treePlacementRecord
	decorations treePlacementRecord
}

var debugTreeTraceValue = os.Getenv("DEBUG_TREE_TRACE")

func debugTreeTraceEnabled() bool { return debugTreeTraceValue != "" }

func newTreeRuntime(g Generator, c *chunk.Chunk, origin cube.Pos, cfg gen.TreeConfig, minY, maxY int, rng *gen.WorldgenRandom) treeRuntime {
	chunkX := floorDiv(origin[0], 16)
	chunkZ := floorDiv(origin[2], 16)
	return treeRuntime{
		g:       g,
		c:       c,
		cfg:     cfg,
		rng:     rng,
		minY:    minY,
		maxY:    maxY,
		chunkX:  chunkX,
		chunkZ:  chunkZ,
		origin:  origin,
		biomes:  newSourceBiomeVolume(minY, maxY),
		roots:   newTreePlacementRecord(),
		trunks:  newTreePlacementRecord(),
		foliage: newTreePlacementRecord(),
		decorations: treePlacementRecord{
			set: make(map[cube.Pos]struct{}),
		},
		trunkCanGrowThroughTag: treeTrunkCanGrowThroughTag(cfg.TrunkPlacer),
	}
}

func newTreePlacementRecord() treePlacementRecord {
	return treePlacementRecord{set: make(map[cube.Pos]struct{})}
}

func (r *treePlacementRecord) add(pos cube.Pos) {
	if _, ok := r.set[pos]; ok {
		return
	}
	r.set[pos] = struct{}{}
	r.order = append(r.order, pos)
}

func (r treePlacementRecord) empty() bool {
	return len(r.set) == 0
}

func (r treePlacementRecord) contains(pos cube.Pos) bool {
	_, ok := r.set[pos]
	return ok
}

// sorted mirrors TreeDecorator.Context: the vanilla position sets are
// java.util.HashSets copied into a list (HashSet iteration order) and then
// stable-sorted by Y only.
func (r treePlacementRecord) sorted() []cube.Pos {
	positions := javaHashSetOrder(r.order)
	sort.SliceStable(positions, func(i, j int) bool {
		return positions[i][1] < positions[j][1]
	})
	return positions
}

func (d treeDirection) offset() cube.Pos {
	switch d {
	case treeEast:
		return cube.Pos{1, 0, 0}
	case treeWest:
		return cube.Pos{-1, 0, 0}
	case treeSouth:
		return cube.Pos{0, 0, 1}
	default:
		return cube.Pos{0, 0, -1}
	}
}

func (d treeDirection) axis() string {
	if d == treeEast || d == treeWest {
		return "x"
	}
	return "z"
}

func (d treeDirection) opposite() treeDirection {
	switch d {
	case treeEast:
		return treeWest
	case treeWest:
		return treeEast
	case treeSouth:
		return treeNorth
	default:
		return treeSouth
	}
}

func randomTreeDirection(rng *gen.WorldgenRandom) treeDirection {
	return treeDirection(rng.NextInt(4))
}

func (g Generator) executeJavaTree(c *chunk.Chunk, pos cube.Pos, cfg gen.TreeConfig, minY, maxY int, rng *gen.WorldgenRandom) bool {
	rt := newTreeRuntime(g, c, pos, cfg, minY, maxY, rng)

	treeHeight, _ := sampleTreeHeight(cfg.TrunkPlacer, rng)
	if debugTreeTraceEnabled() {
		fmt.Printf("TRACE tree @ %d %d %d height=%d type=%s\n", pos[0], pos[1], pos[2], treeHeight, cfg.TrunkPlacer.Type)
	}
	if treeHeight <= 0 {
		return false
	}

	foliageHeight := g.treeFoliageHeight(cfg.FoliagePlacer, rng, treeHeight, cfg)
	trunkHeight := treeHeight - foliageHeight
	leafRadius := g.treeFoliageRadius(cfg.FoliagePlacer, rng, trunkHeight)
	trunkOrigin := g.treeTrunkOrigin(cfg, pos, rng)
	minTreeY := min(pos[1], trunkOrigin[1])
	maxTreeY := max(pos[1], trunkOrigin[1]) + treeHeight
	if minTreeY < minY+1 || maxTreeY > maxY {
		return false
	}

	sizeProfile := decodeTreeMinimumSize(cfg.MinimumSize)
	clippedTreeHeight := rt.maxFreeTreeHeight(treeHeight, trunkOrigin, sizeProfile)
	if clippedTreeHeight < treeHeight && (!sizeProfile.hasMinClippedSize || clippedTreeHeight < sizeProfile.minClippedHeight) {
		return false
	}

	if cfg.RootPlacer.Type != "" && !rt.placeRoots(pos, trunkOrigin) {
		return false
	}

	attachments, ok := rt.placeTrunk(clippedTreeHeight, trunkOrigin)
	if !ok {
		return false
	}
	for _, attachment := range attachments {
		rt.placeFoliage(clippedTreeHeight, attachment, foliageHeight, leafRadius)
	}
	if rt.trunks.empty() && rt.foliage.empty() {
		return false
	}

	rt.applyDecorators()
	rt.updateLeaves()
	return true
}

func (g Generator) treeTrunkOrigin(cfg gen.TreeConfig, origin cube.Pos, rng *gen.WorldgenRandom) cube.Pos {
	if cfg.RootPlacer.Type == "" {
		return origin
	}
	var raw struct {
		TrunkOffsetY gen.IntProvider `json:"trunk_offset_y"`
	}
	if err := json.Unmarshal(cfg.RootPlacer.Data, &raw); err != nil {
		return origin
	}
	return origin.Add(cube.Pos{0, g.sampleIntProvider(raw.TrunkOffsetY, rng), 0})
}

func treeTrunkCanGrowThroughTag(placer gen.TypedJSONValue) string {
	if placer.Type != "upwards_branching_trunk_placer" {
		return ""
	}
	var raw struct {
		CanGrowThrough string `json:"can_grow_through"`
	}
	if err := json.Unmarshal(placer.Data, &raw); err != nil {
		return ""
	}
	return raw.CanGrowThrough
}

func (g Generator) treeFoliageHeight(placer gen.TypedJSONValue, rng *gen.WorldgenRandom, treeHeight int, cfg gen.TreeConfig) int {
	switch placer.Type {
	case "blob_foliage_placer", "fancy_foliage_placer", "bush_foliage_placer":
		var raw struct {
			Height int `json:"height"`
		}
		if err := json.Unmarshal(placer.Data, &raw); err != nil {
			return 0
		}
		return raw.Height
	case "spruce_foliage_placer":
		var raw struct {
			TrunkHeight gen.IntProvider `json:"trunk_height"`
		}
		if err := json.Unmarshal(placer.Data, &raw); err != nil {
			return 0
		}
		return max(4, treeHeight-g.sampleIntProvider(raw.TrunkHeight, rng))
	case "pine_foliage_placer":
		var raw struct {
			Height gen.IntProvider `json:"height"`
		}
		if err := json.Unmarshal(placer.Data, &raw); err != nil {
			return 0
		}
		return g.sampleIntProvider(raw.Height, rng)
	case "acacia_foliage_placer":
		return 0
	case "dark_oak_foliage_placer":
		return 4
	case "random_spread_foliage_placer":
		var raw struct {
			FoliageHeight gen.IntProvider `json:"foliage_height"`
		}
		if err := json.Unmarshal(placer.Data, &raw); err != nil {
			return 0
		}
		return g.sampleIntProvider(raw.FoliageHeight, rng)
	case "cherry_foliage_placer":
		var raw struct {
			Height gen.IntProvider `json:"height"`
		}
		if err := json.Unmarshal(placer.Data, &raw); err != nil {
			return 0
		}
		return g.sampleIntProvider(raw.Height, rng)
	case "jungle_foliage_placer":
		var raw struct {
			Height int `json:"height"`
		}
		if err := json.Unmarshal(placer.Data, &raw); err != nil {
			return 0
		}
		return raw.Height
	case "mega_pine_foliage_placer":
		var raw struct {
			CrownHeight gen.IntProvider `json:"crown_height"`
		}
		if err := json.Unmarshal(placer.Data, &raw); err != nil {
			return 0
		}
		return g.sampleIntProvider(raw.CrownHeight, rng)
	default:
		_ = cfg
		return 0
	}
}

func (g Generator) treeFoliageRadius(placer gen.TypedJSONValue, rng *gen.WorldgenRandom, trunkHeight int) int {
	var raw struct {
		Radius gen.IntProvider `json:"radius"`
	}
	if err := json.Unmarshal(placer.Data, &raw); err != nil {
		return 0
	}
	radius := g.sampleIntProvider(raw.Radius, rng)
	if placer.Type == "pine_foliage_placer" {
		return radius + int(rng.NextInt(uint32(max(trunkHeight+1, 1))))
	}
	return radius
}

func (rt treeRuntime) maxFreeTreeHeight(maxTreeHeight int, origin cube.Pos, sizeProfile treeMinimumSizeProfile) int {
	for y := 0; y <= maxTreeHeight+1; y++ {
		radius := sizeProfile.sizeAtHeight(maxTreeHeight, y)
		for dx := -radius; dx <= radius; dx++ {
			for dz := -radius; dz <= radius; dz++ {
				candidate := origin.Add(cube.Pos{dx, y, dz})
				if !rt.trunkIsFree(candidate) || (!rt.cfg.IgnoreVines && rt.isVine(candidate)) {
					return y - 2
				}
			}
		}
	}
	return maxTreeHeight
}

func (rt treeRuntime) trunkIsFree(pos cube.Pos) bool {
	if rt.validTreePosForTrunk(pos) {
		return true
	}
	name := rt.blockNameAt(pos)
	return strings.HasSuffix(name, "_log") || strings.HasSuffix(name, "_wood") || strings.HasSuffix(name, "_stem")
}

func (rt treeRuntime) validTreePosForTrunk(pos cube.Pos) bool {
	if rt.validTreePos(pos) {
		return true
	}
	return rt.trunkCanGrowThroughTag != "" && rt.g.matchesFeatureBlockTag(rt.blockNameAt(pos), rt.trunkCanGrowThroughTag)
}

func (rt treeRuntime) validTreePos(pos cube.Pos) bool {
	if !rt.inChunk(pos) {
		return false
	}
	name := rt.blockNameAt(pos)
	return name == "air" || rt.g.matchesFeatureBlockTag(name, "replaceable_by_trees")
}

func (rt treeRuntime) isVine(pos cube.Pos) bool {
	return rt.blockNameAt(pos) == "vine"
}

func (rt treeRuntime) inChunk(pos cube.Pos) bool {
	if rt.g.activeTreeRegion != nil {
		return rt.g.activeTreeRegion.contains(pos)
	}
	return rt.g.positionInChunk(pos, rt.chunkX, rt.chunkZ, rt.minY, rt.maxY)
}

func (rt treeRuntime) layerRuntimeID(pos cube.Pos, layer uint8) uint32 {
	chunkAtPos, ok := rt.chunkForPos(pos)
	if !ok {
		return rt.g.airRID
	}
	return chunkAtPos.Block(uint8(pos[0]&15), int16(pos[1]), uint8(pos[2]&15), layer)
}

func (rt treeRuntime) containsWater(pos cube.Pos) bool {
	return rt.layerRuntimeID(pos, 0) == rt.g.waterRID || rt.layerRuntimeID(pos, 1) == rt.g.waterRID
}

func (rt treeRuntime) isAir(pos cube.Pos) bool {
	return rt.layerRuntimeID(pos, 0) == rt.g.airRID && rt.layerRuntimeID(pos, 1) == rt.g.airRID
}

func (rt treeRuntime) blockNameAt(pos cube.Pos) string {
	chunkAtPos, ok := rt.chunkForPos(pos)
	if !ok {
		return "air"
	}
	name := rt.g.blockNameAt(chunkAtPos, pos)
	if name != "air" {
		return name
	}
	rid := rt.layerRuntimeID(pos, 1)
	if rid == rt.g.airRID {
		return "air"
	}
	if cached, ok := rt.g.blockNameCache.Lookup(rid); ok {
		return cached
	}
	b, ok := world.BlockByRuntimeID(rid)
	if !ok {
		return "air"
	}
	name = featureBlockName(b)
	rt.g.blockNameCache.Store(rid, name)
	return name
}

func (rt treeRuntime) persistentLeaves(pos cube.Pos) bool {
	chunkAtPos, ok := rt.chunkForPos(pos)
	if !ok {
		return false
	}
	b, ok := world.BlockByRuntimeID(chunkAtPos.Block(uint8(pos[0]&15), int16(pos[1]), uint8(pos[2]&15), 0))
	if !ok {
		return false
	}
	leaves, ok := b.(block.Leaves)
	return ok && leaves.Persistent
}

func (rt treeRuntime) withPotentialWaterlogging(pos cube.Pos, state gen.BlockState) gen.BlockState {
	if state.Properties == nil {
		return state
	}
	if _, ok := state.Properties["waterlogged"]; !ok {
		return state
	}
	props := make(map[string]string, len(state.Properties))
	for key, value := range state.Properties {
		props[key] = value
	}
	if rt.containsWater(pos) {
		props["waterlogged"] = "true"
	} else {
		props["waterlogged"] = "false"
	}
	state.Properties = props
	return state
}

func (rt *treeRuntime) setRootState(pos cube.Pos, state gen.BlockState) bool {
	if !rt.inChunk(pos) {
		return false
	}
	if !rt.g.setBlockStateDirect(rt.c, pos, rt.withPotentialWaterlogging(pos, state)) {
		return false
	}
	rt.roots.add(pos)
	return true
}

func (rt *treeRuntime) setTrunkState(pos cube.Pos, state gen.BlockState) bool {
	if !rt.inChunk(pos) {
		return false
	}
	if !rt.g.setBlockStateDirect(rt.c, pos, rt.withPotentialWaterlogging(pos, state)) {
		return false
	}
	rt.trunks.add(pos)
	return true
}

func (rt *treeRuntime) setFoliageState(pos cube.Pos, state gen.BlockState) bool {
	if !rt.inChunk(pos) {
		return false
	}
	if !rt.g.setBlockStateDirect(rt.c, pos, rt.withPotentialWaterlogging(pos, state)) {
		return false
	}
	rt.foliage.add(pos)
	return true
}

func (rt *treeRuntime) setDecorationState(pos cube.Pos, state gen.BlockState) bool {
	if !rt.inChunk(pos) {
		return false
	}
	if !rt.g.setBlockStateDirect(rt.c, pos, rt.withPotentialWaterlogging(pos, state)) {
		return false
	}
	rt.decorations.add(pos)
	return true
}

func (rt *treeRuntime) setDecorationBlock(pos cube.Pos, b world.Block) bool {
	if !rt.inChunk(pos) {
		return false
	}
	if !rt.g.setFeatureBlock(rt.c, pos, b) {
		return false
	}
	rt.decorations.add(pos)
	return true
}

func (rt *treeRuntime) placeBelowTrunkBlock(pos cube.Pos) {
	state, ok := rt.g.selectTreeBelowTrunkState(rt.c, pos, rt.cfg, rt.rng, rt.minY, rt.maxY)
	if ok {
		_ = rt.setTrunkState(pos, state)
	}
}

func (g Generator) selectTreeBelowTrunkState(c *chunk.Chunk, pos cube.Pos, cfg gen.TreeConfig, rng *gen.WorldgenRandom, minY, maxY int) (gen.BlockState, bool) {
	if cfg.BelowTrunkProvider.Type != "" {
		return g.selectState(c, cfg.BelowTrunkProvider, pos, rng, minY, maxY)
	}
	if cfg.DirtProvider.Type == "" {
		if g.matchesFeatureBlockTag(g.blockNameAt(c, pos), "cannot_replace_below_tree_trunk") {
			return gen.BlockState{}, false
		}
		return gen.BlockState{Name: "minecraft:dirt"}, true
	}
	if cfg.ForceDirt {
		return g.selectState(c, cfg.DirtProvider, pos, rng, minY, maxY)
	}
	if g.matchesFeatureBlockTag(g.blockNameAt(c, pos), "cannot_replace_below_tree_trunk") {
		return gen.BlockState{}, false
	}
	return g.selectState(c, cfg.DirtProvider, pos, rng, minY, maxY)
}

func (rt treeRuntime) placeRoots(origin, trunkOrigin cube.Pos) bool {
	if rt.cfg.RootPlacer.Type != "mangrove_root_placer" {
		return true
	}
	var raw struct {
		AboveRootPlacement *struct {
			AboveRootPlacementChance float64           `json:"above_root_placement_chance"`
			AboveRootProvider        gen.StateProvider `json:"above_root_provider"`
		} `json:"above_root_placement"`
		MangroveRootPlacement struct {
			CanGrowThrough   string            `json:"can_grow_through"`
			MaxRootLength    int               `json:"max_root_length"`
			MaxRootWidth     int               `json:"max_root_width"`
			MuddyRootsIn     []string          `json:"muddy_roots_in"`
			MuddyRoots       gen.StateProvider `json:"muddy_roots_provider"`
			RandomSkewChance float64           `json:"random_skew_chance"`
		} `json:"mangrove_root_placement"`
		RootProvider gen.StateProvider `json:"root_provider"`
	}
	if err := json.Unmarshal(rt.cfg.RootPlacer.Data, &raw); err != nil {
		return false
	}

	columnPos := origin
	for columnPos[1] < trunkOrigin[1] {
		if !rt.canPlaceRoot(columnPos, raw.MangroveRootPlacement.CanGrowThrough) {
			return false
		}
		columnPos = columnPos.Side(cube.FaceUp)
	}

	rootPositions := []cube.Pos{trunkOrigin.Side(cube.FaceDown)}
	for _, direction := range []treeDirection{treeEast, treeWest, treeSouth, treeNorth} {
		rootPos := trunkOrigin.Add(direction.offset())
		positionsInDirection := make([]cube.Pos, 0, raw.MangroveRootPlacement.MaxRootLength)
		if !rt.simulateMangroveRoots(rootPos, direction, trunkOrigin, positionsInDirection, 0, raw, &positionsInDirection) {
			return false
		}
		rootPositions = append(rootPositions, positionsInDirection...)
		rootPositions = append(rootPositions, trunkOrigin.Add(direction.offset()))
	}

	for _, rootPos := range rootPositions {
		if !rt.placeRoot(rootPos, raw) {
			return false
		}
	}
	return true
}

func (rt treeRuntime) simulateMangroveRoots(pos cube.Pos, prevDir treeDirection, rootOrigin cube.Pos, _ []cube.Pos, layer int, raw struct {
	AboveRootPlacement *struct {
		AboveRootPlacementChance float64           `json:"above_root_placement_chance"`
		AboveRootProvider        gen.StateProvider `json:"above_root_provider"`
	} `json:"above_root_placement"`
	MangroveRootPlacement struct {
		CanGrowThrough   string            `json:"can_grow_through"`
		MaxRootLength    int               `json:"max_root_length"`
		MaxRootWidth     int               `json:"max_root_width"`
		MuddyRootsIn     []string          `json:"muddy_roots_in"`
		MuddyRoots       gen.StateProvider `json:"muddy_roots_provider"`
		RandomSkewChance float64           `json:"random_skew_chance"`
	} `json:"mangrove_root_placement"`
	RootProvider gen.StateProvider `json:"root_provider"`
}, out *[]cube.Pos) bool {
	if layer >= raw.MangroveRootPlacement.MaxRootLength || len(*out) > raw.MangroveRootPlacement.MaxRootLength {
		return false
	}
	for _, candidate := range rt.mangrovePotentialRootPositions(pos, prevDir, rootOrigin, raw.MangroveRootPlacement.MaxRootWidth, raw.MangroveRootPlacement.RandomSkewChance) {
		if !rt.canPlaceRoot(candidate, raw.MangroveRootPlacement.CanGrowThrough) {
			continue
		}
		*out = append(*out, candidate)
		if !rt.simulateMangroveRoots(candidate, prevDir, rootOrigin, nil, layer+1, raw, out) {
			return false
		}
	}
	return true
}

func (rt treeRuntime) mangrovePotentialRootPositions(pos cube.Pos, prevDir treeDirection, rootOrigin cube.Pos, maxRootWidth int, randomSkewChance float64) []cube.Pos {
	below := pos.Side(cube.FaceDown)
	nextTo := pos.Add(prevDir.offset())
	width := abs(pos[0]-rootOrigin[0]) + abs(pos[1]-rootOrigin[1]) + abs(pos[2]-rootOrigin[2])
	if width > maxRootWidth-3 && width <= maxRootWidth {
		if rt.rng.NextDouble() < randomSkewChance {
			return []cube.Pos{below, nextTo.Side(cube.FaceDown)}
		}
		return []cube.Pos{below}
	}
	if width > maxRootWidth || rt.rng.NextDouble() < randomSkewChance {
		return []cube.Pos{below}
	}
	if rt.rng.NextInt(2) == 0 {
		return []cube.Pos{nextTo}
	}
	return []cube.Pos{below}
}

func (rt treeRuntime) canPlaceRoot(pos cube.Pos, canGrowThroughTag string) bool {
	return rt.validTreePos(pos) || (canGrowThroughTag != "" && rt.g.matchesFeatureBlockTag(rt.blockNameAt(pos), canGrowThroughTag))
}

func (rt *treeRuntime) placeRoot(pos cube.Pos, raw struct {
	AboveRootPlacement *struct {
		AboveRootPlacementChance float64           `json:"above_root_placement_chance"`
		AboveRootProvider        gen.StateProvider `json:"above_root_provider"`
	} `json:"above_root_placement"`
	MangroveRootPlacement struct {
		CanGrowThrough   string            `json:"can_grow_through"`
		MaxRootLength    int               `json:"max_root_length"`
		MaxRootWidth     int               `json:"max_root_width"`
		MuddyRootsIn     []string          `json:"muddy_roots_in"`
		MuddyRoots       gen.StateProvider `json:"muddy_roots_provider"`
		RandomSkewChance float64           `json:"random_skew_chance"`
	} `json:"mangrove_root_placement"`
	RootProvider gen.StateProvider `json:"root_provider"`
}) bool {
	if !rt.canPlaceRoot(pos, raw.MangroveRootPlacement.CanGrowThrough) {
		return true
	}

	stateProvider := raw.RootProvider
	blockName := rt.blockNameAt(pos)
	if slices.Contains(raw.MangroveRootPlacement.MuddyRootsIn, "minecraft:"+blockName) || slices.Contains(raw.MangroveRootPlacement.MuddyRootsIn, blockName) {
		stateProvider = raw.MangroveRootPlacement.MuddyRoots
	}
	state, ok := rt.g.selectState(rt.c, stateProvider, pos, rt.rng, rt.minY, rt.maxY)
	if !ok || !rt.setRootState(pos, state) {
		return false
	}

	if raw.AboveRootPlacement != nil && rt.rng.NextDouble() < raw.AboveRootPlacement.AboveRootPlacementChance {
		above := pos.Side(cube.FaceUp)
		if rt.isAir(above) {
			aboveState, ok := rt.g.selectState(rt.c, raw.AboveRootPlacement.AboveRootProvider, above, rt.rng, rt.minY, rt.maxY)
			if ok {
				_ = rt.setRootState(above, aboveState)
			}
		}
	}
	return true
}

type treeStateModifier func(gen.BlockState) gen.BlockState

func (rt *treeRuntime) placeLog(pos cube.Pos, modifier treeStateModifier) bool {
	if !rt.validTreePosForPlacedLog(pos) {
		return false
	}
	state, ok := rt.g.selectState(rt.c, rt.cfg.TrunkProvider, pos, rt.rng, rt.minY, rt.maxY)
	if !ok {
		return false
	}
	if modifier != nil {
		state = modifier(state)
	}
	return rt.setTrunkState(pos, state)
}

func (rt treeRuntime) validTreePosForPlacedLog(pos cube.Pos) bool {
	if rt.cfg.TrunkPlacer.Type == "upwards_branching_trunk_placer" {
		return rt.validTreePosForTrunk(pos)
	}
	return rt.validTreePos(pos)
}

func (rt *treeRuntime) placeLogIfFree(pos cube.Pos, modifier treeStateModifier) {
	if rt.trunkIsFree(pos) {
		_ = rt.placeLog(pos, modifier)
	}
}

func (rt *treeRuntime) placeTrunk(treeHeight int, origin cube.Pos) ([]treeFoliageAttachment, bool) {
	switch rt.cfg.TrunkPlacer.Type {
	case "straight_trunk_placer":
		return rt.placeStraightTrunk(treeHeight, origin), true
	case "forking_trunk_placer":
		return rt.placeForkingTrunk(treeHeight, origin), true
	case "giant_trunk_placer":
		return rt.placeGiantTrunk(treeHeight, origin), true
	case "dark_oak_trunk_placer":
		return rt.placeDarkOakTrunk(treeHeight, origin), true
	case "mega_jungle_trunk_placer":
		return rt.placeMegaJungleTrunk(treeHeight, origin), true
	case "bending_trunk_placer":
		return rt.placeBendingTrunk(treeHeight, origin)
	case "upwards_branching_trunk_placer":
		return rt.placeUpwardsBranchingTrunk(treeHeight, origin)
	case "cherry_trunk_placer":
		return rt.placeCherryTrunk(treeHeight, origin)
	case "fancy_trunk_placer":
		return rt.placeFancyTrunk(treeHeight, origin)
	default:
		return nil, false
	}
}

func (rt *treeRuntime) placeStraightTrunk(treeHeight int, origin cube.Pos) []treeFoliageAttachment {
	rt.placeBelowTrunkBlock(origin.Side(cube.FaceDown))
	for y := 0; y < treeHeight; y++ {
		_ = rt.placeLog(origin.Add(cube.Pos{0, y, 0}), nil)
	}
	return []treeFoliageAttachment{{pos: origin.Add(cube.Pos{0, treeHeight, 0})}}
}

func (rt *treeRuntime) placeForkingTrunk(treeHeight int, origin cube.Pos) []treeFoliageAttachment {
	rt.placeBelowTrunkBlock(origin.Side(cube.FaceDown))
	attachments := make([]treeFoliageAttachment, 0, 2)
	leanDirection := randomTreeDirection(rt.rng)
	leanHeight := treeHeight - int(rt.rng.NextInt(4)) - 1
	leanSteps := 3 - int(rt.rng.NextInt(3))
	tx, tz := origin[0], origin[2]
	endY := -1

	for yo := 0; yo < treeHeight; yo++ {
		yy := origin[1] + yo
		if yo >= leanHeight && leanSteps > 0 {
			tx += leanDirection.offset()[0]
			tz += leanDirection.offset()[2]
			leanSteps--
		}
		if rt.placeLog(cube.Pos{tx, yy, tz}, nil) {
			endY = yy + 1
		}
	}
	if endY >= 0 {
		attachments = append(attachments, treeFoliageAttachment{pos: cube.Pos{tx, endY, tz}, radiusOffset: 1})
	}

	tx, tz = origin[0], origin[2]
	branchDirection := randomTreeDirection(rt.rng)
	if branchDirection != leanDirection {
		branchPos := leanHeight - int(rt.rng.NextInt(2)) - 1
		branchSteps := 1 + int(rt.rng.NextInt(3))
		endY = -1
		for yo := branchPos; yo < treeHeight && branchSteps > 0; branchSteps-- {
			if yo >= 1 {
				yy := origin[1] + yo
				tx += branchDirection.offset()[0]
				tz += branchDirection.offset()[2]
				if rt.placeLog(cube.Pos{tx, yy, tz}, nil) {
					endY = yy + 1
				}
			}
			yo++
		}
		if endY >= 0 {
			attachments = append(attachments, treeFoliageAttachment{pos: cube.Pos{tx, endY, tz}})
		}
	}
	return attachments
}

func (rt *treeRuntime) placeGiantTrunk(treeHeight int, origin cube.Pos) []treeFoliageAttachment {
	below := origin.Side(cube.FaceDown)
	rt.placeBelowTrunkBlock(below)
	rt.placeBelowTrunkBlock(below.Add(cube.Pos{1, 0, 0}))
	rt.placeBelowTrunkBlock(below.Add(cube.Pos{0, 0, 1}))
	rt.placeBelowTrunkBlock(below.Add(cube.Pos{1, 0, 1}))
	for hh := 0; hh < treeHeight; hh++ {
		rt.placeLogIfFree(origin.Add(cube.Pos{0, hh, 0}), nil)
		if hh < treeHeight-1 {
			rt.placeLogIfFree(origin.Add(cube.Pos{1, hh, 0}), nil)
			rt.placeLogIfFree(origin.Add(cube.Pos{1, hh, 1}), nil)
			rt.placeLogIfFree(origin.Add(cube.Pos{0, hh, 1}), nil)
		}
	}
	return []treeFoliageAttachment{{pos: origin.Add(cube.Pos{0, treeHeight, 0}), doubleTrunk: true}}
}

func (rt *treeRuntime) placeDarkOakTrunk(treeHeight int, origin cube.Pos) []treeFoliageAttachment {
	attachments := make([]treeFoliageAttachment, 0, 9)
	below := origin.Side(cube.FaceDown)
	rt.placeBelowTrunkBlock(below)
	rt.placeBelowTrunkBlock(below.Add(cube.Pos{1, 0, 0}))
	rt.placeBelowTrunkBlock(below.Add(cube.Pos{0, 0, 1}))
	rt.placeBelowTrunkBlock(below.Add(cube.Pos{1, 0, 1}))

	leanDirection := randomTreeDirection(rt.rng)
	leanHeight := treeHeight - int(rt.rng.NextInt(4))
	leanSteps := 2 - int(rt.rng.NextInt(3))
	tx, tz := origin[0], origin[2]
	endY := origin[1] + treeHeight - 1

	for dy := 0; dy < treeHeight; dy++ {
		if dy >= leanHeight && leanSteps > 0 {
			tx += leanDirection.offset()[0]
			tz += leanDirection.offset()[2]
			leanSteps--
		}
		yy := origin[1] + dy
		base := cube.Pos{tx, yy, tz}
		if rt.isAirOrLeaves(base) {
			_ = rt.placeLog(base, nil)
			_ = rt.placeLog(base.Add(cube.Pos{1, 0, 0}), nil)
			_ = rt.placeLog(base.Add(cube.Pos{0, 0, 1}), nil)
			_ = rt.placeLog(base.Add(cube.Pos{1, 0, 1}), nil)
		}
	}
	attachments = append(attachments, treeFoliageAttachment{pos: cube.Pos{tx, endY, tz}, doubleTrunk: true})

	for ox := -1; ox <= 2; ox++ {
		for oz := -1; oz <= 2; oz++ {
			if (ox < 0 || ox > 1 || oz < 0 || oz > 1) && rt.rng.NextInt(3) == 0 {
				length := int(rt.rng.NextInt(3)) + 2
				for branchY := 0; branchY < length; branchY++ {
					_ = rt.placeLog(cube.Pos{origin[0] + ox, endY - branchY - 1, origin[2] + oz}, nil)
				}
				attachments = append(attachments, treeFoliageAttachment{pos: cube.Pos{origin[0] + ox, endY, origin[2] + oz}})
			}
		}
	}
	return attachments
}

func (rt *treeRuntime) placeMegaJungleTrunk(treeHeight int, origin cube.Pos) []treeFoliageAttachment {
	attachments := rt.placeGiantTrunk(treeHeight, origin)
	for branchHeight := treeHeight - 2 - int(rt.rng.NextInt(4)); branchHeight > treeHeight/2; branchHeight -= 2 + int(rt.rng.NextInt(4)) {
		angle := rt.rng.NextDouble() * math.Pi * 2
		bx, bz := 0, 0
		for b := 0; b < 5; b++ {
			bx = int(1.5 + math.Cos(angle)*float64(b))
			bz = int(1.5 + math.Sin(angle)*float64(b))
			_ = rt.placeLog(origin.Add(cube.Pos{bx, branchHeight - 3 + b/2, bz}), nil)
		}
		attachments = append(attachments, treeFoliageAttachment{pos: origin.Add(cube.Pos{bx, branchHeight, bz}), radiusOffset: -2})
	}
	return attachments
}

func (rt *treeRuntime) placeBendingTrunk(treeHeight int, origin cube.Pos) ([]treeFoliageAttachment, bool) {
	var raw struct {
		MinHeightForLeaves int             `json:"min_height_for_leaves"`
		BendLength         gen.IntProvider `json:"bend_length"`
	}
	if err := json.Unmarshal(rt.cfg.TrunkPlacer.Data, &raw); err != nil {
		return nil, false
	}

	direction := randomTreeDirection(rt.rng)
	logHeight := treeHeight - 1
	pos := origin
	rt.placeBelowTrunkBlock(pos.Side(cube.FaceDown))
	foliagePoints := make([]treeFoliageAttachment, 0, treeHeight+4)

	for i := 0; i <= logHeight; i++ {
		if i+1 >= logHeight+int(rt.rng.NextInt(2)) {
			pos = pos.Add(direction.offset())
		}
		if rt.validTreePos(pos) {
			_ = rt.placeLog(pos, nil)
		}
		if i >= raw.MinHeightForLeaves {
			foliagePoints = append(foliagePoints, treeFoliageAttachment{pos: pos})
		}
		pos = pos.Side(cube.FaceUp)
	}

	for i, bendLength := 0, rt.g.sampleIntProvider(raw.BendLength, rt.rng); i <= bendLength; i++ {
		if rt.validTreePos(pos) {
			_ = rt.placeLog(pos, nil)
		}
		foliagePoints = append(foliagePoints, treeFoliageAttachment{pos: pos})
		pos = pos.Add(direction.offset())
	}
	return foliagePoints, true
}

func (rt *treeRuntime) placeUpwardsBranchingTrunk(treeHeight int, origin cube.Pos) ([]treeFoliageAttachment, bool) {
	var raw struct {
		ExtraBranchSteps        gen.IntProvider `json:"extra_branch_steps"`
		PlaceBranchPerLogChance float64         `json:"place_branch_per_log_probability"`
		ExtraBranchLength       gen.IntProvider `json:"extra_branch_length"`
		CanGrowThrough          string          `json:"can_grow_through"`
	}
	if err := json.Unmarshal(rt.cfg.TrunkPlacer.Data, &raw); err != nil {
		return nil, false
	}

	attachments := make([]treeFoliageAttachment, 0, treeHeight+8)
	for heightPos := 0; heightPos < treeHeight; heightPos++ {
		currentHeight := origin[1] + heightPos
		logPos := cube.Pos{origin[0], currentHeight, origin[2]}
		if rt.placeLog(logPos, nil) && heightPos < treeHeight-1 && rt.rng.NextDouble() < raw.PlaceBranchPerLogChance {
			branchDir := randomTreeDirection(rt.rng)
			branchLen := rt.g.sampleIntProvider(raw.ExtraBranchLength, rt.rng)
			branchPos := max(0, branchLen-rt.g.sampleIntProvider(raw.ExtraBranchLength, rt.rng)-1)
			branchSteps := rt.g.sampleIntProvider(raw.ExtraBranchSteps, rt.rng)
			rt.placeUpwardsBranch(treeHeight, attachments, logPos, currentHeight, branchDir, branchPos, branchSteps, &attachments)
		}
		if heightPos == treeHeight-1 {
			attachments = append(attachments, treeFoliageAttachment{pos: cube.Pos{origin[0], currentHeight + 1, origin[2]}})
		}
	}
	return attachments, true
}

func (rt *treeRuntime) placeUpwardsBranch(treeHeight int, attachments []treeFoliageAttachment, logPos cube.Pos, currentHeight int, branchDir treeDirection, branchPos, branchSteps int, out *[]treeFoliageAttachment) {
	heightAlongBranch := currentHeight + branchPos
	logX, logZ := logPos[0], logPos[2]
	for branchPlacementIndex := branchPos; branchPlacementIndex < treeHeight && branchSteps > 0; branchPlacementIndex, branchSteps = branchPlacementIndex+1, branchSteps-1 {
		if branchPlacementIndex >= 1 {
			placementHeight := currentHeight + branchPlacementIndex
			logX += branchDir.offset()[0]
			logZ += branchDir.offset()[2]
			heightAlongBranch = placementHeight
			currentPos := cube.Pos{logX, placementHeight, logZ}
			if rt.placeLog(currentPos, nil) {
				heightAlongBranch = placementHeight + 1
			}
			*out = append(*out, treeFoliageAttachment{pos: currentPos})
		}
	}
	if heightAlongBranch-currentHeight > 1 {
		foliagePos := cube.Pos{logX, heightAlongBranch, logZ}
		*out = append(*out, treeFoliageAttachment{pos: foliagePos})
		*out = append(*out, treeFoliageAttachment{pos: foliagePos.Add(cube.Pos{0, -2, 0})})
	}
	_ = attachments
}

func (rt *treeRuntime) placeCherryTrunk(treeHeight int, origin cube.Pos) ([]treeFoliageAttachment, bool) {
	var raw struct {
		BranchCount              gen.IntProvider `json:"branch_count"`
		BranchHorizontalLength   gen.IntProvider `json:"branch_horizontal_length"`
		BranchStartOffsetFromTop gen.IntProvider `json:"branch_start_offset_from_top"`
		BranchEndOffsetFromTop   gen.IntProvider `json:"branch_end_offset_from_top"`
	}
	if err := json.Unmarshal(rt.cfg.TrunkPlacer.Data, &raw); err != nil {
		return nil, false
	}

	startMin := raw.BranchStartOffsetFromTop.MinInclusive
	startMax := raw.BranchStartOffsetFromTop.MaxInclusive
	firstBranchOffset := max(0, treeHeight-1+startMin+int(rt.rng.NextInt(uint32(startMax-startMin+1))))
	secondMax := startMax - 1
	secondBranchOffset := max(0, treeHeight-1+startMin+int(rt.rng.NextInt(uint32(secondMax-startMin+1))))
	if secondBranchOffset >= firstBranchOffset {
		secondBranchOffset++
	}

	branchCount := rt.g.sampleIntProvider(raw.BranchCount, rt.rng)
	hasMiddleBranch := branchCount == 3
	hasBothSideBranches := branchCount >= 2
	trunkHeight := firstBranchOffset + 1
	if hasBothSideBranches {
		trunkHeight = max(firstBranchOffset, secondBranchOffset) + 1
	}
	if hasMiddleBranch {
		trunkHeight = treeHeight
	}

	rt.placeBelowTrunkBlock(origin.Side(cube.FaceDown))
	for y := 0; y < trunkHeight; y++ {
		_ = rt.placeLog(origin.Add(cube.Pos{0, y, 0}), nil)
	}

	attachments := make([]treeFoliageAttachment, 0, 3)
	if hasMiddleBranch {
		attachments = append(attachments, treeFoliageAttachment{pos: origin.Add(cube.Pos{0, trunkHeight, 0})})
	}

	treeDir := randomTreeDirection(rt.rng)
	sidewaysModifier := func(state gen.BlockState) gen.BlockState {
		return setTreeAxis(state, treeDir.axis())
	}

	attachments = append(attachments, rt.generateCherryBranch(treeHeight, origin, raw.BranchHorizontalLength, raw.BranchEndOffsetFromTop, sidewaysModifier, treeDir, firstBranchOffset, firstBranchOffset < trunkHeight-1))
	if hasBothSideBranches {
		attachments = append(attachments, rt.generateCherryBranch(treeHeight, origin, raw.BranchHorizontalLength, raw.BranchEndOffsetFromTop, sidewaysModifier, treeDir.opposite(), secondBranchOffset, secondBranchOffset < trunkHeight-1))
	}
	return attachments, true
}

func (rt *treeRuntime) generateCherryBranch(treeHeight int, origin cube.Pos, branchHorizontalLength, branchEndOffsetFromTop gen.IntProvider, sidewaysModifier treeStateModifier, branchDirection treeDirection, offsetFromOrigin int, middleContinuesUpwards bool) treeFoliageAttachment {
	logPos := origin.Add(cube.Pos{0, offsetFromOrigin, 0})
	branchEndPosOffset := treeHeight - 1 + rt.g.sampleIntProvider(branchEndOffsetFromTop, rt.rng)
	extendBranchAway := middleContinuesUpwards || branchEndPosOffset < offsetFromOrigin
	distanceToTrunk := rt.g.sampleIntProvider(branchHorizontalLength, rt.rng)
	if extendBranchAway {
		distanceToTrunk++
	}
	branchEndPos := origin.Add(cube.Pos{branchDirection.offset()[0] * distanceToTrunk, branchEndPosOffset, branchDirection.offset()[2] * distanceToTrunk})
	stepsHorizontally := 1
	if extendBranchAway {
		stepsHorizontally = 2
	}
	for i := 0; i < stepsHorizontally; i++ {
		logPos = logPos.Add(branchDirection.offset())
		_ = rt.placeLog(logPos, sidewaysModifier)
	}
	verticalStep := 1
	if branchEndPos[1] < logPos[1] {
		verticalStep = -1
	}
	for {
		distance := abs(logPos[0]-branchEndPos[0]) + abs(logPos[1]-branchEndPos[1]) + abs(logPos[2]-branchEndPos[2])
		if distance == 0 {
			return treeFoliageAttachment{pos: branchEndPos.Side(cube.FaceUp)}
		}
		chanceToGrowVertically := float64(abs(branchEndPos[1]-logPos[1])) / float64(distance)
		if rt.rng.NextDouble() < chanceToGrowVertically {
			logPos = logPos.Add(cube.Pos{0, verticalStep, 0})
			_ = rt.placeLog(logPos, nil)
			continue
		}
		logPos = logPos.Add(branchDirection.offset())
		_ = rt.placeLog(logPos, sidewaysModifier)
	}
}

func (rt *treeRuntime) placeFancyTrunk(treeHeight int, origin cube.Pos) ([]treeFoliageAttachment, bool) {
	height := treeHeight + 2
	trunkHeight := int(math.Floor(float64(height) * 0.618))
	rt.placeBelowTrunkBlock(origin.Side(cube.FaceDown))
	clustersPerY := min(1, int(math.Floor(1.382+math.Pow(float64(height)/13.0, 2.0))))
	trunkTop := origin[1] + trunkHeight
	relativeY := height - 5
	foliageCoords := []fancyFoliageCoords{{attachment: treeFoliageAttachment{pos: origin.Add(cube.Pos{0, relativeY, 0})}, branchBase: trunkTop}}

	for ; relativeY >= 0; relativeY-- {
		treeShape := fancyTreeShape(height, relativeY)
		if treeShape < 0 {
			continue
		}
		for i := 0; i < clustersPerY; i++ {
			radius := treeShape * (rt.rng.NextDouble() + 0.328)
			angle := rt.rng.NextDouble() * 2 * math.Pi
			x := radius*math.Sin(angle) + 0.5
			z := radius*math.Cos(angle) + 0.5
			checkStart := origin.Add(cube.Pos{int(math.Floor(x)), relativeY - 1, int(math.Floor(z))})
			checkEnd := checkStart.Add(cube.Pos{0, 5, 0})
			if rt.makeFancyLimb(checkStart, checkEnd, false, nil) {
				dx := origin[0] - checkStart[0]
				dz := origin[2] - checkStart[2]
				branchHeight := float64(checkStart[1]) - math.Sqrt(float64(dx*dx+dz*dz))*0.381
				branchTop := trunkTop
				if int(branchHeight) < branchTop {
					branchTop = int(branchHeight)
				}
				checkBranchBase := cube.Pos{origin[0], branchTop, origin[2]}
				if rt.makeFancyLimb(checkBranchBase, checkStart, false, nil) {
					foliageCoords = append(foliageCoords, fancyFoliageCoords{attachment: treeFoliageAttachment{pos: checkStart}, branchBase: checkBranchBase[1]})
				}
			}
		}
	}

	_ = rt.makeFancyLimb(origin, origin.Add(cube.Pos{0, trunkHeight, 0}), true, func(state gen.BlockState) gen.BlockState {
		return setTreeAxis(state, rt.fancyLogAxis(origin, origin.Add(cube.Pos{0, trunkHeight, 0})))
	})
	rt.makeFancyBranches(height, origin, foliageCoords)

	attachments := make([]treeFoliageAttachment, 0, len(foliageCoords))
	for _, foliageCoord := range foliageCoords {
		if rt.trimFancyBranch(height, foliageCoord.branchBase-origin[1]) {
			attachments = append(attachments, foliageCoord.attachment)
		}
	}
	return attachments, true
}

type fancyFoliageCoords struct {
	attachment treeFoliageAttachment
	branchBase int
}

func (rt *treeRuntime) makeFancyLimb(startPos, endPos cube.Pos, doPlace bool, modifier treeStateModifier) bool {
	if !doPlace && startPos == endPos {
		return true
	}
	delta := cube.Pos{endPos[0] - startPos[0], endPos[1] - startPos[1], endPos[2] - startPos[2]}
	steps := max(abs(delta[0]), max(abs(delta[1]), abs(delta[2])))
	dx := float64(delta[0]) / float64(steps)
	dy := float64(delta[1]) / float64(steps)
	dz := float64(delta[2]) / float64(steps)
	for i := 0; i <= steps; i++ {
		blockPos := cube.Pos{
			startPos[0] + int(math.Floor(0.5+float64(i)*dx)),
			startPos[1] + int(math.Floor(0.5+float64(i)*dy)),
			startPos[2] + int(math.Floor(0.5+float64(i)*dz)),
		}
		if doPlace {
			axis := rt.fancyLogAxis(startPos, blockPos)
			_ = rt.placeLog(blockPos, func(state gen.BlockState) gen.BlockState {
				state = setTreeAxis(state, axis)
				if modifier != nil {
					state = modifier(state)
				}
				return state
			})
			continue
		}
		if !rt.trunkIsFree(blockPos) {
			return false
		}
	}
	return true
}

func (rt treeRuntime) fancyLogAxis(startPos, blockPos cube.Pos) string {
	xdiff := abs(blockPos[0] - startPos[0])
	zdiff := abs(blockPos[2] - startPos[2])
	if max(xdiff, zdiff) <= 0 {
		return "y"
	}
	if xdiff == max(xdiff, zdiff) {
		return "x"
	}
	return "z"
}

func (rt treeRuntime) trimFancyBranch(height, localY int) bool {
	return float64(localY) >= float64(height)*0.2
}

func (rt *treeRuntime) makeFancyBranches(height int, origin cube.Pos, foliageCoords []fancyFoliageCoords) {
	for _, endCoord := range foliageCoords {
		baseCoord := cube.Pos{origin[0], endCoord.branchBase, origin[2]}
		if baseCoord != endCoord.attachment.pos && rt.trimFancyBranch(height, endCoord.branchBase-origin[1]) {
			_ = rt.makeFancyLimb(baseCoord, endCoord.attachment.pos, true, nil)
		}
	}
}

func fancyTreeShape(height, y int) float64 {
	if float64(y) < float64(height)*0.3 {
		return -1
	}
	radius := float64(height) / 2.0
	adjacent := radius - float64(y)
	if math.Abs(adjacent) >= radius {
		return 0
	}
	distance := math.Sqrt(radius*radius - adjacent*adjacent)
	if adjacent == 0 {
		distance = radius
	}
	return distance * 0.5
}

func setTreeAxis(state gen.BlockState, axis string) gen.BlockState {
	if state.Properties == nil {
		state.Properties = make(map[string]string, 1)
	} else {
		props := make(map[string]string, len(state.Properties)+1)
		for key, value := range state.Properties {
			props[key] = value
		}
		state.Properties = props
	}
	state.Properties["axis"] = axis
	return state
}

func (rt treeRuntime) isAirOrLeaves(pos cube.Pos) bool {
	if !rt.inChunk(pos) {
		return false
	}
	name := rt.blockNameAt(pos)
	return name == "air" || strings.HasSuffix(name, "_leaves")
}

func (rt *treeRuntime) placeFoliage(treeHeight int, attachment treeFoliageAttachment, foliageHeight, leafRadius int) {
	switch rt.cfg.FoliagePlacer.Type {
	case "blob_foliage_placer":
		var raw struct {
			Offset gen.IntProvider `json:"offset"`
		}
		if json.Unmarshal(rt.cfg.FoliagePlacer.Data, &raw) == nil {
			offset := rt.g.sampleIntProvider(raw.Offset, rt.rng)
			for yo := offset; yo >= offset-foliageHeight; yo-- {
				currentRadius := max(leafRadius+attachment.radiusOffset-1-yo/2, 0)
				rt.placeLeavesRow(attachment.pos, currentRadius, yo, attachment.doubleTrunk, blobFoliageSkip, nil)
			}
		}
	case "fancy_foliage_placer":
		var raw struct {
			Offset gen.IntProvider `json:"offset"`
		}
		if json.Unmarshal(rt.cfg.FoliagePlacer.Data, &raw) == nil {
			offset := rt.g.sampleIntProvider(raw.Offset, rt.rng)
			for yo := offset; yo >= offset-foliageHeight; yo-- {
				currentRadius := leafRadius
				if yo != offset && yo != offset-foliageHeight {
					currentRadius++
				}
				rt.placeLeavesRow(attachment.pos, currentRadius, yo, attachment.doubleTrunk, fancyFoliageSkip, nil)
			}
		}
	case "bush_foliage_placer":
		var raw struct {
			Offset gen.IntProvider `json:"offset"`
		}
		if json.Unmarshal(rt.cfg.FoliagePlacer.Data, &raw) == nil {
			offset := rt.g.sampleIntProvider(raw.Offset, rt.rng)
			for yo := offset; yo >= offset-foliageHeight; yo-- {
				currentRadius := leafRadius + attachment.radiusOffset - 1 - yo
				rt.placeLeavesRow(attachment.pos, currentRadius, yo, attachment.doubleTrunk, bushFoliageSkip, nil)
			}
		}
	case "spruce_foliage_placer":
		var raw struct {
			Offset gen.IntProvider `json:"offset"`
		}
		if json.Unmarshal(rt.cfg.FoliagePlacer.Data, &raw) == nil {
			offset := rt.g.sampleIntProvider(raw.Offset, rt.rng)
			currentRadius := int(rt.rng.NextInt(2))
			maxRadius := 1
			minRadius := 0
			for yo := offset; yo >= -foliageHeight; yo-- {
				rt.placeLeavesRow(attachment.pos, currentRadius, yo, attachment.doubleTrunk, coniferFoliageSkip, nil)
				if currentRadius >= maxRadius {
					currentRadius = minRadius
					minRadius = 1
					maxRadius = min(maxRadius+1, leafRadius+attachment.radiusOffset)
				} else {
					currentRadius++
				}
			}
		}
	case "pine_foliage_placer":
		var raw struct {
			Offset gen.IntProvider `json:"offset"`
		}
		if json.Unmarshal(rt.cfg.FoliagePlacer.Data, &raw) == nil {
			offset := rt.g.sampleIntProvider(raw.Offset, rt.rng)
			currentRadius := 0
			for yo := offset; yo >= offset-foliageHeight; yo-- {
				rt.placeLeavesRow(attachment.pos, currentRadius, yo, attachment.doubleTrunk, coniferFoliageSkip, nil)
				if currentRadius >= 1 && yo == offset-foliageHeight+1 {
					currentRadius--
				} else if currentRadius < leafRadius+attachment.radiusOffset {
					currentRadius++
				}
			}
		}
	case "acacia_foliage_placer":
		var raw struct {
			Offset gen.IntProvider `json:"offset"`
		}
		if json.Unmarshal(rt.cfg.FoliagePlacer.Data, &raw) == nil {
			foliagePos := attachment.pos.Add(cube.Pos{0, rt.g.sampleIntProvider(raw.Offset, rt.rng), 0})
			rt.placeLeavesRow(foliagePos, leafRadius+attachment.radiusOffset, -1-foliageHeight, attachment.doubleTrunk, acaciaFoliageSkip, nil)
			rt.placeLeavesRow(foliagePos, leafRadius-1, -foliageHeight, attachment.doubleTrunk, acaciaFoliageSkip, nil)
			rt.placeLeavesRow(foliagePos, leafRadius+attachment.radiusOffset-1, 0, attachment.doubleTrunk, acaciaFoliageSkip, nil)
		}
	case "dark_oak_foliage_placer":
		var raw struct {
			Offset gen.IntProvider `json:"offset"`
		}
		if json.Unmarshal(rt.cfg.FoliagePlacer.Data, &raw) == nil {
			pos := attachment.pos.Add(cube.Pos{0, rt.g.sampleIntProvider(raw.Offset, rt.rng), 0})
			if attachment.doubleTrunk {
				rt.placeLeavesRow(pos, leafRadius+2, -1, true, darkOakFoliageSkip, darkOakSignedSkip)
				rt.placeLeavesRow(pos, leafRadius+3, 0, true, darkOakFoliageSkip, darkOakSignedSkip)
				rt.placeLeavesRow(pos, leafRadius+2, 1, true, darkOakFoliageSkip, darkOakSignedSkip)
				if rt.rng.NextInt(2) == 0 {
					rt.placeLeavesRow(pos, leafRadius, 2, true, darkOakFoliageSkip, darkOakSignedSkip)
				}
			} else {
				rt.placeLeavesRow(pos, leafRadius+2, -1, false, darkOakFoliageSkip, nil)
				rt.placeLeavesRow(pos, leafRadius+1, 0, false, darkOakFoliageSkip, nil)
			}
		}
	case "random_spread_foliage_placer":
		origin := attachment.pos
		for i := 0; i < foliageHeight; i++ {
			_ = i
		}
		var raw struct {
			LeafPlacementAttempts int `json:"leaf_placement_attempts"`
		}
		if json.Unmarshal(rt.cfg.FoliagePlacer.Data, &raw) == nil {
			for i := 0; i < raw.LeafPlacementAttempts; i++ {
				candidate := origin.Add(cube.Pos{
					int(rt.rng.NextInt(uint32(max(leafRadius, 1)))) - int(rt.rng.NextInt(uint32(max(leafRadius, 1)))),
					int(rt.rng.NextInt(uint32(max(foliageHeight, 1)))) - int(rt.rng.NextInt(uint32(max(foliageHeight, 1)))),
					int(rt.rng.NextInt(uint32(max(leafRadius, 1)))) - int(rt.rng.NextInt(uint32(max(leafRadius, 1)))),
				})
				_ = rt.tryPlaceLeaf(candidate)
			}
		}
	case "cherry_foliage_placer":
		var raw struct {
			Offset                       gen.IntProvider `json:"offset"`
			WideBottomLayerHoleChance    float64         `json:"wide_bottom_layer_hole_chance"`
			CornerHoleChance             float64         `json:"corner_hole_chance"`
			HangingLeavesChance          float64         `json:"hanging_leaves_chance"`
			HangingLeavesExtensionChance float64         `json:"hanging_leaves_extension_chance"`
		}
		if json.Unmarshal(rt.cfg.FoliagePlacer.Data, &raw) == nil {
			foliagePos := attachment.pos.Add(cube.Pos{0, rt.g.sampleIntProvider(raw.Offset, rt.rng), 0})
			currentRadius := leafRadius + attachment.radiusOffset - 1
			skip := cherryFoliageSkip(raw.WideBottomLayerHoleChance, raw.CornerHoleChance)
			rt.placeLeavesRow(foliagePos, currentRadius-2, foliageHeight-3, attachment.doubleTrunk, skip, nil)
			rt.placeLeavesRow(foliagePos, currentRadius-1, foliageHeight-4, attachment.doubleTrunk, skip, nil)
			for y := foliageHeight - 5; y >= 0; y-- {
				rt.placeLeavesRow(foliagePos, currentRadius, y, attachment.doubleTrunk, skip, nil)
			}
			rt.placeLeavesRowWithHangingLeavesBelow(foliagePos, currentRadius, -1, attachment.doubleTrunk, skip, raw.HangingLeavesChance, raw.HangingLeavesExtensionChance)
			rt.placeLeavesRowWithHangingLeavesBelow(foliagePos, currentRadius-1, -2, attachment.doubleTrunk, skip, raw.HangingLeavesChance, raw.HangingLeavesExtensionChance)
		}
	case "jungle_foliage_placer":
		var raw struct {
			Offset gen.IntProvider `json:"offset"`
		}
		if json.Unmarshal(rt.cfg.FoliagePlacer.Data, &raw) == nil {
			leafHeight := 1 + int(rt.rng.NextInt(2))
			if attachment.doubleTrunk {
				leafHeight = foliageHeight
			}
			offset := rt.g.sampleIntProvider(raw.Offset, rt.rng)
			for yo := offset; yo >= offset-leafHeight; yo-- {
				currentRadius := leafRadius + attachment.radiusOffset + 1 - yo
				rt.placeLeavesRow(attachment.pos, currentRadius, yo, attachment.doubleTrunk, megaFoliageSkip, nil)
			}
		}
	case "mega_pine_foliage_placer":
		var raw struct {
			Offset gen.IntProvider `json:"offset"`
		}
		if json.Unmarshal(rt.cfg.FoliagePlacer.Data, &raw) == nil {
			offset := rt.g.sampleIntProvider(raw.Offset, rt.rng)
			prevRadius := 0
			for yy := attachment.pos[1] - foliageHeight + offset; yy <= attachment.pos[1]+offset; yy++ {
				yo := attachment.pos[1] - yy
				smoothRadius := leafRadius + attachment.radiusOffset + int(math.Floor(float64(yo)/float64(max(foliageHeight, 1))*3.5))
				jaggedRadius := smoothRadius
				if yo > 0 && smoothRadius == prevRadius && (yy&1) == 0 {
					jaggedRadius++
				}
				rt.placeLeavesRow(cube.Pos{attachment.pos[0], yy, attachment.pos[2]}, jaggedRadius, 0, attachment.doubleTrunk, megaFoliageSkip, nil)
				prevRadius = smoothRadius
			}
		}
	}
	_ = treeHeight
}

func (rt *treeRuntime) placeLeavesRow(center cube.Pos, currentRadius, y int, doubleTrunk bool, skip, signedSkip treeFoliageSkip) {
	if currentRadius < 0 {
		return
	}
	offset := 0
	if doubleTrunk {
		offset = 1
	}
	for dx := -currentRadius; dx <= currentRadius+offset; dx++ {
		for dz := -currentRadius; dz <= currentRadius+offset; dz++ {
			if signedSkip != nil && signedSkip(rt.rng, dx, y, dz, currentRadius, doubleTrunk) {
				continue
			}
			minDx, minDz := abs(dx), abs(dz)
			if doubleTrunk {
				minDx = min(abs(dx), abs(dx-1))
				minDz = min(abs(dz), abs(dz-1))
			}
			if skip != nil && skip(rt.rng, minDx, y, minDz, currentRadius, doubleTrunk) {
				continue
			}
			_ = rt.tryPlaceLeaf(center.Add(cube.Pos{dx, y, dz}))
		}
	}
}

func (rt *treeRuntime) placeLeavesRowWithHangingLeavesBelow(center cube.Pos, currentRadius, y int, doubleTrunk bool, skip treeFoliageSkip, hangingChance, extensionChance float64) {
	rt.placeLeavesRow(center, currentRadius, y, doubleTrunk, skip, nil)
	offset := 0
	if doubleTrunk {
		offset = 1
	}
	logPos := center.Side(cube.FaceDown)
	for _, alongEdge := range []treeDirection{treeEast, treeWest, treeSouth, treeNorth} {
		toEdge := treeSouth
		switch alongEdge {
		case treeEast:
			toEdge = treeSouth
		case treeWest:
			toEdge = treeNorth
		case treeSouth:
			toEdge = treeWest
		case treeNorth:
			toEdge = treeEast
		}
		offsetToEdge := currentRadius
		if toEdge == treeEast || toEdge == treeSouth {
			offsetToEdge = currentRadius + offset
		}
		pos := center.Add(cube.Pos{0, y - 1, 0}).Add(mulPos(toEdge.offset(), offsetToEdge)).Add(mulPos(alongEdge.offset(), -currentRadius))
		for offsetAlongEdge := -currentRadius; offsetAlongEdge < currentRadius+offset; offsetAlongEdge++ {
			above := pos.Side(cube.FaceUp)
			if rt.foliage.contains(above) {
				if rt.tryPlaceHangingLeaf(pos, hangingChance, logPos) {
					down := pos.Side(cube.FaceDown)
					_ = rt.tryPlaceHangingLeaf(down, extensionChance, logPos)
				}
			}
			pos = pos.Add(alongEdge.offset())
		}
	}
}

func (rt *treeRuntime) tryPlaceHangingLeaf(pos cube.Pos, chance float64, logPos cube.Pos) bool {
	if manhattanDistance(pos, logPos) >= 7 || rt.rng.NextDouble() > chance {
		return false
	}
	return rt.tryPlaceLeaf(pos)
}

func (rt *treeRuntime) tryPlaceLeaf(pos cube.Pos) bool {
	if rt.persistentLeaves(pos) || !rt.validTreePos(pos) {
		return false
	}
	state, ok := rt.g.selectState(rt.c, rt.cfg.FoliageProvider, pos, rt.rng, rt.minY, rt.maxY)
	if !ok {
		return false
	}
	return rt.setFoliageState(pos, state)
}

func (rt *treeRuntime) applyDecorators() {
	for _, decorator := range rt.cfg.Decorators {
		switch decorator.Type {
		case "beehive":
			var cfg struct {
				Probability float64 `json:"probability"`
			}
			if json.Unmarshal(decorator.Data, &cfg) == nil {
				rt.placeBeehiveDecorator(cfg.Probability)
			}
		case "place_on_ground":
			var cfg struct {
				BlockStateProvider gen.StateProvider `json:"block_state_provider"`
				Height             int               `json:"height"`
				Radius             int               `json:"radius"`
				Tries              int               `json:"tries"`
			}
			if json.Unmarshal(decorator.Data, &cfg) == nil {
				rt.placeOnGroundDecorator(cfg.BlockStateProvider, max(1, cfg.Tries), max(0, cfg.Radius), max(0, cfg.Height))
			}
		case "leave_vine":
			var cfg struct {
				Probability float64 `json:"probability"`
			}
			if json.Unmarshal(decorator.Data, &cfg) == nil {
				rt.placeLeafVineDecorator(cfg.Probability)
			}
		case "trunk_vine":
			rt.placeTrunkVineDecorator()
		case "attached_to_logs":
			var cfg struct {
				Probability   float64           `json:"probability"`
				BlockProvider gen.StateProvider `json:"block_provider"`
				Directions    []string          `json:"directions"`
			}
			if json.Unmarshal(decorator.Data, &cfg) == nil {
				rt.placeAttachedToLogsDecorator(cfg.Probability, cfg.BlockProvider, cfg.Directions)
			}
		case "attached_to_leaves":
			var cfg struct {
				Probability         float64           `json:"probability"`
				ExclusionRadiusXZ   int               `json:"exclusion_radius_xz"`
				ExclusionRadiusY    int               `json:"exclusion_radius_y"`
				BlockProvider       gen.StateProvider `json:"block_provider"`
				RequiredEmptyBlocks int               `json:"required_empty_blocks"`
				Directions          []string          `json:"directions"`
			}
			if json.Unmarshal(decorator.Data, &cfg) == nil {
				rt.placeAttachedToLeavesDecorator(cfg.Probability, cfg.ExclusionRadiusXZ, cfg.ExclusionRadiusY, cfg.BlockProvider, cfg.RequiredEmptyBlocks, cfg.Directions)
			}
		case "alter_ground":
			var cfg struct {
				Provider gen.StateProvider `json:"provider"`
			}
			if json.Unmarshal(decorator.Data, &cfg) == nil {
				rt.placeAlterGroundDecorator(cfg.Provider)
			}
		case "pale_moss":
			var cfg struct {
				LeavesProbability float64 `json:"leaves_probability"`
				TrunkProbability  float64 `json:"trunk_probability"`
				GroundProbability float64 `json:"ground_probability"`
			}
			if json.Unmarshal(decorator.Data, &cfg) == nil {
				rt.placePaleMossDecorator(cfg.LeavesProbability, cfg.TrunkProbability, cfg.GroundProbability)
			}
		case "creaking_heart":
			var cfg struct {
				Probability float64 `json:"probability"`
			}
			if json.Unmarshal(decorator.Data, &cfg) == nil {
				rt.placeCreakingHeartDecorator(cfg.Probability)
			}
		}
	}
}

func (rt treeRuntime) sortedLogs() []cube.Pos   { return rt.trunks.sorted() }
func (rt treeRuntime) sortedLeaves() []cube.Pos { return rt.foliage.sorted() }
func (rt treeRuntime) sortedRoots() []cube.Pos  { return rt.roots.sorted() }

func (rt treeRuntime) lowestTrunkOrRootOfTree() []cube.Pos {
	roots := rt.sortedRoots()
	logs := rt.sortedLogs()
	if len(roots) == 0 {
		return logs
	}
	if len(logs) != 0 && roots[0][1] == logs[0][1] {
		out := make([]cube.Pos, 0, len(logs)+len(roots))
		out = append(out, logs...)
		out = append(out, roots...)
		return out
	}
	return roots
}

func (rt *treeRuntime) placeBeehiveDecorator(probability float64) {
	logs := rt.sortedLogs()
	// Vanilla BeehiveDecorator.place: the log emptiness check happens before
	// the probability roll, which uses nextFloat (a single RNG draw). The
	// nextInt(3) hive height roll only happens when there are no leaves.
	if len(logs) == 0 || float64(rt.rng.NextFloat()) >= probability {
		return
	}
	leaves := rt.sortedLeaves()
	var hiveY int
	if len(leaves) != 0 {
		hiveY = max(leaves[0][1]-1, logs[0][1]+1)
	} else {
		hiveY = min(logs[0][1]+1+int(rt.rng.NextInt(3)), logs[len(logs)-1][1])
	}
	spawnDirections := []treeDirection{treeEast, treeWest, treeSouth}
	hivePlacements := make([]cube.Pos, 0, len(logs)*len(spawnDirections))
	for _, pos := range logs {
		if pos[1] != hiveY {
			continue
		}
		for _, direction := range spawnDirections {
			hivePlacements = append(hivePlacements, pos.Add(direction.offset()))
		}
	}
	treeShuffle(rt.rng, hivePlacements)
	for _, pos := range hivePlacements {
		if rt.isAir(pos) && rt.isAir(pos.Add(treeSouth.offset())) {
			beeNest, ok := world.BlockByName("minecraft:bee_nest", map[string]any{"direction": int32(3), "honey_level": int32(0)})
			if ok {
				_ = rt.setDecorationBlock(pos, beeNest)
			}
			// Vanilla stores 2+nextInt(2) bee occupants, each consuming a
			// nextInt(599) roll for ticksInHive.
			numBees := 2 + int(rt.rng.NextInt(2))
			for i := 0; i < numBees; i++ {
				_ = rt.rng.NextInt(599)
			}
			return
		}
	}
}

func (rt *treeRuntime) placeOnGroundDecorator(provider gen.StateProvider, tries, radius, height int) {
	positions := rt.lowestTrunkOrRootOfTree()
	if len(positions) == 0 {
		return
	}
	origin := positions[0]
	minY := origin[1]
	minX, maxX := origin[0], origin[0]
	minZ, maxZ := origin[2], origin[2]
	for _, pos := range positions {
		if pos[1] != minY {
			continue
		}
		minX = min(minX, pos[0])
		maxX = max(maxX, pos[0])
		minZ = min(minZ, pos[2])
		maxZ = max(maxZ, pos[2])
	}
	for i := 0; i < tries; i++ {
		pos := cube.Pos{
			treeRandomBetweenInclusive(rt.rng, minX-radius, maxX+radius),
			treeRandomBetweenInclusive(rt.rng, minY-height, minY+height),
			treeRandomBetweenInclusive(rt.rng, minZ-radius, maxZ+radius),
		}
		rt.attemptPlaceGroundBlock(pos, provider)
	}
}

func (rt *treeRuntime) attemptPlaceGroundBlock(pos cube.Pos, provider gen.StateProvider) {
	above := pos.Side(cube.FaceUp)
	if !(rt.isAir(above) || rt.blockNameAt(above) == "vine") {
		return
	}
	if !rt.solidRender(pos) {
		return
	}
	if rt.g.heightmapPlacementY(rt.c, above[0]&15, above[2]&15, "MOTION_BLOCKING_NO_LEAVES", rt.minY, rt.maxY) > above[1] {
		return
	}
	state, ok := rt.g.selectState(rt.c, provider, above, rt.rng, rt.minY, rt.maxY)
	if ok {
		_ = rt.setDecorationState(above, state)
	}
}

func (rt treeRuntime) solidRender(pos cube.Pos) bool {
	if !rt.inChunk(pos) {
		return false
	}
	rid := rt.layerRuntimeID(pos, 0)
	if rid == rt.g.airRID {
		return false
	}
	return rt.g.isSolidRID(rid)
}

func (rt *treeRuntime) placeLeafVineDecorator(probability float64) {
	for _, pos := range rt.sortedLeaves() {
		if rt.rng.NextDouble() < probability {
			west := pos.Add(treeWest.offset())
			if rt.isAir(west) {
				rt.addHangingVine(west, cube.East)
			}
		}
		if rt.rng.NextDouble() < probability {
			east := pos.Add(treeEast.offset())
			if rt.isAir(east) {
				rt.addHangingVine(east, cube.West)
			}
		}
		if rt.rng.NextDouble() < probability {
			north := pos.Add(treeNorth.offset())
			if rt.isAir(north) {
				rt.addHangingVine(north, cube.South)
			}
		}
		if rt.rng.NextDouble() < probability {
			south := pos.Add(treeSouth.offset())
			if rt.isAir(south) {
				rt.addHangingVine(south, cube.North)
			}
		}
	}
}

func (rt *treeRuntime) addHangingVine(pos cube.Pos, direction cube.Direction) {
	rt.placeVine(pos, direction)
	maxDir := 4
	for current := pos.Side(cube.FaceDown); rt.isAir(current) && maxDir > 0; maxDir-- {
		rt.placeVine(current, direction)
		current = current.Side(cube.FaceDown)
	}
}

func (rt *treeRuntime) placeTrunkVineDecorator() {
	for _, pos := range rt.sortedLogs() {
		if rt.rng.NextInt(3) > 0 {
			west := pos.Add(treeWest.offset())
			if rt.isAir(west) {
				rt.placeVine(west, cube.East)
			}
		}
		if rt.rng.NextInt(3) > 0 {
			east := pos.Add(treeEast.offset())
			if rt.isAir(east) {
				rt.placeVine(east, cube.West)
			}
		}
		if rt.rng.NextInt(3) > 0 {
			north := pos.Add(treeNorth.offset())
			if rt.isAir(north) {
				rt.placeVine(north, cube.South)
			}
		}
		if rt.rng.NextInt(3) > 0 {
			south := pos.Add(treeSouth.offset())
			if rt.isAir(south) {
				rt.placeVine(south, cube.North)
			}
		}
	}
}

func (rt *treeRuntime) placeVine(pos cube.Pos, direction cube.Direction) {
	vine := block.Vines{}.WithAttachment(direction, true)
	_ = rt.setDecorationBlock(pos, vine)
}

func (rt *treeRuntime) placeAttachedToLogsDecorator(probability float64, provider gen.StateProvider, directions []string) {
	logs := rt.sortedLogs()
	treeShuffle(rt.rng, logs)
	for _, logPos := range logs {
		if len(directions) == 0 {
			return
		}
		direction := directions[int(rt.rng.NextInt(uint32(len(directions))))]
		placementPos := logPos.Add(blockColumnDirection(direction))
		if rt.rng.NextDouble() <= probability && rt.isAir(placementPos) {
			state, ok := rt.g.selectState(rt.c, provider, placementPos, rt.rng, rt.minY, rt.maxY)
			if ok {
				_ = rt.setDecorationState(placementPos, state)
			}
		}
	}
}

func (rt *treeRuntime) placeAttachedToLeavesDecorator(probability float64, exclusionRadiusXZ, exclusionRadiusY int, provider gen.StateProvider, requiredEmptyBlocks int, directions []string) {
	blacklist := make(map[cube.Pos]struct{})
	leaves := rt.sortedLeaves()
	treeShuffle(rt.rng, leaves)
	for _, leafPos := range leaves {
		if len(directions) == 0 {
			return
		}
		direction := directions[int(rt.rng.NextInt(uint32(len(directions))))]
		offset := blockColumnDirection(direction)
		placementPos := leafPos.Add(offset)
		if _, blocked := blacklist[placementPos]; blocked || rt.rng.NextDouble() >= probability || !rt.hasRequiredEmptyBlocks(leafPos, offset, requiredEmptyBlocks) {
			continue
		}
		for x := placementPos[0] - exclusionRadiusXZ; x <= placementPos[0]+exclusionRadiusXZ; x++ {
			for y := placementPos[1] - exclusionRadiusY; y <= placementPos[1]+exclusionRadiusY; y++ {
				for z := placementPos[2] - exclusionRadiusXZ; z <= placementPos[2]+exclusionRadiusXZ; z++ {
					blacklist[cube.Pos{x, y, z}] = struct{}{}
				}
			}
		}
		state, ok := rt.g.selectState(rt.c, provider, placementPos, rt.rng, rt.minY, rt.maxY)
		if ok {
			_ = rt.setDecorationState(placementPos, state)
		}
	}
}

func (rt treeRuntime) hasRequiredEmptyBlocks(leafPos, direction cube.Pos, requiredEmptyBlocks int) bool {
	for i := 1; i <= requiredEmptyBlocks; i++ {
		if !rt.isAir(leafPos.Add(mulPos(direction, i))) {
			return false
		}
	}
	return true
}

func (rt *treeRuntime) placeAlterGroundDecorator(provider gen.StateProvider) {
	positions := rt.lowestTrunkOrRootOfTree()
	if len(positions) == 0 {
		return
	}
	minY := positions[0][1]
	for _, pos := range positions {
		if pos[1] != minY {
			continue
		}
		rt.placeAlterGroundCircle(pos.Add(cube.Pos{-1, 0, -1}), provider)
		rt.placeAlterGroundCircle(pos.Add(cube.Pos{2, 0, -1}), provider)
		rt.placeAlterGroundCircle(pos.Add(cube.Pos{-1, 0, 2}), provider)
		rt.placeAlterGroundCircle(pos.Add(cube.Pos{2, 0, 2}), provider)
		for i := 0; i < 5; i++ {
			placement := int(rt.rng.NextInt(64))
			xx := placement % 8
			zz := placement / 8
			if xx == 0 || xx == 7 || zz == 0 || zz == 7 {
				rt.placeAlterGroundCircle(pos.Add(cube.Pos{-3 + xx, 0, -3 + zz}), provider)
			}
		}
	}
}

func (rt *treeRuntime) placeAlterGroundCircle(pos cube.Pos, provider gen.StateProvider) {
	for xx := -2; xx <= 2; xx++ {
		for zz := -2; zz <= 2; zz++ {
			if abs(xx) != 2 || abs(zz) != 2 {
				rt.placeAlterGroundBlock(pos.Add(cube.Pos{xx, 0, zz}), provider)
			}
		}
	}
}

func (rt *treeRuntime) placeAlterGroundBlock(pos cube.Pos, provider gen.StateProvider) {
	for dy := 2; dy >= -3; dy-- {
		cursor := pos.Add(cube.Pos{0, dy, 0})
		state, ok := rt.g.selectState(rt.c, provider, cursor, rt.rng, rt.minY, rt.maxY)
		if ok {
			_ = rt.setDecorationState(cursor, state)
			return
		}
		if !rt.isAir(cursor) && dy < 0 {
			return
		}
	}
}

func (rt *treeRuntime) placePaleMossDecorator(leavesProbability, trunkProbability, groundProbability float64) {
	logs := rt.sortedLogs()
	if len(logs) == 0 {
		return
	}
	origin := logs[0]
	if rt.rng.NextDouble() < groundProbability {
		_ = rt.g.executeConfiguredFeature(rt.c, rt.biomes, origin.Side(cube.FaceUp), gen.ConfiguredFeatureRef{Name: "pale_moss_patch"}, "pale_moss_patch", rt.chunkX, rt.chunkZ, rt.minY, rt.maxY, rt.rng, 0)
	}
	for _, pos := range logs {
		if rt.rng.NextDouble() < trunkProbability {
			down := pos.Side(cube.FaceDown)
			if rt.isAir(down) {
				rt.addPaleMossHanger(down)
			}
		}
	}
	for _, pos := range rt.sortedLeaves() {
		if rt.rng.NextDouble() < leavesProbability {
			down := pos.Side(cube.FaceDown)
			if rt.isAir(down) {
				rt.addPaleMossHanger(down)
			}
		}
	}
}

func (rt *treeRuntime) addPaleMossHanger(pos cube.Pos) {
	for rt.isAir(pos.Side(cube.FaceDown)) && rt.rng.NextDouble() >= 0.5 {
		_ = rt.setDecorationState(pos, gen.BlockState{Name: "minecraft:pale_hanging_moss", Properties: map[string]string{"tip": "false"}})
		pos = pos.Side(cube.FaceDown)
	}
	_ = rt.setDecorationState(pos, gen.BlockState{Name: "minecraft:pale_hanging_moss", Properties: map[string]string{"tip": "true"}})
}

func (rt *treeRuntime) placeCreakingHeartDecorator(probability float64) {
	logs := rt.sortedLogs()
	if len(logs) == 0 || rt.rng.NextDouble() >= probability {
		return
	}
	treeShuffle(rt.rng, logs)
	for _, pos := range logs {
		enclosed := true
		for _, neighbor := range []cube.Pos{{1, 0, 0}, {-1, 0, 0}, {0, 1, 0}, {0, -1, 0}, {0, 0, 1}, {0, 0, -1}} {
			name := rt.blockNameAt(pos.Add(neighbor))
			if !strings.HasSuffix(name, "_log") && !strings.HasSuffix(name, "_wood") && !strings.HasSuffix(name, "_stem") {
				enclosed = false
				break
			}
		}
		if enclosed {
			axis := "y"
			if b, ok := world.BlockByRuntimeID(rt.layerRuntimeID(pos, 0)); ok {
				if log, ok := b.(block.Log); ok {
					axis = log.Axis.String()
				}
			}
			_ = rt.setDecorationState(pos, gen.BlockState{Name: "minecraft:creaking_heart", Properties: map[string]string{"axis": axis, "natural": "true", "state": "dormant"}})
			return
		}
	}
}

func (rt *treeRuntime) updateLeaves() {
	minBound, maxBound, ok := rt.treeUpdateBounds()
	if !ok {
		return
	}

	connected := make(map[cube.Pos]struct{}, len(rt.trunks.set)+len(rt.foliage.set))
	visited := make(map[cube.Pos]struct{}, len(rt.trunks.set)+len(rt.foliage.set))
	frontier := make([]cube.Pos, 0, len(rt.trunks.set))
	for _, pos := range rt.sortedLogs() {
		if !rt.treeBoundsContain(pos, minBound, maxBound) {
			continue
		}
		frontier = append(frontier, pos)
		visited[pos] = struct{}{}
	}

	for distance := 0; distance < 7 && len(frontier) > 0; distance++ {
		next := make([]cube.Pos, 0, len(frontier)*6)
		for _, pos := range frontier {
			if distance != 0 {
				connected[pos] = struct{}{}
			}
			for _, neighbor := range []cube.Pos{
				pos.Side(cube.FaceDown),
				pos.Side(cube.FaceUp),
				pos.Side(cube.FaceNorth),
				pos.Side(cube.FaceSouth),
				pos.Side(cube.FaceWest),
				pos.Side(cube.FaceEast),
			} {
				if !rt.treeBoundsContain(neighbor, minBound, maxBound) {
					continue
				}
				if _, seen := visited[neighbor]; seen {
					continue
				}
				if _, ok := rt.leafBlockAt(neighbor); !ok {
					continue
				}
				visited[neighbor] = struct{}{}
				next = append(next, neighbor)
			}
		}
		frontier = next
	}

	for y := minBound[1]; y <= maxBound[1]; y++ {
		for x := minBound[0]; x <= maxBound[0]; x++ {
			for z := minBound[2]; z <= maxBound[2]; z++ {
				pos := cube.Pos{x, y, z}
				leaves, ok := rt.leafBlockAt(pos)
				if !ok || leaves.Persistent {
					continue
				}
				_, shouldStay := connected[pos]
				leaves.ShouldUpdate = !shouldStay
				rt.setExistingBlock(pos, leaves)
			}
		}
	}
}

func (rt treeRuntime) treeUpdateBounds() (cube.Pos, cube.Pos, bool) {
	positions := make([]cube.Pos, 0, len(rt.roots.set)+len(rt.trunks.set)+len(rt.foliage.set)+len(rt.decorations.set))
	for pos := range rt.roots.set {
		positions = append(positions, pos)
	}
	for pos := range rt.trunks.set {
		positions = append(positions, pos)
	}
	for pos := range rt.foliage.set {
		positions = append(positions, pos)
	}
	for pos := range rt.decorations.set {
		positions = append(positions, pos)
	}
	if len(positions) == 0 {
		return cube.Pos{}, cube.Pos{}, false
	}

	minBound := positions[0]
	maxBound := positions[0]
	for _, pos := range positions[1:] {
		minBound = cube.Pos{
			min(minBound[0], pos[0]),
			min(minBound[1], pos[1]),
			min(minBound[2], pos[2]),
		}
		maxBound = cube.Pos{
			max(maxBound[0], pos[0]),
			max(maxBound[1], pos[1]),
			max(maxBound[2], pos[2]),
		}
	}
	return minBound, maxBound, true
}

func (rt treeRuntime) treeBoundsContain(pos, minBound, maxBound cube.Pos) bool {
	return pos[0] >= minBound[0] && pos[0] <= maxBound[0] &&
		pos[1] >= minBound[1] && pos[1] <= maxBound[1] &&
		pos[2] >= minBound[2] && pos[2] <= maxBound[2]
}

func (rt treeRuntime) leafBlockAt(pos cube.Pos) (block.Leaves, bool) {
	chunkAtPos, ok := rt.chunkForPos(pos)
	if !ok {
		return block.Leaves{}, false
	}
	b, ok := world.BlockByRuntimeID(chunkAtPos.Block(uint8(pos[0]&15), int16(pos[1]), uint8(pos[2]&15), 0))
	if !ok {
		return block.Leaves{}, false
	}
	leaves, ok := b.(block.Leaves)
	return leaves, ok
}

func (rt treeRuntime) setExistingBlock(pos cube.Pos, b world.Block) {
	if _, ok := rt.chunkForPos(pos); !ok {
		return
	}
	_ = rt.g.setFeatureBlock(rt.c, pos, b)
}

func (rt treeRuntime) chunkForPos(pos cube.Pos) (*chunk.Chunk, bool) {
	if rt.g.activeTreeRegion != nil {
		chunkAtPos, ok := rt.g.activeTreeRegion.chunkAtPos(pos)
		return chunkAtPos, ok
	}
	if !rt.g.positionInChunk(pos, rt.chunkX, rt.chunkZ, rt.minY, rt.maxY) {
		return nil, false
	}
	return rt.c, true
}

// sturdyBelow mirrors FallenTreeFeature.isOverSolidGround: the block below
// must have a sturdy up face (approximated by solid render).
func (rt treeRuntime) sturdyBelow(pos cube.Pos) bool {
	return rt.solidRender(pos.Side(cube.FaceDown))
}

// decorateFallenLogs mirrors FallenTreeFeature.decorateLogs: a decorator
// context whose log list follows java HashSet iteration order stable-sorted
// by Y, applied to each decorator in order.
func (rt *treeRuntime) decorateFallenLogs(logs []cube.Pos, decorators []gen.FeatureDecorator) {
	if len(decorators) == 0 || len(logs) == 0 {
		return
	}
	ordered := javaHashSetOrder(logs)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i][1] < ordered[j][1] })
	for _, decorator := range decorators {
		if decorator.Type != "attached_to_logs" {
			continue
		}
		var cfg struct {
			Probability   float64           `json:"probability"`
			BlockProvider gen.StateProvider `json:"block_provider"`
			Directions    []string          `json:"directions"`
		}
		if json.Unmarshal(decorator.Data, &cfg) != nil {
			continue
		}
		rt.placeAttachedToLogsAt(ordered, cfg.Probability, cfg.BlockProvider, cfg.Directions)
	}
}

// placeAttachedToLogsAt mirrors AttachedToLogsDecorator.place over an
// explicit log list.
func (rt *treeRuntime) placeAttachedToLogsAt(logs []cube.Pos, probability float64, provider gen.StateProvider, directions []string) {
	if len(directions) == 0 {
		return
	}
	shuffled := make([]cube.Pos, len(logs))
	copy(shuffled, logs)
	treeShuffle(rt.rng, shuffled)
	for _, logPos := range shuffled {
		// Util.getRandom always consumes one bounded draw, even for a
		// single-element direction list.
		direction := directions[int(rt.rng.NextInt(uint32(len(directions))))]
		placement := logPos.Add(treeDecoratorDirectionOffset(direction))
		if float64(rt.rng.NextFloat()) <= probability && rt.isAir(placement) {
			state, ok := rt.g.selectState(rt.c, provider, placement, rt.rng, rt.minY, rt.maxY)
			if ok {
				_ = rt.setDecorationState(placement, state)
			}
		}
	}
}

func treeDecoratorDirectionOffset(direction string) cube.Pos {
	switch direction {
	case "up":
		return cube.Pos{0, 1, 0}
	case "down":
		return cube.Pos{0, -1, 0}
	case "north":
		return cube.Pos{0, 0, -1}
	case "south":
		return cube.Pos{0, 0, 1}
	case "west":
		return cube.Pos{-1, 0, 0}
	case "east":
		return cube.Pos{1, 0, 0}
	}
	return cube.Pos{}
}

// javaHashSetOrder reproduces java.util.HashSet<BlockPos> iteration order for
// the given insertion sequence: buckets scanned in table order, entries
// within a bucket in insertion order, with HashMap's power-of-two resize
// (threshold 0.75) preserving relative order on split. Duplicate positions
// keep their first slot.
func javaHashSetOrder(positions []cube.Pos) []cube.Pos {
	capacity := 16
	threshold := 12
	buckets := make([][]cube.Pos, capacity)
	hashes := make(map[cube.Pos]uint32, len(positions))
	size := 0
	spread := func(pos cube.Pos) uint32 {
		h := uint32((pos[1]+pos[2]*31)*31 + pos[0])
		return h ^ (h >> 16)
	}
insert:
	for _, pos := range positions {
		h := spread(pos)
		idx := h & uint32(capacity-1)
		for _, existing := range buckets[idx] {
			if existing == pos {
				continue insert
			}
		}
		buckets[idx] = append(buckets[idx], pos)
		hashes[pos] = h
		size++
		if size > threshold {
			newCapacity := capacity * 2
			newBuckets := make([][]cube.Pos, newCapacity)
			for i := 0; i < capacity; i++ {
				for _, entry := range buckets[i] {
					nidx := hashes[entry] & uint32(newCapacity-1)
					newBuckets[nidx] = append(newBuckets[nidx], entry)
				}
			}
			capacity, buckets = newCapacity, newBuckets
			threshold *= 2
		}
	}
	out := make([]cube.Pos, 0, size)
	for _, bucket := range buckets {
		out = append(out, bucket...)
	}
	return out
}

func treeShuffle[T any](rng *gen.WorldgenRandom, values []T) {
	for i := len(values) - 1; i > 0; i-- {
		j := int(rng.NextInt(uint32(i + 1)))
		values[i], values[j] = values[j], values[i]
	}
}

func treeRandomBetweenInclusive(rng *gen.WorldgenRandom, minV, maxV int) int {
	if maxV <= minV {
		return minV
	}
	return minV + int(rng.NextInt(uint32(maxV-minV+1)))
}

func mulPos(pos cube.Pos, n int) cube.Pos {
	return cube.Pos{pos[0] * n, pos[1] * n, pos[2] * n}
}

func manhattanDistance(a, b cube.Pos) int {
	return abs(a[0]-b[0]) + abs(a[1]-b[1]) + abs(a[2]-b[2])
}
