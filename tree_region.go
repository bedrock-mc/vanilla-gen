package vanilla

import (
	"math"
	"sync"

	gen "github.com/bedrock-mc/vanilla-gen/gen"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/df-mc/dragonfly/server/world/chunk"
)

type featureMarginCache struct {
	mu     sync.RWMutex
	byName map[string]int
}

func newFeatureMarginCache() *featureMarginCache {
	return &featureMarginCache{byName: make(map[string]int)}
}

func (c *featureMarginCache) Lookup(name string) (int, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	margin, ok := c.byName[name]
	return margin, ok
}

func (c *featureMarginCache) Store(name string, margin int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.byName[name] = margin
}

type treeDecorationRegion struct {
	g            Generator
	centerChunkX int
	centerChunkZ int
	minY         int
	maxY         int
	centerChunk  *chunk.Chunk
	slots        map[[2]int]treeDecorationRegionSlot
}

type treeDecorationRegionSlot struct {
	chunk  *chunk.Chunk
	biomes sourceBiomeVolume
}

func newTreeDecorationRegion(g Generator, c *chunk.Chunk, biomes sourceBiomeVolume, chunkX, chunkZ, minY, maxY int) *treeDecorationRegion {
	region := &treeDecorationRegion{
		g:            g,
		centerChunkX: chunkX,
		centerChunkZ: chunkZ,
		minY:         minY,
		maxY:         maxY,
		centerChunk:  c,
		slots:        make(map[[2]int]treeDecorationRegionSlot, 9),
	}
	region.set(chunkX, chunkZ, c, biomes)
	return region
}

func (r *treeDecorationRegion) set(chunkX, chunkZ int, c *chunk.Chunk, biomes sourceBiomeVolume) {
	r.slots[[2]int{chunkX, chunkZ}] = treeDecorationRegionSlot{chunk: c, biomes: biomes}
}

func (r *treeDecorationRegion) slot(chunkX, chunkZ int) (treeDecorationRegionSlot, bool) {
	slot, ok := r.slots[[2]int{chunkX, chunkZ}]
	return slot, ok
}

func (r *treeDecorationRegion) contains(pos cube.Pos) bool {
	chunkX := floorDiv(pos[0], 16)
	chunkZ := floorDiv(pos[2], 16)
	return abs(chunkX-r.centerChunkX) <= 1 &&
		abs(chunkZ-r.centerChunkZ) <= 1 &&
		pos[1] >= r.minY &&
		pos[1] <= r.maxY
}

func (r *treeDecorationRegion) ensureSlot(chunkX, chunkZ int) (treeDecorationRegionSlot, bool) {
	if abs(chunkX-r.centerChunkX) > 1 || abs(chunkZ-r.centerChunkZ) > 1 {
		return treeDecorationRegionSlot{}, false
	}
	if slot, ok := r.slot(chunkX, chunkZ); ok {
		return slot, true
	}
	neighbor := chunk.New(r.g.blockRegistry, cube.Range{r.minY, r.maxY})
	neighborBiomes, _, _, _, _ := r.g.prepareChunkForDecoration(world.ChunkPos{int32(chunkX), int32(chunkZ)}, neighbor)
	slot := treeDecorationRegionSlot{chunk: neighbor, biomes: neighborBiomes}
	r.set(chunkX, chunkZ, neighbor, neighborBiomes)
	return slot, true
}

func (r *treeDecorationRegion) chunkAtPos(pos cube.Pos) (*chunk.Chunk, bool) {
	chunkX := floorDiv(pos[0], 16)
	chunkZ := floorDiv(pos[2], 16)
	if chunkX == r.centerChunkX && chunkZ == r.centerChunkZ {
		if r.centerChunk == nil {
			if slot, ok := r.slot(chunkX, chunkZ); ok {
				r.centerChunk = slot.chunk
			}
		}
		return r.centerChunk, true
	}
	slot, ok := r.ensureSlot(chunkX, chunkZ)
	if !ok {
		return nil, false
	}
	return slot.chunk, true
}

