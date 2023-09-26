// Package tensor provides GPU tensor and tensor operation implementations.
package tensor

import "github.com/sarchlab/mgpusim/v3/driver"

// A Tensor is a multi-dementional array.
type Tensor struct {
	driver *driver.Driver
	ctx    *driver.Context

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

// Vector copies the data from the GPU to the simulator.
func (t *Tensor) Vector() []float64 {
	raw := make([]float32, t.NumElement())

	t.driver.MemCopyD2H(t.ctx, raw, t.ptr)

	out := make([]float64, t.NumElement())
	for i := 0; i < t.NumElement(); i++ {
		out[i] = float64(raw[i])
	}

	return out
}

// Ptr returns the GPU pointer of the tensor.
func (t *Tensor) Ptr() driver.Ptr {
	return t.ptr
}
