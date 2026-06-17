// Package fdtd2d implements the PolyBench FDTD-2D benchmark, ported from
// sarchlab/gpu_benchmarks (tier2/polybench_fdtd2d) for the MGPUSim MI300A
// (CDNA3 / gfx942) model.
//
// FDTD-2D is a 2D Finite Difference Time Domain electromagnetic simulation
// over three NX x NY field arrays (ex, ey, hz). Each time step launches three
// kernels (update ex, update ey, update hz). The kernel binary is compiled for
// gfx942 only (see native/), so the benchmark must be run with `-arch cdna3`
// (the MI300A configuration).
//
// The kernels use a constant 16x16 block dimension, so no hidden ABI arguments
// are emitted (kernarg_segment_size = 24 for ex/ey, 32 for hz).
//
// Limitation: the kernels update the ex/ey/hz arrays in place and read
// neighbor elements (hz reads ex[i][j+1]/ey[i+1][j]; the next step's ex/ey
// read hz). The functional emulator computes the correct result for any grid
// size, but in timing mode the per-CU L1 / shared L2 caches do not remain
// coherent for these in-place dependent reads once the grid spans more than
// one work-group, even with a DrainCommandQueue (or an explicit cache flush)
// between every launch. The default grid is therefore a single 16x16
// work-group, which stays coherent across all time steps and verifies in
// timing mode.
package fdtd2d

import (
	"log"
	"math"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

const blockSize = 16

// ExEyKernelArgs matches the layout of fdtd_update_ex / fdtd_update_ey.
//
// Verified against AMDGPU metadata (kernarg_segment_size = 24): two 8-byte
// global_buffer pointers followed by two 4-byte by_value ints, packed with no
// padding (mgpusim serializes args with binary.Write, which inserts no
// alignment padding). No hidden ABI arguments are present.
type ExEyKernelArgs struct {
	Field driver.Ptr // offset 0  (ex or ey, in/out)
	Hz    driver.Ptr // offset 8  (hz, read-only)
	NX    int32      // offset 16
	NY    int32      // offset 20
}

// HzKernelArgs matches the layout of fdtd_update_hz.
//
// Verified against AMDGPU metadata (kernarg_segment_size = 32): three 8-byte
// global_buffer pointers followed by two 4-byte by_value ints. No hidden ABI
// arguments are present.
type HzKernelArgs struct {
	Ex driver.Ptr // offset 0  (read-only)
	Ey driver.Ptr // offset 8  (read-only)
	Hz driver.Ptr // offset 16 (in/out)
	NX int32      // offset 24
	NY int32      // offset 28
}

// Benchmark defines the FDTD-2D benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	queue   *driver.CommandQueue

	kernelEx *insts.KernelCodeObject
	kernelEy *insts.KernelCodeObject
	kernelHz *insts.KernelCodeObject

	gpus []int

	Arch arch.Type
	N    int // grid dimension (NX = NY = N)
	TMax int // number of time steps

	exInit []float32
	eyInit []float32
	hzInit []float32

	gEx driver.Ptr
	gEy driver.Ptr
	gHz driver.Ptr

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new FDTD-2D benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.context = b.driver.Init()
	b.queue = driver.CreateCommandQueue(b.context)

	return b
}

func (b *Benchmark) loadProgram() {
	b.kernelEx = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "fdtd_update_ex")
	b.kernelEy = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "fdtd_update_ey")
	b.kernelHz = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "fdtd_update_hz")
	if b.kernelEx == nil || b.kernelEy == nil || b.kernelHz == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// SelectGPU selects the GPUs to run on. FDTD-2D uses a single GPU.
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
		log.Panic("the polybench fdtd2d benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300a")
	}

	b.loadProgram()

	b.driver.SelectGPU(b.context, b.gpus[0])
	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	if b.N <= 0 {
		// Default to a single 16x16 work-group. The kernels update the field
		// arrays in place and read neighbor elements across the dependency
		// chain (hz reads ex/ey, the next step's ex/ey read hz); with more
		// than one work-group the timing-mode caches do not stay coherent
		// across these in-place dependent launches even with a drain in
		// between (the emulator computes the correct result at any size — see
		// the package doc comment). A single work-group keeps the run fully
		// coherent across all time steps.
		b.N = 16
	}
	if b.TMax <= 0 {
		b.TMax = 10
	}

	n := b.N
	numElem := n * n

	b.exInit = make([]float32, numElem)
	b.eyInit = make([]float32, numElem)
	b.hzInit = make([]float32, numElem)

	// Deterministic host init reproducible in Verify().
	for i := 0; i < numElem; i++ {
		b.exInit[i] = float32(i%100) / 10.0
		b.eyInit[i] = float32((i*2)%100) / 10.0
		b.hzInit[i] = float32((i*3)%100) / 10.0
	}

	if b.useUnifiedMemory {
		b.gEx = b.driver.AllocateUnifiedMemory(b.context, uint64(numElem*4))
		b.gEy = b.driver.AllocateUnifiedMemory(b.context, uint64(numElem*4))
		b.gHz = b.driver.AllocateUnifiedMemory(b.context, uint64(numElem*4))
	} else {
		b.gEx = b.driver.AllocateMemory(b.context, uint64(numElem*4))
		b.gEy = b.driver.AllocateMemory(b.context, uint64(numElem*4))
		b.gHz = b.driver.AllocateMemory(b.context, uint64(numElem*4))
	}

	b.driver.MemCopyH2D(b.context, b.gEx, b.exInit)
	b.driver.MemCopyH2D(b.context, b.gEy, b.eyInit)
	b.driver.MemCopyH2D(b.context, b.gHz, b.hzInit)
}

