package vanilla

import (
	"fmt"
	"hash/fnv"
	"math"
	"sort"
	"sync"

	gen "github.com/bedrock-mc/vanilla-gen/gen"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/df-mc/dragonfly/server/world/chunk"
)

type structureStartCache struct {
	mu    sync.RWMutex
	byKey map[structureStartKey]cachedStructureStart
}

type cachedStructureStart struct {
	loaded bool
	exists bool
	start  plannedStructureStart
}

type structureStartKey struct {
	setName string
	chunkX  int32
	chunkZ  int32
}

type plannedStructureStart struct {
	setName           string
	structureName     string
	templateName      string
	terrainAdaptation string
	startChunk        world.ChunkPos
	origin            cube.Pos
	size              [3]int
	rootOrigin        cube.Pos
	rootSize          [3]int
	pieces            []plannedStructurePiece
}

type PlannedStructureInfo struct {
	StructureSet string
	Structure    string
	Template     string
	StartChunk   world.ChunkPos
	Origin       cube.Pos
	Size         [3]int
	PaletteNames []string
}

type structurePlanner struct {
	setName             string
	placementType       string
	randomPlacement     gen.RandomSpreadPlacement
	concentricPlacement gen.ConcentricRingsPlacement
	candidates          []structurePlannerCandidate
	candidateByName     map[string]int
	totalWeight         int
	maxBackreachX       int
	maxBackreachZ       int
}

type structurePlannerCandidate struct {
	structureName       string
	structureType       string
	biomeTag            string
	generationStep      gen.GenerationStep
	hasGenerationStep   bool
	terrainAdaptation   string
	weight              int
	generic             gen.GenericStructureDef
	netherFossil        gen.NetherFossilStructureDef
	jigsaw              gen.JigsawStructureDef
	shipwreck           gen.ShipwreckStructureDef
	oceanRuin           gen.OceanRuinStructureDef
	ruinedPortal        gen.RuinedPortalStructureDef
	startTemplates      []weightedStartTemplate
	totalTemplateWeight int
	maxBackreachX       int
	maxBackreachZ       int
}

type weightedStartTemplate struct {
	name       string
	weight     int
	size       [3]int
	ignoreAir  bool
	projection string
	jigsaws    []structureJigsaw
	processors []structureProcessor
}

type structureStepEntry struct {
	structureName  string
	plannerIndex   int
	structureIndex int
}

var javaStructureBootstrapOrder = []string{
	"pillager_outpost",
	"mineshaft",
	"mineshaft_mesa",
	"mansion",
	"jungle_pyramid",
	"desert_pyramid",
	"igloo",
	"shipwreck",
	"shipwreck_beached",
	"swamp_hut",
	"stronghold",
	"monument",
	"ocean_ruin_cold",
	"ocean_ruin_warm",
	"fortress",
	"nether_fossil",
	"end_city",
	"buried_treasure",
	"bastion_remnant",
	"village_plains",
	"village_desert",
	"village_savanna",
	"village_snowy",
	"village_taiga",
	"ruined_portal",
	"ruined_portal_desert",
	"ruined_portal_jungle",
	"ruined_portal_swamp",
	"ruined_portal_mountain",
	"ruined_portal_ocean",
	"ruined_portal_nether",
	"ancient_city",
	"trail_ruins",
	"trial_chambers",
}

func newStructureStartCache() *structureStartCache {
	return &structureStartCache{byKey: make(map[structureStartKey]cachedStructureStart)}
}

func buildStructureStepOrder(planners []structurePlanner) [][]structureStepEntry {
	order := make([][]structureStepEntry, featureStepCount)
	indexByStep := make([]int, featureStepCount)
	seenStructures := make(map[string]struct{}, len(javaStructureBootstrapOrder))

	appendEntry := func(structureName string) {
		for plannerIndex, planner := range planners {
			step, ok := planner.generationStepForStructure(structureName)
			if !ok {
				continue
			}
			stepIndex := int(step)
			if stepIndex < 0 || stepIndex >= featureStepCount {
				return
			}
			order[stepIndex] = append(order[stepIndex], structureStepEntry{
				structureName:  structureName,
				plannerIndex:   plannerIndex,
				structureIndex: indexByStep[stepIndex],
			})
			indexByStep[stepIndex]++
			seenStructures[structureName] = struct{}{}
			return
		}
	}

	for _, structureName := range javaStructureBootstrapOrder {
		appendEntry(structureName)
	}
	for plannerIndex, planner := range planners {
		names := make([]string, 0, len(planner.candidateByName))
		for structureName := range planner.candidateByName {
			names = append(names, structureName)
		}
		sort.Strings(names)
		for _, structureName := range names {
			if _, ok := seenStructures[structureName]; ok {
				continue
			}
			step, ok := planner.generationStepForStructure(structureName)
			if !ok {
				continue
			}
			stepIndex := int(step)
			if stepIndex < 0 || stepIndex >= featureStepCount {
				continue
			}
			order[stepIndex] = append(order[stepIndex], structureStepEntry{
				structureName:  structureName,
				plannerIndex:   plannerIndex,
				structureIndex: indexByStep[stepIndex],
			})
			indexByStep[stepIndex]++
		}
	}
	return order
}

func (c *structureStartCache) Lookup(key structureStartKey) (plannedStructureStart, bool, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.byKey[key]
	if !ok || !entry.loaded {
		return plannedStructureStart{}, false, false
	}
	return entry.start, entry.exists, true
}

func (c *structureStartCache) Store(key structureStartKey, start plannedStructureStart, exists bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.byKey[key] = cachedStructureStart{loaded: true, exists: exists, start: start}
}