func (g Generator) chunkForActiveTreePos(c *chunk.Chunk, pos cube.Pos) *chunk.Chunk {
	if g.activeTreeRegion == nil {
		return c
	}
	if floorDiv(pos[0], 16) == g.activeTreeRegion.centerChunkX && floorDiv(pos[2], 16) == g.activeTreeRegion.centerChunkZ {
		if g.activeTreeRegion.centerChunk == nil {
			if slot, ok := g.activeTreeRegion.slot(g.activeTreeRegion.centerChunkX, g.activeTreeRegion.centerChunkZ); ok {
				g.activeTreeRegion.centerChunk = slot.chunk
			}
		}
		if g.activeTreeRegion.centerChunk == nil || c == g.activeTreeRegion.centerChunk {
			return c
		}
		return g.activeTreeRegion.centerChunk
	}
	if regionChunk, ok := g.activeTreeRegion.chunkAtPos(pos); ok {
		return regionChunk
	}
	return c
}

func (g Generator) runPlacedFeatureAcrossRegion(region *treeDecorationRegion, step gen.GenerationStep, featureName string, featureIndex int) {
	if g.placedFeatureNeedsReplayAcrossRegion(featureName) {
		g.runPlacedTreeFeatureAcrossRegion(region, step, featureName, featureIndex)
		return
	}
	g.runPlacedFeatureWithRegion(region, step, featureName, featureIndex)
}

func (g Generator) runPlacedFeatureWithRegion(region *treeDecorationRegion, step gen.GenerationStep, featureName string, featureIndex int) {
	centerSlot, ok := region.ensureSlot(region.centerChunkX, region.centerChunkZ)
	if !ok {
		return
	}
	if region.centerChunk == nil {
		region.centerChunk = centerSlot.chunk
	}
	placed, err := g.features.Placed(featureName)
	if err != nil {
		return
	}

	decorationSeed := g.decorationSeed(region.centerChunkX, region.centerChunkZ)
	origin := cube.Pos{region.centerChunkX * 16, region.minY, region.centerChunkZ * 16}
	rng := g.featureRNG(decorationSeed, featureIndex, step)
	regionMargin := g.decorationMarginForPlacedFeatureName(featureName)
	g.placeWithModifiers(centerSlot.chunk, centerSlot.biomes, origin, placed.Placement, featureName, region.centerChunkX, region.centerChunkZ, region.minY, region.maxY, rng, func(pos cube.Pos) {
		executor := g
		if regionMargin > 0 && needsDecorationRegionForPos(pos, region.centerChunkX, region.centerChunkZ, regionMargin) {
			executor.activeTreeRegion = region
		}
		executor.executeConfiguredFeature(centerSlot.chunk, centerSlot.biomes, pos, placed.Feature, featureName, region.centerChunkX, region.centerChunkZ, region.minY, region.maxY, rng, 0)
	})
}

func (g Generator) runPlacedTreeFeatureAcrossRegion(region *treeDecorationRegion, step gen.GenerationStep, featureName string, featureIndex int) {
	placed, err := g.features.Placed(featureName)
	if err != nil {
		return
	}

	regionGenerator := g
	regionGenerator.activeTreeRegion = region
	for sourceChunkZ := region.centerChunkZ - 1; sourceChunkZ <= region.centerChunkZ+1; sourceChunkZ++ {
		for sourceChunkX := region.centerChunkX - 1; sourceChunkX <= region.centerChunkX+1; sourceChunkX++ {
			slot, ok := region.ensureSlot(sourceChunkX, sourceChunkZ)
			if !ok {
				continue
			}

			origin := cube.Pos{sourceChunkX * 16, region.minY, sourceChunkZ * 16}
			rng := regionGenerator.featureRNG(regionGenerator.decorationSeed(sourceChunkX, sourceChunkZ), featureIndex, step)
			regionGenerator.placeWithModifiers(
				slot.chunk,
				slot.biomes,
				origin,
				placed.Placement,
				featureName,
				sourceChunkX,
				sourceChunkZ,
				region.minY,
				region.maxY,
				rng,
				func(pos cube.Pos) {
					regionGenerator.executeConfiguredFeature(
						slot.chunk,
						slot.biomes,
						pos,
						placed.Feature,
						featureName,
						sourceChunkX,
						sourceChunkZ,
						region.minY,
						region.maxY,
						rng,
						0,
					)
				},
			)
		}
	}
}

// placedFeatureNeedsReplayAcrossRegion reports whether neighbor-chunk origins
// of this feature can write into the center chunk (vanilla features write
// into the whole WorldGenRegion); such features are re-simulated from every
// chunk in the 3x3 region.
func (g Generator) placedFeatureNeedsReplayAcrossRegion(featureName string) bool {
	return g.decorationMarginForPlacedFeatureName(featureName) > 0
}

