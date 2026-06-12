// Package l1cachebw implements L1 cache bandwidth microbenchmark.
package l1cachebw

import (
	"log"

	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// KernelArgs defines kernel arguments for l1_cache_bw_kernel.
type KernelArgs struct {
	A                   driver.Ptr
	B                   driver.Ptr
	N                   int32
	NumRepeats          int32
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
	bytesPerRead      = 4
	readGroupElements = 256
)

// Benchmark defines the L1 cache bandwidth benchmark.
//
// The embedded simulator kernel does not implement the HIP/CUDA fixed-total
// working_set_size path. It keeps each 256-thread read group on a 256-float
// footprint and allocates one such input group per covered NumElements range.
// NumRepeats is the simulator spelling of the HIP/CUDA num_iterations repeat
// count, so requested read traffic is still NumElements * NumRepeats * 4 bytes.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	kernel *insts.KernelCodeObject

	NumElements int
	NumRepeats  int
	BlockSize   int

	useUnifiedMemory bool

	hA []float32
	hB []float32

	dA driver.Ptr
	dB driver.Ptr
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

// NewBenchmark creates a new L1 cache bandwidth benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.NumElements = 1048576
	b.NumRepeats = 256
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

// SetNumElements sets the selected work-axis problem size.
//
// The simulator L1 input footprint grows as read groups are added:
// ceil(NumElements / 256) * 256 * 4 bytes. This is intentionally documented
// separately from HIP/CUDA's fixed total working_set_size implementation.
// Total read bytes = NumElements * NumRepeats * 4 bytes per float32 read.
func (b *Benchmark) SetNumElements(n int) {
	b.NumElements = n
}

// SetNumRepeats sets repeated float32 reads per output element.
//
// NumRepeats is equivalent to the HIP/CUDA workload's num_iterations parameter.
func (b *Benchmark) SetNumRepeats(n int) {
	b.NumRepeats = n
}

// TotalReadBytes returns the kernel's requested read traffic in bytes.
//
// Formula: total_read_bytes = NumElements * NumRepeats * 4. In workload
// metadata, the same repeat count is named num_iterations.
func (b *Benchmark) TotalReadBytes() uint64 {
	return uint64(b.NumElements) * uint64(b.NumRepeats) * bytesPerRead
}

func (b *Benchmark) inputElements() int {
	return roundUpToReadGroup(b.NumElements)
}

func (b *Benchmark) inputFootprintBytes() uint64 {
	return uint64(b.inputElements()) * bytesPerRead
}

func roundUpToReadGroup(n int) int {
	if n <= 0 {
		return 0
	}
	return ((n + readGroupElements - 1) / readGroupElements) * readGroupElements
}

func (b *Benchmark) loadProgram() {
	b.kernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "l1_cache_bw_kernel")
	if b.kernel == nil {
		log.Panic("Failed to load l1_cache_bw_kernel")
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
	aElems := b.inputElements()

	b.hA = make([]float32, aElems)
	b.hB = make([]float32, n)

	for i := 0; i < aElems; i++ {
		b.hA[i] = float32(i%readGroupElements) * 0.01
	}

	aBytes := b.inputFootprintBytes()
	bBytes := uint64(n) * bytesPerRead

	if b.useUnifiedMemory {
		b.dA = b.driver.AllocateUnifiedMemory(b.context, aBytes)
		b.dB = b.driver.AllocateUnifiedMemory(b.context, bBytes)
	} else {
		b.dA = b.driver.AllocateMemory(b.context, aBytes)
		b.dB = b.driver.AllocateMemory(b.context, bBytes)
	}

	b.driver.MemCopyH2D(b.context, b.dA, b.hA)
}

func (b *Benchmark) exec() {
	n := b.NumElements
	blockSize := b.BlockSize
	grid := (n + blockSize - 1) / blockSize

	globalSize := [3]uint32{uint32(grid * blockSize), 1, 1}
	localSize := [3]uint16{uint16(blockSize), 1, 1}

	args := KernelArgs{
		A:                 b.dA,
		B:                 b.dB,
		N:                 int32(n),
		NumRepeats:        int32(b.NumRepeats),
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

	b.driver.MemCopyD2H(b.context, b.hB, b.dB)
}

// Verify verifies the result.
func (b *Benchmark) Verify() {
	n := b.NumElements
	errors := 0

	for i := 0; i < n; i++ {
		tid := i % readGroupElements
		base := (i / readGroupElements) * readGroupElements
		sum := float32(0)
		for r := 0; r < b.NumRepeats; r++ {
			idx := base + (tid+r)%readGroupElements
			sum += b.hA[idx]
		}
		if b.hB[i] != sum {
			if errors < 10 {
				log.Printf("Mismatch at %d: got %f, expected %f",
					i, b.hB[i], sum)
			}
			errors++
		}
	}

	if errors > 0 {
		log.Panicf("FAIL: %d errors in l1_cache_bw verification", errors)
	}
	log.Printf("Passed!")
}
