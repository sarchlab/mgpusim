package main

import (
	"flag"
	"log"
	"math/rand"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/samples/runner"
)

// Benchmark defines a benchmark
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpu     int

	ByteSize uint64
	data     []byte
	retData  []byte

	useUnifiedMemory bool
}

// NewBenchmark creates a new benchmar
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	return b
}

// SelectGPU selects gpu
func (b *Benchmark) SelectGPU(gpus []int) {
	if len(gpus) > 1 {
		panic("memory copy benchmark only support a single GPU")
	}
	b.gpu = gpus[0]
}

// SetUnifiedMemory Use Unified Memory
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

// Run runs the benchmark
func (b *Benchmark) Run() {
	b.driver.SelectGPU(b.context, b.gpu)

	b.data = make([]byte, b.ByteSize)
	b.retData = make([]byte, b.ByteSize)
	for i := uint64(0); i < b.ByteSize; i++ {
		b.data[i] = byte(rand.Int())
	}

	gpuData := b.driver.AllocateMemory(b.context, b.ByteSize)

	if b.useUnifiedMemory {
		gpuData = b.driver.AllocateUnifiedMemory(b.context, b.ByteSize)
	}

	b.driver.MemCopyH2D(b.context, gpuData, b.data)
	b.driver.MemCopyD2H(b.context, b.retData, gpuData)
}

// Verify verifies
func (b *Benchmark) Verify() {
	for i := uint64(0); i < b.ByteSize; i++ {
		if b.data[i] != b.retData[i] {
			log.Panicf("error at %d, expected %02x, but get %02x",
				i, b.data[i], b.retData[i])
		}
	}
	log.Printf("Passed!")
}

func main() {
	flag.Parse()

	runner := new(runner.Runner).Init()

	benchmark := NewBenchmark(runner.Driver())
	benchmark.ByteSize = 1048576

	runner.AddBenchmark(benchmark)

	runner.Run()
}
