package vanilla

import (
	"math"
	"strconv"

	gen "github.com/bedrock-mc/vanilla-gen/gen"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/df-mc/dragonfly/server/world/chunk"
)

const noWaterHeight = -1 << 31

// applySurfaceAndBiomes ports SurfaceSystem.buildSurface: a single top-down
// pass per column with vanilla stone depth accounting (1-based, ceiling scan
// for stoneDepthBelow), fluid-aware water heights, per-block zoomed biomes and
// the eroded badlands / frozen ocean extensions.
func (g Generator) applySurfaceAndBiomes(c *chunk.Chunk, biomes sourceBiomeVolume, chunkX, chunkZ, minY, maxY int) {
	if g.surface == nil {
		return
	}

	var heights [16][16]int
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			heights[x][z] = g.topNonAirY(c, x, z, minY, maxY)
		}
	}
	corners := g.preliminarySurfaceCorners(chunkX, chunkZ)

	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			blockX := chunkX*16 + x
			blockZ := chunkZ*16 + z
			startingHeight := heights[x][z] + 1
			surfaceBiome := g.zoomedBiomeAt(blockX, startingHeight, blockZ)
			if surfaceBiome == gen.BiomeErodedBadlands {
				g.erodedBadlandsExtension(c, x, z, blockX, blockZ, startingHeight, minY, maxY, &heights)
			}

			height := heights[x][z] + 1
			surfaceDepth := g.surface.SurfaceDepth(blockX, blockZ)
			surfaceSecondary := g.surface.SurfaceSecondary(blockX, blockZ)
			steep := steepColumn(&heights, x, z)
			minSurfaceLevel := g.minSurfaceLevel(corners, blockX, blockZ, surfaceDepth)

			stoneAboveDepth := 0
			waterHeight := noWaterHeight
			nextCeilingStoneY := int(^uint(0) >> 1)

			for y := height; y >= minY; y-- {
				rid := c.Block(uint8(x), int16(y), uint8(z), 0)
				if rid == g.airRID {
					stoneAboveDepth = 0
					waterHeight = noWaterHeight
					continue
				}
				if g.isSurfaceFluidRID(rid) {
					if waterHeight == noWaterHeight {
						waterHeight = y + 1
					}
					continue
				}

				if nextCeilingStoneY >= y {
					nextCeilingStoneY = minY - 1
					for lookaheadY := y - 1; lookaheadY >= minY-1; lookaheadY-- {
						if lookaheadY < minY {
							nextCeilingStoneY = lookaheadY + 1
							break
						}
						lookRID := c.Block(uint8(x), int16(lookaheadY), uint8(z), 0)
						if lookRID == g.airRID || g.isSurfaceFluidRID(lookRID) {
							nextCeilingStoneY = lookaheadY + 1
							break
						}
					}
				}

				stoneAboveDepth++
				stoneBelowDepth := y - nextCeilingStoneY + 1
				if rid != g.defaultBlockRID {
					continue
				}

				ctx := gen.SurfaceContext{
					BlockX:           blockX,
					BlockY:           y,
					BlockZ:           blockZ,
					SurfaceDepth:     surfaceDepth,
					SurfaceSecondary: surfaceSecondary,
					WaterHeight:      waterHeight,
					StoneDepthAbove:  stoneAboveDepth,
					StoneDepthBelow:  stoneBelowDepth,
					Steep:            steep,
					Biome:            g.zoomedBiomeAt(blockX, y, blockZ),
					MinSurfaceLevel:  minSurfaceLevel,
					MinY:             minY,
					MaxY:             maxY,
				}
				if replacement, ok := g.surface.TryApply(ctx, g.lookupSurfaceBlock); ok && replacement != rid {
					c.SetBlock(uint8(x), int16(y), uint8(z), 0, replacement)
				}
			}

			if surfaceBiome == gen.BiomeFrozenOcean || surfaceBiome == gen.BiomeDeepFrozenOcean {
				g.frozenOceanExtension(c, x, z, blockX, blockZ, minSurfaceLevel, surfaceBiome, heights[x][z]+1, minY, maxY, &heights)
			}
		}
	}
}

func (g Generator) topNonAirY(c *chunk.Chunk, x, z, minY, maxY int) int {
	for y := maxY; y >= minY; y-- {
		if c.Block(uint8(x), int16(y), uint8(z), 0) != g.airRID {
			return y
		}
	}
	return minY - 1
}

