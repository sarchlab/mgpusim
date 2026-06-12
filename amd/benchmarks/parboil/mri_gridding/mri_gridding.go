// Package mri_gridding implements the Parboil MRI Gridding benchmark.
// Reconstructs MRI data via scatter gridding using a Kaiser-Bessel window.
package mri_gridding

import (
	"log"
	"math/rand"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// Matches: gridding_kernel(samples_x, samples_y, samples_real, samples_imag,
//
//	grid_real, grid_imag, num_samples, grid_size, kernel_width, beta)
// KernelArgs defines kernel arguments.
// KernelArgs defines the explicit kernel arguments for gridding_kernel.
// V5 implicit args (256 bytes) are filled automatically by the driver.
type KernelArgs struct {
	SamplesX    driver.Ptr
	SamplesY    driver.Ptr
	SamplesReal driver.Ptr
	SamplesImag driver.Ptr
	GridReal    driver.Ptr
	GridImag    driver.Ptr
	NumSamples  int32
	GridSize    int32
	KernelWidth int32
	Beta        float32
}

// Benchmark defines the Parboil MRI Gridding benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	griddingKernel *insts.KernelCodeObject

	NumSamples  int
	GridSize    int
	KernelWidth int
	BlockSize   int

	useUnifiedMemory bool

	hSamplesX    []float32
	hSamplesY    []float32
	hSamplesReal []float32
	hSamplesImag []float32
	hGridReal    []float32
	hGridImag    []float32

	dSamplesX    driver.Ptr
	dSamplesY    driver.Ptr
	dSamplesReal driver.Ptr
	dSamplesImag driver.Ptr
	dGridReal    driver.Ptr
	dGridImag    driver.Ptr
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

// NewBenchmark creates a new Parboil MRI Gridding benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.NumSamples = 65536
	b.GridSize = 256
	b.KernelWidth = 4
	b.BlockSize = 256
	return b
}

// SelectGPU selects GPU.
func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
}

// SetUnifiedMemory uses Unified Memory.
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

func (b *Benchmark) loadProgram() {
	b.griddingKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "gridding_kernel")
	if b.griddingKernel == nil {
		log.Panic("Failed to load gridding_kernel kernel binary")
	}
}

// Run runs the benchmark.
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
	rng := rand.New(rand.NewSource(42)) //nolint:gosec

	b.hSamplesX = make([]float32, b.NumSamples)
	b.hSamplesY = make([]float32, b.NumSamples)
	b.hSamplesReal = make([]float32, b.NumSamples)
	b.hSamplesImag = make([]float32, b.NumSamples)
	b.hGridReal = make([]float32, b.GridSize*b.GridSize)
	b.hGridImag = make([]float32, b.GridSize*b.GridSize)

	gsf := float32(b.GridSize - 1)
	for i := 0; i < b.NumSamples; i++ {
		b.hSamplesX[i] = rng.Float32() * gsf
		b.hSamplesY[i] = rng.Float32() * gsf
		b.hSamplesReal[i] = rng.Float32()*2.0 - 1.0
		b.hSamplesImag[i] = rng.Float32()*2.0 - 1.0
	}

	floatBytes := uint64(b.NumSamples) * 4
	gridBytes := uint64(b.GridSize*b.GridSize) * 4

	if b.useUnifiedMemory {
		b.dSamplesX = b.driver.AllocateUnifiedMemory(b.context, floatBytes)
		b.dSamplesY = b.driver.AllocateUnifiedMemory(b.context, floatBytes)
		b.dSamplesReal = b.driver.AllocateUnifiedMemory(b.context, floatBytes)
		b.dSamplesImag = b.driver.AllocateUnifiedMemory(b.context, floatBytes)
		b.dGridReal = b.driver.AllocateUnifiedMemory(b.context, gridBytes)
		b.dGridImag = b.driver.AllocateUnifiedMemory(b.context, gridBytes)
	} else {
		b.dSamplesX = b.driver.AllocateMemory(b.context, floatBytes)
		b.dSamplesY = b.driver.AllocateMemory(b.context, floatBytes)
		b.dSamplesReal = b.driver.AllocateMemory(b.context, floatBytes)
		b.dSamplesImag = b.driver.AllocateMemory(b.context, floatBytes)
		b.dGridReal = b.driver.AllocateMemory(b.context, gridBytes)
		b.dGridImag = b.driver.AllocateMemory(b.context, gridBytes)
	}

	b.driver.MemCopyH2D(b.context, b.dSamplesX, b.hSamplesX)
	b.driver.MemCopyH2D(b.context, b.dSamplesY, b.hSamplesY)
	b.driver.MemCopyH2D(b.context, b.dSamplesReal, b.hSamplesReal)
	b.driver.MemCopyH2D(b.context, b.dSamplesImag, b.hSamplesImag)
	b.driver.MemCopyH2D(b.context, b.dGridReal, b.hGridReal)
	b.driver.MemCopyH2D(b.context, b.dGridImag, b.hGridImag)
}

func (b *Benchmark) exec() {
	numBlocks := (b.NumSamples + b.BlockSize - 1) / b.BlockSize
	globalSize := [3]uint32{uint32(numBlocks * b.BlockSize), 1, 1}
	localSize := [3]uint16{uint16(b.BlockSize), 1, 1}

	beta := 5.0 * float32(b.KernelWidth) / 4.0

	args := KernelArgs{
		SamplesX:    b.dSamplesX,
		SamplesY:    b.dSamplesY,
		SamplesReal: b.dSamplesReal,
		SamplesImag: b.dSamplesImag,
		GridReal:    b.dGridReal,
		GridImag:    b.dGridImag,
		NumSamples:  int32(b.NumSamples),
		GridSize:    int32(b.GridSize),
		KernelWidth: int32(b.KernelWidth),
		Beta:        beta,
	}
	b.driver.LaunchKernel(b.context,
		b.griddingKernel,
		globalSize, localSize,
		&args,
	)

	b.driver.MemCopyD2H(b.context, b.hGridReal, b.dGridReal)
	b.driver.MemCopyD2H(b.context, b.hGridImag, b.dGridImag)
}

// Verify checks that the output grid contains non-zero values.
func (b *Benchmark) Verify() {
	var sumReal, sumImag float64
	for i := 0; i < b.GridSize*b.GridSize; i++ {
		if b.hGridReal[i] < 0 {
			sumReal -= float64(b.hGridReal[i])
		} else {
			sumReal += float64(b.hGridReal[i])
		}
		if b.hGridImag[i] < 0 {
			sumImag -= float64(b.hGridImag[i])
		} else {
			sumImag += float64(b.hGridImag[i])
		}
	}

	if sumReal == 0 && sumImag == 0 {
		log.Panic("FAIL: grid output is all zeros after gridding")
	}

	log.Printf("Passed! grid_real sum=%.4f, grid_imag sum=%.4f", sumReal, sumImag)
}
