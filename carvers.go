package vanilla

import (
	"math"

	gen "github.com/bedrock-mc/vanilla-gen/gen"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/df-mc/dragonfly/server/world/chunk"
)

// Port of NoiseBasedChunkGenerator.applyCarvers and the vanilla
// CaveWorldCarver/CanyonWorldCarver. Each of the 17x17 surrounding chunks
// contributes the carvers of its own noise biome, seeded with
// setLargeFeatureSeed(seed + carverIndex, originX, originZ) on a
// legacy-backed WorldgenRandom.
const carverSearchRadius = 8 // getRange() * 2

type carvingMask struct {
	minY int
	bits []uint64
}

func newCarvingMask(minY, maxY int) *carvingMask {
	height := maxY - minY + 1
	return &carvingMask{minY: minY, bits: make([]uint64, (height*256+63)/64)}
}

func (m *carvingMask) index(localX, worldY, localZ int) int {
	return (worldY-m.minY)*256 + localZ*16 + localX
}

func (m *carvingMask) get(localX, worldY, localZ int) bool {
	i := m.index(localX, worldY, localZ)
	return m.bits[i>>6]&(1<<(i&63)) != 0
}

func (m *carvingMask) set(localX, worldY, localZ int) {
	i := m.index(localX, worldY, localZ)
	m.bits[i>>6] |= 1 << (i & 63)
}

type carveContext struct {
	c       *chunk.Chunk
	chunkX  int
	chunkZ  int
	minY    int
	maxY    int
	aquifer *gen.NoiseBasedAquifer
	mask    *carvingMask
}

func (g Generator) carveTerrain(c *chunk.Chunk, biomes sourceBiomeVolume, chunkX, chunkZ, minY, maxY int, aquifer *gen.NoiseBasedAquifer) {
	if g.carvers == nil {
		return
	}

	ctx := &carveContext{
		c:       c,
		chunkX:  chunkX,
		chunkZ:  chunkZ,
		minY:    minY,
		maxY:    maxY,
		aquifer: aquifer,
		mask:    newCarvingMask(minY, maxY),
	}
	rng := gen.NewWorldgenRandomLegacy(0)

	for dx := -carverSearchRadius; dx <= carverSearchRadius; dx++ {
		for dz := -carverSearchRadius; dz <= carverSearchRadius; dz++ {
			originX := chunkX + dx
			originZ := chunkZ + dz
			// Vanilla resolves the carver list from the noise biome at the
			// origin chunk's minimum block corner at y=0.
			biome := g.biomeSource.GetBiome(originX*16, 0, originZ*16)
			for index, carverName := range g.biomeGeneration.carverNames[biome] {
				configured, err := g.carvers.Configured(carverName)
				if err != nil {
					continue
				}
				rng.SetLargeFeatureSeed(g.seed+int64(index), originX, originZ)
				switch configured.Type {
				case "cave", "nether_cave":
					cfg, err := configured.Cave()
					if err != nil {
						continue
					}
					if rng.NextFloat() <= float32(cfg.Probability) {
						g.carveCave(ctx, cfg, rng, originX, originZ)
					}
				case "canyon":
					cfg, err := configured.Canyon()
					if err != nil {
						continue
					}
					if rng.NextFloat() <= float32(cfg.Probability) {
						g.carveCanyon(ctx, cfg, rng, originX, originZ)
					}
				}
			}
		}
	}
}

