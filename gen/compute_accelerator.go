package gen

import (
	"errors"
)

// ErrAcceleratorUnavailable is returned when the requested compute runtime or
// device is not available. Callers using an automatic policy may safely fall
// back to the CPU when this error is returned.
var ErrAcceleratorUnavailable = errors.New("world-generation accelerator unavailable")

// ComputeAccelerator evaluates the large, data-parallel overworld jobs:
// final-density corners, climate samples, and climate-to-biome lookup.
// Implementations must be safe for concurrent use.
type ComputeAccelerator interface {
	Name() string
	FinalDensity(chunkX, chunkZ, minY, maxY int, flat *FlatCacheGrid) (*FinalDensityChunk, error)
	SampleClimate(points []FunctionContext, dst [][6]int64) error
	SampleBiomes(points []FunctionContext, dst []Biome) error
	Close() error
}

// OpenCLConfig controls the OpenCL compute backend. Zero values select the
// first GPU and conservative throughput-oriented defaults.
type OpenCLConfig struct {
	// LibraryPath optionally names the OpenCL loader. Empty uses the platform
	// defaults (libOpenCL.so.1 and libOpenCL.so on Linux).
	LibraryPath string

	// ICDLibraryPath optionally names a vendor OpenCL implementation for ICD
	// loaders that have no configured vendor files. Empty auto-detects the
	// NixOS NVIDIA driver under /run/opengl-driver when needed.
	ICDLibraryPath string

	// PlatformIndex and DeviceIndex select a GPU when multiple OpenCL devices
	// are installed. Negative values and zero both select the first device.
	PlatformIndex int
	DeviceIndex   int

	// ClimateBatchSize is the maximum number of climate points submitted in
	// one kernel launch. Values below 256 use the default of 16,384.
	ClimateBatchSize int
}

// AcceleratorInfo describes the selected compute device.
type AcceleratorInfo struct {
	Backend string
	Device  string
	Vendor  string
}

// Info is implemented by accelerators that expose device information.
type AcceleratorInfoProvider interface {
	AcceleratorInfo() AcceleratorInfo
}