func buildStructurePlanners(worldgen *gen.WorldgenRegistry, templates *gen.StructureTemplateRegistry, dim world.Dimension) []structurePlanner {
	if worldgen == nil || templates == nil {
		return nil
	}

	names := worldgen.StructureSetNames()
	planners := make([]structurePlanner, 0, len(names))
	for _, setName := range names {
		set, err := worldgen.StructureSet(setName)
		if err != nil {
			continue
		}

		planner := structurePlanner{
			setName:         setName,
			placementType:   normalizeIdentifierName(set.Placement.Type),
			candidateByName: make(map[string]int),
		}
		switch planner.placementType {
		case "random_spread":
			placement, err := set.Placement.RandomSpread()
			if err != nil {
				continue
			}
			planner.randomPlacement = placement
		case "concentric_rings":
			placement, err := set.Placement.ConcentricRings()
			if err != nil {
				continue
			}
			planner.concentricPlacement = placement
		default:
			continue
		}
		for _, entry := range set.Structures {
			if entry.Weight <= 0 {
				continue
			}
			structureName := normalizeStructureName(entry.Structure)
			def, err := worldgen.Structure(structureName)
			if err != nil {
				continue
			}
			if !structureSupportedInDimension(structureName, def.Type, dim) {
				continue
			}
			candidate := structurePlannerCandidate{
				structureName: structureName,
				structureType: def.Type,
				weight:        entry.Weight,
			}

			switch def.Type {
			case "jigsaw":
				jigsaw, err := def.Jigsaw()
				if err != nil {
					continue
				}
				startTemplates, totalTemplateWeight, maxBackreachX, maxBackreachZ := buildStartTemplates(worldgen, templates, jigsaw.StartPool)
				if len(startTemplates) == 0 || totalTemplateWeight <= 0 {
					continue
				}
				candidate.biomeTag = normalizeStructureTag(jigsaw.Biomes)
				candidate.terrainAdaptation = normalizeIdentifierName(jigsaw.TerrainAdaptation)
				candidate.jigsaw = jigsaw
				candidate.startTemplates = startTemplates
				candidate.totalTemplateWeight = totalTemplateWeight
				candidate.maxBackreachX = maxBackreachX
				candidate.maxBackreachZ = maxBackreachZ
				reachChunks := templateBackreachChunks(jigsaw.MaxDistanceFromCenter*2 + 1)
				if reachChunks > candidate.maxBackreachX {
					candidate.maxBackreachX = reachChunks
				}
				if reachChunks > candidate.maxBackreachZ {
					candidate.maxBackreachZ = reachChunks
				}
			case "igloo", "buried_treasure", "swamp_hut", "desert_pyramid", "jungle_temple", "stronghold", "fortress", "mineshaft", "ocean_monument", "woodland_mansion":
				generic, err := def.Generic()
				if err != nil {
					continue
				}
				candidate.biomeTag = normalizeStructureTag(generic.Biomes)
				candidate.terrainAdaptation = normalizeIdentifierName(generic.TerrainAdaptation)
				candidate.generic = generic
				candidate.maxBackreachX, candidate.maxBackreachZ = estimateDirectStructureBackreach(structureName, def.Type)
			case "end_city":
				generic, err := def.Generic()
				if err != nil {
					continue
				}
				candidate.biomeTag = normalizeStructureTag(generic.Biomes)
				candidate.terrainAdaptation = normalizeIdentifierName(generic.TerrainAdaptation)
				candidate.generic = generic
				candidate.maxBackreachX, candidate.maxBackreachZ = estimateDirectStructureBackreach(structureName, def.Type)
			case "nether_fossil":
				netherFossil, err := def.NetherFossil()
				if err != nil {
					continue
				}
				candidate.biomeTag = normalizeStructureTag(netherFossil.Biomes)
				candidate.terrainAdaptation = normalizeIdentifierName(netherFossil.TerrainAdaptation)
				candidate.netherFossil = netherFossil
				candidate.maxBackreachX, candidate.maxBackreachZ = estimateDirectStructureBackreach(structureName, def.Type)
			case "shipwreck":
				shipwreck, err := def.Shipwreck()
				if err != nil {
					continue
				}
				candidate.biomeTag = normalizeStructureTag(shipwreck.Biomes)
				candidate.shipwreck = shipwreck
				candidate.maxBackreachX, candidate.maxBackreachZ = estimateDirectStructureBackreach(structureName, def.Type)
			case "ocean_ruin":
				oceanRuin, err := def.OceanRuin()
				if err != nil {
					continue
				}
				candidate.biomeTag = normalizeStructureTag(oceanRuin.Biomes)
				candidate.oceanRuin = oceanRuin
				candidate.maxBackreachX, candidate.maxBackreachZ = estimateDirectStructureBackreach(structureName, def.Type)
			case "ruined_portal":
				ruinedPortal, err := def.RuinedPortal()
				if err != nil {
					continue
				}
				candidate.biomeTag = normalizeStructureTag(ruinedPortal.Biomes)
				candidate.ruinedPortal = ruinedPortal
				candidate.maxBackreachX, candidate.maxBackreachZ = estimateDirectStructureBackreach(structureName, def.Type)
			default:
				continue
			}
			step, ok := structurePlannerCandidateStep(candidate)
			if !ok {
				continue
			}
			candidate.generationStep = step
			candidate.hasGenerationStep = true
			planner.candidateByName[structureName] = len(planner.candidates)
			planner.candidates = append(planner.candidates, candidate)
			planner.totalWeight += entry.Weight
			if candidate.maxBackreachX > planner.maxBackreachX {
				planner.maxBackreachX = candidate.maxBackreachX
			}
			if candidate.maxBackreachZ > planner.maxBackreachZ {
				planner.maxBackreachZ = candidate.maxBackreachZ
			}
		}
		if len(planner.candidates) == 0 {
			continue
		}
		planners = append(planners, planner)
	}
	return planners
}

