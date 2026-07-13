package vanilla

import (
	gen "github.com/bedrock-mc/vanilla-gen/gen"
	"github.com/df-mc/dragonfly/server/block/cube"
)

func (g Generator) buildJungleTempleStructure(startX, startZ int, surfaceSampler *structureHeightSampler, rng *gen.WorldgenRandom) (string, []plannedStructurePiece, structureBox, cube.Pos, [3]int, bool) {
	const (
		width     = 12
		height    = 10
		depth     = 15
		localMinY = -4
		localMaxY = 9
	)

	rotation := randomStructureRotation(rng)
	bboxMin, rotation := proceduralStructurePlacementFromFoot(
		cube.Pos{startX, 64, startZ},
		0, 0, 0,
		width, height, depth,
		structureFacingFromRotation(rotation),
	)
	footprint := proceduralStructureWorldBox(cube.Pos{bboxMin[0], 0, bboxMin[2]}, [3]int{width, height, depth}, rotation)
	totalY := 0
	count := 0
	for x := footprint.minX; x <= footprint.maxX; x++ {
		for z := footprint.minZ; z <= footprint.maxZ; z++ {
			totalY += surfaceSampler.worldSurfaceLevelAt(x, z) - 1
			count++
		}
	}
	if count == 0 {
		return "", nil, emptyStructureBox(), cube.Pos{}, [3]int{}, false
	}
	bboxMinY := clamp(totalY/count, surfaceSampler.minY+1, surfaceSampler.maxY)
	builder := newProceduralStructureBuilder(
		cube.Pos{bboxMin[0], bboxMinY + localMinY, bboxMin[2]},
		cube.Pos{0, localMinY, 0},
		[3]int{width, localMaxY - localMinY + 1, depth},
		rotation,
	)

	g.buildJungleTempleBlocks(builder, rng)
	piece := builder.piece()
	rootOrigin, rootSize := piece.bounds.originAndSize()
	return "jungle_pyramid", []plannedStructurePiece{piece}, piece.bounds, rootOrigin, rootSize, true
}

