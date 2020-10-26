package layers

import "gitlab.com/akita/mgpusim/driver"

// Tensor defines multi-dimension matrices.
type Tensor struct {
	size []int
	ptr  driver.GPUPtr

	driver     *driver.Driver
	ctx        *driver.Context
	descriptor string
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

// Dim returns the dimension of the tensor.
func (t Tensor) Dim() int {
	return len(t.size)
}

// NumElement returns the total number of scalar numbers in a tensor.
func (t Tensor) NumElement() int {
	n := 1

	for _, s := range t.size {
		n *= s
	}

	return n
}

// Reshape creates another tensor with different sizes. The new tensor shares
// the buffer with the old tensor.
func (t Tensor) Reshape(newSize []int) *Tensor {
	numElement := t.NumElement()

	newT := &Tensor{
		ptr:    t.ptr,
		size:   newSize,
		driver: t.driver,
		ctx:    t.ctx,
	}
	newNumElement := newT.NumElement()

	if numElement != newNumElement {
		panic("mismatch in shape")
	}

	return newT
}

// Descriptor returns the tensor descriptor
func (t Tensor) Descriptor() string {
	return t.descriptor
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
