// Command testserver runs a minimal Dragonfly server with the vanilla
// generator, for eyeballing parity against a Java world with the same seed.
//
//	go run ./cmd/testserver          # seed 1 (matches the parity ground truth)
//	go run ./cmd/testserver -seed 42
//	go run ./cmd/testserver -gpu -listen :19133
//
// Connect with a Bedrock client to the configured address (port 19132 by
// default). Delete world/ between runs if you change the seed or generator.
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	vanilla "github.com/bedrock-mc/vanilla-gen"
	_ "github.com/bedrock-mc/vanilla-gen/block"
	"github.com/df-mc/dragonfly/server"
	"github.com/df-mc/dragonfly/server/player/chat"
	"github.com/df-mc/dragonfly/server/world"
)

func main() {
	seed := flag.Int64("seed", 1, "world seed (1 matches the Java parity world)")
	gpu := flag.Bool("gpu", false, "require OpenCL GPU acceleration for overworld generation")
	listenAddress := flag.String("listen", ":19132", "address on which the Bedrock server listens")
	chunkRadius := flag.Int("chunk-radius", 6, "maximum client chunk radius (lower values reduce first-join latency)")
	openCLLibrary := flag.String("opencl-library", os.Getenv("VANILLA_GEN_OPENCL_LIBRARY"), "path to the OpenCL loader (defaults to VANILLA_GEN_OPENCL_LIBRARY)")
	openCLICD := flag.String("opencl-icd", "", "optional path to an OpenCL vendor implementation (NixOS NVIDIA is auto-detected)")
	flag.Parse()
	if *chunkRadius < 2 {
		panic("chunk-radius must be at least 2")
	}

	chat.Global.Subscribe(chat.StdoutSubscriber{})
	userConfig := server.DefaultConfig()
	userConfig.Network.Address = *listenAddress
	userConfig.Players.MaximumChunkRadius = *chunkRadius
	conf, err := userConfig.Config(slog.Default())
	if err != nil {
		panic(err)
	}
	var acceleratedGenerators []*vanilla.Generator
	conf.Generator = func(dim world.Dimension) world.Generator {
		if !*gpu || dim != world.Overworld {
			return vanilla.NewForDimension(*seed, dim)
		}
		g, err := vanilla.NewForDimensionWithConfig(*seed, dim, vanilla.GeneratorConfig{
			Acceleration: vanilla.AccelerationConfig{
				Mode: vanilla.AccelerationOpenCL,
				OpenCL: vanilla.OpenCLConfig{
					LibraryPath:    *openCLLibrary,
					ICDLibraryPath: *openCLICD,
				},
			},
		})
		if err != nil {
			panic(fmt.Errorf("start OpenCL world generator: %w", err))
		}
		status := g.AccelerationStatus()
		fmt.Printf("worldgen acceleration: %s on %s\n", status.Backend, status.Device)
		acceleratedGenerators = append(acceleratedGenerators, &g)
		return g
	}
	conf.ChunkLoadWorkers = 1

	srv := conf.New()
	srv.CloseOnProgramEnd()
	defer func() {
		for _, generator := range acceleratedGenerators {
			_ = generator.Close()
		}
	}()

	// The world provider's saved spawn defaults to the origin, which on many
	// seeds (including seed 1) is deep ocean. Move it to the nearest land
	// like vanilla does. Delete the world/ directory when changing seeds so
	// a stale spawn is not restored from disk.
	g := vanilla.NewForDimension(*seed, world.Overworld)
	spawn := g.DefaultSpawn(world.Overworld)
	srv.World().SetSpawn(spawn)
	fmt.Printf("spawn set to %v\n", spawn)

	fmt.Printf("listening on %s, seed %d, maximum chunk radius %d\n", *listenAddress, *seed, *chunkRadius)
	srv.Listen()
	for range srv.Accept() {
	}
}
