package vanilla

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"

	gen "github.com/bedrock-mc/vanilla-gen/gen"
	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/df-mc/dragonfly/server/world/chunk"
)

func (g Generator) decorateFeatures(c *chunk.Chunk, biomes sourceBiomeVolume, chunkX, chunkZ, minY, maxY int) {
	if g.features == nil || g.biomeGeneration == nil {
		return
	}

	possibleBiomes := g.collectPossibleFeatureBiomes(chunkX, chunkZ, minY, maxY)
	if len(possibleBiomes) == 0 {
		return
	}

	decorationSeed := g.decorationSeed(chunkX, chunkZ)
	regionFeatureCache := make(map[string]bool)
	var treeRegion *treeDecorationRegion
	for stepIndex := 0; stepIndex < featureStepCount; stepIndex++ {
		step := gen.GenerationStep(stepIndex)
		for _, featureIndex := range g.biomeGeneration.featureIndexesForStep(possibleBiomes, step) {
			featureName := g.biomeGeneration.stepFeatures[stepIndex].features[featureIndex]
			needsRegion, ok := regionFeatureCache[featureName]
			if !ok {
				needsRegion = g.placedFeatureNeedsDecorationRegion(featureName)
				regionFeatureCache[featureName] = needsRegion
			}
			if needsRegion {
				if treeRegion == nil {
					treeRegion = newTreeDecorationRegion(g, c, biomes, chunkX, chunkZ, minY, maxY)
				}
				g.runPlacedFeatureAcrossRegion(treeRegion, step, featureName, featureIndex)
				continue
			}
			g.runPlacedFeature(c, biomes, chunkX, chunkZ, minY, maxY, step, featureName, featureIndex, decorationSeed)
		}
	}
}

func (g Generator) decorateFeaturesAndStructures(c *chunk.Chunk, biomes sourceBiomeVolume, chunkX, chunkZ, minY, maxY int) {
	var (
		possibleBiomes []gen.Biome
		decorationSeed int64
		treeRegion     *treeDecorationRegion
	)
	if g.features != nil && g.biomeGeneration != nil {
		possibleBiomes = g.collectPossibleFeatureBiomes(chunkX, chunkZ, minY, maxY)
		if len(possibleBiomes) > 0 {
			decorationSeed = g.decorationSeed(chunkX, chunkZ)
		}
	}
	regionFeatureCache := make(map[string]bool)

	var surfaceSampler *structureHeightSampler
	if g.structureTemplates != nil && g.structureStarts != nil && len(g.structurePlanners) > 0 {
		surfaceSampler = newStructureHeightSampler(g, minY, maxY)
	}

	for stepIndex := 0; stepIndex < featureStepCount; stepIndex++ {
		step := gen.GenerationStep(stepIndex)
		if surfaceSampler != nil {
			g.placeStructuresForStep(c, biomes, chunkX, chunkZ, minY, maxY, step, surfaceSampler)
		}
		if len(possibleBiomes) == 0 {
			continue
		}
		for _, featureIndex := range g.biomeGeneration.featureIndexesForStep(possibleBiomes, step) {
			featureName := g.biomeGeneration.stepFeatures[stepIndex].features[featureIndex]
			needsRegion, ok := regionFeatureCache[featureName]
			if !ok {
				needsRegion = g.placedFeatureNeedsDecorationRegion(featureName)
				regionFeatureCache[featureName] = needsRegion
			}
			if needsRegion {
				if treeRegion == nil {
					treeRegion = newTreeDecorationRegion(g, c, biomes, chunkX, chunkZ, minY, maxY)
				}
				g.runPlacedFeatureAcrossRegion(treeRegion, step, featureName, featureIndex)
				continue
			}
			g.runPlacedFeature(c, biomes, chunkX, chunkZ, minY, maxY, step, featureName, featureIndex, decorationSeed)
		}
	}
}

func (g Generator) collectChunkBiomes(c *chunk.Chunk, biomes sourceBiomeVolume, minY, maxY int, surfaceOnly bool) []gen.Biome {
	var seen biomeSet
	if surfaceOnly {
		for localX := 0; localX < 16; localX++ {
			for localZ := 0; localZ < 16; localZ++ {
				surfaceY := g.heightmapPlacementY(c, localX, localZ, "WORLD_SURFACE", minY, maxY) - 1
				if surfaceY < minY {
					surfaceY = minY
				}
				if surfaceY > maxY {
					surfaceY = maxY
				}
				seen.add(biomes.biomeAt(localX, surfaceY, localZ))
			}
		}
	} else {
		for localX := 0; localX < 16; localX += 4 {
			for localZ := 0; localZ < 16; localZ += 4 {
				for y := minY; y <= maxY; y += 4 {
					seen.add(biomes.biomeAt(localX, y, localZ))
				}
			}
		}
	}

	out := make([]gen.Biome, 0, 8)
	for _, biome := range sortedBiomesByKey {
		if seen.contains(biome) {
			out = append(out, biome)
		}
	}
	return out
}

func (g Generator) collectPossibleFeatureBiomes(chunkX, chunkZ, minY, maxY int) []gen.Biome {
	var seen biomeSet
	startY := alignDown(minY, biomeCellSize)
	for sampleChunkX := chunkX - 1; sampleChunkX <= chunkX+1; sampleChunkX++ {
		for sampleChunkZ := chunkZ - 1; sampleChunkZ <= chunkZ+1; sampleChunkZ++ {
			for localX := 0; localX < 16; localX += biomeCellSize {
				worldX := sampleChunkX*16 + localX
				for localZ := 0; localZ < 16; localZ += biomeCellSize {
					worldZ := sampleChunkZ*16 + localZ
					for y := startY; y <= maxY; y += biomeCellSize {
						seen.add(g.biomeSource.GetBiome(worldX, y, worldZ))
					}
				}
			}
		}
	}

	out := make([]gen.Biome, 0, 16)
	for _, biome := range sortedBiomesByKey {
		if seen.contains(biome) {
			out = append(out, biome)
		}
	}
	return out
}

func (g Generator) runPlacedFeature(c *chunk.Chunk, biomes sourceBiomeVolume, chunkX, chunkZ, minY, maxY int, step gen.GenerationStep, featureName string, featureIndex int, decorationSeed int64) {
	placed, err := g.features.Placed(featureName)
	if err != nil {
		return
	}

	origin := cube.Pos{chunkX * 16, minY, chunkZ * 16}
	rng := g.featureRNG(decorationSeed, featureIndex, step)
	positions, ok := g.applyPlacementModifiers(c, biomes, []cube.Pos{origin}, placed.Placement, featureName, chunkX, chunkZ, minY, maxY, rng)
	if !ok {
		return
	}

	for _, pos := range positions {
		g.executeConfiguredFeature(c, biomes, pos, placed.Feature, featureName, chunkX, chunkZ, minY, maxY, rng, 0)
	}
}

func (g Generator) executeConfiguredFeature(c *chunk.Chunk, biomes sourceBiomeVolume, pos cube.Pos, featureRef gen.ConfiguredFeatureRef, topFeatureName string, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom, depth int) bool {
	if depth > 8 {
		return false
	}

	feature, err := g.features.ResolveConfigured(featureRef)
	if err != nil {
		return false
	}

	switch feature.Type {
	case "random_patch":
		cfg, err := feature.RandomPatch()
		if err != nil {
			return false
		}
		return g.executeRandomPatch(c, biomes, pos, cfg, topFeatureName, chunkX, chunkZ, minY, maxY, rng, depth+1)
	case "flower":
		cfg, err := feature.Flower()
		if err != nil {
			return false
		}
		return g.executeRandomPatch(c, biomes, pos, cfg, topFeatureName, chunkX, chunkZ, minY, maxY, rng, depth+1)
	case "simple_block":
		cfg, err := feature.SimpleBlock()
		if err != nil {
			return false
		}
		return g.placeStateProviderBlock(c, pos, cfg.ToPlace, rng, minY, maxY)
	case "block_blob":
		cfg, err := feature.BlockBlob()
		if err != nil {
			return false
		}
		return g.executeBlockBlob(c, pos, cfg, chunkX, chunkZ, minY, maxY, rng)
	case "block_column":
		cfg, err := feature.BlockColumn()
		if err != nil {
			return false
		}
		return g.executeBlockColumn(c, pos, cfg, chunkX, chunkZ, minY, maxY, rng)
	case "random_selector":
		cfg, err := feature.RandomSelector()
		if err != nil {
			return false
		}
		for _, entry := range cfg.Features {
			if rng.NextDouble() < entry.Chance {
				return g.executePlacedFeatureRef(c, biomes, pos, entry.Feature, topFeatureName, chunkX, chunkZ, minY, maxY, rng, depth+1)
			}
		}
		return g.executePlacedFeatureRef(c, biomes, pos, cfg.Default, topFeatureName, chunkX, chunkZ, minY, maxY, rng, depth+1)
	case "simple_random_selector":
		cfg, err := feature.SimpleRandomSelector()
		if err != nil || len(cfg.Features) == 0 {
			return false
		}
		ref := cfg.Features[int(rng.NextInt(uint32(len(cfg.Features))))]
		return g.executePlacedFeatureRef(c, biomes, pos, ref, topFeatureName, chunkX, chunkZ, minY, maxY, rng, depth+1)
	case "random_boolean_selector":
		cfg, err := feature.RandomBooleanSelector()
		if err != nil {
			return false
		}
		if rng.NextDouble() < 0.5 {
			return g.executePlacedFeatureRef(c, biomes, pos, cfg.FeatureTrue, topFeatureName, chunkX, chunkZ, minY, maxY, rng, depth+1)
		}
		return g.executePlacedFeatureRef(c, biomes, pos, cfg.FeatureFalse, topFeatureName, chunkX, chunkZ, minY, maxY, rng, depth+1)
	case "seagrass":
		cfg, err := feature.Seagrass()
		if err != nil {
			return false
		}
		return g.executeSeagrass(c, pos, cfg, minY, maxY, rng)
	case "kelp":
		return g.executeKelp(c, pos, minY, maxY, rng)
	case "multiface_growth":
		cfg, err := feature.MultifaceGrowth()
		if err != nil {
			return false
		}
		return g.executeMultifaceGrowth(c, pos, cfg, chunkX, chunkZ, minY, maxY, rng)
	case "ore":
		cfg, err := feature.Ore()
		if err != nil {
			return false
		}
		return g.executeOre(c, pos, cfg, chunkX, chunkZ, minY, maxY, rng, false)
	case "scattered_ore":
		cfg, err := feature.ScatteredOre()
		if err != nil {
			return false
		}
		return g.executeOre(c, pos, cfg, chunkX, chunkZ, minY, maxY, rng, true)
	case "disk":
		cfg, err := feature.Disk()
		if err != nil {
			return false
		}
		return g.executeDisk(c, pos, cfg, chunkX, chunkZ, minY, maxY, rng)
	case "spring_feature":
		cfg, err := feature.SpringFeature()
		if err != nil {
			return false
		}
		return g.executeSpringFeature(c, pos, cfg, chunkX, chunkZ, minY, maxY, rng)
	case "underwater_magma":
		cfg, err := feature.UnderwaterMagma()
		if err != nil {
			return false
		}
		return g.executeUnderwaterMagma(c, pos, cfg, chunkX, chunkZ, minY, maxY, rng)
	case "pointed_dripstone":
		cfg, err := feature.PointedDripstone()
		if err != nil {
			return false
		}
		return g.executePointedDripstone(c, pos, cfg, chunkX, chunkZ, minY, maxY, rng)
	case "dripstone_cluster":
		cfg, err := feature.DripstoneCluster()
		if err != nil {
			return false
		}
		return g.executeDripstoneCluster(c, pos, cfg, chunkX, chunkZ, minY, maxY, rng)
	case "large_dripstone":
		cfg, err := feature.LargeDripstone()
		if err != nil {
			return false
		}
		return g.executeLargeDripstone(c, pos, cfg, chunkX, chunkZ, minY, maxY, rng)
	case "geode":
		cfg, err := feature.Geode()
		if err != nil {
			return false
		}
		return g.executeGeode(c, pos, cfg, chunkX, chunkZ, minY, maxY, rng)
	case "fossil":
		cfg, err := feature.Fossil()
		if err != nil {
			return false
		}
		return g.executeFossil(c, pos, cfg, chunkX, chunkZ, minY, maxY, rng)
	case "sculk_patch":
		cfg, err := feature.SculkPatch()
		if err != nil {
			return false
		}
		return g.executeSculkPatch(c, pos, cfg, chunkX, chunkZ, minY, maxY, rng)
	case "vines":
		return g.executeVines(c, pos, chunkX, chunkZ, minY, maxY, rng)
	case "sea_pickle":
		cfg, err := feature.SeaPickle()
		if err != nil {
			return false
		}
		return g.executeSeaPickle(c, pos, cfg, chunkX, chunkZ, minY, maxY, rng)
	case "lake":
		cfg, err := feature.Lake()
		if err != nil {
			return false
		}
		return g.executeLake(c, pos, cfg, chunkX, chunkZ, minY, maxY, rng)
	case "freeze_top_layer":
		cfg, err := feature.FreezeTopLayer()
		if err != nil {
			return false
		}
		return g.executeFreezeTopLayer(c, biomes, cfg, chunkX, chunkZ, minY, maxY)
	case "bamboo":
		cfg, err := feature.Bamboo()
		if err != nil {
			return false
		}
		return g.executeBamboo(c, pos, cfg, chunkX, chunkZ, minY, maxY, rng)
	case "vegetation_patch":
		cfg, err := feature.VegetationPatch()
		if err != nil {
			return false
		}
		return g.executeVegetationPatch(c, biomes, pos, cfg, topFeatureName, chunkX, chunkZ, minY, maxY, rng, depth+1, false)
	case "waterlogged_vegetation_patch":
		cfg, err := feature.WaterloggedVegetationPatch()
		if err != nil {
			return false
		}
		return g.executeVegetationPatch(c, biomes, pos, cfg, topFeatureName, chunkX, chunkZ, minY, maxY, rng, depth+1, true)
	case "root_system":
		cfg, err := feature.RootSystem()
		if err != nil {
			return false
		}
		return g.executeRootSystem(c, biomes, pos, cfg, topFeatureName, chunkX, chunkZ, minY, maxY, rng, depth+1)
	case "fallen_tree":
		cfg, err := feature.FallenTree()
		if err != nil {
			return false
		}
		return g.executeFallenTree(c, pos, cfg, chunkX, chunkZ, minY, maxY, rng)
	case "tree":
		cfg, err := feature.Tree()
		if err != nil {
			return false
		}
		return g.executeTree(c, pos, cfg, minY, maxY, rng)
	case "huge_brown_mushroom":
		cfg, err := feature.HugeBrownMushroom()
		if err != nil {
			return false
		}
		return g.executeHugeBrownMushroom(c, pos, cfg, chunkX, chunkZ, minY, maxY, rng)
	case "huge_red_mushroom":
		cfg, err := feature.HugeRedMushroom()
		if err != nil {
			return false
		}
		return g.executeHugeRedMushroom(c, pos, cfg, chunkX, chunkZ, minY, maxY, rng)
	case "desert_well":
		if _, err := feature.DesertWell(); err != nil {
			return false
		}
		return g.executeDesertWell(c, pos, chunkX, chunkZ, minY, maxY, rng)
	case "iceberg":
		cfg, err := feature.Iceberg()
		if err != nil {
			return false
		}
		return g.executeIceberg(c, pos, cfg, chunkX, chunkZ, minY, maxY, rng)
	case "blue_ice":
		if _, err := feature.BlueIce(); err != nil {
			return false
		}
		return g.executeBlueIce(c, pos, chunkX, chunkZ, minY, maxY, rng)
	case "monster_room":
		if _, err := feature.MonsterRoom(); err != nil {
			return false
		}
		return g.executeMonsterRoom(c, pos, chunkX, chunkZ, minY, maxY, rng)
	case "huge_fungus":
		cfg, err := feature.HugeFungus()
		if err != nil {
			return false
		}
		return g.executeHugeFungus(c, pos, cfg, chunkX, chunkZ, minY, maxY, rng)
	case "nether_forest_vegetation":
		cfg, err := feature.NetherForestVegetation()
		if err != nil {
			return false
		}
		return g.executeNetherForestVegetation(c, pos, cfg, chunkX, chunkZ, minY, maxY, rng)
	case "twisting_vines":
		cfg, err := feature.TwistingVines()
		if err != nil {
			return false
		}
		return g.executeTwistingVines(c, pos, cfg, chunkX, chunkZ, minY, maxY, rng)
	case "weeping_vines":
		cfg, err := feature.WeepingVines()
		if err != nil {
			return false
		}
		return g.executeWeepingVines(c, pos, cfg, chunkX, chunkZ, minY, maxY, rng)
	case "netherrack_replace_blobs":
		cfg, err := feature.NetherrackReplaceBlobs()
		if err != nil {
			return false
		}
		return g.executeNetherrackReplaceBlobs(c, pos, cfg, chunkX, chunkZ, minY, maxY, rng)
	case "glowstone_blob":
		cfg, err := feature.GlowstoneBlob()
		if err != nil {
			return false
		}
		return g.executeGlowstoneBlob(c, pos, cfg, chunkX, chunkZ, minY, maxY, rng)
	case "basalt_pillar":
		cfg, err := feature.BasaltPillar()
		if err != nil {
			return false
		}
		return g.executeBasaltPillar(c, pos, cfg, chunkX, chunkZ, minY, maxY, rng)
	case "basalt_columns":
		cfg, err := feature.BasaltColumns()
		if err != nil {
			return false
		}
		return g.executeBasaltColumns(c, pos, cfg, chunkX, chunkZ, minY, maxY, rng)
	case "delta_feature":
		cfg, err := feature.DeltaFeature()
		if err != nil {
			return false
		}
		return g.executeDeltaFeature(c, pos, cfg, chunkX, chunkZ, minY, maxY, rng)
	case "chorus_plant":
		cfg, err := feature.ChorusPlant()
		if err != nil {
			return false
		}
		return g.executeChorusPlant(c, pos, cfg, chunkX, chunkZ, minY, maxY, rng)
	case "end_island":
		cfg, err := feature.EndIsland()
		if err != nil {
			return false
		}
		return g.executeEndIsland(c, pos, cfg, chunkX, chunkZ, minY, maxY, rng)
	case "spike":
		cfg, err := feature.Spike()
		if err != nil {
			return false
		}
		return g.executeSpike(c, pos, cfg, chunkX, chunkZ, minY, maxY, rng)
	case "end_spike":
		cfg, err := feature.EndSpike()
		if err != nil {
			return false
		}
		return g.executeEndSpike(c, pos, cfg, chunkX, chunkZ, minY, maxY, rng)
	case "end_platform":
		cfg, err := feature.EndPlatform()
		if err != nil {
			return false
		}
		return g.executeEndPlatform(c, pos, cfg, chunkX, chunkZ, minY, maxY)
	case "end_gateway":
		cfg, err := feature.EndGateway()
		if err != nil {
			return false
		}
		return g.executeEndGateway(c, pos, cfg, chunkX, chunkZ, minY, maxY)
	case "void_start_platform":
		if _, err := feature.VoidStartPlatform(); err != nil {
			return false
		}
		return g.executeVoidStartPlatform(c, pos, chunkX, chunkZ, minY, maxY)
	default:
		return false
	}
}

func (g Generator) executePlacedFeatureRef(c *chunk.Chunk, biomes sourceBiomeVolume, pos cube.Pos, placedRef gen.PlacedFeatureRef, topFeatureName string, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom, depth int) bool {
	if depth > 8 {
		return false
	}

	placed, err := g.features.ResolvePlaced(placedRef)
	if err != nil {
		return false
	}
	if topFeatureName == "" {
		topFeatureName = placedRef.Name
	}
	positions, ok := g.applyPlacementModifiers(c, biomes, []cube.Pos{pos}, placed.Placement, topFeatureName, chunkX, chunkZ, minY, maxY, rng)
	if !ok {
		return false
	}

	var placedAny bool
	for _, candidate := range positions {
		if g.executeConfiguredFeature(c, biomes, candidate, placed.Feature, topFeatureName, chunkX, chunkZ, minY, maxY, rng, depth+1) {
			placedAny = true
		}
	}
	return placedAny
}

func (g Generator) executeRandomPatch(c *chunk.Chunk, biomes sourceBiomeVolume, origin cube.Pos, cfg gen.RandomPatchConfig, topFeatureName string, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom, depth int) bool {
	var placedAny bool
	for attempt := 0; attempt < cfg.Tries; attempt++ {
		pos := origin.Add(cube.Pos{
			g.signedSpread(rng, cfg.XZSpread),
			g.signedSpread(rng, cfg.YSpread),
			g.signedSpread(rng, cfg.XZSpread),
		})
		if !g.positionInFeatureScope(pos, chunkX, chunkZ, minY, maxY) {
			continue
		}
		if g.executePlacedFeatureRef(c, biomes, pos, cfg.Feature, topFeatureName, chunkX, chunkZ, minY, maxY, rng, depth+1) {
			placedAny = true
		}
	}
	return placedAny
}

func (g Generator) executeBlockColumn(c *chunk.Chunk, origin cube.Pos, cfg gen.BlockColumnConfig, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom) bool {
	dir := blockColumnDirection(cfg.Direction)
	if dir == (cube.Pos{}) {
		return false
	}

	current := origin
	var placedAny bool
	for _, layer := range cfg.Layers {
		height := max(0, g.sampleIntProvider(layer.Height, rng))
		for i := 0; i < height; i++ {
			if !g.positionInChunk(current, chunkX, chunkZ, minY, maxY) {
				return placedAny
			}
			if !g.testBlockPredicate(c, current, cfg.AllowedPlacement, chunkX, chunkZ, minY, maxY, rng) {
				return placedAny
			}
			if !g.placeStateProviderBlock(c, current, layer.Provider, rng, minY, maxY) {
				return placedAny
			}
			placedAny = true
			current = current.Add(dir)
		}
	}
	return placedAny
}

func (g Generator) executeSeagrass(c *chunk.Chunk, pos cube.Pos, cfg gen.SeagrassConfig, minY, maxY int, rng *gen.WorldgenRandom) bool {
	if pos[1] <= minY || pos[1] >= maxY {
		return false
	}
	if c.Block(uint8(pos[0]&15), int16(pos[1]), uint8(pos[2]&15), 0) != g.waterRID {
		return false
	}
	belowRID := c.Block(uint8(pos[0]&15), int16(pos[1]-1), uint8(pos[2]&15), 0)
	if !g.isSolidRID(belowRID) {
		return false
	}

	if cfg.Probability > 0 && rng.NextDouble() < cfg.Probability {
		upper := pos.Side(cube.FaceUp)
		if upper[1] <= maxY && c.Block(uint8(upper[0]&15), int16(upper[1]), uint8(upper[2]&15), 0) == g.waterRID {
			return g.setBlockStateDirect(c, pos, gen.BlockState{Name: "tall_seagrass", Properties: map[string]string{"half": "lower"}}) &&
				g.setBlockStateDirect(c, upper, gen.BlockState{Name: "tall_seagrass", Properties: map[string]string{"half": "upper"}})
		}
	}
	return g.setBlockStateDirect(c, pos, gen.BlockState{Name: "seagrass"})
}

func (g Generator) executeKelp(c *chunk.Chunk, pos cube.Pos, minY, maxY int, rng *gen.WorldgenRandom) bool {
	if pos[1] <= minY || pos[1] >= maxY {
		return false
	}
	if c.Block(uint8(pos[0]&15), int16(pos[1]), uint8(pos[2]&15), 0) != g.waterRID {
		return false
	}

	height := 1 + int(rng.NextInt(10))
	var placedAny bool
	for i := 0; i < height && pos[1]+i <= maxY; i++ {
		current := pos.Add(cube.Pos{0, i, 0})
		if c.Block(uint8(current[0]&15), int16(current[1]), uint8(current[2]&15), 0) != g.waterRID {
			break
		}
		if !g.setBlockStateDirect(c, current, gen.BlockState{Name: "kelp", Properties: map[string]string{"age": strconv.Itoa(int(rng.NextInt(25)))}}) {
			break
		}
		placedAny = true
	}
	return placedAny
}

func (g Generator) executeMultifaceGrowth(c *chunk.Chunk, pos cube.Pos, cfg gen.MultifaceGrowthConfig, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom) bool {
	for attempt := 0; attempt <= max(1, cfg.SearchRange); attempt++ {
		candidate := pos.Add(cube.Pos{
			int(rng.NextInt(uint32(max(1, cfg.SearchRange*2+1)))) - cfg.SearchRange,
			int(rng.NextInt(uint32(max(1, cfg.SearchRange*2+1)))) - cfg.SearchRange,
			int(rng.NextInt(uint32(max(1, cfg.SearchRange*2+1)))) - cfg.SearchRange,
		})
		if !g.positionInChunk(candidate, chunkX, chunkZ, minY, maxY) {
			continue
		}
		rid := c.Block(uint8(candidate[0]&15), int16(candidate[1]), uint8(candidate[2]&15), 0)
		if rid != g.airRID && rid != g.waterRID {
			continue
		}
		if state, ok := g.multifaceStateAt(c, candidate, cfg, chunkX, chunkZ, minY, maxY); ok {
			return g.setBlockStateDirect(c, candidate, state)
		}
	}
	return false
}

func (g Generator) executeOre(c *chunk.Chunk, pos cube.Pos, cfg gen.OreConfig, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom, scattered bool) bool {
	if cfg.Size <= 0 {
		return false
	}
	if scattered {
		var placedAny bool
		for i := 0; i < cfg.Size; i++ {
			candidate := pos.Add(cube.Pos{
				int(rng.NextInt(5)) - 2,
				int(rng.NextInt(5)) - 2,
				int(rng.NextInt(5)) - 2,
			})
			if g.tryPlaceOreAt(c, candidate, cfg, chunkX, chunkZ, minY, maxY, rng) {
				placedAny = true
			}
		}
		return placedAny
	}

	angle := rng.NextDouble() * math.Pi
	spread := float64(cfg.Size) / 8.0
	x1 := float64(pos[0]) + math.Sin(angle)*spread
	x2 := float64(pos[0]) - math.Sin(angle)*spread
	z1 := float64(pos[2]) + math.Cos(angle)*spread
	z2 := float64(pos[2]) - math.Cos(angle)*spread
	y1 := float64(pos[1] + int(rng.NextInt(3)) - 1)
	y2 := float64(pos[1] + int(rng.NextInt(3)) - 1)

	var placedAny bool
	for i := 0; i < cfg.Size; i++ {
		t := float64(i) / float64(cfg.Size)
		cx := lerp(t, x1, x2)
		cy := lerp(t, y1, y2)
		cz := lerp(t, z1, z2)
		radius := ((1.0-math.Abs(2.0*t-1.0))*float64(cfg.Size)/16.0 + 1.0) / 2.0
		minX, maxX := int(math.Floor(cx-radius)), int(math.Ceil(cx+radius))
		minZ, maxZ := int(math.Floor(cz-radius)), int(math.Ceil(cz+radius))
		minBlockY, maxBlockY := int(math.Floor(cy-radius)), int(math.Ceil(cy+radius))
		for x := minX; x <= maxX; x++ {
			for y := minBlockY; y <= maxBlockY; y++ {
				for z := minZ; z <= maxZ; z++ {
					candidate := cube.Pos{x, y, z}
					if !g.positionInChunk(candidate, chunkX, chunkZ, minY, maxY) {
						continue
					}
					dx, dy, dz := float64(x)-cx, float64(y)-cy, float64(z)-cz
					if dx*dx+dy*dy+dz*dz > radius*radius {
						continue
					}
					if g.tryPlaceOreAt(c, candidate, cfg, chunkX, chunkZ, minY, maxY, rng) {
						placedAny = true
					}
				}
			}
		}
	}
	return placedAny
}

func (g Generator) executeDisk(c *chunk.Chunk, pos cube.Pos, cfg gen.DiskConfig, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom) bool {
	radius := max(1, g.sampleIntProvider(cfg.Radius, rng))
	var placedAny bool
	for dx := -radius; dx <= radius; dx++ {
		for dz := -radius; dz <= radius; dz++ {
			if dx*dx+dz*dz > radius*radius {
				continue
			}
			for dy := -cfg.HalfHeight; dy <= cfg.HalfHeight; dy++ {
				candidate := pos.Add(cube.Pos{dx, dy, dz})
				if !g.positionInChunk(candidate, chunkX, chunkZ, minY, maxY) {
					continue
				}
				if !g.testBlockPredicate(c, candidate, cfg.Target, chunkX, chunkZ, minY, maxY, rng) {
					continue
				}
				state, ok := g.selectState(c, cfg.StateProvider, candidate, rng, minY, maxY)
				if !ok || !g.setBlockStateDirect(c, candidate, state) {
					continue
				}
				placedAny = true
			}
		}
	}
	return placedAny
}

