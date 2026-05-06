package vanilla

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"

	gen "github.com/bedrock-mc/vanilla-gen/gen"
	"github.com/df-mc/dragonfly/server/block/cube"
)

type structureResolver struct {
	worldgen  *gen.WorldgenRegistry
	templates *gen.StructureTemplateRegistry

	mu        sync.RWMutex
	pools     map[string]resolvedStructurePool
	prewarmMu sync.Mutex
	prewarmed map[string]struct{}
}

type resolvedStructurePool struct {
	fallback string
	entries  []resolvedPoolElement
	weighted []int
}

type resolvedPoolElement struct {
	elementType string
	projection  string
	placements  []structureTemplatePlacement
	features    []structureFeaturePlacement
	jigsaws     []structureJigsaw
	rotated     [4]precomputedPlacedJigsaws
	size        [3]int
}

type precomputedPlacedJigsaws struct {
	values []placedStructureJigsaw
	groups []jigsawPriorityGroup
}

type jigsawPriorityGroup struct {
	start int
	end   int
}

type structureTemplatePlacement struct {
	templateName string
	ignoreAir    bool
	processors   []structureProcessor
}

type structureFeaturePlacement struct {
	featureName string
}

type structureJigsaw struct {
	pos               [3]int
	front             structureDirection
	top               structureDirection
	joint             string
	name              string
	pool              string
	target            string
	placementPriority int
	selectionPriority int
}

type placedStructureJigsaw struct {
	pos               cube.Pos
	localY            int
	front             structureDirection
	top               structureDirection
	joint             string
	name              string
	pool              string
	target            string
	placementPriority int
	selectionPriority int
}

type plannedStructurePiece struct {
	element              resolvedPoolElement
	origin               cube.Pos
	rotation             structureRotation
	mirror               structureMirror
	pivot                cube.Pos
	useTemplateTransform bool
	bounds               structureBox
	groundLevelDelta     int
	junctions            []plannedStructureJunction
	manualBlocks         []plannedStructureBlock
	genTag               int
	rootPiece            bool
}

type plannedStructureJunction struct {
	sourceX       int
	sourceGroundY int
	sourceZ       int
}

type structureRotation uint8

const (
	structureRotationNone structureRotation = iota
	structureRotationClockwise90
	structureRotationClockwise180
	structureRotationCounterclockwise90
)

type structureDirection uint8

const (
	structureDown structureDirection = iota
	structureUp
	structureNorth
	structureSouth
	structureWest
	structureEast
)

type structureBox struct {
	minX int
	minY int
	minZ int
	maxX int
	maxY int
	maxZ int
}

type structureCollisionSpace struct {
	allowed structureBox
	cutouts []structureBox
}

type listPoolElementDef struct {
	Elements   []gen.TemplatePoolElementDef `json:"elements"`
	Projection string                       `json:"projection"`
}

type featurePoolElementDef struct {
	Feature    string `json:"feature"`
	Projection string `json:"projection"`
}

type pendingStructurePiece struct {
	piece      plannedStructurePiece
	pieceIndex int
	depth      int
	priority   int
	space      *structureCollisionSpace
}

func newStructureResolver(worldgen *gen.WorldgenRegistry, templates *gen.StructureTemplateRegistry) *structureResolver {
	return &structureResolver{
		worldgen:  worldgen,
		templates: templates,
		pools:     make(map[string]resolvedStructurePool),
		prewarmed: make(map[string]struct{}),
	}
}

func (r *structureResolver) prewarmJigsawCandidates(planners []structurePlanner) {
	var queue []string
	for _, planner := range planners {
		for _, candidate := range planner.candidates {
			if candidate.structureType != "jigsaw" {
				continue
			}
			if root := normalizeIdentifierName(candidate.jigsaw.StartPool); root != "" && root != "empty" {
				queue = append(queue, root)
			}
			queue = append(queue, collectPoolAliasTargets(candidate.jigsaw.PoolAliases)...)
		}
	}
	r.prewarmPoolGraph(queue)
}

func (r *structureResolver) prewarmPoolGraph(queue []string) {
	if len(queue) == 0 {
		return
	}

	r.prewarmMu.Lock()
	defer r.prewarmMu.Unlock()

	visited := make(map[string]struct{}, len(r.prewarmed)+len(queue))
	for name := range r.prewarmed {
		visited[name] = struct{}{}
	}

	for len(queue) > 0 {
		name := normalizeIdentifierName(queue[len(queue)-1])
		queue = queue[:len(queue)-1]
		if name == "" || name == "empty" {
			continue
		}
		if _, ok := visited[name]; ok {
			continue
		}
		visited[name] = struct{}{}

		pool, err := r.Pool(name)
		if err != nil {
			continue
		}
		if fallback := normalizeIdentifierName(pool.fallback); fallback != "" && fallback != "empty" {
			queue = append(queue, fallback)
		}
		for _, entry := range pool.entries {
			for _, jigsaw := range entry.jigsaws {
				if poolName := normalizeIdentifierName(jigsaw.pool); poolName != "" && poolName != "empty" {
					queue = append(queue, poolName)
				}
			}
		}
	}

	for name := range visited {
		r.prewarmed[name] = struct{}{}
	}
}