func (g Generator) buildJungleTempleBlocks(b *proceduralStructureBuilder, rng *gen.WorldgenRandom) {
	air := structureState("air")
	cobble := structureState("cobblestone")
	mossy := structureState("mossy_cobblestone")
	cobbleSelector := func(_, _, _ int, _ bool) gen.BlockState {
		if rng.NextDouble() < 0.4 {
			return cobble
		}
		return mossy
	}
	cobbleNorth := structureState("stone_stairs", "facing", "north")
	cobbleSouth := structureState("stone_stairs", "facing", "south")
	cobbleEast := structureState("stone_stairs", "facing", "east")
	cobbleWest := structureState("stone_stairs", "facing", "west")
	chiseledStoneBricks := structureState("chiseled_stone_bricks")

	b.fillSelectedBox(0, -4, 0, 11, 0, 14, cobbleSelector)
	b.fillSelectedBox(2, 1, 2, 9, 2, 2, cobbleSelector)
	b.fillSelectedBox(2, 1, 12, 9, 2, 12, cobbleSelector)
	b.fillSelectedBox(2, 1, 3, 2, 2, 11, cobbleSelector)
	b.fillSelectedBox(9, 1, 3, 9, 2, 11, cobbleSelector)
	b.fillSelectedBox(1, 3, 1, 10, 6, 1, cobbleSelector)
	b.fillSelectedBox(1, 3, 13, 10, 6, 13, cobbleSelector)
	b.fillSelectedBox(1, 3, 2, 1, 6, 12, cobbleSelector)
	b.fillSelectedBox(10, 3, 2, 10, 6, 12, cobbleSelector)
	b.fillSelectedBox(2, 3, 2, 9, 3, 12, cobbleSelector)
	b.fillSelectedBox(2, 6, 2, 9, 6, 12, cobbleSelector)
	b.fillSelectedBox(3, 7, 3, 8, 7, 11, cobbleSelector)
	b.fillSelectedBox(4, 8, 4, 7, 8, 10, cobbleSelector)

	b.fillAirBox(3, 1, 3, 8, 2, 11)
	b.fillAirBox(4, 3, 6, 7, 3, 9)
	b.fillAirBox(2, 4, 2, 9, 5, 12)
	b.fillAirBox(4, 6, 5, 7, 6, 9)
	b.fillAirBox(5, 7, 6, 6, 7, 8)
	b.fillAirBox(5, 1, 2, 6, 2, 2)
	b.fillAirBox(5, 2, 12, 6, 2, 12)
	b.fillAirBox(5, 5, 1, 6, 5, 1)
	b.fillAirBox(5, 5, 13, 6, 5, 13)

	b.setBlock(1, 5, 5, air)
	b.setBlock(10, 5, 5, air)
	b.setBlock(1, 5, 9, air)
	b.setBlock(10, 5, 9, air)

	for z := 0; z <= 14; z += 14 {
		b.fillSelectedBox(2, 4, z, 2, 5, z, cobbleSelector)
		b.fillSelectedBox(4, 4, z, 4, 5, z, cobbleSelector)
		b.fillSelectedBox(7, 4, z, 7, 5, z, cobbleSelector)
		b.fillSelectedBox(9, 4, z, 9, 5, z, cobbleSelector)
	}

	b.fillSelectedBox(5, 6, 0, 6, 6, 0, cobbleSelector)
	for x := 0; x <= 11; x += 11 {
		for z := 2; z <= 12; z += 2 {
			b.fillSelectedBox(x, 4, z, x, 5, z, cobbleSelector)
		}
		b.fillSelectedBox(x, 6, 5, x, 6, 5, cobbleSelector)
		b.fillSelectedBox(x, 6, 9, x, 6, 9, cobbleSelector)
	}

	b.fillSelectedBox(2, 7, 2, 2, 9, 2, cobbleSelector)
	b.fillSelectedBox(9, 7, 2, 9, 9, 2, cobbleSelector)
	b.fillSelectedBox(2, 7, 12, 2, 9, 12, cobbleSelector)
	b.fillSelectedBox(9, 7, 12, 9, 9, 12, cobbleSelector)
	b.fillSelectedBox(4, 9, 4, 4, 9, 4, cobbleSelector)
	b.fillSelectedBox(7, 9, 4, 7, 9, 4, cobbleSelector)
	b.fillSelectedBox(4, 9, 10, 4, 9, 10, cobbleSelector)
	b.fillSelectedBox(7, 9, 10, 7, 9, 10, cobbleSelector)
	b.fillSelectedBox(5, 9, 7, 6, 9, 7, cobbleSelector)

	b.setBlock(5, 9, 6, cobbleNorth)
	b.setBlock(6, 9, 6, cobbleNorth)
	b.setBlock(5, 9, 8, cobbleSouth)
	b.setBlock(6, 9, 8, cobbleSouth)
	b.setBlock(4, 0, 0, cobbleNorth)
	b.setBlock(5, 0, 0, cobbleNorth)
	b.setBlock(6, 0, 0, cobbleNorth)
	b.setBlock(7, 0, 0, cobbleNorth)
	b.setBlock(4, 1, 8, cobbleNorth)
	b.setBlock(4, 2, 9, cobbleNorth)
	b.setBlock(4, 3, 10, cobbleNorth)
	b.setBlock(7, 1, 8, cobbleNorth)
	b.setBlock(7, 2, 9, cobbleNorth)
	b.setBlock(7, 3, 10, cobbleNorth)

	b.fillSelectedBox(4, 1, 9, 4, 1, 9, cobbleSelector)
	b.fillSelectedBox(7, 1, 9, 7, 1, 9, cobbleSelector)
	b.fillSelectedBox(4, 1, 10, 7, 2, 10, cobbleSelector)
	b.fillSelectedBox(5, 4, 5, 6, 4, 5, cobbleSelector)
	b.setBlock(4, 4, 5, cobbleEast)
	b.setBlock(7, 4, 5, cobbleWest)

	for i := 0; i < 4; i++ {
		b.setBlock(5, -i, 6+i, cobbleSouth)
		b.setBlock(6, -i, 6+i, cobbleSouth)
		b.fillAirBox(5, -i, 7+i, 6, -i, 9+i)
	}

	b.fillAirBox(1, -3, 12, 10, -1, 13)
	b.fillAirBox(1, -3, 1, 3, -1, 13)
	b.fillAirBox(1, -3, 1, 9, -1, 5)

	for z := 1; z <= 13; z += 2 {
		b.fillSelectedBox(1, -3, z, 1, -2, z, cobbleSelector)
	}
	for z := 2; z <= 12; z += 2 {
		b.fillSelectedBox(1, -1, z, 3, -1, z, cobbleSelector)
	}

	b.fillSelectedBox(2, -2, 1, 5, -2, 1, cobbleSelector)
	b.fillSelectedBox(7, -2, 1, 9, -2, 1, cobbleSelector)
	b.fillSelectedBox(6, -3, 1, 6, -3, 1, cobbleSelector)
	b.fillSelectedBox(6, -1, 1, 6, -1, 1, cobbleSelector)

	b.placeTripwireHook(1, -3, 8, "east", true)
	b.placeTripwireHook(4, -3, 8, "west", true)
	b.placeTripwire(2, -3, 8, false, true, false, true, true)
	b.placeTripwire(3, -3, 8, false, true, false, true, true)
	b.placeRedstoneWire(5, -3, 7, "side", "none", "side", "none")
	b.placeRedstoneWire(5, -3, 6, "side", "none", "side", "none")
	b.placeRedstoneWire(5, -3, 5, "side", "none", "side", "none")
	b.placeRedstoneWire(5, -3, 4, "side", "none", "side", "none")
	b.placeRedstoneWire(5, -3, 3, "side", "none", "side", "none")
	b.placeRedstoneWire(5, -3, 2, "side", "none", "side", "none")
	b.placeRedstoneWire(5, -3, 1, "side", "none", "none", "side")
	b.placeRedstoneWire(4, -3, 1, "none", "side", "none", "side")
	b.setBlock(3, -3, 1, mossy)
	b.placeDispenser(3, -2, 1, "north")
	b.placeVine(3, -2, 2, false, false, true, false)

	b.placeTripwireHook(7, -3, 1, "north", true)
	b.placeTripwireHook(7, -3, 5, "south", true)
	b.placeTripwire(7, -3, 2, true, false, true, false, true)
	b.placeTripwire(7, -3, 3, true, false, true, false, true)
	b.placeTripwire(7, -3, 4, true, false, true, false, true)

	b.placeRedstoneWire(8, -3, 6, "none", "side", "none", "side")
	b.placeRedstoneWire(9, -3, 6, "none", "none", "side", "side")
	b.placeRedstoneWire(9, -3, 5, "side", "none", "up", "none")
	b.setBlock(9, -3, 4, mossy)
	b.placeRedstoneWire(9, -2, 4, "side", "none", "side", "none")
	b.placeDispenser(9, -2, 3, "west")
	b.placeVine(8, -1, 3, false, true, false, false)
	b.placeVine(8, -2, 3, false, true, false, false)
	b.placeChest(8, -3, 3, "south")

	b.setBlock(9, -3, 2, mossy)
	b.setBlock(8, -3, 1, mossy)
	b.setBlock(4, -3, 5, mossy)
	b.setBlock(5, -2, 5, mossy)
	b.setBlock(5, -1, 5, mossy)
	b.setBlock(6, -3, 5, mossy)
	b.setBlock(7, -2, 5, mossy)
	b.setBlock(7, -1, 5, mossy)
	b.setBlock(8, -3, 5, mossy)
	b.fillSelectedBox(9, -1, 1, 9, -1, 5, cobbleSelector)
	b.fillAirBox(8, -3, 8, 10, -1, 10)

	b.setBlock(8, -2, 11, chiseledStoneBricks)
	b.setBlock(9, -2, 11, chiseledStoneBricks)
	b.setBlock(10, -2, 11, chiseledStoneBricks)
	b.placeLever(8, -2, 12, "north", "wall")
	b.placeLever(9, -2, 12, "north", "wall")
	b.placeLever(10, -2, 12, "north", "wall")
	b.fillSelectedBox(8, -3, 8, 8, -3, 10, cobbleSelector)
	b.fillSelectedBox(10, -3, 8, 10, -3, 10, cobbleSelector)
	b.setBlock(10, -2, 9, mossy)
	b.placeRedstoneWire(8, -2, 9, "side", "none", "side", "none")
	b.placeRedstoneWire(8, -2, 10, "side", "none", "side", "none")
	b.placeRedstoneWire(10, -1, 9, "side", "side", "side", "side")
	b.placeStickyPiston(9, -2, 8, "up")
	b.placeStickyPiston(10, -2, 8, "west")
	b.placeStickyPiston(10, -1, 8, "west")
	b.placeRepeater(10, -2, 10, false, "north", 0)
	b.placeChest(9, -3, 10, "south")
}