func (g Generator) executeSpringFeature(c *chunk.Chunk, pos cube.Pos, cfg gen.SpringFeatureConfig, chunkX, chunkZ, minY, maxY int, _ *gen.WorldgenRandom) bool {
	if !g.positionInChunk(pos, chunkX, chunkZ, minY, maxY) {
		return false
	}
	if g.blockNameAt(c, pos) != "air" {
		return false
	}

	valid := func(target cube.Pos) bool {
		if !g.positionInChunk(target, chunkX, chunkZ, minY, maxY) {
			return false
		}
		return slices.Contains(cfg.ValidBlocks.Values, g.blockNameAt(c, target))
	}

	if cfg.RequiresBlockBelow && !valid(pos.Side(cube.FaceDown)) {
		return false
	}
	if !valid(pos.Side(cube.FaceUp)) {
		return false
	}

	rocks, holes := 0, 0
	for _, face := range append(cube.HorizontalFaces(), cube.FaceDown) {
		neighbor := pos.Side(face)
		if valid(neighbor) {
			rocks++
			continue
		}
		if g.positionInChunk(neighbor, chunkX, chunkZ, minY, maxY) && g.blockNameAt(c, neighbor) == "air" {
			holes++
		}
	}
	if rocks != cfg.RockCount || holes != cfg.HoleCount {
		return false
	}
	return g.setBlockStateDirect(c, pos, cfg.State)
}

func (g Generator) executeUnderwaterMagma(c *chunk.Chunk, pos cube.Pos, cfg gen.UnderwaterMagmaConfig, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom) bool {
	floorY := -1
	for y := pos[1]; y >= max(minY, pos[1]-cfg.FloorSearchRange); y-- {
		candidate := cube.Pos{pos[0], y, pos[2]}
		if c.Block(uint8(candidate[0]&15), int16(candidate[1]), uint8(candidate[2]&15), 0) == g.waterRID &&
			g.isSolidRID(c.Block(uint8(candidate[0]&15), int16(candidate[1]-1), uint8(candidate[2]&15), 0)) {
			floorY = y - 1
			break
		}
	}
	if floorY < minY {
		return false
	}

	var placedAny bool
	for dx := -cfg.PlacementRadiusAroundFloor; dx <= cfg.PlacementRadiusAroundFloor; dx++ {
		for dz := -cfg.PlacementRadiusAroundFloor; dz <= cfg.PlacementRadiusAroundFloor; dz++ {
			candidate := cube.Pos{pos[0] + dx, floorY, pos[2] + dz}
			if !g.positionInChunk(candidate, chunkX, chunkZ, minY, maxY) || rng.NextDouble() > cfg.PlacementProbabilityPerValidPosition {
				continue
			}
			above := candidate.Side(cube.FaceUp)
			if c.Block(uint8(above[0]&15), int16(above[1]), uint8(above[2]&15), 0) != g.waterRID {
				continue
			}
			if g.setBlockStateDirect(c, candidate, gen.BlockState{Name: "magma"}) {
				placedAny = true
			}
		}
	}
	return placedAny
}

func (g Generator) executePointedDripstone(c *chunk.Chunk, pos cube.Pos, cfg gen.PointedDripstoneConfig, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom) bool {
	if !g.positionInChunk(pos, chunkX, chunkZ, minY, maxY) {
		return false
	}
	currentRID := c.Block(uint8(pos[0]&15), int16(pos[1]), uint8(pos[2]&15), 0)
	if currentRID != g.airRID && currentRID != g.waterRID {
		return false
	}

	upSolid := g.isSolidInChunk(c, pos.Side(cube.FaceUp), chunkX, chunkZ, minY, maxY)
	downSolid := g.isSolidInChunk(c, pos.Side(cube.FaceDown), chunkX, chunkZ, minY, maxY)
	var direction string
	switch {
	case upSolid && !downSolid:
		direction = "down"
	case downSolid && !upSolid:
		direction = "up"
	case upSolid:
		direction = "down"
	default:
		return false
	}

	if !g.setBlockStateDirect(c, pos, pointedDripstoneState(direction, "tip")) {
		return false
	}
	if rng.NextDouble() < cfg.ChanceOfTallerDripstone {
		var next cube.Pos
		if direction == "down" {
			next = pos.Side(cube.FaceDown)
		} else {
			next = pos.Side(cube.FaceUp)
		}
		if g.positionInChunk(next, chunkX, chunkZ, minY, maxY) {
			rid := c.Block(uint8(next[0]&15), int16(next[1]), uint8(next[2]&15), 0)
			if rid == g.airRID || rid == g.waterRID {
				_ = g.setBlockStateDirect(c, next, pointedDripstoneState(direction, "base"))
			}
		}
	}
	return true
}

func (g Generator) executeDripstoneCluster(c *chunk.Chunk, pos cube.Pos, cfg gen.DripstoneClusterConfig, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom) bool {
	floor, ceiling, ok := g.findFloorAndCeiling(c, pos, cfg.FloorToCeilingSearchRange, chunkX, chunkZ, minY, maxY)
	if !ok {
		return false
	}
	radius := max(1, g.sampleIntProvider(cfg.Radius, rng))
	thickness := max(1, g.sampleIntProvider(cfg.DripstoneBlockLayerThickness, rng))
	height := max(1, g.sampleIntProvider(cfg.Height, rng))
	var placedAny bool

	for dx := -radius; dx <= radius; dx++ {
		for dz := -radius; dz <= radius; dz++ {
			if dx*dx+dz*dz > radius*radius {
				continue
			}
			for t := 0; t < thickness; t++ {
				if g.setBlockStateDirect(c, floor.Add(cube.Pos{dx, t, dz}), gen.BlockState{Name: "dripstone_block"}) {
					placedAny = true
				}
				if g.setBlockStateDirect(c, ceiling.Add(cube.Pos{dx, -t, dz}), gen.BlockState{Name: "dripstone_block"}) {
					placedAny = true
				}
			}
			base := floor.Add(cube.Pos{dx, thickness, dz})
			top := ceiling.Add(cube.Pos{dx, -thickness, dz})
			for i := 0; i < height && base[1]+i < top[1]; i++ {
				if g.setBlockStateDirect(c, base.Add(cube.Pos{0, i, 0}), pointedDripstoneState("up", "tip")) {
					placedAny = true
				}
				if g.setBlockStateDirect(c, top.Add(cube.Pos{0, -i, 0}), pointedDripstoneState("down", "tip")) {
					placedAny = true
				}
			}
		}
	}
	return placedAny
}

func (g Generator) executeLargeDripstone(c *chunk.Chunk, pos cube.Pos, cfg gen.LargeDripstoneConfig, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom) bool {
	if !g.positionInFeatureScope(pos, chunkX, chunkZ, minY, maxY) {
		return false
	}
	rid := g.runtimeIDAt(c, pos)
	if rid != g.airRID && rid != g.waterRID {
		return false
	}

	floor, ceiling, ok := g.findFloorAndCeiling(c, pos, cfg.FloorToCeilingSearchRange, chunkX, chunkZ, minY, maxY)
	if !ok {
		return false
	}
	columnHeight := ceiling[1] - floor[1] - 1
	if columnHeight < 4 {
		return false
	}

	maxColumnRadiusBasedOnHeight := int(float64(columnHeight) * cfg.MaxColumnRadiusToCaveHeightRatio)
	maxColumnRadius := clamp(maxColumnRadiusBasedOnHeight, cfg.ColumnRadius.MinInclusive, cfg.ColumnRadius.MaxInclusive)
	radius := clamp(g.sampleIntProvider(cfg.ColumnRadius, rng), cfg.ColumnRadius.MinInclusive, maxColumnRadius)
	if radius <= 0 {
		return false
	}

	scaleDown := g.sampleFloatProvider(cfg.StalactiteBluntness, rng)
	scaleUp := g.sampleFloatProvider(cfg.StalagmiteBluntness, rng)
	heightScale := g.sampleFloatProvider(cfg.HeightScale, rng)
	if scaleDown <= 0 {
		scaleDown = 0.3
	}
	if scaleUp <= 0 {
		scaleUp = 0.4
	}
	if heightScale <= 0 {
		heightScale = 1
	}

	stalactiteOrigin := ceiling.Side(cube.FaceDown)
	stalagmiteOrigin := floor.Side(cube.FaceUp)
	placedDown := g.placeLargeDripstoneColumn(c, stalactiteOrigin, false, radius, scaleDown, heightScale, chunkX, chunkZ, minY, maxY, rng)
	placedUp := g.placeLargeDripstoneColumn(c, stalagmiteOrigin, true, radius, scaleUp, heightScale, chunkX, chunkZ, minY, maxY, rng)
	return placedDown || placedUp
}

func (g Generator) executeGeode(c *chunk.Chunk, pos cube.Pos, cfg gen.GeodeConfig, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom) bool {
	type geodePoint struct {
		pos    cube.Pos
		offset int
	}
	numPoints := max(1, g.sampleIntProvider(cfg.DistributionPoints, rng))
	points := make([]geodePoint, 0, numPoints)
	numInvalid := 0
	for range numPoints {
		x := g.sampleIntProvider(cfg.OuterWallDistance, rng)
		y := g.sampleIntProvider(cfg.OuterWallDistance, rng)
		z := g.sampleIntProvider(cfg.OuterWallDistance, rng)
		pointPos := pos.Add(cube.Pos{x, y, z})
		if !g.positionInFeatureScope(pointPos, chunkX, chunkZ, minY, maxY) {
			numInvalid++
			continue
		}
		name := g.blockNameAt(c, pointPos)
		if name == "air" || g.matchesFeatureBlockTag(name, cfg.Blocks.InvalidBlocks) {
			numInvalid++
			if numInvalid > cfg.InvalidBlocksThreshold {
				return false
			}
		}
		points = append(points, geodePoint{pos: pointPos, offset: max(1, g.sampleIntProvider(cfg.PointOffset, rng))})
	}
	if len(points) == 0 {
		return false
	}

	var crackPoints []cube.Pos
	shouldCrack := rng.NextDouble() < cfg.Crack.GenerateCrackChance
	if shouldCrack {
		crackOffset := numPoints*2 + 1
		switch int(rng.NextInt(4)) {
		case 0:
			crackPoints = []cube.Pos{pos.Add(cube.Pos{crackOffset, 7, 0}), pos.Add(cube.Pos{crackOffset, 5, 0}), pos.Add(cube.Pos{crackOffset, 1, 0})}
		case 1:
			crackPoints = []cube.Pos{pos.Add(cube.Pos{0, 7, crackOffset}), pos.Add(cube.Pos{0, 5, crackOffset}), pos.Add(cube.Pos{0, 1, crackOffset})}
		case 2:
			crackPoints = []cube.Pos{pos.Add(cube.Pos{crackOffset, 7, crackOffset}), pos.Add(cube.Pos{crackOffset, 5, crackOffset}), pos.Add(cube.Pos{crackOffset, 1, crackOffset})}
		default:
			crackPoints = []cube.Pos{pos.Add(cube.Pos{0, 7, 0}), pos.Add(cube.Pos{0, 5, 0}), pos.Add(cube.Pos{0, 1, 0})}
		}
	}

	crackSizeAdjustment := float64(numPoints) / float64(max(1, cfg.OuterWallDistance.MaxInclusive))
	innerAir := 1.0 / math.Sqrt(cfg.Layers.Filling)
	innermostBlockLayer := 1.0 / math.Sqrt(cfg.Layers.InnerLayer+crackSizeAdjustment)
	innerCrust := 1.0 / math.Sqrt(cfg.Layers.MiddleLayer+crackSizeAdjustment)
	outerCrust := 1.0 / math.Sqrt(cfg.Layers.OuterLayer+crackSizeAdjustment)
	crackSize := 1.0 / math.Sqrt(cfg.Crack.BaseCrackSize+rng.NextDouble()/2.0+func() float64 {
		if numPoints > 3 {
			return crackSizeAdjustment
		}
		return 0
	}())

	var potentialCrystalPlacements []cube.Pos
	placedAny := false
	for x := pos[0] + cfg.MinGenOffset; x <= pos[0]+cfg.MaxGenOffset; x++ {
		for y := pos[1] + cfg.MinGenOffset; y <= pos[1]+cfg.MaxGenOffset; y++ {
			for z := pos[2] + cfg.MinGenOffset; z <= pos[2]+cfg.MaxGenOffset; z++ {
				pointInside := cube.Pos{x, y, z}
				if !g.positionInFeatureScope(pointInside, chunkX, chunkZ, minY, maxY) {
					continue
				}
				noiseOffset := g.geodeNoiseValue(pointInside) * cfg.NoiseMultiplier
				distSumShell := 0.0
				distSumCrack := 0.0
				for _, point := range points {
					distSumShell += 1.0/math.Sqrt(posDistSquared(pointInside, point.pos)+float64(point.offset)) + noiseOffset
				}
				for _, point := range crackPoints {
					distSumCrack += 1.0/math.Sqrt(posDistSquared(pointInside, point)+float64(cfg.Crack.CrackPointOffset)) + noiseOffset
				}
				if distSumShell < outerCrust {
					continue
				}
				switch {
				case shouldCrack && distSumCrack >= crackSize && distSumShell < innerAir:
					if g.setGeodeState(c, pointInside, gen.BlockState{Name: "air"}, cfg.Blocks.CannotReplace, chunkX, chunkZ, minY, maxY) {
						placedAny = true
					}
				case distSumShell >= innerAir:
					if state, ok := g.selectState(c, cfg.Blocks.FillingProvider, pointInside, rng, minY, maxY); ok && g.setGeodeState(c, pointInside, state, cfg.Blocks.CannotReplace, chunkX, chunkZ, minY, maxY) {
						placedAny = true
					}
				case distSumShell >= innermostBlockLayer:
					useAlt := rng.NextDouble() < cfg.UseAlternateLayer0Chance
					provider := cfg.Blocks.InnerLayerProvider
					if useAlt {
						provider = cfg.Blocks.AlternateInnerLayerProvider
					}
					if state, ok := g.selectState(c, provider, pointInside, rng, minY, maxY); ok && g.setGeodeState(c, pointInside, state, cfg.Blocks.CannotReplace, chunkX, chunkZ, minY, maxY) {
						placedAny = true
					}
					if (!cfg.PlacementsRequireLayer0Alt || useAlt) && rng.NextDouble() < cfg.UsePotentialPlacementsChance {
						potentialCrystalPlacements = append(potentialCrystalPlacements, pointInside)
					}
				case distSumShell >= innerCrust:
					if state, ok := g.selectState(c, cfg.Blocks.MiddleLayerProvider, pointInside, rng, minY, maxY); ok && g.setGeodeState(c, pointInside, state, cfg.Blocks.CannotReplace, chunkX, chunkZ, minY, maxY) {
						placedAny = true
					}
				case distSumShell >= outerCrust:
					if state, ok := g.selectState(c, cfg.Blocks.OuterLayerProvider, pointInside, rng, minY, maxY); ok && g.setGeodeState(c, pointInside, state, cfg.Blocks.CannotReplace, chunkX, chunkZ, minY, maxY) {
						placedAny = true
					}
				}
			}
		}
	}

	for _, crystalPos := range potentialCrystalPlacements {
		if len(cfg.Blocks.InnerPlacements) == 0 {
			break
		}
		state := cfg.Blocks.InnerPlacements[int(rng.NextInt(uint32(len(cfg.Blocks.InnerPlacements))))]
		for _, face := range cube.Faces() {
			placePos := crystalPos.Side(face)
			if !g.positionInFeatureScope(placePos, chunkX, chunkZ, minY, maxY) {
				continue
			}
			placeName := g.blockNameAt(c, placePos)
			if placeName != "air" && placeName != "water" {
				continue
			}
			props := cloneStateProperties(state.Properties)
			if props == nil {
				props = map[string]string{}
			}
			props["minecraft:block_face"] = face.String()
			if g.setGeodeState(c, placePos, gen.BlockState{Name: state.Name, Properties: props}, cfg.Blocks.CannotReplace, chunkX, chunkZ, minY, maxY) {
				placedAny = true
				break
			}
		}
	}
	return placedAny
}

func (g Generator) executeFossil(c *chunk.Chunk, pos cube.Pos, cfg gen.FossilConfig, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom) bool {
	if len(cfg.FossilStructures) == 0 || len(cfg.FossilStructures) != len(cfg.OverlayStructures) || g.structureTemplates == nil {
		return false
	}
	index := int(rng.NextInt(uint32(len(cfg.FossilStructures))))
	baseName := cfg.FossilStructures[index]
	overlayName := cfg.OverlayStructures[index]
	baseTemplate, err := g.structureTemplates.Template(baseName)
	if err != nil {
		return false
	}
	overlayTemplate, err := g.structureTemplates.Template(overlayName)
	if err != nil {
		return false
	}
	rotation := randomStructureRotation(rng)
	rotSize := rotatedStructureSize(baseTemplate.Size, rotation)
	lowCornerX := pos[0] - rotSize[0]/2
	lowCornerZ := pos[2] - rotSize[2]/2
	lowestSurfaceY := pos[1]
	for xscan := 0; xscan < rotSize[0]; xscan++ {
		for zscan := 0; zscan < rotSize[2]; zscan++ {
			y := g.worldSurfaceLevelAt(lowCornerX+xscan, lowCornerZ+zscan, minY, maxY)
			if y < lowestSurfaceY {
				lowestSurfaceY = y
			}
		}
	}
	targetY := max(lowestSurfaceY-15-int(rng.NextInt(10)), minY+10)
	reference := cube.Pos{lowCornerX, targetY, lowCornerZ}
	box := structureBox{
		minX: reference[0],
		minY: reference[1],
		minZ: reference[2],
		maxX: reference[0] + rotSize[0] - 1,
		maxY: reference[1] + rotSize[1] - 1,
		maxZ: reference[2] + rotSize[2] - 1,
	}
	if g.fossilEmptyCorners(c, chunkX, chunkZ, minY, maxY, box) > cfg.MaxEmptyCornersAllowed {
		return false
	}

	basePlacement := structureTemplatePlacement{
		templateName: baseName,
		ignoreAir:    false,
		processors:   compileStructureProcessors(g.worldgen, cfg.FossilProcessors),
	}
	overlayPlacement := structureTemplatePlacement{
		templateName: overlayName,
		ignoreAir:    false,
		processors:   compileStructureProcessors(g.worldgen, cfg.OverlayProcessors),
	}

	placedAny := false
	for _, blockInfo := range g.processStructureTemplatePlacement(c, chunkX, chunkZ, reference, rotation, structureMirrorNone, cube.Pos{}, false, baseTemplate, basePlacement) {
		if !g.positionInFeatureScope(blockInfo.worldPos, chunkX, chunkZ, minY, maxY) {
			continue
		}
		if blockInfo.state.Name == "structure_void" || blockInfo.state.Name == "jigsaw" || blockInfo.state.Name == "structure_block" || blockInfo.state.Name == "air" {
			continue
		}
		placedState := applyPlacedStructureStateTransform(blockInfo.state, structureMirrorNone, rotation)
		g.placeStructureBlockState(c, chunkX, chunkZ, minY, maxY, blockInfo.worldPos, placedState)
		placedAny = true
	}
	for _, blockInfo := range g.processStructureTemplatePlacement(c, chunkX, chunkZ, reference, rotation, structureMirrorNone, cube.Pos{}, false, overlayTemplate, overlayPlacement) {
		if !g.positionInFeatureScope(blockInfo.worldPos, chunkX, chunkZ, minY, maxY) {
			continue
		}
		if blockInfo.state.Name == "structure_void" || blockInfo.state.Name == "jigsaw" || blockInfo.state.Name == "structure_block" || blockInfo.state.Name == "air" {
			continue
		}
		placedState := applyPlacedStructureStateTransform(blockInfo.state, structureMirrorNone, rotation)
		g.placeStructureBlockState(c, chunkX, chunkZ, minY, maxY, blockInfo.worldPos, placedState)
		placedAny = true
	}
	return placedAny
}

func (g Generator) fossilEmptyCorners(c *chunk.Chunk, chunkX, chunkZ, minY, maxY int, box structureBox) int {
	corners := []cube.Pos{
		{box.minX, box.minY, box.minZ},
		{box.minX, box.minY, box.maxZ},
		{box.minX, box.maxY, box.minZ},
		{box.minX, box.maxY, box.maxZ},
		{box.maxX, box.minY, box.minZ},
		{box.maxX, box.minY, box.maxZ},
		{box.maxX, box.maxY, box.minZ},
		{box.maxX, box.maxY, box.maxZ},
	}
	count := 0
	for _, corner := range corners {
		if !g.positionInFeatureScope(corner, chunkX, chunkZ, minY, maxY) {
			continue
		}
		name := g.blockNameAt(c, corner)
		if name == "air" || name == "lava" || name == "water" {
			count++
		}
	}
	return count
}

func (g Generator) setGeodeState(c *chunk.Chunk, pos cube.Pos, state gen.BlockState, cannotReplaceTag string, chunkX, chunkZ, minY, maxY int) bool {
	if !g.positionInFeatureScope(pos, chunkX, chunkZ, minY, maxY) {
		return false
	}
	if g.matchesFeatureBlockTag(g.blockNameAt(c, pos), cannotReplaceTag) {
		return false
	}
	return g.setBlockStateDirect(c, pos, state)
}

func posDistSquared(a, b cube.Pos) float64 {
	dx := float64(a[0] - b[0])
	dy := float64(a[1] - b[1])
	dz := float64(a[2] - b[2])
	return dx*dx + dy*dy + dz*dz
}

func (g Generator) geodeNoiseValue(pos cube.Pos) float64 {
	v := math.Sin(float64(pos[0])*12.9898 + float64(pos[1])*78.233 + float64(pos[2])*37.719 + float64(g.seed&0xffff))
	return v - math.Floor(v)
}

func (g Generator) placeLargeDripstoneColumn(c *chunk.Chunk, root cube.Pos, pointingUp bool, radius int, bluntness, scale float64, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom) bool {
	var placedAny bool
	for dx := -radius; dx <= radius; dx++ {
		for dz := -radius; dz <= radius; dz++ {
			currentRadius := math.Sqrt(float64(dx*dx + dz*dz))
			if currentRadius > float64(radius) {
				continue
			}
			height := g.largeDripstoneHeightAtRadius(currentRadius, radius, scale, bluntness)
			if height <= 0 {
				continue
			}
			if rng.NextDouble() < 0.2 {
				height = int(float64(height) * (0.8 + rng.NextDouble()*0.2))
			}
			pos := root.Add(cube.Pos{dx, 0, dz})
			hasBeenOutOfStone := false
			maxYLimit := math.MaxInt
			if pointingUp {
				maxYLimit = g.heightmapPlacementYAtPos(c, pos, "WORLD_SURFACE_WG", chunkX, chunkZ, minY, maxY)
			}
			for i := 0; i < height && pos[1] < maxYLimit; i++ {
				if !g.positionInFeatureScope(pos, chunkX, chunkZ, minY, maxY) {
					break
				}
				name := g.blockNameAt(c, pos)
				if name == "air" || name == "water" || name == "lava" {
					hasBeenOutOfStone = true
					if g.setBlockStateDirect(c, pos, gen.BlockState{Name: "dripstone_block"}) {
						placedAny = true
					}
				} else if hasBeenOutOfStone && g.matchesFeatureBlockTag(name, "base_stone_overworld") {
					break
				}
				if pointingUp {
					pos = pos.Side(cube.FaceUp)
				} else {
					pos = pos.Side(cube.FaceDown)
				}
			}
		}
	}
	return placedAny
}

func (g Generator) largeDripstoneHeightAtRadius(checkRadius float64, radius int, scale, bluntness float64) int {
	if radius <= 0 {
		return 0
	}
	normalized := 1.0 - checkRadius/float64(max(radius, 1))
	if normalized <= 0 {
		return 0
	}
	height := normalized * normalized * float64(radius) * (1.0 + scale) / max(bluntness, 0.1)
	return int(math.Round(height))
}

func (g Generator) executeSculkPatch(c *chunk.Chunk, pos cube.Pos, cfg gen.SculkPatchConfig, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom) bool {
	attempts := max(1, min(cfg.SpreadAttempts, cfg.ChargeCount*cfg.SpreadRounds*4))
	var placedAny bool
	for i := 0; i < attempts; i++ {
		candidate := pos.Add(cube.Pos{
			int(rng.NextInt(9)) - 4,
			int(rng.NextInt(5)) - 2,
			int(rng.NextInt(9)) - 4,
		})
		if !g.positionInChunk(candidate, chunkX, chunkZ, minY, maxY) {
			continue
		}
		floor := candidate.Side(cube.FaceDown)
		if !g.isSolidInChunk(c, floor, chunkX, chunkZ, minY, maxY) {
			continue
		}
		rid := c.Block(uint8(candidate[0]&15), int16(candidate[1]), uint8(candidate[2]&15), 0)
		if rid != g.airRID && rid != g.waterRID {
			continue
		}
		if g.setBlockStateDirect(c, floor, gen.BlockState{Name: "sculk"}) {
			placedAny = true
		}
		if rng.NextDouble() < 0.35 {
			_, _ = cfg, rng
			_ = g.executeMultifaceGrowth(c, candidate, gen.MultifaceGrowthConfig{
				Block:             "sculk_vein",
				CanBePlacedOn:     []string{"stone", "andesite", "diorite", "granite", "dripstone_block", "calcite", "tuff", "deepslate", "sculk"},
				CanPlaceOnCeiling: true,
				CanPlaceOnFloor:   true,
				CanPlaceOnWall:    true,
				ChanceOfSpreading: 1.0,
				SearchRange:       4,
			}, chunkX, chunkZ, minY, maxY, rng)
		}
	}
	return placedAny
}

func (g Generator) executeVines(c *chunk.Chunk, pos cube.Pos, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom) bool {
	if !g.positionInChunk(pos, chunkX, chunkZ, minY, maxY) {
		return false
	}
	attachments := []struct {
		face cube.Face
		dir  cube.Direction
	}{
		{cube.FaceNorth, cube.North},
		{cube.FaceEast, cube.East},
		{cube.FaceSouth, cube.South},
		{cube.FaceWest, cube.West},
	}
	var vine block.Vines
	for _, attachment := range attachments {
		support := pos.Side(attachment.face)
		if g.isSolidInChunk(c, support, chunkX, chunkZ, minY, maxY) {
			vine = vine.WithAttachment(attachment.dir.Opposite(), true)
		}
	}
	if len(vine.Attachments()) == 0 {
		return false
	}
	height := 1 + int(rng.NextInt(4))
	var placedAny bool
	for i := 0; i < height && pos[1]-i > minY; i++ {
		current := pos.Add(cube.Pos{0, -i, 0})
		rid := c.Block(uint8(current[0]&15), int16(current[1]), uint8(current[2]&15), 0)
		if rid != g.airRID {
			break
		}
		c.SetBlock(uint8(current[0]&15), int16(current[1]), uint8(current[2]&15), 0, runtimeIDForBlock(vine))
		placedAny = true
	}
	return placedAny
}

func (g Generator) executeSeaPickle(c *chunk.Chunk, pos cube.Pos, cfg gen.SeaPickleConfig, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom) bool {
	if !g.positionInChunk(pos, chunkX, chunkZ, minY, maxY) {
		return false
	}
	if c.Block(uint8(pos[0]&15), int16(pos[1]), uint8(pos[2]&15), 0) != g.waterRID {
		return false
	}
	below := pos.Side(cube.FaceDown)
	if !g.isSolidInChunk(c, below, chunkX, chunkZ, minY, maxY) {
		return false
	}
	additional := 0
	if cfg.Count > 1 {
		additional = int(rng.NextInt(uint32(min(cfg.Count, 4))))
	}
	return g.setFeatureBlock(c, pos, block.SeaPickle{AdditionalCount: additional})
}

func (g Generator) executeLake(c *chunk.Chunk, pos cube.Pos, cfg gen.LakeConfig, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom) bool {
	if !g.positionInChunk(pos, chunkX, chunkZ, minY, maxY) || pos[1] <= minY+4 || pos[1] >= maxY-4 {
		return false
	}

	fluid, ok := g.selectState(c, cfg.Fluid, pos, rng, minY, maxY)
	if !ok {
		return false
	}
	barrier, barrierOK := g.selectState(c, cfg.Barrier, pos, rng, minY, maxY)
	fluidBlock, ok := g.featureBlockFromState(fluid, nil)
	if !ok {
		return false
	}
	fluidRID := runtimeIDForBlock(fluidBlock)

	radiusX := 2 + int(rng.NextInt(3))
	radiusZ := 2 + int(rng.NextInt(3))
	depth := 2 + int(rng.NextInt(2))
	var placedAny bool

	for dx := -radiusX - 1; dx <= radiusX+1; dx++ {
		for dz := -radiusZ - 1; dz <= radiusZ+1; dz++ {
			for dy := -depth - 1; dy <= 1; dy++ {
				candidate := pos.Add(cube.Pos{dx, dy, dz})
				if !g.positionInChunk(candidate, chunkX, chunkZ, minY, maxY) || g.lakeInterior(radiusX, radiusZ, depth, dx, dy, dz) {
					continue
				}
				if !g.lakeTouchesInterior(radiusX, radiusZ, depth, dx, dy, dz) {
					continue
				}

				rid := g.columnScanRuntimeID(c, candidate[0]-chunkX*16, candidate[1], candidate[2]-chunkZ*16)
				if dy > 0 {
					if rid == g.waterRID || rid == g.lavaRID {
						return false
					}
					continue
				}
				if !g.isSolidRID(rid) && rid != fluidRID {
					return false
				}
			}
		}
	}

	for dx := -radiusX; dx <= radiusX; dx++ {
		for dz := -radiusZ; dz <= radiusZ; dz++ {
			for dy := -depth; dy <= 1; dy++ {
				if !g.lakeInterior(radiusX, radiusZ, depth, dx, dy, dz) {
					continue
				}

				candidate := pos.Add(cube.Pos{dx, dy, dz})
				if !g.positionInChunk(candidate, chunkX, chunkZ, minY, maxY) {
					continue
				}

				if dy <= 0 {
					if g.setBlockStateDirect(c, candidate, fluid) {
						placedAny = true
					}
					continue
				}
				c.SetBlock(uint8(candidate[0]&15), int16(candidate[1]), uint8(candidate[2]&15), 0, g.airRID)
				c.SetBlock(uint8(candidate[0]&15), int16(candidate[1]), uint8(candidate[2]&15), 1, g.airRID)
				placedAny = true
			}
		}
	}

	if !placedAny || !barrierOK {
		return placedAny
	}

	for dx := -radiusX - 1; dx <= radiusX+1; dx++ {
		for dz := -radiusZ - 1; dz <= radiusZ+1; dz++ {
			for dy := -depth - 1; dy <= 0; dy++ {
				candidate := pos.Add(cube.Pos{dx, dy, dz})
				if !g.positionInChunk(candidate, chunkX, chunkZ, minY, maxY) {
					continue
				}
				if !g.lakeTouchesInterior(radiusX, radiusZ, depth, dx, dy, dz) || g.lakeInterior(radiusX, radiusZ, depth, dx, dy, dz) {
					continue
				}
				if dy > 0 && rng.NextInt(2) == 0 {
					continue
				}
				rid := g.columnScanRuntimeID(c, candidate[0]-chunkX*16, candidate[1], candidate[2]-chunkZ*16)
				if g.isSolidRID(rid) {
					_ = g.setBlockStateDirect(c, candidate, barrier)
				}
			}
		}
	}
	return placedAny
}