func (g Generator) isSurfaceFluidRID(rid uint32) bool {
	return rid == g.waterRID || rid == g.lavaRID || (g.defaultFluidRID != g.airRID && rid == g.defaultFluidRID)
}

// steepColumn mirrors SurfaceRules.SteepMaterialCondition.
func steepColumn(heights *[16][16]int, x, z int) bool {
	zNorth := max(z-1, 0)
	zSouth := min(z+1, 15)
	if heights[x][zSouth] >= heights[x][zNorth]+4 {
		return true
	}
	xWest := max(x-1, 0)
	xEast := min(x+1, 15)
	return heights[xWest][z] >= heights[xEast][z]+4
}

// preliminarySurfaceCorners evaluates the preliminary_surface_level density
// function at the four 16-aligned corners like NoiseChunk.preliminarySurfaceLevel.
func (g Generator) preliminarySurfaceCorners(chunkX, chunkZ int) [4]int {
	var corners [4]int
	root := g.rootIndex("preliminary_surface_level")
	if root < 0 {
		for i := range corners {
			corners[i] = g.metadata.MinY + g.metadata.Height
		}
		return corners
	}
	offsets := [4][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}}
	for i, off := range offsets {
		blockX := (chunkX + off[0]) * 16
		blockZ := (chunkZ + off[1]) * 16
		ctx := gen.FunctionContext{BlockX: blockX, BlockY: 0, BlockZ: blockZ}
		value := g.graph.Eval(root, ctx, g.noises, nil, nil, nil)
		corners[i] = int(math.Floor(value))
	}
	return corners
}

// minSurfaceLevel mirrors SurfaceRules.Context.getMinSurfaceLevel.
func (g Generator) minSurfaceLevel(corners [4]int, blockX, blockZ, surfaceDepth int) int {
	fx := float64(float32(blockX&15) / 16.0)
	fz := float64(float32(blockZ&15) / 16.0)
	x0 := float64(corners[0]) + fx*(float64(corners[1])-float64(corners[0]))
	x1 := float64(corners[2]) + fx*(float64(corners[3])-float64(corners[2]))
	level := x0 + fz*(x1-x0)
	return int(math.Floor(level)) + surfaceDepth - 8
}

// erodedBadlandsExtension mirrors SurfaceSystem.erodedBadlandsExtension.
func (g Generator) erodedBadlandsExtension(c *chunk.Chunk, x, z, blockX, blockZ, height, minY, maxY int, heights *[16][16]int) {
	pillarBuffer := math.Min(
		math.Abs(g.noises.Sample(gen.NoiseBadlandsSurface, float64(blockX), 0.0, float64(blockZ))*8.25),
		g.noises.Sample(gen.NoiseBadlandsPillar, float64(blockX)*0.2, 0.0, float64(blockZ)*0.2)*15.0,
	)
	if pillarBuffer <= 0.0 {
		return
	}
	pillarFloor := math.Abs(g.noises.Sample(gen.NoiseBadlandsPillarRoof, float64(blockX)*0.75, 0.0, float64(blockZ)*0.75) * 1.5)
	extensionTop := 64.0 + math.Min(pillarBuffer*pillarBuffer*2.5, math.Ceil(pillarFloor*50.0)+24.0)
	startY := int(math.Floor(extensionTop))
	if height > startY {
		return
	}
	for y := startY; y >= minY; y-- {
		rid := c.Block(uint8(x), int16(y), uint8(z), 0)
		if rid == g.defaultBlockRID {
			break
		}
		if rid == g.waterRID {
			return
		}
	}
	for y := startY; y >= minY && c.Block(uint8(x), int16(y), uint8(z), 0) == g.airRID; y-- {
		c.SetBlock(uint8(x), int16(y), uint8(z), 0, g.defaultBlockRID)
		if y > heights[x][z] {
			heights[x][z] = y
		}
	}
}