func (g Generator) buildDesertPyramidStructure(startX, startZ int, surfaceSampler *structureHeightSampler, rng *gen.WorldgenRandom) (string, []plannedStructurePiece, structureBox, cube.Pos, [3]int, bool) {
	const (
		width     = 21
		height    = 15
		depth     = 21
		localMinY = -14
		localMaxY = 10
	)

	rotation := randomStructureRotation(rng)
	bboxMin, rotation := proceduralStructurePlacementFromFoot(
		cube.Pos{startX, 64, startZ},
		0, 0, 0,
		width, height, depth,
		structureFacingFromRotation(rotation),
	)
	footprint := proceduralStructureWorldBox(cube.Pos{bboxMin[0], 0, bboxMin[2]}, [3]int{width, height, depth}, rotation)
	lowestGround := surfaceSampler.maxY + 1
	for x := footprint.minX; x <= footprint.maxX; x++ {
		for z := footprint.minZ; z <= footprint.maxZ; z++ {
			y := surfaceSampler.worldSurfaceLevelAt(x, z) - 1
			if y < lowestGround {
				lowestGround = y
			}
		}
	}
	if lowestGround > surfaceSampler.maxY {
		return "", nil, emptyStructureBox(), cube.Pos{}, [3]int{}, false
	}
	bboxMinY := clamp(lowestGround-int(rng.NextInt(3)), surfaceSampler.minY+1, surfaceSampler.maxY)
	builder := newProceduralStructureBuilder(
		cube.Pos{bboxMin[0], bboxMinY + localMinY, bboxMin[2]},
		cube.Pos{0, localMinY, 0},
		[3]int{width, localMaxY - localMinY + 1, depth},
		rotation,
	)

	g.buildDesertPyramidBlocks(builder, rng, surfaceSampler.minY)
	piece := builder.piece()
	rootOrigin, rootSize := piece.bounds.originAndSize()
	return "desert_pyramid", []plannedStructurePiece{piece}, piece.bounds, rootOrigin, rootSize, true
}

