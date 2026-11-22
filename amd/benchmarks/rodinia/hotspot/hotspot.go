// Package hotspot implements the Rodinia Hotspot benchmark, ported from
// sarchlab/gpu_benchmarks (tier2/rodinia_hotspot) for the MGPUSim MI300X
// (CDNA3 / gfx942) model.
//
// Hotspot is an iterative 2D stencil thermal simulation: each grid cell's
// temperature is updated from its four neighbors (N/S/E/W), the local power
// density, and thermal resistances. The kernel is launched NumIterations
// times with ping-pong source/destination buffers. The kernel binary is
// compiled for gfx942 only (see native/), so the benchmark must be run with
// `-arch cdna3` (the MI300X configuration).
package hotspot

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

// blockSize is the (constant) thread-block dimension baked into the kernel.
const blockSize = 16

// Rodinia thermal constants.
const (
	ambTemp    = 80.0
	chipHeight = 0.016
	chipWidth  = 0.016
	tChip      = 0.0005
	kSi        = 100.0
	cSi        = 1.75e6
)

// KernelArgs describes the shared-memory pyramidal HotSpot kernel.
type KernelArgs struct {
	Iterations                         int32
	_                                  uint32
	Power, TempSrc, TempDst            gpuruntime.Ptr
	Cols, Rows, BorderCols, BorderRows int32
	Cap, RX, RY, RZ, DT, TimeElapsed   float32
}

// Benchmark defines the Hotspot benchmark.
type Benchmark struct {
	*State
	benchmarks.Checkpoint[State]
	rt *gpuruntime.Runtime

	hsaco *insts.KernelCodeObject
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new Hotspot benchmark.
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
		cdna3HSACOBytes, "calculate_temp")
	if b.hsaco == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// SelectGPU selects the GPUs to run on. Hotspot uses a single GPU.
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
		log.Panic("the rodinia hotspot benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300x")
	}

	b.loadProgram()

	b.rt.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.rt.CreateCommandQueue(b.Context)
	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	if b.GridSize <= 0 {
		b.GridSize = 32
	}
	if b.NumIterations <= 0 {
		b.NumIterations = 10
	}

	n := b.GridSize
	total := n * n

	b.Temp = make([]float32, total)
	b.Power = make([]float32, total)
	rand := benchmarks.NewGlibcRand(42)
	for i := range b.Temp {
		b.Temp[i] = float32(ambTemp) + float32(rand.Next()%200)/10.0
		b.Power[i] = float32(rand.Next()%100) / 500.0
	}

	if b.UseUnifiedMemory {
		b.GTempA = b.rt.AllocateUnifiedMemory(b.Context, uint64(total*4))
		b.GTempB = b.rt.AllocateUnifiedMemory(b.Context, uint64(total*4))
		b.GPower = b.rt.AllocateUnifiedMemory(b.Context, uint64(total*4))
	} else {
		b.GTempA = b.rt.AllocateMemory(b.Context, uint64(total*4))
		b.GTempB = b.rt.AllocateMemory(b.Context, uint64(total*4))
		b.GPower = b.rt.AllocateMemory(b.Context, uint64(total*4))
	}

	b.Upload(b.rt, b.Queue, b.GTempA, b.Temp)
	b.Upload(b.rt, b.Queue, b.GPower, b.Power)
}

// thermalParams computes the derived thermal scalars for the current grid.
func up(value float64) float64 {
	return math.Nextafter(value, math.Inf(1))
}

type thermalCoefficients struct {
	Cap, RX, RY, RZ, DT float32
	SDC, IX, IY, IZ     float64
}

func (c thermalCoefficients) rowNorm() float64 {
	x, y, a := c.SDC*c.IX, c.SDC*c.IY, c.SDC*c.IZ
	sum := 2*x + 2*y + a
	const eps = 0x1p-52
	return up(up(math.Abs(1-sum)+up(2*x+2*y)) + up(8*eps*up(1+sum)))
}

func (b *Benchmark) thermalParams() thermalCoefficients {
	const thickness float32 = 0.0005
	dx := float32(0.016) / float32(b.GridSize)
	dy := float32(0.016) / float32(b.GridSize)
	slope := float32(3e6 / (0.5 * float64(thickness) * 1.75e6))
	c := thermalCoefficients{
		Cap: float32(0.5 * 1.75e6 * float64(thickness) * float64(dx) * float64(dy)),
		RX:  float32(float64(dx) / (2 * 100 * float64(thickness) * float64(dy))),
		RY:  float32(float64(dy) / (2 * 100 * float64(thickness) * float64(dx))),
		RZ:  thickness / (100 * dy * dx), DT: float32(0.001 / float64(slope)),
	}
	weights := func() {
		c.SDC = float64(c.DT / c.Cap)
		c.IX, c.IY, c.IZ = float64(1/c.RX), float64(1/c.RY), float64(1/c.RZ)
	}
	weights()
	if c.rowNorm() > 1 {
		c.DT = float32(0.9 * float64(c.Cap) / (2/float64(c.RX) + 2/float64(c.RY) + 1/float64(c.RZ)))
		weights()
	}
	if c.rowNorm() > 1 || math.IsNaN(c.rowNorm()) {
		panic("HotSpot stencil coefficients amplify the input")
	}
	return c
}