func structureSupportedInDimension(structureName, structureType string, dim world.Dimension) bool {
	switch dim {
	case world.Nether:
		switch structureName {
		case "bastion_remnant", "fortress", "nether_fossil", "ruined_portal_nether":
			return true
		default:
			return false
		}
	case world.End:
		return structureName == "end_city"
	default:
		switch structureType {
		case "end_city", "nether_fossil":
			return false
		default:
			return structureName != "bastion_remnant" && structureName != "ruined_portal_nether"
		}
	}
}

func buildStartTemplates(worldgen *gen.WorldgenRegistry, templates *gen.StructureTemplateRegistry, poolName string) ([]weightedStartTemplate, int, int, int) {
	pool, err := worldgen.TemplatePool(poolName)
	if err != nil {
		return nil, 0, 0, 0
	}

	startTemplates := make([]weightedStartTemplate, 0, len(pool.Elements))
	totalWeight := 0
	maxBackreachX := 0
	maxBackreachZ := 0
	for _, entry := range pool.Elements {
		single, err := entry.Element.Single()
		if err != nil || single.Location == "" || entry.Weight <= 0 {
			continue
		}
		template, err := templates.Template(single.Location)
		if err != nil {
			continue
		}
		startTemplates = append(startTemplates, weightedStartTemplate{
			name:       single.Location,
			weight:     entry.Weight,
			size:       template.Size,
			ignoreAir:  entry.Element.ElementType == "legacy_single_pool_element",
			projection: normalizeIdentifierName(single.Projection),
			jigsaws:    extractTemplateJigsaws(template),
			processors: compileStructureProcessors(worldgen, single.Processors),
		})
		totalWeight += entry.Weight
		if backreach := templateBackreachChunks(template.Size[0]); backreach > maxBackreachX {
			maxBackreachX = backreach
		}
		if backreach := templateBackreachChunks(template.Size[2]); backreach > maxBackreachZ {
			maxBackreachZ = backreach
		}
	}
	return startTemplates, totalWeight, maxBackreachX, maxBackreachZ
}

func templateBackreachChunks(size int) int {
	if size <= 1 {
		return 0
	}
	return (size - 1) / 16
}

func (g Generator) findStructurePlanner(setName string) (structurePlanner, bool) {
	normalized := normalizeStructureName(setName)
	for _, planner := range g.structurePlanners {
		if planner.setName == normalized {
			return planner, true
		}
	}
	return structurePlanner{}, false
}

func (g Generator) placeStructures(c *chunk.Chunk, biomes sourceBiomeVolume, chunkX, chunkZ, minY, maxY int) {
	if g.structureTemplates == nil || g.structureStarts == nil || len(g.structurePlanners) == 0 {
		return
	}

	surfaceSampler := newStructureHeightSampler(g, minY, maxY)
	for stepIndex := 0; stepIndex < featureStepCount; stepIndex++ {
		g.placeStructuresForStep(c, biomes, chunkX, chunkZ, minY, maxY, gen.GenerationStep(stepIndex), surfaceSampler)
	}
}

func (g Generator) placeStructuresForStep(c *chunk.Chunk, biomes sourceBiomeVolume, chunkX, chunkZ, minY, maxY int, step gen.GenerationStep, surfaceSampler *structureHeightSampler) {
	if g.structureTemplates == nil || g.structureStarts == nil || len(g.structurePlanners) == 0 {
		return
	}
	if surfaceSampler == nil {
		surfaceSampler = newStructureHeightSampler(g, minY, maxY)
	}
	if int(step) < 0 || int(step) >= len(g.structureStepOrder) {
		return
	}
	decorationSeed := g.decorationSeed(chunkX, chunkZ)
	for _, entry := range g.structureStepOrder[step] {
		planner := g.structurePlanners[entry.plannerIndex]
		rng := g.featureRNG(decorationSeed, entry.structureIndex, step)
		g.placePlannerStructureForStepEntry(c, biomes, chunkX, chunkZ, minY, maxY, planner, entry, surfaceSampler, &rng)
	}
}

func (g Generator) placeStructureSet(c *chunk.Chunk, biomes sourceBiomeVolume, chunkX, chunkZ, minY, maxY int, planner structurePlanner, surfaceSampler *structureHeightSampler) {
	for stepIndex := 0; stepIndex < featureStepCount; stepIndex++ {
		g.placeStructureSetForStep(c, biomes, chunkX, chunkZ, minY, maxY, planner, gen.GenerationStep(stepIndex), surfaceSampler)
	}
}

func (g Generator) placeStructureSetForStep(c *chunk.Chunk, biomes sourceBiomeVolume, chunkX, chunkZ, minY, maxY int, planner structurePlanner, step gen.GenerationStep, surfaceSampler *structureHeightSampler) {
	for _, entry := range g.structureEntriesForPlannerStep(planner, step) {
		rng := g.featureRNG(g.decorationSeed(chunkX, chunkZ), entry.structureIndex, step)
		g.placePlannerStructureForStepEntry(c, biomes, chunkX, chunkZ, minY, maxY, planner, entry, surfaceSampler, &rng)
	}
}

func (g Generator) structureEntriesForPlannerStep(planner structurePlanner, step gen.GenerationStep) []structureStepEntry {
	if int(step) >= 0 && int(step) < len(g.structureStepOrder) {
		var entries []structureStepEntry
		for _, entry := range g.structureStepOrder[step] {
			if entry.plannerIndex < 0 || entry.plannerIndex >= len(g.structurePlanners) {
				continue
			}
			if g.structurePlanners[entry.plannerIndex].setName == planner.setName {
				entries = append(entries, entry)
			}
		}
		if len(entries) != 0 {
			return entries
		}
	}
	names := make([]string, 0, len(planner.candidateByName))
	for structureName := range planner.candidateByName {
		names = append(names, structureName)
	}
	sort.Strings(names)
	entries := make([]structureStepEntry, 0, len(names))
	for structureIndex, structureName := range names {
		startStep, ok := planner.generationStepForStructure(structureName)
		if !ok || startStep != step {
			continue
		}
		entries = append(entries, structureStepEntry{
			structureName:  structureName,
			structureIndex: structureIndex,
		})
	}
	return entries
}

