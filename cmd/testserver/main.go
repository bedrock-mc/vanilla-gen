// Command testserver runs a minimal Dragonfly server with the vanilla
// generator, for eyeballing parity against a Java world with the same seed.
//
//	go run ./cmd/testserver          # seed 1 (matches the parity ground truth)
//	go run ./cmd/testserver -seed 42
//
// Connect with a Bedrock client to <this machine>:19132. Delete the world/
// directory between runs if you change the seed or the generator.
package main

import (
	"flag"
	"fmt"
	"log/slog"

	vanilla "github.com/bedrock-mc/vanilla-gen"
	_ "github.com/bedrock-mc/vanilla-gen/block"
	"github.com/df-mc/dragonfly/server"
	"github.com/df-mc/dragonfly/server/player/chat"
	"github.com/df-mc/dragonfly/server/world"
)

func main() {
	seed := flag.Int64("seed", 1, "world seed (1 matches the Java parity world)")
	flag.Parse()

	chat.Global.Subscribe(chat.StdoutSubscriber{})
	conf, err := server.DefaultConfig().Config(slog.Default())
	if err != nil {
		panic(err)
	}
	conf.Generator = func(dim world.Dimension) world.Generator {
		return vanilla.NewForDimension(*seed, dim)
	}

	srv := conf.New()
	srv.CloseOnProgramEnd()
	fmt.Printf("listening on :19132, seed %d\n", *seed)
	srv.Listen()
	for range srv.Accept() {
	}
}