func (g Generator) buildDesertPyramidBlocks(b *proceduralStructureBuilder, rng *gen.WorldgenRandom, minY int) {
	sand := structureState("sand")
	sandstone := structureState("sandstone")
	cutSandstone := structureState("cut_sandstone")
	chiseledSandstone := structureState("chiseled_sandstone")
	air := structureState("air")
	orangeTerracotta := structureState("orange_terracotta")
	blueTerracotta := structureState("blue_terracotta")
	sandstoneSlab := structureState("sandstone_slab")
	stonePressurePlate := structureState("stone_pressure_plate")
	tnt := structureState("tnt")
	sandstoneNorth := structureState("sandstone_stairs", "facing", "north")
	sandstoneSouth := structureState("sandstone_stairs", "facing", "south")
	sandstoneEast := structureState("sandstone_stairs", "facing", "east")
	sandstoneWest := structureState("sandstone_stairs", "facing", "west")

	b.fillSolidBox(0, -4, 0, 20, 0, 20, sandstone)
	for pos := 1; pos <= 9; pos++ {
		b.fillSolidBox(pos, pos, pos, 20-pos, pos, 20-pos, sandstone)
		b.fillSolidBox(pos+1, pos, pos+1, 19-pos, pos, 19-pos, air)
	}
	for x := 0; x < 21; x++ {
		for z := 0; z < 21; z++ {
			b.fillFoundationColumn(g, sandstone, x, -5, z, minY)
		}
	}

	b.fillHollowBox(0, 0, 0, 4, 9, 4, sandstone, air)
	b.fillSolidBox(1, 10, 1, 3, 10, 3, sandstone)
	b.setBlock(2, 10, 0, sandstoneNorth)
	b.setBlock(2, 10, 4, sandstoneSouth)
	b.setBlock(0, 10, 2, sandstoneEast)
	b.setBlock(4, 10, 2, sandstoneWest)

	b.fillHollowBox(16, 0, 0, 20, 9, 4, sandstone, air)
	b.fillSolidBox(17, 10, 1, 19, 10, 3, sandstone)
	b.setBlock(18, 10, 0, sandstoneNorth)
	b.setBlock(18, 10, 4, sandstoneSouth)
	b.setBlock(16, 10, 2, sandstoneEast)
	b.setBlock(20, 10, 2, sandstoneWest)

	b.fillHollowBox(8, 0, 0, 12, 4, 4, sandstone, air)
	b.fillSolidBox(9, 1, 1, 9, 3, 1, cutSandstone)
	b.fillSolidBox(10, 3, 1, 10, 3, 1, cutSandstone)
	b.fillSolidBox(11, 1, 1, 11, 3, 1, cutSandstone)
	b.fillHollowBox(4, 1, 1, 8, 3, 3, sandstone, air)
	b.fillSolidBox(4, 1, 2, 8, 2, 2, air)
	b.fillHollowBox(12, 1, 1, 16, 3, 3, sandstone, air)
	b.fillSolidBox(12, 1, 2, 16, 2, 2, air)
	b.fillSolidBox(5, 4, 5, 15, 4, 15, sandstone)
	b.fillSolidBox(9, 4, 9, 11, 4, 11, air)
	b.fillSolidBox(8, 1, 8, 8, 3, 8, cutSandstone)
	b.fillSolidBox(12, 1, 8, 12, 3, 8, cutSandstone)
	b.fillSolidBox(8, 1, 12, 8, 3, 12, cutSandstone)
	b.fillSolidBox(12, 1, 12, 12, 3, 12, cutSandstone)
	b.fillSolidBox(1, 1, 5, 4, 4, 11, sandstone)
	b.fillSolidBox(16, 1, 5, 19, 4, 11, sandstone)
	b.fillSolidBox(6, 7, 9, 6, 7, 11, sandstone)
	b.fillSolidBox(14, 7, 9, 14, 7, 11, sandstone)
	b.fillSolidBox(5, 5, 9, 5, 7, 11, cutSandstone)
	b.fillSolidBox(15, 5, 9, 15, 7, 11, cutSandstone)
	b.setBlock(5, 5, 10, air)
	b.setBlock(5, 6, 10, air)
	b.setBlock(6, 6, 10, air)
	b.setBlock(15, 5, 10, air)
	b.setBlock(15, 6, 10, air)
	b.setBlock(14, 6, 10, air)
	b.fillSolidBox(2, 4, 4, 2, 6, 4, air)
	b.fillSolidBox(18, 4, 4, 18, 6, 4, air)
	b.setBlock(2, 4, 5, sandstoneNorth)
	b.setBlock(2, 3, 4, sandstoneNorth)
	b.setBlock(18, 4, 5, sandstoneNorth)
	b.setBlock(18, 3, 4, sandstoneNorth)
	b.fillSolidBox(1, 1, 3, 2, 2, 3, sandstone)
	b.fillSolidBox(18, 1, 3, 19, 2, 3, sandstone)
	b.setBlock(1, 1, 2, sandstone)
	b.setBlock(19, 1, 2, sandstone)
	b.setBlock(1, 2, 2, sandstoneSlab)
	b.setBlock(19, 2, 2, sandstoneSlab)
	b.setBlock(2, 1, 2, sandstoneWest)
	b.setBlock(18, 1, 2, sandstoneEast)
	b.fillSolidBox(4, 3, 5, 4, 3, 17, sandstone)
	b.fillSolidBox(16, 3, 5, 16, 3, 17, sandstone)
	b.fillSolidBox(3, 1, 5, 4, 2, 16, air)
	b.fillSolidBox(15, 1, 5, 16, 2, 16, air)
	for z := 5; z <= 17; z += 2 {
		b.setBlock(4, 1, z, cutSandstone)
		b.setBlock(4, 2, z, chiseledSandstone)
		b.setBlock(16, 1, z, cutSandstone)
		b.setBlock(16, 2, z, chiseledSandstone)
	}

	b.setBlock(10, 0, 7, orangeTerracotta)
	b.setBlock(10, 0, 8, orangeTerracotta)
	b.setBlock(9, 0, 9, orangeTerracotta)
	b.setBlock(11, 0, 9, orangeTerracotta)
	b.setBlock(8, 0, 10, orangeTerracotta)
	b.setBlock(12, 0, 10, orangeTerracotta)
	b.setBlock(7, 0, 10, orangeTerracotta)
	b.setBlock(13, 0, 10, orangeTerracotta)
	b.setBlock(9, 0, 11, orangeTerracotta)
	b.setBlock(11, 0, 11, orangeTerracotta)
	b.setBlock(10, 0, 12, orangeTerracotta)
	b.setBlock(10, 0, 13, orangeTerracotta)
	b.setBlock(10, 0, 10, blueTerracotta)

	for _, x := range []int{0, 20} {
		b.setBlock(x, 2, 1, cutSandstone)
		b.setBlock(x, 2, 2, orangeTerracotta)
		b.setBlock(x, 2, 3, cutSandstone)
		b.setBlock(x, 3, 1, cutSandstone)
		b.setBlock(x, 3, 2, orangeTerracotta)
		b.setBlock(x, 3, 3, cutSandstone)
		b.setBlock(x, 4, 1, orangeTerracotta)
		b.setBlock(x, 4, 2, chiseledSandstone)
		b.setBlock(x, 4, 3, orangeTerracotta)
		b.setBlock(x, 5, 1, cutSandstone)
		b.setBlock(x, 5, 2, orangeTerracotta)
		b.setBlock(x, 5, 3, cutSandstone)
		b.setBlock(x, 6, 1, orangeTerracotta)
		b.setBlock(x, 6, 2, chiseledSandstone)
		b.setBlock(x, 6, 3, orangeTerracotta)
		b.setBlock(x, 7, 1, orangeTerracotta)
		b.setBlock(x, 7, 2, orangeTerracotta)
		b.setBlock(x, 7, 3, orangeTerracotta)
		b.setBlock(x, 8, 1, cutSandstone)
		b.setBlock(x, 8, 2, cutSandstone)
		b.setBlock(x, 8, 3, cutSandstone)
	}

	for _, x := range []int{2, 18} {
		b.setBlock(x-1, 2, 0, cutSandstone)
		b.setBlock(x, 2, 0, orangeTerracotta)
		b.setBlock(x+1, 2, 0, cutSandstone)
		b.setBlock(x-1, 3, 0, cutSandstone)
		b.setBlock(x, 3, 0, orangeTerracotta)
		b.setBlock(x+1, 3, 0, cutSandstone)
		b.setBlock(x-1, 4, 0, orangeTerracotta)
		b.setBlock(x, 4, 0, chiseledSandstone)
		b.setBlock(x+1, 4, 0, orangeTerracotta)
		b.setBlock(x-1, 5, 0, cutSandstone)
		b.setBlock(x, 5, 0, orangeTerracotta)
		b.setBlock(x+1, 5, 0, cutSandstone)
		b.setBlock(x-1, 6, 0, orangeTerracotta)
		b.setBlock(x, 6, 0, chiseledSandstone)
		b.setBlock(x+1, 6, 0, orangeTerracotta)
		b.setBlock(x-1, 7, 0, orangeTerracotta)
		b.setBlock(x, 7, 0, orangeTerracotta)
		b.setBlock(x+1, 7, 0, orangeTerracotta)
		b.setBlock(x-1, 8, 0, cutSandstone)
		b.setBlock(x, 8, 0, cutSandstone)
		b.setBlock(x+1, 8, 0, cutSandstone)
	}

	b.fillSolidBox(8, 4, 0, 12, 6, 0, cutSandstone)
	b.setBlock(8, 6, 0, air)
	b.setBlock(12, 6, 0, air)
	b.setBlock(9, 5, 0, orangeTerracotta)
	b.setBlock(10, 5, 0, chiseledSandstone)
	b.setBlock(11, 5, 0, orangeTerracotta)

	b.fillSolidBox(8, -14, 8, 12, -11, 12, cutSandstone)
	b.fillSolidBox(8, -10, 8, 12, -10, 12, chiseledSandstone)
	b.fillSolidBox(8, -9, 8, 12, -9, 12, cutSandstone)
	b.fillSolidBox(8, -8, 8, 12, -1, 12, sandstone)
	b.fillSolidBox(9, -11, 9, 11, -1, 11, air)
	b.setBlock(10, -11, 10, stonePressurePlate)
	b.fillHollowBox(9, -13, 9, 11, -13, 11, tnt, air)
	b.setBlock(8, -11, 10, air)
	b.setBlock(8, -10, 10, air)
	b.setBlock(7, -10, 10, chiseledSandstone)
	b.setBlock(7, -11, 10, cutSandstone)
	b.setBlock(12, -11, 10, air)
	b.setBlock(12, -10, 10, air)
	b.setBlock(13, -10, 10, chiseledSandstone)
	b.setBlock(13, -11, 10, cutSandstone)
	b.setBlock(10, -11, 8, air)
	b.setBlock(10, -10, 8, air)
	b.setBlock(10, -10, 7, chiseledSandstone)
	b.setBlock(10, -11, 7, cutSandstone)
	b.setBlock(10, -11, 12, air)
	b.setBlock(10, -10, 12, air)
	b.setBlock(10, -10, 13, chiseledSandstone)
	b.setBlock(10, -11, 13, cutSandstone)

	b.placeChest(12, -11, 10, "west")
	b.placeChest(8, -11, 10, "east")
	b.placeChest(10, -11, 8, "south")
	b.placeChest(10, -11, 12, "north")

	g.buildDesertPyramidCellar(b, rng, sand, sandstone, cutSandstone, chiseledSandstone, orangeTerracotta, blueTerracotta)
}