func (g Generator) placedFeatureRefMayPlaceTrees(ref gen.PlacedFeatureRef, seenPlaced, seenConfigured map[string]struct{}) bool {
	if ref.Name != "" {
		if _, ok := seenPlaced[ref.Name]; ok {
			return false
		}
		seenPlaced[ref.Name] = struct{}{}
	}

	placed, err := g.features.ResolvePlaced(ref)
	if err != nil {
		return false
	}
	return g.configuredFeatureRefMayPlaceTrees(placed.Feature, seenPlaced, seenConfigured)
}

func (g Generator) configuredFeatureRefMayPlaceTrees(ref gen.ConfiguredFeatureRef, seenPlaced, seenConfigured map[string]struct{}) bool {
	if ref.Name != "" {
		if _, ok := seenConfigured[ref.Name]; ok {
			return false
		}
		seenConfigured[ref.Name] = struct{}{}
	}

	feature, err := g.features.ResolveConfigured(ref)
	if err != nil {
		return false
	}

	switch feature.Type {
	case "tree":
		return true
	case "random_selector":
		cfg, err := feature.RandomSelector()
		if err != nil {
			return false
		}
		for _, entry := range cfg.Features {
			if g.placedFeatureRefMayPlaceTrees(entry.Feature, seenPlaced, seenConfigured) {
				return true
			}
		}
		return g.placedFeatureRefMayPlaceTrees(cfg.Default, seenPlaced, seenConfigured)
	case "simple_random_selector":
		cfg, err := feature.SimpleRandomSelector()
		if err != nil {
			return false
		}
		for _, entry := range cfg.Features {
			if g.placedFeatureRefMayPlaceTrees(entry, seenPlaced, seenConfigured) {
				return true
			}
		}
		return false
	case "random_boolean_selector":
		cfg, err := feature.RandomBooleanSelector()
		if err != nil {
			return false
		}
		return g.placedFeatureRefMayPlaceTrees(cfg.FeatureTrue, seenPlaced, seenConfigured) ||
			g.placedFeatureRefMayPlaceTrees(cfg.FeatureFalse, seenPlaced, seenConfigured)
	case "random_patch", "flower":
		var (
			cfg gen.RandomPatchConfig
			err error
		)
		if feature.Type == "flower" {
			cfg, err = feature.Flower()
		} else {
			cfg, err = feature.RandomPatch()
		}
		if err != nil {
			return false
		}
		return g.placedFeatureRefMayPlaceTrees(cfg.Feature, seenPlaced, seenConfigured)
	case "vegetation_patch", "waterlogged_vegetation_patch":
		var (
			cfg gen.VegetationPatchConfig
			err error
		)
		if feature.Type == "waterlogged_vegetation_patch" {
			cfg, err = feature.WaterloggedVegetationPatch()
		} else {
			cfg, err = feature.VegetationPatch()
		}
		if err != nil {
			return false
		}
		return g.placedFeatureRefMayPlaceTrees(cfg.VegetationFeature, seenPlaced, seenConfigured)
	case "root_system":
		cfg, err := feature.RootSystem()
		if err != nil {
			return false
		}
		return g.placedFeatureRefMayPlaceTrees(cfg.Feature, seenPlaced, seenConfigured)
	default:
		return false
	}
}

func (g Generator) placedFeatureNeedsDecorationRegion(featureName string) bool {
	return g.placedFeatureRefNeedsDecorationRegion(gen.PlacedFeatureRef{Name: featureName}, map[string]struct{}{}, map[string]struct{}{})
}

func (g Generator) decorationMarginForPlacedFeatureName(featureName string) int {
	if g.featureRegionCache != nil {
		if margin, ok := g.featureRegionCache.Lookup(featureName); ok {
			return margin
		}
	}
	margin := g.placedFeatureDecorationMargin(gen.PlacedFeatureRef{Name: featureName}, map[string]struct{}{}, map[string]struct{}{})
	if g.featureRegionCache != nil {
		g.featureRegionCache.Store(featureName, margin)
	}
	return margin
}

