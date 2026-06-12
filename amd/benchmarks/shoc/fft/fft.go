// Package fft include the benchmark of Fourier
package fft

import (
	"log"
	"math/rand"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// Float2 is two floats
type Float2 struct {
	X, Y float32
}

// KernelArgs defines kernel arguments for CDNA3 (GFX942).
// The CDNA3 kernel uses __shared__ memory, so no Smem parameter.
type KernelArgs struct {
	Work                driver.Ptr
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

// Benchmark defines a benchmark
type Benchmark struct {
	driver           *driver.Driver
	context          *driver.Context
	gpus             []int
	queues           []*driver.CommandQueue
	useUnifiedMemory bool
	fftKernel        *insts.KernelCodeObject

	Bytes      int64
	BytesMode  bool
	Passes     int32
	halfNFfts  int64
	nFfts      int64
	halfNCmplx int64
	usedBytes  uint64
	dSource    driver.Ptr
	source     []Float2
	result     []Float2
}

// NewBenchmark returns a benchmark
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	return b
}

// SelectGPU selects GPU
func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
}

// SetUnifiedMemory uses Unified Memory
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

func (b *Benchmark) loadProgram() {
	b.fftKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "fft1D_512")
	if b.fftKernel == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// Run runs
func (b *Benchmark) Run() {
	b.loadProgram()

	for _, gpu := range b.gpus {
		b.driver.SelectGPU(b.context, gpu)
		b.queues = append(b.queues,
			b.driver.CreateCommandQueue(b.context))
	}

	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	if !b.BytesMode {
		b.Bytes = b.Bytes * 1024 * 1024
	}
	b.halfNFfts = b.Bytes / (512 * 4 * 2 * 2)
	b.nFfts = b.halfNFfts * 2
	b.halfNCmplx = b.halfNFfts * 512
	b.usedBytes = uint64(b.halfNCmplx) * 2 * 4 * 2

	b.source = make([]Float2, b.usedBytes>>3)
	b.result = make([]Float2, b.usedBytes>>3)
	b.fill()

	if b.useUnifiedMemory {
		b.dSource = b.driver.AllocateUnifiedMemory(b.context,
			b.usedBytes)
	} else {
		b.dSource = b.driver.AllocateMemory(b.context,
			b.usedBytes)
	}
	b.driver.MemCopyH2D(b.context, b.dSource, b.source)
}

func (b *Benchmark) exec() {
	localWorkSize := int64(64)
	vectorGlobalWSize := localWorkSize * b.nFfts

	globalSize := [3]uint32{uint32(vectorGlobalWSize), 1, 1}
	localSize := [3]uint16{uint16(localWorkSize), 1, 1}

	for k := int32(0); k < b.Passes; k++ {
		args := KernelArgs{
			Work: b.dSource,
		}

		b.driver.LaunchKernel(b.context,
			b.fftKernel,
			globalSize, localSize,
			&args,
		)
	}
	b.driver.MemCopyD2H(b.context, b.result, b.dSource)
}

// Verify verifies
func (b *Benchmark) Verify() {
	mismatch := false

	if b.fftCPU() == 1 {
		mismatch = true
	}

	if mismatch {
		panic("Mismatch!\n")
	}
	log.Printf("Passed!\n")
}

func (b *Benchmark) fftCPU() int32 {
	fail := int32(0)
	fst := make([]Float2, b.nFfts<<6)
	snd := make([]Float2, b.nFfts<<6)
	for i := int64(0); i < (b.nFfts << 6); i++ {
		fst[i] = b.source[i]
	}

	for i := int64(0); i < (b.nFfts << 6); i++ {
		snd[i] = b.source[b.halfNCmplx+i]
	}

	for i := int64(0); i < (b.nFfts << 6); i++ {
		if fst[i].X != snd[i].X || fst[i].Y != snd[i].Y {
			fail = 1
		}
	}
	return fail
}

func (b *Benchmark) fill() {
	rand.Seed(1)

	for i := int64(0); i < b.halfNCmplx; i++ {
		b.source[i].X = (rand.Float32())*2 - 1
		b.source[i].Y = (rand.Float32())*2 - 1
		b.source[i+b.halfNCmplx].X = b.source[i].X
		b.source[i+b.halfNCmplx].Y = b.source[i].Y
	}
}