func (g Generator) buildDesertPyramidCellar(
	b *proceduralStructureBuilder,
	rng *gen.WorldgenRandom,
	sand, sandstone, cutSandstone, chiseledSandstone, orangeTerracotta, blueTerracotta gen.BlockState,
) {
	sandstoneWest := structureState("sandstone_stairs", "facing", "west")

	b.setBlock(13, -1, 17, sandstoneWest)
	b.setBlock(14, -2, 17, sandstoneWest)
	b.setBlock(15, -3, 17, sandstoneWest)

	x, y, z := 16, -4, 13
	variant := rng.NextDouble() < 0.5
	for dx := -4; dx <= 0; dx++ {
		b.setBlock(x+dx, y+4, z+4, sand)
	}
	b.setBlock(x-2, y+3, z+4, sand)
	if variant {
		b.setBlock(x-1, y+3, z+4, sand)
		b.setBlock(x, y+3, z+4, sandstone)
	} else {
		b.setBlock(x-1, y+3, z+4, sandstone)
		b.setBlock(x, y+3, z+4, sand)
	}
	b.setBlock(x-1, y+2, z+4, sand)
	b.setBlock(x, y+2, z+4, sandstone)
	b.setBlock(x, y+1, z+4, sand)

	b.fillSolidBox(x-3, y+1, z-3, x-3, y+1, z+2, cutSandstone)
	b.fillSolidBox(x+3, y+1, z-3, x+3, y+1, z+2, cutSandstone)
	b.fillSolidBox(x-3, y+1, z-3, x+3, y+1, z-2, cutSandstone)
	b.fillSolidBox(x-3, y+1, z+3, x+3, y+1, z+3, cutSandstone)
	b.fillSolidBox(x-3, y+2, z-3, x-3, y+2, z+2, chiseledSandstone)
	b.fillSolidBox(x+3, y+2, z-3, x+3, y+2, z+2, chiseledSandstone)
	b.fillSolidBox(x-3, y+2, z-3, x+3, y+2, z-2, chiseledSandstone)
	b.fillSolidBox(x-3, y+2, z+3, x+3, y+2, z+3, chiseledSandstone)
	b.fillSolidBox(x-3, -1, z-3, x-3, -1, z+2, cutSandstone)
	b.fillSolidBox(x+3, -1, z-3, x+3, -1, z+2, cutSandstone)
	b.fillSolidBox(x-3, -1, z-3, x+3, -1, z-2, cutSandstone)
	b.fillSolidBox(x-3, -1, z+3, x+3, -1, z+3, cutSandstone)

	b.fillSolidBox(x-2, y+1, z-2, x+2, y+3, z+2, sand)
	for roofX := x - 2; roofX <= x+2; roofX++ {
		for roofZ := z - 2; roofZ <= z+2; roofZ++ {
			if rng.NextDouble() < 0.33 {
				b.setBlock(roofX, y+4, roofZ, sandstone)
			} else {
				b.setBlock(roofX, y+4, roofZ, sand)
			}
		}
	}

	b.setBlock(x, y, z, blueTerracotta)
	b.setBlock(x+1, y, z-1, orangeTerracotta)
	b.setBlock(x+1, y, z+1, orangeTerracotta)
	b.setBlock(x-1, y, z-1, orangeTerracotta)
	b.setBlock(x-1, y, z+1, orangeTerracotta)
	b.setBlock(x+2, y, z, orangeTerracotta)
	b.setBlock(x-2, y, z, orangeTerracotta)
	b.setBlock(x, y, z+2, orangeTerracotta)
	b.setBlock(x, y, z-2, orangeTerracotta)
	b.setBlock(x+3, y, z, orangeTerracotta)
	b.setBlock(x+3, y+1, z, sand)
	b.setBlock(x+3, y+2, z, sand)
	b.setBlock(x+4, y+1, z, cutSandstone)
	b.setBlock(x+4, y+2, z, chiseledSandstone)
	b.setBlock(x-3, y, z, orangeTerracotta)
	b.setBlock(x-3, y+1, z, sand)
	b.setBlock(x-3, y+2, z, sand)
	b.setBlock(x-4, y+1, z, cutSandstone)
	b.setBlock(x-4, y+2, z, chiseledSandstone)
	b.setBlock(x, y, z+3, orangeTerracotta)
	b.setBlock(x, y+1, z+3, sand)
	b.setBlock(x, y+2, z+3, sand)
	b.setBlock(x, y, z-3, orangeTerracotta)
	b.setBlock(x, y+1, z-3, sand)
	b.setBlock(x, y+2, z-3, sand)
	b.setBlock(x, y+1, z-4, cutSandstone)
	b.setBlock(x, -2, z-4, chiseledSandstone)
}