// carveCave mirrors CaveWorldCarver.carve.
func (g Generator) carveCave(ctx *carveContext, cfg gen.CaveCarverConfig, rng *gen.WorldgenRandom, originX, originZ int) {
	const maxDistance = 112 // sectionToBlockCoord(getRange()*2 - 1)
	caveCount := int(rng.NextInt(rng.NextInt(rng.NextInt(15)+1) + 1))

	for cave := 0; cave < caveCount; cave++ {
		x := float64(originX*16 + int(rng.NextInt(16)))
		y := float64(g.sampleHeightProvider(cfg.Y, ctx.minY, ctx.maxY, rng))
		z := float64(originZ*16 + int(rng.NextInt(16)))
		horizontalRadiusMultiplier := float64(g.sampleFloatProvider32(cfg.HorizontalRadiusMultiplier, rng))
		verticalRadiusMultiplier := float64(g.sampleFloatProvider32(cfg.VerticalRadiusMultiplier, rng))
		floorLevel := float64(g.sampleFloatProvider32(cfg.FloorLevel, rng))
		skip := func(xd, yd, zd float64, _ int) bool {
			if yd <= floorLevel {
				return true
			}
			return xd*xd+yd*yd+zd*zd >= 1.0
		}
		tunnels := 1
		if rng.NextInt(4) == 0 {
			yScale := float64(g.sampleFloatProvider32(cfg.YScale, rng))
			thickness := 1.0 + rng.NextFloat()*6.0
			horizontalRadius := 1.5 + float64(gen.MthSin(math.Pi/2))*float64(thickness)
			verticalRadius := horizontalRadius * yScale
			g.carveEllipsoid(ctx, cfg.LavaLevel, x+1.0, y, z, horizontalRadius, verticalRadius, skip)
			tunnels += int(rng.NextInt(4))
		}

		for i := 0; i < tunnels; i++ {
			horizontalRotation := rng.NextFloat() * (math.Pi * 2)
			verticalRotation := (rng.NextFloat() - 0.5) / 4.0
			thickness := caveThickness(rng)
			distance := maxDistance - int(rng.NextInt(maxDistance/4))
			g.createTunnel(ctx, cfg, int64(rng.NextLong()), x, y, z,
				horizontalRadiusMultiplier, verticalRadiusMultiplier, thickness,
				horizontalRotation, verticalRotation, 0, distance, 1.0, floorLevel)
		}
	}
}

func caveThickness(rng *gen.WorldgenRandom) float32 {
	thickness := rng.NextFloat()*2.0 + rng.NextFloat()
	if rng.NextInt(10) == 0 {
		thickness *= rng.NextFloat()*rng.NextFloat()*3.0 + 1.0
	}
	return thickness
}

// createTunnel mirrors CaveWorldCarver.createTunnel; the tunnel random is a
// fresh legacy source like vanilla's RandomSource.create(seed).
func (g Generator) createTunnel(ctx *carveContext, cfg gen.CaveCarverConfig, tunnelSeed int64, x, y, z float64,
	horizontalRadiusMultiplier, verticalRadiusMultiplier float64, thickness float32,
	horizontalRotation, verticalRotation float32, step, distance int, yScale, floorLevel float64) {

	random := gen.NewLegacyRandom(tunnelSeed)
	splitPoint := random.NextInt(distance/2) + distance/4
	steep := random.NextInt(6) == 0
	var yRota, xRota float32

	skip := func(xd, yd, zd float64, _ int) bool {
		if yd <= floorLevel {
			return true
		}
		return xd*xd+yd*yd+zd*zd >= 1.0
	}

	for currentStep := step; currentStep < distance; currentStep++ {
		horizontalRadius := 1.5 + float64(gen.MthSin(float32(math.Pi)*float32(currentStep)/float32(distance)))*float64(thickness)
		verticalRadius := horizontalRadius * yScale
		cosX := gen.MthCos(verticalRotation)
		x += float64(gen.MthCos(horizontalRotation) * cosX)
		y += float64(gen.MthSin(verticalRotation))
		z += float64(gen.MthSin(horizontalRotation) * cosX)
		if steep {
			verticalRotation *= 0.92
		} else {
			verticalRotation *= 0.7
		}
		verticalRotation += xRota * 0.1
		horizontalRotation += yRota * 0.1
		xRota *= 0.9
		yRota *= 0.75
		xRota += (random.NextFloat() - random.NextFloat()) * random.NextFloat() * 2.0
		yRota += (random.NextFloat() - random.NextFloat()) * random.NextFloat() * 4.0
		if currentStep == splitPoint && thickness > 1.0 {
			g.createTunnel(ctx, cfg, random.NextLong(), x, y, z,
				horizontalRadiusMultiplier, verticalRadiusMultiplier,
				random.NextFloat()*0.5+0.5,
				horizontalRotation-(math.Pi/2), verticalRotation/3.0,
				currentStep, distance, 1.0, floorLevel)
			g.createTunnel(ctx, cfg, random.NextLong(), x, y, z,
				horizontalRadiusMultiplier, verticalRadiusMultiplier,
				random.NextFloat()*0.5+0.5,
				horizontalRotation+(math.Pi/2), verticalRotation/3.0,
				currentStep, distance, 1.0, floorLevel)
			return
		}

		if random.NextInt(4) != 0 {
			if !carverCanReach(ctx.chunkX, ctx.chunkZ, x, z, currentStep, distance, thickness) {
				return
			}
			g.carveEllipsoid(ctx, cfg.LavaLevel, x, y, z,
				horizontalRadius*horizontalRadiusMultiplier,
				verticalRadius*verticalRadiusMultiplier, skip)
		}
	}
}

