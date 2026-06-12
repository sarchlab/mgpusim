// Package tiledgemm16 implements tiled GEMM with 16x16 tiles microbenchmark.
package tiledgemm16

import (
	"log"
	"math"

	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// KernelArgs defines kernel arguments for tiled_gemm_16_kernel.
type KernelArgs struct {
	A    driver.Ptr
	B    driver.Ptr
	C    driver.Ptr
	M    int32
	N    int32
	K    int32
	Pad0 int32
}

// Benchmark defines the tiled GEMM 16 benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	kernel *insts.KernelCodeObject

	M int
	N int
	K int

	useUnifiedMemory bool

	hA []float32
	hB []float32
	hC []float32

	dA driver.Ptr
	dB driver.Ptr
	dC driver.Ptr
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

// NewBenchmark creates a new tiled GEMM 16 benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.M = 1024
	b.N = 768
	b.K = 768
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

// SetM sets M dimension.
func (b *Benchmark) SetM(v int) {
	b.M = v
}

// SetN sets N dimension.
func (b *Benchmark) SetN(v int) {
	b.N = v
}

// SetK sets K dimension.
func (b *Benchmark) SetK(v int) {
	b.K = v
}

func (b *Benchmark) loadProgram() {
	b.kernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "tiled_gemm_16_kernel")
	if b.kernel == nil {
		log.Panic("Failed to load tiled_gemm_16_kernel")
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
	b.hA = make([]float32, b.M*b.K)
	b.hB = make([]float32, b.K*b.N)
	b.hC = make([]float32, b.M*b.N)

	for i := 0; i < b.M*b.K; i++ {
		b.hA[i] = float32(i%1000)*0.002 - 1.0
	}
	for i := 0; i < b.K*b.N; i++ {
		b.hB[i] = float32((i+500)%1000)*0.002 - 1.0
	}

	bytesA := uint64(b.M * b.K * 4)
	bytesB := uint64(b.K * b.N * 4)
	bytesC := uint64(b.M * b.N * 4)

	if b.useUnifiedMemory {
		b.dA = b.driver.AllocateUnifiedMemory(b.context, bytesA)
		b.dB = b.driver.AllocateUnifiedMemory(b.context, bytesB)
		b.dC = b.driver.AllocateUnifiedMemory(b.context, bytesC)
	} else {
		b.dA = b.driver.AllocateMemory(b.context, bytesA)
		b.dB = b.driver.AllocateMemory(b.context, bytesB)
		b.dC = b.driver.AllocateMemory(b.context, bytesC)
	}

	b.driver.MemCopyH2D(b.context, b.dA, b.hA)
	b.driver.MemCopyH2D(b.context, b.dB, b.hB)
}

func (b *Benchmark) exec() {
	tileSize := 16
	gridX := (b.N + tileSize - 1) / tileSize
	gridY := (b.M + tileSize - 1) / tileSize

	// 2D grid of 16x16 workgroups
	globalSize := [3]uint32{
		uint32(gridX * tileSize),
		uint32(gridY * tileSize),
		1,
	}
	localSize := [3]uint16{uint16(tileSize), uint16(tileSize), 1}

	args := KernelArgs{
		A: b.dA,
		B: b.dB,
		C: b.dC,
		M: int32(b.M),
		N: int32(b.N),
		K: int32(b.K),
	}
	b.driver.LaunchKernel(b.context, b.kernel,
		globalSize, localSize, &args)

	b.driver.MemCopyD2H(b.context, b.hC, b.dC)
}

// Verify verifies the result.
func (b *Benchmark) Verify() {
	errors := 0

	for r := 0; r < b.M; r++ {
		for c := 0; c < b.N; c++ {
			expected := 0.0
			for k := 0; k < b.K; k++ {
				expected += float64(b.hA[r*b.K+k]) * float64(b.hB[k*b.N+c])
			}
			exp := float32(expected)
			got := b.hC[r*b.N+c]
			diff := math.Abs(float64(got - exp))
			tol := 1e-2*math.Abs(float64(exp)) + 1e-4

			if diff > tol {
				if errors < 10 {
					log.Printf("Mismatch at [%d,%d]: got %f, expected %f",
						r, c, got, exp)
				}
				errors++
			}
		}
	}

	if errors > 0 {
		log.Panicf("FAIL: %d errors in tiled_gemm_16 verification", errors)
	}
	log.Printf("Passed!")
}