func (g Generator) placePlannerStructureForStepEntry(c *chunk.Chunk, biomes sourceBiomeVolume, chunkX, chunkZ, minY, maxY int, planner structurePlanner, entry structureStepEntry, surfaceSampler *structureHeightSampler, rng *gen.Xoroshiro128) {
	for _, startChunk := range g.plannerPotentialStartChunksNearChunk(planner, chunkX, chunkZ, 0, 0) {
		start, ok := g.planStructureStart(planner, startChunk, minY, maxY, surfaceSampler)
		if !ok || !structureIntersectsChunk(start, chunkX, chunkZ, minY, maxY) {
			continue
		}
		if start.structureName != entry.structureName {
			continue
		}
		g.placePlannedStructure(c, biomes, chunkX, chunkZ, minY, maxY, start, rng)
	}
}

func (planner structurePlanner) generationStepForStructure(structureName string) (gen.GenerationStep, bool) {
	index, ok := planner.candidateByName[structureName]
	if !ok || index < 0 || index >= len(planner.candidates) {
		return 0, false
	}
	candidate := planner.candidates[index]
	if !candidate.hasGenerationStep {
		return 0, false
	}
	return candidate.generationStep, true
}

func structurePlannerCandidateStep(candidate structurePlannerCandidate) (gen.GenerationStep, bool) {
	switch candidate.structureType {
	case "jigsaw":
		return parseStructureGenerationStep(candidate.jigsaw.Step)
	case "igloo", "buried_treasure", "swamp_hut", "desert_pyramid", "jungle_temple", "stronghold", "fortress", "mineshaft", "ocean_monument", "woodland_mansion", "end_city":
		return parseStructureGenerationStep(candidate.generic.Step)
	case "nether_fossil":
		return parseStructureGenerationStep(candidate.netherFossil.Step)
	case "shipwreck":
		return parseStructureGenerationStep(candidate.shipwreck.Step)
	case "ocean_ruin":
		return parseStructureGenerationStep(candidate.oceanRuin.Step)
	case "ruined_portal":
		return parseStructureGenerationStep(candidate.ruinedPortal.Step)
	default:
		return 0, false
	}
}

func parseStructureGenerationStep(step string) (gen.GenerationStep, bool) {
	switch normalizeIdentifierName(step) {
	case "raw_generation":
		return gen.GenerationStepRawGeneration, true
	case "lakes":
		return gen.GenerationStepLakes, true
	case "local_modifications":
		return gen.GenerationStepLocalModifications, true
	case "underground_structures":
		return gen.GenerationStepUndergroundStructures, true
	case "surface_structures":
		return gen.GenerationStepSurfaceStructures, true
	case "strongholds":
		return gen.GenerationStepStrongholds, true
	case "underground_ores":
		return gen.GenerationStepUndergroundOres, true
	case "underground_decoration":
		return gen.GenerationStepUndergroundDecoration, true
	case "fluid_springs":
		return gen.GenerationStepFluidSprings, true
	case "vegetal_decoration":
		return gen.GenerationStepVegetalDecoration, true
	case "top_layer_modification":
		return gen.GenerationStepTopLayerModification, true
	default:
		return 0, false
	}
}

func randomSpreadMinGrid(startMinChunk, spacing, separation int) int {
	if spacing <= 0 {
		return 0
	}
	maxOffset := spacing - separation - 1
	if maxOffset < 0 {
		maxOffset = 0
	}
	return ceilDiv(startMinChunk-maxOffset, spacing)
}

func ceilDiv(value, divisor int) int {
	return -floorDiv(-value, divisor)
}

