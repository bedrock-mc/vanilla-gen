package vanilla

import (
	gen "github.com/bedrock-mc/vanilla-gen/gen"
	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
)

func (g Generator) oceanMonumentSurroundingsAllowAt(centerX, centerZ, sampleY int) bool {
	if sampleY < seaLevel {
		sampleY = seaLevel
	}
	for dz := -29; dz <= 29; dz += 4 {
		for dx := -29; dx <= 29; dx += 4 {
			if dx*dx+dz*dz > 29*29 {
				continue
			}
			biome := g.biomeSource.GetBiome(centerX+dx, sampleY, centerZ+dz)
			if biomeMatchesWorldgenTag("is_ocean", biome) || isRiverBiome(biome) {
				continue
			}
			return false
		}
	}
	return true
}

func (g Generator) buildOceanMonumentStructure(candidate structurePlannerCandidate, startChunk world.ChunkPos, startX, startZ int, surfaceSampler *structureHeightSampler, rng *gen.WorldgenRandom) (string, []plannedStructurePiece, structureBox, cube.Pos, [3]int, bool) {
	_ = candidate
	_ = startChunk

	size := [3]int{58, 23, 58}
	rotation := randomStructureRotation(rng)
	originY := clamp(seaLevel-13, surfaceSampler.minY+2, surfaceSampler.maxY-size[1])
	builder := newProceduralStructureBuilder(cube.Pos{startX - 29, originY, startZ - 29}, cube.Pos{}, size, rotation)

	water := blockStateFromWorldBlock(block.Water{Still: true, Depth: 8})
	prismarine := blockStateFromWorldBlock(block.Prismarine{Type: block.NormalPrismarine()})
	prismarineBricks := blockStateFromWorldBlock(block.Prismarine{Type: block.BrickPrismarine()})
	darkPrismarine := blockStateFromWorldBlock(block.Prismarine{Type: block.DarkPrismarine()})
	seaLantern := blockStateFromWorldBlock(block.SeaLantern{})

	builder.fillSelectedBox(0, 0, 0, 57, 14, 57, func(localX, localY, localZ int, edge bool) gen.BlockState {
		if !edge {
			return water
		}
		if localY == 0 || localY == 14 {
			return prismarineBricks
		}
		if localX == 0 || localX == 57 || localZ == 0 || localZ == 57 {
			if (localX+localZ+localY)%7 == 0 {
				return darkPrismarine
			}
			return prismarine
		}
		return prismarineBricks
	})

	builder.fillSelectedBox(10, 2, 10, 47, 17, 47, func(localX, localY, localZ int, edge bool) gen.BlockState {
		if !edge {
			return water
		}
		if localY == 2 || localY == 17 {
			return prismarineBricks
		}
		if (localX == 10 || localX == 47 || localZ == 10 || localZ == 47) && (localX+localZ)%5 == 0 {
			return darkPrismarine
		}
		return prismarine
	})

	builder.fillSelectedBox(18, 4, 18, 39, 21, 39, func(localX, localY, localZ int, edge bool) gen.BlockState {
		if !edge {
			return water
		}
		if localY == 4 || localY == 21 {
			return prismarineBricks
		}
		if localX == 18 || localX == 39 || localZ == 18 || localZ == 39 {
			return darkPrismarine
		}
		return prismarine
	})

	for _, wing := range []struct {
		x0, y0, z0, x1, y1, z1 int
	}{
		{22, 3, 0, 35, 10, 12},
		{22, 3, 45, 35, 10, 57},
		{0, 3, 22, 12, 10, 35},
		{45, 3, 22, 57, 10, 35},
	} {
		builder.fillSelectedBox(wing.x0, wing.y0, wing.z0, wing.x1, wing.y1, wing.z1, func(localX, localY, localZ int, edge bool) gen.BlockState {
			if !edge {
				return water
			}
			if localY == wing.y0 || localY == wing.y1 {
				return prismarineBricks
			}
			if (localX+localZ)%3 == 0 {
				return darkPrismarine
			}
			return prismarine
		})
	}

	builder.fillSolidBox(20, 5, 20, 37, 5, 37, prismarineBricks)
	builder.fillSolidBox(23, 6, 23, 34, 6, 34, prismarine)

	for _, pos := range [][3]int{
		{6, 4, 6}, {6, 4, 51}, {51, 4, 6}, {51, 4, 51},
		{28, 8, 28}, {20, 8, 28}, {36, 8, 28}, {28, 8, 20}, {28, 8, 36},
		{28, 14, 28}, {14, 10, 28}, {42, 10, 28}, {28, 10, 14}, {28, 10, 42},
	} {
		builder.setBlock(pos[0], pos[1], pos[2], seaLantern)
	}

	for _, support := range [][2]int{
		{6, 6}, {6, 51}, {51, 6}, {51, 51},
		{14, 14}, {14, 43}, {43, 14}, {43, 43},
		{28, 6}, {28, 51}, {6, 28}, {51, 28},
	} {
		builder.fillFoundationColumn(g, prismarineBricks, support[0], 0, support[1], surfaceSampler.minY)
	}

	piece := builder.piece()
	rootOrigin, rootSize := piece.bounds.originAndSize()
	return "monument", []plannedStructurePiece{piece}, piece.bounds, rootOrigin, rootSize, true
}

