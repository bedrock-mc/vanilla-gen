package vanilla

// Exact port of vanilla 1.21.11 MineshaftStructure + MineshaftPieces.
//
// Plan phase (planMineshaftSetStart): ChunkGenerator.createStructures picks a
// structure-set entry with a WorldgenRandom(LegacyRandomSource) seeded via
// setLargeFeatureSeed(seed, chunkX, chunkZ); Structure.generate then builds a
// GenerationContext whose random is seeded identically, and
// MineshaftStructure.findGenerationPoint discards one nextDouble (the legacy
// frequency draw) before MineShaftRoom + addChildren recursion and the
// StructurePiecesBuilder.moveBelowSeaLevel height shuffle. Biome validity is
// tested last, at the generation stub position.
//
// Place phase (placeMineshaftStart): StructureStart.placeInChunk runs each
// intersecting piece's postProcess in list order against the chunk being
// generated, sharing the per-chunk WorldgenRandom that
// ChunkGenerator.applyBiomeDecoration seeded with setFeatureSeed.

import (
	gen "github.com/bedrock-mc/vanilla-gen/gen"
	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/df-mc/dragonfly/server/world/chunk"
)

type mineshaftPieceKind uint8

const (
	mineshaftPieceRoom mineshaftPieceKind = iota
	mineshaftPieceCorridor
	mineshaftPieceCrossing
	mineshaftPieceStairs
)

type mineshaftDir int8

const (
	mineshaftDirNone  mineshaftDir = -1
	mineshaftDirNorth mineshaftDir = iota - 1 // z-
	mineshaftDirSouth                         // z+
	mineshaftDirWest                          // x-
	mineshaftDirEast                          // x+
)

func (d mineshaftDir) axisZ() bool { return d == mineshaftDirNorth || d == mineshaftDirSouth }

type mineshaftPiece struct {
	kind           mineshaftPieceKind
	bb             structureBox
	orientation    mineshaftDir // -1 for room and crossing pieces
	genDepth       int
	hasRails       bool
	spiderCorridor bool
	numSections    int
	isTwoFloored   bool
	direction      mineshaftDir   // crossing only
	entrances      []structureBox // room only
}

type plannedMineshaft struct {
	mesa   bool
	pieces []*mineshaftPiece
}

// plannerIsMineshaftSet reports whether every candidate of the structure set
// is a mineshaft (the vanilla "mineshafts" set).
func plannerIsMineshaftSet(planner structurePlanner) bool {
	if len(planner.candidates) == 0 {
		return false
	}
	for _, candidate := range planner.candidates {
		if candidate.structureType != "mineshaft" {
			return false
		}
	}
	return true
}

// planMineshaftSetStart ports the multi-entry selection loop from
// ChunkGenerator.createStructures plus MineshaftStructure generation.
func (g Generator) planMineshaftSetStart(planner structurePlanner, startChunk world.ChunkPos, minY, maxY int, surfaceSampler *structureHeightSampler) (plannedStructureStart, bool) {
	type option struct {
		candidate structurePlannerCandidate
		weight    int
	}
	options := make([]option, 0, len(planner.candidates))
	total := 0
	for _, candidate := range planner.candidates {
		options = append(options, option{candidate: candidate, weight: candidate.weight})
		total += candidate.weight
	}

	if len(options) == 1 {
		return g.generateMineshaftStart(planner, options[0].candidate, startChunk, minY, maxY, surfaceSampler)
	}

	rng := gen.NewWorldgenRandomLegacy(0)
	rng.SetLargeFeatureSeed(g.seed, int(startChunk[0]), int(startChunk[1]))
	for len(options) > 0 && total > 0 {
		choice := int(rng.NextInt(uint32(total)))
		index := 0
		for _, opt := range options {
			choice -= opt.weight
			if choice < 0 {
				break
			}
			index++
		}
		selected := options[index]
		if start, ok := g.generateMineshaftStart(planner, selected.candidate, startChunk, minY, maxY, surfaceSampler); ok {
			return start, true
		}
		options = append(options[:index], options[index+1:]...)
		total -= selected.weight
	}
	return plannedStructureStart{}, false
}

// generateMineshaftStart ports Structure.generate ->
// MineshaftStructure.findGenerationPoint + isValidBiome for one candidate.
func (g Generator) generateMineshaftStart(planner structurePlanner, candidate structurePlannerCandidate, startChunk world.ChunkPos, minY, maxY int, surfaceSampler *structureHeightSampler) (plannedStructureStart, bool) {
	chunkX := int(startChunk[0])
	chunkZ := int(startChunk[1])
	mesa := candidate.generic.MineshaftType == "mesa"

	rng := gen.NewWorldgenRandomLegacy(0)
	rng.SetLargeFeatureSeed(g.seed, chunkX, chunkZ)
	// MineshaftStructure.findGenerationPoint discards the frequency draw.
	rng.NextDouble()

	pieces := generateMineshaftPieces(rng, chunkX, chunkZ)
	total := emptyStructureBox()
	for _, piece := range pieces {
		total = unionStructureBoxes(total, piece.bb)
	}

	var dy int
	if mesa {
		// getCenter + getBaseHeight(WORLD_SURFACE_WG) + randomBetweenInclusive.
		centerX := total.minX + (total.maxX-total.minX+1)/2
		centerY := total.minY + (total.maxY-total.minY+1)/2
		centerZ := total.minZ + (total.maxZ-total.minZ+1)/2
		surfaceHeight := surfaceSampler.worldSurfaceLevelAt(centerX, centerZ)
		targetY := seaLevel
		if surfaceHeight > seaLevel {
			targetY = randomBetweenInclusive(rng, seaLevel, surfaceHeight)
		}
		dy = targetY - centerY
	} else {
		// StructurePiecesBuilder.moveBelowSeaLevel(seaLevel, minY, random, 10).
		maxAllowedY := seaLevel - 10
		y1Pos := (total.maxY - total.minY + 1) + minY + 1
		if y1Pos < maxAllowedY {
			y1Pos += int(rng.NextInt(uint32(maxAllowedY - y1Pos)))
		}
		dy = y1Pos - total.maxY
	}

	for _, piece := range pieces {
		piece.bb = shiftStructureBox(piece.bb, cube.Pos{0, dy, 0})
		for i := range piece.entrances {
			piece.entrances[i] = shiftStructureBox(piece.entrances[i], cube.Pos{0, dy, 0})
		}
	}
	total = shiftStructureBox(total, cube.Pos{0, dy, 0})

	// Structure.isValidBiome at the generation stub position
	// (middle block X, 50 + yOffset, min block Z), raw noise biome.
	stubBiome := g.biomeSource.GetBiome(chunkX*16+8, 50+dy, chunkZ*16)
	if !g.structureCandidateAllowed(candidate, stubBiome) {
		return plannedStructureStart{}, false
	}

	origin, size := total.originAndSize()
	rootOrigin, rootSize := pieces[0].bb.originAndSize()
	return plannedStructureStart{
		setName:           planner.setName,
		structureName:     candidate.structureName,
		templateName:      candidate.structureName,
		terrainAdaptation: candidate.terrainAdaptation,
		startChunk:        startChunk,
		origin:            origin,
		size:              size,
		rootOrigin:        rootOrigin,
		rootSize:          rootSize,
		mineshaft: &plannedMineshaft{
			mesa:   mesa,
			pieces: pieces,
		},
	}, true
}

