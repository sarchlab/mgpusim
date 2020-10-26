package layers

import (
	"fmt"

	"gitlab.com/akita/mgpusim/driver"
	"gitlab.com/akita/mgpusim/insts"
	"gitlab.com/akita/mgpusim/kernels"
)

// A Matrix is a mtrix.
type Matrix struct {
	col, row int
	data     driver.GPUPtr
}

// MatrixOperator can perform matrix operations.
type MatrixOperator struct {
	driver  *driver.Driver
	context *driver.Context

	gemmKernel      *insts.HsaCo
	transposeKernel *insts.HsaCo
}

// NewMatrixOperator creates a new matrix operator.
func NewMatrixOperator(
	driver *driver.Driver,
	context *driver.Context,
) *MatrixOperator {
	mo := &MatrixOperator{
		driver:  driver,
		context: context,
	}

	mo.loadGemmKernel()
	mo.loadMatrixTransposeKernel()

	return mo
}

func (mo *MatrixOperator) loadGemmKernel() {
	bytes := _escFSMustByte(false, "/gpu_gemm.hsaco")
	mo.gemmKernel = kernels.LoadProgramFromMemory(bytes,
		"gemm")
	if mo.gemmKernel == nil {
		panic("failed to load femm kernel")
	}
}

func (mo *MatrixOperator) loadMatrixTransposeKernel() {
	bytes := _escFSMustByte(false, "/trans.hsaco")
	mo.transposeKernel = kernels.LoadProgramFromMemory(bytes,
		"Transpose")
	if mo.transposeKernel == nil {
		panic("failed to load matrix transpose kernel")
	}
}

// CreateMatrix creates a new Matrix.
func (mo *MatrixOperator) CreateMatrix(row, col int) *Matrix {
	m := &Matrix{
		row:  row,
		col:  col,
		data: mo.driver.AllocateMemory(mo.context, uint64(row*col*4)),
	}
	return m
}

// Dump prints the matrix content to a string
func (mo *MatrixOperator) Dump(name string, matrix *Matrix) string {
	sizeOfFloat := 4
	hData := make([]float32, matrix.col*matrix.row*sizeOfFloat)
	mo.driver.MemCopyD2H(mo.context, hData, matrix.data)

	out := fmt.Sprintf("\n\n%s:\n", name)
	for i := 0; i < matrix.row; i++ {
		for j := 0; j < matrix.col; j++ {
			out += fmt.Sprintf("%4f, ", hData[i*matrix.col+j])
		}
		out += "\n"
	}
	out += "\n"

	return out
}

// Free fress the memory of the matrix.
func (mo *MatrixOperator) Free(m *Matrix) {
	err := mo.driver.FreeMemory(mo.context, m.data)
	if err != nil {
		panic(err)
	}
}

// ToGPU copies the memory to a GPU.
func (mo *MatrixOperator) ToGPU(m *Matrix, data []float32) {
	mo.driver.MemCopyH2D(mo.context, m.data, data)
}

// FromGPU copiles the data back from the GPU.
func (mo *MatrixOperator) FromGPU(m *Matrix, data []float32) {
	mo.driver.MemCopyD2H(mo.context, data, m.data)
}

// GemmKernArgs represents the kernel arguments of the gemm operation.
type GemmKernArgs struct {
	M, N, K     int32
	Alpha, Beta float32
	Padding     int32
	A, B, C, D  driver.GPUPtr
}

// Gemm calculates D = alpha * A * B + beta * C.
func (mo *MatrixOperator) Gemm(
	transA, transB bool,
	m, n, k int,
	alpha, beta float32,
	matrixA, matrixB, matrixC, matrixD *Matrix,
) {
	queue := mo.driver.CreateCommandQueue(mo.context)

	blockSize := 16
	wiWidth := uint32(n)
	wiHeight := uint32(m)

	kernArg := GemmKernArgs{
		M:     int32(m),
		N:     int32(n),
		K:     int32(k),
		Alpha: alpha,
		Beta:  beta,
		A:     matrixA.data,
		B:     matrixB.data,
		C:     matrixC.data,
		D:     matrixD.data,
	}

	mo.driver.EnqueueLaunchKernel(
		queue,
		mo.gemmKernel,
		[3]uint32{wiWidth, wiHeight, 1},
		[3]uint16{uint16(blockSize), uint16(blockSize), 1},
		&kernArg,
	)

	mo.driver.DrainCommandQueue(queue)
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
func (mo *MatrixOperator) Transpose(in, out *Matrix) {
	queue := mo.driver.CreateCommandQueue(mo.context)

	blockSize := 16
	wiWidth := uint32(out.col)
	wiHeight := uint32(out.row)

	kernArg := MatrixTransposeKernelArgs{
		Input:               in.data,
		Output:              out.data,
		OutputWidth:         int32(in.row),
		OutputHeight:        int32(in.col),
		HiddenGlobalOffsetX: 0,
		HiddenGlobalOffsetY: 0,
		HiddenGlobalOffsetZ: 0,
	}

	mo.driver.EnqueueLaunchKernel(
		queue,
		mo.transposeKernel,
		[3]uint32{wiWidth, wiHeight, 1},
		[3]uint16{uint16(blockSize), uint16(blockSize), 1},
		&kernArg,
	)

	mo.driver.DrainCommandQueue(queue)

	out.col = in.row
	out.row = in.col
}
