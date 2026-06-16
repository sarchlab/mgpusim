// Package cfd implements the Altis CFD benchmark, ported from
// sarchlab/gpu_benchmarks (tier2/altis_cfd) for the MGPUSim MI300A
// (CDNA3 / gfx942) model.
//
// It solves the compressible Euler equations on a synthetic unstructured
// mesh using a Rusanov (local Lax-Friedrichs) finite-volume flux scheme.
// Each work-item processes one mesh cell and accumulates flux
// contributions from its NUM_NEIGHBORS face neighbors. This port runs the
// dominant compute kernel (compute_flux_kernel) once and verifies its
// output against a CPU reference.
//
// The kernel binary is compiled for gfx942 only (see native/), so the
// benchmark must be run with `-arch cdna3` (the MI300A configuration).
package cfd

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
	blockSize    = 256
	numNeighbors = 4
	gamma        = float32(1.4)
	gammaM1      = float32(0.4)
)

// KernelArgs defines the kernel arguments for the gfx942 (CDNA3) kernel.
//
// The layout below is verified against the compiled kernel's AMDGPU
// metadata (kernarg_segment_size = 108): thirteen 8-byte global_buffer
// pointers followed by one 4-byte by_value scalar (int N), packed with no
// padding (mgpusim serializes args with binary.Write, which does not
// insert alignment padding). The kernel uses a constant BLOCK_SIZE for the
// block geometry, so no hidden ABI arguments are emitted.
type KernelArgs struct {
	Rho        driver.Ptr // offset 0
	Mx         driver.Ptr // offset 8
	My         driver.Ptr // offset 16
	Mz         driver.Ptr // offset 24
	Energy     driver.Ptr // offset 32
	Neighbors  driver.Ptr // offset 40
	Normals    driver.Ptr // offset 48
	Areas      driver.Ptr // offset 56
	FluxRho    driver.Ptr // offset 64
	FluxMx     driver.Ptr // offset 72
	FluxMy     driver.Ptr // offset 80
	FluxMz     driver.Ptr // offset 88
	FluxEnergy driver.Ptr // offset 96
	N          int32      // offset 104
}

// Benchmark defines the Altis CFD benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	queue   *driver.CommandQueue
	hsaco   *insts.KernelCodeObject
	gpus    []int

	Arch arch.Type
	N    int

	// Host data (deterministic, reproducible in Verify).
	hRho       []float32
	hMx        []float32
	hMy        []float32
	hMz        []float32
	hEnergy    []float32
	hNeighbors []int32
	hNormals   []float32
	hAreas     []float32

	// Device buffers.
	gRho        driver.Ptr
	gMx         driver.Ptr
	gMy         driver.Ptr
	gMz         driver.Ptr
	gEnergy     driver.Ptr
	gNeighbors  driver.Ptr
	gNormals    driver.Ptr
	gAreas      driver.Ptr
	gFluxRho    driver.Ptr
	gFluxMx     driver.Ptr
	gFluxMy     driver.Ptr
	gFluxMz     driver.Ptr
	gFluxEnergy driver.Ptr

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new Altis CFD benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.context = b.driver.Init()
	b.queue = driver.CreateCommandQueue(b.context)

	return b
}

func (b *Benchmark) loadProgram() {
	b.hsaco = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "compute_flux_kernel")
	if b.hsaco == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// SelectGPU selects the GPUs to run on. CFD uses a single GPU.
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
		log.Panic("the altis cfd benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300a")
	}

	b.loadProgram()

	b.driver.SelectGPU(b.context, b.gpus[0])
	b.initMem()
	b.exec()
}

// lcgRand mirrors the host PRNG used by the original HIP benchmark so the
// synthetic mesh data (and therefore the GPU result) is reproducible.
func lcgRand(state *uint32) uint32 {
	*state = *state*1664525 + 1013904223
	return *state
}

func randFloat(state *uint32, lo, hi float32) float32 {
	return lo + float32(lcgRand(state)&0xFFFF)/65535.0*(hi-lo)
}

