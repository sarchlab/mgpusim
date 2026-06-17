// Package srad implements the Rodinia SRAD benchmark, ported from
// sarchlab/gpu_benchmarks (tier2/rodinia_srad) for the MGPUSim MI300A
// (CDNA3 / gfx942) model.
//
// SRAD (Speckle-Reducing Anisotropic Diffusion) is an iterative 2D image
// smoothing scheme based on Perona-Malik anisotropic diffusion. Each
// iteration runs two stencil kernels:
//
//	srad1: compute directional gradients (dN/dS/dW/dE) and the diffusion
//	       coefficient c for every pixel.
//	srad2: update the image J using those diffusion coefficients.
//
// Both kernels are launched NumIterations times. The kernel binary is
// compiled for gfx942 only (see native/), so the benchmark must be run with
// `-arch cdna3` (the MI300A configuration).
package srad

import (
	"log"
	"math"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// blockSize is the (constant) thread-block dimension baked into the kernel.
const blockSize = 16

// lambda is the SRAD update strength (matches the HIP host default).
const lambda float32 = 0.25

// q0sqr is the speckle-noise parameter (matches the HIP host default).
const q0sqr float32 = 0.05

// Srad1Args defines the kernel arguments for the gfx942 (CDNA3) srad1 kernel.
//
// Verified against the compiled kernel's AMDGPU metadata
// (kernarg_segment_size = 60): six 8-byte global_buffer pointers followed by
// three 4-byte by_value scalars, packed with no padding (mgpusim serializes
// args with binary.Write, which does not insert alignment padding). The
// kernel uses a constant block dimension, so no hidden ABI arguments are
// emitted.
type Srad1Args struct {
	J     driver.Ptr // offset 0
	DN    driver.Ptr // offset 8
	DS    driver.Ptr // offset 16
	DW    driver.Ptr // offset 24
	DE    driver.Ptr // offset 32
	C     driver.Ptr // offset 40
	Rows  int32      // offset 48
	Cols  int32      // offset 52
	Q0sqr float32    // offset 56
}

// Srad2Args defines the kernel arguments for the gfx942 (CDNA3) srad2 kernel.
// Same layout as Srad1Args (kernarg_segment_size = 60); the final scalar is
// lambda instead of q0sqr.
type Srad2Args struct {
	J      driver.Ptr // offset 0
	DN     driver.Ptr // offset 8
	DS     driver.Ptr // offset 16
	DW     driver.Ptr // offset 24
	DE     driver.Ptr // offset 32
	C      driver.Ptr // offset 40
	Rows   int32      // offset 48
	Cols   int32      // offset 52
	Lambda float32    // offset 56
}

// Benchmark defines the SRAD benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	queue   *driver.CommandQueue
	hsaco1  *insts.KernelCodeObject
	hsaco2  *insts.KernelCodeObject
	gpus    []int

	Arch          arch.Type
	ImageSize     int // image is ImageSize x ImageSize
	NumIterations int // SRAD iterations (srad1+srad2 pairs) per run

	jInit []float32 // initial image (reproduced in Verify)

	gJ  driver.Ptr
	gDN driver.Ptr
	gDS driver.Ptr
	gDW driver.Ptr
	gDE driver.Ptr
	gC  driver.Ptr

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new SRAD benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.context = b.driver.Init()
	b.queue = driver.CreateCommandQueue(b.context)

	return b
}

func (b *Benchmark) loadProgram() {
	b.hsaco1 = insts.LoadKernelCodeObjectFromBytes(cdna3HSACOBytes, "srad1")
	if b.hsaco1 == nil {
		log.Panic("Failed to load srad1 kernel binary")
	}
	b.hsaco2 = insts.LoadKernelCodeObjectFromBytes(cdna3HSACOBytes, "srad2")
	if b.hsaco2 == nil {
		log.Panic("Failed to load srad2 kernel binary")
	}
}

// SelectGPU selects the GPUs to run on. SRAD uses a single GPU.
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
		log.Panic("the rodinia srad benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300a")
	}

	b.loadProgram()

	b.driver.SelectGPU(b.context, b.gpus[0])
	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	if b.ImageSize <= 0 {
		b.ImageSize = 64
	}
	if b.NumIterations <= 0 {
		b.NumIterations = 10
	}

	n := b.ImageSize
	total := n * n

	// Deterministic synthetic image in (0, 1], reproducible in Verify.
	b.jInit = make([]float32, total)
	for i := 0; i < total; i++ {
		b.jInit[i] = float32((i%97)+1) / 98.0
	}

	if b.useUnifiedMemory {
		b.gJ = b.driver.AllocateUnifiedMemory(b.context, uint64(total*4))
		b.gDN = b.driver.AllocateUnifiedMemory(b.context, uint64(total*4))
		b.gDS = b.driver.AllocateUnifiedMemory(b.context, uint64(total*4))
		b.gDW = b.driver.AllocateUnifiedMemory(b.context, uint64(total*4))
		b.gDE = b.driver.AllocateUnifiedMemory(b.context, uint64(total*4))
		b.gC = b.driver.AllocateUnifiedMemory(b.context, uint64(total*4))
	} else {
		b.gJ = b.driver.AllocateMemory(b.context, uint64(total*4))
		b.gDN = b.driver.AllocateMemory(b.context, uint64(total*4))
		b.gDS = b.driver.AllocateMemory(b.context, uint64(total*4))
		b.gDW = b.driver.AllocateMemory(b.context, uint64(total*4))
		b.gDE = b.driver.AllocateMemory(b.context, uint64(total*4))
		b.gC = b.driver.AllocateMemory(b.context, uint64(total*4))
	}

	b.driver.MemCopyH2D(b.context, b.gJ, b.jInit)
}