// carveCanyon mirrors CanyonWorldCarver.carve/doCarve.
func (g Generator) carveCanyon(ctx *carveContext, cfg gen.CanyonCarverConfig, rng *gen.WorldgenRandom, originX, originZ int) {
	const maxDistance = 112
	x := float64(originX*16 + int(rng.NextInt(16)))
	y := float64(g.sampleHeightProvider(cfg.Y, ctx.minY, ctx.maxY, rng))
	z := float64(originZ*16 + int(rng.NextInt(16)))
	horizontalRotation := rng.NextFloat() * (math.Pi * 2)
	verticalRotation := g.sampleFloatProvider32(cfg.VerticalRotation, rng)
	yScale := float64(g.sampleFloatProvider32(cfg.YScale, rng))
	thickness := g.sampleFloatProvider32(cfg.Shape.Thickness, rng)
	distance := int(float32(maxDistance) * g.sampleFloatProvider32(cfg.Shape.DistanceFactor, rng))
	tunnelSeed := int64(rng.NextLong())

	random := gen.NewLegacyRandom(tunnelSeed)
	widthFactors := g.canyonWidthFactors(ctx, cfg, &random)
	var yRota, xRota float32

	skip := func(xd, yd, zd float64, blockY int) bool {
		yIndex := blockY - ctx.minY
		if yIndex-1 < 0 || yIndex-1 >= len(widthFactors) {
			return true
		}
		return (xd*xd+zd*zd)*float64(widthFactors[yIndex-1])+yd*yd/6.0 >= 1.0
	}

	for currentStep := 0; currentStep < distance; currentStep++ {
		horizontalRadius := 1.5 + float64(gen.MthSin(float32(currentStep)*float32(math.Pi)/float32(distance)))*float64(thickness)
		verticalRadius := horizontalRadius * yScale
		horizontalRadius *= float64(g.sampleFloatProvider32(cfg.Shape.HorizontalRadiusFactor, &random))
		verticalRadius = canyonVerticalRadius(cfg, &random, verticalRadius, float32(distance), float32(currentStep))
		cosX := gen.MthCos(verticalRotation)
		sinX := gen.MthSin(verticalRotation)
		x += float64(gen.MthCos(horizontalRotation) * cosX)
		y += float64(sinX)
		z += float64(gen.MthSin(horizontalRotation) * cosX)
		verticalRotation *= 0.7
		verticalRotation += xRota * 0.05
		horizontalRotation += yRota * 0.05
		xRota *= 0.8
		yRota *= 0.5
		xRota += (random.NextFloat() - random.NextFloat()) * random.NextFloat() * 2.0
		yRota += (random.NextFloat() - random.NextFloat()) * random.NextFloat() * 4.0
		if random.NextInt(4) != 0 {
			if !carverCanReach(ctx.chunkX, ctx.chunkZ, x, z, currentStep, distance, thickness) {
				return
			}
			g.carveEllipsoid(ctx, cfg.LavaLevel, x, y, z, horizontalRadius, verticalRadius, skip)
		}
	}
}

func (g Generator) canyonWidthFactors(ctx *carveContext, cfg gen.CanyonCarverConfig, random *gen.LegacyRandom) []float32 {
	depth := ctx.maxY - ctx.minY + 1
	factors := make([]float32, depth)
	widthFactor := float32(1.0)
	for yIndex := 0; yIndex < depth; yIndex++ {
		if yIndex == 0 || random.NextInt(cfg.Shape.WidthSmoothness) == 0 {
			widthFactor = 1.0 + random.NextFloat()*random.NextFloat()
		}
		factors[yIndex] = widthFactor * widthFactor
	}
	return factors
}

