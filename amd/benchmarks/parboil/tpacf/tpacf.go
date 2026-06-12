// Package tpacf implements the Parboil Two-Point Angular Correlation Function benchmark.
// Computes angular separations between pairs of sky catalog points and bins
// them into a histogram.
package tpacf

import (
	"log"
	"math"
	"math/rand"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// KernelArgs defines the explicit kernel arguments for tpacf_kernel.
// V5 implicit args (256 bytes) are filled automatically by the driver.
type KernelArgs struct {
	Ra        driver.Ptr
	Dec       driver.Ptr
	Hist      driver.Ptr
	NumPoints int32
	NumBins   int32
	MinAngle  float32
	BinWidth  float32
}

// Benchmark defines the Parboil TPACF benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	tpacfKernel *insts.KernelCodeObject

	NumPoints int
	NumBins   int
	BlockSize int

	useUnifiedMemory bool

	hRa   []float32
	hDec  []float32
	hHist []uint64

	dRa   driver.Ptr
	dDec  driver.Ptr
	dHist driver.Ptr
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

// NewBenchmark creates a new Parboil TPACF benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.NumPoints = 4096
	b.NumBins = 64
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
	b.tpacfKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "tpacf_kernel")
	if b.tpacfKernel == nil {
		log.Panic("Failed to load tpacf_kernel kernel binary")
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
	rand.Seed(42)

	b.hRa = make([]float32, b.NumPoints)
	b.hDec = make([]float32, b.NumPoints)
	b.hHist = make([]uint64, b.NumBins)

	twoPi := float32(2.0 * math.Pi)
	halfPi := float32(math.Pi / 2.0)
	for i := 0; i < b.NumPoints; i++ {
		b.hRa[i] = rand.Float32() * twoPi
		b.hDec[i] = rand.Float32()*float32(math.Pi) - halfPi
	}

	floatBytes := uint64(b.NumPoints) * 4
	histBytes := uint64(b.NumBins) * 8

	if b.useUnifiedMemory {
		b.dRa = b.driver.AllocateUnifiedMemory(b.context, floatBytes)
		b.dDec = b.driver.AllocateUnifiedMemory(b.context, floatBytes)
		b.dHist = b.driver.AllocateUnifiedMemory(b.context, histBytes)
	} else {
		b.dRa = b.driver.AllocateMemory(b.context, floatBytes)
		b.dDec = b.driver.AllocateMemory(b.context, floatBytes)
		b.dHist = b.driver.AllocateMemory(b.context, histBytes)
	}

	b.driver.MemCopyH2D(b.context, b.dRa, b.hRa)
	b.driver.MemCopyH2D(b.context, b.dDec, b.hDec)
	b.driver.MemCopyH2D(b.context, b.dHist, b.hHist)
}

func (b *Benchmark) exec() {
	numBlocks := (b.NumPoints + b.BlockSize - 1) / b.BlockSize
	globalSize := [3]uint32{uint32(numBlocks * b.BlockSize), 1, 1}
	localSize := [3]uint16{uint16(b.BlockSize), 1, 1}

	minAngle := float32(0.0)
	maxAngle := float32(math.Pi)
	binWidth := (maxAngle - minAngle) / float32(b.NumBins)

	args := KernelArgs{
		Ra:        b.dRa,
		Dec:       b.dDec,
		Hist:      b.dHist,
		NumPoints: int32(b.NumPoints),
		NumBins:   int32(b.NumBins),
		MinAngle:  minAngle,
		BinWidth:  binWidth,
	}
	b.driver.LaunchKernel(b.context,
		b.tpacfKernel,
		globalSize, localSize,
		&args,
	)

	b.driver.MemCopyD2H(b.context, b.hHist, b.dHist)
}

// Verify checks that the histogram contains non-zero values.
func (b *Benchmark) Verify() {
	var total uint64
	for i := 0; i < b.NumBins; i++ {
		total += b.hHist[i]
	}

	if total == 0 {
		log.Panic("FAIL: histogram is all zeros after tpacf kernel")
	}

	log.Printf("Passed! total pairs counted=%d", total)
}