// generateMineshaftPieces ports MineShaftRoom construction plus the
// addChildren recursion (MineshaftStructure.generatePiecesAndAdjust before the
// vertical shift).
func generateMineshaftPieces(rng *gen.WorldgenRandom, chunkX, chunkZ int) []*mineshaftPiece {
	west := chunkX*16 + 2
	north := chunkZ*16 + 2
	room := &mineshaftPiece{
		kind:        mineshaftPieceRoom,
		orientation: mineshaftDirNone,
		genDepth:    0,
	}
	// new BoundingBox(west, 50, north, west+7+nextInt(6), 54+nextInt(6), north+7+nextInt(6))
	maxX := west + 7 + int(rng.NextInt(6))
	maxY := 54 + int(rng.NextInt(6))
	maxZ := north + 7 + int(rng.NextInt(6))
	room.bb = structureBox{minX: west, minY: 50, minZ: north, maxX: maxX, maxY: maxY, maxZ: maxZ}

	pieces := []*mineshaftPiece{room}
	mineshaftAddChildren(&pieces, room, room, rng)
	return pieces
}

func mineshaftAddChildren(pieces *[]*mineshaftPiece, start, piece *mineshaftPiece, rng *gen.WorldgenRandom) {
	switch piece.kind {
	case mineshaftPieceRoom:
		mineshaftRoomAddChildren(pieces, start, piece, rng)
	case mineshaftPieceCorridor:
		mineshaftCorridorAddChildren(pieces, start, piece, rng)
	case mineshaftPieceCrossing:
		mineshaftCrossingAddChildren(pieces, start, piece, rng)
	case mineshaftPieceStairs:
		mineshaftStairsAddChildren(pieces, start, piece, rng)
	}
}

// mineshaftGenerateAndAddPiece ports MineshaftPieces.generateAndAddPiece.
func mineshaftGenerateAndAddPiece(pieces *[]*mineshaftPiece, start *mineshaftPiece, rng *gen.WorldgenRandom, footX, footY, footZ int, direction mineshaftDir, depth int) *mineshaftPiece {
	if depth > 8 {
		return nil
	}
	if abs(footX-start.bb.minX) > 80 || abs(footZ-start.bb.minZ) > 80 {
		return nil
	}
	piece := mineshaftCreateRandomShaftPiece(*pieces, rng, footX, footY, footZ, direction, depth+1)
	if piece != nil {
		*pieces = append(*pieces, piece)
		mineshaftAddChildren(pieces, start, piece, rng)
	}
	return piece
}

// mineshaftCreateRandomShaftPiece ports MineshaftPieces.createRandomShaftPiece.
func mineshaftCreateRandomShaftPiece(pieces []*mineshaftPiece, rng *gen.WorldgenRandom, footX, footY, footZ int, direction mineshaftDir, genDepth int) *mineshaftPiece {
	selection := int(rng.NextInt(100))
	if selection >= 80 {
		if bb, ok := mineshaftFindCrossing(pieces, rng, footX, footY, footZ, direction); ok {
			return &mineshaftPiece{
				kind:         mineshaftPieceCrossing,
				bb:           bb,
				orientation:  mineshaftDirNone,
				genDepth:     genDepth,
				direction:    direction,
				isTwoFloored: bb.maxY-bb.minY+1 > 3,
			}
		}
	} else if selection >= 70 {
		if bb, ok := mineshaftFindStairs(pieces, footX, footY, footZ, direction); ok {
			return &mineshaftPiece{
				kind:        mineshaftPieceStairs,
				bb:          bb,
				orientation: direction,
				genDepth:    genDepth,
			}
		}
	} else {
		if bb, ok := mineshaftFindCorridorSize(pieces, rng, footX, footY, footZ, direction); ok {
			piece := &mineshaftPiece{
				kind:        mineshaftPieceCorridor,
				bb:          bb,
				orientation: direction,
				genDepth:    genDepth,
			}
			piece.hasRails = rng.NextInt(3) == 0
			piece.spiderCorridor = !piece.hasRails && rng.NextInt(23) == 0
			if direction.axisZ() {
				piece.numSections = (bb.maxZ - bb.minZ + 1) / 5
			} else {
				piece.numSections = (bb.maxX - bb.minX + 1) / 5
			}
			return piece
		}
	}
	return nil
}

func mineshaftFindCollision(pieces []*mineshaftPiece, box structureBox) bool {
	for _, piece := range pieces {
		if piece.bb.intersects(box) {
			return true
		}
	}
	return false
}

func mineshaftBoxForDirection(direction mineshaftDir, north, south, west, east structureBox, footX, footY, footZ int) structureBox {
	var box structureBox
	switch direction {
	case mineshaftDirSouth:
		box = south
	case mineshaftDirWest:
		box = west
	case mineshaftDirEast:
		box = east
	default:
		box = north
	}
	return shiftStructureBox(box, cube.Pos{footX, footY, footZ})
}

// mineshaftFindCorridorSize ports MineShaftCorridor.findCorridorSize.
func mineshaftFindCorridorSize(pieces []*mineshaftPiece, rng *gen.WorldgenRandom, footX, footY, footZ int, direction mineshaftDir) (structureBox, bool) {
	for corridorLength := int(rng.NextInt(3)) + 2; corridorLength > 0; corridorLength-- {
		blockLength := corridorLength * 5
		box := mineshaftBoxForDirection(direction,
			structureBox{minX: 0, minY: 0, minZ: -(blockLength - 1), maxX: 2, maxY: 2, maxZ: 0},
			structureBox{minX: 0, minY: 0, minZ: 0, maxX: 2, maxY: 2, maxZ: blockLength - 1},
			structureBox{minX: -(blockLength - 1), minY: 0, minZ: 0, maxX: 0, maxY: 2, maxZ: 2},
			structureBox{minX: 0, minY: 0, minZ: 0, maxX: blockLength - 1, maxY: 2, maxZ: 2},
			footX, footY, footZ)
		if !mineshaftFindCollision(pieces, box) {
			return box, true
		}
	}
	return structureBox{}, false
}

// mineshaftFindCrossing ports MineShaftCrossing.findCrossing.
func mineshaftFindCrossing(pieces []*mineshaftPiece, rng *gen.WorldgenRandom, footX, footY, footZ int, direction mineshaftDir) (structureBox, bool) {
	y1 := 2
	if rng.NextInt(4) == 0 {
		y1 = 6
	}
	box := mineshaftBoxForDirection(direction,
		structureBox{minX: -1, minY: 0, minZ: -4, maxX: 3, maxY: y1, maxZ: 0},
		structureBox{minX: -1, minY: 0, minZ: 0, maxX: 3, maxY: y1, maxZ: 4},
		structureBox{minX: -4, minY: 0, minZ: -1, maxX: 0, maxY: y1, maxZ: 3},
		structureBox{minX: 0, minY: 0, minZ: -1, maxX: 4, maxY: y1, maxZ: 3},
		footX, footY, footZ)
	if mineshaftFindCollision(pieces, box) {
		return structureBox{}, false
	}
	return box, true
}

