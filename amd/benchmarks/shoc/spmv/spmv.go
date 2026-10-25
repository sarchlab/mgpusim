// Package spmv include the benchmark of sparse matrix-vector matiplication.
package spmv

import (
	"encoding/binary"
	"fmt"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"log"
	"math/rand"
	"os"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/matrix/csr"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// KernelArgs sets up kernel arguments for GCN3
type KernelArgs struct {
	Val                 driver.Ptr
	Vec                 driver.Ptr
	Cols                driver.Ptr
	RowDelimiters       driver.Ptr
	Dim                 int32
	Padding             int32
	Out                 driver.Ptr
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

// CDNA3KernelArgs sets up kernel arguments for CDNA3
type CDNA3KernelArgs struct {
	Val                 driver.Ptr
	Vec                 driver.Ptr
	Cols                driver.Ptr
	RowDelimiters       driver.Ptr
	Dim                 int32
	Padding             int32
	Out                 driver.Ptr
	HiddenBlockCountX   uint32
	HiddenBlockCountY   uint32
	HiddenBlockCountZ   uint32
	HiddenGroupSizeX    uint16
	HiddenGroupSizeY    uint16
	HiddenGroupSizeZ    uint16
	HiddenRemainderX    uint16
	HiddenRemainderY    uint16
	HiddenRemainderZ    uint16
	Padding2            [16]byte
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
	HiddenGridDims      uint16
}

// Benchmark set up test parameters
type Benchmark struct {
	benchmarks.Progress
	initialConfig  checkpointConfig
	configCaptured bool
	readbacks      []benchmarks.Readback
	queue          driver.CommandQueue

	driver           *driver.Runtime
	context          driver.Context
	gpus             []int
	queues           []driver.CommandQueue
	useUnifiedMemory bool
	spmvKernel       *insts.KernelCodeObject

	Arch      arch.Type
	Dim       int32
	Sparsity  float64
	dValData  driver.Ptr
	dVecData  driver.Ptr
	dColsData driver.Ptr
	dRowDData driver.Ptr
	dOutData  driver.Ptr
	nItems    int32
	vec       []float32
	out       []float32
	maxval    float32
	matrix    csr.Matrix
}

// NewBenchmark creates a new benchmark
func NewBenchmark(driver *driver.Runtime) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.maxval = 10
	return b
}

// SelectGPU selects GPU
func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
}

// SetUnifiedMemory uses Unified Memory
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

//go:embed spmv.hsaco
var gcn3HSACOBytes []byte

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

func (b *Benchmark) loadProgram() {
	var hsacoBytes []byte
	if b.Arch == arch.CDNA3 {
		hsacoBytes = cdna3HSACOBytes
	} else {
		hsacoBytes = gcn3HSACOBytes
	}

	b.spmvKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "spmv_csr_scalar_kernel")
	if b.spmvKernel == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// Prepare the inputs for the first queued phase.
func (b *Benchmark) setup() {
	b.driver.SelectGPU(b.context, b.gpus[0])
	b.queue = b.driver.CreateCommandQueue(b.context)

	b.loadProgram()

	for _, gpu := range b.gpus {
		b.driver.SelectGPU(b.context, gpu)
		b.queues = append(b.queues, b.driver.CreateCommandQueue(b.context))
	}

	b.initMem()
}

func (b *Benchmark) initMem() {
	b.nItems = int32(float64(b.Dim) * float64(b.Dim) * b.Sparsity)
	fmt.Fprintf(os.Stderr, "Number of non-zero elements %d\n", b.nItems)

	b.matrix = csr.
		MakeMatrixGenerator(uint32(b.Dim), uint32(b.nItems)).
		GenerateMatrix()
	b.vec = make([]float32, b.Dim)
	b.out = make([]float32, b.Dim)

	// Use a local random source so the input is reproducible. The global
	// generator is seeded randomly at startup (rand.Seed is a no-op since
	// Go 1.24), which made this benchmark's input differ on every run.
	rng := rand.New(rand.NewSource(1))
	for j := int32(0); j < b.Dim; j++ {
		b.vec[j] = (rng.Float32() * b.maxval)
	}

	if b.useUnifiedMemory {
		b.allocateUnifiedMemory()
	} else {
		b.allocateMemory()
	}
}

func (b *Benchmark) allocateUnifiedMemory() {
	b.dValData = b.driver.AllocateUnifiedMemory(b.context,
		uint64(b.nItems*4))
	b.dVecData = b.driver.AllocateUnifiedMemory(b.context,
		uint64(b.Dim*4))
	b.dColsData = b.driver.AllocateUnifiedMemory(b.context,
		uint64(b.nItems*4))
	b.dRowDData = b.driver.AllocateUnifiedMemory(b.context,
		uint64((b.Dim+1)*4))
	b.dOutData = b.driver.AllocateUnifiedMemory(b.context,
		uint64(b.Dim*4))
}

