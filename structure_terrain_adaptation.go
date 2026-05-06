package vanilla

import (
	"math"

	gen "github.com/bedrock-mc/vanilla-gen/gen"
)

const (
	structureWeightIndexOffset = 12
	structureWeightEdgeLength  = 24
	structureWeightTableSize   = structureWeightEdgeLength * structureWeightEdgeLength * structureWeightEdgeLength
	structureTerrainMargin     = 12
)

var structureWeightTable = func() [structureWeightTableSize]float64 {
	var table [structureWeightTableSize]float64
	for z := 0; z < structureWeightEdgeLength; z++ {
		for x := 0; x < structureWeightEdgeLength; x++ {
			for y := 0; y < structureWeightEdgeLength; y++ {
				value := structureDensityWeight(x-structureWeightIndexOffset, y-structureWeightIndexOffset, z-structureWeightIndexOffset)
				table[z*structureWeightEdgeLength*structureWeightEdgeLength+x*structureWeightEdgeLength+y] = float64(float32(value))
			}
		}
	}
	return table
}()

type structureTerrainPiece struct {
	box               structureBox
	terrainAdaptation string
	groundLevelDelta  int
}

type structureTerrainSampler struct {
	pieces    []structureTerrainPiece
	junctions []plannedStructureJunction
}

func newStructureTerrainSampler(g Generator, chunkX, chunkZ, minY, maxY int) *structureTerrainSampler {
	if g.structureStarts == nil || len(g.structurePlanners) == 0 {
		return nil
	}

	sampler := &structureTerrainSampler{}
	surfaceSampler := newStructureHeightSampler(g, minY, maxY)
	for _, planner := range g.structurePlanners {
		for _, startChunk := range g.plannerPotentialStartChunksNearChunk(planner, chunkX, chunkZ, 1, 1) {
			start, ok := g.planStructureStart(planner, startChunk, minY, maxY, surfaceSampler)
			if !ok || start.terrainAdaptation == "" || start.terrainAdaptation == "none" {
				continue
			}
			if !structureIntersectsChunkHorizontalMargin(start, chunkX, chunkZ, structureTerrainMargin) {
				continue
			}
			sampler.appendStart(start, chunkX, chunkZ)
		}
	}
	if sampler.empty() {
		return nil
	}
	return sampler
}

func (s *structureTerrainSampler) empty() bool {
	return s == nil || (len(s.pieces) == 0 && len(s.junctions) == 0)
}

func (s *structureTerrainSampler) appendStart(start plannedStructureStart, chunkX, chunkZ int) {
	chunkMinX := chunkX*16 - structureTerrainMargin
	chunkMaxX := chunkX*16 + 15 + structureTerrainMargin
	chunkMinZ := chunkZ*16 - structureTerrainMargin
	chunkMaxZ := chunkZ*16 + 15 + structureTerrainMargin

	for _, piece := range start.pieces {
		if !piece.bounds.intersectsHorizontalBounds(chunkMinX, chunkMaxX, chunkMinZ, chunkMaxZ) {
			continue
		}
		isPoolPiece := piece.element.projection != ""
		if !isPoolPiece || normalizeIdentifierName(piece.element.projection) == "rigid" {
			s.pieces = append(s.pieces, structureTerrainPiece{
				box:               piece.bounds,
				terrainAdaptation: start.terrainAdaptation,
				groundLevelDelta:  piece.groundLevelDelta,
			})
		}
		for _, junction := range piece.junctions {
			if junction.sourceX > chunkMinX && junction.sourceX < chunkMaxX && junction.sourceZ > chunkMinZ && junction.sourceZ < chunkMaxZ {
				s.junctions = append(s.junctions, junction)
			}
		}
	}
}

func (s *structureTerrainSampler) scalarEvaluator(g Generator, baseScalar gen.DensityScalarEvaluator) gen.DensityScalarEvaluator {
	root := g.rootIndex("final_density")
	return func(ctx gen.FunctionContext, noises gen.NoiseSource, flat *gen.FlatCacheGrid, col *gen.ColumnContext) float64 {
		return gen.EvalDensityScalar(g.graph, root, ctx, noises, flat, col, baseScalar) + s.sample(ctx.BlockX, ctx.BlockY, ctx.BlockZ)
	}
}

