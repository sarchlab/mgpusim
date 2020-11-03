package layers

import (
	"fmt"

	"gitlab.com/akita/mgpusim/driver"
	"gitlab.com/akita/mgpusim/insts"
	"gitlab.com/akita/mgpusim/kernels"
)

// TensorOperator can perform operations on tensors.
type TensorOperator struct {
	driver  *driver.Driver
	context *driver.Context

	gemmKernel            *insts.HsaCo
	transposeKernel       *insts.HsaCo
	transposeTensorKernel *insts.HsaCo
}

// NewTensorOperator creates a new tensor operator, injecting depencies // including the GPU driver and the GPU context.
func NewTensorOperator(
	driver *driver.Driver,
	context *driver.Context,
) *TensorOperator {
	to := &TensorOperator{
		driver:  driver,
		context: context,
	}

	to.loadGemmKernel()
	to.loadMatrixTransposeKernel()
	to.loadTransposeTensorKernel()

	return to
}

func (to *TensorOperator) loadGemmKernel() {
	bytes := _escFSMustByte(false, "/gpu_gemm.hsaco")
	to.gemmKernel = kernels.LoadProgramFromMemory(bytes,
		"gemm")
	if to.gemmKernel == nil {
		panic("failed to load femm kernel")
	}
}

func (to *TensorOperator) loadMatrixTransposeKernel() {
	bytes := _escFSMustByte(false, "/trans.hsaco")
	to.transposeKernel = kernels.LoadProgramFromMemory(bytes,
		"Transpose")
	if to.transposeKernel == nil {
		panic("failed to load matrix transpose kernel")
	}
}

func (to *TensorOperator) loadTransposeTensorKernel() {
	bytes := _escFSMustByte(false, "/transpose.hsaco")
	to.transposeTensorKernel = kernels.LoadProgramFromMemory(bytes,
		"transpose_tensor")
	if to.transposeKernel == nil {
		panic("failed to load transpose tensor kernel")
	}
}

// CreateTensor creates a new Tensor.
func (to *TensorOperator) CreateTensor(size []int) *Tensor {
	sizeOfFloat := 4
	numElement := 1
	for _, s := range size {
		numElement *= s
	}

	m := &Tensor{
		driver: to.driver,
		ctx:    to.context,
		size:   size,
		ptr: to.driver.AllocateMemory(
			to.context, uint64(numElement*sizeOfFloat)),
	}

	hostData := make([]float32, numElement)
	to.driver.MemCopyH2D(to.context, m.ptr, hostData)

	return m
}

// CreateTensorWithBuf creates a new tensor without allocating new GPU memory.
func (to *TensorOperator) CreateTensorWithBuf(
	ptr driver.GPUPtr,
	size []int,
) *Tensor {
	t := &Tensor{
		driver: to.driver,
		ctx:    to.context,
		size:   size,
		ptr:    ptr,
	}

	return t
}

// Dump prints the tensor content to a string
func (to *TensorOperator) Dump(name string, tensor *Tensor) string {
	sizeOfFloat := 4

	hData := make([]float32, tensor.NumElement()*sizeOfFloat)
	to.driver.MemCopyD2H(to.context, hData, tensor.ptr)

	// currPos := make([]int, len(tensor.size)+1)
	dimSize := make([]int, tensor.Dim())
	product := 1
	for i := tensor.Dim() - 1; i >= 0; i-- {
		product *= tensor.size[i]
		dimSize[i] = product
	}

	out := fmt.Sprintf("\n\n%s:\n", name)
	indent := 0
	for i := 0; i < tensor.NumElement(); i++ {
		for _, d := range dimSize {
			if i%d == 0 {
				out += "\n"
				for k := 0; k < indent; k++ {
					out += " "
				}
				out += "["
				indent++
			}
		}

		out += fmt.Sprintf("%4f, ", hData[i])

		for _, d := range dimSize {
			if (i+1)%d == 0 {
				out += "],"
				indent--
			}
		}
	}
	out += "\n"

	return out
}

// Free fress the metory of the tensor.
func (to *TensorOperator) Free(t *Tensor) {
	err := to.driver.FreeMemory(to.context, t.ptr)
	if err != nil {
		panic(err)
	}
}

// ToGPU copies the metory to a GPU.
func (to *TensorOperator) ToGPU(m *Tensor, data []float32) {
	to.driver.MemCopyH2D(to.context, m.ptr, data)
}

// FromGPU copiles the data back from the GPU.
func (to *TensorOperator) FromGPU(m *Tensor, data []float32) {
	to.driver.MemCopyD2H(to.context, data, m.ptr)
}

// GemmKernArgs represents the kernel arguments of the gemm operation.
type GemmKernArgs struct {
	M, N, K                   int32
	Alpha, Beta               float32
	Padding                   int32
	A, B, C, D                driver.GPUPtr
	OffsetX, OffsetY, OffsetZ int32
}

// Gemm calculates D = alpha * A * B + beta * C.
func (to *TensorOperator) Gemm(
	transA, transB bool,
	m, n, k int,
	alpha, beta float32,
	matrixA, matrixB, matrixC, matrixD *Tensor,
) {
	blockSize := 16
	wiWidth := uint32(n)
	wiHeight := uint32(m)

	kernArg := GemmKernArgs{
		M:     int32(m),
		N:     int32(n),
		K:     int32(k),
		Alpha: alpha,
		Beta:  beta,
		A:     matrixA.ptr,
		B:     matrixB.ptr,
		C:     matrixC.ptr,
		D:     matrixD.ptr,
	}

	to.driver.LaunchKernel(
		to.context,
		to.gemmKernel,
		[3]uint32{wiWidth, wiHeight, 1},
		[3]uint16{uint16(blockSize), uint16(blockSize), 1},
		&kernArg,
	)
}

