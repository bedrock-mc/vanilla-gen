package vanilla

import (
	"errors"
	"fmt"
	"sync/atomic"

	gen "github.com/bedrock-mc/vanilla-gen/gen"
	"github.com/df-mc/dragonfly/server/world"
)

// AccelerationMode selects the world-generation compute backend.
type AccelerationMode string

const (
	// AccelerationCPU preserves the portable CPU implementation.
	AccelerationCPU AccelerationMode = "cpu"
	// AccelerationAuto uses OpenCL when a suitable GPU is available and falls
	// back to the CPU both at startup and after a runtime device failure.
	AccelerationAuto AccelerationMode = "auto"
	// AccelerationOpenCL requires a float64-capable OpenCL GPU at startup.
	// Runtime failures still fall back to CPU so chunk generation remains live.
	AccelerationOpenCL AccelerationMode = "opencl"
)

// OpenCLConfig controls OpenCL device selection and batch sizing.
type OpenCLConfig = gen.OpenCLConfig

// AccelerationConfig configures optional GPU acceleration.
type AccelerationConfig struct {
	Mode   AccelerationMode
	OpenCL OpenCLConfig
}

// GeneratorConfig configures a Generator. Its zero value selects the CPU and
// is equivalent to New/NewForDimension.
type GeneratorConfig struct {
	Acceleration AccelerationConfig
}

// AccelerationStatus reports the configured and currently active backend.
type AccelerationStatus struct {
	Configured AccelerationMode
	Active     bool
	Backend    string
	Device     string
	Fallback   string
}

type accelerationFailure struct{ err error }

type accelerationState struct {
	configured AccelerationMode
	backend    gen.ComputeAccelerator
	active     atomic.Bool
	failure    atomic.Pointer[accelerationFailure]
}

func newAccelerationState(dim world.Dimension, noises *gen.NoiseRegistry, cfg AccelerationConfig) (*accelerationState, error) {
	mode := cfg.Mode
	if mode == "" {
		mode = AccelerationCPU
	}
	state := &accelerationState{configured: mode}
	switch mode {
	case AccelerationCPU:
		return state, nil
	case AccelerationAuto, AccelerationOpenCL:
	default:
		return nil, fmt.Errorf("unknown acceleration mode %q", mode)
	}
	if dim != world.Overworld {
		err := fmt.Errorf("GPU acceleration currently supports the overworld only")
		if mode == AccelerationOpenCL {
			return nil, err
		}
		state.failure.Store(&accelerationFailure{err: err})
		return state, nil
	}
	backend, err := gen.NewOpenCLAccelerator(noises, cfg.OpenCL)
	if err != nil {
		if mode == AccelerationOpenCL {
			return nil, err
		}
		state.failure.Store(&accelerationFailure{err: err})
		return state, nil
	}
	state.backend = backend
	state.active.Store(true)
	return state, nil
}

func (s *accelerationState) Name() string {
	if s == nil || s.backend == nil {
		return "cpu"
	}
	return s.backend.Name()
}

func (s *accelerationState) FinalDensity(chunkX, chunkZ, minY, maxY int, flat *gen.FlatCacheGrid) (*gen.FinalDensityChunk, error) {
	if s == nil || s.backend == nil || !s.active.Load() {
		return nil, gen.ErrAcceleratorUnavailable
	}
	result, err := s.backend.FinalDensity(chunkX, chunkZ, minY, maxY, flat)
	if err != nil {
		s.disable(err)
	}
	return result, err
}

func (s *accelerationState) SampleClimate(points []gen.FunctionContext, dst [][6]int64) error {
	if s == nil || s.backend == nil || !s.active.Load() {
		return gen.ErrAcceleratorUnavailable
	}
	err := s.backend.SampleClimate(points, dst)
	if err != nil {
		s.disable(err)
	}
	return err
}

func (s *accelerationState) SampleBiomes(points []gen.FunctionContext, dst []gen.Biome) error {
	if s == nil || s.backend == nil || !s.active.Load() {
		return gen.ErrAcceleratorUnavailable
	}
	err := s.backend.SampleBiomes(points, dst)
	if err != nil {
		s.disable(err)
	}
	return err
}

func (s *accelerationState) Close() error {
	if s == nil || s.backend == nil {
		return nil
	}
	s.active.Store(false)
	return s.backend.Close()
}

func (s *accelerationState) disable(err error) {
	if err == nil {
		return
	}
	s.failure.CompareAndSwap(nil, &accelerationFailure{err: err})
	s.active.Store(false)
}

func (s *accelerationState) computeBackend() gen.ComputeAccelerator {
	if s == nil || s.backend == nil || !s.active.Load() {
		return nil
	}
	return s
}

func (s *accelerationState) status() AccelerationStatus {
	if s == nil {
		return AccelerationStatus{Configured: AccelerationCPU, Backend: "cpu"}
	}
	status := AccelerationStatus{Configured: s.configured, Active: s.active.Load(), Backend: "cpu"}
	if s.backend != nil {
		if provider, ok := s.backend.(gen.AcceleratorInfoProvider); ok {
			info := provider.AcceleratorInfo()
			status.Device = info.Device
			if status.Active {
				status.Backend = info.Backend
			}
		} else if status.Active {
			status.Backend = s.backend.Name()
		}
	}
	if failure := s.failure.Load(); failure != nil && failure.err != nil {
		status.Fallback = failure.err.Error()
	}
	return status
}

// IsAcceleratorUnavailable reports whether err indicates that automatic mode
// could not find a usable compute device.
func IsAcceleratorUnavailable(err error) bool {
	return errors.Is(err, gen.ErrAcceleratorUnavailable)
}
