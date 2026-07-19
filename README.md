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

## GPU acceleration

Overworld density, climate sampling, and biome classification can optionally
run on an OpenCL GPU. Existing `New` and `NewForDimension` calls remain
CPU-only. Enable automatic GPU selection explicitly:

```go
generator, err := vanilla.NewWithConfig(seed, vanilla.GeneratorConfig{
	Acceleration: vanilla.AccelerationConfig{
		Mode: vanilla.AccelerationAuto,
		OpenCL: vanilla.OpenCLConfig{
			PlatformIndex:   0,
			DeviceIndex:     0,
			ClimateBatchSize: 16_384,
		},
	},
})
if err != nil {
	return err
}
defer generator.Close()

config := world.Config{Generator: generator}
```

`AccelerationAuto` falls back to CPU at startup or after a device failure.
Use `AccelerationOpenCL` to require a usable GPU at startup, or
`AccelerationCPU` to force the portable path. `AccelerationStatus` reports the
selected device and any fallback reason.

The current backend requires Linux, CGO, an OpenCL loader, and a GPU with
float64 support. It loads OpenCL dynamically, so OpenCL headers are not a build
dependency. Most systems can leave `LibraryPath` empty; installations where
the loader is not on the dynamic-library search path (including some Nix
setups) can set it explicitly:

```go
OpenCL: vanilla.OpenCLConfig{
	LibraryPath: "/path/to/libOpenCL.so.1",
	ICDLibraryPath: "/path/to/vendor/libOpenCL-implementation.so",
}
```

`ICDLibraryPath` is only needed when the loader has no vendor registration.
On NixOS, the NVIDIA implementation under `/run/opengl-driver` is detected
automatically when `/etc/OpenCL/vendors` and the standard ICD environment
variables are absent.

GPU acceleration currently applies to the overworld. Reuse one generator:
construction compiles the kernels and uploads seed-specific noise tables and
the biome search tree once. The generator and accelerator are safe for
concurrent chunk generation.

The included test server caps view distance at 6 chunks by default to keep a
new client's synchronous generation queue small. Override it with
`-chunk-radius N`; large radii take proportionally longer to populate a fresh
world.

### Measured performance

Benchmarks on an AMD Ryzen AI 7 350 (16 threads) and NVIDIA GeForce RTX 5060
Laptop GPU. The table uses the same 64 adjacent chunks before and after the
hot-path work, with five final samples and OpenCL construction outside the
timer:

| Complete generation workload | Before | After | Improvement |
| --- | ---: | ---: | ---: |
| Sequential CPU | 72.33 ms/chunk | 34.58 ms/chunk | 2.09x |
| Sequential OpenCL | 53.77 ms/chunk | 28.08 ms/chunk | 1.91x |
| 16-way CPU | 16.06 ms/chunk | 6.49 ms/chunk | 2.48x |
| 16-way OpenCL | 10.94 ms/chunk | 4.81 ms/chunk | 2.28x |

A completely cold first target measured 126 ms on CPU and 77 ms on OpenCL.
In a longer 256-adjacent-chunk sweep, cache reuse brought the sequential path
to roughly 29.5 ms CPU and 25.3 ms OpenCL per chunk. The original sequential
CPU path allocated about 9.6 MB/89,700 objects per chunk; the sustained path
now allocates about 1.27 MB/6,300 objects.

Full generation gains are smaller than isolated density/climate kernel gains
because feature placement, structures, surfaces, carvers, chunk palettes and
storage remain CPU work. Dragonfly currently requests world chunks
synchronously, so the sequential rows best represent the included test server.

The accelerator keeps immutable noise data and the flattened 7,593-point
biome tree resident on the device. Vertical climate queries are grouped by X/Z
column, avoiding repeated noise evaluation, and production-size climate and
biome batches perform no per-call allocations. GPU/CPU parity tests cover
density, climate values, biome decisions, complete chunks, and concurrent
generation.
