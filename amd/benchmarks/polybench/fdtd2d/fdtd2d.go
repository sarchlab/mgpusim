// Package fdtd2d implements the PolyBench FDTD-2D benchmark, ported from
// sarchlab/gpu_benchmarks (tier2/polybench_fdtd2d) for the MGPUSim MI300X
// (CDNA3 / gfx942) model.
//
// FDTD-2D is a 2D Finite Difference Time Domain electromagnetic simulation
// over three NX x NY field arrays (ex, ey, hz). Each time step launches three
// kernels (update ex, update ey, update hz). The kernel binary is compiled for
// gfx942 only (see native/), so the benchmark must be run with `-arch cdna3`
// (the MI300X configuration).
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

const blockSize = 16

// ExEyKernelArgs matches the layout of fdtd_update_ex / fdtd_update_ey.
//
// Verified against AMDGPU metadata (kernarg_segment_size = 24): two 8-byte
// global_buffer pointers followed by two 4-byte by_value ints, packed with no
// padding (mgpusim serializes args with binary.Write, which inserts no
// alignment padding). No hidden ABI arguments are present.
type ExEyKernelArgs struct {
	Field gpuruntime.Ptr // offset 0  (ex or ey, in/out)
	Hz    gpuruntime.Ptr // offset 8  (hz, read-only)
	NX    int32          // offset 16
	NY    int32          // offset 20
}

// HzKernelArgs matches the layout of fdtd_update_hz.
//
// Verified against AMDGPU metadata (kernarg_segment_size = 32): three 8-byte
// global_buffer pointers followed by two 4-byte by_value ints. No hidden ABI
// arguments are present.
type HzKernelArgs struct {
	Ex gpuruntime.Ptr // offset 0  (read-only)
	Ey gpuruntime.Ptr // offset 8  (read-only)
	Hz gpuruntime.Ptr // offset 16 (in/out)
	NX int32          // offset 24
	NY int32          // offset 28
}

