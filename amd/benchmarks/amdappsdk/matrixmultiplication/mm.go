package matrixmultiplication

import (
	"log"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/arch"
	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
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
	kernel           *insts.KernelCodeObject
	Arch             arch.Type
	useUnifiedMemory bool
	blockABuf        driver.Ptr
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

// KernelArgs defines kernel arguments for GCN3
type KernelArgs struct {
	MatrixA             driver.Ptr
	MatrixB             driver.Ptr
	MatrixC             driver.Ptr
	WidthA              uint32
	BlockA              driver.LocalPtr
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

// CDNA3KernelArgs defines kernel arguments for CDNA3 architecture (GFX942)
type CDNA3KernelArgs struct {
	MatrixA             driver.Ptr
	MatrixB             driver.Ptr
	MatrixC             driver.Ptr
	WidthA              uint32
	Padding1            uint32
	BlockA              driver.Ptr
	HiddenBlockCountX   uint32
	HiddenBlockCountY   uint32
	HiddenBlockCountZ   uint32
	HiddenGroupSizeX    uint16
	HiddenGroupSizeY    uint16
	HiddenGroupSizeZ    uint16
	HiddenRemainderX    uint16
	HiddenRemainderY    uint16
	HiddenRemainderZ    uint16
	Padding2            [16]byte
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
	HiddenGridDims      uint16
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

func (m *GPUMatrixMultiplier) launchKernel( //nolint:funlen
	gA, gB, gC driver.Ptr,
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

		globalSizeX := uint32(width)
		globalSizeY := uint32(height)
		localSizeX := uint16(8)
		localSizeY := uint16(8)

		if m.Arch == arch.CDNA3 {
			if m.blockABuf == 0 {
				m.blockABuf = m.driver.AllocateMemory(m.context,
					uint64(32*32*4))
			}
			kernArgs := &CDNA3KernelArgs{
				MatrixA:             gA,
				MatrixB:             gB,
				MatrixC:             gC,
				WidthA:              mA.Width,
				BlockA:              m.blockABuf,
				HiddenBlockCountX:   globalSizeX / uint32(localSizeX),
				HiddenBlockCountY:   globalSizeY / uint32(localSizeY),
				HiddenBlockCountZ:   1,
				HiddenGroupSizeX:    localSizeX,
				HiddenGroupSizeY:    localSizeY,
				HiddenGroupSizeZ:    1,
				HiddenRemainderX:    uint16(globalSizeX % uint32(localSizeX)),
				HiddenRemainderY:    uint16(globalSizeY % uint32(localSizeY)),
				HiddenRemainderZ:    0,
				HiddenGlobalOffsetX: 0,
				HiddenGlobalOffsetY: int64(height * i),
				HiddenGlobalOffsetZ: 0,
				HiddenGridDims:      2,
			}
			m.driver.EnqueueLaunchKernel(
				q,
				m.kernel,
				[3]uint32{globalSizeX, globalSizeY, 1},
				[3]uint16{localSizeX, localSizeY, 1},
				kernArgs,
			)
		} else {
			kernArgs := &KernelArgs{
				gA, gB, gC,
				mA.Width,
				32 * 32 * 4,
				0, int64(height * i), 0,
			}
			m.driver.EnqueueLaunchKernel(
				q,
				m.kernel,
				[3]uint32{globalSizeX, globalSizeY, 1},
				[3]uint16{localSizeX, localSizeY, 1},
				kernArgs,
			)
		}
	}

	for _, q := range queues {
		m.driver.DrainCommandQueue(q)
	}
}

func (m *GPUMatrixMultiplier) initMemory(
	mA, mB, mC *Matrix,
) (driver.Ptr, driver.Ptr, driver.Ptr) {
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
	gm driver.Ptr,
) {
	m.driver.MemCopyD2H(m.context, matrix.Data, gm)
}

//go:embed kernels.hsaco
var hsacoBytes []byte

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

func (m *GPUMatrixMultiplier) loadKernel() {
	var kernelBytes []byte
	if m.Arch == arch.CDNA3 {
		kernelBytes = cdna3HSACOBytes
	} else {
		kernelBytes = hsacoBytes
	}

	m.kernel = insts.LoadKernelCodeObjectFromBytes(kernelBytes, "mmmKernel_local")
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