// mineshaftFindStairs ports MineShaftStairs.findStairs.
func mineshaftFindStairs(pieces []*mineshaftPiece, footX, footY, footZ int, direction mineshaftDir) (structureBox, bool) {
	box := mineshaftBoxForDirection(direction,
		structureBox{minX: 0, minY: -5, minZ: -8, maxX: 2, maxY: 2, maxZ: 0},
		structureBox{minX: 0, minY: -5, minZ: 0, maxX: 2, maxY: 2, maxZ: 8},
		structureBox{minX: -8, minY: -5, minZ: 0, maxX: 0, maxY: 2, maxZ: 2},
		structureBox{minX: 0, minY: -5, minZ: 0, maxX: 8, maxY: 2, maxZ: 2},
		footX, footY, footZ)
	if mineshaftFindCollision(pieces, box) {
		return structureBox{}, false
	}
	return box, true
}

// mineshaftRoomAddChildren ports MineShaftRoom.addChildren.
func mineshaftRoomAddChildren(pieces *[]*mineshaftPiece, start, room *mineshaftPiece, rng *gen.WorldgenRandom) {
	depth := room.genDepth
	bb := room.bb
	xSpan := bb.maxX - bb.minX + 1
	zSpan := bb.maxZ - bb.minZ + 1
	heightSpace := (bb.maxY - bb.minY + 1) - 3 - 1
	if heightSpace <= 0 {
		heightSpace = 1
	}

	pos := 0
	for pos < xSpan {
		pos += int(rng.NextInt(uint32(xSpan)))
		if pos+3 > xSpan {
			break
		}
		child := mineshaftGenerateAndAddPiece(pieces, start, rng,
			bb.minX+pos, bb.minY+int(rng.NextInt(uint32(heightSpace)))+1, bb.minZ-1, mineshaftDirNorth, depth)
		if child != nil {
			room.entrances = append(room.entrances, structureBox{
				minX: child.bb.minX, minY: child.bb.minY, minZ: bb.minZ,
				maxX: child.bb.maxX, maxY: child.bb.maxY, maxZ: bb.minZ + 1,
			})
		}
		pos += 4
	}

	pos = 0
	for pos < xSpan {
		pos += int(rng.NextInt(uint32(xSpan)))
		if pos+3 > xSpan {
			break
		}
		child := mineshaftGenerateAndAddPiece(pieces, start, rng,
			bb.minX+pos, bb.minY+int(rng.NextInt(uint32(heightSpace)))+1, bb.maxZ+1, mineshaftDirSouth, depth)
		if child != nil {
			room.entrances = append(room.entrances, structureBox{
				minX: child.bb.minX, minY: child.bb.minY, minZ: bb.maxZ - 1,
				maxX: child.bb.maxX, maxY: child.bb.maxY, maxZ: bb.maxZ,
			})
		}
		pos += 4
	}

	pos = 0
	for pos < zSpan {
		pos += int(rng.NextInt(uint32(zSpan)))
		if pos+3 > zSpan {
			break
		}
		child := mineshaftGenerateAndAddPiece(pieces, start, rng,
			bb.minX-1, bb.minY+int(rng.NextInt(uint32(heightSpace)))+1, bb.minZ+pos, mineshaftDirWest, depth)
		if child != nil {
			room.entrances = append(room.entrances, structureBox{
				minX: bb.minX, minY: child.bb.minY, minZ: child.bb.minZ,
				maxX: bb.minX + 1, maxY: child.bb.maxY, maxZ: child.bb.maxZ,
			})
		}
		pos += 4
	}

	pos = 0
	for pos < zSpan {
		pos += int(rng.NextInt(uint32(zSpan)))
		if pos+3 > zSpan {
			break
		}
		child := mineshaftGenerateAndAddPiece(pieces, start, rng,
			bb.maxX+1, bb.minY+int(rng.NextInt(uint32(heightSpace)))+1, bb.minZ+pos, mineshaftDirEast, depth)
		if child != nil {
			room.entrances = append(room.entrances, structureBox{
				minX: bb.maxX - 1, minY: child.bb.minY, minZ: child.bb.minZ,
				maxX: bb.maxX, maxY: child.bb.maxY, maxZ: child.bb.maxZ,
			})
		}
		pos += 4
	}
}

// mineshaftCorridorAddChildren ports MineShaftCorridor.addChildren.
func mineshaftCorridorAddChildren(pieces *[]*mineshaftPiece, start, corridor *mineshaftPiece, rng *gen.WorldgenRandom) {
	depth := corridor.genDepth
	endSelection := int(rng.NextInt(4))
	bb := corridor.bb
	orientation := corridor.orientation
	if orientation != mineshaftDirNone {
		switch orientation {
		case mineshaftDirSouth:
			if endSelection <= 1 {
				mineshaftGenerateAndAddPiece(pieces, start, rng, bb.minX, bb.minY-1+int(rng.NextInt(3)), bb.maxZ+1, orientation, depth)
			} else if endSelection == 2 {
				mineshaftGenerateAndAddPiece(pieces, start, rng, bb.minX-1, bb.minY-1+int(rng.NextInt(3)), bb.maxZ-3, mineshaftDirWest, depth)
			} else {
				mineshaftGenerateAndAddPiece(pieces, start, rng, bb.maxX+1, bb.minY-1+int(rng.NextInt(3)), bb.maxZ-3, mineshaftDirEast, depth)
			}
		case mineshaftDirWest:
			if endSelection <= 1 {
				mineshaftGenerateAndAddPiece(pieces, start, rng, bb.minX-1, bb.minY-1+int(rng.NextInt(3)), bb.minZ, orientation, depth)
			} else if endSelection == 2 {
				mineshaftGenerateAndAddPiece(pieces, start, rng, bb.minX, bb.minY-1+int(rng.NextInt(3)), bb.minZ-1, mineshaftDirNorth, depth)
			} else {
				mineshaftGenerateAndAddPiece(pieces, start, rng, bb.minX, bb.minY-1+int(rng.NextInt(3)), bb.maxZ+1, mineshaftDirSouth, depth)
			}
		case mineshaftDirEast:
			if endSelection <= 1 {
				mineshaftGenerateAndAddPiece(pieces, start, rng, bb.maxX+1, bb.minY-1+int(rng.NextInt(3)), bb.minZ, orientation, depth)
			} else if endSelection == 2 {
				mineshaftGenerateAndAddPiece(pieces, start, rng, bb.maxX-3, bb.minY-1+int(rng.NextInt(3)), bb.minZ-1, mineshaftDirNorth, depth)
			} else {
				mineshaftGenerateAndAddPiece(pieces, start, rng, bb.maxX-3, bb.minY-1+int(rng.NextInt(3)), bb.maxZ+1, mineshaftDirSouth, depth)
			}
		default: // NORTH
			if endSelection <= 1 {
				mineshaftGenerateAndAddPiece(pieces, start, rng, bb.minX, bb.minY-1+int(rng.NextInt(3)), bb.minZ-1, orientation, depth)
			} else if endSelection == 2 {
				mineshaftGenerateAndAddPiece(pieces, start, rng, bb.minX-1, bb.minY-1+int(rng.NextInt(3)), bb.minZ, mineshaftDirWest, depth)
			} else {
				mineshaftGenerateAndAddPiece(pieces, start, rng, bb.maxX+1, bb.minY-1+int(rng.NextInt(3)), bb.minZ, mineshaftDirEast, depth)
			}
		}
	}

	if depth < 8 {
		if orientation != mineshaftDirNorth && orientation != mineshaftDirSouth {
			for x := bb.minX + 3; x+3 <= bb.maxX; x += 5 {
				selection := int(rng.NextInt(5))
				if selection == 0 {
					mineshaftGenerateAndAddPiece(pieces, start, rng, x, bb.minY, bb.minZ-1, mineshaftDirNorth, depth+1)
				} else if selection == 1 {
					mineshaftGenerateAndAddPiece(pieces, start, rng, x, bb.minY, bb.maxZ+1, mineshaftDirSouth, depth+1)
				}
			}
		} else {
			for z := bb.minZ + 3; z+3 <= bb.maxZ; z += 5 {
				selection := int(rng.NextInt(5))
				if selection == 0 {
					mineshaftGenerateAndAddPiece(pieces, start, rng, bb.minX-1, bb.minY, z, mineshaftDirWest, depth+1)
				} else if selection == 1 {
					mineshaftGenerateAndAddPiece(pieces, start, rng, bb.maxX+1, bb.minY, z, mineshaftDirEast, depth+1)
				}
			}
		}
	}
}

