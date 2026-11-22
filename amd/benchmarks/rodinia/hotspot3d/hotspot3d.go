// Package hotspot3d implements the Rodinia HotSpot3D benchmark, ported from
// sarchlab/gpu_benchmarks (tier2/rodinia_hotspot3d) for the MGPUSim MI300X
// (CDNA3 / gfx942) model.
//
// HotSpot3D is a 3D stencil thermal simulation. For an NxNxN grid, each
// cell's temperature is updated from its six neighbors (+/-x, +/-y, +/-z),
// the local power density, and a set of thermal resistances. Boundaries are
// clamped (an edge cell substitutes its own value for the missing neighbor).
// The host iterates the stencil num_iterations times, ping-ponging between
// two temperature buffers.
//
// The kernel binary is compiled for gfx942 only (see native/), so the
// benchmark must be run with `-arch cdna3` (the MI300X configuration).
package hotspot3d

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

const defaultBlockSize = 8

// Coefficients follows the v1.1 ABI, including the host-derived quantities
// preceding the stencil weights.
type Coefficients struct {
	Cap, RX, RY, RZ, DT, UpstreamDT float32
	SDC, CE, CW, CN, CS, CT, CB, CC float32
}

// KernelArgs defines the arguments for the v1.1 HotSpot3D kernel.
type KernelArgs struct {
	TempSrc, TempDst, Power gpuruntime.Ptr
	N                       int32
	Coefficients            Coefficients
	Ambient                 float32
	Launch                  benchmarks.GFX942LaunchDimensions
}

// Benchmark defines the HotSpot3D benchmark.
type Benchmark struct {
	*State
	benchmarks.Checkpoint[State]
	rt *gpuruntime.Runtime

	hsaco *insts.KernelCodeObject
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new HotSpot3D benchmark.
func NewBenchmark(rt *gpuruntime.Runtime) *Benchmark {
	b := new(Benchmark)
	b.State = &State{}
	b.Checkpoint = benchmarks.NewCheckpoint(b.State, b.bind)

	b.rt = rt
	b.Context = b.rt.Init()

	return b
}

func (b *Benchmark) loadProgram() {
	b.hsaco = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "hotspot3d_kernel")
	if b.hsaco == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// SelectGPU selects the GPUs to run on. HotSpot3D uses a single GPU.
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
		log.Panic("the rodinia hotspot3d benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300x")
	}

	b.loadProgram()

	b.rt.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.rt.CreateCommandQueue(b.Context)
	b.initMem()
	b.exec()
}

// computeThermalParams computes the derived thermal parameters exactly as the
// HIP host does, so the Verify() reference uses identical coefficients.
func roundedUp(value float64) float64 {
	return math.Nextafter(value, math.Inf(1))
}

func (c Coefficients) rowNorm() float64 {
	var norm float64
	for _, value := range []float32{c.CC, c.CW, c.CE, c.CS, c.CN, c.CB, c.CT} {
		norm = roundedUp(norm + math.Abs(float64(value)))
	}
	return norm
}

func (b *Benchmark) computeThermalParams() {
	const thickness float32 = 0.0005
	dx := float32(0.016) / float32(b.GridSize)
	dy := float32(0.016) / float32(b.GridSize)
	dz := thickness / float32(b.GridSize)
	c := Coefficients{
		Cap: float32(0.5 * 1.75e6 * float64(thickness) * float64(dx) * float64(dy)),
		RX:  float32(float64(dy) / (2 * 100 * float64(thickness) * float64(dx))),
		RY:  float32(float64(dx) / (2 * 100 * float64(thickness) * float64(dy))),
		RZ:  dz / (100 * dx * dy),
	}
	maxSlope := float32(3e6 / (0.5 * float64(thickness) * 1.75e6))
	c.UpstreamDT = float32(0.001 / float64(maxSlope))
	c.DT = c.UpstreamDT
	setWeights := func() {
		c.SDC = c.DT / c.Cap
		c.CE, c.CW = c.SDC/c.RX, c.SDC/c.RX
		c.CN, c.CS = c.SDC/c.RY, c.SDC/c.RY
		c.CT, c.CB = c.SDC/c.RZ, c.SDC/c.RZ
		c.CC = float32(1 - (2*float64(c.CE) + 2*float64(c.CN) + 3*float64(c.CT)))
	}
	setWeights()
	if c.rowNorm() > 1 {
		c.DT = float32(0.9 * float64(c.Cap) / (2/float64(c.RX) + 2/float64(c.RY) + 3/float64(c.RZ)))
		setWeights()
	}
	if c.rowNorm() > 1 || math.IsNaN(c.rowNorm()) {
		panic("HotSpot3D stencil coefficients amplify the input")
	}
	b.Coefficients = c
}

func (b *Benchmark) initMem() {
	if b.GridSize <= 0 {
		b.GridSize = 32
	}
	if b.NumIterations <= 0 {
		b.NumIterations = 2
	}
	if b.AmbTemp <= 0 {
		b.AmbTemp = 80.0
	}

	b.computeThermalParams()

	n := b.GridSize
	totalCells := n * n * n

	b.TempInit = make([]float32, totalCells)
	b.Power = make([]float32, totalCells)
	rand := benchmarks.NewGlibcRand(42)
	for i := range b.TempInit {
		b.TempInit[i] = b.AmbTemp + float32(rand.Next()%200)/10.0
		b.Power[i] = float32(rand.Next()%100) / 500.0
	}

	bytes := uint64(totalCells * 4)
	if b.UseUnifiedMemory {
		b.GTemp[0] = b.rt.AllocateUnifiedMemory(b.Context, bytes)
		b.GTemp[1] = b.rt.AllocateUnifiedMemory(b.Context, bytes)
		b.GPower = b.rt.AllocateUnifiedMemory(b.Context, bytes)
	} else {
		b.GTemp[0] = b.rt.AllocateMemory(b.Context, bytes)
		b.GTemp[1] = b.rt.AllocateMemory(b.Context, bytes)
		b.GPower = b.rt.AllocateMemory(b.Context, bytes)
	}

	b.Upload(b.rt, b.Queue, b.GTemp[0], b.TempInit)
	b.Upload(b.rt, b.Queue, b.GPower, b.Power)
}

