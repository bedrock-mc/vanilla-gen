package gen

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

const (
	structureNBTTagEnd byte = iota
	structureNBTTagByte
	structureNBTTagShort
	structureNBTTagInt
	structureNBTTagLong
	structureNBTTagFloat
	structureNBTTagDouble
	structureNBTTagByteArray
	structureNBTTagString
	structureNBTTagList
	structureNBTTagCompound
	structureNBTTagIntArray
	structureNBTTagLongArray
)

const structureTemplateMaxDepth = 4096

type structureTemplateNBTReader struct {
	r io.Reader
}

func decodeStructureTemplateFallback(data []byte) (StructureTemplate, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return StructureTemplate{}, err
	}
	defer reader.Close()

	nbtReader := structureTemplateNBTReader{r: reader}
	rootTag, err := nbtReader.readByte()
	if err != nil {
		return StructureTemplate{}, err
	}
	if rootTag != structureNBTTagCompound {
		return StructureTemplate{}, fmt.Errorf("expected root compound tag, got %d", rootTag)
	}
	if _, err := nbtReader.readString(); err != nil {
		return StructureTemplate{}, err
	}
	return nbtReader.readStructureTemplate(0)
}

func (r structureTemplateNBTReader) readStructureTemplate(depth int) (StructureTemplate, error) {
	if depth >= structureTemplateMaxDepth {
		return StructureTemplate{}, fmt.Errorf("structure template NBT exceeded depth %d", structureTemplateMaxDepth)
	}

	var out StructureTemplate
	paletteLoaded := false
	for {
		nextTag, err := r.readByte()
		if err != nil {
			return StructureTemplate{}, err
		}
		if nextTag == structureNBTTagEnd {
			return out, nil
		}
		name, err := r.readString()
		if err != nil {
			return StructureTemplate{}, err
		}

		switch name {
		case "size":
			out.Size, err = r.readStructureTemplateSize(nextTag, depth+1)
		case "palette":
			out.Palette, err = r.readStructureTemplatePalette(nextTag, depth+1)
			paletteLoaded = len(out.Palette) != 0
		case "palettes":
			if paletteLoaded {
				err = r.skipTagPayload(nextTag, depth+1)
				break
			}
			out.Palette, err = r.readStructureTemplatePalettes(nextTag, depth+1)
			paletteLoaded = len(out.Palette) != 0
		case "blocks":
			out.Blocks, err = r.readStructureTemplateBlocks(nextTag, depth+1)
		default:
			err = r.skipTagPayload(nextTag, depth+1)
		}
		if err != nil {
			return StructureTemplate{}, err
		}
	}
}

func (r structureTemplateNBTReader) readStructureTemplateSize(tag byte, depth int) ([3]int, error) {
	var out [3]int

	values, ok, err := r.readFixedIntVector3(tag, depth)
	if err != nil {
		return out, err
	}
	if ok {
		out = values
	}
	return out, nil
}

func (r structureTemplateNBTReader) readStructureTemplatePalette(tag byte, depth int) ([]StructureTemplateBlockState, error) {
	return r.readBlockStateListPayload(tag, depth)
}

func (r structureTemplateNBTReader) readStructureTemplatePalettes(tag byte, depth int) ([]StructureTemplateBlockState, error) {
	if depth >= structureTemplateMaxDepth {
		return nil, fmt.Errorf("structure template NBT exceeded depth %d", structureTemplateMaxDepth)
	}
	if tag != structureNBTTagList {
		if err := r.skipTagPayload(tag, depth); err != nil {
			return nil, err
		}
		return nil, nil
	}

	elemTag, length, err := r.readListHeader()
	if err != nil {
		return nil, err
	}
	var palette []StructureTemplateBlockState
	for i := int32(0); i < length; i++ {
		if i == 0 {
			palette, err = r.readBlockStateListPayload(elemTag, depth+1)
		} else {
			err = r.skipTagPayload(elemTag, depth+1)
		}
		if err != nil {
			return nil, err
		}
	}
	return palette, nil
}