func (g Generator) lakeInterior(radiusX, radiusZ, depth, dx, dy, dz int) bool {
	nx := float64(dx) / float64(radiusX)
	ny := float64(dy) / float64(depth)
	nz := float64(dz) / float64(radiusZ)
	return nx*nx+ny*ny+nz*nz <= 1.0
}

func (g Generator) lakeTouchesInterior(radiusX, radiusZ, depth, dx, dy, dz int) bool {
	for _, off := range []cube.Pos{{1, 0, 0}, {-1, 0, 0}, {0, 1, 0}, {0, -1, 0}, {0, 0, 1}, {0, 0, -1}} {
		if g.lakeInterior(radiusX, radiusZ, depth, dx+off[0], dy+off[1], dz+off[2]) {
			return true
		}
	}
	return false
}

func (g Generator) executeFreezeTopLayer(c *chunk.Chunk, biomes sourceBiomeVolume, _ gen.FreezeTopLayerConfig, chunkX, chunkZ, minY, maxY int) bool {
	var placedAny bool
	for localX := 0; localX < 16; localX++ {
		for localZ := 0; localZ < 16; localZ++ {
			surfaceY := g.heightmapPlacementY(c, localX, localZ, "WORLD_SURFACE", minY, maxY) - 1
			if surfaceY < minY || surfaceY > maxY {
				continue
			}
			if !isFreezingBiomeKey(g.sourceBiomeKeyAt(biomes, localX, surfaceY, localZ)) {
				continue
			}

			top := cube.Pos{chunkX*16 + localX, surfaceY, chunkZ*16 + localZ}
			topRID := c.Block(uint8(localX), int16(surfaceY), uint8(localZ), 0)
			if topRID == g.waterRID {
				if g.setBlockStateDirect(c, top, gen.BlockState{Name: "ice"}) {
					placedAny = true
				}
				continue
			}
			if !g.isSolidRID(topRID) {
				continue
			}

			above := top.Side(cube.FaceUp)
			if above[1] > maxY {
				continue
			}
			if c.Block(uint8(above[0]&15), int16(above[1]), uint8(above[2]&15), 0) != g.airRID {
				continue
			}
			if g.setBlockStateDirect(c, above, gen.BlockState{Name: "snow"}) {
				placedAny = true
			}
		}
	}
	return placedAny
}

func (g Generator) executeFallenTree(c *chunk.Chunk, pos cube.Pos, cfg gen.FallenTreeConfig, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom) bool {
	trunkState, ok := g.selectState(c, cfg.TrunkProvider, pos, rng, minY, maxY)
	if !ok {
		return false
	}
	length := max(1, g.sampleIntProvider(cfg.LogLength, rng))

	validDirs := make([]cube.Pos, 0, 4)
	for _, dir := range []cube.Pos{{1, 0, 0}, {-1, 0, 0}, {0, 0, 1}, {0, 0, -1}} {
		end := pos.Add(cube.Pos{dir[0] * (length - 1), 0, dir[2] * (length - 1)})
		if g.positionInChunk(end, chunkX, chunkZ, minY, maxY) {
			validDirs = append(validDirs, dir)
		}
	}
	if len(validDirs) == 0 {
		return false
	}
	dir := validDirs[int(rng.NextInt(uint32(len(validDirs))))]
	if trunkState.Properties == nil {
		trunkState.Properties = make(map[string]string, 1)
	}
	if dir[0] != 0 {
		trunkState.Properties["axis"] = "x"
	} else {
		trunkState.Properties["axis"] = "z"
	}

	logBlock, ok := g.featureBlockFromState(trunkState, nil)
	if !ok {
		return false
	}

	var placedAny bool
	trunkPositions := make([]cube.Pos, 0, length)
	for i := 0; i < length; i++ {
		candidate := pos.Add(cube.Pos{dir[0] * i, 0, dir[2] * i})
		if !g.positionInChunk(candidate, chunkX, chunkZ, minY, maxY) {
			break
		}
		if !g.isSolidInChunk(c, candidate.Side(cube.FaceDown), chunkX, chunkZ, minY, maxY) {
			break
		}
		currentRID := c.Block(uint8(candidate[0]&15), int16(candidate[1]), uint8(candidate[2]&15), 0)
		currentBlock, _ := world.BlockByRuntimeID(currentRID)
		if !g.canReplaceFeatureBlock(currentBlock, logBlock) {
			break
		}
		if g.setBlockStateDirect(c, candidate, trunkState) {
			placedAny = true
			trunkPositions = append(trunkPositions, candidate)
		}
	}
	if placedAny {
		g.applyAttachedLogDecorators(c, trunkPositions, cfg.LogDecorators, rng, minY, maxY)
	}
	return placedAny
}

func (g Generator) executeTree(c *chunk.Chunk, pos cube.Pos, cfg gen.TreeConfig, minY, maxY int, rng *gen.WorldgenRandom) bool {
	return g.executeJavaTree(c, pos, cfg, minY, maxY, rng)
}

func (g Generator) executeBlockBlob(c *chunk.Chunk, pos cube.Pos, cfg gen.BlockBlobConfig, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom) bool {
	origin := pos
	for origin[1] > minY+3 {
		below := origin.Side(cube.FaceDown)
		if g.testBlockPredicate(c, below, cfg.CanPlaceOn, chunkX, chunkZ, minY, maxY, rng) {
			break
		}
		origin = below
	}
	if origin[1] <= minY+3 {
		return false
	}

	var placedAny bool
	for range 3 {
		xr := int(rng.NextInt(2))
		yr := int(rng.NextInt(2))
		zr := int(rng.NextInt(2))
		tr := float64(xr+yr+zr)*(1.0/3.0) + 0.5
		minPos := origin.Add(cube.Pos{-xr, -yr, -zr})
		maxPos := origin.Add(cube.Pos{xr, yr, zr})
		for x := minPos[0]; x <= maxPos[0]; x++ {
			for y := minPos[1]; y <= maxPos[1]; y++ {
				for z := minPos[2]; z <= maxPos[2]; z++ {
					candidate := cube.Pos{x, y, z}
					dx := float64(candidate[0] - origin[0])
					dy := float64(candidate[1] - origin[1])
					dz := float64(candidate[2] - origin[2])
					if dx*dx+dy*dy+dz*dz > tr*tr {
						continue
					}
					if !g.positionInChunk(candidate, chunkX, chunkZ, minY, maxY) {
						continue
					}
					if g.setBlockStateDirect(c, candidate, cfg.State) {
						placedAny = true
					}
				}
			}
		}
		origin = origin.Add(cube.Pos{
			-1 + int(rng.NextInt(2)),
			-int(rng.NextInt(2)),
			-1 + int(rng.NextInt(2)),
		})
	}
	return placedAny
}

func (g Generator) executeBamboo(c *chunk.Chunk, pos cube.Pos, cfg gen.BambooConfig, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom) bool {
	if !g.positionInChunk(pos, chunkX, chunkZ, minY, maxY) || pos[1] <= minY || pos[1] >= maxY {
		return false
	}
	if c.Block(uint8(pos[0]&15), int16(pos[1]), uint8(pos[2]&15), 0) != g.airRID {
		return false
	}
	belowRID := c.Block(uint8(pos[0]&15), int16(pos[1]-1), uint8(pos[2]&15), 0)
	belowBlock, _ := world.BlockByRuntimeID(belowRID)
	if !supportsBambooBlock(belowBlock) {
		return false
	}

	height := 5 + int(rng.NextInt(12))
	trunk := gen.BlockState{Name: "bamboo", Properties: map[string]string{
		"age_bit":                "false",
		"bamboo_leaf_size":       "no_leaves",
		"bamboo_stalk_thickness": "thick",
	}}
	var placedAny bool
	tipPos := pos
	for i := 0; i < height && tipPos[1] <= maxY; i++ {
		rid := c.Block(uint8(tipPos[0]&15), int16(tipPos[1]), uint8(tipPos[2]&15), 0)
		if rid != g.airRID {
			break
		}
		if g.setBlockStateDirect(c, tipPos, trunk) {
			placedAny = true
		}
		tipPos = tipPos.Side(cube.FaceUp)
	}
	if !placedAny {
		return false
	}
	placedHeight := tipPos[1] - pos[1]
	if placedHeight >= 3 && tipPos[1] <= maxY && c.Block(uint8(tipPos[0]&15), int16(tipPos[1]), uint8(tipPos[2]&15), 0) == g.airRID {
		_ = g.setBlockStateDirect(c, tipPos, gen.BlockState{Name: "bamboo", Properties: map[string]string{
			"age_bit":                "true",
			"bamboo_leaf_size":       "large_leaves",
			"bamboo_stalk_thickness": "thick",
		}})
		_ = g.setBlockStateDirect(c, tipPos.Side(cube.FaceDown), gen.BlockState{Name: "bamboo", Properties: map[string]string{
			"age_bit":                "false",
			"bamboo_leaf_size":       "large_leaves",
			"bamboo_stalk_thickness": "thick",
		}})
		_ = g.setBlockStateDirect(c, tipPos.Side(cube.FaceDown).Side(cube.FaceDown), gen.BlockState{Name: "bamboo", Properties: map[string]string{
			"age_bit":                "false",
			"bamboo_leaf_size":       "small_leaves",
			"bamboo_stalk_thickness": "thick",
		}})
	}

	if cfg.Probability <= 0 || rng.NextDouble() >= cfg.Probability {
		return true
	}

	radius := int(rng.NextInt(4)) + 1
	for x := pos[0] - radius; x <= pos[0]+radius; x++ {
		for z := pos[2] - radius; z <= pos[2]+radius; z++ {
			dx, dz := x-pos[0], z-pos[2]
			if dx*dx+dz*dz > radius*radius {
				continue
			}
			localX, localZ := x-chunkX*16, z-chunkZ*16
			if localX < 0 || localX > 15 || localZ < 0 || localZ > 15 {
				continue
			}
			surfaceY := g.heightmapPlacementY(c, localX, localZ, "WORLD_SURFACE", minY, maxY) - 1
			if surfaceY < minY || surfaceY > maxY {
				continue
			}
			surfacePos := cube.Pos{x, surfaceY, z}
			if !g.matchesFeatureBlockTag(g.blockNameAt(c, surfacePos), "beneath_bamboo_podzol_replaceable") {
				continue
			}
			_ = g.setBlockStateDirect(c, surfacePos, gen.BlockState{Name: "podzol", Properties: map[string]string{"snowy": "false"}})
		}
	}
	return true
}

func (g Generator) executeHugeBrownMushroom(c *chunk.Chunk, pos cube.Pos, cfg gen.HugeMushroomConfig, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom) bool {
	return g.executeHugeMushroom(c, pos, cfg, chunkX, chunkZ, minY, maxY, rng, false)
}

func (g Generator) executeHugeRedMushroom(c *chunk.Chunk, pos cube.Pos, cfg gen.HugeMushroomConfig, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom) bool {
	return g.executeHugeMushroom(c, pos, cfg, chunkX, chunkZ, minY, maxY, rng, true)
}

func (g Generator) executeMonsterRoom(c *chunk.Chunk, pos cube.Pos, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom) bool {
	xr := int(rng.NextInt(2)) + 2
	zr := int(rng.NextInt(2)) + 2
	minX, maxXRoom := -xr-1, xr+1
	minZ, maxZRoom := -zr-1, zr+1
	holeCount := 0

	for dx := minX; dx <= maxXRoom; dx++ {
		for dy := -1; dy <= 4; dy++ {
			for dz := minZ; dz <= maxZRoom; dz++ {
				cursor := pos.Add(cube.Pos{dx, dy, dz})
				if !g.positionInFeatureScope(cursor, chunkX, chunkZ, minY, maxY) {
					return false
				}
				solid := g.isSolidRID(g.runtimeIDAt(c, cursor))
				if dy == -1 && !solid {
					return false
				}
				if dy == 4 && !solid {
					return false
				}
				if (dx == minX || dx == maxXRoom || dz == minZ || dz == maxZRoom) && dy == 0 {
					above := cursor.Side(cube.FaceUp)
					if g.blockNameAt(c, cursor) == "air" && g.positionInFeatureScope(above, chunkX, chunkZ, minY, maxY) && g.blockNameAt(c, above) == "air" {
						holeCount++
					}
				}
			}
		}
	}
	if holeCount < 1 || holeCount > 5 {
		return false
	}

	protected := func(p cube.Pos) bool {
		return g.matchesFeatureBlockTag(g.blockNameAt(c, p), "features_cannot_replace")
	}

	for dx := minX; dx <= maxXRoom; dx++ {
		for dy := 3; dy >= -1; dy-- {
			for dz := minZ; dz <= maxZRoom; dz++ {
				cursor := pos.Add(cube.Pos{dx, dy, dz})
				name := g.blockNameAt(c, cursor)
				if dx == minX || dy == -1 || dz == minZ || dx == maxXRoom || dy == 4 || dz == maxZRoom {
					below := cursor.Side(cube.FaceDown)
					if cursor[1] >= minY && g.positionInFeatureScope(below, chunkX, chunkZ, minY, maxY) && !g.isSolidRID(g.runtimeIDAt(c, below)) {
						if !protected(cursor) {
							_ = g.setBlockStateDirect(c, cursor, gen.BlockState{Name: "air"})
						}
					} else if g.isSolidRID(g.runtimeIDAt(c, cursor)) && name != "chest" {
						if protected(cursor) {
							continue
						}
						if dy == -1 && rng.NextInt(4) != 0 {
							_ = g.setBlockStateDirect(c, cursor, gen.BlockState{Name: "mossy_cobblestone"})
						} else {
							_ = g.setBlockStateDirect(c, cursor, gen.BlockState{Name: "cobblestone"})
						}
					}
				} else if name != "chest" && name != "mob_spawner" && !protected(cursor) {
					_ = g.setBlockStateDirect(c, cursor, gen.BlockState{Name: "air"})
				}
			}
		}
	}

	for range 2 {
		for range 3 {
			xc := pos[0] + int(rng.NextInt(uint32(xr*2+1))) - xr
			zc := pos[2] + int(rng.NextInt(uint32(zr*2+1))) - zr
			chestPos := cube.Pos{xc, pos[1], zc}
			if !g.positionInFeatureScope(chestPos, chunkX, chunkZ, minY, maxY) || g.blockNameAt(c, chestPos) != "air" {
				continue
			}
			wallCount := 0
			for _, dir := range []cube.Pos{{1, 0, 0}, {-1, 0, 0}, {0, 0, 1}, {0, 0, -1}} {
				side := chestPos.Add(dir)
				if g.positionInFeatureScope(side, chunkX, chunkZ, minY, maxY) && g.isSolidRID(g.runtimeIDAt(c, side)) {
					wallCount++
				}
			}
			if wallCount == 1 {
				_ = g.setFeatureBlock(c, chestPos, block.NewChest())
				break
			}
		}
	}

	if !protected(pos) {
		_ = g.setBlockStateDirect(c, pos, gen.BlockState{Name: "mob_spawner"})
	}
	return true
}

func (g Generator) executeDesertWell(c *chunk.Chunk, pos cube.Pos, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom) bool {
	origin := pos.Side(cube.FaceUp)
	for g.blockNameAt(c, origin) == "air" && origin[1] > minY+2 {
		origin = origin.Side(cube.FaceDown)
	}
	if g.blockNameAt(c, origin) != "sand" {
		return false
	}
	for ox := -2; ox <= 2; ox++ {
		for oz := -2; oz <= 2; oz++ {
			below1 := origin.Add(cube.Pos{ox, -1, oz})
			below2 := origin.Add(cube.Pos{ox, -2, oz})
			if g.positionInFeatureScope(below1, chunkX, chunkZ, minY, maxY) && g.positionInFeatureScope(below2, chunkX, chunkZ, minY, maxY) &&
				g.blockNameAt(c, below1) == "air" && g.blockNameAt(c, below2) == "air" {
				return false
			}
		}
	}

	sandstone := gen.BlockState{Name: "sandstone"}
	sand := gen.BlockState{Name: "sand"}
	for oy := -2; oy <= 0; oy++ {
		for ox := -2; ox <= 2; ox++ {
			for oz := -2; oz <= 2; oz++ {
				_ = g.setBlockStateDirect(c, origin.Add(cube.Pos{ox, oy, oz}), sandstone)
			}
		}
	}
	stillWater := block.Water{Still: true, Depth: 8}
	_ = g.setFeatureBlock(c, origin, stillWater)
	for _, dir := range []cube.Pos{{1, 0, 0}, {-1, 0, 0}, {0, 0, 1}, {0, 0, -1}} {
		_ = g.setFeatureBlock(c, origin.Add(dir), stillWater)
	}
	sandCenter := origin.Side(cube.FaceDown)
	_ = g.setBlockStateDirect(c, sandCenter, sand)
	for _, dir := range []cube.Pos{{1, 0, 0}, {-1, 0, 0}, {0, 0, 1}, {0, 0, -1}} {
		_ = g.setBlockStateDirect(c, sandCenter.Add(dir), sand)
	}
	for ox := -2; ox <= 2; ox++ {
		for oz := -2; oz <= 2; oz++ {
			if ox == -2 || ox == 2 || oz == -2 || oz == 2 {
				_ = g.setBlockStateDirect(c, origin.Add(cube.Pos{ox, 1, oz}), sandstone)
			}
		}
	}
	slabBlock := block.Slab{Block: block.Sandstone{}}
	_ = g.setFeatureBlock(c, origin.Add(cube.Pos{2, 1, 0}), slabBlock)
	_ = g.setFeatureBlock(c, origin.Add(cube.Pos{-2, 1, 0}), slabBlock)
	_ = g.setFeatureBlock(c, origin.Add(cube.Pos{0, 1, 2}), slabBlock)
	_ = g.setFeatureBlock(c, origin.Add(cube.Pos{0, 1, -2}), slabBlock)
	for ox := -1; ox <= 1; ox++ {
		for oz := -1; oz <= 1; oz++ {
			if ox == 0 && oz == 0 {
				_ = g.setBlockStateDirect(c, origin.Add(cube.Pos{ox, 4, oz}), sandstone)
			} else {
				_ = g.setFeatureBlock(c, origin.Add(cube.Pos{ox, 4, oz}), slabBlock)
			}
		}
	}
	for oy := 1; oy <= 3; oy++ {
		_ = g.setBlockStateDirect(c, origin.Add(cube.Pos{-1, oy, -1}), sandstone)
		_ = g.setBlockStateDirect(c, origin.Add(cube.Pos{-1, oy, 1}), sandstone)
		_ = g.setBlockStateDirect(c, origin.Add(cube.Pos{1, oy, -1}), sandstone)
		_ = g.setBlockStateDirect(c, origin.Add(cube.Pos{1, oy, 1}), sandstone)
	}
	// Bedrock archaeology/block-entity parity is not implemented yet; keep fallback sand instead of suspicious sand.
	if rng != nil {
		waterPositions := []cube.Pos{
			origin,
			origin.Add(cube.Pos{1, 0, 0}),
			origin.Add(cube.Pos{-1, 0, 0}),
			origin.Add(cube.Pos{0, 0, 1}),
			origin.Add(cube.Pos{0, 0, -1}),
		}
		first := waterPositions[int(rng.NextInt(uint32(len(waterPositions))))]
		second := waterPositions[int(rng.NextInt(uint32(len(waterPositions))))]
		_ = g.setBlockStateDirect(c, first.Add(cube.Pos{0, -1, 0}), sand)
		_ = g.setBlockStateDirect(c, second.Add(cube.Pos{0, -2, 0}), sand)
	}
	return true
}

func (g Generator) executeHugeMushroom(c *chunk.Chunk, pos cube.Pos, cfg gen.HugeMushroomConfig, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom, red bool) bool {
	treeHeight := 4 + int(rng.NextInt(3))
	if rng.NextInt(12) == 0 {
		treeHeight *= 2
	}
	if pos[1] < minY+1 || pos[1]+treeHeight+1 > maxY+1 {
		return false
	}
	if !g.testBlockPredicate(c, pos.Side(cube.FaceDown), cfg.CanPlaceOn, chunkX, chunkZ, minY, maxY, rng) {
		return false
	}

	for dy := 0; dy <= treeHeight; dy++ {
		radius := hugeMushroomRadiusForHeight(treeHeight, cfg.FoliageRadius, dy, red)
		for dx := -radius; dx <= radius; dx++ {
			for dz := -radius; dz <= radius; dz++ {
				candidate := pos.Add(cube.Pos{dx, dy, dz})
				if !g.positionInChunk(candidate, chunkX, chunkZ, minY, maxY) {
					return false
				}
				name := g.blockNameAt(c, candidate)
				if name != "air" && !strings.HasSuffix(name, "_leaves") {
					return false
				}
			}
		}
	}

	stemState, ok := g.selectState(c, cfg.StemProvider, pos, rng, minY, maxY)
	if !ok {
		return false
	}
	capState, ok := g.selectState(c, cfg.CapProvider, pos, rng, minY, maxY)
	if !ok {
		return false
	}

	placedAny := false
	if red {
		placedAny = g.placeHugeRedMushroomCap(c, pos, treeHeight, cfg.FoliageRadius, capState, chunkX, chunkZ, minY, maxY) || placedAny
	} else {
		placedAny = g.placeHugeBrownMushroomCap(c, pos, treeHeight, cfg.FoliageRadius, capState, chunkX, chunkZ, minY, maxY) || placedAny
	}
	for dy := 0; dy < treeHeight; dy++ {
		if g.placeHugeMushroomState(c, pos.Add(cube.Pos{0, dy, 0}), stemState, chunkX, chunkZ, minY, maxY) {
			placedAny = true
		}
	}
	return placedAny
}

func hugeMushroomRadiusForHeight(treeHeight, leafRadius, y int, red bool) int {
	if red {
		if y < treeHeight && y >= treeHeight-3 {
			return leafRadius
		}
		if y == treeHeight {
			return leafRadius
		}
		return 0
	}
	if y <= 3 {
		return 0
	}
	return leafRadius
}

func (g Generator) placeHugeBrownMushroomCap(c *chunk.Chunk, pos cube.Pos, treeHeight, radius int, state gen.BlockState, chunkX, chunkZ, minY, maxY int) bool {
	var placedAny bool
	for dx := -radius; dx <= radius; dx++ {
		for dz := -radius; dz <= radius; dz++ {
			minX, maxX := dx == -radius, dx == radius
			minZ, maxZ := dz == -radius, dz == radius
			xEdge, zEdge := minX || maxX, minZ || maxZ
			if xEdge && zEdge {
				continue
			}
			candidate := pos.Add(cube.Pos{dx, treeHeight, dz})
			props := cloneStateProperties(state.Properties)
			west := minX || (zEdge && dx == 1-radius)
			east := maxX || (zEdge && dx == radius-1)
			north := minZ || (xEdge && dz == 1-radius)
			south := maxZ || (xEdge && dz == radius-1)
			props["west"] = strconv.FormatBool(west)
			props["east"] = strconv.FormatBool(east)
			props["north"] = strconv.FormatBool(north)
			props["south"] = strconv.FormatBool(south)
			if g.placeHugeMushroomState(c, candidate, gen.BlockState{Name: state.Name, Properties: props}, chunkX, chunkZ, minY, maxY) {
				placedAny = true
			}
		}
	}
	return placedAny
}

func (g Generator) placeHugeRedMushroomCap(c *chunk.Chunk, pos cube.Pos, treeHeight, radius int, state gen.BlockState, chunkX, chunkZ, minY, maxY int) bool {
	var placedAny bool
	for dy := treeHeight - 3; dy <= treeHeight; dy++ {
		layerRadius := radius
		if dy >= treeHeight {
			layerRadius--
		}
		center := radius - 2
		for dx := -layerRadius; dx <= layerRadius; dx++ {
			for dz := -layerRadius; dz <= layerRadius; dz++ {
				minX, maxX := dx == -layerRadius, dx == layerRadius
				minZ, maxZ := dz == -layerRadius, dz == layerRadius
				xEdge, zEdge := minX || maxX, minZ || maxZ
				if dy < treeHeight && xEdge == zEdge {
					continue
				}
				candidate := pos.Add(cube.Pos{dx, dy, dz})
				props := cloneStateProperties(state.Properties)
				props["up"] = strconv.FormatBool(dy >= treeHeight-1)
				props["west"] = strconv.FormatBool(dx < -center)
				props["east"] = strconv.FormatBool(dx > center)
				props["north"] = strconv.FormatBool(dz < -center)
				props["south"] = strconv.FormatBool(dz > center)
				if g.placeHugeMushroomState(c, candidate, gen.BlockState{Name: state.Name, Properties: props}, chunkX, chunkZ, minY, maxY) {
					placedAny = true
				}
			}
		}
	}
	return placedAny
}

func cloneStateProperties(props map[string]string) map[string]string {
	if len(props) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(props))
	for key, value := range props {
		out[key] = value
	}
	return out
}

func (g Generator) placeHugeMushroomState(c *chunk.Chunk, pos cube.Pos, state gen.BlockState, chunkX, chunkZ, minY, maxY int) bool {
	if !g.positionInChunk(pos, chunkX, chunkZ, minY, maxY) {
		return false
	}
	name := g.blockNameAt(c, pos)
	if name != "air" && !g.matchesFeatureBlockTag(name, "replaceable_by_mushrooms") {
		return false
	}
	return g.setBlockStateDirect(c, pos, state)
}

func (g Generator) executeVegetationPatch(c *chunk.Chunk, biomes sourceBiomeVolume, pos cube.Pos, cfg gen.VegetationPatchConfig, topFeatureName string, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom, depth int, waterlogged bool) bool {
	radius := max(1, g.sampleIntProvider(cfg.XZRadius, rng))
	patchDepth := max(1, g.sampleIntProvider(cfg.Depth, rng))
	var placedAny bool

	for dx := -radius; dx <= radius; dx++ {
		for dz := -radius; dz <= radius; dz++ {
			dist2 := dx*dx + dz*dz
			if dist2 > radius*radius {
				continue
			}
			if dist2 >= (radius-1)*(radius-1) && cfg.ExtraEdgeColumnChance > 0 && rng.NextDouble() > cfg.ExtraEdgeColumnChance {
				continue
			}

			basePos, plantPos, ok := g.findVegetationPatchSurface(c, pos.Add(cube.Pos{dx, 0, dz}), cfg.Surface, cfg.VerticalRange, chunkX, chunkZ, minY, maxY)
			if !ok || !g.matchesFeatureBlockTag(g.blockNameAt(c, basePos), cfg.Replaceable) {
				continue
			}

			groundState, ok := g.selectState(c, cfg.GroundState, basePos, rng, minY, maxY)
			if !ok {
				continue
			}
			for d := 0; d < patchDepth; d++ {
				target := basePos
				if strings.EqualFold(cfg.Surface, "ceiling") {
					target = target.Add(cube.Pos{0, d, 0})
				} else {
					target = target.Add(cube.Pos{0, -d, 0})
				}
				if !g.positionInChunk(target, chunkX, chunkZ, minY, maxY) || !g.matchesFeatureBlockTag(g.blockNameAt(c, target), cfg.Replaceable) {
					continue
				}
				if g.setBlockStateDirect(c, target, groundState) {
					placedAny = true
				}
			}

			if cfg.VegetationChance <= 0 || rng.NextDouble() >= cfg.VegetationChance {
				continue
			}
			if waterlogged && c.Block(uint8(plantPos[0]&15), int16(plantPos[1]), uint8(plantPos[2]&15), 0) == g.airRID {
				c.SetBlock(uint8(plantPos[0]&15), int16(plantPos[1]), uint8(plantPos[2]&15), 0, g.waterRID)
			}
			if g.executePlacedFeatureRef(c, biomes, plantPos, cfg.VegetationFeature, topFeatureName, chunkX, chunkZ, minY, maxY, rng, depth+1) {
				placedAny = true
			}
		}
	}
	return placedAny
}