func (b *Benchmark) exec() {
	n := b.N
	gridDim := uint32((n + blockSize - 1) / blockSize)
	globalX := gridDim * blockSize
	globalY := gridDim * blockSize

	exArgs := ExEyKernelArgs{Field: b.gEx, Hz: b.gHz, NX: int32(n), NY: int32(n)}
	eyArgs := ExEyKernelArgs{Field: b.gEy, Hz: b.gHz, NX: int32(n), NY: int32(n)}
	hzArgs := HzKernelArgs{
		Ex: b.gEx, Ey: b.gEy, Hz: b.gHz, NX: int32(n), NY: int32(n),
	}

	// The three kernels in each time step have read-after-write dependencies:
	// the hz update reads the ex/ey written this step, and the next step's
	// ex/ey updates read the hz written this step. Draining the command queue
	// between dependent launches serializes them, as in the chained-GEMM
	// benchmarks (2mm/3mm), which is sufficient for the single-workgroup
	// default configuration. (See the package doc comment about the
	// multi-workgroup timing-mode cache-coherency limitation.)
	cohere := func() {
		b.driver.DrainCommandQueue(b.queue)
	}

	for t := 0; t < b.TMax; t++ {
		b.driver.EnqueueLaunchKernel(
			b.queue, b.kernelEx,
			[3]uint32{globalX, globalY, 1},
			[3]uint16{blockSize, blockSize, 1},
			&exArgs,
		)
		cohere()

		b.driver.EnqueueLaunchKernel(
			b.queue, b.kernelEy,
			[3]uint32{globalX, globalY, 1},
			[3]uint16{blockSize, blockSize, 1},
			&eyArgs,
		)
		cohere()

		b.driver.EnqueueLaunchKernel(
			b.queue, b.kernelHz,
			[3]uint32{globalX, globalY, 1},
			[3]uint16{blockSize, blockSize, 1},
			&hzArgs,
		)
		cohere()
	}
}

// Verify checks the GPU result against a CPU reference computation.
func (b *Benchmark) Verify() { //nolint:funlen,gocognit
	n := b.N
	numElem := n * n

	gpuEx := make([]float32, numElem)
	gpuEy := make([]float32, numElem)
	gpuHz := make([]float32, numElem)
	b.driver.MemCopyD2H(b.context, gpuEx, b.gEx)
	b.driver.MemCopyD2H(b.context, gpuEy, b.gEy)
	b.driver.MemCopyD2H(b.context, gpuHz, b.gHz)

	// CPU reference: same field init, same TMax time steps, in float32 to
	// mirror the GPU arithmetic.
	ex := make([]float32, numElem)
	ey := make([]float32, numElem)
	hz := make([]float32, numElem)
	copy(ex, b.exInit)
	copy(ey, b.eyInit)
	copy(hz, b.hzInit)

	for t := 0; t < b.TMax; t++ {
		// Update ex.
		for i := 0; i < n; i++ {
			for j := 0; j < n; j++ {
				if i == 0 {
					ex[0*n+j] = 0.0
				} else {
					ex[i*n+j] += 0.5 * (hz[i*n+j] - hz[(i-1)*n+j])
				}
			}
		}
		// Update ey.
		for i := 0; i < n; i++ {
			for j := 0; j < n; j++ {
				if j == 0 {
					ey[i*n+0] = 0.0
				} else {
					ey[i*n+j] += 0.5 * (hz[i*n+j] - hz[i*n+(j-1)])
				}
			}
		}
		// Update hz.
		for i := 0; i < n-1; i++ {
			for j := 0; j < n-1; j++ {
				hz[i*n+j] -= 0.7 * (ex[i*n+(j+1)] - ex[i*n+j] +
					ey[(i+1)*n+j] - ey[i*n+j])
			}
		}
	}

	check := func(name string, ref, got []float32) {
		for idx := 0; idx < numElem; idx++ {
			r := float64(ref[idx])
			g := float64(got[idx])
			denom := math.Abs(r)
			if denom < 1.0 {
				denom = 1.0
			}
			if math.Abs(r-g)/denom > 1e-3 {
				log.Fatalf("%s mismatch at (%d,%d): expected %f, got %f.\n",
					name, idx/n, idx%n, r, g)
			}
		}
	}

	check("ex", ex, gpuEx)
	check("ey", ey, gpuEy)
	check("hz", hz, gpuHz)

	log.Printf("Passed!\n")
}