// mineshaftCrossingAddChildren ports MineShaftCrossing.addChildren.
func mineshaftCrossingAddChildren(pieces *[]*mineshaftPiece, start, crossing *mineshaftPiece, rng *gen.WorldgenRandom) {
	depth := crossing.genDepth
	bb := crossing.bb
	switch crossing.direction {
	case mineshaftDirSouth:
		mineshaftGenerateAndAddPiece(pieces, start, rng, bb.minX+1, bb.minY, bb.maxZ+1, mineshaftDirSouth, depth)
		mineshaftGenerateAndAddPiece(pieces, start, rng, bb.minX-1, bb.minY, bb.minZ+1, mineshaftDirWest, depth)
		mineshaftGenerateAndAddPiece(pieces, start, rng, bb.maxX+1, bb.minY, bb.minZ+1, mineshaftDirEast, depth)
	case mineshaftDirWest:
		mineshaftGenerateAndAddPiece(pieces, start, rng, bb.minX+1, bb.minY, bb.minZ-1, mineshaftDirNorth, depth)
		mineshaftGenerateAndAddPiece(pieces, start, rng, bb.minX+1, bb.minY, bb.maxZ+1, mineshaftDirSouth, depth)
		mineshaftGenerateAndAddPiece(pieces, start, rng, bb.minX-1, bb.minY, bb.minZ+1, mineshaftDirWest, depth)
	case mineshaftDirEast:
		mineshaftGenerateAndAddPiece(pieces, start, rng, bb.minX+1, bb.minY, bb.minZ-1, mineshaftDirNorth, depth)
		mineshaftGenerateAndAddPiece(pieces, start, rng, bb.minX+1, bb.minY, bb.maxZ+1, mineshaftDirSouth, depth)
		mineshaftGenerateAndAddPiece(pieces, start, rng, bb.maxX+1, bb.minY, bb.minZ+1, mineshaftDirEast, depth)
	default: // NORTH
		mineshaftGenerateAndAddPiece(pieces, start, rng, bb.minX+1, bb.minY, bb.minZ-1, mineshaftDirNorth, depth)
		mineshaftGenerateAndAddPiece(pieces, start, rng, bb.minX-1, bb.minY, bb.minZ+1, mineshaftDirWest, depth)
		mineshaftGenerateAndAddPiece(pieces, start, rng, bb.maxX+1, bb.minY, bb.minZ+1, mineshaftDirEast, depth)
	}

	if crossing.isTwoFloored {
		if rng.NextBool() {
			mineshaftGenerateAndAddPiece(pieces, start, rng, bb.minX+1, bb.minY+3+1, bb.minZ-1, mineshaftDirNorth, depth)
		}
		if rng.NextBool() {
			mineshaftGenerateAndAddPiece(pieces, start, rng, bb.minX-1, bb.minY+3+1, bb.minZ+1, mineshaftDirWest, depth)
		}
		if rng.NextBool() {
			mineshaftGenerateAndAddPiece(pieces, start, rng, bb.maxX+1, bb.minY+3+1, bb.minZ+1, mineshaftDirEast, depth)
		}
		if rng.NextBool() {
			mineshaftGenerateAndAddPiece(pieces, start, rng, bb.minX+1, bb.minY+3+1, bb.maxZ+1, mineshaftDirSouth, depth)
		}
	}
}

// mineshaftStairsAddChildren ports MineShaftStairs.addChildren.
func mineshaftStairsAddChildren(pieces *[]*mineshaftPiece, start, stairs *mineshaftPiece, rng *gen.WorldgenRandom) {
	depth := stairs.genDepth
	bb := stairs.bb
	switch stairs.orientation {
	case mineshaftDirSouth:
		mineshaftGenerateAndAddPiece(pieces, start, rng, bb.minX, bb.minY, bb.maxZ+1, mineshaftDirSouth, depth)
	case mineshaftDirWest:
		mineshaftGenerateAndAddPiece(pieces, start, rng, bb.minX-1, bb.minY, bb.minZ, mineshaftDirWest, depth)
	case mineshaftDirEast:
		mineshaftGenerateAndAddPiece(pieces, start, rng, bb.maxX+1, bb.minY, bb.minZ, mineshaftDirEast, depth)
	default: // NORTH
		mineshaftGenerateAndAddPiece(pieces, start, rng, bb.minX, bb.minY, bb.minZ-1, mineshaftDirNorth, depth)
	}
}

// ---------------------------------------------------------------------------
// Placement (StructureStart.placeInChunk / MineShaftPiece.postProcess)
// ---------------------------------------------------------------------------

type mineshaftPlacer struct {
	g       Generator
	c       *chunk.Chunk
	rng     *gen.WorldgenRandom
	chunkBB structureBox
	// world height limits: level.getMinY() and level.getMaxY() (inclusive).
	levelMinY int
	levelMaxY int

	planksName string
	logName    string
	fenceName  string

	airRID     uint32
	planksRID  uint32
	fenceRID   uint32
	logRID     uint32
	chainRID   uint32
	webRID     uint32
	spawnerRID uint32
	railNSRID  uint32
	railEWRID  uint32
	// wall torch runtime IDs by world-facing (N, S, W, E)
	torchRIDs [4]uint32
}