func collectPoolAliasTargets(defs []gen.PoolAliasDef) []string {
	if len(defs) == 0 {
		return nil
	}
	var out []string
	var walk func([]gen.PoolAliasDef)
	walk = func(defs []gen.PoolAliasDef) {
		for _, def := range defs {
			switch def.Type {
			case "direct":
				var raw directPoolAliasDef
				if err := json.Unmarshal(def.Raw, &raw); err == nil {
					if target := normalizeIdentifierName(raw.Target); target != "" {
						out = append(out, target)
					}
				}
			case "random":
				var raw randomPoolAliasDef
				if err := json.Unmarshal(def.Raw, &raw); err == nil {
					for _, target := range raw.Targets {
						if name := normalizeIdentifierName(target.Data); name != "" {
							out = append(out, name)
						}
					}
				}
			case "random_group":
				var raw randomGroupPoolAliasDef
				if err := json.Unmarshal(def.Raw, &raw); err == nil {
					for _, group := range raw.Groups {
						walk(group.Data)
					}
				}
			}
		}
	}
	walk(defs)
	return out
}

func (r *structureResolver) Pool(name string) (resolvedStructurePool, error) {
	key := normalizeIdentifierName(name)

	r.mu.RLock()
	if pool, ok := r.pools[key]; ok {
		r.mu.RUnlock()
		return pool, nil
	}
	r.mu.RUnlock()

	def, err := r.worldgen.TemplatePool(key)
	if err != nil {
		return resolvedStructurePool{}, err
	}

	pool := resolvedStructurePool{
		fallback: normalizeIdentifierName(def.Fallback),
		entries:  make([]resolvedPoolElement, 0, len(def.Elements)),
	}
	for _, entry := range def.Elements {
		element, err := r.resolvePoolElement(entry.Element)
		if err != nil {
			continue
		}
		if entry.Weight <= 0 {
			continue
		}
		index := len(pool.entries)
		pool.entries = append(pool.entries, element)
		for i := 0; i < entry.Weight; i++ {
			pool.weighted = append(pool.weighted, index)
		}
	}

	r.mu.Lock()
	r.pools[key] = pool
	r.mu.Unlock()
	return pool, nil
}

func (r *structureResolver) resolvePoolElement(def gen.TemplatePoolElementDef) (resolvedPoolElement, error) {
	switch def.ElementType {
	case "legacy_single_pool_element", "single_pool_element":
		single, err := def.Single()
		if err != nil {
			return resolvedPoolElement{}, err
		}
		template, err := r.templates.Template(single.Location)
		if err != nil {
			return resolvedPoolElement{}, err
		}
		return precomputeResolvedPoolElementJigsaws(resolvedPoolElement{
			elementType: def.ElementType,
			projection:  normalizeIdentifierName(single.Projection),
			placements: []structureTemplatePlacement{{
				templateName: single.Location,
				ignoreAir:    def.ElementType == "legacy_single_pool_element",
				processors:   compileStructureProcessors(r.worldgen, single.Processors),
			}},
			jigsaws: extractTemplateJigsaws(template),
			size:    template.Size,
		}), nil
	case "list_pool_element":
		var raw listPoolElementDef
		if err := json.Unmarshal(def.Raw, &raw); err != nil {
			return resolvedPoolElement{}, err
		}
		out := resolvedPoolElement{
			elementType: def.ElementType,
			projection:  normalizeIdentifierName(raw.Projection),
		}
		for i, inner := range raw.Elements {
			resolved, err := r.resolvePoolElement(inner)
			if err != nil {
				continue
			}
			out.placements = append(out.placements, resolved.placements...)
			out.features = append(out.features, resolved.features...)
			out.size = maxStructureSize(out.size, resolved.size)
			if i == 0 {
				out.jigsaws = append(out.jigsaws, resolved.jigsaws...)
			}
		}
		return precomputeResolvedPoolElementJigsaws(out), nil
	case "feature_pool_element":
		var raw featurePoolElementDef
		if err := json.Unmarshal(def.Raw, &raw); err != nil {
			return resolvedPoolElement{}, err
		}
		return precomputeResolvedPoolElementJigsaws(resolvedPoolElement{
			elementType: def.ElementType,
			projection:  normalizeIdentifierName(raw.Projection),
			features: []structureFeaturePlacement{{
				featureName: normalizeIdentifierName(raw.Feature),
			}},
			size: [3]int{1, 1, 1},
			jigsaws: []structureJigsaw{{
				pos:    [3]int{0, 0, 0},
				front:  structureDown,
				top:    structureSouth,
				joint:  "rollable",
				name:   "bottom",
				pool:   "empty",
				target: "empty",
			}},
		}), nil
	case "empty_pool_element":
		return resolvedPoolElement{elementType: def.ElementType}, nil
	default:
		return resolvedPoolElement{}, fmt.Errorf("unsupported pool element type %q", def.ElementType)
	}
}

func maxStructureSize(a, b [3]int) [3]int {
	if b[0] > a[0] {
		a[0] = b[0]
	}
	if b[1] > a[1] {
		a[1] = b[1]
	}
	if b[2] > a[2] {
		a[2] = b[2]
	}
	return a
}

