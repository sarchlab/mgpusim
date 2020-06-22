package layers

import "gitlab.com/akita/mgpusim/driver"

// Tensor defines multi-dimension matrices.
type Tensor struct {
	size []int
	ptr  driver.GPUPtr

	driver *driver.Driver
	ctx    *driver.Context
}

// NewTensor creates a new tensor.
func NewTensor(driver *driver.Driver, ctx *driver.Context) *Tensor {
	return &Tensor{
		driver: driver,
		ctx:    ctx,
	}
}

// Init allocates the memory on the GPU and copies the data to the GPU memory.
func (t *Tensor) Init(data []float64, size []int) {
	t.size = size
	t.ptr = t.driver.AllocateMemory(t.ctx, uint64(len(data)*4))

	tempData := make([]float32, len(data))
	for i, value := range data {
		tempData[i] = float32(value)
	}

	t.driver.MemCopyH2D(t.ctx, t.ptr, tempData)
}

// Size returns the sizes for the tensor in each dimension.
func (t Tensor) Size() []int {
	return t.size
}

// Vector returns the tensor data as an array.
func (t Tensor) Vector() []float64 {
	numElem := 1
	for _, n := range t.size {
		numElem *= n
	}

	tempOutput := make([]float32, numElem)
	t.driver.MemCopyD2H(t.ctx, tempOutput, t.ptr)

	out := make([]float64, numElem)
	for i, value := range tempOutput {
		out[i] = float64(value)
	}

	return out
}

// Matrix returns the tensor as a matrix. This function panics if the tensor
// is not a 2-dimension tensor.
func (t Tensor) Matrix() *Matrix {
	if len(t.size) != 2 {
		panic("not a matrix")
	}

	m := &Matrix{
		col:  t.size[1],
		row:  t.size[0],
		data: t.ptr,
	}
	return m
}
