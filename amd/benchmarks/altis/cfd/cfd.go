// Package cfd implements the Altis CFD benchmark, ported from
// sarchlab/gpu_benchmarks (tier2/altis_cfd) for the MGPUSim MI300X
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
// benchmark must be run with `-arch cdna3` (the MI300X configuration).
package cfd

import (
	// embed hsaco files
	_ "embed"
	"fmt"
	"log"
	"math"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
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
	*State
	benchmarks.Saved[checkpointConfig, State]

	driver *driver.Runtime

	hsaco *insts.KernelCodeObject
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new Altis CFD benchmark.
func NewBenchmark(driver *driver.Runtime) *Benchmark {
	b := new(Benchmark)
	b.State = &State{}
	b.Saved = benchmarks.NewSaved(b.State, b.checkpointConfig, b.bind)

	b.driver = driver
	b.Context = b.driver.Init()

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
	b.Gpus = gpus
}

// SetUnifiedMemory requests the use of unified memory.
func (b *Benchmark) SetUnifiedMemory() {
	b.UseUnifiedMemory = true
}

// Prepare the inputs for the first queued phase.
func (b *Benchmark) submit() {
	if b.Arch != arch.CDNA3 {
		log.Panic("the altis cfd benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300x")
	}

	b.loadProgram()

	b.driver.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.driver.CreateCommandQueue(b.Context)
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

	b.HRho = make([]float32, n)
	b.HMx = make([]float32, n)
	b.HMy = make([]float32, n)
	b.HMz = make([]float32, n)
	b.HEnergy = make([]float32, n)
	b.HNeighbors = make([]int32, n*numNeighbors)
	b.HNormals = make([]float32, n*numNeighbors*3)
	b.HAreas = make([]float32, n*numNeighbors)

	var seed uint32 = 42

	for i := 0; i < n; i++ {
		b.HRho[i] = 1.0 + 0.01*randFloat(&seed, -1.0, 1.0)
		b.HMx[i] = 0.5 + 0.01*randFloat(&seed, -1.0, 1.0)
		b.HMy[i] = 0.0 + 0.01*randFloat(&seed, -1.0, 1.0)
		b.HMz[i] = 0.0 + 0.01*randFloat(&seed, -1.0, 1.0)
		p := 1.0 / gamma
		ke := 0.5 * (b.HMx[i]*b.HMx[i] + b.HMy[i]*b.HMy[i] + b.HMz[i]*b.HMz[i]) / b.HRho[i]
		b.HEnergy[i] = p/gammaM1 + ke
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
			b.HNeighbors[i*numNeighbors+f] = int32(j)

			nx := randFloat(&seed, -1.0, 1.0)
			ny := randFloat(&seed, -1.0, 1.0)
			nz := randFloat(&seed, -1.0, 1.0)
			length := float32(math.Sqrt(float64(nx*nx + ny*ny + nz*nz)))
			if length < 1e-6 {
				nx, ny, nz, length = 1.0, 0.0, 0.0, 1.0
			}
			base := (i*numNeighbors + f) * 3
			b.HNormals[base+0] = nx / length
			b.HNormals[base+1] = ny / length
			b.HNormals[base+2] = nz / length

			b.HAreas[i*numNeighbors+f] = 0.001 + 0.0005*randFloat(&seed, 0.0, 1.0)
		}
	}

	alloc := b.driver.AllocateMemory
	if b.UseUnifiedMemory {
		alloc = b.driver.AllocateUnifiedMemory
	}

	cellBytes := uint64(n * 4)
	neighBytes := uint64(n * numNeighbors * 4)
	normalBytes := uint64(n * numNeighbors * 3 * 4)
	areaBytes := uint64(n * numNeighbors * 4)

	b.GRho = alloc(b.Context, cellBytes)
	b.GMx = alloc(b.Context, cellBytes)
	b.GMy = alloc(b.Context, cellBytes)
	b.GMz = alloc(b.Context, cellBytes)
	b.GEnergy = alloc(b.Context, cellBytes)
	b.GNeighbors = alloc(b.Context, neighBytes)
	b.GNormals = alloc(b.Context, normalBytes)
	b.GAreas = alloc(b.Context, areaBytes)
	b.GFluxRho = alloc(b.Context, cellBytes)
	b.GFluxMx = alloc(b.Context, cellBytes)
	b.GFluxMy = alloc(b.Context, cellBytes)
	b.GFluxMz = alloc(b.Context, cellBytes)
	b.GFluxEnergy = alloc(b.Context, cellBytes)

	b.driver.EnqueueMemCopyH2D(b.Queue, b.GRho, b.HRho)
	b.driver.EnqueueMemCopyH2D(b.Queue, b.GMx, b.HMx)
	b.driver.EnqueueMemCopyH2D(b.Queue, b.GMy, b.HMy)
	b.driver.EnqueueMemCopyH2D(b.Queue, b.GMz, b.HMz)
	b.driver.EnqueueMemCopyH2D(b.Queue, b.GEnergy, b.HEnergy)
	b.driver.EnqueueMemCopyH2D(b.Queue, b.GNeighbors, b.HNeighbors)
	b.driver.EnqueueMemCopyH2D(b.Queue, b.GNormals, b.HNormals)
	b.driver.EnqueueMemCopyH2D(b.Queue, b.GAreas, b.HAreas)
}