func (g Generator) planStructureStart(planner structurePlanner, startChunk world.ChunkPos, minY, maxY int, surfaceSampler *structureHeightSampler) (plannedStructureStart, bool) {
	cacheKey := structureStartKey{setName: planner.setName, chunkX: startChunk[0], chunkZ: startChunk[1]}
	if start, exists, ok := g.structureStarts.Lookup(cacheKey); ok {
		return start, exists
	}
	if planner.placementType == "random_spread" {
		if !structurePlacementAllows(g.seed, planner.randomPlacement, int(startChunk[0]), int(startChunk[1])) {
			g.structureStarts.Store(cacheKey, plannedStructureStart{}, false)
			return plannedStructureStart{}, false
		}
		if g.structurePlacementExcludedByOtherSet(planner, startChunk, minY, maxY, surfaceSampler) {
			g.structureStarts.Store(cacheKey, plannedStructureStart{}, false)
			return plannedStructureStart{}, false
		}
	}

	startX := int(startChunk[0]) * 16
	startZ := int(startChunk[1]) * 16
	if surfaceSampler == nil {
		surfaceSampler = newStructureHeightSampler(g, minY, maxY)
	}
	surfaceHeightmapY := surfaceSampler.worldSurfaceLevelAt(startX+8, startZ+8)
	surfaceY := clamp(surfaceHeightmapY-1, minY, maxY)
	surfaceBiome := g.biomeSource.GetBiome(startX+8, surfaceY, startZ+8)

	candidate, ok := g.chooseStructureForPlanner(planner, surfaceBiome, startChunk)
	if !ok {
		g.structureStarts.Store(cacheKey, plannedStructureStart{}, false)
		return plannedStructureStart{}, false
	}
	if !g.structurePlanningAllowedAt(candidate, startX, startZ, surfaceY) {
		g.structureStarts.Store(cacheKey, plannedStructureStart{}, false)
		return plannedStructureStart{}, false
	}

	rng := g.structureRNG(planner.setName, startChunk)
	var (
		templateName  string
		pieces        []plannedStructurePiece
		overallBounds structureBox
		rootOrigin    cube.Pos
		rootSize      [3]int
		okBuild       bool
	)
	if candidate.structureType == "jigsaw" {
		startTemplate, ok := chooseStartTemplate(candidate, &rng)
		if !ok {
			g.structureStarts.Store(cacheKey, plannedStructureStart{}, false)
			return plannedStructureStart{}, false
		}
		templateName = startTemplate.name
		pieces, overallBounds, rootOrigin, rootSize, okBuild = g.buildPlannedStructure(candidate, startTemplate, startX, startZ, surfaceSampler, &rng)
	} else {
		templateName, pieces, overallBounds, rootOrigin, rootSize, okBuild = g.buildPlannedDirectStructure(candidate, planner.randomPlacement, startChunk, startX, startZ, surfaceY, surfaceSampler, &rng)
	}
	if !okBuild || len(pieces) == 0 {
		g.structureStarts.Store(cacheKey, plannedStructureStart{}, false)
		return plannedStructureStart{}, false
	}
	overallOrigin, overallSize := overallBounds.originAndSize()
	start := plannedStructureStart{
		setName:           planner.setName,
		structureName:     candidate.structureName,
		templateName:      templateName,
		terrainAdaptation: candidate.terrainAdaptation,
		startChunk:        startChunk,
		origin:            overallOrigin,
		size:              overallSize,
		rootOrigin:        rootOrigin,
		rootSize:          rootSize,
		pieces:            pieces,
	}
	g.structureStarts.Store(cacheKey, start, true)
	return start, true
}

func (g Generator) structurePlacementExcludedByOtherSet(planner structurePlanner, startChunk world.ChunkPos, minY, maxY int, surfaceSampler *structureHeightSampler) bool {
	otherSet := normalizeStructureName(planner.randomPlacement.ExclusionZone.OtherSet)
	chunkCount := planner.randomPlacement.ExclusionZone.ChunkCount
	if otherSet == "" || chunkCount <= 0 || otherSet == planner.setName {
		return false
	}

	otherPlanner, ok := g.findStructurePlanner(otherSet)
	if !ok {
		return false
	}
	if surfaceSampler == nil {
		surfaceSampler = newStructureHeightSampler(g, minY, maxY)
	}

	minChunkX := int(startChunk[0]) - chunkCount
	maxChunkX := int(startChunk[0]) + chunkCount
	minChunkZ := int(startChunk[1]) - chunkCount
	maxChunkZ := int(startChunk[1]) + chunkCount
	for _, otherStartChunk := range g.plannerPotentialStartChunksNearChunk(otherPlanner, int(startChunk[0]), int(startChunk[1]), chunkCount, chunkCount) {
		if int(otherStartChunk[0]) < minChunkX || int(otherStartChunk[0]) > maxChunkX || int(otherStartChunk[1]) < minChunkZ || int(otherStartChunk[1]) > maxChunkZ {
			continue
		}
		if _, exists := g.planStructureStart(otherPlanner, otherStartChunk, minY, maxY, surfaceSampler); exists {
			return true
		}
	}
	return false
}

func (g Generator) chooseStructureForPlanner(planner structurePlanner, biome gen.Biome, startChunk world.ChunkPos) (structurePlannerCandidate, bool) {
	if len(planner.candidates) == 0 {
		return structurePlannerCandidate{}, false
	}

	if len(planner.candidates) == 1 {
		if g.structureCandidateAllowed(planner.candidates[0], biome) {
			return planner.candidates[0], true
		}
		return structurePlannerCandidate{}, false
	}

	totalWeight := 0
	for _, candidate := range planner.candidates {
		if candidate.weight <= 0 || !g.structureCandidateAllowed(candidate, biome) {
			continue
		}
		totalWeight += candidate.weight
	}
	if totalWeight <= 0 {
		return structurePlannerCandidate{}, false
	}

	rng := g.structureRNG(planner.setName+":structure", startChunk)
	pick := int(rng.NextInt(uint32(totalWeight)))
	for _, candidate := range planner.candidates {
		if candidate.weight <= 0 {
			continue
		}
		if !g.structureCandidateAllowed(candidate, biome) {
			continue
		}
		if pick < candidate.weight {
			return candidate, true
		}
		pick -= candidate.weight
	}
	return structurePlannerCandidate{}, false
}

func (g Generator) structurePlanningAllowedAt(candidate structurePlannerCandidate, startX, startZ, surfaceY int) bool {
	switch candidate.structureType {
	case "ocean_monument":
		return g.oceanMonumentSurroundingsAllowAt(startX, startZ, surfaceY)
	default:
		return true
	}
}

func chooseStartTemplate(candidate structurePlannerCandidate, rng *gen.Xoroshiro128) (weightedStartTemplate, bool) {
	if candidate.totalTemplateWeight <= 0 {
		return weightedStartTemplate{}, false
	}

	pick := int(rng.NextInt(uint32(candidate.totalTemplateWeight)))
	for _, startTemplate := range candidate.startTemplates {
		if startTemplate.weight <= 0 {
			continue
		}
		if pick < startTemplate.weight {
			return startTemplate, true
		}
		pick -= startTemplate.weight
	}
	return weightedStartTemplate{}, false
}