func (g Generator) executeRootSystem(c *chunk.Chunk, biomes sourceBiomeVolume, pos cube.Pos, cfg gen.RootSystemConfig, topFeatureName string, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom, depth int) bool {
	if !g.positionInFeatureScope(pos, chunkX, chunkZ, minY, maxY) || !g.testBlockPredicate(c, pos, cfg.AllowedTreePosition, chunkX, chunkZ, minY, maxY, rng) {
		return false
	}

	waterBlocks := 0
	for y := 0; y < max(1, cfg.RequiredVerticalSpaceForTree); y++ {
		current := pos.Add(cube.Pos{0, y, 0})
		if !g.positionInChunk(current, chunkX, chunkZ, minY, maxY) {
			return false
		}
		rid := c.Block(uint8(current[0]&15), int16(current[1]), uint8(current[2]&15), 0)
		if rid == g.waterRID {
			waterBlocks++
			continue
		}
		if rid != g.airRID {
			return false
		}
	}
	if waterBlocks > cfg.AllowedVerticalWaterForTree {
		return false
	}
	if !g.executePlacedFeatureRef(c, biomes, pos, cfg.Feature, topFeatureName, chunkX, chunkZ, minY, maxY, rng, depth+1) {
		return false
	}

	rootState, rootStateOK := g.selectState(c, cfg.RootStateProvider, pos, rng, minY, maxY)
	hangingState, hangingStateOK := g.selectState(c, cfg.HangingRootStateProvider, pos, rng, minY, maxY)
	for i := 0; i < cfg.RootPlacementAttempts; i++ {
		candidate := pos.Add(cube.Pos{
			int(rng.NextInt(uint32(max(1, cfg.RootRadius*2+1)))) - cfg.RootRadius,
			-int(rng.NextInt(uint32(max(1, min(4, cfg.RootColumnMaxHeight)+1)))),
			int(rng.NextInt(uint32(max(1, cfg.RootRadius*2+1)))) - cfg.RootRadius,
		})
		if !g.positionInChunk(candidate, chunkX, chunkZ, minY, maxY) || !rootStateOK {
			continue
		}
		if g.matchesFeatureBlockTag(g.blockNameAt(c, candidate), cfg.RootReplaceable) {
			_ = g.setBlockStateDirect(c, candidate, rootState)
		}
	}
	for i := 0; i < cfg.HangingRootPlacementAttempts; i++ {
		candidate := pos.Add(cube.Pos{
			int(rng.NextInt(uint32(max(1, cfg.HangingRootRadius*2+1)))) - cfg.HangingRootRadius,
			-int(rng.NextInt(uint32(max(1, cfg.HangingRootsVerticalSpan+1)))),
			int(rng.NextInt(uint32(max(1, cfg.HangingRootRadius*2+1)))) - cfg.HangingRootRadius,
		})
		if !g.positionInChunk(candidate, chunkX, chunkZ, minY, maxY) || !hangingStateOK {
			continue
		}
		rid := c.Block(uint8(candidate[0]&15), int16(candidate[1]), uint8(candidate[2]&15), 0)
		if rid != g.airRID || !g.isSolidInChunk(c, candidate.Side(cube.FaceUp), chunkX, chunkZ, minY, maxY) {
			continue
		}
		_ = g.setBlockStateDirect(c, candidate, hangingState)
	}
	return true
}

func (g Generator) executeHugeFungus(c *chunk.Chunk, pos cube.Pos, cfg gen.HugeFungusConfig, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom) bool {
	if !g.positionInChunk(pos, chunkX, chunkZ, minY, maxY) || pos[1] <= minY+1 {
		return false
	}

	basePos := pos.Side(cube.FaceDown)
	if !g.positionInChunk(basePos, chunkX, chunkZ, minY, maxY) {
		return false
	}
	if g.blockNameAt(c, basePos) != normalizeFeatureStateName(cfg.ValidBaseBlock.Name) {
		return false
	}
	if !g.testBlockPredicate(c, pos, cfg.ReplaceableBlocks, chunkX, chunkZ, minY, maxY, rng) {
		return false
	}

	height := 5 + int(rng.NextInt(7))
	if cfg.Planted {
		height = max(4, height-2)
	}
	if pos[1]+height > maxY {
		height = maxY - pos[1]
	}
	if height <= 0 {
		return false
	}

	var placedAny bool
	for dy := 0; dy < height; dy++ {
		current := pos.Add(cube.Pos{0, dy, 0})
		if !g.positionInChunk(current, chunkX, chunkZ, minY, maxY) {
			continue
		}
		if dy != 0 && !g.testBlockPredicate(c, current, cfg.ReplaceableBlocks, chunkX, chunkZ, minY, maxY, rng) {
			break
		}
		if g.setBlockStateDirect(c, current, cfg.StemState) {
			placedAny = true
		}
	}

	topY := pos[1] + height - 1
	for y := topY - 3; y <= topY; y++ {
		if y < minY || y > maxY {
			continue
		}
		layer := topY - y
		radius := 2
		if layer == 3 {
			radius = 1
		}
		for dx := -radius; dx <= radius; dx++ {
			for dz := -radius; dz <= radius; dz++ {
				candidate := cube.Pos{pos[0] + dx, y, pos[2] + dz}
				if !g.positionInChunk(candidate, chunkX, chunkZ, minY, maxY) {
					continue
				}
				if dx*dx+dz*dz > radius*radius+1 {
					continue
				}
				if !g.testBlockPredicate(c, candidate, cfg.ReplaceableBlocks, chunkX, chunkZ, minY, maxY, rng) {
					continue
				}
				state := cfg.HatState
				if layer <= 1 && (abs(dx) == radius || abs(dz) == radius) && rng.NextDouble() < 0.2 {
					state = cfg.DecorState
				}
				if g.setBlockStateDirect(c, candidate, state) {
					placedAny = true
				}
			}
		}
	}
	return placedAny
}

func (g Generator) executeNetherForestVegetation(c *chunk.Chunk, pos cube.Pos, cfg gen.NetherForestVegetationConfig, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom) bool {
	attempts := max(16, cfg.SpreadWidth*cfg.SpreadWidth)
	var placedAny bool
	for i := 0; i < attempts; i++ {
		candidate := pos.Add(cube.Pos{
			int(rng.NextInt(uint32(max(1, cfg.SpreadWidth*2+1)))) - cfg.SpreadWidth,
			int(rng.NextInt(uint32(max(1, cfg.SpreadHeight*2+1)))) - cfg.SpreadHeight,
			int(rng.NextInt(uint32(max(1, cfg.SpreadWidth*2+1)))) - cfg.SpreadWidth,
		})
		if !g.positionInChunk(candidate, chunkX, chunkZ, minY, maxY) {
			continue
		}
		if c.Block(uint8(candidate[0]&15), int16(candidate[1]), uint8(candidate[2]&15), 0) != g.airRID {
			continue
		}
		if !supportsNetherFloraBlock(g.worldBlockAtChunkSafe(c, candidate.Side(cube.FaceDown), chunkX, chunkZ, minY, maxY)) {
			continue
		}
		if g.placeStateProviderBlock(c, candidate, cfg.StateProvider, rng, minY, maxY) {
			placedAny = true
		}
	}
	return placedAny
}

func (g Generator) executeTwistingVines(c *chunk.Chunk, pos cube.Pos, cfg gen.TwistingVinesConfig, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom) bool {
	attempts := max(16, cfg.SpreadWidth*cfg.SpreadWidth)
	var placedAny bool
	for i := 0; i < attempts; i++ {
		candidate := pos.Add(cube.Pos{
			int(rng.NextInt(uint32(max(1, cfg.SpreadWidth*2+1)))) - cfg.SpreadWidth,
			int(rng.NextInt(uint32(max(1, cfg.SpreadHeight*2+1)))) - cfg.SpreadHeight,
			int(rng.NextInt(uint32(max(1, cfg.SpreadWidth*2+1)))) - cfg.SpreadWidth,
		})
		if !g.positionInChunk(candidate, chunkX, chunkZ, minY, maxY) {
			continue
		}
		if c.Block(uint8(candidate[0]&15), int16(candidate[1]), uint8(candidate[2]&15), 0) != g.airRID {
			continue
		}
		if !supportsTwistingVinesBlock(g.worldBlockAtChunkSafe(c, candidate.Side(cube.FaceDown), chunkX, chunkZ, minY, maxY)) {
			continue
		}
		height := 1 + int(rng.NextInt(uint32(max(1, cfg.MaxHeight))))
		for dy := 0; dy < height && candidate[1]+dy <= maxY; dy++ {
			current := candidate.Add(cube.Pos{0, dy, 0})
			if !g.positionInChunk(current, chunkX, chunkZ, minY, maxY) || c.Block(uint8(current[0]&15), int16(current[1]), uint8(current[2]&15), 0) != g.airRID {
				break
			}
			if g.setBlockStateDirect(c, current, gen.BlockState{Name: "twisting_vines", Properties: map[string]string{"age": strconv.Itoa(int(rng.NextInt(26)))}}) {
				placedAny = true
			}
		}
	}
	return placedAny
}

func (g Generator) executeWeepingVines(c *chunk.Chunk, pos cube.Pos, _ gen.WeepingVinesConfig, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom) bool {
	var placedAny bool
	for i := 0; i < 32; i++ {
		candidate := pos.Add(cube.Pos{
			int(rng.NextInt(17)) - 8,
			int(rng.NextInt(9)) - 4,
			int(rng.NextInt(17)) - 8,
		})
		if !g.positionInChunk(candidate, chunkX, chunkZ, minY, maxY) {
			continue
		}
		if c.Block(uint8(candidate[0]&15), int16(candidate[1]), uint8(candidate[2]&15), 0) != g.airRID {
			continue
		}
		if !supportsWeepingVinesBlock(g.worldBlockAtChunkSafe(c, candidate.Side(cube.FaceUp), chunkX, chunkZ, minY, maxY)) {
			continue
		}
		height := 1 + int(rng.NextInt(8))
		for dy := 0; dy < height && candidate[1]-dy > minY; dy++ {
			current := candidate.Add(cube.Pos{0, -dy, 0})
			if !g.positionInChunk(current, chunkX, chunkZ, minY, maxY) || c.Block(uint8(current[0]&15), int16(current[1]), uint8(current[2]&15), 0) != g.airRID {
				break
			}
			if g.setBlockStateDirect(c, current, gen.BlockState{Name: "weeping_vines", Properties: map[string]string{"age": strconv.Itoa(int(rng.NextInt(26)))}}) {
				placedAny = true
			}
		}
	}
	return placedAny
}

func (g Generator) executeNetherrackReplaceBlobs(c *chunk.Chunk, pos cube.Pos, cfg gen.NetherrackReplaceBlobsConfig, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom) bool {
	radius := max(1, g.sampleIntProvider(cfg.Radius, rng))
	targetName := normalizeFeatureStateName(cfg.Target.Name)
	var placedAny bool
	for dx := -radius; dx <= radius; dx++ {
		for dy := -radius; dy <= radius; dy++ {
			for dz := -radius; dz <= radius; dz++ {
				if dx*dx+dy*dy+dz*dz > radius*radius {
					continue
				}
				candidate := pos.Add(cube.Pos{dx, dy, dz})
				if !g.positionInChunk(candidate, chunkX, chunkZ, minY, maxY) {
					continue
				}
				if g.blockNameAt(c, candidate) != targetName {
					continue
				}
				if g.setBlockStateDirect(c, candidate, cfg.State) {
					placedAny = true
				}
			}
		}
	}
	return placedAny
}

func (g Generator) executeGlowstoneBlob(c *chunk.Chunk, pos cube.Pos, _ gen.GlowstoneBlobConfig, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom) bool {
	if !g.positionInChunk(pos, chunkX, chunkZ, minY, maxY) || c.Block(uint8(pos[0]&15), int16(pos[1]), uint8(pos[2]&15), 0) != g.airRID {
		return false
	}
	aboveName := g.blockNameAtSafe(c, pos.Side(cube.FaceUp), chunkX, chunkZ, minY, maxY)
	if aboveName != "netherrack" && aboveName != "basalt" && aboveName != "blackstone" {
		return false
	}

	var placedAny bool
	for i := 0; i < 40; i++ {
		candidate := pos.Add(cube.Pos{
			int(rng.NextInt(9)) - 4,
			-int(rng.NextInt(6)),
			int(rng.NextInt(9)) - 4,
		})
		if !g.positionInChunk(candidate, chunkX, chunkZ, minY, maxY) {
			continue
		}
		if c.Block(uint8(candidate[0]&15), int16(candidate[1]), uint8(candidate[2]&15), 0) != g.airRID {
			continue
		}
		neighbors := 0
		for _, face := range cube.Faces() {
			if g.blockNameAtSafe(c, candidate.Side(face), chunkX, chunkZ, minY, maxY) == "glowstone" {
				neighbors++
			}
		}
		if candidate == pos || neighbors > 0 {
			if g.setBlockStateDirect(c, candidate, gen.BlockState{Name: "glowstone"}) {
				placedAny = true
			}
		}
	}
	return placedAny
}

func (g Generator) executeBasaltPillar(c *chunk.Chunk, pos cube.Pos, _ gen.BasaltPillarConfig, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom) bool {
	if !g.positionInChunk(pos, chunkX, chunkZ, minY, maxY) {
		return false
	}
	start := pos
	for start[1] < maxY && c.Block(uint8(start[0]&15), int16(start[1]), uint8(start[2]&15), 0) == g.airRID {
		start = start.Side(cube.FaceUp)
	}
	if !supportsBasaltAnchorBlock(g.worldBlockAtChunkSafe(c, start, chunkX, chunkZ, minY, maxY)) {
		return false
	}
	start = start.Side(cube.FaceDown)

	var placedAny bool
	height := 2 + int(rng.NextInt(8))
	for dy := 0; dy < height && start[1]-dy >= minY; dy++ {
		current := start.Add(cube.Pos{0, -dy, 0})
		if !g.positionInChunk(current, chunkX, chunkZ, minY, maxY) {
			continue
		}
		if c.Block(uint8(current[0]&15), int16(current[1]), uint8(current[2]&15), 0) != g.airRID {
			break
		}
		if g.setBlockStateDirect(c, current, gen.BlockState{Name: "basalt", Properties: map[string]string{"axis": "y"}}) {
			placedAny = true
		}
	}
	return placedAny
}

func (g Generator) executeBasaltColumns(c *chunk.Chunk, pos cube.Pos, cfg gen.BasaltColumnsConfig, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom) bool {
	reach := max(1, g.sampleIntProvider(cfg.Reach, rng))
	height := max(1, g.sampleIntProvider(cfg.Height, rng))
	var placedAny bool
	for dx := -reach; dx <= reach; dx++ {
		for dz := -reach; dz <= reach; dz++ {
			if dx*dx+dz*dz > reach*reach {
				continue
			}
			base := pos.Add(cube.Pos{dx, 0, dz})
			if !g.positionInChunk(base, chunkX, chunkZ, minY, maxY) {
				continue
			}
			for base[1] > minY && g.positionInChunk(base, chunkX, chunkZ, minY, maxY) && c.Block(uint8(base[0]&15), int16(base[1]), uint8(base[2]&15), 0) == g.airRID {
				base = base.Side(cube.FaceDown)
			}
			if !supportsBasaltAnchorBlock(g.worldBlockAtChunkSafe(c, base, chunkX, chunkZ, minY, maxY)) {
				continue
			}
			for dy := 1; dy <= height && base[1]+dy <= maxY; dy++ {
				current := base.Add(cube.Pos{0, dy, 0})
				if !g.positionInChunk(current, chunkX, chunkZ, minY, maxY) || c.Block(uint8(current[0]&15), int16(current[1]), uint8(current[2]&15), 0) != g.airRID {
					break
				}
				if g.setBlockStateDirect(c, current, gen.BlockState{Name: "basalt", Properties: map[string]string{"axis": "y"}}) {
					placedAny = true
				}
			}
		}
	}
	return placedAny
}

func (g Generator) executeDeltaFeature(c *chunk.Chunk, pos cube.Pos, cfg gen.DeltaFeatureConfig, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom) bool {
	size := max(2, g.sampleIntProvider(cfg.Size, rng))
	rim := max(0, g.sampleIntProvider(cfg.RimSize, rng))
	var placedAny bool
	for dx := -size - rim; dx <= size+rim; dx++ {
		for dz := -size - rim; dz <= size+rim; dz++ {
			candidate := pos.Add(cube.Pos{dx, 0, dz})
			if !g.positionInChunk(candidate, chunkX, chunkZ, minY, maxY) {
				continue
			}
			dist2 := dx*dx + dz*dz
			if dist2 > (size+rim)*(size+rim) {
				continue
			}
			state := cfg.Rim
			if dist2 <= size*size {
				state = cfg.Contents
			}
			if g.blockNameAt(c, candidate) == "air" || g.blockNameAt(c, candidate) == "lava" || g.blockNameAt(c, candidate) == "netherrack" || g.blockNameAt(c, candidate) == "magma" {
				if g.setBlockStateDirect(c, candidate, state) {
					placedAny = true
				}
			}
		}
	}
	return placedAny
}

func (g Generator) executeChorusPlant(c *chunk.Chunk, pos cube.Pos, _ gen.ChorusPlantConfig, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom) bool {
	if !g.positionInChunk(pos, chunkX, chunkZ, minY, maxY) || !supportsChorusBlock(g.worldBlockAtChunkSafe(c, pos.Side(cube.FaceDown), chunkX, chunkZ, minY, maxY)) {
		return false
	}
	height := 2 + int(rng.NextInt(4))
	var placedAny bool
	top := pos
	for dy := 0; dy < height && pos[1]+dy <= maxY; dy++ {
		current := pos.Add(cube.Pos{0, dy, 0})
		if c.Block(uint8(current[0]&15), int16(current[1]), uint8(current[2]&15), 0) != g.airRID {
			break
		}
		if g.setBlockStateDirect(c, current, gen.BlockState{Name: "chorus_plant"}) {
			placedAny = true
			top = current
		}
	}
	branchCount := 1 + int(rng.NextInt(4))
	for i := 0; i < branchCount; i++ {
		dir := []cube.Pos{{1, 0, 0}, {-1, 0, 0}, {0, 0, 1}, {0, 0, -1}}[int(rng.NextInt(4))]
		length := 1 + int(rng.NextInt(3))
		current := top
		for step := 0; step < length; step++ {
			current = current.Add(dir)
			if !g.positionInChunk(current, chunkX, chunkZ, minY, maxY) || c.Block(uint8(current[0]&15), int16(current[1]), uint8(current[2]&15), 0) != g.airRID {
				break
			}
			if g.setBlockStateDirect(c, current, gen.BlockState{Name: "chorus_plant"}) {
				placedAny = true
			}
		}
		flower := current.Side(cube.FaceUp)
		if g.positionInChunk(flower, chunkX, chunkZ, minY, maxY) && c.Block(uint8(flower[0]&15), int16(flower[1]), uint8(flower[2]&15), 0) == g.airRID {
			if g.setBlockStateDirect(c, flower, gen.BlockState{Name: "chorus_flower", Properties: map[string]string{"age": "0"}}) {
				placedAny = true
			}
		}
	}
	if !placedAny {
		return g.setBlockStateDirect(c, top.Side(cube.FaceUp), gen.BlockState{Name: "chorus_flower", Properties: map[string]string{"age": "0"}})
	}
	return true
}

func (g Generator) executeEndIsland(c *chunk.Chunk, pos cube.Pos, _ gen.EndIslandConfig, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom) bool {
	radius := 3 + int(rng.NextInt(4))
	var placedAny bool
	for layer := 0; radius > 0; layer++ {
		y := pos[1] - layer
		if y < minY || y > maxY {
			break
		}
		for dx := -radius; dx <= radius; dx++ {
			for dz := -radius; dz <= radius; dz++ {
				if dx*dx+dz*dz > radius*radius {
					continue
				}
				candidate := cube.Pos{pos[0] + dx, y, pos[2] + dz}
				if !g.positionInChunk(candidate, chunkX, chunkZ, minY, maxY) {
					continue
				}
				if g.setBlockStateDirect(c, candidate, gen.BlockState{Name: "end_stone"}) {
					placedAny = true
				}
			}
		}
		radius--
	}
	return placedAny
}

func checkerboardDistance(ax, az, bx, bz int) int {
	return max(abs(ax-bx), abs(az-bz))
}

func (g Generator) executeVoidStartPlatform(c *chunk.Chunk, pos cube.Pos, chunkX, chunkZ, minY, maxY int) bool {
	const (
		platformOffsetX = 8
		platformOffsetY = 3
		platformOffsetZ = 8
		platformRadius  = 16
	)
	platformChunkX := floorDiv(platformOffsetX, 16)
	platformChunkZ := floorDiv(platformOffsetZ, 16)
	if checkerboardDistance(chunkX, chunkZ, platformChunkX, platformChunkZ) > 1 {
		return true
	}
	y := pos[1] + platformOffsetY
	if y < minY || y > maxY {
		return false
	}
	platformOrigin := cube.Pos{platformOffsetX, y, platformOffsetZ}
	placedAny := false
	for z := chunkZ * 16; z <= chunkZ*16+15; z++ {
		for x := chunkX * 16; x <= chunkX*16+15; x++ {
			if checkerboardDistance(platformOrigin[0], platformOrigin[2], x, z) > platformRadius {
				continue
			}
			state := gen.BlockState{Name: "stone"}
			if x == platformOrigin[0] && z == platformOrigin[2] {
				state = gen.BlockState{Name: "cobblestone"}
			}
			if g.setBlockStateDirect(c, cube.Pos{x, y, z}, state) {
				placedAny = true
			}
		}
	}
	return placedAny
}

func (g Generator) executeIceberg(c *chunk.Chunk, pos cube.Pos, cfg gen.BlockStateFeatureConfig, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom) bool {
	origin := cube.Pos{pos[0], seaLevel, pos[2]}
	snowOnTop := rng.NextDouble() > 0.7
	shapeAngle := rng.NextDouble() * 2.0 * math.Pi
	shapeEllipseA := 11 - int(rng.NextInt(5))
	shapeEllipseC := 3 + int(rng.NextInt(3))
	isEllipse := rng.NextDouble() > 0.7
	overWaterHeight := 3 + int(rng.NextInt(15))
	if isEllipse {
		overWaterHeight = 6 + int(rng.NextInt(6))
	}
	if !isEllipse && rng.NextDouble() > 0.9 {
		overWaterHeight += 7 + int(rng.NextInt(19))
	}
	underWaterHeight := min(overWaterHeight+int(rng.NextInt(11)), 18)
	width := min(overWaterHeight+int(rng.NextInt(7))-int(rng.NextInt(5)), 11)
	a := 11
	if isEllipse {
		a = shapeEllipseA
	}

	placedAny := false
	for xo := -a; xo < a; xo++ {
		for zo := -a; zo < a; zo++ {
			for yOff := 0; yOff < overWaterHeight; yOff++ {
				radius := icebergHeightDependentRadiusRound(rng, yOff, overWaterHeight, width)
				if isEllipse {
					radius = icebergHeightDependentRadiusEllipse(yOff, overWaterHeight, width)
				}
				if isEllipse || xo < radius {
					if g.generateIcebergBlock(c, origin, overWaterHeight, xo, yOff, zo, radius, a, isEllipse, shapeEllipseC, shapeAngle, snowOnTop, cfg.State, chunkX, chunkZ, minY, maxY, rng) {
						placedAny = true
					}
				}
			}
		}
	}
	g.smoothIceberg(c, origin, width, overWaterHeight, isEllipse, shapeEllipseA, chunkX, chunkZ, minY, maxY)
	for xo := -a; xo < a; xo++ {
		for zo := -a; zo < a; zo++ {
			for yOff := -1; yOff > -underWaterHeight; yOff-- {
				newA := a
				if isEllipse {
					newA = int(math.Ceil(float64(a) * (1.0 - math.Pow(float64(yOff), 2.0)/(float64(underWaterHeight)*8.0))))
				}
				radius := icebergHeightDependentRadiusSteep(rng, -yOff, underWaterHeight, width)
				if xo < radius {
					if g.generateIcebergBlock(c, origin, underWaterHeight, xo, yOff, zo, radius, newA, isEllipse, shapeEllipseC, shapeAngle, snowOnTop, cfg.State, chunkX, chunkZ, minY, maxY, rng) {
						placedAny = true
					}
				}
			}
		}
	}
	return placedAny
}

func (g Generator) executeBlueIce(c *chunk.Chunk, pos cube.Pos, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom) bool {
	if pos[1] > seaLevel-1 || !g.positionInFeatureScope(pos, chunkX, chunkZ, minY, maxY) {
		return false
	}
	originName := g.blockNameAt(c, pos)
	belowName := "air"
	below := pos.Side(cube.FaceDown)
	if g.positionInFeatureScope(below, chunkX, chunkZ, minY, maxY) {
		belowName = g.blockNameAt(c, below)
	}
	if originName != "water" && belowName != "water" {
		return false
	}
	foundPackedIce := false
	for _, face := range cube.Faces() {
		if face == cube.FaceDown {
			continue
		}
		neighbor := pos.Side(face)
		if g.positionInFeatureScope(neighbor, chunkX, chunkZ, minY, maxY) && g.blockNameAt(c, neighbor) == "packed_ice" {
			foundPackedIce = true
			break
		}
	}
	if !foundPackedIce {
		return false
	}
	placedAny := g.setBlockStateDirect(c, pos, gen.BlockState{Name: "blue_ice"})
	for range 200 {
		yOff := int(rng.NextInt(5)) - int(rng.NextInt(6))
		xzDiff := 3
		if yOff < 2 {
			xzDiff += yOff / 2
		}
		if xzDiff < 1 {
			continue
		}
		placePos := pos.Add(cube.Pos{
			int(rng.NextInt(uint32(xzDiff))) - int(rng.NextInt(uint32(xzDiff))),
			yOff,
			int(rng.NextInt(uint32(xzDiff))) - int(rng.NextInt(uint32(xzDiff))),
		})
		if !g.positionInFeatureScope(placePos, chunkX, chunkZ, minY, maxY) {
			continue
		}
		placeName := g.blockNameAt(c, placePos)
		if placeName != "air" && placeName != "water" && placeName != "packed_ice" && placeName != "ice" {
			continue
		}
		for _, face := range cube.Faces() {
			neighbor := placePos.Side(face)
			if g.positionInFeatureScope(neighbor, chunkX, chunkZ, minY, maxY) && g.blockNameAt(c, neighbor) == "blue_ice" {
				if g.setBlockStateDirect(c, placePos, gen.BlockState{Name: "blue_ice"}) {
					placedAny = true
				}
				break
			}
		}
	}
	return placedAny
}

func icebergHeightDependentRadiusRound(rng *gen.WorldgenRandom, yOff, height, width int) int {
	k := 3.5 - rng.NextDouble()
	scale := (1.0 - math.Pow(float64(yOff), 2.0)/(float64(height)*k)) * float64(width)
	if height > 15+int(rng.NextInt(5)) {
		tempYOff := yOff
		if yOff < 3+int(rng.NextInt(6)) {
			tempYOff = yOff / 2
		}
		scale = (1.0 - float64(tempYOff)/(float64(height)*k*0.4)) * float64(width)
	}
	return int(math.Ceil(scale / 2.0))
}

func icebergHeightDependentRadiusEllipse(yOff, height, width int) int {
	scale := (1.0 - math.Pow(float64(yOff), 2.0)/float64(height)) * float64(width)
	return int(math.Ceil(scale / 2.0))
}

func icebergHeightDependentRadiusSteep(rng *gen.WorldgenRandom, yOff, height, width int) int {
	k := 1.0 + rng.NextDouble()/2.0
	scale := (1.0 - float64(yOff)/(float64(height)*k)) * float64(width)
	return int(math.Ceil(scale / 2.0))
}

func icebergSignedDistanceCircle(xo, zo int, radius int, rng *gen.WorldgenRandom) float64 {
	off := 10.0 * max(0.2, min(0.8, rng.NextDouble())) / float64(radius)
	return off + math.Pow(float64(xo), 2.0) + math.Pow(float64(zo), 2.0) - math.Pow(float64(radius), 2.0)
}

func icebergSignedDistanceEllipse(xo, zo int, a, c int, angle float64) float64 {
	if a == 0 || c == 0 {
		return 1
	}
	x := (float64(xo)*math.Cos(angle) - float64(zo)*math.Sin(angle)) / float64(a)
	z := (float64(xo)*math.Sin(angle) + float64(zo)*math.Cos(angle)) / float64(c)
	return x*x + z*z - 1.0
}

func icebergEllipseC(yOff, height, shapeEllipseC int) int {
	c := shapeEllipseC
	if yOff > 0 && height-yOff <= 3 {
		c = shapeEllipseC - (4 - (height - yOff))
	}
	return c
}

func (g Generator) generateIcebergBlock(c *chunk.Chunk, origin cube.Pos, height, xo, yOff, zo, radius, a int, isEllipse bool, shapeEllipseC int, shapeAngle float64, snowOnTop bool, mainState gen.BlockState, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom) bool {
	var signedDist float64
	if isEllipse {
		signedDist = icebergSignedDistanceEllipse(xo, zo, a, icebergEllipseC(yOff, height, shapeEllipseC), shapeAngle)
	} else {
		signedDist = icebergSignedDistanceCircle(xo, zo, radius, rng)
	}
	if signedDist >= 0 {
		return false
	}
	pos := origin.Add(cube.Pos{xo, yOff, zo})
	compareVal := -6.0 - float64(int(rng.NextInt(3)))
	if isEllipse {
		compareVal = -0.5
	}
	if signedDist > compareVal && rng.NextDouble() > 0.9 {
		return false
	}
	return g.setIcebergBlock(c, pos, height-yOff, height, isEllipse, snowOnTop, mainState, chunkX, chunkZ, minY, maxY, rng)
}

