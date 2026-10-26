// Package cachelatency implements the cache_latency microbenchmark, ported
// from sarchlab/gpu_benchmarks (tier1/cache_latency) for the MGPUSim MI300X
// (CDNA3 / gfx942) model.
//
// A single thread walks a linked-list-style index chain where each array
// element holds the index of the next element to visit. Because each load
// depends on the previous result (a true data dependency), the loads cannot
// overlap, directly exposing per-access memory latency rather than bandwidth.
// The thread writes its final index to result[0].
//
// The kernel binary is compiled for gfx942 only (see native/), so the
// benchmark must be run with `-arch cdna3` (the MI300X configuration).
package cachelatency

import (
	// embed hsaco files
	_ "embed"
	"fmt"
	"log"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// KernelArgs defines the kernel arguments for the gfx942 (CDNA3) kernel.
//
// The layout is verified against the compiled kernel's AMDGPU metadata
// (kernarg_segment_size = 24): one 8-byte global_buffer pointer, two 4-byte
// by_value scalars, then another 8-byte global_buffer pointer, packed with
// no padding (mgpusim serializes args with binary.Write, which inserts no
// alignment padding). The kernel reads only blockIdx.x / threadIdx.x in a
// guard, so no hidden ABI arguments are emitted.
type KernelArgs struct {
	Arr         driver.Ptr // offset 0
	StartIdx    uint32     // offset 8
	NumAccesses uint32     // offset 12
	Result      driver.Ptr // offset 16
}

// Benchmark defines the cache_latency benchmark.
type Benchmark struct {
	*State
	benchmarks.Saved[checkpointConfig, State]

	driver *driver.Runtime

	hsaco *insts.KernelCodeObject
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new cache_latency benchmark.
func NewBenchmark(driver *driver.Runtime) *Benchmark {
	b := new(Benchmark)
	b.State = &State{}
	b.Saved = benchmarks.NewSaved(b.State,
		b.checkpointConfig, b.bind)

	b.driver = driver
	b.Context = b.driver.Init()

	return b
}

func (b *Benchmark) loadProgram() {
	b.hsaco = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "pointer_chase_kernel")
	if b.hsaco == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// SelectGPU selects the GPUs to run on. cache_latency uses a single GPU.
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
		log.Panic("the cache_latency benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300x")
	}

	b.loadProgram()

	b.driver.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.driver.CreateCommandQueue(b.Context)
	b.initMem()
	b.exec()
}

// Timed-access bounds, matching the real benchmark: a floor so tiny resident
// sizes still get enough laps (and the first cold lap is amortized), and a
// ceiling so the largest sizes stay bounded.
const (
	minTimedAccesses = 131072
	maxTimedAccesses = 16000000
)

// buildChain builds a random Hamiltonian cycle over CACHELINE-strided nodes,
// matching the real benchmark: only the first word of each cacheline is a chase
// node (element indices {0, stride, 2*stride, ...}), so consecutive accesses
// land on distinct cachelines (no intra-line spatial reuse) while the touched
// lines still span the whole array (cache footprint == ArrayBytes). Non-node
// slots stay zero; the chase only ever visits node slots.
//
// The permutation uses a deterministic Fisher-Yates shuffle driven by a
// splitmix-style PRNG so the cycle is exactly reproducible in Verify.
func (b *Benchmark) buildChain() {
	m, s := b.M, uint32(b.Stride)

	perm := make([]uint32, m)
	for i := 0; i < m; i++ {
		perm[i] = uint32(i)
	}

	state := uint64(b.Seed)*2654435761 + 0x9E3779B97F4A7C15
	next := func() uint64 {
		state += 0x9E3779B97F4A7C15
		z := state
		z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
		z = (z ^ (z >> 27)) * 0x94D049BB133111EB
		return z ^ (z >> 31)
	}

	// Fisher-Yates over the m node ordinals.
	for i := m - 1; i > 0; i-- {
		j := int(next() % uint64(i+1))
		perm[i], perm[j] = perm[j], perm[i]
	}

	b.Chain = make([]uint32, b.N) // non-node slots stay 0
	for i := 0; i < m; i++ {
		node := perm[i] * s
		nextNode := perm[(i+1)%m] * s
		b.Chain[node] = nextNode
	}
	b.StartIdx = perm[0] * s
}

