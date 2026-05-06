package vanilla

import (
	gen "github.com/bedrock-mc/vanilla-gen/gen"
	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
)

func (g Generator) buildStrongholdStructure(startChunk world.ChunkPos, startX, startZ int, surfaceSampler *structureHeightSampler, rng *gen.Xoroshiro128) (string, []plannedStructurePiece, structureBox, cube.Pos, [3]int, bool) {
	_ = startChunk

	size := [3]int{37, 13, 45}
	rotation := randomStructureRotation(rng)
	surfaceY := surfaceSampler.worldSurfaceLevelAt(startX+8, startZ+8) - 1
	originY := clamp(min(surfaceY-26, seaLevel-22), surfaceSampler.minY+5, seaLevel-22)
	origin := cube.Pos{startX + 2, originY, startZ + 2}
	builder := newProceduralStructureBuilder(origin, cube.Pos{}, size, rotation)

	air := gen.BlockState{Name: "air"}
	stoneBricks := blockStateFromWorldBlock(block.StoneBricks{Type: block.NormalStoneBricks()})
	mossyStoneBricks := blockStateFromWorldBlock(block.StoneBricks{Type: block.MossyStoneBricks()})
	crackedStoneBricks := blockStateFromWorldBlock(block.StoneBricks{Type: block.CrackedStoneBricks()})
	cobblestone := blockStateFromWorldBlock(block.Cobblestone{})
	planks := blockStateFromWorldBlock(block.Planks{Wood: block.OakWood()})
	bookshelf := blockStateFromWorldBlock(block.Bookshelf{})
	ironBars := blockStateFromWorldBlock(block.IronBars{})
	oakFence := blockStateFromWorldBlock(block.WoodFence{Wood: block.OakWood()})
	stoneBrickSlab := blockStateFromWorldBlock(block.Slab{Block: block.StoneBricks{Type: block.NormalStoneBricks()}})
	stoneBrickStairsNorth := blockStateFromWorldBlock(block.Stairs{Block: block.StoneBricks{Type: block.NormalStoneBricks()}, Facing: cube.North})
	stoneBrickStairsSouth := blockStateFromWorldBlock(block.Stairs{Block: block.StoneBricks{Type: block.NormalStoneBricks()}, Facing: cube.South})
	stoneBrickStairsEast := blockStateFromWorldBlock(block.Stairs{Block: block.StoneBricks{Type: block.NormalStoneBricks()}, Facing: cube.East})
	stoneBrickStairsWest := blockStateFromWorldBlock(block.Stairs{Block: block.StoneBricks{Type: block.NormalStoneBricks()}, Facing: cube.West})
	torchFloor := blockStateFromWorldBlock(block.Torch{Facing: cube.FaceDown, Type: block.NormalFire()})
	torchEastWall := blockStateFromWorldBlock(block.Torch{Facing: cube.FaceEast, Type: block.NormalFire()})
	torchWestWall := blockStateFromWorldBlock(block.Torch{Facing: cube.FaceWest, Type: block.NormalFire()})
	ladderSouth := blockStateFromWorldBlock(block.Ladder{Facing: cube.South})
	lava := blockStateFromWorldBlock(block.Lava{Still: true, Depth: 8})
	endPortal := gen.BlockState{Name: "end_portal"}

	fillRoom := func(x0, y0, z0, x1, y1, z1 int) {
		builder.fillSelectedBox(x0, y0, z0, x1, y1, z1, func(_, _, _ int, edge bool) gen.BlockState {
			if !edge {
				return air
			}
			return strongholdRandomStoneState(rng, stoneBricks, mossyStoneBricks, crackedStoneBricks)
		})
	}

	// Main entrance, hub, wings, and portal-room chain.
	fillRoom(14, 0, 0, 22, 8, 8)
	fillRoom(12, 0, 8, 24, 7, 18)
	fillRoom(0, 0, 11, 10, 9, 27)
	fillRoom(26, 0, 11, 36, 7, 21)
	fillRoom(14, 0, 19, 22, 6, 28)
	fillRoom(12, 0, 29, 24, 8, 44)

	// Connecting corridors and openings.
	builder.fillAirBox(17, 1, 8, 19, 3, 8)
	builder.fillAirBox(10, 1, 13, 12, 3, 15)
	builder.fillAirBox(24, 1, 13, 26, 3, 15)
	builder.fillAirBox(17, 1, 18, 19, 3, 19)
	builder.fillAirBox(17, 1, 28, 19, 3, 29)

	// Entrance descent.
	for i := 0; i < 5; i++ {
		y := 6 - i
		z := 1 + i
		builder.setBlock(17, y, z, stoneBrickStairsSouth)
		builder.setBlock(18, y, z, stoneBrickStairsSouth)
		builder.setBlock(19, y, z, stoneBrickStairsSouth)
		if i < 4 {
			builder.fillSolidBox(17, y-1, z, 19, y-1, z, stoneBricks)
		}
	}

	// Hub detailing.
	builder.fillSolidBox(16, 0, 10, 20, 0, 16, stoneBricks)
	builder.setBlock(15, 2, 12, torchEastWall)
	builder.setBlock(21, 2, 12, torchWestWall)
	builder.setBlock(15, 2, 16, torchEastWall)
	builder.setBlock(21, 2, 16, torchWestWall)
	builder.setBlock(18, 1, 13, stoneBrickSlab)
	builder.setBlock(18, 1, 15, stoneBrickSlab)

	// Library wing.
	builder.fillSolidBox(1, 0, 12, 9, 0, 26, planks)
	for z := 12; z <= 26; z++ {
		if z == 14 || z == 18 || z == 22 {
			builder.fillSolidBox(1, 1, z, 1, 5, z, planks)
			builder.fillSolidBox(9, 1, z, 9, 5, z, planks)
			continue
		}
		builder.fillSolidBox(1, 1, z, 1, 5, z, bookshelf)
		builder.fillSolidBox(9, 1, z, 9, 5, z, bookshelf)
	}
	builder.fillSolidBox(3, 1, 14, 7, 3, 14, bookshelf)
	builder.fillSolidBox(3, 1, 18, 7, 3, 18, bookshelf)
	builder.fillSolidBox(3, 1, 22, 7, 3, 22, bookshelf)
	builder.fillSolidBox(2, 6, 13, 8, 6, 25, planks)
	for z := 14; z <= 24; z++ {
		builder.setBlock(2, 7, z, oakFence)
		builder.setBlock(8, 7, z, oakFence)
	}
	for x := 3; x <= 7; x++ {
		builder.setBlock(x, 7, 13, oakFence)
		builder.setBlock(x, 7, 25, oakFence)
	}
	for y := 1; y <= 6; y++ {
		builder.setBlock(8, y, 24, ladderSouth)
	}
	builder.setBlock(5, 1, 20, torchFloor)
	builder.setBlock(5, 7, 20, torchFloor)

	// Prison wing.
	builder.fillSolidBox(27, 0, 12, 35, 0, 20, strongholdRandomStoneState(rng, stoneBricks, mossyStoneBricks, crackedStoneBricks))
	builder.fillSolidBox(30, 1, 13, 30, 4, 19, strongholdRandomStoneState(rng, stoneBricks, mossyStoneBricks, crackedStoneBricks))
	for y := 1; y <= 4; y++ {
		for z := 14; z <= 18; z++ {
			builder.setBlock(29, y, z, ironBars)
			builder.setBlock(31, y, z, ironBars)
		}
	}
	builder.fillAirBox(28, 1, 14, 28, 3, 16)
	builder.fillAirBox(32, 1, 17, 32, 3, 19)
	builder.setBlock(33, 2, 14, torchFloor)
	builder.setBlock(33, 2, 18, torchFloor)

	// Central corridor to the portal room.
	builder.fillSolidBox(16, 0, 20, 20, 0, 27, stoneBricks)
	builder.setBlock(17, 1, 22, stoneBrickSlab)
	builder.setBlock(19, 1, 22, stoneBrickSlab)
	builder.setBlock(17, 1, 25, stoneBrickSlab)
	builder.setBlock(19, 1, 25, stoneBrickSlab)
	builder.setBlock(18, 2, 24, torchFloor)

	// Portal-room floor, stair dais, and lava trench.
	builder.fillSolidBox(13, 0, 30, 23, 0, 43, stoneBricks)
	builder.fillSolidBox(15, 1, 33, 21, 1, 40, stoneBricks)
	builder.fillSolidBox(16, 1, 34, 20, 1, 39, lava)
	for x := 16; x <= 20; x++ {
		builder.setBlock(x, 1, 31, stoneBrickStairsSouth)
		builder.setBlock(x, 2, 32, stoneBrickStairsSouth)
		builder.setBlock(x, 3, 33, stoneBrickStairsSouth)
	}
	for z := 33; z <= 42; z += 2 {
		builder.setBlock(13, 3, z, ironBars)
		builder.setBlock(23, 3, z, ironBars)
	}
	for x := 15; x <= 21; x += 2 {
		builder.setBlock(x, 3, 44, ironBars)
	}

	eyes := make([]bool, 12)
	allEyes := true
	for i := range eyes {
		eyes[i] = rng.NextDouble() > 0.9
		allEyes = allEyes && eyes[i]
	}
	builder.setBlock(17, 3, 36, endPortalFrameState(cube.South, eyes[0]))
	builder.setBlock(18, 3, 36, endPortalFrameState(cube.South, eyes[1]))
	builder.setBlock(19, 3, 36, endPortalFrameState(cube.South, eyes[2]))
	builder.setBlock(17, 3, 40, endPortalFrameState(cube.North, eyes[3]))
	builder.setBlock(18, 3, 40, endPortalFrameState(cube.North, eyes[4]))
	builder.setBlock(19, 3, 40, endPortalFrameState(cube.North, eyes[5]))
	builder.setBlock(16, 3, 37, endPortalFrameState(cube.East, eyes[6]))
	builder.setBlock(16, 3, 38, endPortalFrameState(cube.East, eyes[7]))
	builder.setBlock(16, 3, 39, endPortalFrameState(cube.East, eyes[8]))
	builder.setBlock(20, 3, 37, endPortalFrameState(cube.West, eyes[9]))
	builder.setBlock(20, 3, 38, endPortalFrameState(cube.West, eyes[10]))
	builder.setBlock(20, 3, 39, endPortalFrameState(cube.West, eyes[11]))
	if allEyes {
		for x := 17; x <= 19; x++ {
			for z := 37; z <= 39; z++ {
				builder.setBlock(x, 3, z, endPortal)
			}
		}
	}
	builder.setBlock(18, 3, 34, torchFloor)
	builder.setBlock(15, 5, 35, torchFloor)
	builder.setBlock(21, 5, 35, torchFloor)

	// Extra detailing to avoid a completely boxy footprint.
	builder.fillSolidBox(12, 6, 8, 24, 6, 8, cobblestone)
	builder.fillSolidBox(12, 6, 18, 24, 6, 18, cobblestone)
	builder.fillSolidBox(14, 6, 29, 22, 6, 29, cobblestone)
	builder.setBlock(14, 2, 31, stoneBrickStairsEast)
	builder.setBlock(22, 2, 31, stoneBrickStairsWest)
	builder.setBlock(14, 2, 41, stoneBrickStairsEast)
	builder.setBlock(22, 2, 41, stoneBrickStairsWest)
	builder.setBlock(17, 1, 34, stoneBrickStairsNorth)
	builder.setBlock(19, 1, 34, stoneBrickStairsNorth)

	piece := builder.piece()
	rootOrigin, rootSize := builder.bounds.originAndSize()
	return "stronghold", []plannedStructurePiece{piece}, builder.bounds, rootOrigin, rootSize, true
}

func endPortalFrameState(facing cube.Direction, eye bool) gen.BlockState {
	return gen.BlockState{
		Name: "end_portal_frame",
		Properties: map[string]string{
			"minecraft:cardinal_direction": facing.String(),
			"end_portal_eye_bit":           boolString(eye),
		},
	}
}

func strongholdRandomStoneState(rng *gen.Xoroshiro128, normal, mossy, cracked gen.BlockState) gen.BlockState {
	switch roll := int(rng.NextInt(100)); {
	case roll < 55:
		return normal
	case roll < 82:
		return mossy
	case roll < 95:
		return cracked
	default:
		return normal
	}
}
