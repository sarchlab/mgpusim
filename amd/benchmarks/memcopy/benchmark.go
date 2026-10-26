package memcopy

import (
	"fmt"
	"log"
	"math/rand"

	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
)

// Benchmark defines a benchmark
type Benchmark struct {
	*State
	benchmarks.Saved[config, State]
	driver *driver.Runtime
}

// NewBenchmark creates a new benchmar
func NewBenchmark(driver *driver.Runtime) *Benchmark {
	b := &Benchmark{State: &State{}}
	b.Saved = benchmarks.NewSaved(b.State, b.config, nil)
	b.driver = driver
	b.Context = driver.Init()
	return b
}

// SelectGPU selects gpu
func (b *Benchmark) SelectGPU(gpus []int) {
	if len(gpus) > 1 {
		panic("memory copy benchmark only support a single GPU")
	}
	b.GPU = gpus[0]
}

// SetUnifiedMemory Use Unified Memory
func (b *Benchmark) SetUnifiedMemory() {
	b.UseUnifiedMemory = true
}

func (b *Benchmark) Step() error {
	if err := b.Prepare(); err != nil {
		return err
	}
	if err := b.ReleaseUploads(b.driver); err != nil {
		return err
	}
	switch b.Phase {
	case 0:
		b.driver.SelectGPU(b.Context, b.GPU)
		b.Queue = b.driver.CreateCommandQueue(b.Context)
		rng := rand.New(rand.NewSource(1))
		b.Data = make([]byte, b.ByteSize)
		b.Result = make([]byte, b.ByteSize)
		for i := range b.Data {
			b.Data[i] = byte(rng.Int())
		}
		if b.UseUnifiedMemory {
			b.Ptr = b.driver.AllocateUnifiedMemory(b.Context, b.ByteSize)
		} else {
			b.Ptr = b.driver.AllocateMemory(b.Context, b.ByteSize)
		}
		b.Upload(b.driver, b.Queue, b.Ptr, b.Data)
		b.Phase = 1
	case 1:
		b.Readback = benchmarks.Download(b.driver, b.Queue, b.Ptr, int(b.ByteSize))
		b.Phase = 2
	case 2:
		if err := benchmarks.Read(b.driver, b.Readback, b.Result); err != nil {
			return err
		}
		if b.Verification {
			if err := b.verify(); err != nil {
				return err
			}
		}
		b.Ended = true
	}
	return nil
}

// Verify verifies
func (b *Benchmark) verify() error {
	for i := uint64(0); i < b.ByteSize; i++ {
		if b.Data[i] != b.Result[i] {
			return fmt.Errorf("error at %d, expected %02x, but get %02x",
				i, b.Data[i], b.Result[i])
		}
	}
	log.Printf("Passed!")
	return nil
}

type config struct {
	GPU                   int
	Bytes                 uint64
	Unified, Verification bool
}
type State struct {
	benchmarks.Progress
	Context          driver.Context
	Queue            driver.CommandQueue
	Ptr              driver.Ptr
	Readback         driver.HostBuffer
	Data, Result     []byte
	GPU              int
	ByteSize         uint64
	UseUnifiedMemory bool
}

func (b *Benchmark) config() config {
	return config{b.GPU, b.ByteSize, b.UseUnifiedMemory, b.Verification}
}
