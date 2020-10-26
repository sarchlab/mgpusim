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

	gemmKernel      *insts.HsaCo
	transposeKernel *insts.HsaCo
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

// CreateTensor creates a new Tensor.
func (to *TensorOperator) CreateTensor(size []int) *Tensor {
	sizeOfFloat := 4
	numElement := 1
	for _, s := range size {
		numElement *= s
	}

	m := &Tensor{
		size: size,
		ptr: to.driver.AllocateMemory(
			to.context, uint64(numElement*sizeOfFloat)),
	}
	return m
}

// Dump prints the tensor content to a string
func (to *TensorOperator) Dump(name string, tensor *Tensor) string {
	sizeOfFloat := 4

	hData := make([]float32, tensor.NumElement()*sizeOfFloat)
	to.driver.MemCopyD2H(to.context, hData, tensor.ptr)

	// currPos := make([]int, len(tensor.size))

	out := fmt.Sprintf("\n\n%s:\n", name)
	for i := 0; i < tensor.NumElement(); i++ {
		out += fmt.Sprintf("%4f, ", hData[i])
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
	M, N, K     int32
	Alpha, Beta float32
	Padding     int32
	A, B, C, D  driver.GPUPtr
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

// Transpose transposes the in Matrix and stores the results in the out Matrix.
func (to *TensorOperator) Transpose(in, out *Tensor) {
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