func (g Generator) resolveJigsawStartY(def gen.JigsawStructureDef, blockX, blockZ, minY, maxY int, rng *gen.Xoroshiro128) int {
	base := g.sampleStructureHeightProvider(def.StartHeight, minY, maxY, rng)
	if def.ProjectStartToHeight != "" {
		return g.worldSurfaceLevelAt(blockX, blockZ, minY, maxY) + base
	}
	return base
}

func (g Generator) sampleStructureHeightProvider(provider gen.StructureHeightProvider, minY, maxY int, rng *gen.Xoroshiro128) int {
	switch provider.Kind {
	case "constant":
		return resolveVerticalAnchor(provider.Anchor, minY, maxY)
	case "uniform", "trapezoid", "biased_to_bottom", "very_biased_to_bottom":
		minValue := resolveVerticalAnchor(provider.MinInclusive, minY, maxY)
		maxValue := resolveVerticalAnchor(provider.MaxInclusive, minY, maxY)
		if maxValue <= minValue {
			return minValue
		}
		return minValue + int(rng.NextInt(uint32(maxValue-minValue+1)))
	default:
		return minY
	}
}

func resolveVerticalAnchor(anchor gen.VerticalAnchor, minY, maxY int) int {
	switch anchor.Kind {
	case "above_bottom":
		return minY + anchor.Value
	case "below_top":
		return maxY - anchor.Value
	default:
		return anchor.Value
	}
}

func (g Generator) preliminarySurfaceLevelAt(blockX, blockZ, minY, maxY int) int {
	return newStructureHeightSampler(g, minY, maxY).preliminarySurfaceLevelAt(blockX, blockZ)
}

func (g Generator) worldSurfaceLevelAt(blockX, blockZ, minY, maxY int) int {
	return newStructureHeightSampler(g, minY, maxY).worldSurfaceLevelAt(blockX, blockZ)
}

func structureIntersectsChunk(start plannedStructureStart, chunkX, chunkZ, minY, maxY int) bool {
	return structureBox{
		minX: start.origin[0],
		minY: start.origin[1],
		minZ: start.origin[2],
		maxX: start.origin[0] + start.size[0] - 1,
		maxY: start.origin[1] + start.size[1] - 1,
		maxZ: start.origin[2] + start.size[2] - 1,
	}.intersectsChunk(chunkX, chunkZ, minY, maxY)
}

func (g Generator) placePlannedStructure(c *chunk.Chunk, biomes sourceBiomeVolume, chunkX, chunkZ, minY, maxY int, start plannedStructureStart, structureRNG *gen.Xoroshiro128) {
	for _, piece := range start.pieces {
		if !piece.bounds.intersectsChunk(chunkX, chunkZ, minY, maxY) {
			continue
		}
		for _, blockInfo := range piece.manualBlocks {
			g.placeStructureBlockState(c, chunkX, chunkZ, minY, maxY, blockInfo.worldPos, blockInfo.state)
		}
		for _, placement := range piece.element.placements {
			template, err := g.structureTemplates.Template(placement.templateName)
			if err != nil {
				continue
			}
			for _, blockInfo := range g.processStructureTemplatePlacement(c, chunkX, chunkZ, piece.origin, piece.rotation, piece.mirror, piece.pivot, piece.useTemplateTransform, template, placement) {
				switch blockInfo.state.Name {
				case "structure_void", "jigsaw", "structure_block":
					continue
				}
				if blockInfo.state.Name == "air" && placement.ignoreAir && blockInfo.originalState.Name == "air" {
					continue
				}
				worldX := blockInfo.worldPos[0]
				worldY := blockInfo.worldPos[1]
				worldZ := blockInfo.worldPos[2]
				if worldX < chunkX*16 || worldX >= chunkX*16+16 || worldZ < chunkZ*16 || worldZ >= chunkZ*16+16 || worldY < minY || worldY > maxY {
					continue
				}

				placedState := applyPlacedStructureStateTransform(blockInfo.state, piece.mirror, piece.rotation)
				g.placeStructureBlockState(c, chunkX, chunkZ, minY, maxY, blockInfo.worldPos, placedState)
			}
		}

		for _, feature := range piece.element.features {
			if piece.origin[0] < chunkX*16 || piece.origin[0] >= chunkX*16+16 || piece.origin[2] < chunkZ*16 || piece.origin[2] >= chunkZ*16+16 {
				continue
			}
			if piece.origin[1] <= minY || piece.origin[1] > maxY {
				continue
			}
			featureRNG := structureRNG
			if featureRNG == nil {
				rng := g.structureFeatureRNG(start.structureName, feature.featureName, piece.origin)
				featureRNG = &rng
			}
			_ = g.executePlacedFeatureRef(
				c,
				biomes,
				piece.origin,
				gen.PlacedFeatureRef{Name: feature.featureName},
				feature.featureName,
				chunkX,
				chunkZ,
				minY,
				maxY,
				featureRNG,
				0,
			)
		}
	}
}

func (g Generator) placeStructureBlockState(c *chunk.Chunk, chunkX, chunkZ, minY, maxY int, worldPos cube.Pos, state gen.BlockState) {
	worldX := worldPos[0]
	worldY := worldPos[1]
	worldZ := worldPos[2]
	if !g.positionInFeatureScope(worldPos, chunkX, chunkZ, minY, maxY) {
		return
	}
	rid, ok := g.lookupTemplateBlock(structureLookupName(state.Name), structureLookupProperties(state.Name, state.Properties))
	if !ok {
		return
	}
	c = g.chunkForActiveTreePos(c, worldPos)
	c.SetBlock(uint8(worldX&15), int16(worldY), uint8(worldZ&15), 0, rid)
}