func (b *Benchmark) allocateMemory() {
	b.dValData = b.driver.AllocateMemory(b.context,
		uint64(b.nItems*4))
	b.dVecData = b.driver.AllocateMemory(b.context,
		uint64(b.Dim*4))
	b.dColsData = b.driver.AllocateMemory(b.context,
		uint64(b.nItems*4))
	b.dRowDData = b.driver.AllocateMemory(b.context,
		uint64((b.Dim+1)*4))
	b.dOutData = b.driver.AllocateMemory(b.context,
		uint64(b.Dim*4))
}

func (b *Benchmark) exec() {
	b.driver.EnqueueMemCopyH2D(b.queue, b.dValData, b.matrix.Values)
	b.driver.EnqueueMemCopyH2D(b.queue, b.dVecData, b.vec)
	b.driver.EnqueueMemCopyH2D(b.queue, b.dColsData, b.matrix.ColumnNumbers)
	b.driver.EnqueueMemCopyH2D(b.queue, b.dRowDData, b.matrix.RowOffsets)
	b.driver.EnqueueMemCopyH2D(b.queue, b.dOutData, b.out)

	blockSize := int32(128)

	globalSize := [3]uint32{uint32(b.Dim), 1, 1}
	localSize := [3]uint16{uint16(blockSize), 1, 1}

	if b.Arch == arch.CDNA3 {
		b.execCDNA3(globalSize, localSize)
	} else {
		b.execGCN3(globalSize, localSize)
	}

	b.readbacks = append(b.readbacks, benchmarks.Readback{Source: b.dOutData, Target: 0})
}

func (b *Benchmark) execGCN3(
	globalSize [3]uint32, localSize [3]uint16,
) {
	args := KernelArgs{
		Val:           b.dValData,
		Vec:           b.dVecData,
		Cols:          b.dColsData,
		RowDelimiters: b.dRowDData,
		Dim:           b.Dim,
		Padding:       0,
		Out:           b.dOutData,
	}

	b.driver.EnqueueLaunchKernel(b.queue,
		b.spmvKernel,
		globalSize, localSize,
		&args,
	)
}

func (b *Benchmark) execCDNA3(
	globalSize [3]uint32, localSize [3]uint16,
) {
	args := CDNA3KernelArgs{
		Val:           b.dValData,
		Vec:           b.dVecData,
		Cols:          b.dColsData,
		RowDelimiters: b.dRowDData,
		Dim:           b.Dim,
		Out:           b.dOutData,
	}

	b.driver.EnqueueLaunchKernel(b.queue,
		b.spmvKernel,
		globalSize, localSize,
		&args,
	)
}

// Verify verifies results
func (b *Benchmark) verify() error {
	cpuOutput := b.spmvCPU()

	mismatch := false
	for i := int32(0); i < b.Dim; i++ {
		if b.out[i] != cpuOutput[i] {
			mismatch = true
			log.Printf("not match at (%d), expected %f to equal %f\n",
				i,
				b.out[i], cpuOutput[i])
		}
	}

	if mismatch {
		return fmt.Errorf("Mismatch!\n")
	}

	log.Printf("Passed!\n")

	return nil
}

func (b *Benchmark) spmvCPU() []float32 {
	cpuOutput := make([]float32, b.Dim)
	for i := int32(0); i < b.Dim; i++ {
		t := float32(0)
		for j := b.matrix.RowOffsets[i]; j < b.matrix.RowOffsets[i+1]; j++ {
			col := b.matrix.ColumnNumbers[j]
			t += b.matrix.Values[j] * b.vec[col]
		}
		cpuOutput[i] = t
	}
	return cpuOutput
}

func (b *Benchmark) Step() error {
	switch b.Phase {
	case 0:
		b.initialConfig = b.checkpointConfig()
		b.configCaptured = true
		b.setup()
		b.Phase = 1
	case 1:
		b.exec()
		b.Phase = 2
	case 2:
		for i := range b.readbacks {
			var dst any
			switch b.readbacks[i].Target {
			case 0:
				dst = b.out
			}
			b.readbacks[i].Command = b.driver.EnqueueMemCopyD2H(b.queue, b.readbacks[i].Source, binary.Size(dst))
		}
		b.Phase = 3
	case 3:
		for _, copy := range b.readbacks {
			var dst any
			switch copy.Target {
			case 0:
				dst = b.out
			}
			if err := b.driver.ReadResult(copy.Command, dst); err != nil {
				return err
			}
		}
		if b.Verification {
			if err := b.verify(); err != nil {
				return err
			}
		}
		b.readbacks = nil
		b.Ended = true
	}
	return nil
}