func canyonVerticalRadius(cfg gen.CanyonCarverConfig, random *gen.LegacyRandom, verticalRadius float64, distance, currentStep float32) float64 {
	verticalMultiplier := 1.0 - abs32(0.5-currentStep/distance)*2.0
	factor := float32(cfg.Shape.VerticalRadiusDefaultFactor) + float32(cfg.Shape.VerticalRadiusCenterFactor)*verticalMultiplier
	between := 0.75 + random.NextFloat()*(1.0-0.75)
	return float64(factor) * verticalRadius * float64(between)
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func carverCanReach(chunkX, chunkZ int, x, z float64, currentStep, totalSteps int, thickness float32) bool {
	xMid := float64(chunkX*16 + 8)
	zMid := float64(chunkZ*16 + 8)
	xd := x - xMid
	zd := z - zMid
	remaining := float64(totalSteps - currentStep)
	rr := float64(thickness + 2.0 + 16.0)
	return xd*xd+zd*zd-remaining*remaining <= rr*rr
}

// carveEllipsoid mirrors WorldCarver.carveEllipsoid: y iterates downward and
// each carved position is masked and passed through carveBlock.
func (g Generator) carveEllipsoid(ctx *carveContext, lavaLevel gen.VerticalAnchor, x, y, z, horizontalRadius, verticalRadius float64, skip func(xd, yd, zd float64, blockY int) bool) {
	centerX := float64(ctx.chunkX*16 + 8)
	centerZ := float64(ctx.chunkZ*16 + 8)
	maxDelta := 16.0 + horizontalRadius*2.0
	if math.Abs(x-centerX) > maxDelta || math.Abs(z-centerZ) > maxDelta {
		return
	}
	chunkMinX := ctx.chunkX * 16
	chunkMinZ := ctx.chunkZ * 16
	minXIndex := max(int(math.Floor(x-horizontalRadius))-chunkMinX-1, 0)
	maxXIndex := min(int(math.Floor(x+horizontalRadius))-chunkMinX, 15)
	minY := max(int(math.Floor(y-verticalRadius))-1, ctx.minY+1)
	maxY := min(int(math.Floor(y+verticalRadius))+1, ctx.maxY-7)
	minZIndex := max(int(math.Floor(z-horizontalRadius))-chunkMinZ-1, 0)
	maxZIndex := min(int(math.Floor(z+horizontalRadius))-chunkMinZ, 15)

	lavaLevelY := g.anchorY(lavaLevel, ctx.minY, ctx.maxY)

	for xIndex := minXIndex; xIndex <= maxXIndex; xIndex++ {
		worldX := chunkMinX + xIndex
		xd := (float64(worldX) + 0.5 - x) / horizontalRadius
		for zIndex := minZIndex; zIndex <= maxZIndex; zIndex++ {
			worldZ := chunkMinZ + zIndex
			zd := (float64(worldZ) + 0.5 - z) / horizontalRadius
			if xd*xd+zd*zd >= 1.0 {
				continue
			}
			hasGrass := false
			for worldY := maxY; worldY > minY; worldY-- {
				yd := (float64(worldY) - 0.5 - y) / verticalRadius
				if !skip(xd, yd, zd, worldY) && !ctx.mask.get(xIndex, worldY, zIndex) {
					ctx.mask.set(xIndex, worldY, zIndex)
					g.carveBlock(ctx, lavaLevelY, xIndex, worldY, zIndex, worldX, worldZ, &hasGrass)
				}
			}
		}
	}
}

// carveBlock mirrors WorldCarver.carveBlock including the exposed-dirt
// topMaterial fixup that reapplies the surface rules below carved grass.
func (g Generator) carveBlock(ctx *carveContext, lavaLevelY, localX, worldY, localZ, worldX, worldZ int, hasGrass *bool) {
	rid := ctx.c.Block(uint8(localX), int16(worldY), uint8(localZ), 0)
	name := g.carverBlockName(rid)
	if name == "grass" || name == "grass_block" || name == "mycelium" {
		*hasGrass = true
	}
	if !g.isCarverReplaceable(rid, name) {
		return
	}
	state, carved := g.carveState(ctx, lavaLevelY, worldX, worldY, worldZ)
	if !carved {
		return
	}
	ctx.c.SetBlock(uint8(localX), int16(worldY), uint8(localZ), 0, state)
	if *hasGrass && worldY-1 >= ctx.minY {
		belowRID := ctx.c.Block(uint8(localX), int16(worldY-1), uint8(localZ), 0)
		if g.carverBlockName(belowRID) == "dirt" {
			underFluid := state == g.waterRID || state == g.lavaRID || state == g.defaultFluidRID
			if topMaterial, ok := g.carveTopMaterial(ctx, worldX, worldY-1, worldZ, underFluid); ok {
				ctx.c.SetBlock(uint8(localX), int16(worldY-1), uint8(localZ), 0, topMaterial)
			}
		}
	}
}

// carveState mirrors WorldCarver.getCarveState: lava below the lava level,
// otherwise the aquifer decides between air, water and lava; a barrier means
// the block stays solid.
func (g Generator) carveState(ctx *carveContext, lavaLevelY, worldX, worldY, worldZ int) (uint32, bool) {
	if worldY <= lavaLevelY {
		return g.lavaRID, true
	}
	if ctx.aquifer != nil {
		switch ctx.aquifer.ComputeSubstance(gen.FunctionContext{BlockX: worldX, BlockY: worldY, BlockZ: worldZ}, 0.0) {
		case gen.AquiferBarrier:
			return 0, false
		case gen.AquiferWater:
			return g.waterRID, true
		case gen.AquiferLava:
			return g.lavaRID, true
		default:
			return g.airRID, true
		}
	}
	// Disabled aquifer (nether/end): the fluid picker decides directly.
	if g.defaultFluidRID != g.airRID && worldY < g.metadata.SeaLevel {
		return g.defaultFluidRID, true
	}
	return g.airRID, true
}

// carveTopMaterial mirrors CarvingContext.topMaterial: reapply the surface
// rule at the position with stone depths of 1.
func (g Generator) carveTopMaterial(ctx *carveContext, worldX, worldY, worldZ int, underFluid bool) (uint32, bool) {
	if g.surface == nil {
		return 0, false
	}
	waterHeight := -1 << 31
	if underFluid {
		waterHeight = worldY + 1
	}
	surfaceDepth := g.surface.SurfaceDepth(worldX, worldZ)
	sctx := gen.SurfaceContext{
		BlockX:           worldX,
		BlockY:           worldY,
		BlockZ:           worldZ,
		SurfaceDepth:     surfaceDepth,
		SurfaceSecondary: g.surface.SurfaceSecondary(worldX, worldZ),
		WaterHeight:      waterHeight,
		StoneDepthAbove:  1,
		StoneDepthBelow:  1,
		Biome:            g.zoomedBiomeAt(worldX, worldY, worldZ),
		MinSurfaceLevel:  worldY - surfaceDepth,
		MinY:             ctx.minY,
		MaxY:             ctx.maxY,
	}
	return g.surface.TryApply(sctx, g.lookupSurfaceBlock)
}

func (g Generator) carverBlockName(rid uint32) string {
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

// isCarverReplaceable stands in for configuration.replaceable: every block
// state that can exist before decoration runs is in the carver replaceable
// tags except air, lava and bedrock (plus gravel in the nether, which is
// absent from nether_carver_replaceables).
func (g Generator) isCarverReplaceable(rid uint32, name string) bool {
	if rid == g.airRID || rid == g.lavaRID || rid == g.bedrockRID {
		return false
	}
	if g.dimension == world.Nether && name == "gravel" {
		return false
	}
	return true
}

// floatRandom is satisfied by both *gen.WorldgenRandom and *gen.LegacyRandom.
type floatRandom interface {
	NextFloat() float32
}

func (g Generator) sampleFloatProvider32(provider gen.FloatProvider, rng floatRandom) float32 {
	switch provider.Kind {
	case "uniform":
		minV := float32(provider.Min)
		maxV := float32(provider.Max)
		return minV + rng.NextFloat()*(maxV-minV)
	case "trapezoid":
		minV := float32(provider.Min)
		maxV := float32(provider.Max)
		plateau := float32(provider.Plateau)
		spread := maxV - minV
		plateauStart := (spread - plateau) / 2.0
		plateauEnd := spread - plateauStart
		return minV + rng.NextFloat()*plateauEnd + rng.NextFloat()*plateauStart
	default:
		if provider.Constant != nil {
			return float32(*provider.Constant)
		}
		return float32(provider.Min)
	}
}

// sampleFloatProvider is the float64 variant shared with features and
// terrain adaptation; it consumes the RNG exactly like the vanilla float
// providers (float32 arithmetic over nextFloat).
func (g Generator) sampleFloatProvider(provider gen.FloatProvider, rng *gen.WorldgenRandom) float64 {
	return float64(g.sampleFloatProvider32(provider, rng))
}

func clampFloat(value, low, high float64) float64 {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
