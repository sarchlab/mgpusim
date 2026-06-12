// Package sgemm implements the Parboil SGEMM benchmark.
// Computes C = alpha*A*B + beta*C using a tiled SGEMM kernel.
package sgemm

import (
	"log"
	"math"
	"math/rand"

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
	N                   int32
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

// Benchmark defines the Parboil SGEMM benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	sgemmKernel *insts.KernelCodeObject

	N         int
	TileWidth int
	Alpha     float32
	Beta      float32

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

// NewBenchmark creates a new Parboil SGEMM benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.N = 512
	b.TileWidth = 16
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
	b.sgemmKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "sgemm_tiled")
	if b.sgemmKernel == nil {
		log.Panic("Failed to load sgemm_tiled kernel binary")
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
	size := b.N * b.N

	b.hA = make([]float32, size)
	b.hB = make([]float32, size)
	b.hC = make([]float32, size)
	b.hRef = make([]float32, size)

	rand.Seed(42)
	for i := 0; i < size; i++ {
		b.hA[i] = rand.Float32()
		b.hB[i] = rand.Float32()
		b.hC[i] = 0.0
		b.hRef[i] = 0.0
	}
}

func (b *Benchmark) allocGPUMem() {
	bytes := uint64(b.N*b.N) * 4

	if b.useUnifiedMemory {
		b.dA = b.driver.AllocateUnifiedMemory(b.context, bytes)
		b.dB = b.driver.AllocateUnifiedMemory(b.context, bytes)
		b.dC = b.driver.AllocateUnifiedMemory(b.context, bytes)
	} else {
		b.dA = b.driver.AllocateMemory(b.context, bytes)
		b.dB = b.driver.AllocateMemory(b.context, bytes)
		b.dC = b.driver.AllocateMemory(b.context, bytes)
	}
}

func (b *Benchmark) exec() {
	gridX := (b.N + b.TileWidth - 1) / b.TileWidth
	gridY := (b.N + b.TileWidth - 1) / b.TileWidth

	globalSize := [3]uint32{
		uint32(gridX * b.TileWidth),
		uint32(gridY * b.TileWidth),
		1,
	}
	localSize := [3]uint16{
		uint16(b.TileWidth),
		uint16(b.TileWidth),
		1,
	}

	b.launch(globalSize, localSize)

	b.driver.MemCopyD2H(b.context, b.hC, b.dC)
}

func (b *Benchmark) launch(
	globalSize [3]uint32, localSize [3]uint16,
) {
	args := KernelArgs{
		A:                 b.dA,
		B:                 b.dB,
		C:                 b.dC,
		N:                 int32(b.N),
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
		b.sgemmKernel,
		globalSize, localSize,
		&args,
	)
}

// Verify verifies the result against CPU computation.
func (b *Benchmark) Verify() {
	b.cpuSGEMM()
	b.checkResults()
}

func (b *Benchmark) cpuSGEMM() {
	for i := 0; i < b.N; i++ {
		for j := 0; j < b.N; j++ {
			var sum float32
			for k := 0; k < b.N; k++ {
				sum += b.hA[i*b.N+k] * b.hB[k*b.N+j]
			}
			idx := i*b.N + j
			b.hRef[idx] = b.Alpha*sum + b.Beta*b.hRef[idx]
		}
	}
}

func (b *Benchmark) checkResults() {
	errors := 0
	total := b.N * b.N

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
		log.Panicf("FAIL: %d errors in SGEMM verification", errors)
	}

	log.Printf("Passed!")
}
