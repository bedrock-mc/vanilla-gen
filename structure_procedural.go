package vanilla

import (
	"sort"
	"strconv"

	gen "github.com/bedrock-mc/vanilla-gen/gen"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
)

type proceduralStructureBuilder struct {
	origin   cube.Pos
	localMin cube.Pos
	size     [3]int
	rotation structureRotation
	bounds   structureBox
	blocks   map[cube.Pos]gen.BlockState
}

func newProceduralStructureBuilder(origin, localMin cube.Pos, size [3]int, rotation structureRotation) *proceduralStructureBuilder {
	return &proceduralStructureBuilder{
		origin:   origin,
		localMin: localMin,
		size:     size,
		rotation: rotation,
		bounds:   proceduralStructureWorldBox(origin, size, rotation),
		blocks:   make(map[cube.Pos]gen.BlockState),
	}
}

func proceduralStructureWorldBox(origin cube.Pos, size [3]int, rotation structureRotation) structureBox {
	rotated := rotatedStructureSize(size, rotation)
	return structureBox{
		minX: origin[0],
		minY: origin[1],
		minZ: origin[2],
		maxX: origin[0] + rotated[0] - 1,
		maxY: origin[1] + rotated[1] - 1,
		maxZ: origin[2] + rotated[2] - 1,
	}
}

func (b *proceduralStructureBuilder) worldPos(localX, localY, localZ int) cube.Pos {
	relative := [3]int{
		localX - b.localMin[0],
		localY - b.localMin[1],
		localZ - b.localMin[2],
	}
	rotated := rotateStructurePos(b.size, relative, b.rotation)
	return b.origin.Add(cube.Pos{rotated[0], rotated[1], rotated[2]})
}

func (b *proceduralStructureBuilder) setBlock(localX, localY, localZ int, state gen.BlockState) {
	b.blocks[b.worldPos(localX, localY, localZ)] = rotatePlacedStructureState(state, b.rotation)
}

func (b *proceduralStructureBuilder) setRawBlock(localX, localY, localZ int, state gen.BlockState) {
	b.blocks[b.worldPos(localX, localY, localZ)] = cloneBlockState(state)
}

func (b *proceduralStructureBuilder) setWorldBlock(localX, localY, localZ int, block world.Block) {
	b.setRawBlock(localX, localY, localZ, blockStateFromWorldBlock(block))
}

func (b *proceduralStructureBuilder) setWorldBlockAtWorld(pos cube.Pos, block world.Block) {
	b.setRawBlockAtWorld(pos, blockStateFromWorldBlock(block))
}

func (b *proceduralStructureBuilder) fillSolidBox(x0, y0, z0, x1, y1, z1 int, state gen.BlockState) {
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			for z := z0; z <= z1; z++ {
				b.setBlock(x, y, z, state)
			}
		}
	}
}

func (b *proceduralStructureBuilder) fillHollowBox(x0, y0, z0, x1, y1, z1 int, edge, fill gen.BlockState) {
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			for z := z0; z <= z1; z++ {
				if y == y0 || y == y1 || x == x0 || x == x1 || z == z0 || z == z1 {
					b.setBlock(x, y, z, edge)
				} else {
					b.setBlock(x, y, z, fill)
				}
			}
		}
	}
}

func (b *proceduralStructureBuilder) fillSelectedBox(x0, y0, z0, x1, y1, z1 int, selector func(localX, localY, localZ int, edge bool) gen.BlockState) {
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			for z := z0; z <= z1; z++ {
				edge := y == y0 || y == y1 || x == x0 || x == x1 || z == z0 || z == z1
				b.setBlock(x, y, z, selector(x, y, z, edge))
			}
		}
	}
}

func (b *proceduralStructureBuilder) fillAirBox(x0, y0, z0, x1, y1, z1 int) {
	b.fillSolidBox(x0, y0, z0, x1, y1, z1, gen.BlockState{Name: "air"})
}

func (b *proceduralStructureBuilder) fillWorldBox(x0, y0, z0, x1, y1, z1 int, block world.Block) {
	state := blockStateFromWorldBlock(block)
	b.fillSolidBox(x0, y0, z0, x1, y1, z1, state)
}

func (b *proceduralStructureBuilder) fillFoundationColumn(g Generator, state gen.BlockState, localX, startY, localZ, minY int) {
	pos := b.worldPos(localX, startY, localZ)
	for y := pos[1]; y > minY+1; y-- {
		if g.sampleStructureSubstanceAt(pos[0], y, pos[2]) == structureSubstanceSolid {
			return
		}
		b.setRawBlockAtWorld(cube.Pos{pos[0], y, pos[2]}, state)
	}
}

