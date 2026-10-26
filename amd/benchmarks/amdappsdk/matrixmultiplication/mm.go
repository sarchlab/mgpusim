package matrixmultiplication

import (
	// embed hsaco files
	_ "embed"
	"log"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

// A MatrixMultiplier is a service type that can calculate the result of matrix
// -matrix multiplication.
type MatrixMultiplier interface {
	Multiply(mA, mB *Matrix) *Matrix
}

// KernelArgs defines kernel arguments for GCN3
type KernelArgs struct {
	MatrixA             gpuruntime.Ptr
	MatrixB             gpuruntime.Ptr
	MatrixC             gpuruntime.Ptr
	WidthA              uint32
	BlockA              gpuruntime.LocalPtr
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

// CDNA3KernelArgs defines kernel arguments for CDNA3 architecture (GFX942)
type CDNA3KernelArgs struct {
	MatrixA             gpuruntime.Ptr
	MatrixB             gpuruntime.Ptr
	MatrixC             gpuruntime.Ptr
	WidthA              uint32
	Padding1            uint32
	BlockA              gpuruntime.Ptr
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

// COV5KernelArgs defines kernel arguments for a Code Object V5 build of the
// GCN3 (gfx803) kernel. The explicit arguments are identical to KernelArgs
// (BlockA is still an LDS/local pointer), but modern LLVM emits the richer
// COV5 hidden-argument block (block counts, group sizes, remainders, grid
// dims) that the kernel reads for get_local_size()/get_num_groups(), rather
// than the three global-offset words used by the legacy COV2 build. Field
// order and sizes are chosen so binary.Write packs them at the byte offsets
// the compiled kernel expects (matrixA@0, widthA@24, blockA@28, block
// counts@32, group sizes@44, remainders@50, global offsets@72, grid dims@96).
type COV5KernelArgs struct {
	MatrixA             gpuruntime.Ptr
	MatrixB             gpuruntime.Ptr
	MatrixC             gpuruntime.Ptr
	WidthA              uint32
	BlockA              gpuruntime.LocalPtr
	HiddenBlockCountX   uint32
	HiddenBlockCountY   uint32
	HiddenBlockCountZ   uint32
	HiddenGroupSizeX    uint16
	HiddenGroupSizeY    uint16
	HiddenGroupSizeZ    uint16
	HiddenRemainderX    uint16
	HiddenRemainderY    uint16
	HiddenRemainderZ    uint16
	Padding             [16]byte
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
	HiddenGridDims      uint16
}

func (m *Benchmark) launchKernel( //nolint:funlen
	gA, gB, gC gpuruntime.Ptr,
	mA *Matrix,
	mC *Matrix,
) {
	queues := make([]gpuruntime.CommandQueue, len(m.Gpus))

	for i, gpu := range m.Gpus {
		m.rt.SelectGPU(m.Context, gpu)
		q := m.rt.CreateCommandQueue(m.Context)

		queues[i] = q

		width := int(mC.Width) / 4
		height := int(mC.Height) / 4 / len(m.Gpus)

		globalSizeX := uint32(width)
		globalSizeY := uint32(height)
		localSizeX := uint16(8)
		localSizeY := uint16(8)

		if m.Arch == arch.CDNA3 {
			if m.BlockABuf == 0 {
				m.BlockABuf = m.rt.AllocateMemory(m.Context,
					uint64(32*32*4))
			}
			kernArgs := &CDNA3KernelArgs{
				MatrixA:             gA,
				MatrixB:             gB,
				MatrixC:             gC,
				WidthA:              mA.Width,
				BlockA:              m.BlockABuf,
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
			m.rt.EnqueueLaunchKernel(
				q,
				m.kernel,
				[3]uint32{globalSizeX, globalSizeY, 1},
				[3]uint16{localSizeX, localSizeY, 1},
				kernArgs,
			)
		} else if m.kernel.Version == insts.CodeObjectV5 {
			kernArgs := &COV5KernelArgs{
				MatrixA:             gA,
				MatrixB:             gB,
				MatrixC:             gC,
				WidthA:              mA.Width,
				BlockA:              32 * 32 * 4,
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
			m.rt.EnqueueLaunchKernel(
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
			m.rt.EnqueueLaunchKernel(
				q,
				m.kernel,
				[3]uint32{globalSizeX, globalSizeY, 1},
				[3]uint16{localSizeX, localSizeY, 1},
				kernArgs,
			)
		}
	}
}

func (m *Benchmark) initMemory(
	mA, mB, mC *Matrix,
) (gpuruntime.Ptr, gpuruntime.Ptr, gpuruntime.Ptr) {
	if m.UseUnifiedMemory {
		gA := m.rt.AllocateUnifiedMemory(m.Context, uint64(mA.Width*mA.Height*4))
		gB := m.rt.AllocateUnifiedMemory(m.Context, uint64(mB.Width*mB.Height*4))
		gC := m.rt.AllocateUnifiedMemory(m.Context, uint64(mC.Width*mC.Height*4))
		m.Upload(m.rt, m.Queue, gA, mA.Data)
		m.Upload(m.rt, m.Queue, gB, mB.Data)

		return gA, gB, gC
	}
	gA := m.rt.AllocateMemory(m.Context, uint64(mA.Width*mA.Height*4))
	m.rt.Distribute(m.Context, gA, uint64(mA.Width*mA.Height*4), m.Gpus)

	gB := m.rt.AllocateMemory(m.Context, uint64(mB.Width*mB.Height*4))
	m.rt.Distribute(m.Context, gB, uint64(mB.Width*mB.Height*4), m.Gpus)

	gC := m.rt.AllocateMemory(m.Context, uint64(mC.Width*mC.Height*4))
	m.rt.Distribute(m.Context, gC, uint64(mC.Width*mC.Height*4), m.Gpus)
	m.Upload(m.rt, m.Queue, gA, mA.Data)
	m.Upload(m.rt, m.Queue, gB, mB.Data)

	return gA, gB, gC
}

//go:embed kernels.hsaco
var hsacoBytes []byte

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

func (m *Benchmark) loadKernel() {
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
