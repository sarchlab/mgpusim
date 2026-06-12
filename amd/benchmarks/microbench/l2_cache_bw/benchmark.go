// Package l2cachebw implements L2 cache bandwidth microbenchmark.
package l2cachebw

import (
	"log"

	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// KernelArgs defines kernel arguments for l2_cache_bw_kernel.
type KernelArgs struct {
	A                   driver.Ptr
	B                   driver.Ptr
	N                   int32
	BufSize             int32
	NumRepeats          int32
	Pad0                int32
	HiddenBlockCountX   uint32
	HiddenBlockCountY   uint32
	HiddenBlockCountZ   uint32
	HiddenGroupSizeX    uint16
	HiddenGroupSizeY    uint16
	HiddenGroupSizeZ    uint16
	HiddenRemainderX    uint16
	HiddenRemainderY    uint16
	HiddenRemainderZ    uint16
	Pad1                [16]byte
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
	HiddenGridDims      uint16
}

const bytesPerRead = 4

// Benchmark defines the L2 cache bandwidth benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	kernel *insts.KernelCodeObject

	NumElements int
	BufSize     int
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

// NewBenchmark creates a new L2 cache bandwidth benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.NumElements = 1048576
	b.BufSize = 65536 // 256KB buffer (fits in L2)
	b.NumRepeats = 128
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

// SetWorkingSetSizeBytes sets the fixed L2-resident footprint in bytes.
func (b *Benchmark) SetWorkingSetSizeBytes(bytes int) {
	elems := (bytes + bytesPerRead - 1) / bytesPerRead
	if elems < 1 {
		elems = 1
	}
	b.BufSize = elems
}

// TotalReadBytes returns the kernel's requested read traffic in bytes.
//
// Formula: total_read_bytes = NumElements * NumRepeats * 4. In workload
// metadata, the same repeat count is named num_iterations.
func (b *Benchmark) TotalReadBytes() uint64 {
	return uint64(b.NumElements) * uint64(b.NumRepeats) * bytesPerRead
}

func (b *Benchmark) loadProgram() {
	b.kernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "l2_cache_bw_kernel")
	if b.kernel == nil {
		log.Panic("Failed to load l2_cache_bw_kernel")
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

	b.hA = make([]float32, b.BufSize)
	b.hB = make([]float32, n)

	for i := 0; i < b.BufSize; i++ {
		b.hA[i] = float32(i%1000) * 0.001
	}

	aBuf := uint64(b.BufSize) * bytesPerRead
	bBuf := uint64(n) * bytesPerRead

	if b.useUnifiedMemory {
		b.dA = b.driver.AllocateUnifiedMemory(b.context, aBuf)
		b.dB = b.driver.AllocateUnifiedMemory(b.context, bBuf)
	} else {
		b.dA = b.driver.AllocateMemory(b.context, aBuf)
		b.dB = b.driver.AllocateMemory(b.context, bBuf)
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
		BufSize:           int32(b.BufSize),
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
		sum := float32(0)
		for r := 0; r < b.NumRepeats; r++ {
			idx := (i + r*64) % b.BufSize
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
		log.Panicf("FAIL: %d errors in l2_cache_bw verification", errors)
	}
	log.Printf("Passed!")
}