func extractTemplateJigsaws(template gen.StructureTemplate) []structureJigsaw {
	jigsaws := make([]structureJigsaw, 0, 8)
	for _, block := range template.Blocks {
		if block.State < 0 || block.State >= len(template.Palette) {
			continue
		}
		state := template.Palette[block.State]
		if state.Name != "minecraft:jigsaw" {
			continue
		}
		front, top := parseJigsawOrientation(state.Properties)
		joint := parseJigsawJoint(block.NBT, front)
		jigsaws = append(jigsaws, structureJigsaw{
			pos:               block.Pos,
			front:             front,
			top:               top,
			joint:             joint,
			name:              normalizeIdentifierName(anyString(block.NBT["name"])),
			pool:              normalizeIdentifierName(anyString(block.NBT["pool"])),
			target:            normalizeIdentifierName(anyString(block.NBT["target"])),
			placementPriority: anyInt(block.NBT["placement_priority"]),
			selectionPriority: anyInt(block.NBT["selection_priority"]),
		})
	}
	return jigsaws
}

func parseJigsawOrientation(properties map[string]any) (structureDirection, structureDirection) {
	orientation := "north_up"
	if properties != nil {
		if value, ok := properties["orientation"]; ok {
			if s := strings.ToLower(anyString(value)); s != "" {
				orientation = s
			}
		}
	}
	parts := strings.SplitN(orientation, "_", 2)
	if len(parts) != 2 {
		return structureNorth, structureUp
	}
	return parseStructureDirection(parts[0]), parseStructureDirection(parts[1])
}

func parseJigsawJoint(nbt map[string]any, front structureDirection) string {
	if joint := strings.ToLower(anyString(nbt["joint"])); joint != "" {
		return joint
	}
	if front.isHorizontal() {
		return "aligned"
	}
	return "rollable"
}

func parseStructureDirection(value string) structureDirection {
	switch strings.ToLower(value) {
	case "down":
		return structureDown
	case "up":
		return structureUp
	case "south":
		return structureSouth
	case "west":
		return structureWest
	case "east":
		return structureEast
	default:
		return structureNorth
	}
}

func anyString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	default:
		return ""
	}
}