func (g Generator) placeMineshaftStart(c *chunk.Chunk, chunkX, chunkZ, minY, maxY int, start plannedStructureStart, rng *gen.WorldgenRandom) {
	ms := start.mineshaft
	if ms == nil || rng == nil {
		return
	}

	woodType := block.OakWood()
	if ms.mesa {
		woodType = block.DarkOakWood()
	}
	w := &mineshaftPlacer{
		g:   g,
		c:   c,
		rng: rng,
		chunkBB: structureBox{
			minX: chunkX * 16, minY: minY + 1, minZ: chunkZ * 16,
			maxX: chunkX*16 + 15, maxY: maxY, maxZ: chunkZ*16 + 15,
		},
		levelMinY: minY,
		levelMaxY: maxY,
	}
	w.planksName = featureBlockName(block.Planks{Wood: woodType})
	w.logName = featureBlockName(block.Log{Wood: woodType, Axis: cube.Y})
	w.fenceName = featureBlockName(block.WoodFence{Wood: woodType})
	w.airRID = g.airRID
	w.planksRID = runtimeIDForBlock(block.Planks{Wood: woodType})
	w.fenceRID = runtimeIDForBlock(block.WoodFence{Wood: woodType})
	w.logRID = runtimeIDForBlock(block.Log{Wood: woodType, Axis: cube.Y})
	w.chainRID = runtimeIDForBlock(block.IronChain{Axis: cube.Y})
	w.webRID = runtimeIDForBlock(block.Cobweb{})
	if rid, ok := g.lookupTemplateBlock("minecraft:mob_spawner", nil); ok {
		w.spawnerRID = rid
	} else {
		w.spawnerRID = w.airRID
	}
	if rid, ok := g.lookupTemplateBlock("minecraft:rail", map[string]any{"rail_direction": int32(0)}); ok {
		w.railNSRID = rid
	} else {
		w.railNSRID = w.airRID
	}
	if rid, ok := g.lookupTemplateBlock("minecraft:rail", map[string]any{"rail_direction": int32(1)}); ok {
		w.railEWRID = rid
	} else {
		w.railEWRID = w.airRID
	}
	// Wall torches: dragonfly's Facing points at the supporting block, the
	// opposite of the Java wall_torch facing.
	w.torchRIDs[mineshaftDirNorth] = runtimeIDForBlock(block.Torch{Facing: cube.FaceSouth, Type: block.NormalFire()})
	w.torchRIDs[mineshaftDirSouth] = runtimeIDForBlock(block.Torch{Facing: cube.FaceNorth, Type: block.NormalFire()})
	w.torchRIDs[mineshaftDirWest] = runtimeIDForBlock(block.Torch{Facing: cube.FaceEast, Type: block.NormalFire()})
	w.torchRIDs[mineshaftDirEast] = runtimeIDForBlock(block.Torch{Facing: cube.FaceWest, Type: block.NormalFire()})

	for _, piece := range ms.pieces {
		if !piece.bb.intersects(w.chunkBB) {
			continue
		}
		switch piece.kind {
		case mineshaftPieceRoom:
			w.roomPostProcess(piece)
		case mineshaftPieceCorridor:
			w.corridorPostProcess(piece)
		case mineshaftPieceCrossing:
			w.crossingPostProcess(piece)
		case mineshaftPieceStairs:
			w.stairsPostProcess(piece)
		}
	}
}

// worldPos ports StructurePiece.getWorldPos for the piece orientation.
func (w *mineshaftPlacer) worldPos(p *mineshaftPiece, x, y, z int) cube.Pos {
	if p.orientation == mineshaftDirNone {
		return cube.Pos{x, y, z}
	}
	var wx, wz int
	switch p.orientation {
	case mineshaftDirNorth:
		wx = p.bb.minX + x
		wz = p.bb.maxZ - z
	case mineshaftDirSouth:
		wx = p.bb.minX + x
		wz = p.bb.minZ + z
	case mineshaftDirWest:
		wx = p.bb.maxX - z
		wz = p.bb.minZ + x
	default: // EAST
		wx = p.bb.minX + z
		wz = p.bb.minZ + x
	}
	return cube.Pos{wx, p.bb.minY + y, wz}
}

func (w *mineshaftPlacer) inside(pos cube.Pos) bool {
	return w.chunkBB.containsPos(pos)
}

func (w *mineshaftPlacer) ridAt(pos cube.Pos) uint32 {
	return w.c.Block(uint8(pos[0]&15), int16(pos[1]), uint8(pos[2]&15), 0)
}

func (w *mineshaftPlacer) nameAt(pos cube.Pos) string {
	return w.g.carverBlockName(w.ridAt(pos))
}

func (w *mineshaftPlacer) setBlock(pos cube.Pos, rid uint32) {
	w.c.SetBlock(uint8(pos[0]&15), int16(pos[1]), uint8(pos[2]&15), 0, rid)
	w.c.SetBlock(uint8(pos[0]&15), int16(pos[1]), uint8(pos[2]&15), 1, w.airRID)
}

// getBlockName ports StructurePiece.getBlock: air outside the chunk box.
func (w *mineshaftPlacer) getBlockName(p *mineshaftPiece, x, y, z int) string {
	pos := w.worldPos(p, x, y, z)
	if !w.inside(pos) {
		return "air"
	}
	return w.nameAt(pos)
}

func mineshaftIsAirName(name string) bool {
	return name == "air"
}

func mineshaftIsLiquidName(name string) bool {
	switch name {
	case "water", "flowing_water", "lava", "flowing_lava":
		return true
	default:
		return false
	}
}

func mineshaftIsLavaName(name string) bool {
	return name == "lava" || name == "flowing_lava"
}

// isReplaceableByStructures ports StructurePiece.isReplaceableByStructures.
func mineshaftIsReplaceableByStructures(name string) bool {
	switch name {
	case "air", "glow_lichen", "seagrass", "tall_seagrass":
		return true
	default:
		return mineshaftIsLiquidName(name)
	}
}

// faceSturdy approximates BlockStateBase.isFaceSturdy for the block set that
// can exist underground while the mineshaft places: full solid blocks are
// sturdy, everything non-solid plus the mineshaft's own partial blocks is not.
func (w *mineshaftPlacer) faceSturdy(name string) bool {
	if !w.g.javaSolidName(name) {
		return false
	}
	switch name {
	case w.fenceName, "iron_chain", "chest", "chain":
		return false
	}
	return true
}

// isSolidRender approximates BlockStateBase.isSolidRender (used only for the
// rail floor check).
func (w *mineshaftPlacer) isSolidRender(name string) bool {
	if !w.faceSturdy(name) {
		return false
	}
	switch name {
	case "mob_spawner", "glass", "ice", "glowstone", "sea_lantern":
		return false
	}
	if len(name) > 7 && name[len(name)-7:] == "_leaves" {
		return false
	}
	return true
}

// canBeReplaced ports MineShaftPiece.canBeReplaced.
func (w *mineshaftPlacer) canBeReplaced(name string) bool {
	switch name {
	case w.planksName, w.logName, w.fenceName, "iron_chain":
		return false
	}
	return true
}

// placeBlock ports StructurePiece.placeBlock (bounding-box guard plus the
// MineShaftPiece canBeReplaced override). The runtime ID is expected to be
// pre-rotated/mirrored for the piece orientation.
func (w *mineshaftPlacer) placeBlock(p *mineshaftPiece, rid uint32, x, y, z int) {
	pos := w.worldPos(p, x, y, z)
	if !w.inside(pos) {
		return
	}
	if !w.canBeReplaced(w.nameAt(pos)) {
		return
	}
	w.setBlock(pos, rid)
}

