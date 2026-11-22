// Package correlation implements the PolyBench Correlation benchmark, ported
// from sarchlab/gpu_benchmarks (tier2/polybench_correlation) for the MGPUSim
// MI300X (CDNA3 / gfx942) model.
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
// benchmark must be run with `-arch cdna3` (the MI300X configuration).
//
// The 1D kernels use a constant block size and the tiled kernel a constant
// TILE_SIZE, so the compiled kernels carry no hidden ABI arguments.
package correlation

import (
	// embed hsaco files
	_ "embed"
	"fmt"
	"log"
	"math"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

const (
	tileSize  = 16  // must match TILE_SIZE in native/polybench_correlation.cpp
	blockSize = 256 // must match BLOCK_SIZE in native/polybench_correlation.cpp
)

// MeanArgs encodes explicit kernel arguments and gfx942 launch geometry.
type MeanArgs struct {
	Data   gpuruntime.Ptr // offset 0
	Mean   gpuruntime.Ptr // offset 8
	M      int32          // offset 16
	N      int32          // offset 20
	Launch benchmarks.GFX942LaunchDimensions
}

// StddevArgs: kernarg_segment_size = 32.
type StddevArgs struct {
	Data   gpuruntime.Ptr // offset 0
	Mean   gpuruntime.Ptr // offset 8
	Stddev gpuruntime.Ptr // offset 16
	M      int32          // offset 24
	N      int32          // offset 28
	Launch benchmarks.GFX942LaunchDimensions
}

// NormalizeArgs: kernarg_segment_size = 32.
type NormalizeArgs struct {
	Data   gpuruntime.Ptr // offset 0
	Mean   gpuruntime.Ptr // offset 8
	Stddev gpuruntime.Ptr // offset 16
	M      int32          // offset 24
	N      int32          // offset 28
	Launch benchmarks.GFX942LaunchDimensions
}

// CorrelationArgs: kernarg_segment_size = 24.
type CorrelationArgs struct {
	Data gpuruntime.Ptr // offset 0
	Corr gpuruntime.Ptr // offset 8
	M    int32          // offset 16
	N    int32          // offset 20
}

// Benchmark defines the Correlation benchmark.
type Benchmark struct {
	*State
	benchmarks.Checkpoint[State]
	rt *gpuruntime.Runtime

	meanKernel        *insts.KernelCodeObject
	stddevKernel      *insts.KernelCodeObject
	normalizeKernel   *insts.KernelCodeObject
	correlationKernel *insts.KernelCodeObject
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new Correlation benchmark.
func NewBenchmark(rt *gpuruntime.Runtime) *Benchmark {
	b := new(Benchmark)
	b.State = &State{}
	b.Checkpoint = benchmarks.NewCheckpoint(b.State, b.bind)

	b.rt = rt
	b.Context = b.rt.Init()

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
	b.Gpus = gpus
}

// SetUnifiedMemory requests the use of unified memory.
func (b *Benchmark) SetUnifiedMemory() {
	b.UseUnifiedMemory = true
}

// Prepare the inputs for the first queued phase.
func (b *Benchmark) submit() {
	if b.Arch != arch.CDNA3 {
		log.Panic("the polybench correlation benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300x")
	}

	b.loadProgram()

	b.rt.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.rt.CreateCommandQueue(b.Context)
	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	if b.N <= 0 {
		b.N = 64
	}

	n := b.N
	numElem := n * n // M == N

	b.Data = make([]float32, numElem)
	rand := benchmarks.NewGlibcRand(42)
	for i := range b.Data {
		b.Data[i] = float32(rand.Next()%1000) / 100.0
	}

	if b.UseUnifiedMemory {
		b.GData = b.rt.AllocateUnifiedMemory(b.Context, uint64(numElem*4))
		b.GMean = b.rt.AllocateUnifiedMemory(b.Context, uint64(n*4))
		b.GStddev = b.rt.AllocateUnifiedMemory(b.Context, uint64(n*4))
		b.GCorr = b.rt.AllocateUnifiedMemory(b.Context, uint64(numElem*4))
	} else {
		b.GData = b.rt.AllocateMemory(b.Context, uint64(numElem*4))
		b.GMean = b.rt.AllocateMemory(b.Context, uint64(n*4))
		b.GStddev = b.rt.AllocateMemory(b.Context, uint64(n*4))
		b.GCorr = b.rt.AllocateMemory(b.Context, uint64(numElem*4))
	}

	b.Upload(b.rt, b.Queue, b.GData, b.Data)
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
		Data:   b.GData,
		Mean:   b.GMean,
		M:      int32(m),
		N:      int32(n),
		Launch: benchmarks.NewGFX942LaunchDimensions([3]uint32{globalN, 1, 1}, [3]uint16{blockSize, 1, 1}),
	}
	b.rt.EnqueueLaunchKernel(
		b.Queue, b.meanKernel,
		[3]uint32{globalN, 1, 1},
		[3]uint16{blockSize, 1, 1},
		&meanArgs,
	)

	stddevArgs := StddevArgs{
		Data:   b.GData,
		Mean:   b.GMean,
		Stddev: b.GStddev,
		M:      int32(m),
		N:      int32(n),
		Launch: benchmarks.NewGFX942LaunchDimensions([3]uint32{globalN, 1, 1}, [3]uint16{blockSize, 1, 1}),
	}
	b.rt.EnqueueLaunchKernel(
		b.Queue, b.stddevKernel,
		[3]uint32{globalN, 1, 1},
		[3]uint16{blockSize, 1, 1},
		&stddevArgs,
	)

	normalizeArgs := NormalizeArgs{
		Data:   b.GData,
		Mean:   b.GMean,
		Stddev: b.GStddev,
		M:      int32(m),
		N:      int32(n),
		Launch: benchmarks.NewGFX942LaunchDimensions([3]uint32{globalMN, 1, 1}, [3]uint16{blockSize, 1, 1}),
	}
	b.rt.EnqueueLaunchKernel(
		b.Queue, b.normalizeKernel,
		[3]uint32{globalMN, 1, 1},
		[3]uint16{blockSize, 1, 1},
		&normalizeArgs,
	)

	correlationArgs := CorrelationArgs{
		Data: b.GData,
		Corr: b.GCorr,
		M:    int32(m),
		N:    int32(n),
	}
	b.rt.EnqueueLaunchKernel(
		b.Queue, b.correlationKernel,
		[3]uint32{globalCorr, globalCorr, 1},
		[3]uint16{tileSize, tileSize, 1},
		&correlationArgs,
	)
}

// reference evaluates the archived independent double-precision CPU pipeline.
func (b *Benchmark) reference() [4][]float64 {
	n := b.N
	work := make([]float64, n*n)
	mean, stddev := make([]float64, n), make([]float64, n)
	for i, value := range b.Data {
		mean[i%n] += float64(value)
	}
	for j := range mean {
		mean[j] /= float64(n)
	}
	for i, value := range b.Data {
		diff := float64(value) - mean[i%n]
		stddev[i%n] += diff * diff
	}
	for j := range stddev {
		stddev[j] = math.Sqrt(stddev[j] / float64(n))
		if stddev[j] < float64(float32(1e-12)) {
			stddev[j] = 1
		}
	}
	sqrtN := math.Sqrt(float64(n))
	for i, value := range b.Data {
		j := i % n
		work[i] = (float64(value) - mean[j]) / (sqrtN * stddev[j])
	}

	corr := make([]float64, n*n)
	for row := 0; row < n; row++ {
		for col := 0; col < n; col++ {
			for k := 0; k < n; k++ {
				corr[row*n+col] += work[k*n+row] * work[k*n+col]
			}
		}
		corr[row*n+row] = 1
	}
	return [4][]float64{mean, stddev, work, corr}
}

func (b *Benchmark) verify() error {
	names := [4]string{"mean", "standard deviation", "normalized data", "correlation"}
	for stage, refs := range b.reference() {
		if err := b.verifyStage(stage, names[stage], refs); err != nil {
			return err
		}
	}
	log.Printf("Passed!\n")
	return nil
}

func (b *Benchmark) verifyStage(stage int, name string, expected []float64) error {
	actual := make([]float32, len(expected))
	if err := benchmarks.Read(b.rt, b.Readbacks[stage], actual); err != nil {
		return err
	}
	for i, ref := range expected {
		got := float64(actual[i])
		tolerance := 1e-5 * math.Max(1, math.Abs(ref))
		if stage == 3 && i%(b.N+1) == 0 {
			tolerance = 0
		}
		if math.IsInf(got, 0) || math.IsInf(ref, 0) || !(math.Abs(ref-got) <= tolerance) {
			return fmt.Errorf("%s[%d] got %g, expected %g within %g", name, i, got, ref, tolerance)
		}
	}
	return nil
}

func (b *Benchmark) enqueueVerification() {
	n := b.N
	b.Readbacks = append(b.Readbacks,
		benchmarks.Download(b.rt, b.Queue, b.GMean, 4*n),
		benchmarks.Download(b.rt, b.Queue, b.GStddev, 4*n),
		benchmarks.Download(b.rt, b.Queue, b.GData, 4*n*n),
		benchmarks.Download(b.rt, b.Queue, b.GCorr, 4*n*n),
	)
}

// Step submits computation, then verification readback, then consumes results.
func (b *Benchmark) Step() error {
	if err := b.Prepare(); err != nil {
		return err
	}
	if err := b.ReleaseUploads(b.rt); err != nil {
		return err
	}

	switch b.Phase {
	case 0:
		b.submit()
		b.Phase = 1
	case 1:
		if !b.Verification {
			b.Ended = true
			return nil
		}
		b.enqueueVerification()
		b.Phase = 2
	case 2:
		if err := b.verify(); err != nil {
			return err
		}
		b.Readbacks = nil
		b.Ended = true
	}
	return nil
}