func (b *Benchmark) initMem() { //nolint:funlen,gocognit
	if b.N <= 0 {
		b.N = 256
	}
	n := b.N

	b.hRho = make([]float32, n)
	b.hMx = make([]float32, n)
	b.hMy = make([]float32, n)
	b.hMz = make([]float32, n)
	b.hEnergy = make([]float32, n)
	b.hNeighbors = make([]int32, n*numNeighbors)
	b.hNormals = make([]float32, n*numNeighbors*3)
	b.hAreas = make([]float32, n*numNeighbors)

	var seed uint32 = 42

	for i := 0; i < n; i++ {
		b.hRho[i] = 1.0 + 0.01*randFloat(&seed, -1.0, 1.0)
		b.hMx[i] = 0.5 + 0.01*randFloat(&seed, -1.0, 1.0)
		b.hMy[i] = 0.0 + 0.01*randFloat(&seed, -1.0, 1.0)
		b.hMz[i] = 0.0 + 0.01*randFloat(&seed, -1.0, 1.0)
		p := 1.0 / gamma
		ke := 0.5 * (b.hMx[i]*b.hMx[i] + b.hMy[i]*b.hMy[i] + b.hMz[i]*b.hMz[i]) / b.hRho[i]
		b.hEnergy[i] = p/gammaM1 + ke
		// volumes consumed from the PRNG stream in the original host code,
		// even though this port does not use the rk_update kernel; keep the
		// stream in lockstep so the neighbor/geometry data matches exactly.
		_ = 0.01 + 0.001*randFloat(&seed, 0.0, 1.0)
	}

	for i := 0; i < n; i++ {
		for f := 0; f < numNeighbors; f++ {
			var j int
			for {
				j = int(lcgRand(&seed) % uint32(n))
				if j != i {
					break
				}
			}
			b.hNeighbors[i*numNeighbors+f] = int32(j)

			nx := randFloat(&seed, -1.0, 1.0)
			ny := randFloat(&seed, -1.0, 1.0)
			nz := randFloat(&seed, -1.0, 1.0)
			length := float32(math.Sqrt(float64(nx*nx + ny*ny + nz*nz)))
			if length < 1e-6 {
				nx, ny, nz, length = 1.0, 0.0, 0.0, 1.0
			}
			base := (i*numNeighbors + f) * 3
			b.hNormals[base+0] = nx / length
			b.hNormals[base+1] = ny / length
			b.hNormals[base+2] = nz / length

			b.hAreas[i*numNeighbors+f] = 0.001 + 0.0005*randFloat(&seed, 0.0, 1.0)
		}
	}

	alloc := b.driver.AllocateMemory
	if b.useUnifiedMemory {
		alloc = b.driver.AllocateUnifiedMemory
	}

	cellBytes := uint64(n * 4)
	neighBytes := uint64(n * numNeighbors * 4)
	normalBytes := uint64(n * numNeighbors * 3 * 4)
	areaBytes := uint64(n * numNeighbors * 4)

	b.gRho = alloc(b.context, cellBytes)
	b.gMx = alloc(b.context, cellBytes)
	b.gMy = alloc(b.context, cellBytes)
	b.gMz = alloc(b.context, cellBytes)
	b.gEnergy = alloc(b.context, cellBytes)
	b.gNeighbors = alloc(b.context, neighBytes)
	b.gNormals = alloc(b.context, normalBytes)
	b.gAreas = alloc(b.context, areaBytes)
	b.gFluxRho = alloc(b.context, cellBytes)
	b.gFluxMx = alloc(b.context, cellBytes)
	b.gFluxMy = alloc(b.context, cellBytes)
	b.gFluxMz = alloc(b.context, cellBytes)
	b.gFluxEnergy = alloc(b.context, cellBytes)

	b.driver.MemCopyH2D(b.context, b.gRho, b.hRho)
	b.driver.MemCopyH2D(b.context, b.gMx, b.hMx)
	b.driver.MemCopyH2D(b.context, b.gMy, b.hMy)
	b.driver.MemCopyH2D(b.context, b.gMz, b.hMz)
	b.driver.MemCopyH2D(b.context, b.gEnergy, b.hEnergy)
	b.driver.MemCopyH2D(b.context, b.gNeighbors, b.hNeighbors)
	b.driver.MemCopyH2D(b.context, b.gNormals, b.hNormals)
	b.driver.MemCopyH2D(b.context, b.gAreas, b.hAreas)
}

func (b *Benchmark) exec() {
	n := b.N
	gridX := uint32((n + blockSize - 1) / blockSize)
	globalX := gridX * blockSize

	args := KernelArgs{
		Rho:        b.gRho,
		Mx:         b.gMx,
		My:         b.gMy,
		Mz:         b.gMz,
		Energy:     b.gEnergy,
		Neighbors:  b.gNeighbors,
		Normals:    b.gNormals,
		Areas:      b.gAreas,
		FluxRho:    b.gFluxRho,
		FluxMx:     b.gFluxMx,
		FluxMy:     b.gFluxMy,
		FluxMz:     b.gFluxMz,
		FluxEnergy: b.gFluxEnergy,
		N:          int32(n),
	}

	b.driver.EnqueueLaunchKernel(
		b.queue,
		b.hsaco,
		[3]uint32{globalX, 1, 1},
		[3]uint16{blockSize, 1, 1},
		&args,
	)

	b.driver.DrainCommandQueue(b.queue)
}

// cpuPressure / cpuSpeedOfSound mirror the device helpers.
func cpuPressure(rho, mx, my, mz, e float32) float32 {
	ke := 0.5 * (mx*mx + my*my + mz*mz) / fmax32(rho, 1e-10)
	return gammaM1 * (e - ke)
}

func cpuSpeedOfSound(rho, p float32) float32 {
	return float32(math.Sqrt(float64(fmax32(gamma*p/fmax32(rho, 1e-10), 1e-10))))
}

