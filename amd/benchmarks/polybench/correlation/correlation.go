// Package correlation implements the PolyBench Correlation benchmark, ported
// from sarchlab/gpu_benchmarks (tier2/polybench_correlation) for the MGPUSim
// MI300A (CDNA3 / gfx942) model.
//
// Given an N-by-N data matrix (M = N samples, N features) it computes the
// N-by-N correlation matrix in four steps:
//
//  1. mean_kernel        - column means
//  2. stddev_kernel      - column standard deviations
//  3. normalize_kernel   - normalize each element by mean/stddev/sqrt(M)
//  4. correlation_kernel - tiled matmul of normalized^T * normalized
//
// The kernel binary is compiled for gfx942 only (see native/), so the
// benchmark must be run with `-arch cdna3` (the MI300A configuration).
//
// The 1D kernels use a constant block size and the tiled kernel a constant
// TILE_SIZE, so the compiled kernels carry no hidden ABI arguments.
package correlation

import (
	"log"
	"math"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

const (
	tileSize  = 16  // must match TILE_SIZE in native/polybench_correlation.cpp
	blockSize = 256 // must match BLOCK_SIZE in native/polybench_correlation.cpp
)

// MeanArgs / StddevArgs / NormalizeArgs / CorrelationArgs mirror the kernarg
// layout reported by the compiled gfx942 kernels (verified against the AMDGPU
// metadata). mgpusim serializes args with binary.Write, which packs fields in
// declaration order with no alignment padding, so each field lands on its
// metadata offset. No hidden ABI arguments are emitted (constant block dims).

// MeanArgs: kernarg_segment_size = 24.
type MeanArgs struct {
	Data driver.Ptr // offset 0
	Mean driver.Ptr // offset 8
	M    int32      // offset 16
	N    int32      // offset 20
}

// StddevArgs: kernarg_segment_size = 32.
type StddevArgs struct {
	Data   driver.Ptr // offset 0
	Mean   driver.Ptr // offset 8
	Stddev driver.Ptr // offset 16
	M      int32      // offset 24
	N      int32      // offset 28
}

// NormalizeArgs: kernarg_segment_size = 32.
type NormalizeArgs struct {
	Data   driver.Ptr // offset 0
	Mean   driver.Ptr // offset 8
	Stddev driver.Ptr // offset 16
	M      int32      // offset 24
	N      int32      // offset 28
}

// CorrelationArgs: kernarg_segment_size = 24.
type CorrelationArgs struct {
	Data driver.Ptr // offset 0
	Corr driver.Ptr // offset 8
	M    int32      // offset 16
	N    int32      // offset 20
}

// Benchmark defines the Correlation benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	queue   *driver.CommandQueue
	gpus    []int

	meanKernel        *insts.KernelCodeObject
	stddevKernel      *insts.KernelCodeObject
	normalizeKernel   *insts.KernelCodeObject
	correlationKernel *insts.KernelCodeObject

	Arch arch.Type
	N    int // data matrix is N x N (M = N samples, N features)

	data []float32

	gData   driver.Ptr
	gMean   driver.Ptr
	gStddev driver.Ptr
	gCorr   driver.Ptr

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new Correlation benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.context = b.driver.Init()
	b.queue = driver.CreateCommandQueue(b.context)

	return b
}

func (b *Benchmark) loadProgram() {
	b.meanKernel = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "mean_kernel")
	b.stddevKernel = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "stddev_kernel")
	b.normalizeKernel = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "normalize_kernel")
	b.correlationKernel = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "correlation_kernel")

	if b.meanKernel == nil || b.stddevKernel == nil ||
		b.normalizeKernel == nil || b.correlationKernel == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// SelectGPU selects the GPUs to run on. Correlation uses a single GPU.
func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
}

// SetUnifiedMemory requests the use of unified memory.
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

// Run runs the benchmark.
func (b *Benchmark) Run() {
	if b.Arch != arch.CDNA3 {
		log.Panic("the polybench correlation benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300a")
	}

	b.loadProgram()

	b.driver.SelectGPU(b.context, b.gpus[0])
	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	if b.N <= 0 {
		b.N = 64
	}

	n := b.N
	numElem := n * n // M == N

	b.data = make([]float32, numElem)
	for i := 0; i < numElem; i++ {
		// Deterministic host init reproduced exactly in Verify().
		b.data[i] = float32((i*7+3)%1000) / 100.0
	}

	if b.useUnifiedMemory {
		b.gData = b.driver.AllocateUnifiedMemory(b.context, uint64(numElem*4))
		b.gMean = b.driver.AllocateUnifiedMemory(b.context, uint64(n*4))
		b.gStddev = b.driver.AllocateUnifiedMemory(b.context, uint64(n*4))
		b.gCorr = b.driver.AllocateUnifiedMemory(b.context, uint64(numElem*4))
	} else {
		b.gData = b.driver.AllocateMemory(b.context, uint64(numElem*4))
		b.gMean = b.driver.AllocateMemory(b.context, uint64(n*4))
		b.gStddev = b.driver.AllocateMemory(b.context, uint64(n*4))
		b.gCorr = b.driver.AllocateMemory(b.context, uint64(numElem*4))
	}

	b.driver.MemCopyH2D(b.context, b.gData, b.data)
}

