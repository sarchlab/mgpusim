// Package gpubuilder defines the interface for GPU builders used in timing
// simulation.
package gpubuilder

import (
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
)

// GPUBuilder is the interface for building GPUs of different types.
type GPUBuilder interface {
	WithGPUID(id uint64) GPUBuilder
	WithMemAddrOffset(offset uint64) GPUBuilder
	WithRDMAAddressMapper(mapper mem.AddressToPortMapper) GPUBuilder
	Build(name string) *sim.Domain
}