func anyInt(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

func normalizeIdentifierName(name string) string {
	if strings.HasPrefix(name, "minecraft:") {
		return name[len("minecraft:"):]
	}
	return name
}

func randomStructureRotation(rng *gen.Xoroshiro128) structureRotation {
	return structureRotation(rng.NextInt(4))
}

func fillShuffledStructureRotations(dst *[4]structureRotation, rng *gen.Xoroshiro128) {
	*dst = [4]structureRotation{
		structureRotationNone,
		structureRotationClockwise90,
		structureRotationClockwise180,
		structureRotationCounterclockwise90,
	}
	for i := len(dst) - 1; i > 0; i-- {
		j := int(rng.NextInt(uint32(i + 1)))
		dst[i], dst[j] = dst[j], dst[i]
	}
}

func rotateStructurePos(size, pos [3]int, rotation structureRotation) [3]int {
	switch rotation {
	case structureRotationClockwise90:
		return [3]int{size[2] - 1 - pos[2], pos[1], pos[0]}
	case structureRotationClockwise180:
		return [3]int{size[0] - 1 - pos[0], pos[1], size[2] - 1 - pos[2]}
	case structureRotationCounterclockwise90:
		return [3]int{pos[2], pos[1], size[0] - 1 - pos[0]}
	default:
		return pos
	}
}

func rotateStructureDirection(direction structureDirection, rotation structureRotation) structureDirection {
	switch direction {
	case structureNorth:
		switch rotation {
		case structureRotationClockwise90:
			return structureEast
		case structureRotationClockwise180:
			return structureSouth
		case structureRotationCounterclockwise90:
			return structureWest
		}
	case structureSouth:
		switch rotation {
		case structureRotationClockwise90:
			return structureWest
		case structureRotationClockwise180:
			return structureNorth
		case structureRotationCounterclockwise90:
			return structureEast
		}
	case structureWest:
		switch rotation {
		case structureRotationClockwise90:
			return structureNorth
		case structureRotationClockwise180:
			return structureEast
		case structureRotationCounterclockwise90:
			return structureSouth
		}
	case structureEast:
		switch rotation {
		case structureRotationClockwise90:
			return structureSouth
		case structureRotationClockwise180:
			return structureWest
		case structureRotationCounterclockwise90:
			return structureNorth
		}
	}
	return direction
}

func (d structureDirection) isHorizontal() bool {
	return d >= structureNorth
}

func (d structureDirection) stepX() int {
	switch d {
	case structureWest:
		return -1
	case structureEast:
		return 1
	default:
		return 0
	}
}

func (d structureDirection) stepY() int {
	switch d {
	case structureDown:
		return -1
	case structureUp:
		return 1
	default:
		return 0
	}
}

func (d structureDirection) stepZ() int {
	switch d {
	case structureNorth:
		return -1
	case structureSouth:
		return 1
	default:
		return 0
	}
}

func (d structureDirection) opposite() structureDirection {
	switch d {
	case structureDown:
		return structureUp
	case structureUp:
		return structureDown
	case structureNorth:
		return structureSouth
	case structureSouth:
		return structureNorth
	case structureWest:
		return structureEast
	default:
		return structureWest
	}
}

func rotatedStructureSize(size [3]int, rotation structureRotation) [3]int {
	switch rotation {
	case structureRotationClockwise90, structureRotationCounterclockwise90:
		return [3]int{size[2], size[1], size[0]}
	default:
		return size
	}
}

func (element resolvedPoolElement) worldBox(origin cube.Pos, rotation structureRotation) structureBox {
	if element.size[0] <= 0 || element.size[1] <= 0 || element.size[2] <= 0 {
		return emptyStructureBox()
	}
	size := rotatedStructureSize(element.size, rotation)
	return structureBox{
		minX: origin[0],
		minY: origin[1],
		minZ: origin[2],
		maxX: origin[0] + size[0] - 1,
		maxY: origin[1] + size[1] - 1,
		maxZ: origin[2] + size[2] - 1,
	}
}

func emptyStructureBox() structureBox {
	return structureBox{minX: 1, minY: 1, minZ: 1, maxX: 0, maxY: 0, maxZ: 0}
}

func (b structureBox) empty() bool {
	return b.maxX < b.minX || b.maxY < b.minY || b.maxZ < b.minZ
}

func (b structureBox) intersectsChunk(chunkX, chunkZ, minY, maxY int) bool {
	if b.empty() {
		return false
	}
	minBlockX := chunkX * 16
	maxBlockX := minBlockX + 15
	minBlockZ := chunkZ * 16
	maxBlockZ := minBlockZ + 15
	return !(b.maxX < minBlockX || b.minX > maxBlockX || b.maxZ < minBlockZ || b.minZ > maxBlockZ || b.maxY < minY || b.minY > maxY)
}

func (b structureBox) intersects(other structureBox) bool {
	if b.empty() || other.empty() {
		return false
	}
	return !(b.maxX < other.minX || b.minX > other.maxX || b.maxY < other.minY || b.minY > other.maxY || b.maxZ < other.minZ || b.minZ > other.maxZ)
}

func (b structureBox) containsPos(pos cube.Pos) bool {
	if b.empty() {
		return false
	}
	return pos[0] >= b.minX && pos[0] <= b.maxX &&
		pos[1] >= b.minY && pos[1] <= b.maxY &&
		pos[2] >= b.minZ && pos[2] <= b.maxZ
}

func (b structureBox) ySpan() int {
	if b.empty() {
		return 0
	}
	return b.maxY - b.minY + 1
}

func unionStructureBoxes(a, b structureBox) structureBox {
	if a.empty() {
		return b
	}
	if b.empty() {
		return a
	}
	return structureBox{
		minX: min(a.minX, b.minX),
		minY: min(a.minY, b.minY),
		minZ: min(a.minZ, b.minZ),
		maxX: max(a.maxX, b.maxX),
		maxY: max(a.maxY, b.maxY),
		maxZ: max(a.maxZ, b.maxZ),
	}
}

func (b structureBox) originAndSize() (cube.Pos, [3]int) {
	if b.empty() {
		return cube.Pos{}, [3]int{}
	}
	return cube.Pos{b.minX, b.minY, b.minZ}, [3]int{b.maxX - b.minX + 1, b.maxY - b.minY + 1, b.maxZ - b.minZ + 1}
}

func newStructureCollisionSpace(allowed structureBox) *structureCollisionSpace {
	return &structureCollisionSpace{allowed: allowed}
}

func (s *structureCollisionSpace) canFit(box structureBox) bool {
	if box.empty() {
		return true
	}
	if s == nil || s.allowed.empty() || !boxDeflatedWithin(box, s.allowed) {
		return false
	}
	for _, cutout := range s.cutouts {
		if boxDeflatedIntersects(box, cutout) {
			return false
		}
	}
	return true
}

func (s *structureCollisionSpace) occupy(box structureBox) {
	if s == nil || box.empty() {
		return
	}
	s.cutouts = append(s.cutouts, box)
}

func boxDeflatedWithin(box, allowed structureBox) bool {
	if box.empty() {
		return true
	}
	if allowed.empty() {
		return false
	}
	minX, maxX := deflatedBoxQuarterBounds(box.minX, box.maxX)
	minY, maxY := deflatedBoxQuarterBounds(box.minY, box.maxY)
	minZ, maxZ := deflatedBoxQuarterBounds(box.minZ, box.maxZ)
	return minX >= allowed.minX*4 &&
		maxX <= (allowed.maxX+1)*4 &&
		minY >= allowed.minY*4 &&
		maxY <= (allowed.maxY+1)*4 &&
		minZ >= allowed.minZ*4 &&
		maxZ <= (allowed.maxZ+1)*4
}

func boxDeflatedIntersects(box, other structureBox) bool {
	if box.empty() || other.empty() {
		return false
	}
	boxMinX, boxMaxX := deflatedBoxQuarterBounds(box.minX, box.maxX)
	boxMinY, boxMaxY := deflatedBoxQuarterBounds(box.minY, box.maxY)
	boxMinZ, boxMaxZ := deflatedBoxQuarterBounds(box.minZ, box.maxZ)
	otherMinX, otherMaxX := fullBoxQuarterBounds(other.minX, other.maxX)
	otherMinY, otherMaxY := fullBoxQuarterBounds(other.minY, other.maxY)
	otherMinZ, otherMaxZ := fullBoxQuarterBounds(other.minZ, other.maxZ)
	return boxMinX < otherMaxX && boxMaxX > otherMinX &&
		boxMinY < otherMaxY && boxMaxY > otherMinY &&
		boxMinZ < otherMaxZ && boxMaxZ > otherMinZ
}

func deflatedBoxQuarterBounds(minValue, maxValue int) (int, int) {
	return minValue*4 + 1, maxValue*4 + 3
}

func fullBoxQuarterBounds(minValue, maxValue int) (int, int) {
	return minValue * 4, (maxValue + 1) * 4
}

func shuffleWithRNG[T any](values []T, rng *gen.Xoroshiro128) {
	for i := len(values) - 1; i > 0; i-- {
		j := int(rng.NextInt(uint32(i + 1)))
		values[i], values[j] = values[j], values[i]
	}
}

func precomputeResolvedPoolElementJigsaws(element resolvedPoolElement) resolvedPoolElement {
	element.rotated = precomputePlacedJigsaws(element.size, element.jigsaws)
	return element
}

func precomputePlacedJigsaws(size [3]int, jigsaws []structureJigsaw) [4]precomputedPlacedJigsaws {
	var out [4]precomputedPlacedJigsaws
	if len(jigsaws) == 0 {
		return out
	}
	for i := range out {
		rotation := structureRotation(i)
		values := make([]placedStructureJigsaw, len(jigsaws))
		for j, jigsaw := range jigsaws {
			rotated := rotateStructurePos(size, jigsaw.pos, rotation)
			values[j] = placedStructureJigsaw{
				pos: cube.Pos{
					rotated[0],
					rotated[1],
					rotated[2],
				},
				localY:            rotated[1],
				front:             rotateStructureDirection(jigsaw.front, rotation),
				top:               rotateStructureDirection(jigsaw.top, rotation),
				joint:             jigsaw.joint,
				name:              jigsaw.name,
				pool:              jigsaw.pool,
				target:            jigsaw.target,
				placementPriority: jigsaw.placementPriority,
				selectionPriority: jigsaw.selectionPriority,
			}
		}
		sort.Slice(values, func(a, b int) bool {
			return values[a].selectionPriority > values[b].selectionPriority
		})
		groups := make([]jigsawPriorityGroup, 0, len(values))
		for start := 0; start < len(values); {
			end := start + 1
			for end < len(values) && values[end].selectionPriority == values[start].selectionPriority {
				end++
			}
			groups = append(groups, jigsawPriorityGroup{start: start, end: end})
			start = end
		}
		out[i] = precomputedPlacedJigsaws{values: values, groups: groups}
	}
	return out
}

func (element resolvedPoolElement) appendShuffledJigsaws(dst []placedStructureJigsaw, origin cube.Pos, rotation structureRotation, rng *gen.Xoroshiro128) []placedStructureJigsaw {
	precomputed := element.rotated[rotation]
	if len(precomputed.values) == 0 {
		precomputed = precomputePlacedJigsaws(element.size, element.jigsaws)[rotation]
	}
	start := len(dst)
	dst = slices.Grow(dst, len(precomputed.values))
	dst = dst[:start+len(precomputed.values)]
	if origin == (cube.Pos{}) {
		copy(dst[start:], precomputed.values)
	} else {
		for i, jigsaw := range precomputed.values {
			placed := jigsaw
			placed.pos = cube.Pos{
				origin[0] + jigsaw.pos[0],
				origin[1] + jigsaw.pos[1],
				origin[2] + jigsaw.pos[2],
			}
			dst[start+i] = placed
		}
	}
	ordered := dst[start:]
	for _, group := range precomputed.groups {
		shuffleWithRNG(ordered[group.start:group.end], rng)
	}
	return dst
}

func stableSortJigsawsBySelectionPriority(jigsaws []placedStructureJigsaw) {
	for i := 1; i < len(jigsaws); i++ {
		current := jigsaws[i]
		j := i
		for j > 0 && jigsaws[j-1].selectionPriority < current.selectionPriority {
			jigsaws[j] = jigsaws[j-1]
			j--
		}
		jigsaws[j] = current
	}
}

func appendShuffledPoolElements(dst []resolvedPoolElement, pool resolvedStructurePool, rng *gen.Xoroshiro128) []resolvedPoolElement {
	start := len(dst)
	dst = slices.Grow(dst, len(pool.weighted))
	dst = dst[:start+len(pool.weighted)]
	for i, index := range pool.weighted {
		if index >= 0 && index < len(pool.entries) {
			dst[start+i] = pool.entries[index]
		}
	}
	shuffleWithRNG(dst[start:], rng)
	return dst
}

func canAttachJigsaws(source, target placedStructureJigsaw) bool {
	return source.front == target.front.opposite() &&
		(source.joint == "rollable" || source.top == target.top) &&
		source.target == target.name
}

func insertPendingPiece(queue []pendingStructurePiece, pending pendingStructurePiece) []pendingStructurePiece {
	index := len(queue)
	for i, existing := range queue {
		if pending.priority > existing.priority {
			index = i
			break
		}
	}
	queue = append(queue, pendingStructurePiece{})
	copy(queue[index+1:], queue[index:])
	queue[index] = pending
	return queue
}

type structurePoolAliasMapping map[string]string

type directPoolAliasDef struct {
	Alias  string `json:"alias"`
	Target string `json:"target"`
}

type weightedTargetDef struct {
	Data   string `json:"data"`
	Weight int    `json:"weight"`
}

type randomPoolAliasDef struct {
	Alias   string              `json:"alias"`
	Targets []weightedTargetDef `json:"targets"`
}

type weightedAliasGroupDef struct {
	Data   []gen.PoolAliasDef `json:"data"`
	Weight int                `json:"weight"`
}

type randomGroupPoolAliasDef struct {
	Groups []weightedAliasGroupDef `json:"groups"`
}

func resolveStructurePoolAliases(defs []gen.PoolAliasDef, pos cube.Pos, seed int64) structurePoolAliasMapping {
	if len(defs) == 0 {
		return nil
	}
	rng := gen.NewPositionalRandomFactory(seed).At(pos[0], pos[1], pos[2])
	out := make(structurePoolAliasMapping, len(defs))
	for _, def := range defs {
		applyStructurePoolAlias(out, def, &rng)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func applyStructurePoolAlias(out structurePoolAliasMapping, def gen.PoolAliasDef, rng *gen.Xoroshiro128) {
	switch def.Type {
	case "direct":
		var raw directPoolAliasDef
		if err := json.Unmarshal(def.Raw, &raw); err != nil {
			return
		}
		alias := normalizeIdentifierName(raw.Alias)
		target := normalizeIdentifierName(raw.Target)
		if alias != "" && target != "" {
			out[alias] = target
		}
	case "random":
		var raw randomPoolAliasDef
		if err := json.Unmarshal(def.Raw, &raw); err != nil {
			return
		}
		alias := normalizeIdentifierName(raw.Alias)
		target := normalizeIdentifierName(weightedStringChoice(raw.Targets, rng))
		if alias != "" && target != "" {
			out[alias] = target
		}
	case "random_group":
		var raw randomGroupPoolAliasDef
		if err := json.Unmarshal(def.Raw, &raw); err != nil {
			return
		}
		group := weightedAliasGroupChoice(raw.Groups, rng)
		for _, inner := range group {
			applyStructurePoolAlias(out, inner, rng)
		}
	}
}

func weightedStringChoice(entries []weightedTargetDef, rng *gen.Xoroshiro128) string {
	total := 0
	for _, entry := range entries {
		if entry.Weight > 0 {
			total += entry.Weight
		}
	}
	if total <= 0 {
		return ""
	}
	pick := int(rng.NextInt(uint32(total)))
	for _, entry := range entries {
		if entry.Weight <= 0 {
			continue
		}
		if pick < entry.Weight {
			return entry.Data
		}
		pick -= entry.Weight
	}
	return ""
}

func weightedAliasGroupChoice(entries []weightedAliasGroupDef, rng *gen.Xoroshiro128) []gen.PoolAliasDef {
	total := 0
	for _, entry := range entries {
		if entry.Weight > 0 {
			total += entry.Weight
		}
	}
	if total <= 0 {
		return nil
	}
	pick := int(rng.NextInt(uint32(total)))
	for _, entry := range entries {
		if entry.Weight <= 0 {
			continue
		}
		if pick < entry.Weight {
			return entry.Data
		}
		pick -= entry.Weight
	}
	return nil
}

func (m structurePoolAliasMapping) lookup(name string) string {
	key := normalizeIdentifierName(name)
	if len(m) == 0 {
		return key
	}
	if target, ok := m[key]; ok && target != "" {
		return target
	}
	return key
}

func structureCollisionRangeBox(centerX, centerY, centerZ, maxDistance, minY, maxY, padding int) structureBox {
	if maxDistance <= 0 {
		return emptyStructureBox()
	}
	return structureBox{
		minX: centerX - maxDistance,
		minY: max(centerY-maxDistance, minY+padding),
		minZ: centerZ - maxDistance,
		maxX: centerX + maxDistance,
		maxY: min(centerY+maxDistance, maxY-padding),
		maxZ: centerZ + maxDistance,
	}
}

func structureFitsDimensionPadding(box structureBox, minY, maxY, padding int) bool {
	if box.empty() || padding <= 0 {
		return true
	}
	return box.minY >= minY+padding && box.maxY <= maxY-padding
}

func expandStructureBoxForJigsawHack(box structureBox, expandTo int) structureBox {
	if box.empty() || expandTo <= 0 {
		return box
	}
	newSize := max(expandTo+1, box.maxY-box.minY)
	if expandedMaxY := box.minY + newSize; expandedMaxY > box.maxY {
		box.maxY = expandedMaxY
	}
	return box
}

func resolvedStructurePoolMaxYSpan(pool resolvedStructurePool) int {
	maxSpan := 0
	for _, entry := range pool.entries {
		if entry.elementType == "empty_pool_element" {
			continue
		}
		if entry.size[1] > maxSpan {
			maxSpan = entry.size[1]
		}
	}
	return maxSpan
}

func (g Generator) structureExpansionHackY(hackBox structureBox, jigsaws []placedStructureJigsaw, aliasLookup structurePoolAliasMapping) int {
	if hackBox.empty() || hackBox.ySpan() > 16 || g.structureResolver == nil {
		return 0
	}
	expandTo := 0
	for _, jigsaw := range jigsaws {
		frontPos := jigsaw.pos.Add(cube.Pos{jigsaw.front.stepX(), jigsaw.front.stepY(), jigsaw.front.stepZ()})
		if !hackBox.containsPos(frontPos) {
			continue
		}
		pool, err := g.structureResolver.Pool(aliasLookup.lookup(jigsaw.pool))
		if err != nil {
			continue
		}
		childPoolSize := resolvedStructurePoolMaxYSpan(pool)
		childFallbackSize := 0
		if pool.fallback != "" {
			if fallback, err := g.structureResolver.Pool(aliasLookup.lookup(pool.fallback)); err == nil {
				childFallbackSize = resolvedStructurePoolMaxYSpan(fallback)
			}
		}
		if childPoolSize > expandTo {
			expandTo = childPoolSize
		}
		if childFallbackSize > expandTo {
			expandTo = childFallbackSize
		}
	}
	return expandTo
}

func (g Generator) buildPlannedStructure(
	candidate structurePlannerCandidate,
	start weightedStartTemplate,
	startX, startZ int,
	surfaceSampler *structureHeightSampler,
	rng *gen.Xoroshiro128,
) ([]plannedStructurePiece, structureBox, cube.Pos, [3]int, bool) {
	rootElement := precomputeResolvedPoolElementJigsaws(resolvedPoolElement{
		elementType: "start",
		projection:  start.projection,
		placements: []structureTemplatePlacement{{
			templateName: start.name,
			ignoreAir:    start.ignoreAir,
			processors:   start.processors,
		}},
		jigsaws: start.jigsaws,
		size:    start.size,
	})
	if len(rootElement.placements) == 0 {
		return nil, emptyStructureBox(), cube.Pos{}, [3]int{}, false
	}

	rootRotation := randomStructureRotation(rng)
	anchor := [3]int{}
	if candidate.jigsaw.StartJigsawName != "" {
		targetName := normalizeIdentifierName(candidate.jigsaw.StartJigsawName)
		found := false
		var rootJigsaws []placedStructureJigsaw
		rootJigsaws = rootElement.appendShuffledJigsaws(rootJigsaws[:0], cube.Pos{}, rootRotation, rng)
		for _, jigsaw := range rootJigsaws {
			if jigsaw.name == targetName {
				anchor = [3]int{jigsaw.pos[0], jigsaw.pos[1], jigsaw.pos[2]}
				found = true
				break
			}
		}
		if !found {
			return nil, emptyStructureBox(), cube.Pos{}, [3]int{}, false
		}
	}

	rootOrigin := cube.Pos{startX - anchor[0], 0, startZ - anchor[2]}
	rootBox := rootElement.worldBox(rootOrigin, rootRotation)
	centerX := (rootBox.minX + rootBox.maxX) / 2
	centerZ := (rootBox.minZ + rootBox.maxZ) / 2
	surfaceLevelAt := func(x, z int) int {
		return surfaceSampler.worldSurfaceLevelAt(x, z)
	}
	baseY := g.sampleStructureHeightProvider(candidate.jigsaw.StartHeight, surfaceSampler.minY, surfaceSampler.maxY, rng)
	if candidate.jigsaw.ProjectStartToHeight != "" {
		baseY += surfaceLevelAt(centerX, centerZ)
	}
	rootOrigin[1] = baseY - 1
	rootBox = rootElement.worldBox(rootOrigin, rootRotation)
	if !structureFitsDimensionPadding(rootBox, surfaceSampler.minY, surfaceSampler.maxY, candidate.jigsaw.DimensionPadding) {
		return nil, emptyStructureBox(), cube.Pos{}, [3]int{}, false
	}

	rootPiece := plannedStructurePiece{
		element:          rootElement,
		origin:           rootOrigin,
		rotation:         rootRotation,
		bounds:           rootBox,
		groundLevelDelta: structurePoolElementGroundLevelDelta(rootElement),
		rootPiece:        true,
	}

	pieces := []plannedStructurePiece{rootPiece}
	overall := rootBox

	if candidate.jigsaw.Size <= 0 {
		return pieces, overall, rootOrigin, rotatedStructureSize(rootElement.size, rootRotation), true
	}

	contextSpace := newStructureCollisionSpace(structureCollisionRangeBox(
		centerX,
		baseY+anchor[1],
		centerZ,
		candidate.jigsaw.MaxDistanceFromCenter,
		surfaceSampler.minY,
		surfaceSampler.maxY,
		candidate.jigsaw.DimensionPadding,
	))
	if contextSpace.allowed.empty() {
		return nil, emptyStructureBox(), cube.Pos{}, [3]int{}, false
	}
	contextSpace.occupy(rootBox)
	queue := []pendingStructurePiece{{piece: rootPiece, pieceIndex: 0, depth: 0, space: contextSpace}}
	aliasLookup := resolveStructurePoolAliases(candidate.jigsaw.PoolAliases, cube.Pos{startX, baseY, startZ}, g.seed)
	var (
		candidates      []resolvedPoolElement
		sourceJigsaws   []placedStructureJigsaw
		targetJigsaws   []placedStructureJigsaw
		targetRotations [4]structureRotation
	)
	for len(queue) > 0 {
		state := queue[0]
		queue = queue[1:]
		if state.depth >= candidate.jigsaw.Size {
			continue
		}

		sourceRigid := normalizeIdentifierName(state.piece.element.projection) == "rigid"
		sourceSurfaceY := 0
		sourceSurfaceLoaded := false
		var sourceSpace *structureCollisionSpace
		sourceJigsaws = state.piece.element.appendShuffledJigsaws(sourceJigsaws[:0], state.piece.origin, state.piece.rotation, rng)

	sourceJigsawLoop:
		for _, sourceJigsaw := range sourceJigsaws {
			pool, err := g.structureResolver.Pool(aliasLookup.lookup(sourceJigsaw.pool))
			if err != nil {
				continue
			}

			candidates = candidates[:0]
			if state.depth != candidate.jigsaw.Size {
				candidates = appendShuffledPoolElements(candidates, pool, rng)
			}
			if pool.fallback != "" {
				fallback, err := g.structureResolver.Pool(aliasLookup.lookup(pool.fallback))
				if err == nil {
					candidates = appendShuffledPoolElements(candidates, fallback, rng)
				}
			}

			for _, targetElement := range candidates {
				if targetElement.elementType == "empty_pool_element" {
					break
				}
				fillShuffledStructureRotations(&targetRotations, rng)
				for _, targetRotation := range targetRotations {
					targetJigsaws = targetElement.appendShuffledJigsaws(targetJigsaws[:0], cube.Pos{}, targetRotation, rng)
					targetRigid := normalizeIdentifierName(targetElement.projection) == "rigid"
					expandTo := 0
					if candidate.jigsaw.UseExpansionHack {
						expandTo = g.structureExpansionHackY(targetElement.worldBox(cube.Pos{}, targetRotation), targetJigsaws, aliasLookup)
					}

					for _, targetJigsaw := range targetJigsaws {
						if !canAttachJigsaws(sourceJigsaw, targetJigsaw) {
							continue
						}

						targetPos := sourceJigsaw.pos.Add(cube.Pos{
							sourceJigsaw.front.stepX(),
							sourceJigsaw.front.stepY(),
							sourceJigsaw.front.stepZ(),
						})
						targetOrigin := cube.Pos{
							targetPos[0] - targetJigsaw.pos[0],
							targetPos[1] - targetJigsaw.pos[1],
							targetPos[2] - targetJigsaw.pos[2],
						}

						deltaY := sourceJigsaw.localY - targetJigsaw.localY + sourceJigsaw.front.stepY()
						if sourceRigid && targetRigid {
							targetOrigin[1] = state.piece.bounds.minY + deltaY
						} else {
							if !sourceSurfaceLoaded {
								sourceSurfaceY = surfaceLevelAt(sourceJigsaw.pos[0], sourceJigsaw.pos[2])
								sourceSurfaceLoaded = true
							}
							targetOrigin[1] = sourceSurfaceY - targetJigsaw.localY
						}

						targetBox := targetElement.worldBox(targetOrigin, targetRotation)
						targetCollisionBox := expandStructureBoxForJigsawHack(targetBox, expandTo)
						childrenSpace := state.space
						if state.piece.bounds.containsPos(targetPos) {
							if sourceSpace == nil {
								sourceSpace = newStructureCollisionSpace(state.piece.bounds)
							}
							childrenSpace = sourceSpace
						}
						if !childrenSpace.canFit(targetCollisionBox) {
							continue
						}

						targetPiece := plannedStructurePiece{
							element:          targetElement,
							origin:           targetOrigin,
							rotation:         targetRotation,
							bounds:           targetCollisionBox,
							groundLevelDelta: structurePoolElementGroundLevelDelta(targetElement),
						}
						if targetRigid {
							targetPiece.groundLevelDelta = state.piece.groundLevelDelta - deltaY
						}
						sharedGroundY := sourceSurfaceY + deltaY/2
						if sourceRigid {
							sharedGroundY = state.piece.bounds.minY + sourceJigsaw.localY
						} else if targetRigid {
							sharedGroundY = targetPiece.bounds.minY + targetJigsaw.localY
						}
						state.piece.junctions = append(state.piece.junctions, plannedStructureJunction{
							sourceX:       targetPos[0],
							sourceGroundY: sharedGroundY - sourceJigsaw.localY + state.piece.groundLevelDelta,
							sourceZ:       targetPos[2],
						})
						targetPiece.junctions = append(targetPiece.junctions, plannedStructureJunction{
							sourceX:       sourceJigsaw.pos[0],
							sourceGroundY: sharedGroundY - targetJigsaw.localY + targetPiece.groundLevelDelta,
							sourceZ:       sourceJigsaw.pos[2],
						})
						pieces[state.pieceIndex] = state.piece
						pieces = append(pieces, targetPiece)
						targetPieceIndex := len(pieces) - 1
						childrenSpace.occupy(targetCollisionBox)
						if !targetPiece.bounds.empty() {
							overall = unionStructureBoxes(overall, targetPiece.bounds)
						}
						if len(targetElement.jigsaws) != 0 && state.depth+1 <= candidate.jigsaw.Size {
							queue = insertPendingPiece(queue, pendingStructurePiece{
								piece:      targetPiece,
								pieceIndex: targetPieceIndex,
								depth:      state.depth + 1,
								priority:   sourceJigsaw.placementPriority,
								space:      childrenSpace,
							})
						}
						continue sourceJigsawLoop
					}
				}
			}
		}
	}

	rootSize := rotatedStructureSize(rootElement.size, rootRotation)
	return pieces, overall, rootOrigin, rootSize, true
}

func structurePoolElementGroundLevelDelta(resolvedPoolElement) int {
	return 1
}
