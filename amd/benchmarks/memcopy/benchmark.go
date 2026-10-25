package memcopy

import (
	"fmt"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"io"
	"log"
	"math/rand"

	"github.com/sarchlab/mgpusim/v5/amd/driver"
)

// Benchmark defines a benchmark
type Benchmark struct {
	benchmarks.Progress
	queue    driver.CommandQueue
	ptr      driver.Ptr
	readback driver.CommandID
	driver   *driver.Runtime
	context  driver.Context
	gpu      int

	ByteSize uint64
	data     []byte
	retData  []byte

	useUnifiedMemory bool
}

// NewBenchmark creates a new benchmar
func NewBenchmark(driver *driver.Runtime) *Benchmark {
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

func (b *Benchmark) Step() error {
	switch b.Phase {
	case 0:
		b.driver.SelectGPU(b.context, b.gpu)
		b.queue = b.driver.CreateCommandQueue(b.context)
		rng := rand.New(rand.NewSource(1))
		b.data = make([]byte, b.ByteSize)
		b.retData = make([]byte, b.ByteSize)
		for i := range b.data {
			b.data[i] = byte(rng.Int())
		}
		if b.useUnifiedMemory {
			b.ptr = b.driver.AllocateUnifiedMemory(b.context, b.ByteSize)
		} else {
			b.ptr = b.driver.AllocateMemory(b.context, b.ByteSize)
		}
		b.driver.EnqueueMemCopyH2D(b.queue, b.ptr, b.data)
		b.Phase = 1
	case 1:
		b.readback = b.driver.EnqueueMemCopyD2H(b.queue, b.ptr, int(b.ByteSize))
		b.Phase = 2
	case 2:
		if err := b.driver.ReadResult(b.readback, b.retData); err != nil {
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
		if b.data[i] != b.retData[i] {
			return fmt.Errorf("error at %d, expected %02x, but get %02x",
				i, b.data[i], b.retData[i])
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
type state struct {
	Progress     benchmarks.Progress
	Context      driver.Context
	Queue        driver.CommandQueue
	Ptr          driver.Ptr
	Readback     driver.CommandID
	Data, Result []byte
}

func (b *Benchmark) config() config {
	return config{b.gpu, b.ByteSize, b.useUnifiedMemory, b.Verification}
}
func (b *Benchmark) SaveCheckpoint(w io.Writer) error {
	return modeling.WriteCheckpoint(
		w,
		b.config(),
		state{b.Progress, b.context, b.queue, b.ptr, b.readback, b.data, b.retData},
		nil,
	)
}
func (b *Benchmark) LoadCheckpoint(r io.Reader) error {
	var s state
	if err := modeling.ReadCheckpoint(r, b.config(), &s, nil); err != nil {
		return err
	}
	b.Progress = s.Progress
	b.context = s.Context
	b.queue = s.Queue
	b.ptr = s.Ptr
	b.readback = s.Readback
	b.data = s.Data
	b.retData = s.Result
	return nil
}