func (g Generator) lookupTemplateBlock(name string, properties map[string]any) (uint32, bool) {
	key := templateBlockCacheKey(name, properties)
	if rid, ok := g.templateBlockCache.Lookup(key); ok {
		return rid, true
	}

	blockProps := make(map[string]any, len(properties))
	for key, value := range properties {
		switch v := value.(type) {
		case bool, int32, string:
			blockProps[key] = v
		case float64:
			blockProps[key] = int32(v)
		default:
			blockProps[key] = fmt.Sprint(v)
		}
	}
	if len(blockProps) == 0 {
		blockProps = nil
	}

	rid, ok := g.blockRegistry.StateToRuntimeID(name, blockProps)
	if !ok {
		return 0, false
	}
	g.templateBlockCache.Store(key, rid)
	return rid, true
}

func templateBlockCacheKey(name string, properties map[string]any) string {
	if len(properties) == 0 {
		return name
	}

	keys := make([]string, 0, len(properties))
	for key := range properties {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	h := fnv.New64a()
	_, _ = h.Write([]byte(name))
	for _, key := range keys {
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(key))
		_, _ = h.Write([]byte{'='})
		_, _ = h.Write([]byte(fmt.Sprint(properties[key])))
	}
	return fmt.Sprintf("%s#%x", name, h.Sum64())
}

func (g Generator) structureRNG(name string, pos world.ChunkPos) gen.Xoroshiro128 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(name))
	seed := int64(h.Sum64()) ^ g.seed ^ int64(pos[0])*341873128712 ^ int64(pos[1])*132897987541
	return gen.NewXoroshiro128FromSeed(seed)
}

func (g Generator) structureFeatureRNG(structureName, featureName string, pos cube.Pos) gen.Xoroshiro128 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(structureName))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(featureName))
	seed := int64(h.Sum64()) ^ g.seed ^ int64(pos[0])*341873128712 ^ int64(pos[1])*132897987541 ^ int64(pos[2])*42317861
	return gen.NewXoroshiro128FromSeed(seed)
}

func randomSpreadPotentialChunk(seed int64, placement gen.RandomSpreadPlacement, gridX, gridZ int) world.ChunkPos {
	rng := newLegacyRandom(int64(gridX)*341873128712 + int64(gridZ)*132897987541 + seed + int64(placement.Salt))
	limit := placement.Spacing - placement.Separation
	if limit <= 0 {
		return world.ChunkPos{int32(gridX * placement.Spacing), int32(gridZ * placement.Spacing)}
	}

	spreadX := rng.NextInt(limit)
	spreadZ := rng.NextInt(limit)
	if placement.SpreadType == "triangular" {
		spreadX = (spreadX + rng.NextInt(limit)) / 2
		spreadZ = (spreadZ + rng.NextInt(limit)) / 2
	}
	return world.ChunkPos{
		int32(gridX*placement.Spacing + spreadX),
		int32(gridZ*placement.Spacing + spreadZ),
	}
}

type legacyRandom struct {
	seed int64
}

func newLegacyRandom(seed int64) legacyRandom {
	return legacyRandom{seed: (seed ^ 25214903917) & 281474976710655}
}

func (r *legacyRandom) next(bits int) int {
	r.seed = (r.seed*25214903917 + 11) & 281474976710655
	return int(uint64(r.seed) >> (48 - bits))
}

func (r *legacyRandom) NextInt(bound int) int {
	if bound <= 1 {
		return 0
	}
	if bound&(bound-1) == 0 {
		return int((int64(bound) * int64(r.next(31))) >> 31)
	}
	for {
		bits := r.next(31)
		value := bits % bound
		if bits-value+(bound-1) >= 0 {
			return value
		}
	}
}

func (r *legacyRandom) NextFloat64() float64 {
	return float64(r.next(24)) / (1 << 24)
}

func (r *legacyRandom) NextDouble() float64 {
	return (float64(uint64(r.next(26))<<27) + float64(r.next(27))) / (1 << 53)
}

func floorDiv(value, divisor int) int {
	quotient := value / divisor
	remainder := value % divisor
	if remainder != 0 && ((remainder < 0) != (divisor < 0)) {
		quotient--
	}
	return quotient
}

func normalizeStructureName(name string) string {
	if len(name) >= 10 && name[:10] == "minecraft:" {
		return name[10:]
	}
	return name
}

func FindPlannedStructureStart(seed int64, setName string, maxGridDistance int) (PlannedStructureInfo, bool) {
	return FindPlannedStructureStartForDimension(seed, world.Overworld, setName, maxGridDistance)
}

func (g Generator) LocateNearestPlannedStructureStart(setName string, origin cube.Pos, maxChunkDistance int) (PlannedStructureInfo, bool) {
	return g.locateNearestPlannedStructureStart(setName, origin, maxChunkDistance)
}

func LocateNearestPlannedStructureStart(seed int64, setName string, origin cube.Pos, maxChunkDistance int) (PlannedStructureInfo, bool) {
	return LocateNearestPlannedStructureStartForDimension(seed, world.Overworld, setName, origin, maxChunkDistance)
}

func LocateNearestPlannedStructureStartForDimension(seed int64, dim world.Dimension, setName string, origin cube.Pos, maxChunkDistance int) (PlannedStructureInfo, bool) {
	g := NewForDimension(seed, dim)
	return g.locateNearestPlannedStructureStart(setName, origin, maxChunkDistance)
}