func (g Generator) configuredFeatureDecorationMargin(ref gen.ConfiguredFeatureRef, seenPlaced, seenConfigured map[string]struct{}) int {
	if ref.Name != "" {
		if _, ok := seenConfigured[ref.Name]; ok {
			return 0
		}
		seenConfigured[ref.Name] = struct{}{}
	}

	feature, err := g.features.ResolveConfigured(ref)
	if err != nil {
		return 0
	}

	switch feature.Type {
	case "tree":
		return 16
	case "geode":
		cfg, err := feature.Geode()
		if err != nil {
			return 0
		}
		return max(
			max(abs(cfg.MinGenOffset), abs(cfg.MaxGenOffset)+1),
			max(abs(cfg.OuterWallDistance.MinInclusive), abs(cfg.OuterWallDistance.MaxInclusive)),
			cfg.DistributionPoints.MaxInclusive*2+1,
		)
	case "fossil":
		cfg, err := feature.Fossil()
		if err != nil {
			return 0
		}
		maxWidth := 0
		for _, name := range cfg.FossilStructures {
			if g.structureTemplates == nil {
				continue
			}
			template, err := g.structureTemplates.Template(name)
			if err != nil {
				continue
			}
			maxWidth = max(maxWidth, max(template.Size[0], template.Size[2]))
		}
		return (maxWidth + 1) / 2
	case "iceberg":
		return 11
	case "monster_room":
		return 4
	case "ore":
		cfg, err := feature.Ore()
		if err != nil {
			return 0
		}
		spread := int(math.Ceil(float64(cfg.Size) / 8.0))
		maxRadius := int(math.Ceil((float64(cfg.Size)/16.0*2.0 + 1.0) / 2.0))
		return spread + maxRadius
	case "scattered_ore":
		return 7
	case "disk":
		cfg, err := feature.Disk()
		if err != nil {
			return 0
		}
		upper := cfg.Radius.MaxInclusive
		if cfg.Radius.Constant != nil && *cfg.Radius.Constant > upper {
			upper = *cfg.Radius.Constant
		}
		return max(upper, 1)
	case "desert_well":
		return 2
	case "blue_ice":
		return 2
	case "random_selector":
		cfg, err := feature.RandomSelector()
		if err != nil {
			return 0
		}
		margin := g.placedFeatureDecorationMargin(cfg.Default, seenPlaced, seenConfigured)
		for _, entry := range cfg.Features {
			margin = max(margin, g.placedFeatureDecorationMargin(entry.Feature, seenPlaced, seenConfigured))
		}
		return margin
	case "simple_random_selector":
		cfg, err := feature.SimpleRandomSelector()
		if err != nil {
			return 0
		}
		margin := 0
		for _, entry := range cfg.Features {
			margin = max(margin, g.placedFeatureDecorationMargin(entry, seenPlaced, seenConfigured))
		}
		return margin
	case "random_boolean_selector":
		cfg, err := feature.RandomBooleanSelector()
		if err != nil {
			return 0
		}
		return max(
			g.placedFeatureDecorationMargin(cfg.FeatureTrue, seenPlaced, seenConfigured),
			g.placedFeatureDecorationMargin(cfg.FeatureFalse, seenPlaced, seenConfigured),
		)
	case "random_patch", "flower":
		var (
			cfg gen.RandomPatchConfig
			err error
		)
		if feature.Type == "flower" {
			cfg, err = feature.Flower()
		} else {
			cfg, err = feature.RandomPatch()
		}
		if err != nil {
			return 0
		}
		return g.placedFeatureDecorationMargin(cfg.Feature, seenPlaced, seenConfigured)
	case "vegetation_patch", "waterlogged_vegetation_patch":
		var (
			cfg gen.VegetationPatchConfig
			err error
		)
		if feature.Type == "waterlogged_vegetation_patch" {
			cfg, err = feature.WaterloggedVegetationPatch()
		} else {
			cfg, err = feature.VegetationPatch()
		}
		if err != nil {
			return 0
		}
		return g.placedFeatureDecorationMargin(cfg.VegetationFeature, seenPlaced, seenConfigured)
	case "root_system":
		cfg, err := feature.RootSystem()
		if err != nil {
			return 0
		}
		return g.placedFeatureDecorationMargin(cfg.Feature, seenPlaced, seenConfigured)
	default:
		return 0
	}
}