func (b *proceduralStructureBuilder) setRawBlockAtWorld(pos cube.Pos, state gen.BlockState) {
	b.blocks[pos] = cloneBlockState(state)
}

func (b *proceduralStructureBuilder) placeCardinalBlock(localX, localY, localZ int, name, facing string, extra map[string]string) {
	facing = rotateHorizontalDirectionName(facing, b.rotation)
	state := gen.BlockState{Name: name}
	if len(extra) != 0 {
		state.Properties = make(map[string]string, len(extra)+1)
		for key, value := range extra {
			state.Properties[key] = value
		}
	} else {
		state.Properties = make(map[string]string, 1)
	}
	state.Properties["minecraft:cardinal_direction"] = facing
	b.setRawBlock(localX, localY, localZ, state)
}

func (b *proceduralStructureBuilder) placeChest(localX, localY, localZ int, facing string) {
	b.placeCardinalBlock(localX, localY, localZ, "chest", facing, nil)
}

func (b *proceduralStructureBuilder) placeTripwireHook(localX, localY, localZ int, facing string, attached bool) {
	facing = rotateHorizontalDirectionName(facing, b.rotation)
	b.setRawBlock(localX, localY, localZ, gen.BlockState{
		Name: "tripwire_hook",
		Properties: map[string]string{
			"attached_bit": boolString(attached),
			"direction":    structureHorizontalDirectionValue(facing),
			"powered_bit":  "false",
		},
	})
}

func (b *proceduralStructureBuilder) placeTripwire(localX, localY, localZ int, north, east, south, west, attached bool) {
	state := gen.BlockState{
		Name: "trip_wire",
		Properties: map[string]string{
			"attached_bit":  boolString(attached),
			"disarmed_bit":  "false",
			"east":          boolString(east),
			"north":         boolString(north),
			"powered_bit":   "false",
			"south":         boolString(south),
			"suspended_bit": "false",
			"west":          boolString(west),
		},
	}
	b.setBlock(localX, localY, localZ, state)
}

func (b *proceduralStructureBuilder) placeRedstoneWire(localX, localY, localZ int, north, east, south, west string) {
	north, east, south, west = rotateRedstoneSides(north, east, south, west, b.rotation)
	b.setRawBlock(localX, localY, localZ, gen.BlockState{
		Name: "redstone_wire",
		Properties: map[string]string{
			"east_redstone":   east,
			"north_redstone":  north,
			"redstone_signal": "0",
			"south_redstone":  south,
			"west_redstone":   west,
		},
	})
}

func (b *proceduralStructureBuilder) placeRepeater(localX, localY, localZ int, powered bool, facing string, delay int) {
	facing = rotateHorizontalDirectionName(facing, b.rotation)
	name := "unpowered_repeater"
	if powered {
		name = "powered_repeater"
	}
	b.setRawBlock(localX, localY, localZ, gen.BlockState{
		Name: name,
		Properties: map[string]string{
			"minecraft:cardinal_direction": facing,
			"repeater_delay":               strconv.Itoa(clamp(delay, 0, 3)),
		},
	})
}

func (b *proceduralStructureBuilder) placeLever(localX, localY, localZ int, facing, face string) {
	facing = rotateHorizontalDirectionName(facing, b.rotation)
	leverDirection := facing
	switch face {
	case "floor":
		if facing == "north" || facing == "south" {
			leverDirection = "up_north_south"
		} else {
			leverDirection = "up_east_west"
		}
	case "ceiling":
		if facing == "north" || facing == "south" {
			leverDirection = "down_north_south"
		} else {
			leverDirection = "down_east_west"
		}
	}
	b.setRawBlock(localX, localY, localZ, gen.BlockState{
		Name: "lever",
		Properties: map[string]string{
			"lever_direction": leverDirection,
			"open_bit":        "false",
		},
	})
}

func (b *proceduralStructureBuilder) placeStickyPiston(localX, localY, localZ int, facing string) {
	facing = rotateFullDirectionName(facing, b.rotation)
	b.setRawBlock(localX, localY, localZ, gen.BlockState{
		Name: "sticky_piston",
		Properties: map[string]string{
			"facing_direction": structureFacingDirectionValue(facing),
		},
	})
}

func (b *proceduralStructureBuilder) placeDispenser(localX, localY, localZ int, facing string) {
	facing = rotateFullDirectionName(facing, b.rotation)
	b.setRawBlock(localX, localY, localZ, gen.BlockState{
		Name: "dispenser",
		Properties: map[string]string{
			"facing_direction": structureFacingDirectionValue(facing),
			"triggered_bit":    "false",
		},
	})
}

