// Package gpubuilder defines the interface for GPU builders used in timing
// simulation.
package gpubuilder

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/mgpusim/v4/domain"
)

// GPUBuilder is the interface for building GPUs of different types.
type GPUBuilder interface {
	WithGPUID(id uint64) GPUBuilder
	WithMemAddrOffset(offset uint64) GPUBuilder
	WithRDMAAddressMapper(mapper mem.AddressToPortMapper) GPUBuilder
	Build(name string) *domain.Domain
}