// MatrixTransposeKernelArgs represents the kernel arguments of the matrix
// transpose kernel.
type MatrixTransposeKernelArgs struct {
	Input               driver.GPUPtr
	Output              driver.GPUPtr
	OutputWidth         int32
	OutputHeight        int32
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

// TransposeMatrix transposes the in Matrix and stores the results in the out
// Matrix.
func (to *TensorOperator) TransposeMatrix(in, out *Tensor) {
	to.mustBeMatrix(in)
	to.mustBeMatrix(out)

	blockSize := 16
	wiWidth := uint32(out.size[1])
	wiHeight := uint32(out.size[0])

	kernArg := MatrixTransposeKernelArgs{
		Input:               in.ptr,
		Output:              out.ptr,
		OutputWidth:         int32(in.size[0]),
		OutputHeight:        int32(in.size[1]),
		HiddenGlobalOffsetX: 0,
		HiddenGlobalOffsetY: 0,
		HiddenGlobalOffsetZ: 0,
	}

	to.driver.LaunchKernel(
		to.context,
		to.transposeKernel,
		[3]uint32{wiWidth, wiHeight, 1},
		[3]uint16{uint16(blockSize), uint16(blockSize), 1},
		&kernArg,
	)

	out.size = []int{in.size[1], in.size[0]}
}

func (to *TensorOperator) mustBeMatrix(t *Tensor) {
	if t.Dim() != 2 {
		panic("not a matrix")
	}
}

type transposeTensorArgs struct {
	In                        driver.GPUPtr
	Out                       driver.GPUPtr
	InSize                    driver.GPUPtr
	OutSize                   driver.GPUPtr
	Order                     driver.GPUPtr
	InIndexBuf                driver.GPUPtr
	OutIndexBuf               driver.GPUPtr
	Dim                       int32
	Padding                   int32
	OffsetX, OffsetY, OffsetZ int32
}

// TransposeTensor reorders the axis order.
func (to *TensorOperator) TransposeTensor(in, out *Tensor, order []int) {
	sizeOfInt32 := int32(4)
	dim := int32(in.Dim())
	hOrder := make([]int32, dim)
	hInSize := make([]int32, dim)
	hOutSize := make([]int32, dim)

	for i := int32(0); i < dim; i++ {
		hOrder[i] = int32(order[i])
		hInSize[i] = int32(in.size[i])
		hOutSize[i] = int32(out.size[i])
	}

	dOrder := to.driver.AllocateMemory(to.context, uint64(dim*sizeOfInt32))
	to.driver.MemCopyH2D(to.context, dOrder, hOrder)
	defer to.driver.FreeMemory(to.context, dOrder)

	dInSize := to.driver.AllocateMemory(to.context, uint64(dim*sizeOfInt32))
	to.driver.MemCopyH2D(to.context, dInSize, hInSize)
	defer to.driver.FreeMemory(to.context, dInSize)

	dOutSize := to.driver.AllocateMemory(to.context, uint64(dim*sizeOfInt32))
	to.driver.MemCopyH2D(to.context, dOutSize, hOutSize)
	defer to.driver.FreeMemory(to.context, dOutSize)

	dInIndexBuf := to.driver.AllocateMemory(to.context,
		uint64(int32(in.NumElement())*dim*sizeOfInt32))
	defer to.driver.FreeMemory(to.context, dInIndexBuf)

	dOutIndexBuf := to.driver.AllocateMemory(to.context,
		uint64(int32(in.NumElement())*dim*sizeOfInt32))
	defer to.driver.FreeMemory(to.context, dOutIndexBuf)

	args := transposeTensorArgs{
		In:          in.ptr,
		Out:         out.ptr,
		InSize:      dInSize,
		OutSize:     dOutSize,
		Order:       dOrder,
		InIndexBuf:  dInIndexBuf,
		OutIndexBuf: dOutIndexBuf,
		Dim:         dim,
	}

	to.driver.LaunchKernel(
		to.context,
		to.transposeTensorKernel,
		[3]uint32{uint32(in.NumElement()), 1, 1},
		[3]uint16{uint16(64), 1, 1},
		&args,
	)

	out.descriptor = ""
	for i := 0; i < len(in.descriptor); i++ {
		out.descriptor += string(in.descriptor[order[i]])
	}
}

// Rotate180 will rotate the lowest two dimensions by 180 degree.
func (to *TensorOperator) Rotate180(in, out *Tensor) {
	inV := in.Vector()
	outV := make([]float64, len(inV))
	for i := 0; i < len(inV); i++ {
		outIndex := i
		outPos := make([]int, len(in.size))
		inPos := make([]int, len(in.size))

		sizeLeft := outIndex
		accumulatedLength := 1
		for d := 0; d < len(in.size); d++ {
			p := sizeLeft % in.size[len(in.size)-d-1]
			sizeLeft /= in.size[len(in.size)-d-1]
			outPos[len(in.size)-d-1] = p
		}

		for d := 0; d < len(in.size); d++ {
			if d < len(in.size)-2 {
				inPos[d] = outPos[d]
			} else {
				inPos[d] = in.size[d] - outPos[d] - 1
			}
		}

		inIndex := 0
		accumulatedLength = 1
		for d := 0; d < len(in.size); d++ {
			inIndex += inPos[len(in.size)-d-1] * accumulatedLength
			accumulatedLength *= in.size[len(in.size)-d-1]
		}

		outV[outIndex] = inV[inIndex]
	}

	out.Init(outV, in.size)
}
