// Package tensor defines the tensor interface.
package tensor

// A Tensor is a multi-dimension matrix.
type Tensor interface {
	// Dim returns the number of dimension of the tensor
	Dim() int

	// NumElement returns the total number of elements in the tensor
	NumElement() int

	// Size returns the length of the tensor in each dimension, from the
	// outermost dimension to the innermost dimension.
	Size() []int

	// SetSize sets the size of the tensor.
	SetSize([]int)

	// Vector returns the data of the tensor represented in a pure vector.
	// Here, we use float64. However, the concrete tensor implementation can
	// use lower-precision numbers.
	Vector() []float64

	// Descriptor represents what does each dimension of the tensor represents.
	//
	// Tokens that represent the meaning of the dimensions include N, C, H, W.
	Descriptor() string

	// SetDescriptor sets the descriptor of the tensor.
	SetDescriptor(d string)
}

// A SimpleTensor is a multi-dimensional matrix.
type SimpleTensor struct {
	size       []int
	data       []float64
	descriptor string
}

// Dim returns the number of dimensions that the tensor has.
func (t SimpleTensor) Dim() int {
	return len(t.size)
}

// NumElement returns the total number of elements in the tensor.
func (t SimpleTensor) NumElement() int {
	n := 1

	for _, s := range t.size {
		n *= s
	}

	return n
}

// Size returns the size of the tensor.
func (t SimpleTensor) Size() []int {
	return t.size
}

// SetSize sets the size of the tensor
func (t *SimpleTensor) SetSize(newSize []int) {
	t.size = newSize
}

// Vector returns the raw data of the tensor.
func (t SimpleTensor) Vector() []float64 {
	return t.data
}

// Descriptor returns the descriptor of the tensor.
func (t SimpleTensor) Descriptor() string {
	return t.descriptor
}

// SetDescriptor sets the descriptor of the tensor
func (t *SimpleTensor) SetDescriptor(d string) {
	t.descriptor = d
}
