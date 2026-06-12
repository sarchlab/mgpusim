// Package layernorm implements layer normalization microbenchmark.
package layernorm

import (
	"log"
	"math"

	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// KernelArgs defines kernel arguments for layernorm_kernel.
type KernelArgs struct {
	A     driver.Ptr
	B     driver.Ptr
	Gamma driver.Ptr
	Beta  driver.Ptr
	Rows  int32
	Hid   int32
}

// Benchmark defines the layernorm benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	kernel *insts.KernelCodeObject

	Rows      int
	Hid       int
	BlockSize int

	useUnifiedMemory bool

	hA     []float32
	hB     []float32
	hGamma []float32
	hBeta  []float32

	dA     driver.Ptr
	dB     driver.Ptr
	dGamma driver.Ptr
	dBeta  driver.Ptr
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

// NewBenchmark creates a new layernorm benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.Rows = 1024
	b.Hid = 4096
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

// SetRows sets number of rows.
func (b *Benchmark) SetRows(r int) {
	b.Rows = r
}

// SetHid sets hidden dimension size.
func (b *Benchmark) SetHid(h int) {
	b.Hid = h
}

func (b *Benchmark) loadProgram() {
	b.kernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "layernorm_kernel")
	if b.kernel == nil {
		log.Panic("Failed to load layernorm_kernel")
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
	n := b.Rows * b.Hid

	b.hA = make([]float32, n)
	b.hB = make([]float32, n)
	b.hGamma = make([]float32, b.Hid)
	b.hBeta = make([]float32, b.Hid)

	for i := 0; i < n; i++ {
		b.hA[i] = float32(i%1000)*0.002 - 1.0
	}
	for i := 0; i < b.Hid; i++ {
		b.hGamma[i] = 1.0
		b.hBeta[i] = 0.0
	}

	matBytes := uint64(n * 4)
	vecBytes := uint64(b.Hid * 4)

	if b.useUnifiedMemory {
		b.dA = b.driver.AllocateUnifiedMemory(b.context, matBytes)
		b.dB = b.driver.AllocateUnifiedMemory(b.context, matBytes)
		b.dGamma = b.driver.AllocateUnifiedMemory(b.context, vecBytes)
		b.dBeta = b.driver.AllocateUnifiedMemory(b.context, vecBytes)
	} else {
		b.dA = b.driver.AllocateMemory(b.context, matBytes)
		b.dB = b.driver.AllocateMemory(b.context, matBytes)
		b.dGamma = b.driver.AllocateMemory(b.context, vecBytes)
		b.dBeta = b.driver.AllocateMemory(b.context, vecBytes)
	}

	b.driver.MemCopyH2D(b.context, b.dA, b.hA)
	b.driver.MemCopyH2D(b.context, b.dGamma, b.hGamma)
	b.driver.MemCopyH2D(b.context, b.dBeta, b.hBeta)
}

func (b *Benchmark) exec() {
	blockSize := b.BlockSize

	// One workgroup per row
	globalSize := [3]uint32{uint32(b.Rows * blockSize), 1, 1}
	localSize := [3]uint16{uint16(blockSize), 1, 1}

	args := KernelArgs{
		A:     b.dA,
		B:     b.dB,
		Gamma: b.dGamma,
		Beta:  b.dBeta,
		Rows:  int32(b.Rows),
		Hid:   int32(b.Hid),
	}
	b.driver.LaunchKernel(b.context, b.kernel,
		globalSize, localSize, &args)

	b.driver.MemCopyD2H(b.context, b.hB, b.dB)
}

// Verify verifies the result.
func (b *Benchmark) Verify() {
	errors := 0
	eps := 1e-5

	for r := 0; r < b.Rows; r++ {
		offset := r * b.Hid

		// Compute mean
		sum := 0.0
		for j := 0; j < b.Hid; j++ {
			sum += float64(b.hA[offset+j])
		}
		mean := sum / float64(b.Hid)

		// Compute variance
		varSum := 0.0
		for j := 0; j < b.Hid; j++ {
			diff := float64(b.hA[offset+j]) - mean
			varSum += diff * diff
		}
		variance := varSum / float64(b.Hid)
		invStd := 1.0 / math.Sqrt(variance+eps)

		// Check normalized output
		for j := 0; j < b.Hid; j++ {
			norm := (float64(b.hA[offset+j]) - mean) * invStd
			exp := float32(float64(b.hGamma[j])*norm + float64(b.hBeta[j]))
			diff := math.Abs(float64(b.hB[offset+j] - exp))
			tol := 1e-3*math.Abs(float64(exp)) + 1e-5

			if diff > tol {
				if errors < 10 {
					log.Printf("Mismatch at [%d,%d]: got %f, expected %f",
						r, j, b.hB[offset+j], exp)
				}
				errors++
			}
		}
	}

	if errors > 0 {
		log.Panicf("FAIL: %d errors in layernorm verification", errors)
	}
	log.Printf("Passed!")
}
