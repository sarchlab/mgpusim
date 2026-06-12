// Package s3d implements the SHOC S3D chemical kinetics benchmark.
package s3d

import (
	"log"
	"math"

	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// KernelArgs defines kernel arguments for s3d_kernel.
type KernelArgs struct {
	T          driver.Ptr
	C          driver.Ptr
	Rates      driver.Ptr
	N          int32
	NumSpecies int32
}

// Benchmark defines the S3D chemical kinetics benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	kernel *insts.KernelCodeObject

	NumCells   int
	NumSpecies int
	BlockSize  int

	useUnifiedMemory bool

	hT     []float32
	hC     []float32
	hRates []float32

	dT     driver.Ptr
	dC     driver.Ptr
	dRates driver.Ptr
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

// NewBenchmark creates a new S3D benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.NumCells = 4096
	b.NumSpecies = 22
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

// SetNumElements sets the number of grid cells.
func (b *Benchmark) SetNumElements(n int) {
	b.NumCells = n
}

func (b *Benchmark) loadProgram() {
	b.kernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "s3d_kernel")
	if b.kernel == nil {
		log.Panic("Failed to load s3d_kernel")
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
	n := b.NumCells
	ns := b.NumSpecies

	// Initialize temperatures (300K - 3000K range)
	b.hT = make([]float32, n)
	for i := 0; i < n; i++ {
		b.hT[i] = 300.0 + float32(i%2700)
	}

	// Initialize concentrations (0.01 - 1.0 range)
	b.hC = make([]float32, n*ns)
	for i := 0; i < n*ns; i++ {
		b.hC[i] = 0.01 + float32(i%100)*0.01
	}

	b.hRates = make([]float32, n*ns)

	tBytes := uint64(n * 4)
	cBytes := uint64(n * ns * 4)
	rBytes := uint64(n * ns * 4)

	if b.useUnifiedMemory {
		b.dT = b.driver.AllocateUnifiedMemory(b.context, tBytes)
		b.dC = b.driver.AllocateUnifiedMemory(b.context, cBytes)
		b.dRates = b.driver.AllocateUnifiedMemory(b.context, rBytes)
	} else {
		b.dT = b.driver.AllocateMemory(b.context, tBytes)
		b.dC = b.driver.AllocateMemory(b.context, cBytes)
		b.dRates = b.driver.AllocateMemory(b.context, rBytes)
	}

	b.driver.MemCopyH2D(b.context, b.dT, b.hT)
	b.driver.MemCopyH2D(b.context, b.dC, b.hC)
}

func (b *Benchmark) exec() {
	n := b.NumCells
	blockSize := b.BlockSize
	grid := (n + blockSize - 1) / blockSize

	globalSize := [3]uint32{uint32(grid * blockSize), 1, 1}
	localSize := [3]uint16{uint16(blockSize), 1, 1}

	args := KernelArgs{
		T:          b.dT,
		C:          b.dC,
		Rates:      b.dRates,
		N:          int32(n),
		NumSpecies: int32(b.NumSpecies),
	}
	b.driver.LaunchKernel(b.context, b.kernel,
		globalSize, localSize, &args)

	b.driver.MemCopyD2H(b.context, b.hRates, b.dRates)
}

// Verify verifies the result.
func (b *Benchmark) Verify() {
	n := b.NumCells
	ns := b.NumSpecies
	errors := 0
	Rgas := 8.314

	for i := 0; i < n; i++ {
		temp := float64(b.hT[i])
		invRT := 1.0 / (Rgas * temp)
		base := i * ns

		for s := 0; s < ns; s++ {
			A := 1.0e10 + float64(s)*1.0e8
			nExp := 0.5 + float64(s)*0.1
			Ea := 20000.0 + float64(s)*5000.0

			k := A * math.Pow(temp, nExp) * math.Exp(-Ea*invRT)
			conc := float64(b.hC[base+s])
			expected := float32(k * conc)
			got := b.hRates[base+s]

			diff := math.Abs(float64(got - expected))
			tol := 1e-1*math.Abs(float64(expected)) + 1e-3

			if diff > tol {
				if errors < 10 {
					log.Printf("Mismatch at cell %d species %d: got %e, expected %e",
						i, s, got, expected)
				}
				errors++
			}
		}
	}

	if errors > 0 {
		log.Panicf("FAIL: %d errors in s3d verification", errors)
	}
	log.Printf("Passed!")
}
