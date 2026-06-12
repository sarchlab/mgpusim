// Package gemm implements the SHOC GEMM benchmark.
// Computes C = alpha*A*B + beta*C — dense matrix-matrix multiplication.
package gemm

import (
	"log"
	"math"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// KernelArgs defines kernel arguments.
type KernelArgs struct {
	A                   driver.Ptr
	B                   driver.Ptr
	C                   driver.Ptr
	M                   int32
	N                   int32
	K                   int32
	Alpha               float32
	Beta                float32
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

// Benchmark defines the GEMM benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	gemmKernel *insts.KernelCodeObject

	M          int
	N          int
	K          int
	BlockSizeX int
	BlockSizeY int
	Alpha      float32
	Beta       float32

	useUnifiedMemory bool

	hA   []float32
	hB   []float32
	hC   []float32
	hRef []float32

	dA driver.Ptr
	dB driver.Ptr
	dC driver.Ptr
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

// NewBenchmark creates a new GEMM benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.M = 512
	b.N = 512
	b.K = 512
	b.BlockSizeX = 16
	b.BlockSizeY = 16
	b.Alpha = 1.0
	b.Beta = 0.0
	return b
}

// SelectGPU selects GPU.
func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
}

// SetUnifiedMemory uses Unified Memory.
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

func (b *Benchmark) loadProgram() {
	b.gemmKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "gemm_kernel")
	if b.gemmKernel == nil {
		log.Panic("Failed to load gemm_kernel binary")
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
	b.initHostData()
	b.allocGPUMem()

	b.driver.MemCopyH2D(b.context, b.dA, b.hA)
	b.driver.MemCopyH2D(b.context, b.dB, b.hB)
	b.driver.MemCopyH2D(b.context, b.dC, b.hC)
}

func (b *Benchmark) initHostData() {
	sizeA := b.M * b.K
	sizeB := b.K * b.N
	sizeC := b.M * b.N

	b.hA = make([]float32, sizeA)
	b.hB = make([]float32, sizeB)
	b.hC = make([]float32, sizeC)
	b.hRef = make([]float32, sizeC)

	for i := 0; i < sizeA; i++ {
		b.hA[i] = float32(i%1000) * 0.001
	}

	for i := 0; i < sizeB; i++ {
		b.hB[i] = float32((i+37)%1000) * 0.001
	}

	for i := 0; i < sizeC; i++ {
		b.hC[i] = 0.0
		b.hRef[i] = 0.0
	}
}

func (b *Benchmark) allocGPUMem() {
	bytesA := uint64(b.M*b.K) * 4
	bytesB := uint64(b.K*b.N) * 4
	bytesC := uint64(b.M*b.N) * 4

	if b.useUnifiedMemory {
		b.dA = b.driver.AllocateUnifiedMemory(b.context, bytesA)
		b.dB = b.driver.AllocateUnifiedMemory(b.context, bytesB)
		b.dC = b.driver.AllocateUnifiedMemory(b.context, bytesC)
	} else {
		b.dA = b.driver.AllocateMemory(b.context, bytesA)
		b.dB = b.driver.AllocateMemory(b.context, bytesB)
		b.dC = b.driver.AllocateMemory(b.context, bytesC)
	}
}

func (b *Benchmark) exec() {
	gridX := (b.N + b.BlockSizeX - 1) / b.BlockSizeX
	gridY := (b.M + b.BlockSizeY - 1) / b.BlockSizeY

	globalSize := [3]uint32{
		uint32(gridX * b.BlockSizeX),
		uint32(gridY * b.BlockSizeY),
		1,
	}
	localSize := [3]uint16{
		uint16(b.BlockSizeX),
		uint16(b.BlockSizeY),
		1,
	}

	args := KernelArgs{
		A:                 b.dA,
		B:                 b.dB,
		C:                 b.dC,
		M:                 int32(b.M),
		N:                 int32(b.N),
		K:                 int32(b.K),
		Alpha:             b.Alpha,
		Beta:              b.Beta,
		HiddenBlockCountX: globalSize[0] / uint32(localSize[0]),
		HiddenBlockCountY: globalSize[1] / uint32(localSize[1]),
		HiddenBlockCountZ: 1,
		HiddenGroupSizeX:  localSize[0],
		HiddenGroupSizeY:  localSize[1],
		HiddenGroupSizeZ:  localSize[2],
		HiddenRemainderX:  uint16(globalSize[0] % uint32(localSize[0])),
		HiddenRemainderY:  uint16(globalSize[1] % uint32(localSize[1])),
		HiddenRemainderZ:  0,
		HiddenGridDims:    2,
	}
	b.driver.LaunchKernel(b.context,
		b.gemmKernel,
		globalSize, localSize,
		&args,
	)

	b.driver.MemCopyD2H(b.context, b.hC, b.dC)
}

// Verify verifies the result against CPU computation.
func (b *Benchmark) Verify() {
	b.cpuGEMM()
	b.checkResults()
}

func (b *Benchmark) cpuGEMM() {
	for i := 0; i < b.M; i++ {
		for j := 0; j < b.N; j++ {
			var sum float32

			for k := 0; k < b.K; k++ {
				sum += b.hA[i*b.K+k] * b.hB[k*b.N+j]
			}

			idx := i*b.N + j
			b.hRef[idx] = b.Alpha*sum + b.Beta*b.hRef[idx]
		}
	}
}

func (b *Benchmark) checkResults() {
	errors := 0
	total := b.M * b.N

	for i := 0; i < total; i++ {
		diff := math.Abs(float64(b.hC[i] - b.hRef[i]))
		tol := 1e-3*math.Abs(float64(b.hRef[i])) + 1e-5

		if diff > tol {
			if errors < 10 {
				log.Printf("Mismatch at %d: got %f, expected %f",
					i, b.hC[i], b.hRef[i])
			}
			errors++
		}
	}

	if errors > 0 {
		log.Panicf("FAIL: %d errors in GEMM verification", errors)
	}

	log.Printf("Passed!")
}
