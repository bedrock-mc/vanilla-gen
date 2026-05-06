# vanilla-gen

Standalone vanilla-style world generator for Dragonfly.

```go
import vanilla "github.com/bedrock-mc/vanilla-gen"

world.Config{
	Generator: vanilla.New(seed),
}
```

If you want Dragonfly to register generic implementations for block states that
exist in the runtime palette but are missing typed upstream block structs, add a
blank import:

```go
import (
	_ "github.com/bedrock-mc/vanilla-gen/block"
	vanilla "github.com/bedrock-mc/vanilla-gen"
)
```

The blank import runs during init and silently registers Dragonfly block states
that upstream still exposes as unknown blocks.

The module is pinned to the current Dragonfly upstream `master` pseudo-version in `go.mod`.