func (b *Benchmark) exec() { //nolint:funlen,gocognit
	n := b.N
	m := n // M == N

	// 1D launches over N columns.
	blocksN := (n + blockSize - 1) / blockSize
	globalN := uint32(blocksN * blockSize)

	// 1D launch over M*N elements.
	blocksMN := (m*n + blockSize - 1) / blockSize
	globalMN := uint32(blocksMN * blockSize)

	// 2D tiled launch over N x N.
	gridDim := uint32((n + tileSize - 1) / tileSize)
	globalCorr := gridDim * tileSize

	meanArgs := MeanArgs{
		Data: b.gData,
		Mean: b.gMean,
		M:    int32(m),
		N:    int32(n),
	}
	b.driver.EnqueueLaunchKernel(
		b.queue, b.meanKernel,
		[3]uint32{globalN, 1, 1},
		[3]uint16{blockSize, 1, 1},
		&meanArgs,
	)

	stddevArgs := StddevArgs{
		Data:   b.gData,
		Mean:   b.gMean,
		Stddev: b.gStddev,
		M:      int32(m),
		N:      int32(n),
	}
	b.driver.EnqueueLaunchKernel(
		b.queue, b.stddevKernel,
		[3]uint32{globalN, 1, 1},
		[3]uint16{blockSize, 1, 1},
		&stddevArgs,
	)

	normalizeArgs := NormalizeArgs{
		Data:   b.gData,
		Mean:   b.gMean,
		Stddev: b.gStddev,
		M:      int32(m),
		N:      int32(n),
	}
	b.driver.EnqueueLaunchKernel(
		b.queue, b.normalizeKernel,
		[3]uint32{globalMN, 1, 1},
		[3]uint16{blockSize, 1, 1},
		&normalizeArgs,
	)

	correlationArgs := CorrelationArgs{
		Data: b.gData,
		Corr: b.gCorr,
		M:    int32(m),
		N:    int32(n),
	}
	b.driver.EnqueueLaunchKernel(
		b.queue, b.correlationKernel,
		[3]uint32{globalCorr, globalCorr, 1},
		[3]uint16{tileSize, tileSize, 1},
		&correlationArgs,
	)

	b.driver.DrainCommandQueue(b.queue)
}

// Verify checks the GPU result against a CPU reference computation that
// reproduces the exact four-kernel pipeline (in float32) on the host.
func (b *Benchmark) Verify() { //nolint:funlen,gocognit
	n := b.N
	m := n

	gpuCorr := make([]float32, n*n)
	b.driver.MemCopyD2H(b.context, gpuCorr, b.gCorr)

	// CPU reference, mirroring the kernels element-for-element in float32.
	work := make([]float32, m*n)
	copy(work, b.data)

	mean := make([]float32, n)
	for j := 0; j < n; j++ {
		var sum float32
		for i := 0; i < m; i++ {
			sum += work[i*n+j]
		}
		mean[j] = sum / float32(m)
	}

	stddev := make([]float32, n)
	for j := 0; j < n; j++ {
		mj := mean[j]
		var sum float32
		for i := 0; i < m; i++ {
			diff := work[i*n+j] - mj
			sum += diff * diff
		}
		s := float32(math.Sqrt(float64(sum / float32(m))))
		if s < 1e-12 {
			s = 1.0
		}
		stddev[j] = s
	}

	sqrtM := float32(math.Sqrt(float64(m)))
	for idx := 0; idx < m*n; idx++ {
		j := idx % n
		work[idx] = (work[idx] - mean[j]) / (sqrtM * stddev[j])
	}

	corr := make([]float32, n*n)
	for row := 0; row < n; row++ {
		for col := 0; col < n; col++ {
			if row == col {
				corr[row*n+col] = 1.0
				continue
			}
			var sum float32
			for k := 0; k < m; k++ {
				sum += work[k*n+row] * work[k*n+col]
			}
			corr[row*n+col] = sum
		}
	}

	for row := 0; row < n; row++ {
		for col := 0; col < n; col++ {
			ref := float64(corr[row*n+col])
			got := float64(gpuCorr[row*n+col])

			denom := math.Abs(ref)
			if denom < 1.0 {
				denom = 1.0
			}
			if math.Abs(ref-got)/denom > 1e-3 {
				log.Fatalf("At (%d,%d), expected %f, but got %f.\n",
					row, col, ref, got)
			}
		}
	}

	log.Printf("Passed!\n")
}