func (b *proceduralStructureBuilder) placeVine(localX, localY, localZ int, north, east, south, west bool) {
	north, east, south, west = rotateSideBools(north, east, south, west, b.rotation)
	bits := 0
	if south {
		bits |= 1 << 0
	}
	if west {
		bits |= 1 << 1
	}
	if north {
		bits |= 1 << 2
	}
	if east {
		bits |= 1 << 3
	}
	b.setRawBlock(localX, localY, localZ, gen.BlockState{
		Name: "vine",
		Properties: map[string]string{
			"vine_direction_bits": strconv.Itoa(bits),
		},
	})
}

func (b *proceduralStructureBuilder) manualBlocks() []plannedStructureBlock {
	if len(b.blocks) == 0 {
		return nil
	}
	positions := make([]cube.Pos, 0, len(b.blocks))
	for pos := range b.blocks {
		positions = append(positions, pos)
	}
	sort.Slice(positions, func(i, j int) bool {
		if positions[i][1] != positions[j][1] {
			return positions[i][1] < positions[j][1]
		}
		if positions[i][0] != positions[j][0] {
			return positions[i][0] < positions[j][0]
		}
		return positions[i][2] < positions[j][2]
	})

	blocks := make([]plannedStructureBlock, 0, len(positions))
	for _, pos := range positions {
		blocks = append(blocks, plannedStructureBlock{
			worldPos: pos,
			state:    cloneBlockState(b.blocks[pos]),
		})
	}
	return blocks
}

func (b *proceduralStructureBuilder) piece() plannedStructurePiece {
	return plannedStructurePiece{
		origin:       b.origin,
		bounds:       b.bounds,
		manualBlocks: b.manualBlocks(),
		rootPiece:    true,
	}
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func structureState(name string, keyValues ...string) gen.BlockState {
	state := gen.BlockState{Name: name}
	if len(keyValues) == 0 {
		return state
	}
	state.Properties = make(map[string]string, len(keyValues)/2)
	for i := 0; i+1 < len(keyValues); i += 2 {
		state.Properties[keyValues[i]] = keyValues[i+1]
	}
	return state
}

func rotateRedstoneSides(north, east, south, west string, rotation structureRotation) (string, string, string, string) {
	switch rotation {
	case structureRotationClockwise90:
		return west, north, east, south
	case structureRotationClockwise180:
		return south, west, north, east
	case structureRotationCounterclockwise90:
		return east, south, west, north
	default:
		return north, east, south, west
	}
}

func rotateSideBools(north, east, south, west bool, rotation structureRotation) (bool, bool, bool, bool) {
	switch rotation {
	case structureRotationClockwise90:
		return west, north, east, south
	case structureRotationClockwise180:
		return south, west, north, east
	case structureRotationCounterclockwise90:
		return east, south, west, north
	default:
		return north, east, south, west
	}
}

func rotateFullDirectionName(value string, rotation structureRotation) string {
	switch value {
	case "north", "east", "south", "west":
		return rotateHorizontalDirectionName(value, rotation)
	default:
		return value
	}
}

func structureHorizontalDirectionValue(facing string) string {
	switch facing {
	case "south":
		return "0"
	case "west":
		return "1"
	case "north":
		return "2"
	case "east":
		return "3"
	default:
		return "0"
	}
}

func structureFacingDirectionValue(facing string) string {
	switch facing {
	case "down":
		return "0"
	case "up":
		return "1"
	case "north":
		return "2"
	case "south":
		return "3"
	case "west":
		return "4"
	case "east":
		return "5"
	default:
		return "0"
	}
}

func structureFacingFromRotation(rotation structureRotation) structureDirection {
	switch rotation {
	case structureRotationClockwise90:
		return structureWest
	case structureRotationClockwise180:
		return structureNorth
	case structureRotationCounterclockwise90:
		return structureEast
	default:
		return structureSouth
	}
}

func proceduralStructurePlacementFromFoot(foot cube.Pos, offX, offY, offZ int, width, height, depth int, facing structureDirection) (cube.Pos, structureRotation) {
	switch facing {
	case structureNorth:
		return cube.Pos{foot[0] + offX, foot[1] + offY, foot[2] - depth + 1 + offZ}, structureRotationClockwise180
	case structureWest:
		return cube.Pos{foot[0] - depth + 1 + offZ, foot[1] + offY, foot[2] + offX}, structureRotationClockwise90
	case structureEast:
		return cube.Pos{foot[0] + offZ, foot[1] + offY, foot[2] + offX}, structureRotationCounterclockwise90
	default:
		return cube.Pos{foot[0] + offX, foot[1] + offY, foot[2] + offZ}, structureRotationNone
	}
}