// frozenOceanExtension mirrors SurfaceSystem.frozenOceanExtension.
func (g Generator) frozenOceanExtension(c *chunk.Chunk, x, z, blockX, blockZ, minSurfaceLevel int, biome gen.Biome, height, minY, maxY int, heights *[16][16]int) {
	iceberg := math.Min(
		math.Abs(g.noises.Sample(gen.NoiseIcebergSurface, float64(blockX), 0.0, float64(blockZ))*8.25),
		g.noises.Sample(gen.NoiseIcebergPillar, float64(blockX)*1.28, 0.0, float64(blockZ)*1.28)*15.0,
	)
	if iceberg <= 1.8 {
		return
	}
	icebergRoof := math.Abs(g.noises.Sample(gen.NoiseIcebergPillarRoof, float64(blockX)*1.17, 0.0, float64(blockZ)*1.17) * 1.5)
	top := math.Min(iceberg*iceberg*1.2, math.Ceil(icebergRoof*40.0)+14.0)
	if gen.BiomeShouldMeltIceberg(biome, blockX, g.metadata.SeaLevel, blockZ, g.metadata.SeaLevel) {
		top -= 2.0
	}
	var extensionBottom float64
	seaLevel := g.metadata.SeaLevel
	if top > 2.0 {
		extensionBottom = float64(seaLevel) - top - 7.0
		top += float64(seaLevel)
	} else {
		top = 0.0
		extensionBottom = 0.0
	}
	random := g.surface.NoiseRandomAt(blockX, 0, blockZ)
	maxSnowDepth := 2 + int(random.NextInt(4))
	minSnowHeight := seaLevel + 18 + int(random.NextInt(10))
	snowDepth := 0
	snowRID := g.lookupSurfaceBlock("minecraft:snow_block", nil)
	packedIceRID := g.lookupSurfaceBlock("minecraft:packed_ice", nil)

	for y := max(height, int(top)+1); y >= minSurfaceLevel; y-- {
		if y < minY || y > maxY {
			continue
		}
		rid := c.Block(uint8(x), int16(y), uint8(z), 0)
		isAirIce := rid == g.airRID && y < int(top) && random.NextDouble() > 0.01
		isWaterIce := rid == g.waterRID && y > int(extensionBottom) && y < seaLevel && extensionBottom != 0.0 && random.NextDouble() > 0.15
		if isAirIce || isWaterIce {
			if snowDepth <= maxSnowDepth && y > minSnowHeight {
				c.SetBlock(uint8(x), int16(y), uint8(z), 0, snowRID)
				snowDepth++
			} else {
				c.SetBlock(uint8(x), int16(y), uint8(z), 0, packedIceRID)
			}
			if y > heights[x][z] {
				heights[x][z] = y
			}
		}
	}
}

func (g Generator) lookupSurfaceBlock(name string, properties map[string]string) uint32 {
	key := blockStateCacheKey(name, properties)
	if rid, ok := g.surfaceBlockCache.Lookup(key); ok {
		return rid
	}

	switch name {
	case "minecraft:air":
		g.surfaceBlockCache.Store(key, g.airRID)
		return g.airRID
	case "minecraft:water":
		g.surfaceBlockCache.Store(key, g.waterRID)
		return g.waterRID
	case "minecraft:lava":
		g.surfaceBlockCache.Store(key, g.lavaRID)
		return g.lavaRID
	case "minecraft:bedrock":
		g.surfaceBlockCache.Store(key, g.bedrockRID)
		return g.bedrockRID
	case "minecraft:deepslate":
		g.surfaceBlockCache.Store(key, g.deepRID)
		return g.deepRID
	}

	if rid, ok := g.lookupRegisteredSurfaceBlock(name, properties); ok {
		g.surfaceBlockCache.Store(key, rid)
		return rid
	}
	if len(properties) != 0 {
		if rid, ok := g.lookupRegisteredSurfaceBlock(name, nil); ok {
			g.surfaceBlockCache.Store(key, rid)
			return rid
		}
	}

	g.surfaceBlockCache.Store(key, g.airRID)
	return g.airRID
}

func (g Generator) lookupRegisteredSurfaceBlock(name string, properties map[string]string) (uint32, bool) {
	blockProps := make(map[string]any, len(properties))
	for key, value := range properties {
		switch value {
		case "true":
			blockProps[key] = true
		case "false":
			blockProps[key] = false
		default:
			if n, err := strconv.ParseInt(value, 10, 32); err == nil {
				blockProps[key] = int32(n)
			} else {
				blockProps[key] = value
			}
		}
	}
	if len(blockProps) == 0 {
		blockProps = nil
	}

	b, ok := world.BlockByName(name, blockProps)
	if !ok {
		return 0, false
	}
	return runtimeIDForBlock(b), true
}

func (g Generator) isSurfaceBaseRID(rid uint32) bool {
	if rid == g.defaultBlockRID {
		return true
	}
	return g.dimension == world.Overworld && rid == g.deepRID
}