func (g Generator) setIcebergBlock(c *chunk.Chunk, pos cube.Pos, hDiff, height int, isEllipse, snowOnTop bool, mainState gen.BlockState, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom) bool {
	if !g.positionInFeatureScope(pos, chunkX, chunkZ, minY, maxY) {
		return false
	}
	name := g.blockNameAt(c, pos)
	if name != "air" && name != "snow" && name != "packed_ice" && name != "blue_ice" && name != "water" {
		return false
	}
	randomness := !isEllipse || rng.NextDouble() > 0.05
	divisor := 2
	if isEllipse {
		divisor = 3
	}
	if snowOnTop && name != "water" && hDiff <= int(rng.NextInt(uint32(max(1, height/divisor))))+int(float64(height)*0.6) && randomness {
		return g.setBlockStateDirect(c, pos, gen.BlockState{Name: "snow"})
	}
	return g.setBlockStateDirect(c, pos, mainState)
}

func (g Generator) smoothIceberg(c *chunk.Chunk, origin cube.Pos, width, height int, isEllipse bool, shapeEllipseA int, chunkX, chunkZ, minY, maxY int) {
	a := width / 2
	if isEllipse {
		a = shapeEllipseA
	}
	for x := -a; x <= a; x++ {
		for z := -a; z <= a; z++ {
			for yOff := 0; yOff <= height; yOff++ {
				pos := origin.Add(cube.Pos{x, yOff, z})
				if !g.positionInFeatureScope(pos, chunkX, chunkZ, minY, maxY) {
					continue
				}
				name := g.blockNameAt(c, pos)
				if name != "packed_ice" && name != "blue_ice" && name != "snow" {
					continue
				}
				below := pos.Side(cube.FaceDown)
				if g.positionInFeatureScope(below, chunkX, chunkZ, minY, maxY) && g.blockNameAt(c, below) == "air" {
					_ = g.setBlockStateDirect(c, pos, gen.BlockState{Name: "air"})
					above := pos.Side(cube.FaceUp)
					if g.positionInFeatureScope(above, chunkX, chunkZ, minY, maxY) {
						_ = g.setBlockStateDirect(c, above, gen.BlockState{Name: "air"})
					}
					continue
				}
				if name == "packed_ice" || name == "blue_ice" {
					counter := 0
					for _, dir := range []cube.Pos{{-1, 0, 0}, {1, 0, 0}, {0, 0, -1}, {0, 0, 1}} {
						side := pos.Add(dir)
						sideName := "air"
						if g.positionInFeatureScope(side, chunkX, chunkZ, minY, maxY) {
							sideName = g.blockNameAt(c, side)
						}
						if sideName != "packed_ice" && sideName != "blue_ice" && sideName != "snow" {
							counter++
						}
					}
					if counter >= 3 {
						_ = g.setBlockStateDirect(c, pos, gen.BlockState{Name: "air"})
					}
				}
			}
		}
	}
}

func (g Generator) executeSpike(c *chunk.Chunk, pos cube.Pos, cfg gen.SpikeConfig, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom) bool {
	origin := pos
	for g.blockNameAt(c, origin) == "air" && origin[1] > minY+2 {
		origin = origin.Side(cube.FaceDown)
	}
	if !g.testBlockPredicate(c, origin, cfg.CanPlaceOn, chunkX, chunkZ, minY, maxY, rng) {
		return false
	}

	origin = origin.Add(cube.Pos{0, int(rng.NextInt(4)), 0})
	height := 7 + int(rng.NextInt(4))
	width := height/4 + int(rng.NextInt(2))
	if width > 1 && rng.NextInt(60) == 0 {
		origin = origin.Add(cube.Pos{0, 10 + int(rng.NextInt(30)), 0})
	}

	var placedAny bool
	for yOff := 0; yOff < height; yOff++ {
		scale := (1.0 - float64(yOff)/float64(height)) * float64(width)
		newWidth := int(math.Ceil(scale))
		for xo := -newWidth; xo <= newWidth; xo++ {
			dx := math.Abs(float64(xo)) - 0.25
			for zo := -newWidth; zo <= newWidth; zo++ {
				dz := math.Abs(float64(zo)) - 0.25
				if xo != 0 || zo != 0 {
					if dx*dx+dz*dz > scale*scale {
						continue
					}
					if (xo == -newWidth || xo == newWidth || zo == -newWidth || zo == newWidth) && rng.NextDouble() > 0.75 {
						continue
					}
				}

				positive := origin.Add(cube.Pos{xo, yOff, zo})
				if g.positionInChunk(positive, chunkX, chunkZ, minY, maxY) {
					name := g.blockNameAt(c, positive)
					if name == "air" || g.testBlockPredicate(c, positive, cfg.CanReplace, chunkX, chunkZ, minY, maxY, rng) {
						if g.setBlockStateDirect(c, positive, cfg.State) {
							placedAny = true
						}
					}
				}
				if yOff != 0 && newWidth > 1 {
					negative := origin.Add(cube.Pos{xo, -yOff, zo})
					if g.positionInChunk(negative, chunkX, chunkZ, minY, maxY) {
						name := g.blockNameAt(c, negative)
						if name == "air" || g.testBlockPredicate(c, negative, cfg.CanReplace, chunkX, chunkZ, minY, maxY, rng) {
							if g.setBlockStateDirect(c, negative, cfg.State) {
								placedAny = true
							}
						}
					}
				}
			}
		}
	}

	pillarWidth := width - 1
	if pillarWidth < 0 {
		pillarWidth = 0
	} else if pillarWidth > 1 {
		pillarWidth = 1
	}
	for xo := -pillarWidth; xo <= pillarWidth; xo++ {
		for zo := -pillarWidth; zo <= pillarWidth; zo++ {
			cursor := origin.Add(cube.Pos{xo, -1, zo})
			runLength := 50
			if abs(xo) == 1 && abs(zo) == 1 {
				runLength = int(rng.NextInt(5))
			}
			for cursor[1] > 50 {
				if !g.positionInChunk(cursor, chunkX, chunkZ, minY, maxY) {
					cursor = cursor.Side(cube.FaceDown)
					runLength--
					if runLength <= 0 {
						cursor = cursor.Add(cube.Pos{0, -(int(rng.NextInt(5)) + 1), 0})
						runLength = int(rng.NextInt(5))
					}
					continue
				}
				name := g.blockNameAt(c, cursor)
				if name != "air" && !g.testBlockPredicate(c, cursor, cfg.CanReplace, chunkX, chunkZ, minY, maxY, rng) && name != normalizeFeatureStateName(cfg.State.Name) {
					break
				}
				if g.setBlockStateDirect(c, cursor, cfg.State) {
					placedAny = true
				}
				cursor = cursor.Side(cube.FaceDown)
				runLength--
				if runLength <= 0 {
					cursor = cursor.Add(cube.Pos{0, -(int(rng.NextInt(5)) + 1), 0})
					runLength = int(rng.NextInt(5))
				}
			}
		}
	}
	return placedAny
}

func (g Generator) executeEndSpike(c *chunk.Chunk, _ cube.Pos, _ gen.EndSpikeConfig, chunkX, chunkZ, minY, maxY int, _ *gen.WorldgenRandom) bool {
	var placedAny bool
	for _, spike := range endSpikesForSeed(g.seed) {
		if !spikeIntersectsChunk(spike, chunkX, chunkZ) {
			continue
		}
		for x := spike.X - spike.Radius; x <= spike.X+spike.Radius; x++ {
			for z := spike.Z - spike.Radius; z <= spike.Z+spike.Radius; z++ {
				if x>>4 != chunkX || z>>4 != chunkZ {
					continue
				}
				dx, dz := x-spike.X, z-spike.Z
				if dx*dx+dz*dz > spike.Radius*spike.Radius {
					continue
				}
				for y := max(minY, 45); y <= min(maxY, spike.Height); y++ {
					if g.setBlockStateDirect(c, cube.Pos{x, y, z}, gen.BlockState{Name: "obsidian"}) {
						placedAny = true
					}
				}
			}
		}
		top := cube.Pos{spike.X, spike.Height + 1, spike.Z}
		if g.positionInChunk(top, chunkX, chunkZ, minY, maxY) {
			_ = g.setBlockStateDirect(c, top.Side(cube.FaceDown), plainBedrockFeatureState())
			_ = g.setBlockStateDirect(c, top, gen.BlockState{Name: "fire", Properties: map[string]string{"age": "0"}})
			placedAny = true
		}
	}
	return placedAny
}

func (g Generator) executeEndPlatform(c *chunk.Chunk, pos cube.Pos, _ gen.EndPlatformConfig, chunkX, chunkZ, minY, maxY int) bool {
	var placedAny bool
	for x := pos[0] - 2; x <= pos[0]+2; x++ {
		for z := pos[2] - 2; z <= pos[2]+2; z++ {
			floor := cube.Pos{x, pos[1] - 1, z}
			if g.positionInChunk(floor, chunkX, chunkZ, minY, maxY) && g.setBlockStateDirect(c, floor, gen.BlockState{Name: "obsidian"}) {
				placedAny = true
			}
			for y := pos[1]; y <= min(maxY, pos[1]+3); y++ {
				current := cube.Pos{x, y, z}
				if !g.positionInChunk(current, chunkX, chunkZ, minY, maxY) {
					continue
				}
				c.SetBlock(uint8(current[0]&15), int16(current[1]), uint8(current[2]&15), 0, g.airRID)
				placedAny = true
			}
		}
	}
	return placedAny
}

func (g Generator) executeEndGateway(c *chunk.Chunk, pos cube.Pos, _ gen.EndGatewayConfig, chunkX, chunkZ, minY, maxY int) bool {
	if !g.positionInChunk(pos, chunkX, chunkZ, minY, maxY) {
		return false
	}
	var placedAny bool
	for _, offset := range []cube.Pos{{0, 0, 0}, {0, -1, 0}, {0, 1, 0}, {1, 0, 0}, {-1, 0, 0}, {0, 0, 1}, {0, 0, -1}} {
		current := pos.Add(offset)
		if !g.positionInChunk(current, chunkX, chunkZ, minY, maxY) {
			continue
		}
		state := plainBedrockFeatureState()
		if offset == (cube.Pos{}) {
			state = gen.BlockState{Name: "end_gateway"}
		}
		if g.setBlockStateDirect(c, current, state) {
			placedAny = true
		}
	}
	return placedAny
}

type endSpike struct {
	X      int
	Z      int
	Radius int
	Height int
}

func endSpikesForSeed(seed int64) []endSpike {
	rng := gen.NewWorldgenRandomXoroshiro(seed ^ 0x4f9939f508)
	out := make([]endSpike, 0, 10)
	for i := 0; i < 10; i++ {
		angle := float64(i) * (2 * math.Pi / 10)
		heightClass := int(rng.NextInt(10))
		out = append(out, endSpike{
			X:      int(math.Round(math.Cos(angle) * 42)),
			Z:      int(math.Round(math.Sin(angle) * 42)),
			Radius: 2 + heightClass/3,
			Height: 76 + heightClass*3,
		})
	}
	return out
}

func spikeIntersectsChunk(spike endSpike, chunkX, chunkZ int) bool {
	minX, maxX := chunkX*16, chunkX*16+15
	minZ, maxZ := chunkZ*16, chunkZ*16+15
	return spike.X+spike.Radius >= minX && spike.X-spike.Radius <= maxX &&
		spike.Z+spike.Radius >= minZ && spike.Z-spike.Radius <= maxZ
}

func (g Generator) findVegetationPatchSurface(c *chunk.Chunk, origin cube.Pos, surface string, verticalRange, chunkX, chunkZ, minY, maxY int) (cube.Pos, cube.Pos, bool) {
	for dy := verticalRange; dy >= -verticalRange; dy-- {
		candidate := origin.Add(cube.Pos{0, dy, 0})
		if !g.positionInChunk(candidate, chunkX, chunkZ, minY, maxY) {
			continue
		}
		if strings.EqualFold(surface, "ceiling") {
			if !g.isSolidInChunk(c, candidate, chunkX, chunkZ, minY, maxY) {
				continue
			}
			plantPos := candidate.Side(cube.FaceDown)
			rid := c.Block(uint8(plantPos[0]&15), int16(plantPos[1]), uint8(plantPos[2]&15), 0)
			if rid == g.airRID || rid == g.waterRID {
				return candidate, plantPos, true
			}
			continue
		}
		if !g.isSolidInChunk(c, candidate, chunkX, chunkZ, minY, maxY) {
			continue
		}
		plantPos := candidate.Side(cube.FaceUp)
		rid := c.Block(uint8(plantPos[0]&15), int16(plantPos[1]), uint8(plantPos[2]&15), 0)
		if rid == g.airRID || rid == g.waterRID {
			return candidate, plantPos, true
		}
	}
	return cube.Pos{}, cube.Pos{}, false
}

func (g Generator) collectTreeStructure(c *chunk.Chunk, origin, top cube.Pos, height int) ([]cube.Pos, []cube.Pos) {
	radius := max(6, height/2+4)
	minY := max(c.Range().Min(), origin[1]-2)
	maxY := min(c.Range().Max(), top[1]+4)
	minX := max(origin[0]-radius, origin[0]&^15)
	maxX := min(origin[0]+radius, (origin[0]&^15)+15)
	minZ := max(origin[2]-radius, origin[2]&^15)
	maxZ := min(origin[2]+radius, (origin[2]&^15)+15)

	trunks := make([]cube.Pos, 0, height+8)
	leaves := make([]cube.Pos, 0, height*6)
	for x := minX; x <= maxX; x++ {
		for z := minZ; z <= maxZ; z++ {
			for y := minY; y <= maxY; y++ {
				pos := cube.Pos{x, y, z}
				name := g.blockNameAt(c, pos)
				switch {
				case strings.HasSuffix(name, "_log"), strings.HasSuffix(name, "_wood"), strings.HasSuffix(name, "_stem"):
					trunks = append(trunks, pos)
				case strings.HasSuffix(name, "_leaves"):
					leaves = append(leaves, pos)
				}
			}
		}
	}
	return trunks, leaves
}

func (g Generator) applyTreeDecorators(c *chunk.Chunk, origin cube.Pos, trunkPositions, leafPositions []cube.Pos, decorators []gen.FeatureDecorator, rng *gen.WorldgenRandom, minY, maxY int) {
	if len(decorators) == 0 {
		return
	}

	for _, decorator := range decorators {
		switch decorator.Type {
		case "beehive":
			var cfg struct {
				Probability float64 `json:"probability"`
			}
			if err := json.Unmarshal(decorator.Data, &cfg); err == nil {
				g.placeBeeNestDecorator(c, trunkPositions, rng, cfg.Probability)
			}
		case "place_on_ground":
			var cfg struct {
				BlockStateProvider gen.StateProvider `json:"block_state_provider"`
				Height             int               `json:"height"`
				Radius             int               `json:"radius"`
				Tries              int               `json:"tries"`
			}
			if err := json.Unmarshal(decorator.Data, &cfg); err == nil {
				g.placeGroundDecorator(c, origin, cfg.BlockStateProvider, max(1, cfg.Height), max(1, cfg.Radius), max(1, cfg.Tries), rng, minY, maxY)
			}
		case "leave_vine":
			var cfg struct {
				Probability float64 `json:"probability"`
			}
			if err := json.Unmarshal(decorator.Data, &cfg); err == nil {
				g.placeLeafVines(c, leafPositions, rng, cfg.Probability, minY)
			}
		case "trunk_vine":
			g.placeTrunkVines(c, trunkPositions, rng, minY)
		case "attached_to_leaves":
			var cfg struct {
				BlockProvider       gen.StateProvider `json:"block_provider"`
				Directions          []string          `json:"directions"`
				Probability         float64           `json:"probability"`
				RequiredEmptyBlocks int               `json:"required_empty_blocks"`
			}
			if err := json.Unmarshal(decorator.Data, &cfg); err == nil {
				g.placeAttachedToLeaves(c, leafPositions, cfg.BlockProvider, cfg.Directions, cfg.Probability, max(1, cfg.RequiredEmptyBlocks), rng, minY, maxY)
			}
		case "alter_ground":
			var cfg struct {
				Provider gen.StateProvider `json:"provider"`
			}
			if err := json.Unmarshal(decorator.Data, &cfg); err == nil {
				g.alterGroundAroundTree(c, origin, cfg.Provider, rng, minY, maxY)
			}
		}
	}
}

func (g Generator) applyAttachedLogDecorators(c *chunk.Chunk, logPositions []cube.Pos, decorators []gen.FeatureDecorator, rng *gen.WorldgenRandom, minY, maxY int) {
	for _, decorator := range decorators {
		if decorator.Type != "attached_to_logs" {
			continue
		}
		var cfg struct {
			BlockProvider gen.StateProvider `json:"block_provider"`
			Directions    []string          `json:"directions"`
			Probability   float64           `json:"probability"`
		}
		if err := json.Unmarshal(decorator.Data, &cfg); err != nil {
			continue
		}
		for _, logPos := range logPositions {
			for _, direction := range cfg.Directions {
				if cfg.Probability > 0 && rng.NextDouble() >= cfg.Probability {
					continue
				}
				offset := blockColumnDirection(direction)
				candidate := logPos.Add(offset)
				if candidate[1] <= minY || candidate[1] > maxY {
					continue
				}
				if c.Block(uint8(candidate[0]&15), int16(candidate[1]), uint8(candidate[2]&15), 0) != g.airRID {
					continue
				}
				_ = g.placeStateProviderBlock(c, candidate, cfg.BlockProvider, rng, minY, maxY)
			}
		}
	}
}

func (g Generator) placeBeeNestDecorator(c *chunk.Chunk, trunkPositions []cube.Pos, rng *gen.WorldgenRandom, probability float64) {
	if len(trunkPositions) == 0 || probability <= 0 || rng.NextDouble() >= probability {
		return
	}
	target := trunkPositions[len(trunkPositions)*2/3]
	for _, dir := range []cube.Pos{{1, 0, 0}, {-1, 0, 0}, {0, 0, 1}, {0, 0, -1}} {
		candidate := target.Add(dir)
		if c.Block(uint8(candidate[0]&15), int16(candidate[1]), uint8(candidate[2]&15), 0) != g.airRID {
			continue
		}
		beeNest, ok := world.BlockByName("minecraft:bee_nest", map[string]any{"direction": int32(2), "honey_level": int32(0)})
		if !ok {
			return
		}
		c.SetBlock(uint8(candidate[0]&15), int16(candidate[1]), uint8(candidate[2]&15), 0, runtimeIDForBlock(beeNest))
		return
	}
}

func (g Generator) placeGroundDecorator(c *chunk.Chunk, origin cube.Pos, provider gen.StateProvider, height, radius, tries int, rng *gen.WorldgenRandom, minY, maxY int) {
	for i := 0; i < tries; i++ {
		x := origin[0] + int(rng.NextInt(uint32(radius*2+1))) - radius
		z := origin[2] + int(rng.NextInt(uint32(radius*2+1))) - radius
		for y := min(maxY, origin[1]+height); y >= max(minY, origin[1]-height); y-- {
			ground := cube.Pos{x, y, z}
			above := ground.Side(cube.FaceUp)
			if !g.isSolidRID(c.Block(uint8(ground[0]&15), int16(ground[1]), uint8(ground[2]&15), 0)) {
				continue
			}
			if c.Block(uint8(above[0]&15), int16(above[1]), uint8(above[2]&15), 0) != g.airRID {
				break
			}
			_ = g.placeStateProviderBlock(c, above, provider, rng, minY, maxY)
			break
		}
	}
}

func (g Generator) placeLeafVines(c *chunk.Chunk, leafPositions []cube.Pos, rng *gen.WorldgenRandom, probability float64, minY int) {
	if probability <= 0 {
		return
	}
	for _, leafPos := range leafPositions {
		for _, face := range cube.HorizontalFaces() {
			if rng.NextDouble() >= probability {
				continue
			}
			candidate := leafPos.Side(face)
			if c.Block(uint8(candidate[0]&15), int16(candidate[1]), uint8(candidate[2]&15), 0) != g.airRID {
				continue
			}
			g.placeVineColumn(c, candidate, face, minY, 3+int(rng.NextInt(3)))
		}
	}
}

func (g Generator) placeTrunkVines(c *chunk.Chunk, trunkPositions []cube.Pos, rng *gen.WorldgenRandom, minY int) {
	for _, trunkPos := range trunkPositions {
		for _, face := range cube.HorizontalFaces() {
			if rng.NextDouble() >= 0.15 {
				continue
			}
			candidate := trunkPos.Side(face)
			if c.Block(uint8(candidate[0]&15), int16(candidate[1]), uint8(candidate[2]&15), 0) != g.airRID {
				continue
			}
			g.placeVineColumn(c, candidate, face, minY, 1+int(rng.NextInt(2)))
		}
	}
}

func (g Generator) placeVineColumn(c *chunk.Chunk, pos cube.Pos, supportFace cube.Face, minY, length int) {
	vine := block.Vines{}
	switch supportFace {
	case cube.FaceNorth:
		vine = vine.WithAttachment(cube.South, true)
	case cube.FaceSouth:
		vine = vine.WithAttachment(cube.North, true)
	case cube.FaceEast:
		vine = vine.WithAttachment(cube.West, true)
	case cube.FaceWest:
		vine = vine.WithAttachment(cube.East, true)
	default:
		return
	}
	for i := 0; i < length && pos[1]-i > minY; i++ {
		current := pos.Add(cube.Pos{0, -i, 0})
		if c.Block(uint8(current[0]&15), int16(current[1]), uint8(current[2]&15), 0) != g.airRID {
			break
		}
		c.SetBlock(uint8(current[0]&15), int16(current[1]), uint8(current[2]&15), 0, runtimeIDForBlock(vine))
	}
}

func (g Generator) placeAttachedToLeaves(c *chunk.Chunk, leafPositions []cube.Pos, provider gen.StateProvider, directions []string, probability float64, requiredEmptyBlocks int, rng *gen.WorldgenRandom, minY, maxY int) {
	for _, leafPos := range leafPositions {
		for _, direction := range directions {
			if probability > 0 && rng.NextDouble() >= probability {
				continue
			}
			offset := blockColumnDirection(direction)
			candidate := leafPos.Add(offset)
			if candidate[1] <= minY || candidate[1] > maxY {
				continue
			}
			empty := true
			for i := 0; i < requiredEmptyBlocks; i++ {
				check := candidate.Add(cube.Pos{0, -i, 0})
				rid := c.Block(uint8(check[0]&15), int16(check[1]), uint8(check[2]&15), 0)
				if rid != g.airRID {
					empty = false
					break
				}
			}
			if !empty {
				continue
			}
			state, ok := g.selectState(c, provider, candidate, rng, minY, maxY)
			if !ok {
				continue
			}
			_ = g.setBlockStateDirect(c, candidate, state)
		}
	}
}

func (g Generator) alterGroundAroundTree(c *chunk.Chunk, origin cube.Pos, provider gen.StateProvider, rng *gen.WorldgenRandom, minY, maxY int) {
	for dx := -2; dx <= 2; dx++ {
		for dz := -2; dz <= 2; dz++ {
			x := origin[0] + dx
			z := origin[2] + dz
			localX := x & 15
			localZ := z & 15
			surfaceY := g.heightmapPlacementY(c, localX, localZ, "WORLD_SURFACE", minY, maxY) - 1
			if surfaceY < minY || surfaceY > maxY {
				continue
			}
			surface := cube.Pos{x, surfaceY, z}
			name := g.blockNameAt(c, surface)
			if name != "grass" && name != "dirt" && name != "podzol" && name != "coarse_dirt" {
				continue
			}
			_ = g.placeStateProviderBlock(c, surface, provider, rng, minY, maxY)
		}
	}
}

func (g Generator) applyPlacementModifiers(c *chunk.Chunk, biomes sourceBiomeVolume, positions []cube.Pos, modifiers []gen.PlacementModifier, topFeatureName string, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom) ([]cube.Pos, bool) {
	out := slices.Clone(positions)

	for _, modifier := range modifiers {
		switch modifier.Type {
		case "count":
			cfg, err := modifier.Count()
			if err != nil {
				return nil, false
			}
			next := make([]cube.Pos, 0, len(out))
			for _, pos := range out {
				for i := 0; i < g.sampleIntProvider(cfg.Count, rng); i++ {
					next = append(next, pos)
				}
			}
			out = next
		case "count_on_every_layer":
			cfg, err := modifier.CountOnEveryLayer()
			if err != nil {
				return nil, false
			}
			next := make([]cube.Pos, 0, len(out))
			for _, pos := range out {
				for layer := 0; ; layer++ {
					foundAny := false
					attempts := g.sampleIntProvider(cfg.Count, rng)
					for i := 0; i < attempts; i++ {
						localX := int(rng.NextInt(16))
						localZ := int(rng.NextInt(16))
						y, ok := g.findCountOnEveryLayerY(c, localX, localZ, layer, minY, maxY)
						if !ok {
							continue
						}
						next = append(next, cube.Pos{pos[0] + localX, y, pos[2] + localZ})
						foundAny = true
					}
					if !foundAny {
						break
					}
				}
			}
			out = next
		case "noise_threshold_count":
			cfg, err := modifier.NoiseThresholdCount()
			if err != nil {
				return nil, false
			}
			next := make([]cube.Pos, 0, len(out))
			for _, pos := range out {
				count := cfg.BelowNoise
				if g.featureCountNoise(pos[0], pos[2]) > cfg.NoiseLevel {
					count = cfg.AboveNoise
				}
				for i := 0; i < count; i++ {
					next = append(next, pos)
				}
			}
			out = next
		case "noise_based_count":
			cfg, err := modifier.NoiseBasedCount()
			if err != nil {
				return nil, false
			}
			next := make([]cube.Pos, 0, len(out))
			for _, pos := range out {
				count := g.sampleNoiseBasedCount(cfg, pos)
				for i := 0; i < count; i++ {
					next = append(next, pos)
				}
			}
			out = next
		case "rarity_filter":
			cfg, err := modifier.RarityFilter()
			if err != nil {
				return nil, false
			}
			next := make([]cube.Pos, 0, len(out))
			for _, pos := range out {
				if cfg.Chance <= 1 || rng.NextInt(uint32(cfg.Chance)) == 0 {
					next = append(next, pos)
				}
			}
			out = next
		case "in_square":
			for i, pos := range out {
				pos[0] = chunkX*16 + int(rng.NextInt(16))
				pos[2] = chunkZ*16 + int(rng.NextInt(16))
				out[i] = pos
			}
		case "height_range":
			cfg, err := modifier.HeightRange()
			if err != nil {
				return nil, false
			}
			for i, pos := range out {
				pos[1] = g.sampleHeightProvider(cfg.Height, minY, maxY, rng)
				out[i] = pos
			}
		case "heightmap":
			cfg, err := modifier.Heightmap()
			if err != nil {
				return nil, false
			}
			for i, pos := range out {
				localX := pos[0] - chunkX*16
				localZ := pos[2] - chunkZ*16
				pos[1] = g.heightmapPlacementY(c, localX, localZ, cfg.Heightmap, minY, maxY)
				out[i] = pos
			}
		case "surface_water_depth_filter":
			cfg, err := modifier.SurfaceWaterDepthFilter()
			if err != nil {
				return nil, false
			}
			next := make([]cube.Pos, 0, len(out))
			for _, pos := range out {
				if g.surfaceWaterDepthAt(c, pos[0]-chunkX*16, pos[2]-chunkZ*16, minY) <= cfg.MaxWaterDepth {
					next = append(next, pos)
				}
			}
			out = next
		case "biome":
			next := make([]cube.Pos, 0, len(out))
			for _, pos := range out {
				if g.biomeGeneration.biomeHasFeature(g.zoomedBiomeAt(pos[0], pos[1], pos[2]), topFeatureName) {
					next = append(next, pos)
				}
			}
			out = next
		case "random_offset":
			cfg, err := modifier.RandomOffset()
			if err != nil {
				return nil, false
			}
			for i, pos := range out {
				pos[0] += g.sampleIntProvider(cfg.XZSpread, rng)
				pos[1] += g.sampleIntProvider(cfg.YSpread, rng)
				pos[2] += g.sampleIntProvider(cfg.XZSpread, rng)
				out[i] = pos
			}
		case "fixed_placement":
			cfg, err := modifier.FixedPlacement()
			if err != nil {
				return nil, false
			}
			next := make([]cube.Pos, 0, len(out)*max(1, len(cfg.Positions)))
			for range out {
				for _, fixed := range cfg.Positions {
					next = append(next, cube.Pos(fixed))
				}
			}
			out = next
		case "environment_scan":
			cfg, err := modifier.EnvironmentScan()
			if err != nil {
				return nil, false
			}
			next := make([]cube.Pos, 0, len(out))
			for _, pos := range out {
				if scanned, ok := g.scanEnvironment(c, pos, cfg, chunkX, chunkZ, minY, maxY, rng); ok {
					next = append(next, scanned)
				}
			}
			out = next
		case "surface_relative_threshold_filter":
			cfg, err := modifier.SurfaceRelativeThresholdFilter()
			if err != nil {
				return nil, false
			}
			next := make([]cube.Pos, 0, len(out))
			for _, pos := range out {
				surfaceY := g.heightmapPlacementY(c, pos[0]-chunkX*16, pos[2]-chunkZ*16, cfg.Heightmap, minY, maxY)
				delta := pos[1] - surfaceY
				if cfg.MinInclusive != nil && delta < *cfg.MinInclusive {
					continue
				}
				if cfg.MaxInclusive != nil && delta > *cfg.MaxInclusive {
					continue
				}
				next = append(next, pos)
			}
			out = next
		case "block_predicate_filter":
			cfg, err := modifier.BlockPredicateFilter()
			if err != nil {
				return nil, false
			}
			next := make([]cube.Pos, 0, len(out))
			for _, pos := range out {
				if g.testBlockPredicate(c, pos, cfg.Predicate, chunkX, chunkZ, minY, maxY, rng) {
					next = append(next, pos)
				}
			}
			out = next
		default:
			return nil, false
		}

		if len(out) == 0 {
			return nil, true
		}
	}
	return out, true
}