func (s *structureTerrainSampler) vectorEvaluator(g Generator, baseScalar gen.DensityScalarEvaluator, baseVector gen.DensityVectorEvaluator) gen.DensityVectorEvaluator {
	root := g.rootIndex("final_density")
	return func(ctx gen.FunctionContext4, noises gen.NoiseSource, flat *gen.FlatCacheGrid, col *gen.ColumnContext) [4]float64 {
		var out [4]float64
		if baseVector != nil {
			out = baseVector(ctx, noises, flat, col)
		} else {
			for i := range out {
				out[i] = gen.EvalDensityScalar(
					g.graph,
					root,
					gen.FunctionContext{BlockX: ctx.BlockX, BlockY: ctx.BlockY[i], BlockZ: ctx.BlockZ},
					noises,
					flat,
					col,
					baseScalar,
				)
			}
		}
		for i := range out {
			out[i] += s.sample(ctx.BlockX, ctx.BlockY[i], ctx.BlockZ)
		}
		return out
	}
}

func (s *structureTerrainSampler) sample(blockX, blockY, blockZ int) float64 {
	if s.empty() {
		return 0
	}

	value := 0.0
	for _, piece := range s.pieces {
		dx := max(0, max(piece.box.minX-blockX, blockX-piece.box.maxX))
		dz := max(0, max(piece.box.minZ-blockZ, blockZ-piece.box.maxZ))
		groundY := piece.box.minY + piece.groundLevelDelta
		deltaY := blockY - groundY

		vertical := 0
		switch piece.terrainAdaptation {
		case "bury", "beard_thin":
			vertical = deltaY
		case "beard_box":
			vertical = max(0, max(groundY-blockY, blockY-piece.box.maxY))
		case "encapsulate":
			vertical = max(0, max(piece.box.minY-blockY, blockY-piece.box.maxY))
		default:
			continue
		}

		switch piece.terrainAdaptation {
		case "bury":
			value += structureMagnitudeWeight(float64(dx), float64(vertical)/2.0, float64(dz))
		case "beard_thin", "beard_box":
			value += structurePieceWeight(dx, vertical, dz, deltaY) * 0.8
		case "encapsulate":
			value += structureMagnitudeWeight(float64(dx)/2.0, float64(vertical)/2.0, float64(dz)/2.0) * 0.8
		}
	}

	for _, junction := range s.junctions {
		dx := blockX - junction.sourceX
		dy := blockY - junction.sourceGroundY
		dz := blockZ - junction.sourceZ
		value += structurePieceWeight(dx, dy, dz, dy) * 0.4
	}
	return value
}

func structureIntersectsChunkHorizontalMargin(start plannedStructureStart, chunkX, chunkZ, margin int) bool {
	return structureBox{
		minX: start.origin[0],
		minY: start.origin[1],
		minZ: start.origin[2],
		maxX: start.origin[0] + start.size[0] - 1,
		maxY: start.origin[1] + start.size[1] - 1,
		maxZ: start.origin[2] + start.size[2] - 1,
	}.intersectsHorizontalBounds(chunkX*16-margin, chunkX*16+15+margin, chunkZ*16-margin, chunkZ*16+15+margin)
}

func (b structureBox) intersectsHorizontalBounds(minX, maxX, minZ, maxZ int) bool {
	if b.empty() {
		return false
	}
	return b.maxX >= minX && b.minX <= maxX && b.maxZ >= minZ && b.minZ <= maxZ
}

func structureMagnitudeWeight(x, y, z float64) float64 {
	distance := math.Sqrt(x*x + y*y + z*z)
	return clampFloat64(1.0-distance/6.0, 0.0, 1.0)
}

func structurePieceWeight(x, y, z, yy int) float64 {
	i := x + structureWeightIndexOffset
	j := y + structureWeightIndexOffset
	k := z + structureWeightIndexOffset
	if !structureWeightIndexInBounds(i) || !structureWeightIndexInBounds(j) || !structureWeightIndexInBounds(k) {
		return 0
	}

	offsetY := float64(yy) + 0.5
	distanceSq := float64(x*x+z*z) + offsetY*offsetY
	if distanceSq == 0 {
		return 0
	}
	inv := 1.0 / math.Sqrt(distanceSq/2.0)
	weight := -offsetY * inv / 2.0
	return weight * structureWeightTable[k*structureWeightEdgeLength*structureWeightEdgeLength+i*structureWeightEdgeLength+j]
}

func structureDensityWeight(x, y, z int) float64 {
	offsetY := float64(y) + 0.5
	distanceSq := float64(x*x+z*z) + offsetY*offsetY
	return math.Exp(-distanceSq / 16.0)
}

func structureWeightIndexInBounds(i int) bool {
	return i >= 0 && i < structureWeightEdgeLength
}

func clampFloat64(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
