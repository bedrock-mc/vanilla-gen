package parity

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type World struct {
	regions map[[2]int][]byte
}

func OpenRegionDir(dir string) (*World, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	w := &World{regions: make(map[[2]int][]byte)}
	for _, e := range entries {
		var rx, rz int
		if n, _ := fmt.Sscanf(e.Name(), "r.%d.%d.mca", &rx, &rz); n != 2 {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		if len(data) < 8192 {
			return nil, fmt.Errorf("parity: region %s: truncated header", e.Name())
		}
		w.regions[[2]int{rx, rz}] = data
	}
	if len(w.regions) == 0 {
		return nil, fmt.Errorf("parity: no region files in %s", dir)
	}
	return w, nil
}

func (w *World) Chunk(x, z int) (*JavaChunk, error) {
	region, ok := w.regions[[2]int{x >> 5, z >> 5}]
	if !ok {
		return nil, fmt.Errorf("parity: region for chunk %d,%d not loaded", x, z)
	}
	loc := 4 * ((x & 31) + (z&31)*32)
	offset := int(region[loc])<<16 | int(region[loc+1])<<8 | int(region[loc+2])
	sectors := int(region[loc+3])
	if offset == 0 || sectors == 0 {
		return nil, fmt.Errorf("parity: chunk %d,%d not present", x, z)
	}
	start := offset * 4096
	if start+5 > len(region) {
		return nil, fmt.Errorf("parity: chunk %d,%d: offset beyond file", x, z)
	}
	length := int(binary.BigEndian.Uint32(region[start:]))
	if length < 1 || start+4+length > len(region) {
		return nil, fmt.Errorf("parity: chunk %d,%d: bad payload length %d", x, z, length)
	}
	compression := region[start+4]
	payload := region[start+5 : start+4+length]

	var r io.Reader
	switch compression {
	case 1:
		gz, err := gzip.NewReader(bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		r = gz
	case 2:
		zr, err := zlib.NewReader(bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		r = zr
	case 3:
		r = bytes.NewReader(payload)
	default:
		return nil, fmt.Errorf("parity: chunk %d,%d: unsupported compression type %d", x, z, compression)
	}
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	root, err := parseNBT(raw)
	if err != nil {
		return nil, err
	}
	return newJavaChunk(root)
}