func (g Generator) findCountOnEveryLayerY(c *chunk.Chunk, localX, localZ, layer, minY, maxY int) (int, bool) {
	if localX < 0 || localX >= 16 || localZ < 0 || localZ >= 16 {
		return 0, false
	}
	startY := clamp(g.heightmapPlacementY(c, localX, localZ, "MOTION_BLOCKING", minY, maxY), minY, maxY)
	currentRID := g.columnScanRuntimeID(c, localX, startY, localZ)
	currentLayer := 0

	for y := startY; y >= minY+1; y-- {
		belowRID := g.columnScanRuntimeID(c, localX, y-1, localZ)
		if !g.countOnEveryLayerEmptyRID(belowRID) && g.countOnEveryLayerEmptyRID(currentRID) && belowRID != g.bedrockRID {
			if currentLayer == layer {
				return y, true
			}
			currentLayer++
		}
		currentRID = belowRID
	}
	return 0, false
}

func (g Generator) countOnEveryLayerEmptyRID(rid uint32) bool {
	return rid == g.airRID || rid == g.waterRID || rid == g.lavaRID
}

func (g Generator) placeStateProviderBlock(c *chunk.Chunk, pos cube.Pos, provider gen.StateProvider, rng *gen.WorldgenRandom, minY, maxY int) bool {
	state, ok := g.selectState(c, provider, pos, rng, minY, maxY)
	if !ok {
		return false
	}
	return g.placeFeatureState(c, pos, state, rng, minY, maxY)
}

func (g Generator) selectState(c *chunk.Chunk, provider gen.StateProvider, pos cube.Pos, rng *gen.WorldgenRandom, minY, maxY int) (gen.BlockState, bool) {
	c = g.chunkForActiveTreePos(c, pos)
	switch provider.Type {
	case "simple_state_provider":
		cfg, err := provider.SimpleState()
		if err != nil {
			return gen.BlockState{}, false
		}
		return cfg.State, true
	case "weighted_state_provider":
		cfg, err := provider.WeightedState()
		if err != nil || len(cfg.Entries) == 0 {
			return gen.BlockState{}, false
		}
		total := 0
		for _, entry := range cfg.Entries {
			total += entry.Weight
		}
		if total <= 0 {
			return gen.BlockState{}, false
		}
		pick := int(rng.NextInt(uint32(total)))
		for _, entry := range cfg.Entries {
			pick -= entry.Weight
			if pick < 0 {
				return entry.Data, true
			}
		}
		return cfg.Entries[len(cfg.Entries)-1].Data, true
	case "randomized_int_state_provider":
		cfg, err := provider.RandomizedIntState()
		if err != nil {
			return gen.BlockState{}, false
		}
		state, ok := g.selectState(c, cfg.Source, pos, rng, minY, maxY)
		if !ok {
			return gen.BlockState{}, false
		}
		if state.Properties == nil {
			state.Properties = make(map[string]string, 1)
		}
		state.Properties[cfg.Property] = strconv.Itoa(g.sampleIntProvider(cfg.Values, rng))
		return state, true
	case "rule_based_state_provider":
		cfg, err := provider.RuleBasedState()
		if err != nil {
			return gen.BlockState{}, false
		}
		for _, rule := range cfg.Rules {
			if g.testBlockPredicate(c, pos, rule.IfTrue, pos[0]>>4, pos[2]>>4, minY, maxY, rng) {
				return g.selectState(c, rule.Then, pos, rng, minY, maxY)
			}
		}
		return g.selectState(c, cfg.Fallback, pos, rng, minY, maxY)
	case "noise_threshold_provider":
		cfg, err := provider.NoiseThreshold()
		if err != nil {
			return gen.BlockState{}, false
		}
		value := g.noiseThresholdProviderValue(provider, cfg, pos)
		if value < cfg.Threshold && len(cfg.LowStates) > 0 {
			return cfg.LowStates[int(rng.NextInt(uint32(len(cfg.LowStates))))], true
		}
		if len(cfg.HighStates) > 0 && rng.NextDouble() < cfg.HighChance {
			return cfg.HighStates[int(rng.NextInt(uint32(len(cfg.HighStates))))], true
		}
		return cfg.DefaultState, true
	default:
		return gen.BlockState{}, false
	}
}

func (g Generator) placeFeatureState(c *chunk.Chunk, pos cube.Pos, state gen.BlockState, rng *gen.WorldgenRandom, minY, maxY int) bool {
	featureBlock, ok := g.featureBlockFromState(state, rng)
	if !ok || pos[1] <= minY || pos[1] > maxY {
		return false
	}

	localX := uint8(pos[0] & 15)
	localZ := uint8(pos[2] & 15)
	currentRID := c.Block(localX, int16(pos[1]), localZ, 0)
	currentBlock, _ := world.BlockByRuntimeID(currentRID)
	if !g.canReplaceFeatureBlock(currentBlock, featureBlock) {
		return false
	}

	if !g.canFeatureBlockSurvive(c, pos, featureBlock, state.Name, minY, maxY) {
		return false
	}

	var upperPos cube.Pos
	var upperFeatureBlock world.Block
	var upperState gen.BlockState
	var hasUpperFeatureBlock bool
	var hasUpperState bool
	switch featureBlock := featureBlock.(type) {
	case block.DoubleTallGrass:
		if !featureBlock.UpperPart {
			hasUpperFeatureBlock = true
			upperPos = pos.Side(cube.FaceUp)
			upperFeatureBlock = block.DoubleTallGrass{Type: featureBlock.Type, UpperPart: true}
		}
	}
	switch state.Name {
	case "small_dripleaf", "small_dripleaf_block":
		if state.Properties["half"] != "upper" && !parseStateBool(state.Properties, "upper_block_bit") {
			hasUpperState = true
			upperPos = pos.Side(cube.FaceUp)
			upperState = state
			upperState.Properties = maps.Clone(state.Properties)
			if upperState.Properties == nil {
				upperState.Properties = map[string]string{}
			}
			upperState.Properties["upper_block_bit"] = "true"
			upperState.Properties["half"] = "upper"
			var ok bool
			upperFeatureBlock, ok = g.featureBlockFromState(upperState, rng)
			if !ok {
				return false
			}
		}
	}
	if hasUpperFeatureBlock || hasUpperState {
		if upperPos[1] > maxY {
			return false
		}
		upperRID := c.Block(uint8(upperPos[0]&15), int16(upperPos[1]), uint8(upperPos[2]&15), 0)
		upperBlock, _ := world.BlockByRuntimeID(upperRID)
		if !g.canReplaceFeatureBlock(upperBlock, upperFeatureBlock) {
			return false
		}
	}

	g.setFeatureBlock(c, pos, featureBlock)

	if hasUpperFeatureBlock {
		g.setFeatureBlock(c, upperPos, upperFeatureBlock)
	} else if hasUpperState {
		g.setBlockStateDirect(c, upperPos, upperState)
	}

	return true
}

func (g Generator) positionInFeatureScope(pos cube.Pos, chunkX, chunkZ, minY, maxY int) bool {
	if g.activeTreeRegion != nil {
		return g.activeTreeRegion.contains(pos)
	}
	return g.positionInChunk(pos, chunkX, chunkZ, minY, maxY)
}

func (g Generator) runtimeIDAt(c *chunk.Chunk, pos cube.Pos) uint32 {
	c = g.chunkForActiveTreePos(c, pos)
	return c.Block(uint8(pos[0]&15), int16(pos[1]), uint8(pos[2]&15), 0)
}

func (g Generator) heightmapPlacementYAtPos(c *chunk.Chunk, pos cube.Pos, kind string, chunkX, chunkZ, minY, maxY int) int {
	if !g.positionInFeatureScope(pos, chunkX, chunkZ, minY, maxY) {
		return minY
	}
	c = g.chunkForActiveTreePos(c, pos)
	return g.heightmapPlacementY(c, pos[0]&15, pos[2]&15, kind, minY, maxY)
}

func (g Generator) featureBlockFromState(state gen.BlockState, rng *gen.WorldgenRandom) (world.Block, bool) {
	state = normalizeFeatureState(state)

	switch state.Name {
	case "tall_grass":
		upper := state.Properties["half"] == "upper"
		return block.DoubleTallGrass{Type: block.NormalDoubleTallGrass(), UpperPart: upper}, true
	case "large_fern":
		upper := state.Properties["half"] == "upper"
		return block.DoubleTallGrass{Type: block.FernDoubleTallGrass(), UpperPart: upper}, true
	case "sugar_cane":
		return block.SugarCane{Age: parseStateInt(state.Properties, "age")}, true
	case "cactus":
		return block.Cactus{Age: parseStateInt(state.Properties, "age")}, true
	case "kelp":
		return block.Kelp{Age: parseStateInt(state.Properties, "age")}, true
	case "water":
		return block.Water{Depth: 8, Falling: state.Properties["falling"] == "true"}, true
	case "lava":
		return block.Lava{Depth: 8, Falling: state.Properties["falling"] == "true"}, true
	case "pumpkin":
		facing := cube.Direction(0)
		if rng != nil {
			facing = cube.Direction(rng.NextInt(4))
		}
		return block.Pumpkin{Facing: facing}, true
	case "bamboo":
		properties := map[string]any{
			"age_bit":                parseStateBool(state.Properties, "age_bit"),
			"bamboo_leaf_size":       state.Properties["bamboo_leaf_size"],
			"bamboo_stalk_thickness": state.Properties["bamboo_stalk_thickness"],
		}
		if properties["bamboo_leaf_size"] == "" {
			properties["bamboo_leaf_size"] = "no_leaves"
		}
		if properties["bamboo_stalk_thickness"] == "" {
			properties["bamboo_stalk_thickness"] = "thin"
		}
		return world.BlockByName("minecraft:bamboo", properties)
	}

	props := featureBlockProperties(state.Properties)
	name := state.Name
	if !strings.Contains(name, ":") {
		name = "minecraft:" + name
	}
	featureBlock, ok := world.BlockByName(name, props)
	if ok {
		return featureBlock, true
	}
	return nil, false
}

func featureBlockProperties(properties map[string]string) map[string]any {
	if len(properties) == 0 {
		return nil
	}

	out := make(map[string]any, len(properties))
	for key, value := range properties {
		switch value {
		case "true":
			out[key] = true
		case "false":
			out[key] = false
		default:
			if n, err := strconv.ParseInt(value, 10, 32); err == nil {
				out[key] = int32(n)
			} else {
				out[key] = value
			}
		}
	}
	return out
}

func parseStateInt(properties map[string]string, key string) int {
	if properties == nil {
		return 0
	}
	value, ok := properties[key]
	if !ok {
		return 0
	}
	n, _ := strconv.Atoi(value)
	return n
}

func parseStateBool(properties map[string]string, keys ...string) bool {
	if properties == nil {
		return false
	}
	for _, key := range keys {
		value, ok := properties[key]
		if !ok {
			continue
		}
		return value == "true" || value == "1"
	}
	return false
}

func parseStateDirection(properties map[string]string, keys ...string) cube.Direction {
	if properties == nil {
		return cube.North
	}
	for _, key := range keys {
		value, ok := properties[key]
		if !ok {
			continue
		}
		switch value {
		case "south":
			return cube.South
		case "west":
			return cube.West
		case "east":
			return cube.East
		default:
			return cube.North
		}
	}
	return cube.North
}

