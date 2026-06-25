// Package emptykernel implements the empty_kernel launch-overhead microbenchmark,
// ported from sarchlab/gpu_benchmarks (tier1/empty_kernel) for the MGPUSim MI300X
// (CDNA3 / gfx942) model.
//
// The kernel does no work, so the simulated kernel time for a single launch is the
// model's pure launch / dispatch / grid-scheduling / completion overhead --
// directly comparable to the real benchmark's launch_sync_us (the round-trip
// launch+synchronize cost per launch). The kernel binary is compiled for gfx942
// only (see native/), so the benchmark must be run with `-arch cdna3`.
package emptykernel

import (
	"log"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// emptyArgs is the kernel argument struct: one unused pointer, present only so
// the kernarg segment is non-zero (the driver cannot allocate 0 bytes). The
// kernel never dereferences it.
type emptyArgs struct {
	Out driver.Ptr // offset 0
}

// Benchmark defines the empty_kernel benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	queue   *driver.CommandQueue
	hsaco   *insts.KernelCodeObject
	gpus    []int

	Arch arch.Type

	// NumBlocks is the number of work-groups in the (empty) grid; the scaling axis.
	NumBlocks int
	// BlockSize is the number of work-items per work-group.
	BlockSize int

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new empty_kernel benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.context = b.driver.Init()
	b.queue = driver.CreateCommandQueue(b.context)

	return b
}

func (b *Benchmark) loadProgram() {
	b.hsaco = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "empty_kernel")
	if b.hsaco == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// SelectGPU selects the GPUs to run on. empty_kernel uses a single GPU.
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
		log.Panic("the empty_kernel benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300x")
	}

	b.loadProgram()

	b.driver.SelectGPU(b.context, b.gpus[0])
	b.exec()
}

func (b *Benchmark) exec() {
	if b.NumBlocks <= 0 {
		b.NumBlocks = 1
	}
	if b.BlockSize <= 0 {
		b.BlockSize = 256
	}

	// A tiny unused buffer just to give the kernel a valid (non-zero) kernarg.
	var out driver.Ptr
	if b.useUnifiedMemory {
		out = b.driver.AllocateUnifiedMemory(b.context, 4)
	} else {
		out = b.driver.AllocateMemory(b.context, 4)
	}

	globalX := uint32(b.NumBlocks * b.BlockSize)
	args := emptyArgs{Out: out}

	b.driver.EnqueueLaunchKernel(
		b.queue,
		b.hsaco,
		[3]uint32{globalX, 1, 1},
		[3]uint16{uint16(b.BlockSize), 1, 1},
		&args,
	)

	b.driver.DrainCommandQueue(b.queue)
}

// Verify is a no-op: the empty kernel produces no output to check.
func (b *Benchmark) Verify() {
	log.Printf("Passed!\n")
}
