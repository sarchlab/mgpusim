// Package cachelatency implements the cache_latency microbenchmark, ported
// from sarchlab/gpu_benchmarks (tier1/cache_latency) for the MGPUSim MI300A
// (CDNA3 / gfx942) model.
//
// A single thread walks a linked-list-style index chain where each array
// element holds the index of the next element to visit. Because each load
// depends on the previous result (a true data dependency), the loads cannot
// overlap, directly exposing per-access memory latency rather than bandwidth.
// The thread writes its final index to result[0].
//
// The kernel binary is compiled for gfx942 only (see native/), so the
// benchmark must be run with `-arch cdna3` (the MI300A configuration).
package cachelatency

import (
	"log"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
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
	driver  *driver.Driver
	context *driver.Context
	queue   *driver.CommandQueue
	hsaco   *insts.KernelCodeObject
	gpus    []int

	Arch arch.Type

	// ArrayBytes is the size of the pointer-chasing array in bytes.
	ArrayBytes int
	// NumAccesses is the number of dependent loads the single thread performs.
	NumAccesses int
	// Seed controls the deterministic chain permutation.
	Seed uint32

	n        int
	startIdx uint32
	chain    []uint32

	gArr    driver.Ptr
	gResult driver.Ptr

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new cache_latency benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.context = b.driver.Init()
	b.queue = driver.CreateCommandQueue(b.context)

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
	b.gpus = gpus
}

// SetUnifiedMemory requests the use of unified memory.
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

// Run runs the benchmark.
func (b *Benchmark) Run() {
	if b.Arch != arch.CDNA3 {
		log.Panic("the cache_latency benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300a")
	}

	b.loadProgram()

	b.driver.SelectGPU(b.context, b.gpus[0])
	b.initMem()
	b.exec()
}

// buildChain builds a single random Hamiltonian pointer-chasing cycle.
//
// A permutation is produced by a deterministic Fisher-Yates shuffle driven by
// a splitmix-style PRNG (so the result is exactly reproducible in Verify),
// then each element points to the successor of its position in the
// permutation, forming one big cycle.
func (b *Benchmark) buildChain() {
	n := b.n

	perm := make([]uint32, n)
	for i := 0; i < n; i++ {
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

	// Fisher-Yates: for i from n-1 down to 1, swap perm[i] with perm[j].
	for i := n - 1; i > 0; i-- {
		j := int(next() % uint64(i+1))
		perm[i], perm[j] = perm[j], perm[i]
	}

	b.chain = make([]uint32, n)
	for i := 0; i < n; i++ {
		b.chain[perm[i]] = perm[(i+1)%n]
	}
	b.startIdx = perm[0]
}

func (b *Benchmark) initMem() {
	if b.ArrayBytes <= 0 {
		b.ArrayBytes = 16 * 1024 // 16 KB -> 4096 elements
	}
	if b.NumAccesses <= 0 {
		b.NumAccesses = 1024
	}
	if b.Seed == 0 {
		b.Seed = 42
	}

	b.n = b.ArrayBytes / 4
	if b.n < 1 {
		b.n = 1
		b.ArrayBytes = 4
	}

	b.buildChain()

	if b.useUnifiedMemory {
		b.gArr = b.driver.AllocateUnifiedMemory(
			b.context, uint64(b.n*4))
		b.gResult = b.driver.AllocateUnifiedMemory(
			b.context, uint64(4))
	} else {
		b.gArr = b.driver.AllocateMemory(b.context, uint64(b.n*4))
		b.gResult = b.driver.AllocateMemory(b.context, uint64(4))
	}

	b.driver.MemCopyH2D(b.context, b.gArr, b.chain)
	// Initialize the result buffer so its page is resident on the GPU before
	// the kernel writes to it (output-only buffers are otherwise never mapped).
	b.driver.MemCopyH2D(b.context, b.gResult, []uint32{0})
}

func (b *Benchmark) exec() {
	args := KernelArgs{
		Arr:         b.gArr,
		StartIdx:    b.startIdx,
		NumAccesses: uint32(b.NumAccesses),
		Result:      b.gResult,
	}

	// Launch a single work-item (grid 1, block 1), matching the HIP source.
	// The kernel guards on blockIdx.x == 0 && threadIdx.x == 0, so only one
	// thread performs the dependent pointer chase and writes result[0].
	b.driver.EnqueueLaunchKernel(
		b.queue,
		b.hsaco,
		[3]uint32{1, 1, 1},
		[3]uint16{1, 1, 1},
		&args,
	)

	b.driver.DrainCommandQueue(b.queue)
}

// Verify checks the GPU result against a CPU reference walk of the same chain.
// The thread starts at startIdx and applies num_accesses chain hops.
func (b *Benchmark) Verify() {
	gpuResult := make([]uint32, 1)
	b.driver.MemCopyD2H(b.context, gpuResult, b.gResult)

	idx := b.startIdx
	for i := 0; i < b.NumAccesses; i++ {
		idx = b.chain[idx]
	}

	if gpuResult[0] != idx {
		log.Fatalf("Mismatch: expected final index %d, but got %d.\n",
			idx, gpuResult[0])
	}

	log.Printf("Passed!\n")
}