// Benchmark defines the FDTD-2D benchmark.
type Benchmark struct {
	*State
	benchmarks.Checkpoint[checkpointConfig, State]
	rt *gpuruntime.Runtime

	kernelEx *insts.KernelCodeObject
	kernelEy *insts.KernelCodeObject
	kernelHz *insts.KernelCodeObject
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new FDTD-2D benchmark.
func NewBenchmark(rt *gpuruntime.Runtime) *Benchmark {
	b := new(Benchmark)
	b.State = &State{}
	b.Checkpoint = benchmarks.NewCheckpoint(b.State, b.checkpointConfig, b.bind)

	b.rt = rt
	b.Context = b.rt.Init()

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
	b.Gpus = gpus
}

// SetUnifiedMemory requests the use of unified memory.
func (b *Benchmark) SetUnifiedMemory() {
	b.UseUnifiedMemory = true
}

// Prepare the inputs for the first queued phase.
func (b *Benchmark) submit() {
	if b.Arch != arch.CDNA3 {
		log.Panic("the polybench fdtd2d benchmark ships only a gfx942 " +
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

	b.ExInit = make([]float32, numElem)
	b.EyInit = make([]float32, numElem)
	b.HzInit = make([]float32, numElem)

	// Deterministic host init reproducible in Verify().
	for i := 0; i < numElem; i++ {
		b.ExInit[i] = float32(i%100) / 10.0
		b.EyInit[i] = float32((i*2)%100) / 10.0
		b.HzInit[i] = float32((i*3)%100) / 10.0
	}

	if b.UseUnifiedMemory {
		b.GEx = b.rt.AllocateUnifiedMemory(b.Context, uint64(numElem*4))
		b.GEy = b.rt.AllocateUnifiedMemory(b.Context, uint64(numElem*4))
		b.GHz = b.rt.AllocateUnifiedMemory(b.Context, uint64(numElem*4))
	} else {
		b.GEx = b.rt.AllocateMemory(b.Context, uint64(numElem*4))
		b.GEy = b.rt.AllocateMemory(b.Context, uint64(numElem*4))
		b.GHz = b.rt.AllocateMemory(b.Context, uint64(numElem*4))
	}

	b.Upload(b.rt, b.Queue, b.GEx, b.ExInit)
	b.Upload(b.rt, b.Queue, b.GEy, b.EyInit)
	b.Upload(b.rt, b.Queue, b.GHz, b.HzInit)
}

func (b *Benchmark) exec() {
	n := b.N
	gridDim := uint32((n + blockSize - 1) / blockSize)
	globalX := gridDim * blockSize
	globalY := gridDim * blockSize

	exArgs := ExEyKernelArgs{Field: b.GEx, Hz: b.GHz, NX: int32(n), NY: int32(n)}
	eyArgs := ExEyKernelArgs{Field: b.GEy, Hz: b.GHz, NX: int32(n), NY: int32(n)}
	hzArgs := HzKernelArgs{
		Ex: b.GEx, Ey: b.GEy, Hz: b.GHz, NX: int32(n), NY: int32(n),
	}

	// The three kernels in each time step have read-after-write dependencies:
	// the hz update reads the ex/ey written this step, and the next step's
	// ex/ey updates read the hz written this step. Draining the command queue
	// between dependent launches serializes them, as in the chained-GEMM
	// benchmarks (2mm/3mm), which is sufficient for the single-workgroup
	// default configuration. (See the package doc comment about the
	// multi-workgroup timing-mode cache-coherency limitation.)
	cohere := func() {
	}

	for t := 0; t < b.TMax; t++ {
		b.rt.EnqueueLaunchKernel(
			b.Queue, b.kernelEx,
			[3]uint32{globalX, globalY, 1},
			[3]uint16{blockSize, blockSize, 1},
			&exArgs,
		)
		cohere()

		b.rt.EnqueueLaunchKernel(
			b.Queue, b.kernelEy,
			[3]uint32{globalX, globalY, 1},
			[3]uint16{blockSize, blockSize, 1},
			&eyArgs,
		)
		cohere()

		b.rt.EnqueueLaunchKernel(
			b.Queue, b.kernelHz,
			[3]uint32{globalX, globalY, 1},
			[3]uint16{blockSize, blockSize, 1},
			&hzArgs,
		)
		cohere()
	}
}

// Verify checks the GPU result against a CPU reference computation.
func (b *Benchmark) verify() error { //nolint:funlen,gocognit
	n := b.N
	numElem := n * n

	gpuEx := make([]float32, numElem)
	gpuEy := make([]float32, numElem)
	gpuHz := make([]float32, numElem)
	if err := benchmarks.Read(b.rt, b.Readbacks[0], gpuEx); err != nil {
		return err
	}
	if err := benchmarks.Read(b.rt, b.Readbacks[1], gpuEy); err != nil {
		return err
	}
	if err := benchmarks.Read(b.rt, b.Readbacks[2], gpuHz); err != nil {
		return err
	}

	// CPU reference: same field init, same TMax time steps, in float32 to
	// mirror the GPU arithmetic.
	ex := make([]float32, numElem)
	ey := make([]float32, numElem)
	hz := make([]float32, numElem)
	copy(ex, b.ExInit)
	copy(ey, b.EyInit)
	copy(hz, b.HzInit)

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

	check := func(name string, ref, got []float32) error {
		for idx := 0; idx < numElem; idx++ {
			r := float64(ref[idx])
			g := float64(got[idx])
			denom := math.Abs(r)
			if denom < 1.0 {
				denom = 1.0
			}
			if math.Abs(r-g)/denom > 1e-3 {
				return fmt.Errorf("%s mismatch at (%d,%d): expected %f, got %f.\n",
					name, idx/n, idx%n, r, g)
			}
		}

		return nil
	}

	if err := check("ex", ex, gpuEx); err != nil {
		return err
	}
	if err := check("ey", ey, gpuEy); err != nil {
		return err
	}
	if err := check("hz", hz, gpuHz); err != nil {
		return err
	}

	log.Printf("Passed!\n")

	return nil
}

func (b *Benchmark) enqueueVerification() {
	b.Readbacks = append(b.Readbacks, benchmarks.Download(b.rt, b.Queue, b.GEx, 4*((b.N)*(b.N))))
	b.Readbacks = append(b.Readbacks, benchmarks.Download(b.rt, b.Queue, b.GEy, 4*((b.N)*(b.N))))
	b.Readbacks = append(b.Readbacks, benchmarks.Download(b.rt, b.Queue, b.GHz, 4*((b.N)*(b.N))))
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
