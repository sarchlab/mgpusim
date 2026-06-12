// Package rope implements rotary position embedding microbenchmark.
package rope

import (
	"log"
	"math"

	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// KernelArgs defines kernel arguments for rope_kernel.
type KernelArgs struct {
	X        driver.Ptr
	Y        driver.Ptr
	Batch    int32
	SeqLen   int32
	NumHeads int32
	HeadDim  int32
}

// Benchmark defines the RoPE benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	kernel *insts.KernelCodeObject

	Batch     int
	SeqLen    int
	NumHeads  int
	HeadDim   int
	BlockSize int

	useUnifiedMemory bool

	hX []float32
	hY []float32

	dX driver.Ptr
	dY driver.Ptr
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

// NewBenchmark creates a new RoPE benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.Batch = 4
	b.SeqLen = 128
	b.NumHeads = 32
	b.HeadDim = 128
	b.BlockSize = 256
	return b
}

// SelectGPU selects GPU.
func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
}

// SetUnifiedMemory uses unified memory.
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

// SetBatch sets batch size.
func (b *Benchmark) SetBatch(v int) {
	b.Batch = v
}

// SetSeqLen sets sequence length.
func (b *Benchmark) SetSeqLen(v int) {
	b.SeqLen = v
}

// SetNumHeads sets number of heads.
func (b *Benchmark) SetNumHeads(v int) {
	b.NumHeads = v
}

// SetHeadDim sets head dimension.
func (b *Benchmark) SetHeadDim(v int) {
	b.HeadDim = v
}

func (b *Benchmark) loadProgram() {
	b.kernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "rope_kernel")
	if b.kernel == nil {
		log.Panic("Failed to load rope_kernel")
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
	n := b.Batch * b.SeqLen * b.NumHeads * b.HeadDim

	b.hX = make([]float32, n)
	b.hY = make([]float32, n)

	for i := 0; i < n; i++ {
		b.hX[i] = float32(i%1000)*0.002 - 1.0
	}

	bytes := uint64(n * 4)

	if b.useUnifiedMemory {
		b.dX = b.driver.AllocateUnifiedMemory(b.context, bytes)
		b.dY = b.driver.AllocateUnifiedMemory(b.context, bytes)
	} else {
		b.dX = b.driver.AllocateMemory(b.context, bytes)
		b.dY = b.driver.AllocateMemory(b.context, bytes)
	}

	b.driver.MemCopyH2D(b.context, b.dX, b.hX)
}

func (b *Benchmark) exec() {
	halfDim := b.HeadDim / 2
	totalPairs := b.Batch * b.SeqLen * b.NumHeads * halfDim
	blockSize := b.BlockSize
	grid := (totalPairs + blockSize - 1) / blockSize

	globalSize := [3]uint32{uint32(grid * blockSize), 1, 1}
	localSize := [3]uint16{uint16(blockSize), 1, 1}

	args := KernelArgs{
		X:        b.dX,
		Y:        b.dY,
		Batch:    int32(b.Batch),
		SeqLen:   int32(b.SeqLen),
		NumHeads: int32(b.NumHeads),
		HeadDim:  int32(b.HeadDim),
	}
	b.driver.LaunchKernel(b.context, b.kernel,
		globalSize, localSize, &args)

	b.driver.MemCopyD2H(b.context, b.hY, b.dY)
}

func (b *Benchmark) verifyElement(s, base, i int) int {
	x0 := float64(b.hX[base])
	x1 := float64(b.hX[base+1])

	freq := 1.0 / math.Pow(10000.0,
		2.0*float64(i)/float64(b.HeadDim))
	theta := float64(s) * freq

	cosT := math.Cos(theta)
	sinT := math.Sin(theta)

	expY0 := float32(x0*cosT - x1*sinT)
	expY1 := float32(x0*sinT + x1*cosT)

	errs := 0

	diff0 := math.Abs(float64(b.hY[base] - expY0))
	if diff0 > 1e-3*math.Abs(float64(expY0))+1e-5 {
		errs++
	}

	diff1 := math.Abs(float64(b.hY[base+1] - expY1))
	if diff1 > 1e-3*math.Abs(float64(expY1))+1e-5 {
		errs++
	}

	return errs
}

// Verify verifies the result.
func (b *Benchmark) Verify() {
	errors := 0
	halfDim := b.HeadDim / 2

	for ba := 0; ba < b.Batch; ba++ {
		for s := 0; s < b.SeqLen; s++ {
			for h := 0; h < b.NumHeads; h++ {
				for i := 0; i < halfDim; i++ {
					base := ((ba*b.SeqLen+s)*b.NumHeads+h)*b.HeadDim + 2*i
					errors += b.verifyElement(s, base, i)
				}
			}
		}
	}

	if errors > 0 {
		log.Panicf("FAIL: %d errors in rope verification", errors)
	}
	log.Printf("Passed!")
}
