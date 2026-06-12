// Package memlatencychase implements memory latency pointer chase microbenchmark.
package memlatencychase

import (
	"log"
	"math/rand"

	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// KernelArgs defines kernel arguments for mem_latency_chase_kernel.
type KernelArgs struct {
	Indices             driver.Ptr
	Result              driver.Ptr
	N                   int32
	NumChases           int32
	HiddenBlockCountX   uint32
	HiddenBlockCountY   uint32
	HiddenBlockCountZ   uint32
	HiddenGroupSizeX    uint16
	HiddenGroupSizeY    uint16
	HiddenGroupSizeZ    uint16
	HiddenRemainderX    uint16
	HiddenRemainderY    uint16
	HiddenRemainderZ    uint16
	Pad0                [16]byte
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
	HiddenGridDims      uint16
}

const (
	// A 32 KiB int array is resident in the modeled L1V cache. Larger chase
	// arrays use a single random chain so the working set exercises L2 latency.
	l1ResidentChaseElements = 8192
	randomChaseBlockSize    = 64
	// The hardware HIP path defaults to --num_hops=1024. Use the same hop
	// count in the simulator's L2-region random chase mode.
	hardwareDefaultNumHops = 1024
)

// Benchmark defines the memory latency chase benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	kernel *insts.KernelCodeObject

	NumElements int
	NumChases   int
	BlockSize   int

	numChasesExplicit bool
	useUnifiedMemory  bool

	hIndices []int32
	hResult  []int32

	dIndices driver.Ptr
	dResult  driver.Ptr
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

// NewBenchmark creates a new memory latency chase benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.NumElements = 1048576
	b.NumChases = hardwareDefaultNumHops
	b.BlockSize = 256
	return b
}

// SelectGPU selects GPU.
func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
}

// SetUnifiedMemory uses unified memory.
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

// SetNumElements sets the number of elements.
func (b *Benchmark) SetNumElements(n int) {
	b.NumElements = n
}

// SetNumChases sets the number of pointer-chase hops.
func (b *Benchmark) SetNumChases(n int) {
	b.NumChases = n
	b.numChasesExplicit = true
}

func (b *Benchmark) loadProgram() {
	b.kernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "mem_latency_chase_kernel")
	if b.kernel == nil {
		log.Panic("Failed to load mem_latency_chase_kernel")
	}
}

// Run runs the benchmark.
func (b *Benchmark) Run() {
	b.loadProgram()

	for _, gpu := range b.gpus {
		b.driver.SelectGPU(b.context, gpu)
		b.queues = append(b.queues,
			b.driver.CreateCommandQueue(b.context))
	}

	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	n := b.NumElements

	b.hIndices = make([]int32, n)
	if b.useRandomChaseMode() {
		b.initRandomChase()
	} else {
		b.initSequentialChase()
	}

	b.hResult = make([]int32, n)

	bytes := uint64(n * 4)

	if b.useUnifiedMemory {
		b.dIndices = b.driver.AllocateUnifiedMemory(b.context, bytes)
		b.dResult = b.driver.AllocateUnifiedMemory(b.context, bytes)
	} else {
		b.dIndices = b.driver.AllocateMemory(b.context, bytes)
		b.dResult = b.driver.AllocateMemory(b.context, bytes)
	}

	b.driver.MemCopyH2D(b.context, b.dIndices, b.hIndices)
}

func (b *Benchmark) useRandomChaseMode() bool {
	return b.NumElements > l1ResidentChaseElements
}

func (b *Benchmark) chaseIterations() int {
	if b.useRandomChaseMode() && !b.numChasesExplicit {
		return hardwareDefaultNumHops
	}
	return b.NumChases
}

func (b *Benchmark) initSequentialChase() {
	for i := 0; i < b.NumElements; i++ {
		b.hIndices[i] = int32((i + 1) % b.NumElements)
	}
}

func (b *Benchmark) initRandomChase() {
	perm := rand.New(rand.NewSource(42)).Perm(b.NumElements)
	for i := 0; i < b.NumElements-1; i++ {
		b.hIndices[perm[i]] = int32(perm[i+1])
	}
	b.hIndices[perm[b.NumElements-1]] = int32(perm[0])
}

func (b *Benchmark) exec() {
	n := b.NumElements
	blockSize := b.BlockSize
	grid := (n + blockSize - 1) / blockSize
	if b.useRandomChaseMode() {
		blockSize = randomChaseBlockSize
		grid = 1
	}

	globalSize := [3]uint32{uint32(grid * blockSize), 1, 1}
	localSize := [3]uint16{uint16(blockSize), 1, 1}

	args := KernelArgs{
		Indices:           b.dIndices,
		Result:            b.dResult,
		N:                 int32(n),
		NumChases:         int32(b.chaseIterations()),
		HiddenBlockCountX: globalSize[0] / uint32(localSize[0]),
		HiddenBlockCountY: 1,
		HiddenBlockCountZ: 1,
		HiddenGroupSizeX:  localSize[0],
		HiddenGroupSizeY:  localSize[1],
		HiddenGroupSizeZ:  localSize[2],
		HiddenRemainderX:  uint16(globalSize[0] % uint32(localSize[0])),
		HiddenRemainderY:  0,
		HiddenRemainderZ:  0,
		HiddenGridDims:    1,
	}
	b.driver.LaunchKernel(b.context, b.kernel,
		globalSize, localSize, &args)

	b.driver.MemCopyD2H(b.context, b.hResult, b.dResult)
}

// Verify verifies the result.
func (b *Benchmark) Verify() {
	n := b.NumElements
	errors := 0

	activeThreads := n
	if b.useRandomChaseMode() {
		activeThreads = randomChaseBlockSize
		if n < activeThreads {
			activeThreads = n
		}
	}

	for i := 0; i < activeThreads; i++ {
		idx := i % n
		for j := 0; j < b.chaseIterations(); j++ {
			idx = int(b.hIndices[idx])
		}
		if b.hResult[i] != int32(idx) {
			if errors < 10 {
				log.Printf("Mismatch at %d: got %d, expected %d",
					i, b.hResult[i], idx)
			}
			errors++
		}
	}

	if errors > 0 {
		log.Panicf("FAIL: %d errors in mem_latency_chase verification", errors)
	}
	log.Printf("Passed!")
}
