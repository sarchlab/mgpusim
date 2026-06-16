// Package lud implements the Rodinia LUD (LU decomposition) benchmark,
// ported from sarchlab/gpu_benchmarks (tier2/rodinia_lud) for the MGPUSim
// MI300A (CDNA3 / gfx942) model.
//
// It performs a blocked LU factorization (no pivoting) of a dense NxN matrix
// using three shared-memory kernels (BSIZE = 16):
//
//	lud_diagonal   — in-place LU factor of the diagonal 16x16 block
//	lud_perimeter  — forward/back-solve for the row/column perimeter blocks
//	lud_internal   — Schur-complement update for interior blocks
//
// The host iterates the three-kernel sequence over each diagonal block, in the
// same order as the original Rodinia driver. The kernel binary is compiled for
// gfx942 only (see native/), so the benchmark must be run with `-arch cdna3`.
package lud

import (
	"log"
	"math"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

const bSize = 16

// SimpleKernelArgs is the kernarg layout for lud_diagonal and lud_internal.
//
// Verified against the compiled gfx942 metadata (kernarg_segment_size = 16):
// one 8-byte global_buffer pointer followed by two 4-byte by_value scalars,
// packed with no padding. These kernels read only threadIdx/blockIdx, so no
// hidden ABI arguments are emitted.
type SimpleKernelArgs struct {
	A      driver.Ptr // offset 0
	N      int32      // offset 8
	Offset int32      // offset 12
}

// PerimeterKernelArgs is the kernarg layout for lud_perimeter.
//
// Verified against the compiled gfx942 metadata (kernarg_segment_size = 272).
// lud_perimeter reads gridDim.x, so the HIP runtime emits hidden ABI args.
// The explicit args (offsets 0..15) are followed by the hidden block/group/
// remainder counts, a 16-byte reserved gap, the 24-byte global offset triple,
// and the 2-byte grid-dims field. The struct is padded to exactly 272 bytes.
type PerimeterKernelArgs struct {
	A      driver.Ptr // offset 0
	N      int32      // offset 8
	Offset int32      // offset 12
	// Hidden kernel arguments (required by HIP runtime for gfx942)
	HiddenBlockCountX   uint32    // offset 16
	HiddenBlockCountY   uint32    // offset 20
	HiddenBlockCountZ   uint32    // offset 24
	HiddenGroupSizeX    uint16    // offset 28
	HiddenGroupSizeY    uint16    // offset 30
	HiddenGroupSizeZ    uint16    // offset 32
	HiddenRemainderX    uint16    // offset 34
	HiddenRemainderY    uint16    // offset 36
	HiddenRemainderZ    uint16    // offset 38
	Padding0            [16]byte  // offset 40-55 - reserved
	HiddenGlobalOffsetX int64     // offset 56
	HiddenGlobalOffsetY int64     // offset 64
	HiddenGlobalOffsetZ int64     // offset 72
	HiddenGridDims      uint16    // offset 80
	Padding1            [190]byte // offset 82-271 - trailing pad to 272
}

// Benchmark defines the LUD benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	queue   *driver.CommandQueue
	gpus    []int

	hsacoDiagonal  *insts.KernelCodeObject
	hsacoPerimeter *insts.KernelCodeObject
	hsacoInternal  *insts.KernelCodeObject

	Arch arch.Type
	N    int

	aOrig []float32
	gA    driver.Ptr

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new LUD benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.context = b.driver.Init()
	b.queue = driver.CreateCommandQueue(b.context)

	return b
}

func (b *Benchmark) loadProgram() {
	b.hsacoDiagonal = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "lud_diagonal")
	b.hsacoPerimeter = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "lud_perimeter")
	b.hsacoInternal = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "lud_internal")
	if b.hsacoDiagonal == nil ||
		b.hsacoPerimeter == nil ||
		b.hsacoInternal == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// SelectGPU selects the GPUs to run on. LUD uses a single GPU.
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
		log.Panic("the rodinia lud benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300a")
	}

	b.loadProgram()

	b.driver.SelectGPU(b.context, b.gpus[0])
	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	if b.N <= 0 {
		b.N = 128
	}
	if b.N%bSize != 0 {
		log.Panicf("N=%d must be divisible by BSIZE=%d", b.N, bSize)
	}

	n := b.N
	numElem := n * n

	b.aOrig = make([]float32, numElem)
	initMatrix(b.aOrig, n)

	if b.useUnifiedMemory {
		b.gA = b.driver.AllocateUnifiedMemory(b.context, uint64(numElem*4))
	} else {
		b.gA = b.driver.AllocateMemory(b.context, uint64(numElem*4))
	}

	b.driver.MemCopyH2D(b.context, b.gA, b.aOrig)
}

