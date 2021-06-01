package matrixmultiplication

import (
	"log"

	// embed hsaco files
	_ "embed"

	"gitlab.com/akita/mgpusim/v2/driver"
	"gitlab.com/akita/mgpusim/v2/insts"
	"gitlab.com/akita/mgpusim/v2/kernels"
)

// A MatrixMultiplier is a service type that can calculate the result of matrix
// -matrix multiplication.
type MatrixMultiplier interface {
	Multiply(mA, mB *Matrix) *Matrix
}

// A GPUMatrixMultiplier is a MatrixMultiplier that runs the
// MatrixMultiplication on GCN3 simulator.
type GPUMatrixMultiplier struct {
	driver           *driver.Driver
	context          *driver.Context
	gpus             []int
	kernel           *insts.HsaCo
	useUnifiedMemory bool
}

// NewGPUMatrixMultiplier creates a new GPUMatrixMultiplier, injecting the
// dependency of driver and the GPU context.
func NewGPUMatrixMultiplier(
	gpuDriver *driver.Driver,
	context *driver.Context,
) *GPUMatrixMultiplier {
	m := &GPUMatrixMultiplier{
		driver:  gpuDriver,
		context: context,
	}
	return m
}

// SelectGPU selects GPU
func (m *GPUMatrixMultiplier) SelectGPU(gpus []int) {
	m.gpus = gpus
}

// KernelArgs defines kernel arguments
type KernelArgs struct {
	MatrixA             driver.GPUPtr
	MatrixB             driver.GPUPtr
	MatrixC             driver.GPUPtr
	WidthA              uint32
	BlockA              driver.LocalPtr
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

// Multiply multiplies two matrice
func (m *GPUMatrixMultiplier) Multiply(mA, mB *Matrix) *Matrix {
	mC := new(Matrix)
	mC.Width = mB.Width
	mC.Height = mA.Height
	mC.Data = make([]float32, mC.Width*mC.Height)

	m.loadKernel()
	gA, gB, gC := m.initMemory(mA, mB, mC)
	m.launchKernel(gA, gB, gC, mA, mC)
	m.copyDataBackFromGPU(mC, gC)

	return mC
}

func (m *GPUMatrixMultiplier) launchKernel(
	gA, gB, gC driver.GPUPtr,
	mA *Matrix,
	mC *Matrix,
) {
	queues := make([]*driver.CommandQueue, len(m.gpus))

	for i, gpu := range m.gpus {
		m.driver.SelectGPU(m.context, gpu)
		q := m.driver.CreateCommandQueue(m.context)

		queues[i] = q

		width := int(mC.Width) / 4
		height := int(mC.Height) / 4 / len(m.gpus)

		kernArgs := &KernelArgs{
			gA, gB, gC,
			mA.Width,
			32 * 32 * 4,
			0, int64(height * i), 0,
		}
		m.driver.EnqueueLaunchKernel(
			q,
			m.kernel,
			[3]uint32{uint32(width), uint32(height), 1},
			[3]uint16{8, 8, 1},
			kernArgs,
		)
	}

	for _, q := range queues {
		m.driver.DrainCommandQueue(q)
	}
}

func (m *GPUMatrixMultiplier) initMemory(
	mA, mB, mC *Matrix,
) (driver.GPUPtr, driver.GPUPtr, driver.GPUPtr) {
	if m.useUnifiedMemory {
		gA := m.driver.AllocateUnifiedMemory(m.context, uint64(mA.Width*mA.Height*4))
		gB := m.driver.AllocateUnifiedMemory(m.context, uint64(mB.Width*mB.Height*4))
		gC := m.driver.AllocateUnifiedMemory(m.context, uint64(mC.Width*mC.Height*4))
		m.driver.MemCopyH2D(m.context, gA, mA.Data)
		m.driver.MemCopyH2D(m.context, gB, mB.Data)

		return gA, gB, gC
	}
	gA := m.driver.AllocateMemory(m.context, uint64(mA.Width*mA.Height*4))
	m.driver.Distribute(m.context, gA, uint64(mA.Width*mA.Height*4), m.gpus)

	gB := m.driver.AllocateMemory(m.context, uint64(mB.Width*mB.Height*4))
	m.driver.Distribute(m.context, gB, uint64(mB.Width*mB.Height*4), m.gpus)

	gC := m.driver.AllocateMemory(m.context, uint64(mC.Width*mC.Height*4))
	m.driver.Distribute(m.context, gC, uint64(mC.Width*mC.Height*4), m.gpus)
	m.driver.MemCopyH2D(m.context, gA, mA.Data)
	m.driver.MemCopyH2D(m.context, gB, mB.Data)

	return gA, gB, gC
}

func (m *GPUMatrixMultiplier) copyDataBackFromGPU(
	matrix *Matrix,
	gm driver.GPUPtr,
) {
	m.driver.MemCopyD2H(m.context, matrix.Data, gm)
}

//go:embed kernels.hsaco
var hsacoBytes []byte

func (m *GPUMatrixMultiplier) loadKernel() {
	m.kernel = kernels.LoadProgramFromMemory(hsacoBytes, "mmmKernel_local")
	if m.kernel == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// CPUMatrixMultiplier is a matrix multiplier
type CPUMatrixMultiplier struct{}

// Multiply multiplies two matrice
func (m *CPUMatrixMultiplier) Multiply(mA, mB *Matrix) *Matrix {
	if mA.Width != mB.Height {
		log.Panic("matrix dimension mismatch")
	}

	mC := new(Matrix)
	mC.Width = mB.Width
	mC.Height = mA.Height
	mC.Data = make([]float32, mC.Width*mC.Height)

	for x := uint32(0); x < mC.Width; x++ {
		for y := uint32(0); y < mC.Height; y++ {
			indexC := y*mC.Width + x

			sum := float32(0)
			for i := uint32(0); i < mA.Width; i++ {
				indexA := y*mA.Width + i
				indexB := i*mB.Width + x
				sum += mA.Data[indexA] * mB.Data[indexB]
			}

			mC.Data[indexC] = sum
		}
	}

	return mC
}