func (b *Benchmark) initMem() {
	if b.ArrayBytes <= 0 {
		b.ArrayBytes = 16 * 1024 // 16 KB
	}
	if b.CachelineBytes <= 0 {
		b.CachelineBytes = 64 // gfx942/MI300X cacheline
	}
	if b.MeasureLaps <= 0 {
		b.MeasureLaps = 4
	}
	if b.Seed == 0 {
		b.Seed = 42
	}

	b.N = b.ArrayBytes / 4
	b.Stride = b.CachelineBytes / 4
	if b.Stride < 1 {
		b.Stride = 1
	}
	// Need at least two cachelines to form a cycle.
	if b.N < b.Stride*2 {
		b.N = b.Stride * 2
		b.ArrayBytes = b.N * 4
	}
	b.M = b.N / b.Stride // cacheline nodes == touched lines

	// Timed accesses: derive from MeasureLaps like the real benchmark unless an
	// explicit NumAccesses was given. The floor gives tiny resident sizes many
	// laps, so the (necessarily counted) first cold lap is amortized -- the sim
	// records cumulative GPU busy time and cannot run a separate untimed warmup
	// the way the real benchmark does.
	if b.NumAccesses <= 0 {
		na := b.MeasureLaps * b.M
		if na < minTimedAccesses {
			na = minTimedAccesses
		}
		if na > maxTimedAccesses {
			na = maxTimedAccesses
		}
		b.NumAccesses = na
	}

	b.buildChain()

	if b.UseUnifiedMemory {
		b.GArr = b.driver.AllocateUnifiedMemory(
			b.Context, uint64(b.N*4))
		b.GResult = b.driver.AllocateUnifiedMemory(
			b.Context, uint64(4))
	} else {
		b.GArr = b.driver.AllocateMemory(b.Context, uint64(b.N*4))
		b.GResult = b.driver.AllocateMemory(b.Context, uint64(4))
	}

	b.driver.EnqueueMemCopyH2D(b.Queue, b.GArr, b.Chain)
	// Initialize the result buffer so its page is resident on the GPU before
	// the kernel writes to it (output-only buffers are otherwise never mapped).
	b.driver.EnqueueMemCopyH2D(b.Queue, b.GResult, []uint32{0})
}

func (b *Benchmark) exec() {
	args := KernelArgs{
		Arr:         b.GArr,
		StartIdx:    b.StartIdx,
		NumAccesses: uint32(b.NumAccesses),
		Result:      b.GResult,
	}

	// Launch a single work-item (grid 1, block 1), matching the HIP source.
	// The kernel guards on blockIdx.x == 0 && threadIdx.x == 0, so only one
	// thread performs the dependent pointer chase and writes result[0].
	b.driver.EnqueueLaunchKernel(
		b.Queue,
		b.hsaco,
		[3]uint32{1, 1, 1},
		[3]uint16{1, 1, 1},
		&args,
	)
}

// Verify checks the GPU result against a CPU reference walk of the same chain.
// The thread starts at startIdx and applies num_accesses chain hops.
func (b *Benchmark) verify() error {
	gpuResult := make([]uint32, 1)
	if err := b.driver.ReadResult(b.Readbacks[0], gpuResult); err != nil {
		return err
	}

	idx := b.StartIdx
	for i := 0; i < b.NumAccesses; i++ {
		idx = b.Chain[idx]
	}

	if gpuResult[0] != idx {
		return fmt.Errorf("Mismatch: expected final index %d, but got %d.\n",
			idx, gpuResult[0])
	}

	log.Printf("Passed!\n")

	return nil
}

func (b *Benchmark) enqueueVerification() {
	b.Readbacks = append(b.Readbacks, b.driver.EnqueueMemCopyD2H(b.Queue, b.GResult, 4*(1)))
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
