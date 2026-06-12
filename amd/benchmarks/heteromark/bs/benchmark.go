// Package bs implements the Heteromark Black-Scholes option pricing benchmark.
// Single kernel: bs_kernel computes call and put option prices.
package bs

import (
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// KernelArgs defines kernel arguments.
type KernelArgs struct {
	StockPrice          driver.Ptr
	StrikePrice         driver.Ptr
	Years               driver.Ptr
	CallPrice           driver.Ptr
	PutPrice            driver.Ptr
	RiskFreeRate        float32
	Volatility          float32
	N                   int32
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

// Benchmark defines the Black-Scholes benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	bsKernel *insts.KernelCodeObject

	NumElements int

	riskFreeRate float32
	volatility   float32

	hStockPrice  []float32
	hStrikePrice []float32
	hYears       []float32
	hCallPrice   []float32
	hPutPrice    []float32

	dStockPrice  driver.Ptr
	dStrikePrice driver.Ptr
	dYears       driver.Ptr
	dCallPrice   driver.Ptr
	dPutPrice    driver.Ptr

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

// NewBenchmark creates a new Black-Scholes benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.NumElements = 1048576
	b.riskFreeRate = 0.02
	b.volatility = 0.30
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
	b.bsKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "bs_kernel")
	if b.bsKernel == nil {
		log.Panic("Failed to load bs_kernel binary")
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

	rand.Seed(42)
	b.hStockPrice = make([]float32, n)
	b.hStrikePrice = make([]float32, n)
	b.hYears = make([]float32, n)
	b.hCallPrice = make([]float32, n)
	b.hPutPrice = make([]float32, n)

	for i := 0; i < n; i++ {
		b.hStockPrice[i] = 5.0 + float32(rand.Intn(1000))*0.1
		b.hStrikePrice[i] = 1.0 + float32(rand.Intn(1000))*0.1
		b.hYears[i] = 0.25 + float32(rand.Intn(100))*0.1
	}

	bytes := uint64(n * 4)
	if b.useUnifiedMemory {
		b.dStockPrice = b.driver.AllocateUnifiedMemory(b.context, bytes)
		b.dStrikePrice = b.driver.AllocateUnifiedMemory(b.context, bytes)
		b.dYears = b.driver.AllocateUnifiedMemory(b.context, bytes)
		b.dCallPrice = b.driver.AllocateUnifiedMemory(b.context, bytes)
		b.dPutPrice = b.driver.AllocateUnifiedMemory(b.context, bytes)
	} else {
		b.dStockPrice = b.driver.AllocateMemory(b.context, bytes)
		b.dStrikePrice = b.driver.AllocateMemory(b.context, bytes)
		b.dYears = b.driver.AllocateMemory(b.context, bytes)
		b.dCallPrice = b.driver.AllocateMemory(b.context, bytes)
		b.dPutPrice = b.driver.AllocateMemory(b.context, bytes)
	}
}

func (b *Benchmark) exec() {
	n := b.NumElements

	b.driver.MemCopyH2D(b.context, b.dStockPrice, b.hStockPrice)
	b.driver.MemCopyH2D(b.context, b.dStrikePrice, b.hStrikePrice)
	b.driver.MemCopyH2D(b.context, b.dYears, b.hYears)

	blockSize := 256
	gsX := uint32(((n - 1) / blockSize + 1) * blockSize)
	localSize := [3]uint16{uint16(blockSize), 1, 1}
	globalSize := [3]uint32{gsX, 1, 1}

	kernelArg := KernelArgs{
		StockPrice:          b.dStockPrice,
		StrikePrice:         b.dStrikePrice,
		Years:               b.dYears,
		CallPrice:           b.dCallPrice,
		PutPrice:            b.dPutPrice,
		RiskFreeRate:        b.riskFreeRate,
		Volatility:          b.volatility,
		N:                   int32(n),
		HiddenBlockCountX:   gsX / uint32(localSize[0]),
		HiddenBlockCountY:   1,
		HiddenBlockCountZ:   1,
		HiddenGroupSizeX:    localSize[0],
		HiddenGroupSizeY:    localSize[1],
		HiddenGroupSizeZ:    localSize[2],
		HiddenRemainderX:    uint16(gsX % uint32(localSize[0])),
		HiddenRemainderY:    0,
		HiddenRemainderZ:    0,
		HiddenGlobalOffsetX: 0,
		HiddenGlobalOffsetY: 0,
		HiddenGlobalOffsetZ: 0,
		HiddenGridDims:      1,
	}
	b.driver.LaunchKernel(b.context, b.bsKernel,
		globalSize, localSize, &kernelArg)

	b.driver.MemCopyD2H(b.context, b.hCallPrice, b.dCallPrice)
	b.driver.MemCopyD2H(b.context, b.hPutPrice, b.dPutPrice)
}

// cndCPU computes the cumulative normal distribution (Abramowitz & Stegun).
func cndCPU(d float64) float64 {
	k := 1.0 / (1.0 + 0.2316419*math.Abs(d))
	cnd := 0.3989422804 * math.Exp(-0.5*d*d) *
		(k * (0.319381530 + k*(-0.356563782+k*(1.781477937+
			k*(-1.821255978+k*1.330274429)))))
	if d > 0 {
		cnd = 1.0 - cnd
	}
	return cnd
}

// Verify verifies the GPU results against a CPU reference.
func (b *Benchmark) Verify() {
	n := b.NumElements

	maxErr := 0.0
	for i := 0; i < n; i++ {
		s := float64(b.hStockPrice[i])
		x := float64(b.hStrikePrice[i])
		t := float64(b.hYears[i])
		r := float64(b.riskFreeRate)
		v := float64(b.volatility)

		sqrtT := math.Sqrt(t)
		d1 := (math.Log(s/x) + (r+0.5*v*v)*t) / (v * sqrtT)
		d2 := d1 - v*sqrtT

		callRef := s*cndCPU(d1) - x*math.Exp(-r*t)*cndCPU(d2)
		putRef := x*math.Exp(-r*t)*cndCPU(-d2) - s*cndCPU(-d1)

		errCall := math.Abs(float64(b.hCallPrice[i]) - callRef)
		errPut := math.Abs(float64(b.hPutPrice[i]) - putRef)
		if errCall > maxErr {
			maxErr = errCall
		}
		if errPut > maxErr {
			maxErr = errPut
		}
	}

	fmt.Fprintf(os.Stderr, "BS max absolute error: %e\n", maxErr)
	if maxErr < 1e-3 {
		fmt.Fprintf(os.Stderr, "Passed!\n")
	} else {
		log.Panicf("FAIL: max error %e exceeds threshold", maxErr)
	}
}
