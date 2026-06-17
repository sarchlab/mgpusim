// Package lbm implements the Parboil LBM (Lattice Boltzmann Method, D3Q19)
// benchmark, ported from sarchlab/gpu_benchmarks (tier2/parboil_lbm) for the
// MGPUSim MI300A (CDNA3 / gfx942) model.
//
// It simulates fluid flow on a regular NxNxN 3D grid using the D3Q19 lattice.
// A fused collide-stream kernel applies the BGK collision operator and streams
// the post-collision distributions to neighbor cells, with bounce-back
// boundary conditions at the grid boundaries. The host iterates the kernel for
// a fixed number of timesteps, swapping the source/destination distribution
// buffers each step.
//
// The kernel binary is compiled for gfx942 only (see native/), so the
// benchmark must be run with `-arch cdna3` (the MI300A configuration). The
// D3Q19 velocity set, weights and opposite tables live as __constant__ arrays
// baked into the code object, and the kernel uses a constant block size, so no
// hidden ABI arguments are emitted (kernarg_segment_size = 32).
package lbm

import (
	"log"
	"math"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// Q is the number of discrete velocities in the D3Q19 lattice.
const Q = 19

// blockSize is the compile-time block dimension baked into the kernel.
const blockSize = 128

// D3Q19 velocity set, weights and opposite-direction table. These mirror the
// __constant__ tables baked into the kernel and are used by the CPU reference.
var (
	hEx = [Q]int{0, 1, -1, 0, 0, 0, 0, 1, -1, 1, -1, 1, -1, 1, -1, 0, 0, 0, 0}
	hEy = [Q]int{0, 0, 0, 1, -1, 0, 0, 1, 1, -1, -1, 0, 0, 0, 0, 1, -1, 1, -1}
	hEz = [Q]int{0, 0, 0, 0, 0, 1, -1, 0, 0, 0, 0, 1, 1, -1, -1, 1, 1, -1, -1}

	hW = [Q]float32{
		1.0 / 3.0,
		1.0 / 18.0, 1.0 / 18.0, 1.0 / 18.0,
		1.0 / 18.0, 1.0 / 18.0, 1.0 / 18.0,
		1.0 / 36.0, 1.0 / 36.0, 1.0 / 36.0, 1.0 / 36.0,
		1.0 / 36.0, 1.0 / 36.0, 1.0 / 36.0, 1.0 / 36.0,
		1.0 / 36.0, 1.0 / 36.0, 1.0 / 36.0, 1.0 / 36.0,
	}

	hOpp = [Q]int{0, 2, 1, 4, 3, 6, 5, 10, 9, 8, 7, 14, 13, 12, 11, 18, 17, 16, 15}
)

// KernelArgs defines the kernel arguments for the gfx942 (CDNA3) kernel.
//
// The layout is verified against the compiled kernel's AMDGPU metadata
// (kernarg_segment_size = 32): two 8-byte global_buffer pointers followed by
// four 4-byte by_value scalars, packed with no padding (mgpusim serializes
// args with binary.Write, which does not insert alignment padding). The kernel
// uses constant block dimensions and __constant__ tables only, so no hidden
// ABI arguments are emitted.
type KernelArgs struct {
	FSrc  driver.Ptr // offset 0
	FDst  driver.Ptr // offset 8
	Nx    int32      // offset 16
	Ny    int32      // offset 20
	Nz    int32      // offset 24
	Omega float32    // offset 28
}

// Benchmark defines the LBM benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	queue   *driver.CommandQueue
	hsaco   *insts.KernelCodeObject
	gpus    []int

	Arch         arch.Type
	GridDim      int     // N for the NxNxN grid
	NumTimesteps int     // number of collide-stream iterations
	Tau          float32 // relaxation time; omega = 1/tau

	hF    []float32 // host initial distribution (Q*N)
	gFSrc driver.Ptr
	gFDst driver.Ptr

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new LBM benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.context = b.driver.Init()
	b.queue = driver.CreateCommandQueue(b.context)

	return b
}

func (b *Benchmark) loadProgram() {
	b.hsaco = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "lbm_collide_stream_kernel")
	if b.hsaco == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// SelectGPU selects the GPUs to run on. LBM uses a single GPU.
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
		log.Panic("the parboil lbm benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300a")
	}

	b.loadProgram()

	b.driver.SelectGPU(b.context, b.gpus[0])
	b.initMem()
	b.exec()
}

// lcgRand reproduces the host LCG PRNG from the original benchmark.
func lcgRand(state *uint32) uint32 {
	*state = *state*1664525 + 1013904223
	return *state
}

// randFloat reproduces the host rand_float helper.
func randFloat(state *uint32, lo, hi float32) float32 {
	return lo + float32(lcgRand(state)&0xFFFF)/65535.0*(hi-lo)
}