func (b *Benchmark) exec() {
	height := b.PyramidHeight
	if height == 0 {
		height = 1
	}
	if height < 1 || height >= blockSize/2 {
		panic("HotSpot pyramid height must be between 1 and 7")
	}
	n := b.GridSize
	covered := blockSize - 2*height
	global := uint32((n + covered - 1) / covered * blockSize)
	grid, group := [3]uint32{global, global, 1}, [3]uint16{blockSize, blockSize, 1}
	c := b.thermalParams()
	src, dst := b.GTempA, b.GTempB
	for done := 0; done < b.NumIterations; {
		steps := min(height, b.NumIterations-done)
		args := KernelArgs{Iterations: int32(steps), Power: b.GPower, TempSrc: src, TempDst: dst,
			Cols: int32(n), Rows: int32(n), BorderCols: int32(height), BorderRows: int32(height),
			Cap: c.Cap, RX: c.RX, RY: c.RY, RZ: c.RZ, DT: c.DT, TimeElapsed: 0.001}
		b.rt.EnqueueLaunchKernel(b.Queue, b.hsaco, grid, group, &args)
		src, dst = dst, src
		done += steps
	}
	b.FinalSrc = src
}

func (b *Benchmark) errorBound(c thermalCoefficients) float64 {
	var magnitude, power float64
	for i, t := range b.Temp {
		magnitude = max(magnitude, math.Abs(float64(t)))
		power = max(power, math.Abs(float64(b.Power[i])))
	}
	g := c.rowNorm()
	h := up(1 + up(c.SDC*up(up(4*c.IX+4*c.IY)+c.IZ)))
	source := up(c.SDC * up(power+up(c.IZ*80)))
	const uf, ud = 0x1p-24, 0x1p-53
	gamma2, gd := up(2*uf/(1-2*uf)), up(24*ud/(1-24*ud))
	tiny := up(64 * 0x1p-126 * up(1+up(c.SDC*up(1+up(c.IX+up(c.IY+c.IZ))))))
	var bound float64
	for t := 0; t < b.NumIterations; t++ {
		gpuMagnitude := up(magnitude + bound)
		next := up(up(g*magnitude) + source)
		cpuRound := up(gd * up(up(h*magnitude)+source))
		pairs := up(2 * uf * up(up(c.IX+c.IY)*gpuMagnitude))
		ambient := up(gamma2 * up(c.IZ*up(80+gpuMagnitude)))
		inner := up(c.SDC * up(pairs+ambient))
		doubleRound := up(gd * up(up(up(h*gpuMagnitude)+source)+inner))
		beforeStore := up(up(up(g*gpuMagnitude)+source) + up(inner+doubleRound))
		gpuRound := up(up(inner+doubleRound) + up(uf*beforeStore))
		bound = up(up(g*bound) + up(gpuRound+up(cpuRound+tiny)))
		magnitude = up(next + up(cpuRound+tiny))
	}
	return bound
}

func (b *Benchmark) verify() error {
	n := b.GridSize
	gpu := make([]float32, n*n)
	if err := benchmarks.Read(b.rt, b.Readbacks[0], gpu); err != nil {
		return err
	}
	c := b.thermalParams()
	cur, next := make([]float64, n*n), make([]float64, n*n)
	for i, t := range b.Temp {
		cur[i] = float64(t)
	}
	for i := 0; i < b.NumIterations; i++ {
		for idx, center := range cur {
			y, x := idx/n, idx%n
			north, south := cur[max(y-1, 0)*n+x], cur[min(y+1, n-1)*n+x]
			west, east := cur[y*n+max(x-1, 0)], cur[y*n+min(x+1, n-1)]
			next[idx] = center + c.SDC*(float64(b.Power[idx])+(south+north-2*center)*c.IY+
				(east+west-2*center)*c.IX+(80-center)*c.IZ)
		}
		cur, next = next, cur
	}
	bound := b.errorBound(c)
	for i, reference := range cur {
		actual := float64(gpu[i])
		if math.IsNaN(actual) || math.IsInf(actual, 0) || math.Abs(reference-actual) > bound {
			return fmt.Errorf("temperature at (%d,%d): got %g, expected %g (bound %g)", i/n, i%n, actual, reference, bound)
		}
	}
	log.Printf("Passed!\n")
	return nil
}

func (b *Benchmark) enqueueVerification() {
	b.Readbacks = append(b.Readbacks, benchmarks.Download(b.rt, b.Queue, b.FinalSrc, 4*((b.GridSize)*(b.GridSize))))
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
