// Package spmv include the benchmark of sparse matrix-vector matiplication.
package spmv

import (
	// embed hsaco files
	_ "embed"
	"encoding/binary"
	"fmt"
	"log"
	"math/rand"
	"os"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
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
	*State
	benchmarks.Saved[checkpointConfig, State]

	driver *driver.Runtime

	spmvKernel *insts.KernelCodeObject
}

// NewBenchmark creates a new benchmark
func NewBenchmark(driver *driver.Runtime) *Benchmark {
	b := new(Benchmark)
	b.State = &State{}
	b.Saved = benchmarks.NewSaved(b.State, b.checkpointConfig, b.bind)

	b.driver = driver
	b.Context = driver.Init()
	b.Maxval = 10
	return b
}

// SelectGPU selects GPU
func (b *Benchmark) SelectGPU(gpus []int) {
	b.Gpus = gpus
}

// SetUnifiedMemory uses Unified Memory
func (b *Benchmark) SetUnifiedMemory() {
	b.UseUnifiedMemory = true
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
	b.driver.SelectGPU(b.Context, b.Gpus[len(b.Gpus)-1])
	b.Queue = b.driver.CreateCommandQueue(b.Context)

	b.loadProgram()

	b.initMem()
}

func (b *Benchmark) initMem() {
	b.NItems = int32(float64(b.Dim) * float64(b.Dim) * b.Sparsity)
	fmt.Fprintf(os.Stderr, "Number of non-zero elements %d\n", b.NItems)

	b.Matrix = csr.MakeMatrixGenerator(uint32(b.Dim), uint32(b.NItems)).
		GenerateMatrix()
	b.Vec = make([]float32, b.Dim)
	b.Out = make([]float32, b.Dim)

	// Use a local random source so the input is reproducible. The global
	// generator is seeded randomly at startup (rand.Seed is a no-op since
	// Go 1.24), which made this benchmark's input differ on every run.
	rng := rand.New(rand.NewSource(1))
	for j := int32(0); j < b.Dim; j++ {
		b.Vec[j] = (rng.Float32() * b.Maxval)
	}

	if b.UseUnifiedMemory {
		b.allocateUnifiedMemory()
	} else {
		b.allocateMemory()
	}
}

func (b *Benchmark) allocateUnifiedMemory() {
	b.DValData = b.driver.AllocateUnifiedMemory(b.Context,
		uint64(b.NItems*4))
	b.DVecData = b.driver.AllocateUnifiedMemory(b.Context,
		uint64(b.Dim*4))
	b.DColsData = b.driver.AllocateUnifiedMemory(b.Context,
		uint64(b.NItems*4))
	b.DRowDData = b.driver.AllocateUnifiedMemory(b.Context,
		uint64((b.Dim+1)*4))
	b.DOutData = b.driver.AllocateUnifiedMemory(b.Context,
		uint64(b.Dim*4))
}

func (b *Benchmark) allocateMemory() {
	b.DValData = b.driver.AllocateMemory(b.Context,
		uint64(b.NItems*4))
	b.DVecData = b.driver.AllocateMemory(b.Context,
		uint64(b.Dim*4))
	b.DColsData = b.driver.AllocateMemory(b.Context,
		uint64(b.NItems*4))
	b.DRowDData = b.driver.AllocateMemory(b.Context,
		uint64((b.Dim+1)*4))
	b.DOutData = b.driver.AllocateMemory(b.Context,
		uint64(b.Dim*4))
}

func (b *Benchmark) exec() {
	b.Upload(b.driver, b.Queue, b.DValData, b.Matrix.Values)
	b.Upload(b.driver, b.Queue, b.DVecData, b.Vec)
	b.Upload(b.driver, b.Queue, b.DColsData, b.Matrix.ColumnNumbers)
	b.Upload(b.driver, b.Queue, b.DRowDData, b.Matrix.RowOffsets)
	b.Upload(b.driver, b.Queue, b.DOutData, b.Out)

	blockSize := int32(128)

	globalSize := [3]uint32{uint32(b.Dim), 1, 1}
	localSize := [3]uint16{uint16(blockSize), 1, 1}

	if b.Arch == arch.CDNA3 {
		b.execCDNA3(globalSize, localSize)
	} else {
		b.execGCN3(globalSize, localSize)
	}

	b.Readbacks = append(b.Readbacks, benchmarks.Readback{Source: b.DOutData, Target: 0})
}

func (b *Benchmark) execGCN3(
	globalSize [3]uint32, localSize [3]uint16,
) {
	args := KernelArgs{
		Val:           b.DValData,
		Vec:           b.DVecData,
		Cols:          b.DColsData,
		RowDelimiters: b.DRowDData,
		Dim:           b.Dim,
		Padding:       0,
		Out:           b.DOutData,
	}

	b.driver.EnqueueLaunchKernel(b.Queue,
		b.spmvKernel,
		globalSize, localSize,
		&args,
	)
}

func (b *Benchmark) execCDNA3(
	globalSize [3]uint32, localSize [3]uint16,
) {
	args := CDNA3KernelArgs{
		Val:           b.DValData,
		Vec:           b.DVecData,
		Cols:          b.DColsData,
		RowDelimiters: b.DRowDData,
		Dim:           b.Dim,
		Out:           b.DOutData,
	}

	b.driver.EnqueueLaunchKernel(b.Queue,
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
		if b.Out[i] != cpuOutput[i] {
			mismatch = true
			log.Printf("not match at (%d), expected %f to equal %f\n",
				i,
				b.Out[i], cpuOutput[i])
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
		for j := b.Matrix.RowOffsets[i]; j < b.Matrix.RowOffsets[i+1]; j++ {
			col := b.Matrix.ColumnNumbers[j]
			t += b.Matrix.Values[j] * b.Vec[col]
		}
		cpuOutput[i] = t
	}
	return cpuOutput
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
		b.setup()
		b.Phase = 1
	case 1:
		b.exec()
		b.Phase = 2
	case 2:
		for i := range b.Readbacks {
			var dst any
			switch b.Readbacks[i].Target {
			case 0:
				dst = b.Out
			}
			b.Readbacks[i].Buffer = benchmarks.Download(b.driver, b.Queue, b.Readbacks[i].Source, binary.Size(dst))
		}
		b.Phase = 3
	case 3:
		for _, copy := range b.Readbacks {
			var dst any
			switch copy.Target {
			case 0:
				dst = b.Out
			}
			if err := benchmarks.Read(b.driver, copy.Buffer, dst); err != nil {
				return err
			}
		}
		if b.Verification {
			if err := b.verify(); err != nil {
				return err
			}
		}
		b.Readbacks = nil
		b.Ended = true
	}
	return nil
}
