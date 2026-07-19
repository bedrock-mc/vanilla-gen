//go:build !linux || !cgo

package gen

import "fmt"

// NewOpenCLAccelerator reports an unavailable backend on builds that cannot
// use the Linux OpenCL loader. CPU and automatic configurations remain fully
// functional on these builds.
func NewOpenCLAccelerator(_ *NoiseRegistry, _ OpenCLConfig) (ComputeAccelerator, error) {
	return nil, fmt.Errorf("%w: OpenCL requires linux with CGO enabled", ErrAcceleratorUnavailable)
}
