// Package fft include the benchmark of Fourier
package fft

import (
	"log"
	"math/rand"

	"gitlab.com/akita/mgpusim/v2/driver"
	"gitlab.com/akita/mgpusim/v2/insts"
	"gitlab.com/akita/mgpusim/v2/kernels"
)

// Float2 is two floats
type Float2 struct {
	X, Y float32
}

// KernelArgs defines kernel arguments
type KernelArgs struct {
	Work                driver.GPUPtr
	Smem                driver.LocalPtr
	Paddinng            int32
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

// Benchmark defines a benchmark
type Benchmark struct {
	driver           *driver.Driver
	context          *driver.Context
	gpus             []int
	queues           []*driver.CommandQueue
	useUnifiedMemory bool
	fftKernel        *insts.HsaCo

	Bytes      int32
	Passes     int32
	halfNFfts  int32
	nFfts      int32
	halfNCmplx int32
	usedBytes  uint64
	dSource    driver.GPUPtr
	source     []Float2
	result     []Float2
}

// NewBenchmark returns a benchmark
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.loadProgram()
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

func (b *Benchmark) loadProgram() {
	hsacoBytes := _escFSMustByte(false, "/fft.hsaco")

	b.fftKernel = kernels.LoadProgramFromMemory(hsacoBytes, "fft1D_512")
	if b.fftKernel == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// Run runs
func (b *Benchmark) Run() {
	for _, gpu := range b.gpus {
		b.driver.SelectGPU(b.context, gpu)
		b.queues = append(b.queues, b.driver.CreateCommandQueue(b.context))
	}

	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	b.Bytes = b.Bytes * 1024 * 1024
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
	localWorkSize := int32(64)
	vectorGlobalWSize := localWorkSize * b.nFfts

	globalSize := [3]uint32{uint32(vectorGlobalWSize), 1, 1}
	localSize := [3]uint16{uint16(localWorkSize), 1, 1}

	for k := int32(0); k < b.Passes; k++ {
		args := KernelArgs{
			Work:                b.dSource,
			Smem:                8 * 8 * 9 * 8,
			Paddinng:            0,
			HiddenGlobalOffsetX: 0,
			HiddenGlobalOffsetY: 0,
			HiddenGlobalOffsetZ: 0,
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
	for i := int32(0); i < (b.nFfts << 6); i++ {
		fst[i] = b.source[i]
	}

	for i := int32(0); i < (b.nFfts << 6); i++ {
		snd[i] = b.source[b.halfNCmplx+i]
	}

	for i := int32(0); i < (b.nFfts << 6); i++ {
		if fst[i].X != snd[i].X || fst[i].Y != snd[i].Y {
			fail = 1
		}
	}
	return fail
}

func (b *Benchmark) fill() {
	rand.Seed(1)

	for i := int32(0); i < b.halfNCmplx; i++ {
		b.source[i].X = (rand.Float32())*2 - 1
		b.source[i].Y = (rand.Float32())*2 - 1
		b.source[i+b.halfNCmplx].X = b.source[i].X
		b.source[i+b.halfNCmplx].Y = b.source[i].Y
	}
}