// initMatrix builds a diagonally-dominant matrix so that LU without pivoting
// is numerically stable. It uses a deterministic LCG so that Verify() can
// reproduce the exact same input matrix.
func initMatrix(a []float32, n int) {
	rng := newRand(42)
	for i := 0; i < n; i++ {
		var rowSum float32
		for j := 0; j < n; j++ {
			v := float32(rng.next()%10+1) * 0.1
			a[i*n+j] = v
			if i != j {
				rowSum += float32(math.Abs(float64(v)))
			}
		}
		a[i*n+i] = rowSum + 1.0
	}
}

// rand is a tiny deterministic PRNG (glibc-style LCG) so the Go host init is
// fully reproducible inside Verify(). The absolute sequence does not need to
// match C rand(); it only needs to be identical between initMem and Verify.
type rand struct{ state uint64 }

func newRand(seed uint64) *rand { return &rand{state: seed} }

func (r *rand) next() int {
	r.state = r.state*6364136223846793005 + 1442695040888963407
	return int((r.state >> 33) & 0x7fffffff)
}

func (b *Benchmark) exec() {
	n := b.N
	numBlocks := n / bSize

	for k := 0; k < numBlocks; k++ {
		b.launchDiagonal(n, k)

		peri := 2 * (numBlocks - k - 1)
		intern := numBlocks - k - 1
		if peri > 0 {
			b.launchPerimeter(n, k, peri)
			b.launchInternal(n, k, intern)
		}
		b.driver.DrainCommandQueue(b.queue)
	}
}

func (b *Benchmark) launchDiagonal(n, offset int) {
	args := SimpleKernelArgs{
		A:      b.gA,
		N:      int32(n),
		Offset: int32(offset),
	}

	b.driver.EnqueueLaunchKernel(
		b.queue,
		b.hsacoDiagonal,
		[3]uint32{bSize, bSize, 1},
		[3]uint16{bSize, bSize, 1},
		&args,
	)
}

func (b *Benchmark) launchPerimeter(n, offset, peri int) {
	globalX := uint32(peri * bSize)
	args := PerimeterKernelArgs{
		A:                 b.gA,
		N:                 int32(n),
		Offset:            int32(offset),
		HiddenBlockCountX: uint32(peri),
		HiddenBlockCountY: 1,
		HiddenBlockCountZ: 1,
		HiddenGroupSizeX:  bSize,
		HiddenGroupSizeY:  bSize,
		HiddenGroupSizeZ:  1,
		HiddenRemainderX:  0,
		HiddenRemainderY:  0,
		HiddenRemainderZ:  0,
		HiddenGridDims:    2,
	}

	b.driver.EnqueueLaunchKernel(
		b.queue,
		b.hsacoPerimeter,
		[3]uint32{globalX, bSize, 1},
		[3]uint16{bSize, bSize, 1},
		&args,
	)
}

func (b *Benchmark) launchInternal(n, offset, intern int) {
	globalX := uint32(intern * bSize)
	globalY := uint32(intern * bSize)
	args := SimpleKernelArgs{
		A:      b.gA,
		N:      int32(n),
		Offset: int32(offset),
	}

	b.driver.EnqueueLaunchKernel(
		b.queue,
		b.hsacoInternal,
		[3]uint32{globalX, globalY, 1},
		[3]uint16{bSize, bSize, 1},
		&args,
	)
}

// Verify reconstructs A = L*U from the in-place GPU result and compares the
// reconstruction against the original matrix, exactly like the reference C
// verifier (relative Frobenius norm < 1e-4).
func (b *Benchmark) Verify() {
	n := b.N
	lu := make([]float32, n*n)
	b.driver.MemCopyD2H(b.context, lu, b.gA)

	var err, normA float64
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			var sum float64
			lim := i
			if j < i {
				lim = j
			}
			for k := 0; k <= lim; k++ {
				l := float64(lu[i*n+k])
				if k == i {
					l = 1.0
				}
				u := float64(lu[k*n+j])
				sum += l * u
			}
			diff := float64(b.aOrig[i*n+j]) - sum
			err += diff * diff
			normA += float64(b.aOrig[i*n+j]) * float64(b.aOrig[i*n+j])
		}
	}

	rel := math.Sqrt(err / normA)
	if rel >= 1e-4 {
		log.Fatalf("Verification failed: ||A - LU||_F / ||A||_F = %.6e\n", rel)
	}

	log.Printf("||A - LU||_F / ||A||_F = %.6e\n", rel)
	log.Printf("Passed!\n")
}