func (b *Benchmark) exec() {
	n := b.N
	gridX := uint32((n + blockSize - 1) / blockSize)
	globalX := gridX * blockSize

	args := KernelArgs{
		Rho:        b.GRho,
		Mx:         b.GMx,
		My:         b.GMy,
		Mz:         b.GMz,
		Energy:     b.GEnergy,
		Neighbors:  b.GNeighbors,
		Normals:    b.GNormals,
		Areas:      b.GAreas,
		FluxRho:    b.GFluxRho,
		FluxMx:     b.GFluxMx,
		FluxMy:     b.GFluxMy,
		FluxMz:     b.GFluxMz,
		FluxEnergy: b.GFluxEnergy,
		N:          int32(n),
	}

	b.driver.EnqueueLaunchKernel(
		b.Queue,
		b.hsaco,
		[3]uint32{globalX, 1, 1},
		[3]uint16{blockSize, 1, 1},
		&args,
	)
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
	rhoI := b.HRho[idx]
	mxI := b.HMx[idx]
	myI := b.HMy[idx]
	mzI := b.HMz[idx]
	eI := b.HEnergy[idx]
	pI := cpuPressure(rhoI, mxI, myI, mzI, eI)
	aI := cpuSpeedOfSound(rhoI, pI)

	invRhoI := 1.0 / fmax32(rhoI, 1e-10)
	vxI := mxI * invRhoI
	vyI := myI * invRhoI
	vzI := mzI * invRhoI

	for f := 0; f < numNeighbors; f++ {
		j := int(b.HNeighbors[idx*numNeighbors+f])
		nbase := (idx*numNeighbors + f) * 3
		nx := b.HNormals[nbase+0]
		ny := b.HNormals[nbase+1]
		nz := b.HNormals[nbase+2]
		area := b.HAreas[idx*numNeighbors+f]

		rhoJ := b.HRho[j]
		mxJ := b.HMx[j]
		myJ := b.HMy[j]
		mzJ := b.HMz[j]
		eJ := b.HEnergy[j]
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
func (b *Benchmark) verify() error {
	n := b.N

	gpuFluxRho := make([]float32, n)
	gpuFluxMx := make([]float32, n)
	gpuFluxMy := make([]float32, n)
	gpuFluxMz := make([]float32, n)
	gpuFluxEnergy := make([]float32, n)
	if err := b.driver.ReadResult(b.Readbacks[0], gpuFluxRho); err != nil {
		return err
	}
	if err := b.driver.ReadResult(b.Readbacks[1], gpuFluxMx); err != nil {
		return err
	}
	if err := b.driver.ReadResult(b.Readbacks[2], gpuFluxMy); err != nil {
		return err
	}
	if err := b.driver.ReadResult(b.Readbacks[3], gpuFluxMz); err != nil {
		return err
	}
	if err := b.driver.ReadResult(b.Readbacks[4], gpuFluxEnergy); err != nil {
		return err
	}

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
				return fmt.Errorf("At cell %d %s, expected %f, but got %f.\n",
					i, names[k], ref[k], got[k])
			}
		}
	}

	log.Printf("Passed!\n")

	return nil
}

func (b *Benchmark) enqueueVerification() {
	b.Readbacks = append(b.Readbacks, b.driver.EnqueueMemCopyD2H(b.Queue, b.GFluxRho, 4*(b.N)))
	b.Readbacks = append(b.Readbacks, b.driver.EnqueueMemCopyD2H(b.Queue, b.GFluxMx, 4*(b.N)))
	b.Readbacks = append(b.Readbacks, b.driver.EnqueueMemCopyD2H(b.Queue, b.GFluxMy, 4*(b.N)))
	b.Readbacks = append(b.Readbacks, b.driver.EnqueueMemCopyD2H(b.Queue, b.GFluxMz, 4*(b.N)))
	b.Readbacks = append(b.Readbacks, b.driver.EnqueueMemCopyD2H(b.Queue, b.GFluxEnergy, 4*(b.N)))
}

// Step submits computation, then verification readback, then consumes results.
func (b *Benchmark) Step() error {
	if err := b.Prepare(); err != nil {
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