func (g Generator) locateNearestPlannedStructureStart(setName string, origin cube.Pos, maxChunkDistance int) (PlannedStructureInfo, bool) {
	planner, ok := g.findStructurePlanner(setName)
	if !ok {
		return PlannedStructureInfo{}, false
	}

	originChunkX := floorDiv(origin.X(), 16)
	originChunkZ := floorDiv(origin.Z(), 16)
	startChunks := g.plannerPotentialStartChunksNearChunk(planner, originChunkX, originChunkZ, maxChunkDistance, maxChunkDistance)
	if len(startChunks) == 0 {
		return PlannedStructureInfo{}, false
	}
	if planner.placementType == "concentric_rings" {
		sort.Slice(startChunks, func(i, j int) bool {
			dxI := int(startChunks[i][0]) - originChunkX
			dzI := int(startChunks[i][1]) - originChunkZ
			dxJ := int(startChunks[j][0]) - originChunkX
			dzJ := int(startChunks[j][1]) - originChunkZ
			return dxI*dxI+dzI*dzI < dxJ*dxJ+dzJ*dzJ
		})
		for _, startChunk := range startChunks {
			start, exists := g.planStructureStart(planner, startChunk, -64, 319, nil)
			if !exists {
				continue
			}
			return plannedStructureInfoForStart(g, setName, start), true
		}
		return PlannedStructureInfo{}, false
	}

	var (
		bestStart plannedStructureStart
		bestDist  = math.MaxFloat64
		found     bool
	)
	for _, startChunk := range startChunks {
		start, exists := g.planStructureStart(planner, startChunk, -64, 319, nil)
		if !exists {
			continue
		}
		infoOrigin, infoSize := structureInfoBounds(start)
		centerX := float64(infoOrigin.X()) + float64(max(infoSize[0], 1))/2
		centerZ := float64(infoOrigin.Z()) + float64(max(infoSize[2], 1))/2
		deltaX := centerX - float64(origin.X())
		deltaZ := centerZ - float64(origin.Z())
		dist := deltaX*deltaX + deltaZ*deltaZ
		if !found || dist < bestDist {
			bestDist = dist
			bestStart = start
			found = true
		}
	}
	if !found {
		return PlannedStructureInfo{}, false
	}
	return plannedStructureInfoForStart(g, setName, bestStart), true
}

func FindPlannedStructureStartForDimension(seed int64, dim world.Dimension, setName string, maxGridDistance int) (PlannedStructureInfo, bool) {
	g := NewForDimension(seed, dim)

	planner, ok := g.findStructurePlanner(setName)
	if !ok {
		return PlannedStructureInfo{}, false
	}

	for _, startChunk := range g.plannerPotentialStartChunksWithinGridDistance(planner, maxGridDistance) {
		start, exists := g.planStructureStart(planner, startChunk, -64, 319, nil)
		if !exists {
			continue
		}
		return plannedStructureInfoForStart(g, setName, start), true
	}
	return PlannedStructureInfo{}, false
}

func plannedStructureInfoForStart(g Generator, setName string, start plannedStructureStart) PlannedStructureInfo {
	paletteNames := make([]string, 0, 16)
	seen := make(map[string]struct{}, 32)
	for _, piece := range start.pieces {
		for _, blockInfo := range piece.manualBlocks {
			stateName, ok := structurePaletteStateName(blockInfo.state.Name)
			if !ok {
				continue
			}
			if _, ok := seen[stateName]; ok {
				continue
			}
			seen[stateName] = struct{}{}
			paletteNames = append(paletteNames, stateName)
		}
		for _, placement := range piece.element.placements {
			template, err := g.structureTemplates.Template(placement.templateName)
			if err != nil {
				continue
			}
			for _, state := range template.Palette {
				stateName, ok := structurePaletteStateName(state.Name)
				if !ok {
					continue
				}
				if _, ok := seen[stateName]; ok {
					continue
				}
				seen[stateName] = struct{}{}
				paletteNames = append(paletteNames, stateName)
			}
		}
	}
	sort.Strings(paletteNames)
	infoOrigin, infoSize := structureInfoBounds(start)
	return PlannedStructureInfo{
		StructureSet: setName,
		Structure:    start.structureName,
		Template:     start.templateName,
		StartChunk:   start.startChunk,
		Origin:       infoOrigin,
		Size:         infoSize,
		PaletteNames: paletteNames,
	}
}

func structurePaletteStateName(name string) (string, bool) {
	if name == "" {
		return "", false
	}
	if name[0] != '#' && name[:min(len(name), len("minecraft:"))] != "minecraft:" {
		name = "minecraft:" + name
	}
	switch name {
	case "minecraft:air", "minecraft:jigsaw", "minecraft:structure_void", "minecraft:structure_block":
		return "", false
	}
	return name, true
}

func (g Generator) plannerPotentialStartChunksWithinGridDistance(planner structurePlanner, maxGridDistance int) []world.ChunkPos {
	if maxGridDistance < 0 {
		return nil
	}
	if planner.placementType == "concentric_rings" {
		positions := g.ringPositionsForPlanner(planner)
		if len(positions) == 0 {
			return nil
		}
		out := make([]world.ChunkPos, 0, len(positions))
		limitX := maxGridDistance + planner.maxBackreachX
		limitZ := maxGridDistance + planner.maxBackreachZ
		for _, startChunk := range positions {
			if abs(int(startChunk[0])) > limitX || abs(int(startChunk[1])) > limitZ {
				continue
			}
			out = append(out, startChunk)
		}
		return out
	}

	gridSpan := maxGridDistance*2 + 1
	out := make([]world.ChunkPos, 0, max(0, gridSpan*gridSpan))
	for gridX := -maxGridDistance; gridX <= maxGridDistance; gridX++ {
		for gridZ := -maxGridDistance; gridZ <= maxGridDistance; gridZ++ {
			out = append(out, randomSpreadPotentialChunk(g.seed, planner.randomPlacement, gridX, gridZ))
		}
	}
	return out
}

func structureInfoBounds(start plannedStructureStart) (cube.Pos, [3]int) {
	infoOrigin := start.rootOrigin
	infoSize := start.rootSize
	if infoSize[0] <= 0 || infoSize[1] <= 0 || infoSize[2] <= 0 {
		infoOrigin = start.origin
		infoSize = start.size
	}
	return infoOrigin, infoSize
}