func (b *Benchmark) exec() {
	block := b.BlockSize
	if block == 0 {
		block = defaultBlockSize
	}
	if block < 1 || block > 10 {
		panic("HotSpot3D block edge must be between 1 and 10")
	}
	global := uint32((b.GridSize + block - 1) / block * block)
	grid := [3]uint32{global, global, global}
	group := [3]uint16{uint16(block), uint16(block), uint16(block)}
	launch := benchmarks.NewGFX942LaunchDimensions(grid, group)
	src, dst := 0, 1
	for i := 0; i < b.NumIterations; i++ {
		args := KernelArgs{TempSrc: b.GTemp[src], TempDst: b.GTemp[dst], Power: b.GPower,
			N: int32(b.GridSize), Coefficients: b.Coefficients, Ambient: b.AmbTemp, Launch: launch}
		b.rt.EnqueueLaunchKernel(b.Queue, b.hsaco, grid, group, &args)
		src, dst = dst, src
	}
	b.FinalBuf = src
}

// cpuStep applies one stencil time-step on the host, matching the kernel.
func (b *Benchmark) cpuStep(src, dst []float64) {
	n := b.GridSize
	for z := 0; z < n; z++ {
		for y := 0; y < n; y++ {
			for x := 0; x < n; x++ {
				idx := z*n*n + y*n + x
				tc := src[idx]

				txm := tc
				if x > 0 {
					txm = src[z*n*n+y*n+(x-1)]
				}
				txp := tc
				if x < n-1 {
					txp = src[z*n*n+y*n+(x+1)]
				}
				tym := tc
				if y > 0 {
					tym = src[z*n*n+(y-1)*n+x]
				}
				typ := tc
				if y < n-1 {
					typ = src[z*n*n+(y+1)*n+x]
				}
				tzm := tc
				if z > 0 {
					tzm = src[(z-1)*n*n+y*n+x]
				}
				tzp := tc
				if z < n-1 {
					tzp = src[(z+1)*n*n+y*n+x]
				}

				c := b.Coefficients
				dst[idx] = float64(c.CC)*tc + float64(c.CW)*txm + float64(c.CE)*txp +
					float64(c.CS)*typ + float64(c.CN)*tym + float64(c.CB)*tzm +
					float64(c.CT)*tzp + float64(c.SDC)*float64(b.Power[idx]) +
					float64(c.CT)*float64(b.AmbTemp)
			}
		}
	}
}

func (b *Benchmark) errorBound() float64 {
	var magnitude, power float64
	for i, temperature := range b.TempInit {
		magnitude = max(magnitude, math.Abs(float64(temperature)))
		power = max(power, math.Abs(float64(b.Power[i])))
	}
	c := b.Coefficients
	g := c.rowNorm()
	source := roundedUp(roundedUp(math.Abs(float64(c.SDC))*power) +
		roundedUp(math.Abs(float64(c.CT))*math.Abs(float64(b.AmbTemp))))
	const uf, ud = 0x1p-24, 0x1p-53
	const tiny = 18 * 0x1p-126
	gf, gd := roundedUp(18*uf/(1-18*uf)), roundedUp(18*ud/(1-18*ud))
	var errorBound float64
	for t := 0; t < b.NumIterations; t++ {
		next := roundedUp(roundedUp(g*magnitude) + source)
		propagated := roundedUp(g * errorBound)
		cpuError := roundedUp(roundedUp(gd*next) + tiny)
		gpuError := roundedUp(gf * roundedUp(next+propagated))
		errorBound = roundedUp(roundedUp(propagated+gpuError) + cpuError)
		magnitude = roundedUp(next + cpuError)
	}
	return errorBound
}

// Verify checks the GPU result against a CPU reference computation.
func (b *Benchmark) verify() error {
	n := b.GridSize
	totalCells := n * n * n

	gpuResult := make([]float32, totalCells)
	if err := benchmarks.Read(b.rt, b.Readbacks[0], gpuResult); err != nil {
		return err
	}

	// CPU reference: ping-pong the stencil for NumIterations steps.
	bufA := make([]float64, totalCells)
	bufB := make([]float64, totalCells)
	for index, initial := range b.TempInit {
		bufA[index] = float64(initial)
	}
	src, dst := bufA, bufB
	for k := 0; k < b.NumIterations; k++ {
		b.cpuStep(src, dst)
		src, dst = dst, src
	}
	// src now holds the final reference result.

	bound := b.errorBound()
	for i := 0; i < totalCells; i++ {
		ref := src[i]
		got := float64(gpuResult[i])

		if math.IsNaN(got) || math.IsInf(got, 0) || math.Abs(ref-got) > bound {
			return fmt.Errorf("At cell %d, expected %f, but got %f.\n",
				i, ref, got)
		}
	}

	log.Printf("Passed!\n")

	return nil
}

func (b *Benchmark) enqueueVerification() {
	b.Readbacks = append(
		b.Readbacks,
		benchmarks.Download(b.rt, b.Queue, b.GTemp[b.FinalBuf], 4*((b.GridSize)*(b.GridSize)*(b.GridSize))),
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