func (b *Benchmark) initMem() {
	if b.GridDim <= 0 {
		b.GridDim = 16
	}
	if b.NumTimesteps <= 0 {
		b.NumTimesteps = 4
	}
	if b.Tau <= 0 {
		b.Tau = 0.7
	}

	n := b.GridDim
	numCells := n * n * n

	// Deterministic equilibrium initialization, identical to the HIP host.
	b.hF = make([]float32, Q*numCells)
	seed := uint32(42)
	for idx := 0; idx < numCells; idx++ {
		rho := 1.0 + 0.001*randFloat(&seed, -1.0, 1.0)
		for q := 0; q < Q; q++ {
			b.hF[q*numCells+idx] = hW[q] * rho
		}
	}

	zeros := make([]float32, Q*numCells)

	if b.useUnifiedMemory {
		b.gFSrc = b.driver.AllocateUnifiedMemory(
			b.context, uint64(Q*numCells*4))
		b.gFDst = b.driver.AllocateUnifiedMemory(
			b.context, uint64(Q*numCells*4))
	} else {
		b.gFSrc = b.driver.AllocateMemory(
			b.context, uint64(Q*numCells*4))
		b.gFDst = b.driver.AllocateMemory(
			b.context, uint64(Q*numCells*4))
	}

	b.driver.MemCopyH2D(b.context, b.gFSrc, b.hF)
	b.driver.MemCopyH2D(b.context, b.gFDst, zeros)
}

func (b *Benchmark) exec() {
	n := b.GridDim
	numCells := n * n * n
	omega := float32(1.0) / b.Tau

	gridSize := uint32((numCells + blockSize - 1) / blockSize)
	globalX := gridSize * blockSize

	src := b.gFSrc
	dst := b.gFDst

	for t := 0; t < b.NumTimesteps; t++ {
		args := KernelArgs{
			FSrc:  src,
			FDst:  dst,
			Nx:    int32(n),
			Ny:    int32(n),
			Nz:    int32(n),
			Omega: omega,
		}

		b.driver.EnqueueLaunchKernel(
			b.queue,
			b.hsaco,
			[3]uint32{globalX, 1, 1},
			[3]uint16{blockSize, 1, 1},
			&args,
		)
		b.driver.DrainCommandQueue(b.queue)

		src, dst = dst, src
	}

	// After the loop, the final result lives in src (it was last written-to
	// destination before the final swap).
	b.gFSrc = src
	b.gFDst = dst
}

// cpuCollideStream runs one collide-stream timestep on the CPU, identical to
// the device kernel.
func cpuCollideStream(fSrc, fDst []float32, n int, omega float32) {
	numCells := n * n * n
	nx, ny, nz := n, n, n

	for idx := 0; idx < numCells; idx++ {
		iz := idx / (nx * ny)
		iy := (idx / nx) % ny
		ix := idx % nx

		isBoundary := ix == 0 || ix == nx-1 ||
			iy == 0 || iy == ny-1 ||
			iz == 0 || iz == nz-1

		var f [Q]float32
		for q := 0; q < Q; q++ {
			f[q] = fSrc[q*numCells+idx]
		}

		var rho, ux, uy, uz float32
		for q := 0; q < Q; q++ {
			rho += f[q]
			ux += f[q] * float32(hEx[q])
			uy += f[q] * float32(hEy[q])
			uz += f[q] * float32(hEz[q])
		}
		invRho := 1.0 / float32(math.Max(float64(rho), 1e-10))
		ux *= invRho
		uy *= invRho
		uz *= invRho

		u2 := ux*ux + uy*uy + uz*uz
		var fPost [Q]float32
		for q := 0; q < Q; q++ {
			eu := float32(hEx[q])*ux + float32(hEy[q])*uy + float32(hEz[q])*uz
			fEq := hW[q] * rho * (1.0 + 3.0*eu + 4.5*eu*eu - 1.5*u2)
			fPost[q] = f[q] + omega*(fEq-f[q])
		}

		for q := 0; q < Q; q++ {
			nxi := ix + hEx[q]
			nyi := iy + hEy[q]
			nzi := iz + hEz[q]

			if isBoundary {
				fDst[hOpp[q]*numCells+idx] = fPost[q]
			} else if nxi >= 0 && nxi < nx && nyi >= 0 && nyi < ny &&
				nzi >= 0 && nzi < nz {
				nidx := nzi*nx*ny + nyi*nx + nxi
				fDst[q*numCells+nidx] = fPost[q]
			} else {
				fDst[hOpp[q]*numCells+idx] = fPost[q]
			}
		}
	}
}

// Verify checks the GPU result against a CPU reference computation.
func (b *Benchmark) Verify() {
	n := b.GridDim
	numCells := n * n * n
	omega := float32(1.0) / b.Tau

	gpuF := make([]float32, Q*numCells)
	b.driver.MemCopyD2H(b.context, gpuF, b.gFSrc)

	// CPU reference: replay the exact same timesteps with buffer swapping.
	src := make([]float32, Q*numCells)
	dst := make([]float32, Q*numCells)
	copy(src, b.hF)
	for i := range dst {
		dst[i] = 0
	}

	for t := 0; t < b.NumTimesteps; t++ {
		cpuCollideStream(src, dst, n, omega)
		src, dst = dst, src
	}
	// src now holds the final result.

	for i := 0; i < Q*numCells; i++ {
		ref := float64(src[i])
		got := float64(gpuF[i])

		denom := math.Abs(ref)
		if denom < 1.0 {
			denom = 1.0
		}
		if math.Abs(ref-got)/denom > 1e-3 {
			q := i / numCells
			cell := i % numCells
			log.Fatalf("At q=%d cell=%d, expected %f, but got %f.\n",
				q, cell, ref, got)
		}
	}

	log.Printf("Passed!\n")
}