// isInterior ports StructurePiece.isInterior: below the OCEAN_FLOOR_WG
// heightmap of the live chunk.
func (w *mineshaftPlacer) isInterior(p *mineshaftPiece, x, y, z int) bool {
	pos := w.worldPos(p, x, y+1, z)
	if !w.inside(pos) {
		return false
	}
	height := w.g.heightmapPlacementY(w.c, pos[0]&15, pos[2]&15, "OCEAN_FLOOR_WG", w.levelMinY, w.levelMaxY)
	return pos[1] < height
}

// generateBox ports StructurePiece.generateBox (skipAir=false variants).
func (w *mineshaftPlacer) generateBox(p *mineshaftPiece, x0, y0, z0, x1, y1, z1 int, edgeRID, fillRID uint32) {
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			for z := z0; z <= z1; z++ {
				if y != y0 && y != y1 && x != x0 && x != x1 && z != z0 && z != z1 {
					w.placeBlock(p, fillRID, x, y, z)
				} else {
					w.placeBlock(p, edgeRID, x, y, z)
				}
			}
		}
	}
}

// generateMaybeBox ports StructurePiece.generateMaybeBox with skipAir=false.
func (w *mineshaftPlacer) generateMaybeBox(p *mineshaftPiece, probability float32, x0, y0, z0, x1, y1, z1 int, edgeRID, fillRID uint32, hasToBeInside bool) {
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			for z := z0; z <= z1; z++ {
				if w.rng.NextFloat() > probability {
					continue
				}
				if hasToBeInside && !w.isInterior(p, x, y, z) {
					continue
				}
				if y != y0 && y != y1 && x != x0 && x != x1 && z != z0 && z != z1 {
					w.placeBlock(p, fillRID, x, y, z)
				} else {
					w.placeBlock(p, edgeRID, x, y, z)
				}
			}
		}
	}
}

func (w *mineshaftPlacer) maybeGenerateBlock(p *mineshaftPiece, probability float32, x, y, z int, rid uint32) {
	if w.rng.NextFloat() < probability {
		w.placeBlock(p, rid, x, y, z)
	}
}

// isInInvalidLocation ports MineShaftPiece.isInInvalidLocation: the
// mineshaft-blocking biome test at the clipped center plus a liquid scan over
// the clipped shell.
func (w *mineshaftPlacer) isInInvalidLocation(p *mineshaftPiece) bool {
	x0 := max(p.bb.minX-1, w.chunkBB.minX)
	y0 := max(p.bb.minY-1, w.chunkBB.minY)
	z0 := max(p.bb.minZ-1, w.chunkBB.minZ)
	x1 := min(p.bb.maxX+1, w.chunkBB.maxX)
	y1 := min(p.bb.maxY+1, w.chunkBB.maxY)
	z1 := min(p.bb.maxZ+1, w.chunkBB.maxZ)
	if w.g.zoomedBiomeAt((x0+x1)/2, (y0+y1)/2, (z0+z1)/2) == gen.BiomeDeepDark {
		return true
	}
	for x := x0; x <= x1; x++ {
		for z := z0; z <= z1; z++ {
			if mineshaftIsLiquidName(w.nameAt(cube.Pos{x, y0, z})) {
				return true
			}
			if mineshaftIsLiquidName(w.nameAt(cube.Pos{x, y1, z})) {
				return true
			}
		}
	}
	for x := x0; x <= x1; x++ {
		for y := y0; y <= y1; y++ {
			if mineshaftIsLiquidName(w.nameAt(cube.Pos{x, y, z0})) {
				return true
			}
			if mineshaftIsLiquidName(w.nameAt(cube.Pos{x, y, z1})) {
				return true
			}
		}
	}
	for z := z0; z <= z1; z++ {
		for y := y0; y <= y1; y++ {
			if mineshaftIsLiquidName(w.nameAt(cube.Pos{x0, y, z})) {
				return true
			}
			if mineshaftIsLiquidName(w.nameAt(cube.Pos{x1, y, z})) {
				return true
			}
		}
	}
	return false
}

// railRID resolves the corridor rail state (SHAPE north_south) after the
// piece's mirror/rotation: east/west corridors rotate it to east_west.
func (w *mineshaftPlacer) railRID(p *mineshaftPiece, northSouth bool) uint32 {
	rotated := p.orientation == mineshaftDirWest || p.orientation == mineshaftDirEast
	if northSouth != rotated {
		return w.railNSRID
	}
	return w.railEWRID
}

// torchRID resolves a WALL_TORCH with the given Java facing after the piece's
// mirror (LEFT_RIGHT for south/west) and rotation (CLOCKWISE_90 for
// west/east) as applied by StructurePiece.placeBlock.
func (w *mineshaftPlacer) torchRID(p *mineshaftPiece, facing mineshaftDir) uint32 {
	switch p.orientation {
	case mineshaftDirSouth:
		// mirror LEFT_RIGHT: N <-> S
		switch facing {
		case mineshaftDirNorth:
			facing = mineshaftDirSouth
		case mineshaftDirSouth:
			facing = mineshaftDirNorth
		}
	case mineshaftDirWest:
		// mirror LEFT_RIGHT then rotate CW90: S -> N -> E, N -> S -> W
		switch facing {
		case mineshaftDirNorth:
			facing = mineshaftDirWest
		case mineshaftDirSouth:
			facing = mineshaftDirEast
		case mineshaftDirWest:
			facing = mineshaftDirSouth
		case mineshaftDirEast:
			facing = mineshaftDirNorth
		}
	case mineshaftDirEast:
		// rotate CW90: N -> E, E -> S, S -> W, W -> N
		switch facing {
		case mineshaftDirNorth:
			facing = mineshaftDirEast
		case mineshaftDirSouth:
			facing = mineshaftDirWest
		case mineshaftDirWest:
			facing = mineshaftDirNorth
		case mineshaftDirEast:
			facing = mineshaftDirSouth
		}
	}
	return w.torchRIDs[facing]
}

// roomPostProcess ports MineShaftRoom.postProcess.
func (w *mineshaftPlacer) roomPostProcess(p *mineshaftPiece) {
	if w.isInInvalidLocation(p) {
		return
	}
	bb := p.bb
	w.generateBox(p, bb.minX, bb.minY+1, bb.minZ, bb.maxX, min(bb.minY+3, bb.maxY), bb.maxZ, w.airRID, w.airRID)
	for _, entrance := range p.entrances {
		w.generateBox(p, entrance.minX, entrance.maxY-2, entrance.minZ, entrance.maxX, entrance.maxY, entrance.maxZ, w.airRID, w.airRID)
	}
	w.generateUpperHalfSphere(p, bb.minX, bb.minY+4, bb.minZ, bb.maxX, bb.maxY, bb.maxZ, w.airRID)
}

