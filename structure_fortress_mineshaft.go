package vanilla

import (
	gen "github.com/bedrock-mc/vanilla-gen/gen"
	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
)

func (g Generator) buildFortressStructure(candidate structurePlannerCandidate, startChunk world.ChunkPos, startX, startZ int, surfaceSampler *structureHeightSampler, rng *gen.WorldgenRandom) (string, []plannedStructurePiece, structureBox, cube.Pos, [3]int, bool) {
	_ = startChunk

	size := [3]int{43, 17, 43}
	rotation := randomStructureRotation(rng)
	originY := clamp(52+int(rng.NextInt(13)), surfaceSampler.minY+8, surfaceSampler.maxY-size[1])
	builder := newProceduralStructureBuilder(cube.Pos{startX + 2, originY, startZ + 2}, cube.Pos{}, size, rotation)

	air := structureState("air")
	netherBricks := blockStateFromWorldBlock(block.NetherBricks{Type: block.NormalNetherBricks()})
	netherBrickFence := blockStateFromWorldBlock(block.NetherBrickFence{})
	netherBrickStairsNorth := blockStateFromWorldBlock(block.Stairs{Block: block.NetherBricks{Type: block.NormalNetherBricks()}, Facing: cube.North})
	netherBrickStairsSouth := blockStateFromWorldBlock(block.Stairs{Block: block.NetherBricks{Type: block.NormalNetherBricks()}, Facing: cube.South})
	netherBrickStairsEast := blockStateFromWorldBlock(block.Stairs{Block: block.NetherBricks{Type: block.NormalNetherBricks()}, Facing: cube.East})
	netherBrickStairsWest := blockStateFromWorldBlock(block.Stairs{Block: block.NetherBricks{Type: block.NormalNetherBricks()}, Facing: cube.West})
	soulSand := blockStateFromWorldBlock(block.SoulSand{})
	netherWart := blockStateFromWorldBlock(block.NetherWart{Age: 3})

	baseY := 5
	builder.fillSelectedBox(14, baseY, 14, 28, baseY+6, 28, func(_, _, _ int, edge bool) gen.BlockState {
		if edge {
			return netherBricks
		}
		return air
	})
	builder.fillAirBox(19, baseY+1, 14, 23, baseY+3, 14)
	builder.fillAirBox(19, baseY+1, 28, 23, baseY+3, 28)
	builder.fillAirBox(14, baseY+1, 19, 14, baseY+3, 23)
	builder.fillAirBox(28, baseY+1, 19, 28, baseY+3, 23)

	builder.fillSolidBox(18, baseY, 18, 24, baseY, 24, netherBricks)
	builder.fillSolidBox(19, baseY+1, 19, 23, baseY+1, 23, soulSand)
	builder.fillSolidBox(19, baseY+2, 19, 23, baseY+2, 23, netherWart)

	for _, z := range []int{16, 26} {
		for x := 16; x <= 26; x++ {
			builder.setBlock(x, baseY+4, z, netherBrickFence)
		}
	}
	for _, x := range []int{16, 26} {
		for z := 17; z <= 25; z++ {
			builder.setBlock(x, baseY+4, z, netherBrickFence)
		}
	}

	builder.fillSolidBox(19, baseY, 0, 23, baseY, 42, netherBricks)
	builder.fillAirBox(19, baseY+1, 0, 23, baseY+3, 42)
	builder.fillSolidBox(0, baseY, 19, 42, baseY, 23, netherBricks)
	builder.fillAirBox(0, baseY+1, 19, 42, baseY+3, 23)

	for z := 0; z <= 42; z += 6 {
		for _, x := range []int{19, 23} {
			builder.fillSolidBox(x, baseY+1, z, x, baseY+3, z, netherBrickFence)
		}
		builder.fillSolidBox(19, baseY+4, z, 23, baseY+4, z, netherBricks)
	}
	for x := 0; x <= 42; x += 6 {
		for _, z := range []int{19, 23} {
			builder.fillSolidBox(x, baseY+1, z, x, baseY+3, z, netherBrickFence)
		}
		builder.fillSolidBox(x, baseY+4, 19, x, baseY+4, 23, netherBricks)
	}

	for z := 0; z <= 42; z++ {
		builder.setBlock(18, baseY+1, z, netherBrickFence)
		builder.setBlock(24, baseY+1, z, netherBrickFence)
		if z != 0 && z != 42 {
			builder.setBlock(19, baseY+1, z, air)
			builder.setBlock(23, baseY+1, z, air)
		}
	}
	for x := 0; x <= 42; x++ {
		builder.setBlock(x, baseY+1, 18, netherBrickFence)
		builder.setBlock(x, baseY+1, 24, netherBrickFence)
		if x != 0 && x != 42 {
			builder.setBlock(x, baseY+1, 19, air)
			builder.setBlock(x, baseY+1, 23, air)
		}
	}

	builder.fillSelectedBox(17, baseY, 0, 25, baseY+5, 6, func(_, _, _ int, edge bool) gen.BlockState {
		if edge {
			return netherBricks
		}
		return air
	})
	builder.fillSelectedBox(17, baseY, 36, 25, baseY+5, 42, func(_, _, _ int, edge bool) gen.BlockState {
		if edge {
			return netherBricks
		}
		return air
	})
	builder.fillSelectedBox(0, baseY, 17, 6, baseY+5, 25, func(_, _, _ int, edge bool) gen.BlockState {
		if edge {
			return netherBricks
		}
		return air
	})
	builder.fillSelectedBox(36, baseY, 17, 42, baseY+5, 25, func(_, _, _ int, edge bool) gen.BlockState {
		if edge {
			return netherBricks
		}
		return air
	})

	for x := 18; x <= 24; x++ {
		builder.setBlock(x, baseY+1, 6, netherBrickStairsSouth)
		builder.setBlock(x, baseY+1, 36, netherBrickStairsNorth)
	}
	for z := 18; z <= 24; z++ {
		builder.setBlock(6, baseY+1, z, netherBrickStairsEast)
		builder.setBlock(36, baseY+1, z, netherBrickStairsWest)
	}

	for _, support := range [][2]int{
		{19, 0}, {23, 0}, {19, 42}, {23, 42},
		{0, 19}, {0, 23}, {42, 19}, {42, 23},
		{16, 16}, {16, 26}, {26, 16}, {26, 26},
	} {
		builder.fillFoundationColumn(g, netherBricks, support[0], baseY-1, support[1], surfaceSampler.minY)
	}

	piece := builder.piece()
	rootOrigin, rootSize := piece.bounds.originAndSize()
	return candidate.structureName, []plannedStructurePiece{piece}, piece.bounds, rootOrigin, rootSize, true
}

