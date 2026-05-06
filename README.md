# vanilla-gen

Standalone vanilla-style world generator for Dragonfly.

```go
import vanilla "github.com/bedrock-mc/vanilla-gen"

world.Config{
	Generator: vanilla.New(seed),
}
```

The module is pinned to the current Dragonfly upstream `master` pseudo-version in `go.mod`.