func fmax32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

func fabs32(a float32) float32 {
	if a < 0 {
		return -a
	}
	return a
}

// cpuFlux computes the Rusanov flux for a single cell, identical to the
// device kernel, for verification.
func (b *Benchmark) cpuFlux(idx int) (fRho, fMx, fMy, fMz, fE float32) { //nolint:funlen,gocognit
	rhoI := b.hRho[idx]
	mxI := b.hMx[idx]
	myI := b.hMy[idx]
	mzI := b.hMz[idx]
	eI := b.hEnergy[idx]
	pI := cpuPressure(rhoI, mxI, myI, mzI, eI)
	aI := cpuSpeedOfSound(rhoI, pI)

	invRhoI := 1.0 / fmax32(rhoI, 1e-10)
	vxI := mxI * invRhoI
	vyI := myI * invRhoI
	vzI := mzI * invRhoI

	for f := 0; f < numNeighbors; f++ {
		j := int(b.hNeighbors[idx*numNeighbors+f])
		nbase := (idx*numNeighbors + f) * 3
		nx := b.hNormals[nbase+0]
		ny := b.hNormals[nbase+1]
		nz := b.hNormals[nbase+2]
		area := b.hAreas[idx*numNeighbors+f]

		rhoJ := b.hRho[j]
		mxJ := b.hMx[j]
		myJ := b.hMy[j]
		mzJ := b.hMz[j]
		eJ := b.hEnergy[j]
		pJ := cpuPressure(rhoJ, mxJ, myJ, mzJ, eJ)
		aJ := cpuSpeedOfSound(rhoJ, pJ)

		invRhoJ := 1.0 / fmax32(rhoJ, 1e-10)
		vxJ := mxJ * invRhoJ
		vyJ := myJ * invRhoJ
		vzJ := mzJ * invRhoJ

		vnI := vxI*nx + vyI*ny + vzI*nz
		vnJ := vxJ*nx + vyJ*ny + vzJ*nz

		fRhoI := rhoI * vnI
		fRhoJ := rhoJ * vnJ
		fMxI := mxI*vnI + pI*nx
		fMxJ := mxJ*vnJ + pJ*nx
		fMyI := myI*vnI + pI*ny
		fMyJ := myJ*vnJ + pJ*ny
		fMzI := mzI*vnI + pI*nz
		fMzJ := mzJ*vnJ + pJ*nz
		fEI := (eI + pI) * vnI
		fEJ := (eJ + pJ) * vnJ

		lambda := fmax32(fabs32(vnI)+aI, fabs32(vnJ)+aJ)

		fRho += area * (0.5*(fRhoI+fRhoJ) - 0.5*lambda*(rhoJ-rhoI))
		fMx += area * (0.5*(fMxI+fMxJ) - 0.5*lambda*(mxJ-mxI))
		fMy += area * (0.5*(fMyI+fMyJ) - 0.5*lambda*(myJ-myI))
		fMz += area * (0.5*(fMzI+fMzJ) - 0.5*lambda*(mzJ-mzI))
		fE += area * (0.5*(fEI+fEJ) - 0.5*lambda*(eJ-eI))
	}

	return fRho, fMx, fMy, fMz, fE
}

// Verify checks the GPU flux output against a CPU reference computation.
func (b *Benchmark) Verify() {
	n := b.N

	gpuFluxRho := make([]float32, n)
	gpuFluxMx := make([]float32, n)
	gpuFluxMy := make([]float32, n)
	gpuFluxMz := make([]float32, n)
	gpuFluxEnergy := make([]float32, n)
	b.driver.MemCopyD2H(b.context, gpuFluxRho, b.gFluxRho)
	b.driver.MemCopyD2H(b.context, gpuFluxMx, b.gFluxMx)
	b.driver.MemCopyD2H(b.context, gpuFluxMy, b.gFluxMy)
	b.driver.MemCopyD2H(b.context, gpuFluxMz, b.gFluxMz)
	b.driver.MemCopyD2H(b.context, gpuFluxEnergy, b.gFluxEnergy)

	for i := 0; i < n; i++ {
		refRho, refMx, refMy, refMz, refE := b.cpuFlux(i)
		got := []float32{
			gpuFluxRho[i], gpuFluxMx[i], gpuFluxMy[i],
			gpuFluxMz[i], gpuFluxEnergy[i],
		}
		ref := []float32{refRho, refMx, refMy, refMz, refE}
		names := []string{"flux_rho", "flux_mx", "flux_my", "flux_mz", "flux_energy"}

		for k := range ref {
			denom := math.Abs(float64(ref[k]))
			if denom < 1e-3 {
				denom = 1e-3
			}
			if math.Abs(float64(ref[k]-got[k]))/denom > 1e-3 {
				log.Fatalf("At cell %d %s, expected %f, but got %f.\n",
					i, names[k], ref[k], got[k])
			}
		}
	}

	log.Printf("Passed!\n")
}