func parseStateFace(properties map[string]string, keys ...string) cube.Face {
	if properties == nil {
		return cube.FaceUp
	}
	for _, key := range keys {
		value, ok := properties[key]
		if !ok {
			continue
		}
		switch value {
		case "down":
			return cube.FaceDown
		case "north":
			return cube.FaceNorth
		case "south":
			return cube.FaceSouth
		case "west":
			return cube.FaceWest
		case "east":
			return cube.FaceEast
		default:
			return cube.FaceUp
		}
	}
	return cube.FaceUp
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func hugeMushroomBitsFromState(properties map[string]string, stem bool) int {
	if properties == nil {
		if stem {
			return 10
		}
		return 5
	}
	if _, ok := properties["huge_mushroom_bits"]; ok {
		return max(0, min(15, parseStateInt(properties, "huge_mushroom_bits")))
	}
	down := parseStateBool(properties, "down")
	east := parseStateBool(properties, "east")
	north := parseStateBool(properties, "north")
	south := parseStateBool(properties, "south")
	up := parseStateBool(properties, "up")
	west := parseStateBool(properties, "west")
	if stem {
		if !up && !down && east && north && south && west {
			return 10
		}
		if up && down && east && north && south && west {
			return 15
		}
	}
	if up && east && north && south && west {
		return 14
	}
	switch {
	case west && north && !east && !south:
		return 1
	case north && !east && !south && !west:
		return 2
	case north && east && !south && !west:
		return 3
	case west && !north && !east && !south:
		return 4
	case !north && !east && !south && !west:
		return 5
	case east && !north && !south && !west:
		return 6
	case south && west && !north && !east:
		return 7
	case south && !north && !east && !west:
		return 8
	case south && east && !north && !west:
		return 9
	default:
		if stem {
			return 10
		}
		return 0
	}
}

func normalizeFeatureState(state gen.BlockState) gen.BlockState {
	state.Name = normalizeFeatureStateName(state.Name)
	if len(state.Properties) == 0 || !featureStateNeedsPropertyNormalization(state.Name, state.Properties) {
		return state
	}

	props := make(map[string]string, len(state.Properties))
	for key, value := range state.Properties {
		props[key] = value
	}

	switch {
	case strings.HasSuffix(state.Name, "_log"),
		strings.HasSuffix(state.Name, "_wood"),
		strings.HasSuffix(state.Name, "_stem"),
		strings.HasSuffix(state.Name, "_hyphae"),
		state.Name == "muddy_mangrove_roots",
		state.Name == "basalt",
		state.Name == "deepslate":
		renameFeatureProperty(props, "axis", "pillar_axis")
	case strings.HasSuffix(state.Name, "_leaves"),
		state.Name == "azalea_leaves",
		state.Name == "azalea_leaves_flowered":
		renameFeatureProperty(props, "persistent", "persistent_bit")
		delete(props, "distance")
		delete(props, "waterlogged")
		if _, ok := props["update_bit"]; !ok {
			props["update_bit"] = "false"
		}
	}
	if state.Name == "hanging_roots" {
		delete(props, "waterlogged")
	}
	if state.Name == "podzol" {
		delete(props, "snowy")
	}

	if len(props) == 0 {
		state.Properties = nil
	} else {
		state.Properties = props
	}
	return state
}

func featureStateNeedsPropertyNormalization(name string, properties map[string]string) bool {
	switch {
	case strings.HasSuffix(name, "_log"),
		strings.HasSuffix(name, "_wood"),
		strings.HasSuffix(name, "_stem"),
		strings.HasSuffix(name, "_hyphae"),
		name == "muddy_mangrove_roots",
		name == "basalt",
		name == "deepslate":
		_, ok := properties["axis"]
		return ok
	case strings.HasSuffix(name, "_leaves"),
		name == "azalea_leaves",
		name == "azalea_leaves_flowered":
		if _, ok := properties["persistent"]; ok {
			return true
		}
		if _, ok := properties["distance"]; ok {
			return true
		}
		if _, ok := properties["waterlogged"]; ok {
			return true
		}
		_, ok := properties["update_bit"]
		return !ok
	case name == "hanging_roots":
		_, ok := properties["waterlogged"]
		return ok
	case name == "podzol":
		_, ok := properties["snowy"]
		return ok
	default:
		return false
	}
}

func normalizeFeatureStateName(name string) string {
	name = strings.TrimPrefix(name, "minecraft:")
	switch name {
	case "lily_pad":
		return "waterlily"
	case "snow_block":
		return "snow"
	case "nether_quartz_ore":
		return "quartz_ore"
	case "flowering_azalea_leaves":
		return "azalea_leaves_flowered"
	default:
		return name
	}
}

func renameFeatureProperty(properties map[string]string, from, to string) {
	value, ok := properties[from]
	if !ok {
		return
	}
	delete(properties, from)
	if _, exists := properties[to]; !exists {
		properties[to] = value
	}
}

func (g Generator) blockEncodedName(b world.Block) string {
	return featureBlockName(b)
}

func featureBlockName(b world.Block) string {
	switch b := b.(type) {
	case block.Grass:
		return "grass"
	default:
		name, _ := b.EncodeBlock()
		return strings.TrimPrefix(name, "minecraft:")
	}
}

func (g Generator) canSaplingSurviveOn(belowBlock world.Block, stateName string) bool {
	if belowBlock == nil {
		return false
	}
	blockName := featureBlockName(belowBlock)
	switch stateName {
	case "azalea", "flowering_azalea":
		return g.matchesFeatureBlockTag(blockName, "supports_azalea")
	case "mangrove_propagule":
		return g.matchesFeatureBlockTag(blockName, "supports_mangrove_propagule")
	default:
		return slices.Contains([]string{"dirt", "coarse_dirt", "grass", "podzol", "farmland"}, blockName)
	}
}

func supportsBambooBlock(b world.Block) bool {
	return matchesFeatureSupportBlockTag(b, "supports_bamboo")
}

func isFreezingBiomeKey(biomeKey string) bool {
	switch biomeKey {
	case "frozen_ocean", "deep_frozen_ocean", "frozen_river", "snowy_beach", "snowy_plains", "snowy_taiga", "ice_spikes", "grove", "snowy_slopes", "frozen_peaks", "jagged_peaks":
		return true
	default:
		return strings.Contains(biomeKey, "snowy") || strings.Contains(biomeKey, "frozen")
	}
}

func (g Generator) canReplaceFeatureBlock(current, with world.Block) bool {
	if current == nil {
		return true
	}
	if _, ok := current.(block.Air); ok {
		return true
	}
	if _, ok := current.(block.Water); ok {
		_, submerged := with.(block.Kelp)
		return submerged || strings.Contains(g.blockEncodedName(with), "seagrass") || strings.Contains(g.blockEncodedName(with), "sea_pickle")
	}
	replaceable, ok := current.(block.Replaceable)
	return ok && replaceable.ReplaceableBy(with)
}

func (g Generator) canBlockStateSurvive(c *chunk.Chunk, pos cube.Pos, state gen.BlockState, rng *gen.WorldgenRandom, minY, maxY int) bool {
	stateName := normalizeFeatureStateName(state.Name)
	if g.canNamedFeatureStateSurvive(c, pos, stateName, minY, maxY) {
		return true
	}

	featureBlock, ok := g.featureBlockFromState(state, rng)
	if !ok {
		return false
	}
	return g.canFeatureBlockSurvive(c, pos, featureBlock, state.Name, minY, maxY)
}

func (g Generator) canNamedFeatureStateSurvive(c *chunk.Chunk, pos cube.Pos, stateName string, minY, maxY int) bool {
	if pos[1] <= minY {
		return false
	}

	belowRID := c.Block(uint8(pos[0]&15), int16(pos[1]-1), uint8(pos[2]&15), 0)
	belowBlock, _ := world.BlockByRuntimeID(belowRID)

	switch stateName {
	case "oak_sapling", "spruce_sapling", "birch_sapling", "jungle_sapling", "acacia_sapling", "dark_oak_sapling", "cherry_sapling", "pale_oak_sapling", "mangrove_propagule", "azalea", "flowering_azalea":
		return g.canSaplingSurviveOn(belowBlock, stateName)
	case "sweet_berry_bush", "brown_mushroom", "red_mushroom", "firefly_bush":
		return belowRID != g.airRID && belowRID != g.waterRID && belowRID != g.lavaRID
	case "warped_fungus", "crimson_fungus":
		return supportsNetherFloraBlock(belowBlock)
	case "warped_roots", "crimson_roots":
		return supportsNetherRootsBlock(belowBlock)
	case "twisting_vines":
		return supportsTwistingVinesBlock(belowBlock)
	case "weeping_vines":
		aboveRID := c.Block(uint8(pos[0]&15), int16(pos[1]+1), uint8(pos[2]&15), 0)
		aboveBlock, _ := world.BlockByRuntimeID(aboveRID)
		return supportsWeepingVinesBlock(aboveBlock)
	case "chorus_plant", "chorus_flower":
		return supportsChorusBlock(belowBlock)
	case "seagrass", "tall_seagrass", "sea_pickle":
		currentRID := c.Block(uint8(pos[0]&15), int16(pos[1]), uint8(pos[2]&15), 0)
		return currentRID == g.waterRID && g.isSolidRID(belowRID)
	case "lily_pad":
		return belowRID == g.waterRID
	default:
		return false
	}
}

func (g Generator) canFeatureBlockSurvive(c *chunk.Chunk, pos cube.Pos, featureBlock world.Block, stateName string, minY, maxY int) bool {
	if pos[1] <= minY {
		return false
	}

	stateName = normalizeFeatureStateName(stateName)
	if g.canNamedFeatureStateSurvive(c, pos, stateName, minY, maxY) {
		return true
	}

	belowRID := c.Block(uint8(pos[0]&15), int16(pos[1]-1), uint8(pos[2]&15), 0)
	belowBlock, _ := world.BlockByRuntimeID(belowRID)
	switch stateName {
	case "azalea", "flowering_azalea":
		return g.matchesFeatureBlockTag(featureBlockName(belowBlock), "supports_azalea")
	case "bamboo":
		return supportsBambooBlock(belowBlock)
	case "crimson_fungus", "warped_fungus":
		return supportsNetherFloraBlock(belowBlock)
	case "crimson_roots", "warped_roots":
		return supportsNetherRootsBlock(belowBlock)
	case "twisting_vines":
		return supportsTwistingVinesBlock(belowBlock)
	case "weeping_vines":
		aboveRID := c.Block(uint8(pos[0]&15), int16(pos[1]+1), uint8(pos[2]&15), 0)
		aboveBlock, _ := world.BlockByRuntimeID(aboveRID)
		return supportsWeepingVinesBlock(aboveBlock)
	case "chorus_plant", "chorus_flower":
		return supportsChorusBlock(belowBlock)
	case "leaf_litter", "pale_moss_carpet":
		return belowRID != g.airRID && belowRID != g.waterRID && belowRID != g.lavaRID
	case "hanging_roots", "pale_hanging_moss":
		if pos[1] >= maxY {
			return false
		}
		aboveRID := c.Block(uint8(pos[0]&15), int16(pos[1]+1), uint8(pos[2]&15), 0)
		return g.isSolidRID(aboveRID)
	case "small_dripleaf", "small_dripleaf_block":
		if parseStateBool(stateNameProperties(featureBlock), "upper_block_bit") {
			return false
		}
		return matchesFeatureSupportBlockTag(belowBlock, "supports_small_dripleaf")
	case "big_dripleaf", "big_dripleaf_stem":
		if featureBlockName(belowBlock) == "big_dripleaf" || featureBlockName(belowBlock) == "big_dripleaf_stem" {
			return true
		}
		return matchesFeatureSupportBlockTag(belowBlock, "supports_big_dripleaf")
	}
	switch featureBlock := featureBlock.(type) {
	case block.ShortGrass, block.DoubleTallGrass, block.Flower:
		soil, ok := belowBlock.(block.Soil)
		return ok && soil.SoilFor(featureBlock)
	case block.SugarCane:
		if !g.positionInChunk(pos, pos[0]>>4, pos[2]>>4, minY, maxY) {
			return false
		}
		if _, ok := belowBlock.(block.SugarCane); ok {
			return true
		}
		for _, face := range cube.HorizontalFaces() {
			side := pos.Side(face).Side(cube.FaceDown)
			if !g.positionInChunk(side, pos[0]>>4, pos[2]>>4, minY, maxY) {
				continue
			}
			if rid := c.Block(uint8(side[0]&15), int16(side[1]), uint8(side[2]&15), 0); rid == g.waterRID {
				soil, ok := belowBlock.(block.Soil)
				return ok && soil.SoilFor(featureBlock)
			}
		}
		return false
	case block.Cactus:
		for _, face := range cube.HorizontalFaces() {
			side := pos.Side(face)
			if !g.positionInChunk(side, pos[0]>>4, pos[2]>>4, minY, maxY) {
				continue
			}
			sideRID := c.Block(uint8(side[0]&15), int16(side[1]), uint8(side[2]&15), 0)
			if sideRID != g.airRID {
				return false
			}
		}
		soil, ok := belowBlock.(block.Soil)
		return ok && soil.SoilFor(featureBlock)
	case block.Kelp:
		currentRID := c.Block(uint8(pos[0]&15), int16(pos[1]), uint8(pos[2]&15), 0)
		if currentRID != g.waterRID {
			return false
		}
		if _, ok := belowBlock.(block.Kelp); ok {
			return true
		}
		return g.isSolidRID(belowRID)
	case block.Pumpkin:
		return belowRID != g.airRID && belowRID != g.waterRID && belowRID != g.lavaRID
	default:
		return false
	}
}

func stateNameProperties(b world.Block) map[string]string {
	_, props := b.EncodeBlock()
	out := make(map[string]string, len(props))
	for k, v := range props {
		out[k] = fmt.Sprint(v)
	}
	return out
}

func matchesFeatureSupportBlockTag(b world.Block, tag string) bool {
	if b == nil {
		return false
	}
	return featureBlockTagMatches(featureBlockName(b), tag)
}

func (g Generator) testBlockPredicate(c *chunk.Chunk, pos cube.Pos, predicate gen.BlockPredicate, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom) bool {
	switch predicate.Type {
	case "matching_blocks":
		cfg, err := predicate.MatchingBlocks()
		if err != nil {
			return false
		}
		target := pos.Add(cube.Pos(cfg.Offset))
		if !g.positionInChunk(target, chunkX, chunkZ, minY, maxY) {
			return false
		}
		name := g.blockNameAt(c, target)
		for _, candidate := range cfg.Blocks.Values {
			if normalizeFeatureStateName(candidate) == name {
				return true
			}
		}
		return false
	case "matching_fluids":
		cfg, err := predicate.MatchingFluids()
		if err != nil {
			return false
		}
		target := pos.Add(cube.Pos(cfg.Offset))
		if !g.positionInChunk(target, chunkX, chunkZ, minY, maxY) {
			return false
		}
		rid := c.Block(uint8(target[0]&15), int16(target[1]), uint8(target[2]&15), 0)
		fluid := g.blockNameAt(c, target)
		if rid == g.waterRID && slices.Contains(cfg.Fluids.Values, "flowing_water") {
			return true
		}
		return slices.Contains(cfg.Fluids.Values, fluid)
	case "matching_block_tag":
		cfg, err := predicate.MatchingBlockTag()
		if err != nil {
			return false
		}
		target := pos.Add(cube.Pos(cfg.Offset))
		if !g.positionInChunk(target, chunkX, chunkZ, minY, maxY) {
			return false
		}
		return g.matchesFeatureBlockTag(g.blockNameAt(c, target), cfg.Tag)
	case "solid":
		rid := c.Block(uint8(pos[0]&15), int16(pos[1]), uint8(pos[2]&15), 0)
		return g.isSolidRID(rid)
	case "all_of":
		var raw struct {
			Predicates []gen.BlockPredicate `json:"predicates"`
		}
		if err := json.Unmarshal(predicate.Data, &raw); err != nil {
			return false
		}
		for _, child := range raw.Predicates {
			if !g.testBlockPredicate(c, pos, child, chunkX, chunkZ, minY, maxY, rng) {
				return false
			}
		}
		return true
	case "any_of":
		var raw struct {
			Predicates []gen.BlockPredicate `json:"predicates"`
		}
		if err := json.Unmarshal(predicate.Data, &raw); err != nil {
			return false
		}
		for _, child := range raw.Predicates {
			if g.testBlockPredicate(c, pos, child, chunkX, chunkZ, minY, maxY, rng) {
				return true
			}
		}
		return false
	case "not":
		cfg, err := predicate.Not()
		if err != nil {
			return false
		}
		return !g.testBlockPredicate(c, pos, cfg.Predicate, chunkX, chunkZ, minY, maxY, rng)
	case "would_survive":
		cfg, err := predicate.WouldSurvive()
		if err != nil {
			return false
		}
		return g.canBlockStateSurvive(c, pos, cfg.State, rng, minY, maxY)
	case "inside_world_bounds":
		var raw struct {
			Offset gen.BlockPos `json:"offset"`
		}
		if err := json.Unmarshal(predicate.Data, &raw); err != nil {
			return false
		}
		target := pos.Add(cube.Pos(raw.Offset))
		return target[1] >= minY && target[1] <= maxY
	default:
		return false
	}
}

func (g Generator) blockNameAt(c *chunk.Chunk, pos cube.Pos) string {
	c = g.chunkForActiveTreePos(c, pos)
	rid := c.Block(uint8(pos[0]&15), int16(pos[1]), uint8(pos[2]&15), 0)
	if name, ok := g.blockNameCache.Lookup(rid); ok {
		return name
	}
	featureBlock, ok := world.BlockByRuntimeID(rid)
	if !ok {
		return "air"
	}
	name := featureBlockName(featureBlock)
	g.blockNameCache.Store(rid, name)
	return name
}

func (g Generator) blockNameAtSafe(c *chunk.Chunk, pos cube.Pos, chunkX, chunkZ, minY, maxY int) string {
	if !g.positionInChunk(pos, chunkX, chunkZ, minY, maxY) {
		return "air"
	}
	return g.blockNameAt(c, pos)
}

func worldBlockAtChunk(c *chunk.Chunk, pos cube.Pos) world.Block {
	rid := c.Block(uint8(pos[0]&15), int16(pos[1]), uint8(pos[2]&15), 0)
	b, _ := world.BlockByRuntimeID(rid)
	return b
}

func (g Generator) worldBlockAtChunkSafe(c *chunk.Chunk, pos cube.Pos, chunkX, chunkZ, minY, maxY int) world.Block {
	if !g.positionInChunk(pos, chunkX, chunkZ, minY, maxY) {
		return nil
	}
	return worldBlockAtChunk(c, pos)
}

func supportsNetherFloraBlock(b world.Block) bool {
	return strings.HasSuffix(featureBlockName(b), "nylium")
}

func supportsNetherRootsBlock(b world.Block) bool {
	name := featureBlockName(b)
	return strings.HasSuffix(name, "nylium") || name == "soul_soil"
}

func supportsTwistingVinesBlock(b world.Block) bool {
	name := featureBlockName(b)
	return name == "netherrack" || strings.HasSuffix(name, "nylium") || name == "nether_wart_block" || name == "blackstone"
}

func supportsWeepingVinesBlock(b world.Block) bool {
	switch b.(type) {
	case block.Netherrack, block.NetherWartBlock, block.Wood, block.Log:
		return true
	default:
		return false
	}
}

func supportsBasaltAnchorBlock(b world.Block) bool {
	if b == nil {
		return false
	}
	switch b.(type) {
	case block.Netherrack, block.Basalt, block.Blackstone, block.SoulSoil, block.SoulSand:
		return true
	default:
		name, _ := b.EncodeBlock()
		name = strings.TrimPrefix(name, "minecraft:")
		return name == "magma" || name == "magma_block"
	}
}

func supportsChorusBlock(b world.Block) bool {
	switch featureBlockName(b) {
	case "end_stone", "chorus_plant":
		return true
	default:
		return false
	}
}

func (g Generator) matchesFeatureBlockTag(blockName, tag string) bool {
	return featureBlockTagMatches(blockName, tag)
}

func featureBlockTagMatches(blockName, tag string) bool {
	tag = normalizeFeatureTag(tag)
	switch tag {
	case "dirt":
		return slices.Contains([]string{"dirt", "coarse_dirt", "rooted_dirt"}, blockName)
	case "mud":
		return slices.Contains([]string{"mud", "muddy_mangrove_roots"}, blockName)
	case "moss_blocks":
		return slices.Contains([]string{"moss_block", "pale_moss_block"}, blockName)
	case "grass_blocks":
		return slices.Contains([]string{"grass", "podzol", "mycelium"}, blockName)
	case "sand":
		return slices.Contains([]string{"sand", "red_sand", "suspicious_sand"}, blockName)
	case "terracotta":
		return blockName == "terracotta" || strings.HasSuffix(blockName, "_terracotta")
	case "base_stone_overworld":
		return slices.Contains([]string{"stone", "granite", "diorite", "andesite", "tuff", "deepslate"}, blockName)
	case "cave_vines":
		return blockName == "cave_vines" || blockName == "cave_vines_plant"
	case "small_flowers":
		return slices.Contains([]string{
			"dandelion",
			"open_eyeblossom",
			"poppy",
			"blue_orchid",
			"allium",
			"azure_bluet",
			"red_tulip",
			"orange_tulip",
			"white_tulip",
			"pink_tulip",
			"oxeye_daisy",
			"cornflower",
			"lily_of_the_valley",
			"wither_rose",
			"torchflower",
			"closed_eyeblossom",
			"golden_dandelion",
		}, blockName)
	case "replaceable_by_trees":
		switch blockName {
		case "pale_moss_carpet", "short_grass", "fern", "dead_bush", "vine", "glow_lichen", "sunflower", "lilac", "rose_bush", "peony", "tall_grass", "large_fern", "hanging_roots", "pitcher_plant", "water", "seagrass", "tall_seagrass", "bush", "firefly_bush", "warped_roots", "nether_sprouts", "crimson_roots", "leaf_litter", "short_dry_grass", "tall_dry_grass":
			return true
		}
		return strings.HasSuffix(blockName, "_leaves") || featureBlockTagMatches(blockName, "small_flowers")
	case "replaceable_by_mushrooms":
		switch blockName {
		case "pale_moss_carpet", "short_grass", "fern", "dead_bush", "vine", "glow_lichen", "sunflower", "lilac", "rose_bush", "peony", "tall_grass", "large_fern", "hanging_roots", "pitcher_plant", "water", "seagrass", "tall_seagrass", "brown_mushroom", "red_mushroom", "brown_mushroom_block", "red_mushroom_block", "warped_roots", "nether_sprouts", "crimson_roots", "leaf_litter", "short_dry_grass", "tall_dry_grass", "bush", "firefly_bush":
			return true
		}
		return strings.HasSuffix(blockName, "_leaves") || featureBlockTagMatches(blockName, "small_flowers")
	case "substrate_overworld":
		return featureBlockTagMatches(blockName, "dirt") ||
			featureBlockTagMatches(blockName, "mud") ||
			featureBlockTagMatches(blockName, "moss_blocks") ||
			featureBlockTagMatches(blockName, "grass_blocks")
	case "supports_vegetation":
		return featureBlockTagMatches(blockName, "substrate_overworld") || blockName == "farmland"
	case "supports_azalea", "supports_mangrove_propagule":
		return featureBlockTagMatches(blockName, "supports_vegetation") || blockName == "clay"
	case "supports_hanging_mangrove_propagule":
		return blockName == "mangrove_leaves"
	case "supports_bamboo":
		return featureBlockTagMatches(blockName, "sand") ||
			featureBlockTagMatches(blockName, "substrate_overworld") ||
			slices.Contains([]string{"gravel", "suspicious_gravel", "bamboo", "bamboo_sapling"}, blockName)
	case "beneath_bamboo_podzol_replaceable":
		return featureBlockTagMatches(blockName, "substrate_overworld")
	case "beneath_tree_podzol_replaceable":
		return featureBlockTagMatches(blockName, "substrate_overworld")
	case "supports_small_dripleaf":
		return blockName == "clay" || blockName == "moss_block"
	case "supports_big_dripleaf":
		return featureBlockTagMatches(blockName, "supports_small_dripleaf") ||
			featureBlockTagMatches(blockName, "dirt") ||
			featureBlockTagMatches(blockName, "grass_blocks") ||
			featureBlockTagMatches(blockName, "mud") ||
			blockName == "farmland"
	case "azalea_grows_on":
		return featureBlockTagMatches(blockName, "substrate_overworld") ||
			featureBlockTagMatches(blockName, "sand") ||
			blockName == "snow_block" ||
			blockName == "powder_snow" ||
			featureBlockTagMatches(blockName, "terracotta")
	case "moss_replaceable":
		return featureBlockTagMatches(blockName, "base_stone_overworld") ||
			featureBlockTagMatches(blockName, "cave_vines") ||
			featureBlockTagMatches(blockName, "dirt") ||
			featureBlockTagMatches(blockName, "mud") ||
			featureBlockTagMatches(blockName, "moss_blocks") ||
			featureBlockTagMatches(blockName, "grass_blocks")
	case "lush_ground_replaceable":
		return featureBlockTagMatches(blockName, "moss_replaceable") ||
			blockName == "clay" ||
			blockName == "gravel" ||
			blockName == "sand"
	case "azalea_root_replaceable":
		return featureBlockTagMatches(blockName, "base_stone_overworld") ||
			featureBlockTagMatches(blockName, "substrate_overworld") ||
			featureBlockTagMatches(blockName, "terracotta") ||
			blockName == "red_sand" ||
			blockName == "clay" ||
			blockName == "gravel" ||
			blockName == "sand" ||
			blockName == "snow_block" ||
			blockName == "powder_snow"
	case "cannot_replace_below_tree_trunk":
		return featureBlockTagMatches(blockName, "dirt") ||
			featureBlockTagMatches(blockName, "mud") ||
			featureBlockTagMatches(blockName, "moss_blocks") ||
			blockName == "podzol"
	case "forest_rock_can_place_on":
		return featureBlockTagMatches(blockName, "substrate_overworld") || featureBlockTagMatches(blockName, "base_stone_overworld")
	case "huge_brown_mushroom_can_place_on", "huge_red_mushroom_can_place_on":
		return featureBlockTagMatches(blockName, "substrate_overworld") ||
			blockName == "mycelium" ||
			blockName == "podzol" ||
			blockName == "crimson_nylium" ||
			blockName == "warped_nylium"
	case "ice_spike_replaceable":
		return featureBlockTagMatches(blockName, "substrate_overworld") ||
			blockName == "snow" ||
			blockName == "snow_block" ||
			blockName == "ice"
	case "features_cannot_replace":
		switch blockName {
		case "bedrock", "mob_spawner", "chest", "end_portal_frame", "reinforced_deepslate", "trial_spawner", "vault":
			return true
		default:
			return false
		}
	case "geode_invalid_blocks":
		switch blockName {
		case "bedrock", "water", "lava", "ice", "packed_ice", "blue_ice":
			return true
		default:
			return false
		}
	case "mangrove_roots_can_grow_through":
		return slices.Contains([]string{"mud", "muddy_mangrove_roots", "mangrove_roots", "moss_carpet", "vine", "mangrove_propagule", "snow"}, blockName)
	case "mangrove_logs_can_grow_through":
		return slices.Contains([]string{"mud", "muddy_mangrove_roots", "mangrove_roots", "mangrove_leaves", "mangrove_log", "mangrove_propagule", "moss_carpet", "vine"}, blockName)
	default:
		return false
	}
}

func normalizeFeatureTag(tag string) string {
	tag = strings.TrimPrefix(tag, "#")
	return strings.TrimPrefix(tag, "minecraft:")
}

func (g Generator) biomeKeyAt(c *chunk.Chunk, localX, y, localZ int) string {
	return biomeKey(biomeFromRuntimeID(c.Biome(uint8(localX), int16(y), uint8(localZ))))
}

func (g Generator) sourceBiomeKeyAt(biomes sourceBiomeVolume, localX, y, localZ int) string {
	return biomeKey(biomes.biomeAt(localX, y, localZ))
}

func (g Generator) heightmapPlacementY(c *chunk.Chunk, localX, localZ int, kind string, minY, maxY int) int {
	switch kind {
	case "WORLD_SURFACE_WG", "WORLD_SURFACE", "MOTION_BLOCKING", "MOTION_BLOCKING_NO_LEAVES":
		return g.columnHeightmapY(c, localX, localZ, kind, minY, maxY)
	case "OCEAN_FLOOR", "OCEAN_FLOOR_WG":
		return g.columnHeightmapY(c, localX, localZ, kind, minY, maxY)
	default:
		return g.columnHeightmapY(c, localX, localZ, "WORLD_SURFACE", minY, maxY)
	}
}

func (g Generator) surfaceWaterDepthAt(c *chunk.Chunk, localX, localZ, minY int) int {
	maxY := c.Range().Max()
	worldSurface := g.heightmapPlacementY(c, localX, localZ, "WORLD_SURFACE", minY, maxY)
	oceanFloor := g.heightmapPlacementY(c, localX, localZ, "OCEAN_FLOOR_WG", minY, maxY)
	if worldSurface <= oceanFloor {
		return 0
	}
	return worldSurface - oceanFloor
}

func (g Generator) columnHeightmapY(c *chunk.Chunk, localX, localZ int, kind string, minY, maxY int) int {
	topY := int(c.HighestBlock(uint8(localX), uint8(localZ)))
	if topY < minY {
		return minY
	}
	if topY > maxY {
		topY = maxY
	}

	switch kind {
	case "WORLD_SURFACE_WG", "WORLD_SURFACE":
		return min(topY+1, maxY)
	case "MOTION_BLOCKING":
		for y := topY; y >= minY; y-- {
			if rid := g.columnScanRuntimeID(c, localX, y, localZ); g.isMotionBlockingRID(rid, false) {
				return min(y+1, maxY)
			}
		}
		return minY
	case "MOTION_BLOCKING_NO_LEAVES":
		for y := topY; y >= minY; y-- {
			if rid := g.columnScanRuntimeID(c, localX, y, localZ); g.isMotionBlockingRID(rid, true) {
				return min(y+1, maxY)
			}
		}
		return minY
	case "OCEAN_FLOOR", "OCEAN_FLOOR_WG":
		for y := topY; y >= minY; y-- {
			rid := g.columnScanRuntimeID(c, localX, y, localZ)
			if rid == g.airRID || rid == g.waterRID || rid == g.lavaRID {
				continue
			}
			if g.isSolidRID(rid) {
				return min(y+1, maxY)
			}
		}
		return minY
	default:
		return min(topY+1, maxY)
	}
}

func (g Generator) columnScanRuntimeID(c *chunk.Chunk, localX, y, localZ int) uint32 {
	rid := c.Block(uint8(localX), int16(y), uint8(localZ), 0)
	if rid != g.airRID {
		return rid
	}
	return c.Block(uint8(localX), int16(y), uint8(localZ), 1)
}

func (g Generator) isMotionBlockingRID(rid uint32, ignoreLeaves bool) bool {
	if rid == g.airRID {
		return false
	}
	if rid == g.waterRID || rid == g.lavaRID {
		return true
	}
	if g.isLeafRID(rid) {
		return !ignoreLeaves
	}
	return g.isSolidRID(rid)
}

func (g Generator) isLeafRID(rid uint32) bool {
	if rid == g.airRID {
		return false
	}
	b, ok := world.BlockByRuntimeID(rid)
	if !ok {
		return false
	}
	name, _ := b.EncodeBlock()
	return strings.HasSuffix(strings.TrimPrefix(name, "minecraft:"), "_leaves")
}

func (g Generator) sampleNoiseBasedCount(cfg gen.NoiseBasedCountPlacement, pos cube.Pos) int {
	noise := g.surface.SurfaceSecondary(int(float64(pos[0])/cfg.NoiseFactor), int(float64(pos[2])/cfg.NoiseFactor))*2.0 - 1.0
	count := int(math.Ceil((noise + cfg.NoiseOffset) * float64(cfg.NoiseToCountRatio)))
	if count < 0 {
		return 0
	}
	return count
}

// sampleHeightProvider mirrors the vanilla height providers exactly: no
// clamping or swapping of the resolved anchors, and the same nextInt call
// sequences.
func (g Generator) sampleHeightProvider(provider gen.HeightProvider, minY, maxY int, rng *gen.WorldgenRandom) int {
	switch provider.Kind {
	case "constant":
		return g.anchorY(provider.Value, minY, maxY)
	case "uniform":
		low := g.anchorY(provider.MinInclusive, minY, maxY)
		high := g.anchorY(provider.MaxInclusive, minY, maxY)
		if low > high {
			return low
		}
		return randomBetweenInclusive(rng, low, high)
	case "trapezoid":
		low := g.anchorY(provider.MinInclusive, minY, maxY)
		high := g.anchorY(provider.MaxInclusive, minY, maxY)
		if low > high {
			return low
		}
		span := high - low
		if provider.Plateau >= span {
			return randomBetweenInclusive(rng, low, high)
		}
		plateauStart := (span - provider.Plateau) / 2
		plateauEnd := span - plateauStart
		return low + randomBetweenInclusive(rng, 0, plateauEnd) + randomBetweenInclusive(rng, 0, plateauStart)
	case "biased_to_bottom":
		low := g.anchorY(provider.MinInclusive, minY, maxY)
		high := g.anchorY(provider.MaxInclusive, minY, maxY)
		if high-low-provider.Inner+1 <= 0 {
			return low
		}
		limit := int(rng.NextInt(uint32(high - low - provider.Inner + 1)))
		return int(rng.NextInt(uint32(limit+provider.Inner))) + low
	case "very_biased_to_bottom":
		low := g.anchorY(provider.MinInclusive, minY, maxY)
		high := g.anchorY(provider.MaxInclusive, minY, maxY)
		if high-low-provider.Inner+1 <= 0 {
			return low
		}
		upper := mthNextInt(rng, low+provider.Inner, high)
		biasedUpper := mthNextInt(rng, low, upper-1)
		return mthNextInt(rng, low, biasedUpper-1+provider.Inner)
	case "clamped_normal":
		low := g.anchorY(provider.MinInclusive, minY, maxY)
		high := g.anchorY(provider.MaxInclusive, minY, maxY)
		return clamp(int(math.Round(g.normalFloat64(rng, provider.Mean, provider.Deviation))), low, high)
	default:
		return g.anchorY(provider.MinInclusive, minY, maxY)
	}
}

// randomBetweenInclusive mirrors Mth.randomBetweenInclusive.
func randomBetweenInclusive(rng *gen.WorldgenRandom, low, high int) int {
	return int(rng.NextInt(uint32(high-low+1))) + low
}

// mthNextInt mirrors Mth.nextInt: returns low when the range is empty.
func mthNextInt(rng *gen.WorldgenRandom, low, high int) int {
	if low >= high {
		return low
	}
	return int(rng.NextInt(uint32(high-low+1))) + low
}

func (g Generator) anchorY(anchor gen.VerticalAnchor, minY, maxY int) int {
	switch anchor.Kind {
	case "absolute":
		return anchor.Value
	case "above_bottom":
		return minY + anchor.Value
	case "below_top":
		return maxY - anchor.Value
	default:
		return minY
	}
}

func (g Generator) scanEnvironment(c *chunk.Chunk, pos cube.Pos, cfg gen.EnvironmentScanPlacement, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom) (cube.Pos, bool) {
	dir := blockColumnDirection(cfg.DirectionOfSearch)
	if dir == (cube.Pos{}) {
		return cube.Pos{}, false
	}
	current := pos
	for step := 0; step <= cfg.MaxSteps; step++ {
		if !g.positionInChunk(current, chunkX, chunkZ, minY, maxY) {
			return cube.Pos{}, false
		}
		if cfg.AllowedSearchCondition != nil && !g.testBlockPredicate(c, current, *cfg.AllowedSearchCondition, chunkX, chunkZ, minY, maxY, rng) {
			return cube.Pos{}, false
		}
		if g.testBlockPredicate(c, current, cfg.TargetCondition, chunkX, chunkZ, minY, maxY, rng) {
			return current, true
		}
		current = current.Add(dir)
	}
	return cube.Pos{}, false
}

func blockColumnDirection(direction string) cube.Pos {
	switch strings.ToLower(direction) {
	case "up":
		return cube.Pos{0, 1, 0}
	case "down":
		return cube.Pos{0, -1, 0}
	case "north":
		return cube.Pos{0, 0, -1}
	case "south":
		return cube.Pos{0, 0, 1}
	case "east":
		return cube.Pos{1, 0, 0}
	case "west":
		return cube.Pos{-1, 0, 0}
	default:
		return cube.Pos{}
	}
}

func (g Generator) setBlockStateDirect(c *chunk.Chunk, pos cube.Pos, state gen.BlockState) bool {
	featureBlock, ok := g.featureBlockFromState(state, nil)
	if !ok {
		return false
	}
	return g.setFeatureBlock(c, pos, featureBlock)
}

func (g Generator) setFeatureBlock(c *chunk.Chunk, pos cube.Pos, featureBlock world.Block) bool {
	c = g.chunkForActiveTreePos(c, pos)
	localX := uint8(pos[0] & 15)
	localZ := uint8(pos[2] & 15)
	y := int16(pos[1])

	liquidRID, displaced := g.displacedLiquidRuntimeID(c, pos, featureBlock)

	c.SetBlock(localX, y, localZ, 0, runtimeIDForBlock(featureBlock))
	if displaced {
		c.SetBlock(localX, y, localZ, 1, liquidRID)
	} else {
		c.SetBlock(localX, y, localZ, 1, g.airRID)
	}
	return true
}

func (g Generator) displacedLiquidRuntimeID(c *chunk.Chunk, pos cube.Pos, featureBlock world.Block) (uint32, bool) {
	c = g.chunkForActiveTreePos(c, pos)
	displacer, ok := featureBlock.(world.LiquidDisplacer)
	if !ok {
		return 0, false
	}

	localX := uint8(pos[0] & 15)
	localZ := uint8(pos[2] & 15)
	y := int16(pos[1])

	for _, layer := range [...]uint8{1, 0} {
		rid := c.Block(localX, y, localZ, layer)
		if rid == g.airRID {
			continue
		}
		placed, ok := world.BlockByRuntimeID(rid)
		if !ok {
			continue
		}
		liquid, ok := placed.(world.Liquid)
		if ok && displacer.CanDisplace(liquid) {
			return rid, true
		}
	}
	return 0, false
}

func (g Generator) tryPlaceOreAt(c *chunk.Chunk, pos cube.Pos, cfg gen.OreConfig, chunkX, chunkZ, minY, maxY int, rng *gen.WorldgenRandom) bool {
	if !g.positionInChunk(pos, chunkX, chunkZ, minY, maxY) {
		return false
	}
	currentName := g.blockNameAt(c, pos)
	targetState, ok := g.matchOreTarget(currentName, cfg.Targets)
	if !ok {
		return false
	}
	if cfg.DiscardChanceOnAirExposure > 0 && g.isExposedToAir(c, pos, chunkX, chunkZ, minY, maxY) && rng.NextDouble() < cfg.DiscardChanceOnAirExposure {
		return false
	}
	return g.setBlockStateDirect(c, pos, targetState)
}

func (g Generator) matchOreTarget(blockName string, targets []gen.OreTargetConfig) (gen.BlockState, bool) {
	for _, target := range targets {
		switch target.Target.PredicateType {
		case "tag_match":
			if g.matchesOreTag(blockName, target.Target.Tag) {
				return target.State, true
			}
		case "block_match":
			if blockName == target.Target.Block {
				return target.State, true
			}
		default:
			if g.matchesOreTag(blockName, target.Target.Tag) {
				return target.State, true
			}
		}
		if target.Target.Block != "" && blockName == target.Target.Block {
			return target.State, true
		}
	}
	return gen.BlockState{}, false
}

func (g Generator) matchesOreTag(blockName, tag string) bool {
	switch tag {
	case "stone_ore_replaceables":
		return slices.Contains([]string{"stone", "granite", "diorite", "andesite", "tuff"}, blockName)
	case "deepslate_ore_replaceables":
		return blockName == "deepslate"
	case "base_stone_overworld":
		return slices.Contains([]string{"stone", "granite", "diorite", "andesite", "tuff", "deepslate"}, blockName)
	case "base_stone_nether":
		return slices.Contains([]string{"netherrack", "basalt", "blackstone"}, blockName)
	default:
		return false
	}
}

func (g Generator) isExposedToAir(c *chunk.Chunk, pos cube.Pos, chunkX, chunkZ, minY, maxY int) bool {
	for _, face := range cube.Faces() {
		neighbor := pos.Side(face)
		if !g.positionInChunk(neighbor, chunkX, chunkZ, minY, maxY) {
			continue
		}
		rid := c.Block(uint8(neighbor[0]&15), int16(neighbor[1]), uint8(neighbor[2]&15), 0)
		if rid == g.airRID {
			return true
		}
	}
	return false
}

func sampleTreeHeight(placer gen.TypedJSONValue, rng *gen.WorldgenRandom) (int, string) {
	var raw struct {
		BaseHeight  int `json:"base_height"`
		HeightRandA int `json:"height_rand_a"`
		HeightRandB int `json:"height_rand_b"`
	}
	if err := json.Unmarshal(placer.Data, &raw); err != nil {
		return 0, placer.Type
	}
	height := raw.BaseHeight
	if raw.HeightRandA > 0 {
		height += int(rng.NextInt(uint32(raw.HeightRandA + 1)))
	}
	if raw.HeightRandB > 0 {
		height += int(rng.NextInt(uint32(raw.HeightRandB + 1)))
	}
	return height, placer.Type
}

type treeMinimumSizeProfile struct {
	kind              string
	limit             int
	upperLimit        int
	lowerSize         int
	middleSize        int
	upperSize         int
	minClippedHeight  int
	hasMinClippedSize bool
}

func decodeTreeMinimumSize(value gen.TypedJSONValue) treeMinimumSizeProfile {
	profile := treeMinimumSizeProfile{
		kind:      value.Type,
		upperSize: 1,
	}
	switch value.Type {
	case "three_layers_feature_size":
		var raw struct {
			Limit            int  `json:"limit"`
			UpperLimit       int  `json:"upper_limit"`
			LowerSize        int  `json:"lower_size"`
			MiddleSize       int  `json:"middle_size"`
			UpperSize        int  `json:"upper_size"`
			MinClippedHeight *int `json:"min_clipped_height"`
		}
		if err := json.Unmarshal(value.Data, &raw); err != nil {
			return profile
		}
		profile.limit = raw.Limit
		profile.upperLimit = raw.UpperLimit
		profile.lowerSize = raw.LowerSize
		profile.middleSize = raw.MiddleSize
		profile.upperSize = raw.UpperSize
		if raw.MinClippedHeight != nil {
			profile.minClippedHeight = *raw.MinClippedHeight
			profile.hasMinClippedSize = true
		}
	default:
		var raw struct {
			Limit            int  `json:"limit"`
			LowerSize        int  `json:"lower_size"`
			UpperSize        int  `json:"upper_size"`
			MinClippedHeight *int `json:"min_clipped_height"`
		}
		if err := json.Unmarshal(value.Data, &raw); err != nil {
			return profile
		}
		profile.kind = "two_layers_feature_size"
		profile.limit = raw.Limit
		profile.lowerSize = raw.LowerSize
		profile.upperSize = raw.UpperSize
		if raw.MinClippedHeight != nil {
			profile.minClippedHeight = *raw.MinClippedHeight
			profile.hasMinClippedSize = true
		}
	}
	if !profile.hasMinClippedSize {
		profile.minClippedHeight = 0
	}
	return profile
}

func (p treeMinimumSizeProfile) sizeAtHeight(treeHeight, y int) int {
	switch p.kind {
	case "three_layers_feature_size":
		if y < p.limit {
			return p.lowerSize
		}
		if y >= treeHeight-p.upperLimit {
			return p.upperSize
		}
		return p.middleSize
	default:
		if y < p.limit {
			return p.lowerSize
		}
		return p.upperSize
	}
}

func (g Generator) maxFreeTreeHeight(c *chunk.Chunk, origin cube.Pos, maxTreeHeight int, sizeProfile treeMinimumSizeProfile, trunkBlock world.Block, minY, maxY int) int {
	chunkX := floorDiv(origin[0], 16)
	chunkZ := floorDiv(origin[2], 16)
	for y := 0; y <= maxTreeHeight+1; y++ {
		radius := sizeProfile.sizeAtHeight(maxTreeHeight, y)
		for dx := -radius; dx <= radius; dx++ {
			for dz := -radius; dz <= radius; dz++ {
				candidate := origin.Add(cube.Pos{dx, y, dz})
				if !g.positionInChunk(candidate, chunkX, chunkZ, minY, maxY) || !g.canTreeGrowInto(c, candidate, trunkBlock) {
					return y - 2
				}
			}
		}
	}
	return maxTreeHeight
}

func (g Generator) canTreeGrowInto(c *chunk.Chunk, pos cube.Pos, trunkBlock world.Block) bool {
	rid := c.Block(uint8(pos[0]&15), int16(pos[1]), uint8(pos[2]&15), 0)
	if rid == g.airRID || g.isLeafRID(rid) {
		return true
	}
	currentBlock, _ := world.BlockByRuntimeID(rid)
	if g.canReplaceFeatureBlock(currentBlock, trunkBlock) {
		return true
	}
	name := g.blockNameAt(c, pos)
	return strings.HasSuffix(name, "_log") || strings.HasSuffix(name, "_wood") || strings.HasSuffix(name, "_stem")
}

func (g Generator) prepareTreeSoil(c *chunk.Chunk, pos cube.Pos, cfg gen.TreeConfig, rng *gen.WorldgenRandom, minY, maxY int) bool {
	if pos[1] <= minY || pos[1] > maxY {
		return false
	}
	below := pos.Side(cube.FaceDown)
	belowRID := c.Block(uint8(below[0]&15), int16(below[1]), uint8(below[2]&15), 0)
	if belowRID != g.airRID && belowRID != g.waterRID && belowRID != g.lavaRID && !cfg.ForceDirt {
		return true
	}
	dirt, ok := g.selectState(c, cfg.DirtProvider, below, rng, minY, maxY)
	if !ok {
		return false
	}
	return g.setBlockStateDirect(c, below, dirt)
}

func (g Generator) placeVerticalTrunk(c *chunk.Chunk, pos cube.Pos, trunk gen.BlockState, height, minY, maxY int) (cube.Pos, bool) {
	current := pos
	for i := 0; i < height; i++ {
		if current[1] <= minY || current[1] > maxY {
			return cube.Pos{}, false
		}
		if !g.setBlockStateDirect(c, current, trunk) {
			return cube.Pos{}, false
		}
		current = current.Side(cube.FaceUp)
	}
	return current.Side(cube.FaceDown), true
}

func (g Generator) placeWideTrunk(c *chunk.Chunk, pos cube.Pos, trunk gen.BlockState, height, minY, maxY int) (cube.Pos, bool) {
	if pos[0]&15 == 15 || pos[2]&15 == 15 {
		return cube.Pos{}, false
	}
	currentY := pos[1]
	for i := 0; i < height; i++ {
		if currentY <= minY || currentY > maxY {
			return cube.Pos{}, false
		}
		for dx := 0; dx < 2; dx++ {
			for dz := 0; dz < 2; dz++ {
				if !g.setBlockStateDirect(c, cube.Pos{pos[0] + dx, currentY, pos[2] + dz}, trunk) {
					return cube.Pos{}, false
				}
			}
		}
		currentY++
	}
	return cube.Pos{pos[0], currentY - 1, pos[2]}, true
}

func (g Generator) placeForkingAcaciaTrunk(c *chunk.Chunk, pos cube.Pos, trunk gen.BlockState, height int, rng *gen.WorldgenRandom, minY, maxY int) (cube.Pos, bool) {
	top, ok := g.placeVerticalTrunk(c, pos, trunk, max(2, height-1), minY, maxY)
	if !ok {
		return cube.Pos{}, false
	}
	branchDir := []cube.Pos{{1, 0, 0}, {-1, 0, 0}, {0, 0, 1}, {0, 0, -1}}[rng.NextInt(4)]
	branch := top
	for i := 0; i < 2; i++ {
		branch = branch.Add(branchDir).Side(cube.FaceUp)
		if branch[1] > maxY || !g.setBlockStateDirect(c, branch, trunk) {
			break
		}
	}
	return branch, true
}

func (g Generator) placeTreeFoliage(c *chunk.Chunk, top cube.Pos, leaf gen.BlockState, placer gen.TypedJSONValue, height int, doubleTrunk bool, rng *gen.WorldgenRandom, minY, maxY int) bool {
	switch placer.Type {
	case "blob_foliage_placer":
		var raw struct {
			Radius gen.IntProvider `json:"radius"`
			Offset gen.IntProvider `json:"offset"`
			Height int             `json:"height"`
		}
		if err := json.Unmarshal(placer.Data, &raw); err != nil {
			return false
		}
		leafRadius := max(0, g.sampleIntProvider(raw.Radius, rng))
		offset := g.sampleIntProvider(raw.Offset, rng)
		for yo := offset; yo >= offset-raw.Height; yo-- {
			currentRadius := max(leafRadius-1-yo/2, 0)
			g.placeTreeLeafRow(c, top, currentRadius, yo, doubleTrunk, leaf, minY, maxY, rng, blobFoliageSkip, nil)
		}
		return true
	case "fancy_foliage_placer":
		var raw struct {
			Radius gen.IntProvider `json:"radius"`
			Offset gen.IntProvider `json:"offset"`
			Height int             `json:"height"`
		}
		if err := json.Unmarshal(placer.Data, &raw); err != nil {
			return false
		}
		leafRadius := max(0, g.sampleIntProvider(raw.Radius, rng))
		offset := g.sampleIntProvider(raw.Offset, rng)
		for yo := offset; yo >= offset-raw.Height; yo-- {
			currentRadius := leafRadius
			if yo != offset && yo != offset-raw.Height {
				currentRadius++
			}
			g.placeTreeLeafRow(c, top, currentRadius, yo, doubleTrunk, leaf, minY, maxY, rng, fancyFoliageSkip, nil)
		}
		return true
	case "bush_foliage_placer":
		var raw struct {
			Radius gen.IntProvider `json:"radius"`
			Offset gen.IntProvider `json:"offset"`
			Height int             `json:"height"`
		}
		if err := json.Unmarshal(placer.Data, &raw); err != nil {
			return false
		}
		leafRadius := max(0, g.sampleIntProvider(raw.Radius, rng))
		offset := g.sampleIntProvider(raw.Offset, rng)
		for yo := offset; yo >= offset-raw.Height; yo-- {
			currentRadius := max(leafRadius-1-yo, 0)
			g.placeTreeLeafRow(c, top, currentRadius, yo, doubleTrunk, leaf, minY, maxY, rng, bushFoliageSkip, nil)
		}
		return true
	case "spruce_foliage_placer":
		var raw struct {
			Radius      gen.IntProvider `json:"radius"`
			Offset      gen.IntProvider `json:"offset"`
			TrunkHeight gen.IntProvider `json:"trunk_height"`
		}
		if err := json.Unmarshal(placer.Data, &raw); err != nil {
			return false
		}
		leafRadius := max(0, g.sampleIntProvider(raw.Radius, rng))
		offset := g.sampleIntProvider(raw.Offset, rng)
		foliageHeight := max(4, height-g.sampleIntProvider(raw.TrunkHeight, rng))
		currentRadius := int(rng.NextInt(2))
		maxRadius := 1
		minRadius := 0
		for yo := offset; yo >= -foliageHeight; yo-- {
			g.placeTreeLeafRow(c, top, currentRadius, yo, doubleTrunk, leaf, minY, maxY, rng, coniferFoliageSkip, nil)
			if currentRadius >= maxRadius {
				currentRadius = minRadius
				minRadius = 1
				maxRadius = min(maxRadius+1, leafRadius)
			} else {
				currentRadius++
			}
		}
		return true
	case "pine_foliage_placer":
		var raw struct {
			Radius gen.IntProvider `json:"radius"`
			Offset gen.IntProvider `json:"offset"`
			Height gen.IntProvider `json:"height"`
		}
		if err := json.Unmarshal(placer.Data, &raw); err != nil {
			return false
		}
		leafRadius := max(0, g.sampleIntProvider(raw.Radius, rng))
		if span := max(height+1, 1); span > 1 {
			leafRadius += int(rng.NextInt(uint32(span)))
		}
		offset := g.sampleIntProvider(raw.Offset, rng)
		foliageHeight := max(0, g.sampleIntProvider(raw.Height, rng))
		currentRadius := 0
		for yo := offset; yo >= offset-foliageHeight; yo-- {
			g.placeTreeLeafRow(c, top, currentRadius, yo, doubleTrunk, leaf, minY, maxY, rng, coniferFoliageSkip, nil)
			if currentRadius >= 1 && yo == offset-foliageHeight+1 {
				currentRadius--
			} else if currentRadius < leafRadius {
				currentRadius++
			}
		}
		return true
	case "acacia_foliage_placer":
		var raw struct {
			Radius gen.IntProvider `json:"radius"`
			Offset gen.IntProvider `json:"offset"`
		}
		if err := json.Unmarshal(placer.Data, &raw); err != nil {
			return false
		}
		leafRadius := max(0, g.sampleIntProvider(raw.Radius, rng))
		foliagePos := top.Add(cube.Pos{0, g.sampleIntProvider(raw.Offset, rng), 0})
		g.placeTreeLeafRow(c, foliagePos, leafRadius, -1, doubleTrunk, leaf, minY, maxY, rng, acaciaFoliageSkip, nil)
		g.placeTreeLeafRow(c, foliagePos, max(leafRadius-1, 0), 0, doubleTrunk, leaf, minY, maxY, rng, acaciaFoliageSkip, nil)
		g.placeTreeLeafRow(c, foliagePos, max(leafRadius-1, 0), 0, doubleTrunk, leaf, minY, maxY, rng, acaciaFoliageSkip, nil)
		return true
	case "dark_oak_foliage_placer":
		var raw struct {
			Radius gen.IntProvider `json:"radius"`
			Offset gen.IntProvider `json:"offset"`
		}
		if err := json.Unmarshal(placer.Data, &raw); err != nil {
			return false
		}
		leafRadius := max(0, g.sampleIntProvider(raw.Radius, rng))
		foliagePos := top.Add(cube.Pos{0, g.sampleIntProvider(raw.Offset, rng), 0})
		if doubleTrunk {
			g.placeTreeLeafRow(c, foliagePos, leafRadius+2, -1, true, leaf, minY, maxY, rng, darkOakFoliageSkip, darkOakSignedSkip)
			g.placeTreeLeafRow(c, foliagePos, leafRadius+3, 0, true, leaf, minY, maxY, rng, darkOakFoliageSkip, darkOakSignedSkip)
			g.placeTreeLeafRow(c, foliagePos, leafRadius+2, 1, true, leaf, minY, maxY, rng, darkOakFoliageSkip, darkOakSignedSkip)
			if rng.NextInt(2) == 0 {
				g.placeTreeLeafRow(c, foliagePos, leafRadius, 2, true, leaf, minY, maxY, rng, darkOakFoliageSkip, darkOakSignedSkip)
			}
		} else {
			g.placeTreeLeafRow(c, foliagePos, leafRadius+2, -1, false, leaf, minY, maxY, rng, darkOakFoliageSkip, nil)
			g.placeTreeLeafRow(c, foliagePos, leafRadius+1, 0, false, leaf, minY, maxY, rng, darkOakFoliageSkip, nil)
		}
		return true
	case "random_spread_foliage_placer":
		var raw struct {
			Radius                gen.IntProvider `json:"radius"`
			Offset                gen.IntProvider `json:"offset"`
			FoliageHeight         gen.IntProvider `json:"foliage_height"`
			LeafPlacementAttempts int             `json:"leaf_placement_attempts"`
		}
		if err := json.Unmarshal(placer.Data, &raw); err != nil {
			return false
		}
		leafRadius := max(1, g.sampleIntProvider(raw.Radius, rng))
		foliageHeight := max(1, g.sampleIntProvider(raw.FoliageHeight, rng))
		attempts := raw.LeafPlacementAttempts
		if attempts <= 0 {
			attempts = max(24, height*8)
		}
		origin := top.Add(cube.Pos{0, g.sampleIntProvider(raw.Offset, rng), 0})
		for i := 0; i < attempts; i++ {
			candidate := origin.Add(cube.Pos{
				int(rng.NextInt(uint32(leafRadius))) - int(rng.NextInt(uint32(leafRadius))),
				int(rng.NextInt(uint32(foliageHeight))) - int(rng.NextInt(uint32(foliageHeight))),
				int(rng.NextInt(uint32(leafRadius))) - int(rng.NextInt(uint32(leafRadius))),
			})
			if candidate[1] <= minY || candidate[1] > maxY {
				continue
			}
			currentRID := c.Block(uint8(candidate[0]&15), int16(candidate[1]), uint8(candidate[2]&15), 0)
			if currentRID == g.airRID {
				_ = g.setBlockStateDirect(c, candidate, leaf)
			}
		}
		return true
	case "cherry_foliage_placer":
		var raw struct {
			Radius                       gen.IntProvider `json:"radius"`
			Offset                       gen.IntProvider `json:"offset"`
			Height                       gen.IntProvider `json:"height"`
			WideBottomLayerHoleChance    float64         `json:"wide_bottom_layer_hole_chance"`
			CornerHoleChance             float64         `json:"corner_hole_chance"`
			HangingLeavesChance          float64         `json:"hanging_leaves_chance"`
			HangingLeavesExtensionChance float64         `json:"hanging_leaves_extension_chance"`
		}
		if err := json.Unmarshal(placer.Data, &raw); err != nil {
			return false
		}
		leafRadius := max(0, g.sampleIntProvider(raw.Radius, rng))
		foliageHeight := max(0, g.sampleIntProvider(raw.Height, rng))
		foliagePos := top.Add(cube.Pos{0, g.sampleIntProvider(raw.Offset, rng), 0})
		currentRadius := max(leafRadius-1, 0)
		g.placeTreeLeafRow(c, foliagePos, max(currentRadius-2, 0), foliageHeight-3, doubleTrunk, leaf, minY, maxY, rng, cherryFoliageSkip(raw.WideBottomLayerHoleChance, raw.CornerHoleChance), nil)
		g.placeTreeLeafRow(c, foliagePos, max(currentRadius-1, 0), foliageHeight-4, doubleTrunk, leaf, minY, maxY, rng, cherryFoliageSkip(raw.WideBottomLayerHoleChance, raw.CornerHoleChance), nil)
		for y := foliageHeight - 5; y >= 0; y-- {
			g.placeTreeLeafRow(c, foliagePos, currentRadius, y, doubleTrunk, leaf, minY, maxY, rng, cherryFoliageSkip(raw.WideBottomLayerHoleChance, raw.CornerHoleChance), nil)
		}
		g.placeTreeLeafRowWithHangingBelow(c, foliagePos, currentRadius, -1, doubleTrunk, leaf, minY, maxY, rng, cherryFoliageSkip(raw.WideBottomLayerHoleChance, raw.CornerHoleChance), raw.HangingLeavesChance, raw.HangingLeavesExtensionChance)
		g.placeTreeLeafRowWithHangingBelow(c, foliagePos, max(currentRadius-1, 0), -2, doubleTrunk, leaf, minY, maxY, rng, cherryFoliageSkip(raw.WideBottomLayerHoleChance, raw.CornerHoleChance), raw.HangingLeavesChance, raw.HangingLeavesExtensionChance)
		return true
	case "jungle_foliage_placer":
		var raw struct {
			Radius gen.IntProvider `json:"radius"`
			Offset gen.IntProvider `json:"offset"`
			Height int             `json:"height"`
		}
		if err := json.Unmarshal(placer.Data, &raw); err != nil {
			return false
		}
		leafRadius := max(0, g.sampleIntProvider(raw.Radius, rng))
		offset := g.sampleIntProvider(raw.Offset, rng)
		leafHeight := 1 + int(rng.NextInt(2))
		if doubleTrunk {
			leafHeight = raw.Height
		}
		for yo := offset; yo >= offset-leafHeight; yo-- {
			currentRadius := max(leafRadius+1-yo, 0)
			g.placeTreeLeafRow(c, top, currentRadius, yo, doubleTrunk, leaf, minY, maxY, rng, megaFoliageSkip, nil)
		}
		return true
	case "mega_pine_foliage_placer":
		var raw struct {
			Radius      gen.IntProvider `json:"radius"`
			Offset      gen.IntProvider `json:"offset"`
			CrownHeight gen.IntProvider `json:"crown_height"`
		}
		if err := json.Unmarshal(placer.Data, &raw); err != nil {
			return false
		}
		leafRadius := max(0, g.sampleIntProvider(raw.Radius, rng))
		offset := g.sampleIntProvider(raw.Offset, rng)
		foliageHeight := max(1, g.sampleIntProvider(raw.CrownHeight, rng))
		prevRadius := 0
		for yy := top[1] - foliageHeight + offset; yy <= top[1]+offset; yy++ {
			yo := top[1] - yy
			smoothRadius := leafRadius + int(math.Floor(float64(yo)/float64(foliageHeight)*3.5))
			jaggedRadius := smoothRadius
			if yo > 0 && smoothRadius == prevRadius && (yy&1) == 0 {
				jaggedRadius++
			}
			g.placeTreeLeafRow(c, cube.Pos{top[0], yy, top[2]}, jaggedRadius, 0, doubleTrunk, leaf, minY, maxY, rng, megaFoliageSkip, nil)
			prevRadius = smoothRadius
		}
		return true
	default:
		_ = height
		_ = rng
		return false
	}
}

type treeFoliageSkip func(rng *gen.WorldgenRandom, dx, y, dz, currentRadius int, doubleTrunk bool) bool

func (g Generator) placeTreeLeafRow(c *chunk.Chunk, center cube.Pos, currentRadius, y int, doubleTrunk bool, leaf gen.BlockState, minY, maxY int, rng *gen.WorldgenRandom, skip, signedSkip treeFoliageSkip) {
	if currentRadius < 0 {
		return
	}
	offset := 0
	if doubleTrunk {
		offset = 1
	}
	for dx := -currentRadius; dx <= currentRadius+offset; dx++ {
		for dz := -currentRadius; dz <= currentRadius+offset; dz++ {
			if signedSkip != nil && signedSkip(rng, dx, y, dz, currentRadius, doubleTrunk) {
				continue
			}
			minDx, minDz := abs(dx), abs(dz)
			if doubleTrunk {
				minDx = min(abs(dx), abs(dx-1))
				minDz = min(abs(dz), abs(dz-1))
			}
			if skip != nil && skip(rng, minDx, y, minDz, currentRadius, doubleTrunk) {
				continue
			}
			candidate := center.Add(cube.Pos{dx, y, dz})
			if candidate[1] <= minY || candidate[1] > maxY {
				continue
			}
			currentRID := c.Block(uint8(candidate[0]&15), int16(candidate[1]), uint8(candidate[2]&15), 0)
			if currentRID != g.airRID {
				continue
			}
			_ = g.setBlockStateDirect(c, candidate, leaf)
		}
	}
}

func (g Generator) placeTreeLeafRowWithHangingBelow(c *chunk.Chunk, center cube.Pos, currentRadius, y int, doubleTrunk bool, leaf gen.BlockState, minY, maxY int, rng *gen.WorldgenRandom, skip treeFoliageSkip, hangingChance, extensionChance float64) {
	g.placeTreeLeafRow(c, center, currentRadius, y, doubleTrunk, leaf, minY, maxY, rng, skip, nil)
	offset := 0
	if doubleTrunk {
		offset = 1
	}
	for dx := -currentRadius; dx <= currentRadius+offset; dx++ {
		for dz := -currentRadius; dz <= currentRadius+offset; dz++ {
			if abs(dx) != currentRadius && abs(dz) != currentRadius && (!doubleTrunk || (dx != currentRadius+offset && dz != currentRadius+offset)) {
				continue
			}
			candidate := center.Add(cube.Pos{dx, y - 1, dz})
			if candidate[1] <= minY || candidate[1] > maxY {
				continue
			}
			above := candidate.Side(cube.FaceUp)
			if !g.isSameTreeLeaf(c, above, leaf) || !g.positionInChunk(candidate, int(center[0])>>4, int(center[2])>>4, minY, maxY) {
				continue
			}
			if c.Block(uint8(candidate[0]&15), int16(candidate[1]), uint8(candidate[2]&15), 0) != g.airRID || rng.NextDouble() > hangingChance {
				continue
			}
			_ = g.setBlockStateDirect(c, candidate, leaf)
			extension := candidate.Side(cube.FaceDown)
			if extension[1] > minY && extension[1] <= maxY && c.Block(uint8(extension[0]&15), int16(extension[1]), uint8(extension[2]&15), 0) == g.airRID && rng.NextDouble() <= extensionChance {
				_ = g.setBlockStateDirect(c, extension, leaf)
			}
		}
	}
}

func (g Generator) isSameTreeLeaf(c *chunk.Chunk, pos cube.Pos, leaf gen.BlockState) bool {
	if pos[1] <= c.Range().Min() || pos[1] > c.Range().Max() {
		return false
	}
	rid := c.Block(uint8(pos[0]&15), int16(pos[1]), uint8(pos[2]&15), 0)
	if rid == g.airRID {
		return false
	}
	b, ok := world.BlockByRuntimeID(rid)
	if !ok {
		return false
	}
	name, _ := b.EncodeBlock()
	return strings.TrimPrefix(name, "minecraft:") == strings.TrimPrefix(leaf.Name, "minecraft:")
}

func blobFoliageSkip(rng *gen.WorldgenRandom, dx, y, dz, currentRadius int, doubleTrunk bool) bool {
	return dx == currentRadius && dz == currentRadius && (rng.NextInt(2) == 0 || y == 0)
}

func fancyFoliageSkip(rng *gen.WorldgenRandom, dx, y, dz, currentRadius int, doubleTrunk bool) bool {
	return (float64(dx)+0.5)*(float64(dx)+0.5)+(float64(dz)+0.5)*(float64(dz)+0.5) > float64(currentRadius*currentRadius)
}

func bushFoliageSkip(rng *gen.WorldgenRandom, dx, y, dz, currentRadius int, doubleTrunk bool) bool {
	return dx == currentRadius && dz == currentRadius && rng.NextInt(2) == 0
}

func coniferFoliageSkip(rng *gen.WorldgenRandom, dx, y, dz, currentRadius int, doubleTrunk bool) bool {
	return dx == currentRadius && dz == currentRadius && currentRadius > 0
}

func acaciaFoliageSkip(rng *gen.WorldgenRandom, dx, y, dz, currentRadius int, doubleTrunk bool) bool {
	if y == 0 {
		return (dx > 1 || dz > 1) && dx != 0 && dz != 0
	}
	return dx == currentRadius && dz == currentRadius && currentRadius > 0
}

func darkOakSignedSkip(rng *gen.WorldgenRandom, dx, y, dz, currentRadius int, doubleTrunk bool) bool {
	if y != 0 || !doubleTrunk {
		return false
	}
	return (dx == -currentRadius || dx >= currentRadius) && (dz == -currentRadius || dz >= currentRadius)
}

func darkOakFoliageSkip(rng *gen.WorldgenRandom, dx, y, dz, currentRadius int, doubleTrunk bool) bool {
	if y == -1 && !doubleTrunk {
		return dx == currentRadius && dz == currentRadius
	}
	if y == 1 {
		return dx+dz > currentRadius*2-2
	}
	return false
}

func megaFoliageSkip(rng *gen.WorldgenRandom, dx, y, dz, currentRadius int, doubleTrunk bool) bool {
	return dx+dz >= 7 || dx*dx+dz*dz > currentRadius*currentRadius
}

func cherryFoliageSkip(wideBottomLayerHoleChance, cornerHoleChance float64) treeFoliageSkip {
	return func(rng *gen.WorldgenRandom, dx, y, dz, currentRadius int, doubleTrunk bool) bool {
		if y == -1 && (dx == currentRadius || dz == currentRadius) && rng.NextDouble() < wideBottomLayerHoleChance {
			return true
		}
		corner := dx == currentRadius && dz == currentRadius
		wideLayer := currentRadius > 2
		if wideLayer {
			return corner || (dx+dz > currentRadius*2-2 && rng.NextDouble() < cornerHoleChance)
		}
		return corner && rng.NextDouble() < cornerHoleChance
	}
}

func (g Generator) multifaceStateAt(c *chunk.Chunk, pos cube.Pos, cfg gen.MultifaceGrowthConfig, chunkX, chunkZ, minY, maxY int) (gen.BlockState, bool) {
	type faceProp struct {
		face cube.Face
		key  string
	}
	faces := []faceProp{
		{cube.FaceUp, "down"},
		{cube.FaceDown, "up"},
		{cube.FaceNorth, "south"},
		{cube.FaceSouth, "north"},
		{cube.FaceEast, "west"},
		{cube.FaceWest, "east"},
	}
	props := map[string]string{
		"down":        "false",
		"up":          "false",
		"north":       "false",
		"south":       "false",
		"east":        "false",
		"west":        "false",
		"waterlogged": "false",
	}
	for _, entry := range faces {
		switch entry.face {
		case cube.FaceUp:
			if !cfg.CanPlaceOnFloor {
				continue
			}
		case cube.FaceDown:
			if !cfg.CanPlaceOnCeiling {
				continue
			}
		default:
			if !cfg.CanPlaceOnWall {
				continue
			}
		}
		support := pos.Side(entry.face)
		if !g.positionInChunk(support, chunkX, chunkZ, minY, maxY) {
			continue
		}
		if slices.Contains(cfg.CanBePlacedOn, g.blockNameAt(c, support)) {
			props[entry.key] = "true"
			return gen.BlockState{Name: strings.TrimPrefix(cfg.Block, "minecraft:"), Properties: props}, true
		}
	}
	return gen.BlockState{}, false
}

func (g Generator) isSolidInChunk(c *chunk.Chunk, pos cube.Pos, chunkX, chunkZ, minY, maxY int) bool {
	if !g.positionInFeatureScope(pos, chunkX, chunkZ, minY, maxY) {
		return false
	}
	return g.isSolidRID(g.runtimeIDAt(c, pos))
}

func (g Generator) findFloorAndCeiling(c *chunk.Chunk, pos cube.Pos, searchRange, chunkX, chunkZ, minY, maxY int) (cube.Pos, cube.Pos, bool) {
	floor, ceiling := cube.Pos{}, cube.Pos{}
	foundFloor, foundCeiling := false, false
	for y := pos[1]; y >= max(minY, pos[1]-searchRange); y-- {
		candidate := cube.Pos{pos[0], y, pos[2]}
		if g.isSolidInChunk(c, candidate, chunkX, chunkZ, minY, maxY) {
			floor = candidate
			foundFloor = true
			break
		}
	}
	for y := pos[1]; y <= min(maxY, pos[1]+searchRange); y++ {
		candidate := cube.Pos{pos[0], y, pos[2]}
		if g.isSolidInChunk(c, candidate, chunkX, chunkZ, minY, maxY) {
			ceiling = candidate
			foundCeiling = true
			break
		}
	}
	return floor, ceiling, foundFloor && foundCeiling && ceiling[1]-floor[1] > 2
}

func pointedDripstoneState(direction, thickness string) gen.BlockState {
	return gen.BlockState{
		Name: "pointed_dripstone",
		Properties: map[string]string{
			"thickness":          thickness,
			"vertical_direction": direction,
			"waterlogged":        "false",
		},
	}
}

func (g Generator) featureCountNoise(x, z int) float64 {
	return g.surface.SurfaceSecondary(x, z)*2.0 - 1.0
}

func (g Generator) noiseThresholdProviderValue(provider gen.StateProvider, cfg gen.NoiseThresholdStateProviderConfig, pos cube.Pos) float64 {
	key := string(provider.Data)
	noise, ok := g.featureNoiseCache.Lookup(key)
	if !ok {
		xr := gen.NewXoroshiro128FromSeed(cfg.Seed)
		noise = gen.NewDoublePerlinNoise(&xr, cfg.Noise.Amplitudes, cfg.Noise.FirstOctave)
		g.featureNoiseCache.Store(key, noise)
	}
	return noise.Sample(float64(pos[0])*cfg.Scale, 0.0, float64(pos[2])*cfg.Scale)
}

func (g Generator) sampleIntProvider(provider gen.IntProvider, rng *gen.WorldgenRandom) int {
	switch provider.Kind {
	case "constant":
		if provider.Constant != nil {
			return *provider.Constant
		}
	case "uniform":
		if provider.MaxInclusive <= provider.MinInclusive {
			return provider.MinInclusive
		}
		return provider.MinInclusive + int(rng.NextInt(uint32(provider.MaxInclusive-provider.MinInclusive+1)))
	case "biased_to_bottom":
		if provider.MaxInclusive <= provider.MinInclusive {
			return provider.MinInclusive
		}
		span := provider.MaxInclusive - provider.MinInclusive + 1
		return provider.MinInclusive + int(rng.NextInt(uint32(max(1, int(rng.NextInt(uint32(span))+1)))))
	case "weighted_list":
		total := 0
		for _, entry := range provider.Distribution {
			total += entry.Weight
		}
		if total <= 0 {
			return 0
		}
		pick := int(rng.NextInt(uint32(total)))
		for _, entry := range provider.Distribution {
			pick -= entry.Weight
			if pick < 0 {
				return entry.Data
			}
		}
	case "clamped":
		if provider.Source == nil {
			return provider.MinInclusive
		}
		return clamp(g.sampleIntProvider(*provider.Source, rng), provider.MinInclusive, provider.MaxInclusive)
	case "clamped_normal":
		return clamp(int(math.Round(g.normalFloat64(rng, provider.Mean, provider.Deviation))), provider.MinInclusive, provider.MaxInclusive)
	}
	return 0
}

func (g Generator) normalFloat64(rng *gen.WorldgenRandom, mean, deviation float64) float64 {
	u1 := rng.NextDouble()
	if u1 <= 0 {
		u1 = math.SmallestNonzeroFloat64
	}
	u2 := rng.NextDouble()
	z0 := math.Sqrt(-2.0*math.Log(u1)) * math.Cos(2.0*math.Pi*u2)
	return mean + z0*deviation
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func lerp(t, a, b float64) float64 {
	return a + (b-a)*t
}

func (g Generator) signedSpread(rng *gen.WorldgenRandom, spread int) int {
	if spread <= 0 {
		return 0
	}
	return int(rng.NextInt(uint32(spread*2+1))) - spread
}

func (g Generator) positionInChunk(pos cube.Pos, chunkX, chunkZ, minY, maxY int) bool {
	return pos[0] >= chunkX*16 && pos[0] < chunkX*16+16 &&
		pos[2] >= chunkZ*16 && pos[2] < chunkZ*16+16 &&
		pos[1] >= minY && pos[1] <= maxY
}

func (g Generator) decorationSeed(chunkX, chunkZ int) int64 {
	rng := gen.NewWorldgenRandomXoroshiro(0)
	return rng.SetDecorationSeed(g.seed, chunkX*16, chunkZ*16)
}

func (g Generator) featureRNG(decorationSeed int64, featureIndex int, step gen.GenerationStep) *gen.WorldgenRandom {
	rng := gen.NewWorldgenRandomXoroshiro(0)
	rng.SetFeatureSeed(decorationSeed, featureIndex, int(step))
	return rng
}