func (g Generator) placedFeatureDecorationMargin(ref gen.PlacedFeatureRef, seenPlaced, seenConfigured map[string]struct{}) int {
	if ref.Name != "" {
		if _, ok := seenPlaced[ref.Name]; ok {
			return 0
		}
		seenPlaced[ref.Name] = struct{}{}
	}

	placed, err := g.features.ResolvePlaced(ref)
	if err != nil {
		return 0
	}
	return g.configuredFeatureDecorationMargin(placed.Feature, seenPlaced, seenConfigured)
}

func needsDecorationRegionForPos(pos cube.Pos, chunkX, chunkZ, margin int) bool {
	posChunkX := floorDiv(pos[0], 16)
	posChunkZ := floorDiv(pos[2], 16)
	if posChunkX != chunkX || posChunkZ != chunkZ {
		return true
	}
	if margin <= 0 {
		return false
	}
	localX := pos[0] - chunkX*16
	localZ := pos[2] - chunkZ*16
	return localX < margin || localX >= 16-margin || localZ < margin || localZ >= 16-margin
}

func (g Generator) placedFeatureRefNeedsDecorationRegion(ref gen.PlacedFeatureRef, seenPlaced, seenConfigured map[string]struct{}) bool {
	if ref.Name != "" {
		if _, ok := seenPlaced[ref.Name]; ok {
			return false
		}
		seenPlaced[ref.Name] = struct{}{}
	}

	placed, err := g.features.ResolvePlaced(ref)
	if err != nil {
		return false
	}
	return g.configuredFeatureRefNeedsDecorationRegion(placed.Feature, seenPlaced, seenConfigured)
}

func (g Generator) configuredFeatureRefNeedsDecorationRegion(ref gen.ConfiguredFeatureRef, seenPlaced, seenConfigured map[string]struct{}) bool {
	if ref.Name != "" {
		if _, ok := seenConfigured[ref.Name]; ok {
			return false
		}
		seenConfigured[ref.Name] = struct{}{}
	}

	feature, err := g.features.ResolveConfigured(ref)
	if err != nil {
		return false
	}

	switch feature.Type {
	case "tree", "geode", "fossil", "monster_room", "desert_well", "iceberg", "blue_ice", "ore", "scattered_ore", "disk":
		return true
	case "random_selector":
		cfg, err := feature.RandomSelector()
		if err != nil {
			return false
		}
		for _, entry := range cfg.Features {
			if g.placedFeatureRefNeedsDecorationRegion(entry.Feature, seenPlaced, seenConfigured) {
				return true
			}
		}
		return g.placedFeatureRefNeedsDecorationRegion(cfg.Default, seenPlaced, seenConfigured)
	case "simple_random_selector":
		cfg, err := feature.SimpleRandomSelector()
		if err != nil {
			return false
		}
		for _, entry := range cfg.Features {
			if g.placedFeatureRefNeedsDecorationRegion(entry, seenPlaced, seenConfigured) {
				return true
			}
		}
		return false
	case "random_boolean_selector":
		cfg, err := feature.RandomBooleanSelector()
		if err != nil {
			return false
		}
		return g.placedFeatureRefNeedsDecorationRegion(cfg.FeatureTrue, seenPlaced, seenConfigured) ||
			g.placedFeatureRefNeedsDecorationRegion(cfg.FeatureFalse, seenPlaced, seenConfigured)
	case "random_patch", "flower":
		var (
			cfg gen.RandomPatchConfig
			err error
		)
		if feature.Type == "flower" {
			cfg, err = feature.Flower()
		} else {
			cfg, err = feature.RandomPatch()
		}
		if err != nil {
			return false
		}
		return g.placedFeatureRefNeedsDecorationRegion(cfg.Feature, seenPlaced, seenConfigured)
	case "vegetation_patch", "waterlogged_vegetation_patch":
		var (
			cfg gen.VegetationPatchConfig
			err error
		)
		if feature.Type == "waterlogged_vegetation_patch" {
			cfg, err = feature.WaterloggedVegetationPatch()
		} else {
			cfg, err = feature.VegetationPatch()
		}
		if err != nil {
			return false
		}
		return g.placedFeatureRefNeedsDecorationRegion(cfg.VegetationFeature, seenPlaced, seenConfigured)
	case "root_system":
		cfg, err := feature.RootSystem()
		if err != nil {
			return false
		}
		return g.placedFeatureRefNeedsDecorationRegion(cfg.Feature, seenPlaced, seenConfigured)
	default:
		return false
	}
}
