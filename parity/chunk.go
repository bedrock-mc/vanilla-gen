package parity

import (
	"math/bits"
	"sort"
	"strings"
)

const (
	MinY        = -64
	MaxY        = 319
	WorldHeight = MaxY - MinY + 1
	minSectionY = MinY >> 4
)

type JavaChunk struct {
	DataVersion int32
	Status      string
	sections    map[int]*chunkSection
	heightmaps  map[string][]int64
}

type chunkSection struct {
	blockPalette []string
	blockData    []int64
	blockBits    int
	biomePalette []string
	biomeData    []int64
	biomeBits    int
}

func newJavaChunk(root map[string]any) (*JavaChunk, error) {
	c := &JavaChunk{
		sections:   make(map[int]*chunkSection),
		heightmaps: make(map[string][]int64),
	}
	c.DataVersion, _ = root["DataVersion"].(int32)
	c.Status, _ = root["Status"].(string)

	sections, _ := root["sections"].([]any)
	for _, s := range sections {
		sec, _ := s.(map[string]any)
		if sec == nil {
			continue
		}
		y, ok := sec["Y"].(int8)
		if !ok {
			continue
		}
		cs := &chunkSection{}
		if bs, _ := sec["block_states"].(map[string]any); bs != nil {
			for _, entry := range bs["palette"].([]any) {
				cs.blockPalette = append(cs.blockPalette, canonicalBlockState(entry.(map[string]any)))
			}
			cs.blockData, _ = bs["data"].([]int64)
			cs.blockBits = max(4, ceilLog2(len(cs.blockPalette)))
			if len(cs.blockPalette) == 1 {
				cs.blockBits = 0
			}
		}
		if bio, _ := sec["biomes"].(map[string]any); bio != nil {
			for _, entry := range bio["palette"].([]any) {
				cs.biomePalette = append(cs.biomePalette, entry.(string))
			}
			cs.biomeData, _ = bio["data"].([]int64)
			cs.biomeBits = ceilLog2(len(cs.biomePalette))
		}
		c.sections[int(y)] = cs
	}

	if hm, _ := root["Heightmaps"].(map[string]any); hm != nil {
		for name, v := range hm {
			if data, ok := v.([]int64); ok {
				c.heightmaps[name] = data
			}
		}
	}
	return c, nil
}

func canonicalBlockState(entry map[string]any) string {
	name, _ := entry["Name"].(string)
	props, _ := entry["Properties"].(map[string]any)
	if len(props) == 0 {
		return name
	}
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	sb.WriteString(name)
	sb.WriteByte('[')
	for i, k := range keys {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(k)
		sb.WriteByte('=')
		sb.WriteString(props[k].(string))
	}
	sb.WriteByte(']')
	return sb.String()
}

func ceilLog2(n int) int {
	if n <= 1 {
		return 0
	}
	return bits.Len(uint(n - 1))
}

func palettedGet(palette []string, data []int64, bitsPer, idx int) string {
	if bitsPer == 0 || len(data) == 0 {
		return palette[0]
	}
	perLong := 64 / bitsPer
	l := uint64(data[idx/perLong])
	v := int(l >> (uint(idx%perLong) * uint(bitsPer)) & (1<<uint(bitsPer) - 1))
	if v >= len(palette) {
		return palette[0]
	}
	return palette[v]
}

func (c *JavaChunk) BlockState(x, y, z int) string {
	if y < MinY || y > MaxY {
		return "minecraft:air"
	}
	sec := c.sections[y>>4]
	if sec == nil || len(sec.blockPalette) == 0 {
		return "minecraft:air"
	}
	idx := (y&15)<<8 | (z&15)<<4 | x&15
	return palettedGet(sec.blockPalette, sec.blockData, sec.blockBits, idx)
}

func (c *JavaChunk) Biome(quartX, quartY, quartZ int) string {
	sec := c.sections[quartY>>2]
	if sec == nil || len(sec.biomePalette) == 0 {
		return ""
	}
	idx := (quartY&3)<<4 | (quartZ&3)<<2 | quartX&3
	return palettedGet(sec.biomePalette, sec.biomeData, sec.biomeBits, idx)
}

func (c *JavaChunk) Heightmaps() map[string][]int {
	bitsPer := ceilLog2(WorldHeight + 1)
	perLong := 64 / bitsPer
	mask := int64(1)<<uint(bitsPer) - 1
	out := make(map[string][]int, 2)
	for _, name := range []string{"WORLD_SURFACE", "OCEAN_FLOOR"} {
		data, ok := c.heightmaps[name]
		if !ok {
			continue
		}
		vals := make([]int, 256)
		for i := range vals {
			if i/perLong < len(data) {
				vals[i] = int(data[i/perLong] >> (uint(i%perLong) * uint(bitsPer)) & mask)
			}
		}
		out[name] = vals
	}
	return out
}

func (c *JavaChunk) SectionBlockPalette(sectionY int) []string {
	sec := c.sections[sectionY]
	if sec == nil {
		return nil
	}
	return append([]string(nil), sec.blockPalette...)
}
