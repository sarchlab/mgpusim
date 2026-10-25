// Package gputensor provides GPU tensor and tensor operation implementations.
package gputensor

import "github.com/sarchlab/mgpusim/v5/amd/driver"

// A Tensor is a multi-dementional array.
type Tensor struct {
	owner     *GPUOperator
	hostEpoch uint64
	host      []float64
	hostReady bool

	driver *driver.Runtime
	ctx    driver.Context

	size []int
	ptr  driver.Ptr

	descriptor string
}

// Dim returns the number of dimensions that the tensor has.
func (t Tensor) Dim() int {
	return len(t.size)
}

// NumElement returns the number of elements in the tensor.
func (t Tensor) NumElement() int {
	n := 1

	for _, d := range t.size {
		n *= d
	}

	return n
}

// Size returns the size of the tensor on each dimension.
func (t Tensor) Size() []int {
	return t.size
}

// SetSize sets the size of the tensor.
func (t *Tensor) SetSize(s []int) {
	t.size = make([]int, len(s))
	copy(t.size, s)
}

// Descriptor returns the descriptor of the tensor.
func (t Tensor) Descriptor() string {
	return t.descriptor
}

// SetDescriptor sets the descriptor of the tensor.
func (t *Tensor) SetDescriptor(d string) {
	t.descriptor = d
}

// Vector returns a completed explicit readback, invalidated by later mutations.
func (t *Tensor) Vector() []float64 {
	if !t.hostReady || t.owner != nil && t.hostEpoch != t.owner.mutationEpoch {
		panic("gputensor: Vector requires a completed explicit readback")
	}
	return append([]float64(nil), t.host...)
}

// Ptr returns the GPU pointer of the tensor.
func (t *Tensor) Ptr() driver.Ptr {
	return t.ptr
}
