// Package gputensor provides GPU tensor and tensor operation implementations.
package gputensor

import (
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/dnn/tensor"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

// A Tensor is a multi-dementional array.
type Tensor struct {
	*TensorState
	owner *GPUOperator
	id    tensor.ID
}

// Dim returns the number of dimensions that the tensor has.
func (t Tensor) Dim() int {
	return len(t.Shape)
}

// NumElement returns the number of elements in the tensor.
func (t Tensor) NumElement() int {
	n := 1

	for _, d := range t.Shape {
		n *= d
	}

	return n
}

// Size returns the size of the tensor on each dimension.
func (t Tensor) Size() []int {
	return t.Shape
}

// SetSize sets the size of the tensor.
func (t *Tensor) SetSize(s []int) {
	t.Shape = make([]int, len(s))
	copy(t.Shape, s)
}

// Descriptor returns the descriptor of the tensor.
func (t Tensor) Descriptor() string {
	return t.Description
}

// SetDescriptor sets the descriptor of the tensor.
func (t *Tensor) SetDescriptor(d string) {
	t.Description = d
}

// Vector returns a completed explicit readback, invalidated by later mutations.
func (t *Tensor) Vector() []float64 {
	if !t.HostReady || t.owner != nil && t.HostEpoch != t.owner.MutationEpoch {
		panic("gputensor: Vector requires a completed explicit readback")
	}
	return append([]float64(nil), t.Host...)
}

// Ptr returns the GPU pointer of the tensor.
func (t *Tensor) Ptr() gpuruntime.Ptr {
	return t.Address
}