// generateUpperHalfSphere ports StructurePiece.generateUpperHalfSphere with
// skipAir=false, matching the float32 math.
func (w *mineshaftPlacer) generateUpperHalfSphere(p *mineshaftPiece, x0, y0, z0, x1, y1, z1 int, fillRID uint32) {
	diagX := float32(x1 - x0 + 1)
	diagY := float32(y1 - y0 + 1)
	diagZ := float32(z1 - z0 + 1)
	cx := float32(x0) + diagX/2.0
	cz := float32(z0) + diagZ/2.0
	for y := y0; y <= y1; y++ {
		ny := float32(y-y0) / diagY
		for x := x0; x <= x1; x++ {
			nx := (float32(x) - cx) / (diagX * 0.5)
			for z := z0; z <= z1; z++ {
				nz := (float32(z) - cz) / (diagZ * 0.5)
				if nx*nx+ny*ny+nz*nz <= 1.05 {
					w.placeBlock(p, fillRID, x, y, z)
				}
			}
		}
	}
}

// corridorPostProcess ports MineShaftCorridor.postProcess.
func (w *mineshaftPlacer) corridorPostProcess(p *mineshaftPiece) {
	if w.isInInvalidLocation(p) {
		return
	}
	length := p.numSections*5 - 1
	w.generateBox(p, 0, 0, 0, 2, 1, length, w.airRID, w.airRID)
	w.generateMaybeBox(p, 0.8, 0, 2, 0, 2, 2, length, w.airRID, w.airRID, false)
	if p.spiderCorridor {
		w.generateMaybeBox(p, 0.6, 0, 0, 0, 2, 1, length, w.webRID, w.airRID, true)
	}

	// hasPlacedSpider persists across chunks on the shared vanilla piece; we
	// re-simulate per chunk, so a corridor spanning several chunks may place
	// a spawner in more than one of them.
	hasPlacedSpider := false
	for section := 0; section < p.numSections; section++ {
		z := 2 + section*5
		w.placeSupport(p, 0, 0, z, 2, 2)
		w.maybePlaceCobWeb(p, 0.1, 0, 2, z-1)
		w.maybePlaceCobWeb(p, 0.1, 2, 2, z-1)
		w.maybePlaceCobWeb(p, 0.1, 0, 2, z+1)
		w.maybePlaceCobWeb(p, 0.1, 2, 2, z+1)
		w.maybePlaceCobWeb(p, 0.05, 0, 2, z-2)
		w.maybePlaceCobWeb(p, 0.05, 2, 2, z-2)
		w.maybePlaceCobWeb(p, 0.05, 0, 2, z+2)
		w.maybePlaceCobWeb(p, 0.05, 2, 2, z+2)
		if w.rng.NextInt(100) == 0 {
			w.createChest(p, 2, 0, z-1)
		}
		if w.rng.NextInt(100) == 0 {
			w.createChest(p, 0, 0, z+1)
		}
		if p.spiderCorridor && !hasPlacedSpider {
			newZ := z - 1 + int(w.rng.NextInt(3))
			pos := w.worldPos(p, 1, 0, newZ)
			if w.inside(pos) && w.isInterior(p, 1, 0, newZ) {
				hasPlacedSpider = true
				w.setBlock(pos, w.spawnerRID)
				// SpawnerBlockEntity.setEntityId does not draw from the
				// worldgen random (empty spawn potentials list).
			}
		}
	}

	for x := 0; x <= 2; x++ {
		for z := 0; z <= length; z++ {
			w.setPlanksBlock(p, x, -1, z)
		}
	}

	w.placeDoubleLowerOrUpperSupport(p, 0, -1, 2)
	if p.numSections > 1 {
		w.placeDoubleLowerOrUpperSupport(p, 0, -1, length-2)
	}

	if p.hasRails {
		for z := 0; z <= length; z++ {
			floorName := w.getBlockName(p, 1, -1, z)
			if !mineshaftIsAirName(floorName) && w.isSolidRender(floorName) {
				probability := float32(0.9)
				if w.isInterior(p, 1, 0, z) {
					probability = 0.7
				}
				w.maybeGenerateBlock(p, probability, 1, 0, z, w.railRID(p, true))
			}
		}
	}
}

// isSupportingBox ports MineShaftPiece.isSupportingBox.
func (w *mineshaftPlacer) isSupportingBox(p *mineshaftPiece, x0, x1, y1, z int) bool {
	for x := x0; x <= x1; x++ {
		if mineshaftIsAirName(w.getBlockName(p, x, y1+1, z)) {
			return false
		}
	}
	return true
}

// placeSupport ports MineShaftCorridor.placeSupport.
func (w *mineshaftPlacer) placeSupport(p *mineshaftPiece, x0, y0, z, y1, x1 int) {
	if !w.isSupportingBox(p, x0, x1, y1, z) {
		return
	}
	w.generateBox(p, x0, y0, z, x0, y1-1, z, w.fenceRID, w.airRID)
	w.generateBox(p, x1, y0, z, x1, y1-1, z, w.fenceRID, w.airRID)
	if w.rng.NextInt(4) == 0 {
		w.generateBox(p, x0, y1, z, x0, y1, z, w.planksRID, w.airRID)
		w.generateBox(p, x1, y1, z, x1, y1, z, w.planksRID, w.airRID)
	} else {
		w.generateBox(p, x0, y1, z, x1, y1, z, w.planksRID, w.airRID)
		w.maybeGenerateBlock(p, 0.05, x0+1, y1, z-1, w.torchRID(p, mineshaftDirSouth))
		w.maybeGenerateBlock(p, 0.05, x0+1, y1, z+1, w.torchRID(p, mineshaftDirNorth))
	}
}

// maybePlaceCobWeb ports MineShaftCorridor.maybePlaceCobWeb: the random draw
// happens only when the position is interior.
func (w *mineshaftPlacer) maybePlaceCobWeb(p *mineshaftPiece, probability float32, x, y, z int) {
	if !w.isInterior(p, x, y, z) {
		return
	}
	if !(w.rng.NextFloat() < probability) {
		return
	}
	if !w.hasSturdyNeighbours(p, x, y, z, 2) {
		return
	}
	w.placeBlock(p, w.webRID, x, y, z)
}

// hasSturdyNeighbours ports MineShaftCorridor.hasSturdyNeighbours.
func (w *mineshaftPlacer) hasSturdyNeighbours(p *mineshaftPiece, x, y, z, count int) bool {
	pos := w.worldPos(p, x, y, z)
	sturdy := 0
	// Direction.values() order: DOWN, UP, NORTH, SOUTH, WEST, EAST.
	for _, offset := range [6]cube.Pos{{0, -1, 0}, {0, 1, 0}, {0, 0, -1}, {0, 0, 1}, {-1, 0, 0}, {1, 0, 0}} {
		neighbour := pos.Add(offset)
		if w.inside(neighbour) && w.faceSturdy(w.nameAt(neighbour)) {
			sturdy++
			if sturdy >= count {
				return true
			}
		}
	}
	return false
}

