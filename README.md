# vanilla-gen

Standalone vanilla-style world generator for Dragonfly.

## Run the example

A ready-to-run test server lives in `cmd/testserver`. It spins up a minimal
Dragonfly server using this generator so you can connect with a Bedrock client
(default `:19132`):

```sh
go run ./cmd/testserver                 # seed 1 (matches the Java parity world)
go run ./cmd/testserver -seed 42
go run ./cmd/testserver -gpu            # require OpenCL GPU acceleration
```

Flags:

| Flag | Default | Description |
| --- | --- | --- |
| `-seed` | `1` | World seed. |
| `-listen` | `:19132` | Address the server listens on. |
| `-chunk-radius` | `6` | Max client view distance; lower reduces first-join latency. |
| `-chunk-workers` | `8` | Concurrent chunk load workers. |
| `-gpu` | `false` | Require OpenCL GPU acceleration for the overworld. |

Delete the `world/` directory between runs when changing the seed or generator.

## Library usage

```go
import vanilla "github.com/bedrock-mc/vanilla-gen"

world.Config{
	Generator: vanilla.New(seed),
}
```

To register generic implementations for block states that exist in the runtime
palette but lack typed upstream block structs, add a blank import. It runs
during init and silently registers the block states upstream still exposes as
unknown blocks:

```go
import (
	_ "github.com/bedrock-mc/vanilla-gen/block"
	vanilla "github.com/bedrock-mc/vanilla-gen"
)
```

The module is pinned to the current Dragonfly upstream `master` pseudo-version in `go.mod`.

## GPU acceleration

Overworld density, climate sampling, and biome classification can optionally run
on an OpenCL GPU. `New` and `NewForDimension` stay CPU-only; enable GPU
selection explicitly:

```go
generator, err := vanilla.NewWithConfig(seed, vanilla.GeneratorConfig{
	Acceleration: vanilla.AccelerationConfig{
		Mode: vanilla.AccelerationAuto,
	},
})
if err != nil {
	return err
}
defer generator.Close()

config := world.Config{Generator: generator}
```

- `AccelerationAuto` — use the GPU when available, fall back to CPU at startup or after a device failure.
- `AccelerationOpenCL` — require a usable GPU at startup.
- `AccelerationCPU` — force the portable path.

`AccelerationStatus` reports the selected device and any fallback reason.

The backend requires Linux, CGO, an OpenCL loader, and a GPU with float64
support. It loads OpenCL dynamically, so headers are not a build dependency.
Most systems can leave `LibraryPath` empty; where the loader is not on the
dynamic-library search path (including some Nix setups), set it explicitly:

```go
OpenCL: vanilla.OpenCLConfig{
	LibraryPath:    "/path/to/libOpenCL.so.1",
	ICDLibraryPath: "/path/to/vendor/libOpenCL-implementation.so",
}
```

`ICDLibraryPath` is only needed when the loader has no vendor registration. On
NixOS the NVIDIA implementation under `/run/opengl-driver` is detected
automatically when `/etc/OpenCL/vendors` and the standard ICD environment
variables are absent.

Reuse one generator: construction compiles the kernels and uploads
seed-specific noise tables and the biome search tree once. The generator and
accelerator are safe for concurrent chunk generation.

## Measured performance

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
