package parity

import (
	"encoding/binary"
	"fmt"
	"math"
)

const (
	tagEnd = iota
	tagByte
	tagShort
	tagInt
	tagLong
	tagFloat
	tagDouble
	tagByteArray
	tagString
	tagList
	tagCompound
	tagIntArray
	tagLongArray
)

type nbtDecoder struct {
	data []byte
	pos  int
}

func parseNBT(data []byte) (root map[string]any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("nbt: %v", r)
		}
	}()
	d := &nbtDecoder{data: data}
	if typ := d.u8(); typ != tagCompound {
		return nil, fmt.Errorf("nbt: root tag type %d, want compound", typ)
	}
	d.string()
	return d.compound(), nil
}

func (d *nbtDecoder) take(n int) []byte {
	if d.pos+n > len(d.data) || n < 0 {
		panic("unexpected end of data")
	}
	b := d.data[d.pos : d.pos+n]
	d.pos += n
	return b
}

func (d *nbtDecoder) u8() byte     { return d.take(1)[0] }
func (d *nbtDecoder) i16() int16   { return int16(binary.BigEndian.Uint16(d.take(2))) }
func (d *nbtDecoder) i32() int32   { return int32(binary.BigEndian.Uint32(d.take(4))) }
func (d *nbtDecoder) i64() int64   { return int64(binary.BigEndian.Uint64(d.take(8))) }
func (d *nbtDecoder) f32() float32 { return math.Float32frombits(binary.BigEndian.Uint32(d.take(4))) }
func (d *nbtDecoder) f64() float64 { return math.Float64frombits(binary.BigEndian.Uint64(d.take(8))) }

func (d *nbtDecoder) string() string {
	n := int(uint16(d.i16()))
	return string(d.take(n))
}

func (d *nbtDecoder) compound() map[string]any {
	m := make(map[string]any)
	for {
		typ := d.u8()
		if typ == tagEnd {
			return m
		}
		name := d.string()
		m[name] = d.payload(typ)
	}
}

func (d *nbtDecoder) payload(typ byte) any {
	switch typ {
	case tagByte:
		return int8(d.u8())
	case tagShort:
		return d.i16()
	case tagInt:
		return d.i32()
	case tagLong:
		return d.i64()
	case tagFloat:
		return d.f32()
	case tagDouble:
		return d.f64()
	case tagByteArray:
		n := int(d.i32())
		b := make([]byte, n)
		copy(b, d.take(n))
		return b
	case tagString:
		return d.string()
	case tagList:
		elem := d.u8()
		n := int(d.i32())
		if n < 0 {
			n = 0
		}
		l := make([]any, n)
		for i := range l {
			l[i] = d.payload(elem)
		}
		return l
	case tagCompound:
		return d.compound()
	case tagIntArray:
		n := int(d.i32())
		a := make([]int32, n)
		for i := range a {
			a[i] = d.i32()
		}
		return a
	case tagLongArray:
		n := int(d.i32())
		a := make([]int64, n)
		for i := range a {
			a[i] = d.i64()
		}
		return a
	default:
		panic(fmt.Sprintf("unknown tag type %d", typ))
	}
}
