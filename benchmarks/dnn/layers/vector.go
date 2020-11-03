package layers

import (
	"math"

	"gitlab.com/akita/dnn/tensor"
	"gitlab.com/akita/mgpusim/driver"
)

// Vector represents a 1D array stored in the GPU memory.
type Vector struct {
	size      int
	ptr       driver.GPUPtr
	GPUDriver *driver.Driver
	GPUCtx    *driver.Context
}

// Init intialized the data and the size of the vector.
func (v *Vector) Init(data []float64, size int) {
	v.size = size
	v.ptr = v.GPUDriver.AllocateMemory(v.GPUCtx, uint64(len(data)*4))

	tempData := make([]float32, len(data))
	for i, value := range data {
		tempData[i] = float32(value)
	}

	v.GPUDriver.MemCopyH2D(v.GPUCtx, v.ptr, tempData)
}

// AsMatrix returns the vector as a matrix, with given row and col size.
func (v Vector) AsMatrix(row, col int) *Tensor {
	m := &Tensor{
		size:   []int{row, col},
		ptr:    v.ptr,
		driver: v.GPUDriver,
		ctx:    v.GPUCtx,
	}
	return m
}

// Raw returns the underlying data stored in the Vector.
func (v Vector) Raw() []float64 {
	tempData := make([]float32, v.size)
	v.GPUDriver.MemCopyD2H(v.GPUCtx, tempData, v.ptr)

	out := make([]float64, v.size)
	for i, value := range tempData {
		out[i] = float64(value)
	}

	return out
}

// Set assignes the data of the vector.
func (v Vector) Set(val []float64) {
	temp := make([]float32, v.size)
	for i, value := range val {
		temp[i] = float32(value)
	}

	v.GPUDriver.MemCopyH2D(v.GPUCtx, v.ptr, temp)
}

// Clone creates a new vector with same data.
func (v Vector) Clone() tensor.Vector {
	vector := &Vector{
		size:      v.size,
		GPUDriver: v.GPUDriver,
		GPUCtx:    v.GPUCtx,
	}
	vector.ptr = v.GPUDriver.AllocateMemory(v.GPUCtx, uint64(v.size*4))

	tempData := make([]float32, v.size)
	v.GPUDriver.MemCopyD2H(v.GPUCtx, tempData, v.ptr)
	v.GPUDriver.MemCopyH2D(v.GPUCtx, vector.ptr, tempData)

	return vector
}

// Scale multiply each numbers in the vector by alpha.
func (v Vector) Scale(alpha float64) {
	raw := v.Raw()

	for i := range raw {
		raw[i] *= alpha
	}

	v.Set(raw)
}

// Add performs a element-wise add operation.
func (v Vector) Add(b tensor.Vector) {
	aRaw := v.Raw()
	bRaw := b.Raw()

	for i := range aRaw {
		aRaw[i] += bRaw[i]
	}

	v.Set(aRaw)
}

// AddScalar adds each element in the vector with alpha.
func (v Vector) AddScalar(alpha float64) {
	aRaw := v.Raw()

	for i := range aRaw {
		aRaw[i] += alpha
	}

	v.Set(aRaw)
}

// ScaleAdd performs an alpha*A + beta*B operation. A is the current vector.
func (v Vector) ScaleAdd(alpha, beta float64, b tensor.Vector) {
	aRaw := v.Raw()
	bRaw := b.Raw()

	for i := range aRaw {
		aRaw[i] = alpha*aRaw[i] + beta*bRaw[i]
	}

	v.Set(aRaw)
}

// MulElemWise performs a element-size multiply operation.
func (v Vector) MulElemWise(b tensor.Vector) {
	aRaw := v.Raw()
	bRaw := b.Raw()

	for i := range aRaw {
		aRaw[i] *= bRaw[i]
	}

	v.Set(aRaw)
}

// DivElemWise performs a element-wise division operation.
func (v Vector) DivElemWise(b tensor.Vector) {
	aRaw := v.Raw()
	bRaw := b.Raw()

	for i := range aRaw {
		aRaw[i] /= bRaw[i]
	}

	v.Set(aRaw)
}

// PowerScalar calculates the power of element element in the vector.
func (v Vector) PowerScalar(alpha float64) {
	aRaw := v.Raw()

	for i := range aRaw {
		aRaw[i] = math.Pow(aRaw[i], alpha)
	}

	v.Set(aRaw)
}