func (b *Benchmark) exec() {
	n := b.ImageSize
	gridDim := uint32((n + blockSize - 1) / blockSize)
	globalX := gridDim * blockSize
	globalY := gridDim * blockSize

	for k := 0; k < b.NumIterations; k++ {
		args1 := Srad1Args{
			J:     b.gJ,
			DN:    b.gDN,
			DS:    b.gDS,
			DW:    b.gDW,
			DE:    b.gDE,
			C:     b.gC,
			Rows:  int32(n),
			Cols:  int32(n),
			Q0sqr: q0sqr,
		}
		b.driver.EnqueueLaunchKernel(
			b.queue,
			b.hsaco1,
			[3]uint32{globalX, globalY, 1},
			[3]uint16{blockSize, blockSize, 1},
			&args1,
		)

		args2 := Srad2Args{
			J:      b.gJ,
			DN:     b.gDN,
			DS:     b.gDS,
			DW:     b.gDW,
			DE:     b.gDE,
			C:      b.gC,
			Rows:   int32(n),
			Cols:   int32(n),
			Lambda: lambda,
		}
		b.driver.EnqueueLaunchKernel(
			b.queue,
			b.hsaco2,
			[3]uint32{globalX, globalY, 1},
			[3]uint16{blockSize, blockSize, 1},
			&args2,
		)

		b.driver.DrainCommandQueue(b.queue)
	}
}

// Verify checks the GPU result against a CPU reference computation that
// reproduces the same two-phase SRAD iteration exactly.
func (b *Benchmark) Verify() { //nolint:funlen,gocognit
	n := b.ImageSize
	total := n * n

	gpu := make([]float32, total)
	b.driver.MemCopyD2H(b.context, gpu, b.gJ)

	jc := make([]float32, total)
	copy(jc, b.jInit)

	dN := make([]float32, total)
	dS := make([]float32, total)
	dW := make([]float32, total)
	dE := make([]float32, total)
	c := make([]float32, total)

	for k := 0; k < b.NumIterations; k++ {
		// Phase 1: gradients + diffusion coefficient.
		for row := 0; row < n; row++ {
			for col := 0; col < n; col++ {
				idx := row*n + col

				iN := row - 1
				if row <= 0 {
					iN = 0
				}
				iS := row + 1
				if row >= n-1 {
					iS = n - 1
				}
				jW := col - 1
				if col <= 0 {
					jW = 0
				}
				jE := col + 1
				if col >= n-1 {
					jE = n - 1
				}

				jcv := jc[idx]
				dn := jc[iN*n+col] - jcv
				ds := jc[iS*n+col] - jcv
				dw := jc[row*n+jW] - jcv
				de := jc[row*n+jE] - jcv

				dN[idx] = dn
				dS[idx] = ds
				dW[idx] = dw
				dE[idx] = de

				g2 := (dn*dn + ds*ds + dw*dw + de*de) / (jcv * jcv)
				l := (dn + ds + dw + de) / jcv
				num := (0.5 * g2) - ((1.0 / 16.0) * (l * l))
				den := 1.0 + (0.25 * l)
				qsqr := num / (den * den)

				den = (qsqr - q0sqr) / (q0sqr * (1.0 + q0sqr))
				ci := 1.0 / (1.0 + den)
				if ci < 0.0 {
					ci = 0.0
				}
				if ci > 1.0 {
					ci = 1.0
				}
				c[idx] = ci
			}
		}

		// Phase 2: image update.
		for row := 0; row < n; row++ {
			for col := 0; col < n; col++ {
				idx := row*n + col

				iS := row + 1
				if row >= n-1 {
					iS = n - 1
				}
				jE := col + 1
				if col >= n-1 {
					jE = n - 1
				}

				cN := c[idx]
				cS := c[iS*n+col]
				cW := c[idx]
				cE := c[row*n+jE]

				d := cN*dN[idx] + cS*dS[idx] + cW*dW[idx] + cE*dE[idx]
				jc[idx] += 0.25 * lambda * d
			}
		}
	}

	for i := 0; i < total; i++ {
		ref := float64(jc[i])
		got := float64(gpu[i])

		denom := math.Abs(ref)
		if denom < 1.0 {
			denom = 1.0
		}
		if math.Abs(ref-got)/denom > 1e-3 {
			log.Fatalf("At pixel %d (row %d, col %d), expected %f, but got %f.\n",
				i, i/n, i%n, ref, got)
		}
	}

	log.Printf("Passed!\n")
}
