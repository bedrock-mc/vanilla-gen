package vanilla

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
)

// TestMineshaftPiecesFixture validates the exact mineshaft piece layout
// against a dump produced by the vanilla 1.21.11 server jar (DumpMineshaft
// driver). Set MINESHAFT_PIECES_FIXTURE to the dump path; the fixture seed
// defaults to VANILLA_GT_SEED (or 1).
func TestMineshaftPiecesFixture(t *testing.T) {
	path := os.Getenv("MINESHAFT_PIECES_FIXTURE")
	if path == "" {
		t.Skip("MINESHAFT_PIECES_FIXTURE not set")
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer file.Close()

	g := New(parityWorldSeed(t))
	planner, ok := g.findStructurePlanner("mineshafts")
	if !ok {
		t.Fatal("mineshafts planner missing")
	}

	type fixturePiece struct {
		line string
	}
	var (
		startChunkX, startChunkZ int
		startName                string
		expected                 []fixturePiece
		startCount               int
	)

	check := func() {
		if startName == "" {
			return
		}
		startCount++
		start, exists := g.planStructureStart(planner, [2]int32{int32(startChunkX), int32(startChunkZ)}, -64, 319, nil)
		if !exists || start.mineshaft == nil {
			t.Errorf("chunk %d,%d: expected %s start with %d pieces, got none", startChunkX, startChunkZ, startName, len(expected))
			return
		}
		if start.structureName != startName {
			t.Errorf("chunk %d,%d: structure %s, want %s", startChunkX, startChunkZ, start.structureName, startName)
		}
		got := make([]string, 0, len(start.mineshaft.pieces))
		for _, p := range start.mineshaft.pieces {
			got = append(got, formatMineshaftPieceForFixture(p))
		}
		if len(got) != len(expected) {
			t.Errorf("chunk %d,%d: %d pieces, want %d", startChunkX, startChunkZ, len(got), len(expected))
		}
		for i := 0; i < len(got) && i < len(expected); i++ {
			if got[i] != expected[i].line {
				t.Errorf("chunk %d,%d piece %d:\n got  %s\n want %s", startChunkX, startChunkZ, i, got[i], expected[i].line)
			}
		}
	}

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "START ") {
			check()
			expected = expected[:0]
			fields := strings.Fields(line)
			chunkPart := strings.TrimPrefix(fields[1], "chunk=")
			coords := strings.Split(chunkPart, ",")
			startChunkX, _ = strconv.Atoi(coords[0])
			startChunkZ, _ = strconv.Atoi(coords[1])
			startName = strings.TrimPrefix(fields[2], "structure=")
			continue
		}
		if strings.HasPrefix(line, "PIECE ") {
			expected = append(expected, fixturePiece{line: line})
		}
	}
	check()
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan fixture: %v", err)
	}
	if startCount == 0 {
		t.Fatal("fixture contained no starts")
	}
	t.Logf("verified %d mineshaft starts", startCount)
}

func mineshaftDirFixtureName(d mineshaftDir) string {
	switch d {
	case mineshaftDirNorth:
		return "north"
	case mineshaftDirSouth:
		return "south"
	case mineshaftDirWest:
		return "west"
	case mineshaftDirEast:
		return "east"
	default:
		return "null"
	}
}

func formatMineshaftPieceForFixture(p *mineshaftPiece) string {
	var b strings.Builder
	switch p.kind {
	case mineshaftPieceRoom:
		b.WriteString("PIECE MineShaftRoom")
	case mineshaftPieceCorridor:
		b.WriteString("PIECE MineShaftCorridor")
	case mineshaftPieceCrossing:
		b.WriteString("PIECE MineShaftCrossing")
	case mineshaftPieceStairs:
		b.WriteString("PIECE MineShaftStairs")
	}
	fmt.Fprintf(&b, " bb=%d,%d,%d,%d,%d,%d o=%s gd=%d",
		p.bb.minX, p.bb.minY, p.bb.minZ, p.bb.maxX, p.bb.maxY, p.bb.maxZ,
		mineshaftDirFixtureName(p.orientation), p.genDepth)
	switch p.kind {
	case mineshaftPieceRoom:
		for _, e := range p.entrances {
			fmt.Fprintf(&b, " entrance=%d,%d,%d,%d,%d,%d", e.minX, e.minY, e.minZ, e.maxX, e.maxY, e.maxZ)
		}
	case mineshaftPieceCorridor:
		fmt.Fprintf(&b, " hasRails=%t spiderCorridor=%t numSections=%d", p.hasRails, p.spiderCorridor, p.numSections)
	case mineshaftPieceCrossing:
		fmt.Fprintf(&b, " direction=%s isTwoFloored=%t", mineshaftDirFixtureName(p.direction), p.isTwoFloored)
	}
	return b.String()
}