func (r structureTemplateNBTReader) readStructureTemplateBlocks(tag byte, depth int) ([]StructureTemplateBlock, error) {
	if depth >= structureTemplateMaxDepth {
		return nil, fmt.Errorf("structure template NBT exceeded depth %d", structureTemplateMaxDepth)
	}
	if tag != structureNBTTagList {
		if err := r.skipTagPayload(tag, depth); err != nil {
			return nil, err
		}
		return nil, nil
	}

	elemTag, length, err := r.readListHeader()
	if err != nil {
		return nil, err
	}
	blocks := make([]StructureTemplateBlock, 0, length)
	for i := int32(0); i < length; i++ {
		block, ok, err := r.readStructureTemplateBlockPayload(elemTag, depth+1)
		if err != nil {
			return nil, err
		}
		if ok {
			blocks = append(blocks, block)
		}
	}
	return blocks, nil
}

func (r structureTemplateNBTReader) readBlockStateListPayload(tag byte, depth int) ([]StructureTemplateBlockState, error) {
	if depth >= structureTemplateMaxDepth {
		return nil, fmt.Errorf("structure template NBT exceeded depth %d", structureTemplateMaxDepth)
	}
	if tag != structureNBTTagList {
		if err := r.skipTagPayload(tag, depth); err != nil {
			return nil, err
		}
		return nil, nil
	}

	elemTag, length, err := r.readListHeader()
	if err != nil {
		return nil, err
	}
	states := make([]StructureTemplateBlockState, 0, length)
	for i := int32(0); i < length; i++ {
		state, err := r.readStructureTemplateBlockStatePayload(elemTag, depth+1)
		if err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	return states, nil
}

func (r structureTemplateNBTReader) readStructureTemplateBlockStatePayload(tag byte, depth int) (StructureTemplateBlockState, error) {
	if depth >= structureTemplateMaxDepth {
		return StructureTemplateBlockState{}, fmt.Errorf("structure template NBT exceeded depth %d", structureTemplateMaxDepth)
	}
	if tag != structureNBTTagCompound {
		if err := r.skipTagPayload(tag, depth); err != nil {
			return StructureTemplateBlockState{}, err
		}
		return StructureTemplateBlockState{}, nil
	}

	var out StructureTemplateBlockState
	for {
		nextTag, err := r.readByte()
		if err != nil {
			return StructureTemplateBlockState{}, err
		}
		if nextTag == structureNBTTagEnd {
			return out, nil
		}
		name, err := r.readString()
		if err != nil {
			return StructureTemplateBlockState{}, err
		}

		switch name {
		case "Name":
			if nextTag != structureNBTTagString {
				if err := r.skipTagPayload(nextTag, depth+1); err != nil {
					return StructureTemplateBlockState{}, err
				}
				continue
			}
			out.Name, err = r.readString()
		case "Properties":
			value, readErr := r.readTagPayload(nextTag, depth+1)
			if readErr != nil {
				return StructureTemplateBlockState{}, readErr
			}
			if properties, ok := value.(map[string]any); ok && len(properties) != 0 {
				out.Properties = properties
			}
		default:
			err = r.skipTagPayload(nextTag, depth+1)
		}
		if err != nil {
			return StructureTemplateBlockState{}, err
		}
	}
}

func (r structureTemplateNBTReader) readStructureTemplateBlockPayload(tag byte, depth int) (StructureTemplateBlock, bool, error) {
	if depth >= structureTemplateMaxDepth {
		return StructureTemplateBlock{}, false, fmt.Errorf("structure template NBT exceeded depth %d", structureTemplateMaxDepth)
	}
	if tag != structureNBTTagCompound {
		if err := r.skipTagPayload(tag, depth); err != nil {
			return StructureTemplateBlock{}, false, err
		}
		return StructureTemplateBlock{}, false, nil
	}

	var (
		out    StructureTemplateBlock
		posSet bool
	)
	for {
		nextTag, err := r.readByte()
		if err != nil {
			return StructureTemplateBlock{}, false, err
		}
		if nextTag == structureNBTTagEnd {
			return out, posSet, nil
		}
		name, err := r.readString()
		if err != nil {
			return StructureTemplateBlock{}, false, err
		}

		switch name {
		case "pos":
			values, ok, readErr := r.readFixedIntVector3(nextTag, depth+1)
			if readErr != nil {
				return StructureTemplateBlock{}, false, readErr
			}
			if ok {
				out.Pos = values
				posSet = true
			}
		case "state":
			out.State, err = r.readIntPayload(nextTag)
		case "nbt":
			value, readErr := r.readTagPayload(nextTag, depth+1)
			if readErr != nil {
				return StructureTemplateBlock{}, false, readErr
			}
			if nbt, ok := value.(map[string]any); ok && len(nbt) != 0 {
				out.NBT = nbt
			}
		default:
			err = r.skipTagPayload(nextTag, depth+1)
		}
		if err != nil {
			return StructureTemplateBlock{}, false, err
		}
	}
}

func (r structureTemplateNBTReader) readFixedIntVector3(tag byte, depth int) ([3]int, bool, error) {
	var out [3]int
	if depth >= structureTemplateMaxDepth {
		return out, false, fmt.Errorf("structure template NBT exceeded depth %d", structureTemplateMaxDepth)
	}

	switch tag {
	case structureNBTTagList:
		elemTag, length, err := r.readListHeader()
		if err != nil {
			return out, false, err
		}
		for i := int32(0); i < length; i++ {
			value, err := r.readIntPayload(elemTag)
			if err != nil {
				return out, false, err
			}
			if i < 3 {
				out[i] = value
			}
		}
		return out, length >= 3, nil
	case structureNBTTagIntArray:
		length, err := r.readInt32()
		if err != nil {
			return out, false, err
		}
		if length < 0 {
			return out, false, fmt.Errorf("negative int array length %d", length)
		}
		for i := int32(0); i < length; i++ {
			value, err := r.readInt32()
			if err != nil {
				return out, false, err
			}
			if i < 3 {
				out[i] = int(value)
			}
		}
		return out, length >= 3, nil
	default:
		if err := r.skipTagPayload(tag, depth); err != nil {
			return out, false, err
		}
		return out, false, nil
	}
}

func (r structureTemplateNBTReader) readIntVector(tag byte, depth int) ([]int, error) {
	if depth >= structureTemplateMaxDepth {
		return nil, fmt.Errorf("structure template NBT exceeded depth %d", structureTemplateMaxDepth)
	}

	switch tag {
	case structureNBTTagList:
		elemTag, length, err := r.readListHeader()
		if err != nil {
			return nil, err
		}
		out := make([]int, 0, length)
		for i := int32(0); i < length; i++ {
			value, err := r.readIntPayload(elemTag)
			if err != nil {
				return nil, err
			}
			out = append(out, value)
		}
		return out, nil
	case structureNBTTagIntArray:
		length, err := r.readInt32()
		if err != nil {
			return nil, err
		}
		if length < 0 {
			return nil, fmt.Errorf("negative int array length %d", length)
		}
		out := make([]int, 0, length)
		for i := int32(0); i < length; i++ {
			value, err := r.readInt32()
			if err != nil {
				return nil, err
			}
			out = append(out, int(value))
		}
		return out, nil
	default:
		if err := r.skipTagPayload(tag, depth); err != nil {
			return nil, err
		}
		return nil, nil
	}
}

func (r structureTemplateNBTReader) readListHeader() (byte, int32, error) {
	elemTag, err := r.readByte()
	if err != nil {
		return 0, 0, err
	}
	length, err := r.readInt32()
	if err != nil {
		return 0, 0, err
	}
	if length < 0 {
		return 0, 0, fmt.Errorf("negative list length %d", length)
	}
	return elemTag, length, nil
}

func (r structureTemplateNBTReader) readIntPayload(tag byte) (int, error) {
	switch tag {
	case structureNBTTagByte:
		value, err := r.readByte()
		return int(int8(value)), err
	case structureNBTTagShort:
		value, err := r.readInt16()
		return int(value), err
	case structureNBTTagInt:
		value, err := r.readInt32()
		return int(value), err
	case structureNBTTagLong:
		value, err := r.readInt64()
		return int(value), err
	default:
		return 0, fmt.Errorf("expected integer tag, got %d", tag)
	}
}

func (r structureTemplateNBTReader) skipTagPayload(tag byte, depth int) error {
	if depth >= structureTemplateMaxDepth {
		return fmt.Errorf("structure template NBT exceeded depth %d", structureTemplateMaxDepth)
	}
	switch tag {
	case structureNBTTagEnd:
		return nil
	case structureNBTTagByte:
		_, err := r.readByte()
		return err
	case structureNBTTagShort:
		_, err := r.readInt16()
		return err
	case structureNBTTagInt:
		_, err := r.readInt32()
		return err
	case structureNBTTagLong:
		_, err := r.readInt64()
		return err
	case structureNBTTagFloat:
		_, err := r.readFloat32()
		return err
	case structureNBTTagDouble:
		_, err := r.readFloat64()
		return err
	case structureNBTTagByteArray:
		length, err := r.readInt32()
		if err != nil {
			return err
		}
		_, err = r.readRaw(int(length))
		return err
	case structureNBTTagString:
		_, err := r.readString()
		return err
	case structureNBTTagList:
		elemTag, length, err := r.readListHeader()
		if err != nil {
			return err
		}
		for i := int32(0); i < length; i++ {
			if err := r.skipTagPayload(elemTag, depth+1); err != nil {
				return err
			}
		}
		return nil
	case structureNBTTagCompound:
		for {
			nextTag, err := r.readByte()
			if err != nil {
				return err
			}
			if nextTag == structureNBTTagEnd {
				return nil
			}
			if _, err := r.readString(); err != nil {
				return err
			}
			if err := r.skipTagPayload(nextTag, depth+1); err != nil {
				return err
			}
		}
	case structureNBTTagIntArray:
		length, err := r.readInt32()
		if err != nil {
			return err
		}
		if length < 0 {
			return fmt.Errorf("negative int array length %d", length)
		}
		for i := int32(0); i < length; i++ {
			if _, err := r.readInt32(); err != nil {
				return err
			}
		}
		return nil
	case structureNBTTagLongArray:
		length, err := r.readInt32()
		if err != nil {
			return err
		}
		if length < 0 {
			return fmt.Errorf("negative long array length %d", length)
		}
		for i := int32(0); i < length; i++ {
			if _, err := r.readInt64(); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported NBT tag %d", tag)
	}
}

func (r structureTemplateNBTReader) readTagPayload(tag byte, depth int) (any, error) {
	if depth >= structureTemplateMaxDepth {
		return nil, fmt.Errorf("structure template NBT exceeded depth %d", structureTemplateMaxDepth)
	}
	switch tag {
	case structureNBTTagEnd:
		return nil, nil
	case structureNBTTagByte:
		v, err := r.readByte()
		return int64(int8(v)), err
	case structureNBTTagShort:
		v, err := r.readInt16()
		return int64(v), err
	case structureNBTTagInt:
		v, err := r.readInt32()
		return int64(v), err
	case structureNBTTagLong:
		v, err := r.readInt64()
		return v, err
	case structureNBTTagFloat:
		v, err := r.readFloat32()
		return float64(v), err
	case structureNBTTagDouble:
		return r.readFloat64()
	case structureNBTTagByteArray:
		length, err := r.readInt32()
		if err != nil {
			return nil, err
		}
		return r.readRaw(int(length))
	case structureNBTTagString:
		return r.readString()
	case structureNBTTagList:
		elemTag, err := r.readByte()
		if err != nil {
			return nil, err
		}
		length, err := r.readInt32()
		if err != nil {
			return nil, err
		}
		if length < 0 {
			return nil, fmt.Errorf("negative list length %d", length)
		}
		out := make([]any, 0, length)
		for i := int32(0); i < length; i++ {
			value, err := r.readTagPayload(elemTag, depth+1)
			if err != nil {
				return nil, err
			}
			out = append(out, value)
		}
		return out, nil
	case structureNBTTagCompound:
		out := make(map[string]any)
		for {
			nextTag, err := r.readByte()
			if err != nil {
				return nil, err
			}
			if nextTag == structureNBTTagEnd {
				return out, nil
			}
			name, err := r.readString()
			if err != nil {
				return nil, err
			}
			value, err := r.readTagPayload(nextTag, depth+1)
			if err != nil {
				return nil, err
			}
			out[name] = value
		}
	case structureNBTTagIntArray:
		length, err := r.readInt32()
		if err != nil {
			return nil, err
		}
		if length < 0 {
			return nil, fmt.Errorf("negative int array length %d", length)
		}
		out := make([]int32, length)
		for i := int32(0); i < length; i++ {
			value, err := r.readInt32()
			if err != nil {
				return nil, err
			}
			out[i] = value
		}
		return out, nil
	case structureNBTTagLongArray:
		length, err := r.readInt32()
		if err != nil {
			return nil, err
		}
		if length < 0 {
			return nil, fmt.Errorf("negative long array length %d", length)
		}
		out := make([]int64, length)
		for i := int32(0); i < length; i++ {
			value, err := r.readInt64()
			if err != nil {
				return nil, err
			}
			out[i] = value
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported NBT tag %d", tag)
	}
}

func (r structureTemplateNBTReader) readRaw(length int) ([]byte, error) {
	if length < 0 {
		return nil, fmt.Errorf("negative byte length %d", length)
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(r.r, data); err != nil {
		return nil, err
	}
	return data, nil
}

func (r structureTemplateNBTReader) readByte() (byte, error) {
	var value [1]byte
	_, err := io.ReadFull(r.r, value[:])
	return value[0], err
}

func (r structureTemplateNBTReader) readInt16() (int16, error) {
	var buf [2]byte
	if _, err := io.ReadFull(r.r, buf[:]); err != nil {
		return 0, err
	}
	return int16(binary.BigEndian.Uint16(buf[:])), nil
}

func (r structureTemplateNBTReader) readInt32() (int32, error) {
	var buf [4]byte
	if _, err := io.ReadFull(r.r, buf[:]); err != nil {
		return 0, err
	}
	return int32(binary.BigEndian.Uint32(buf[:])), nil
}

func (r structureTemplateNBTReader) readInt64() (int64, error) {
	var buf [8]byte
	if _, err := io.ReadFull(r.r, buf[:]); err != nil {
		return 0, err
	}
	return int64(binary.BigEndian.Uint64(buf[:])), nil
}

func (r structureTemplateNBTReader) readFloat32() (float32, error) {
	var buf [4]byte
	if _, err := io.ReadFull(r.r, buf[:]); err != nil {
		return 0, err
	}
	return math.Float32frombits(binary.BigEndian.Uint32(buf[:])), nil
}

func (r structureTemplateNBTReader) readFloat64() (float64, error) {
	var buf [8]byte
	if _, err := io.ReadFull(r.r, buf[:]); err != nil {
		return 0, err
	}
	return math.Float64frombits(binary.BigEndian.Uint64(buf[:])), nil
}

func (r structureTemplateNBTReader) readString() (string, error) {
	length, err := r.readInt16()
	if err != nil {
		return "", err
	}
	if length < 0 {
		return "", fmt.Errorf("negative string length %d", length)
	}
	data, err := r.readRaw(int(length))
	if err != nil {
		return "", err
	}
	return string(data), nil
}