func (g Generator) buildWoodlandMansionStructure(candidate structurePlannerCandidate, startChunk world.ChunkPos, startX, startZ int, surfaceSampler *structureHeightSampler, rng *gen.WorldgenRandom) (string, []plannedStructurePiece, structureBox, cube.Pos, [3]int, bool) {
	_ = candidate
	_ = startChunk

	size := [3]int{45, 24, 45}
	rotation := randomStructureRotation(rng)
	originX := startX - 14
	originZ := startZ - 14

	minSurface := surfaceSampler.maxY
	maxSurface := surfaceSampler.minY
	for x := originX; x < originX+size[0]; x += 4 {
		for z := originZ; z < originZ+size[2]; z += 4 {
			y := surfaceSampler.worldSurfaceLevelAt(x, z) - 1
			if y < minSurface {
				minSurface = y
			}
			if y > maxSurface {
				maxSurface = y
			}
		}
	}
	if minSurface < 60 || maxSurface-minSurface > 20 {
		return "", nil, emptyStructureBox(), cube.Pos{}, [3]int{}, false
	}

	builder := newProceduralStructureBuilder(cube.Pos{originX, minSurface, originZ}, cube.Pos{}, size, rotation)

	air := structureState("air")
	cobblestone := blockStateFromWorldBlock(block.Cobblestone{})
	darkOakPlanks := blockStateFromWorldBlock(block.Planks{Wood: block.DarkOakWood()})
	darkOakLogY := blockStateFromWorldBlock(block.Log{Wood: block.DarkOakWood(), Axis: cube.Y})
	darkOakLogX := blockStateFromWorldBlock(block.Log{Wood: block.DarkOakWood(), Axis: cube.X})
	darkOakLogZ := blockStateFromWorldBlock(block.Log{Wood: block.DarkOakWood(), Axis: cube.Z})
	darkOakStairsNorth := blockStateFromWorldBlock(block.Stairs{Block: block.Planks{Wood: block.DarkOakWood()}, Facing: cube.North})
	darkOakStairsSouth := blockStateFromWorldBlock(block.Stairs{Block: block.Planks{Wood: block.DarkOakWood()}, Facing: cube.South})
	darkOakStairsEast := blockStateFromWorldBlock(block.Stairs{Block: block.Planks{Wood: block.DarkOakWood()}, Facing: cube.East})
	darkOakStairsWest := blockStateFromWorldBlock(block.Stairs{Block: block.Planks{Wood: block.DarkOakWood()}, Facing: cube.West})

	builder.fillSolidBox(0, 0, 0, 44, 2, 44, cobblestone)
	builder.fillSelectedBox(0, 3, 0, 44, 9, 44, func(localX, localY, localZ int, edge bool) gen.BlockState {
		if !edge {
			return air
		}
		if localX == 0 || localX == 44 || localZ == 0 || localZ == 44 {
			if localX%4 == 0 || localZ%4 == 0 {
				return darkOakLogY
			}
		}
		return darkOakPlanks
	})
	builder.fillSelectedBox(4, 10, 4, 40, 16, 40, func(localX, localY, localZ int, edge bool) gen.BlockState {
		if !edge {
			return air
		}
		if localX == 4 || localX == 40 || localZ == 4 || localZ == 40 {
			if localX%4 == 0 || localZ%4 == 0 {
				return darkOakLogY
			}
		}
		return darkOakPlanks
	})

	for _, wing := range []struct {
		x0, y0, z0, x1, y1, z1 int
	}{
		{16, 3, 0, 28, 9, 10},
		{16, 3, 34, 28, 9, 44},
		{0, 3, 16, 10, 9, 28},
		{34, 3, 16, 44, 9, 28},
	} {
		builder.fillSelectedBox(wing.x0, wing.y0, wing.z0, wing.x1, wing.y1, wing.z1, func(localX, localY, localZ int, edge bool) gen.BlockState {
			if !edge {
				return air
			}
			if (localX == wing.x0 || localX == wing.x1 || localZ == wing.z0 || localZ == wing.z1) && (localX+localZ)%4 == 0 {
				return darkOakLogY
			}
			return darkOakPlanks
		})
	}

	builder.fillSolidBox(2, 3, 2, 42, 3, 42, darkOakPlanks)
	builder.fillSolidBox(6, 10, 6, 38, 10, 38, darkOakPlanks)
	builder.fillAirBox(10, 4, 10, 34, 8, 34)
	builder.fillAirBox(12, 11, 12, 32, 15, 32)

	builder.fillSolidBox(12, 4, 12, 32, 4, 14, darkOakPlanks)
	builder.fillSolidBox(12, 4, 30, 32, 4, 32, darkOakPlanks)
	builder.fillSolidBox(12, 4, 15, 14, 4, 29, darkOakPlanks)
	builder.fillSolidBox(30, 4, 15, 32, 4, 29, darkOakPlanks)
	builder.fillSolidBox(20, 4, 20, 24, 8, 24, darkOakLogY)

	for x := 0; x <= 44; x++ {
		builder.setBlock(x, 17, 0, darkOakStairsNorth)
		builder.setBlock(x, 17, 44, darkOakStairsSouth)
	}
	for z := 1; z <= 43; z++ {
		builder.setBlock(0, 17, z, darkOakStairsWest)
		builder.setBlock(44, 17, z, darkOakStairsEast)
	}
	builder.fillSolidBox(1, 18, 1, 43, 18, 43, darkOakPlanks)

	for x := 4; x <= 40; x += 4 {
		builder.fillSolidBox(x, 17, 4, x, 21, 4, darkOakLogY)
		builder.fillSolidBox(x, 17, 40, x, 21, 40, darkOakLogY)
	}
	for z := 8; z <= 36; z += 4 {
		builder.fillSolidBox(4, 17, z, 4, 21, z, darkOakLogY)
		builder.fillSolidBox(40, 17, z, 40, 21, z, darkOakLogY)
	}
	builder.fillSolidBox(8, 21, 8, 36, 21, 8, darkOakLogX)
	builder.fillSolidBox(8, 21, 36, 36, 21, 36, darkOakLogX)
	builder.fillSolidBox(8, 21, 9, 8, 21, 35, darkOakLogZ)
	builder.fillSolidBox(36, 21, 9, 36, 21, 35, darkOakLogZ)

	for _, support := range [][2]int{
		{0, 0}, {0, 44}, {44, 0}, {44, 44},
		{8, 0}, {16, 0}, {24, 0}, {32, 0}, {40, 0},
		{8, 44}, {16, 44}, {24, 44}, {32, 44}, {40, 44},
		{0, 8}, {0, 16}, {0, 24}, {0, 32}, {0, 40},
		{44, 8}, {44, 16}, {44, 24}, {44, 32}, {44, 40},
	} {
		builder.fillFoundationColumn(g, cobblestone, support[0], 1, support[1], surfaceSampler.minY)
	}

	piece := builder.piece()
	rootOrigin, rootSize := piece.bounds.originAndSize()
	return "mansion", []plannedStructurePiece{piece}, piece.bounds, rootOrigin, rootSize, true
}