// createChest ports MineShaftCorridor.createChest: places a rail below the
// chest minecart and consumes the loot-table nextLong; the minecart entity
// itself is not represented in block data.
func (w *mineshaftPlacer) createChest(p *mineshaftPiece, x, y, z int) {
	pos := w.worldPos(p, x, y, z)
	if !w.inside(pos) || !mineshaftIsAirName(w.nameAt(pos)) {
		return
	}
	below := cube.Pos{pos[0], pos[1] - 1, pos[2]}
	if below[1] < w.levelMinY || mineshaftIsAirName(w.nameAt(below)) {
		return
	}
	northSouth := w.rng.NextBool()
	w.placeBlock(p, w.railRID(p, northSouth), x, y, z)
	w.rng.NextLong() // chest minecart loot table seed
}

// setPlanksBlock ports MineShaftPiece.setPlanksBlock.
func (w *mineshaftPlacer) setPlanksBlock(p *mineshaftPiece, x, y, z int) {
	if !w.isInterior(p, x, y, z) {
		return
	}
	pos := w.worldPos(p, x, y, z)
	if pos[1] < w.levelMinY {
		return
	}
	if !w.faceSturdy(w.nameAt(pos)) {
		w.setBlock(pos, w.planksRID)
	}
}

// placeDoubleLowerOrUpperSupport ports
// MineShaftCorridor.placeDoubleLowerOrUpperSupport.
func (w *mineshaftPlacer) placeDoubleLowerOrUpperSupport(p *mineshaftPiece, x, y, z int) {
	if w.getBlockName(p, x, y, z) == w.planksName {
		w.fillPillarDownOrChainUp(p, x, y, z)
	}
	if w.getBlockName(p, x+2, y, z) == w.planksName {
		w.fillPillarDownOrChainUp(p, x+2, y, z)
	}
}

// fillPillarDownOrChainUp ports MineShaftCorridor.fillPillarDownOrChainUp.
func (w *mineshaftPlacer) fillPillarDownOrChainUp(p *mineshaftPiece, x, y, z int) {
	pos := w.worldPos(p, x, y, z)
	if !w.inside(pos) {
		return
	}
	worldY := pos[1]
	distance := 1
	checkBelow := true
	checkAbove := true
	for ; checkBelow || checkAbove; distance++ {
		if checkBelow {
			below := cube.Pos{pos[0], worldY - distance, pos[2]}
			belowName := w.nameAt(below)
			emptyBelow := mineshaftIsReplaceableByStructures(belowName) && !mineshaftIsLavaName(belowName)
			if !emptyBelow && w.faceSturdy(belowName) {
				w.fillColumnBetween(pos[0], pos[2], worldY-distance+1, worldY, w.logRID)
				return
			}
			checkBelow = distance <= 20 && emptyBelow && below[1] > w.levelMinY+1
		}
		if checkAbove {
			above := cube.Pos{pos[0], worldY + distance, pos[2]}
			aboveName := w.nameAt(above)
			emptyAbove := mineshaftIsReplaceableByStructures(aboveName)
			if !emptyAbove && w.canHangChainBelow(aboveName) {
				w.setBlock(cube.Pos{pos[0], worldY + 1, pos[2]}, w.fenceRID)
				w.fillColumnBetween(pos[0], pos[2], worldY+2, worldY+distance, w.chainRID)
				return
			}
			checkAbove = distance <= 50 && emptyAbove && above[1] < w.levelMaxY
		}
	}
}

func (w *mineshaftPlacer) fillColumnBetween(x, z, bottomInclusive, topExclusive int, rid uint32) {
	for y := bottomInclusive; y < topExclusive; y++ {
		w.setBlock(cube.Pos{x, y, z}, rid)
	}
}

// canHangChainBelow ports MineShaftCorridor.canHangChainBelow:
// Block.canSupportCenter (approximated as face-sturdy) and not a FallingBlock.
func (w *mineshaftPlacer) canHangChainBelow(name string) bool {
	switch name {
	case "gravel", "sand", "red_sand", "suspicious_gravel", "suspicious_sand":
		return false
	}
	return w.faceSturdy(name)
}

// crossingPostProcess ports MineShaftCrossing.postProcess. Crossings have a
// null orientation, so local coordinates equal world coordinates.
func (w *mineshaftPlacer) crossingPostProcess(p *mineshaftPiece) {
	if w.isInInvalidLocation(p) {
		return
	}
	bb := p.bb
	if p.isTwoFloored {
		w.generateBox(p, bb.minX+1, bb.minY, bb.minZ, bb.maxX-1, bb.minY+3-1, bb.maxZ, w.airRID, w.airRID)
		w.generateBox(p, bb.minX, bb.minY, bb.minZ+1, bb.maxX, bb.minY+3-1, bb.maxZ-1, w.airRID, w.airRID)
		w.generateBox(p, bb.minX+1, bb.maxY-2, bb.minZ, bb.maxX-1, bb.maxY, bb.maxZ, w.airRID, w.airRID)
		w.generateBox(p, bb.minX, bb.maxY-2, bb.minZ+1, bb.maxX, bb.maxY, bb.maxZ-1, w.airRID, w.airRID)
		w.generateBox(p, bb.minX+1, bb.minY+3, bb.minZ+1, bb.maxX-1, bb.minY+3, bb.maxZ-1, w.airRID, w.airRID)
	} else {
		w.generateBox(p, bb.minX+1, bb.minY, bb.minZ, bb.maxX-1, bb.maxY, bb.maxZ, w.airRID, w.airRID)
		w.generateBox(p, bb.minX, bb.minY, bb.minZ+1, bb.maxX, bb.maxY, bb.maxZ-1, w.airRID, w.airRID)
	}

	w.placeSupportPillar(p, bb.minX+1, bb.minY, bb.minZ+1, bb.maxY)
	w.placeSupportPillar(p, bb.minX+1, bb.minY, bb.maxZ-1, bb.maxY)
	w.placeSupportPillar(p, bb.maxX-1, bb.minY, bb.minZ+1, bb.maxY)
	w.placeSupportPillar(p, bb.maxX-1, bb.minY, bb.maxZ-1, bb.maxY)
	floorY := bb.minY - 1
	for x := bb.minX; x <= bb.maxX; x++ {
		for z := bb.minZ; z <= bb.maxZ; z++ {
			w.setPlanksBlock(p, x, floorY, z)
		}
	}
}

// placeSupportPillar ports MineShaftCrossing.placeSupportPillar.
func (w *mineshaftPlacer) placeSupportPillar(p *mineshaftPiece, x, y0, z, y1 int) {
	if !mineshaftIsAirName(w.getBlockName(p, x, y1+1, z)) {
		w.generateBox(p, x, y0, z, x, y1, z, w.planksRID, w.airRID)
	}
}

// stairsPostProcess ports MineShaftStairs.postProcess.
func (w *mineshaftPlacer) stairsPostProcess(p *mineshaftPiece) {
	if w.isInInvalidLocation(p) {
		return
	}
	w.generateBox(p, 0, 5, 0, 2, 7, 1, w.airRID, w.airRID)
	w.generateBox(p, 0, 0, 7, 2, 2, 8, w.airRID, w.airRID)
	for i := 0; i < 5; i++ {
		extra := 0
		if i < 4 {
			extra = 1
		}
		w.generateBox(p, 0, 5-i-extra, 2+i, 2, 7-i, 2+i, w.airRID, w.airRID)
	}
}
