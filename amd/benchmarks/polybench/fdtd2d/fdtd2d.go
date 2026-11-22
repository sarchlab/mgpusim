// Package fdtd2d implements the PolyBench FDTD-2D benchmark, ported from
// sarchlab/gpu_benchmarks (tier2/polybench_fdtd2d) for the MGPUSim MI300X
// (CDNA3 / gfx942) model.
//
// FDTD-2D is a 2D Finite Difference Time Domain electromagnetic simulation
// over three NX x NY field arrays (ex, ey, hz). Each time step launches three
// kernels (update ex, update ey, update hz). The kernel binary is compiled for
// gfx942 only (see native/), so the benchmark must be run with `-arch cdna3`
// (the MI300X configuration).
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

// ExKernelArgs is the ABI for updating ex.
type ExKernelArgs struct {
	Field  gpuruntime.Ptr
	Hz     gpuruntime.Ptr
	NX, NY int32
	Launch benchmarks.GFX942LaunchDimensions
}

// EyKernelArgs adds the time-varying input on the boundary row.
type EyKernelArgs struct {
	Field  gpuruntime.Ptr
	Hz     gpuruntime.Ptr
	NX, NY int32
	Fict   float32
	_      uint32
	Launch benchmarks.GFX942LaunchDimensions
}

// HzKernelArgs is the ABI for updating hz.
type HzKernelArgs struct {
	Ex, Ey, Hz gpuruntime.Ptr
	NX, NY     int32
	Launch     benchmarks.GFX942LaunchDimensions
}

// Benchmark defines the FDTD-2D benchmark.
type Benchmark struct {
	*State
	benchmarks.Checkpoint[State]
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
	b.Checkpoint = benchmarks.NewCheckpoint(b.State, b.bind)

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
	rand := benchmarks.NewGlibcRand(42)
	for _, field := range [][]float32{b.ExInit, b.EyInit, b.HzInit} {
		for i := range field {
			field[i] = float32(rand.Next()%100) / 10.0
		}
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
	n := int32(b.N)
	global := uint32((b.N + blockSize - 1) / blockSize * blockSize)
	grid := [3]uint32{global, global, 1}
	group := [3]uint16{blockSize, blockSize, 1}
	launch := benchmarks.NewGFX942LaunchDimensions(grid, group)
	exArgs := ExKernelArgs{Field: b.GEx, Hz: b.GHz, NX: n, NY: n, Launch: launch}
	eyArgs := EyKernelArgs{Field: b.GEy, Hz: b.GHz, NX: n, NY: n, Launch: launch}
	hzArgs := HzKernelArgs{Ex: b.GEx, Ey: b.GEy, Hz: b.GHz, NX: n, NY: n, Launch: launch}

	for t := 0; t < b.TMax; t++ {
		eyArgs.Fict = float32(t)
		b.rt.EnqueueLaunchKernel(b.Queue, b.kernelEx, grid, group, &exArgs)
		b.rt.EnqueueLaunchKernel(b.Queue, b.kernelEy, grid, group, &eyArgs)
		b.rt.EnqueueLaunchKernel(b.Queue, b.kernelHz, grid, group, &hzArgs)
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

	// The CPU reference uses double precision and exact boundary checks.
	ex, ey, hz := make([]float64, numElem), make([]float64, numElem), make([]float64, numElem)
	for i := range ex {
		ex[i], ey[i], hz[i] = float64(b.ExInit[i]), float64(b.EyInit[i]), float64(b.HzInit[i])
	}
	for t := 0; t < b.TMax; t++ {
		for i := 0; i < n; i++ {
			for j := 1; j < n; j++ {
				q := i*n + j
				ex[q] -= 0.5 * (hz[q] - hz[q-1])
			}
			for j := 0; j < n; j++ {
				q := i*n + j
				if i == 0 {
					ey[q] = float64(t)
				} else {
					ey[q] -= 0.5 * (hz[q] - hz[q-n])
				}
			}
		}
		for i := 0; i < n-1; i++ {
			for j := 0; j < n-1; j++ {
				q := i*n + j
				hz[q] -= float64(float32(0.7)) * (ex[q+1] - ex[q] + ey[q+n] - ey[q])
			}
		}
	}

	check := func(name string, ref []float64, got []float32, exact func(int, int) bool) error {
		for idx := 0; idx < numElem; idx++ {
			r := ref[idx]
			g := float64(got[idx])
			tol := 1e-3 * math.Max(1, math.Abs(r))
			if exact(idx/n, idx%n) {
				tol = 0
			}
			if math.IsNaN(g) || math.IsInf(g, 0) || math.Abs(r-g) > tol {
				return fmt.Errorf("%s mismatch at (%d,%d): expected %f, got %f.\n",
					name, idx/n, idx%n, r, g)
			}
		}

		return nil
	}

	if err := check("ex", ex, gpuEx, func(_, j int) bool { return j == 0 }); err != nil {
		return err
	}
	if err := check("ey", ey, gpuEy, func(i, _ int) bool { return i == 0 }); err != nil {
		return err
	}
	if err := check("hz", hz, gpuHz, func(i, j int) bool { return i == n-1 || j == n-1 }); err != nil {
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
