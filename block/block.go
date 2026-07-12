// Package block registers vanilla block implementations that are needed by the
// generator but are not available as typed blocks in upstream Dragonfly yet.
package block

import (
	"fmt"
	"math"
	"strings"

	_ "github.com/df-mc/dragonfly/server/block"
	dfcube "github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
)

func init() {
	Register()
}

// Register registers all missing block states. States already implemented by
// upstream Dragonfly are left alone.
func Register() {
	registerAll([]world.Block{
		BuddingAmethyst{},
		RootedDirt{},
		EndGateway{},
		EndPortal{},
		EndStone{},
		HangingRoots{},
		MossBlock{},
		Nylium{},
		Nylium{Warped: true},
		PaleMossBlock{},
		MangroveRoots{},
		Spawner{},
		ChorusPlant{},
		Fungus{},
		Fungus{Warped: true},
		Roots{},
		Roots{Warped: true},
		Portal{Axis: dfcube.X},
		Portal{Axis: dfcube.Z},
	})
	registerAll(allAmethystCluster())
	registerAll(allAzalea())
	registerAll(allBamboo())
	registerAll(allBrownMushroomBlock())
	registerAll(allChorusFlowers())
	registerAll(allCreakingHeart())
	registerAll(allEndPortalFrames())
	registerAll(allLargeAmethystBud())
	registerAll(allLeafLitter())
	registerAll(allMangrovePropagule())
	registerAll(allMediumAmethystBud())
	registerAll(allMushroomStem())
	registerAll(allNetherVines())
	registerAll(allPaleHangingMoss())
	registerAll(allPaleMossCarpet())
	registerAll(allRedMushroomBlock())
	registerAll(allSmallAmethystBud())
	registerAll(allSmallDripleaf())
	registerAll(allBigDripleaf())
}

func registerAll(blocks []world.Block) {
	for _, b := range blocks {
		register(b)
	}
}

func register(b world.Block) {
	defer func() {
		if recovered := recover(); recovered != nil {
			message := fmt.Sprint(recovered)
			if strings.HasPrefix(message, "block state returned is not registered ") ||
				(strings.HasPrefix(message, "block with name and properties ") && strings.HasSuffix(message, " already registered")) {
				return
			}
			panic(recovered)
		}
	}()
	world.RegisterBlock(b)
}

type base struct{}

func (base) Hash() (uint64, uint64) { return 0, math.MaxUint64 }

func (base) Model() world.BlockModel { return emptyModel{} }

type emptyModel struct{}

func (emptyModel) BBox(dfcube.Pos, world.BlockSource) []dfcube.BBox { return nil }

func (emptyModel) FaceSolid(dfcube.Pos, dfcube.Face, world.BlockSource) bool { return false }

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
