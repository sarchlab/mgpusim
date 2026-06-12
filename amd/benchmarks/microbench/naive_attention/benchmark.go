// Package naiveattention implements naive QKV attention microbenchmark.
package naiveattention

import (
	"log"
	"math"

	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// KernelArgs defines kernel arguments for naive_attention_kernel.
type KernelArgs struct {
	Q          driver.Ptr
	K          driver.Ptr
	V          driver.Ptr
	Out        driver.Ptr
	BatchHeads int32
	SeqLen     int32
	HeadDim    int32
	Pad0       int32
}

// Benchmark defines the naive attention benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	kernel *insts.KernelCodeObject

	Batch         int
	SeqLen        int
	NumHeads      int
	HeadDim       int
	BlockSize     int
	MaxBatchHeads int
	MaxQueryRows  int

	useUnifiedMemory bool

	hQ   []float32
	hK   []float32
	hV   []float32
	hOut []float32

	dQ   driver.Ptr
	dK   driver.Ptr
	dV   driver.Ptr
	dOut driver.Ptr
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

// NewBenchmark creates a new naive attention benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.Batch = 4
	b.SeqLen = 128
	b.NumHeads = 12
	b.HeadDim = 64
	b.BlockSize = 256
	b.MaxBatchHeads = 0
	b.MaxQueryRows = 0
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

// SetBatch sets batch size.
func (b *Benchmark) SetBatch(v int) {
	b.Batch = v
}

// SetSeqLen sets sequence length.
func (b *Benchmark) SetSeqLen(v int) {
	b.SeqLen = v
}

// SetNumHeads sets number of heads.
func (b *Benchmark) SetNumHeads(v int) {
	b.NumHeads = v
}

// SetHeadDim sets head dimension.
func (b *Benchmark) SetHeadDim(v int) {
	b.HeadDim = v
}

// SetMaxBatchHeads limits launched batch-head groups (0 = full Batch*NumHeads).
func (b *Benchmark) SetMaxBatchHeads(v int) {
	b.MaxBatchHeads = v
}

// SetMaxQueryRows limits launched query rows (0 = full SeqLen).
func (b *Benchmark) SetMaxQueryRows(v int) {
	b.MaxQueryRows = v
}

func (b *Benchmark) loadProgram() {
	b.kernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "naive_attention_kernel")
	if b.kernel == nil {
		log.Panic("Failed to load naive_attention_kernel")
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
	batchHeads := b.Batch * b.NumHeads
	n := batchHeads * b.SeqLen * b.HeadDim

	b.hQ = make([]float32, n)
	b.hK = make([]float32, n)
	b.hV = make([]float32, n)
	b.hOut = make([]float32, n)

	for i := 0; i < n; i++ {
		b.hQ[i] = float32(i%1000)*0.002 - 1.0
		b.hK[i] = float32((i+333)%1000)*0.002 - 1.0
		b.hV[i] = float32((i+666)%1000)*0.002 - 1.0
	}

	bytes := uint64(n * 4)

	if b.useUnifiedMemory {
		b.dQ = b.driver.AllocateUnifiedMemory(b.context, bytes)
		b.dK = b.driver.AllocateUnifiedMemory(b.context, bytes)
		b.dV = b.driver.AllocateUnifiedMemory(b.context, bytes)
		b.dOut = b.driver.AllocateUnifiedMemory(b.context, bytes)
	} else {
		b.dQ = b.driver.AllocateMemory(b.context, bytes)
		b.dK = b.driver.AllocateMemory(b.context, bytes)
		b.dV = b.driver.AllocateMemory(b.context, bytes)
		b.dOut = b.driver.AllocateMemory(b.context, bytes)
	}

	b.driver.MemCopyH2D(b.context, b.dQ, b.hQ)
	b.driver.MemCopyH2D(b.context, b.dK, b.hK)
	b.driver.MemCopyH2D(b.context, b.dV, b.hV)
}

func (b *Benchmark) exec() {
	batchHeads := b.Batch * b.NumHeads
	blockSize := b.BlockSize

	launchBatchHeads := batchHeads
	if b.MaxBatchHeads > 0 && b.MaxBatchHeads < launchBatchHeads {
		launchBatchHeads = b.MaxBatchHeads
	}

	launchQueryRows := b.SeqLen
	if b.MaxQueryRows > 0 && b.MaxQueryRows < launchQueryRows {
		launchQueryRows = b.MaxQueryRows
	}

	// Grid: (batch_heads, query_rows) — one workgroup per (bh, query_row)
	globalSize := [3]uint32{
		uint32(launchBatchHeads * blockSize),
		uint32(launchQueryRows),
		1,
	}
	localSize := [3]uint16{uint16(blockSize), 1, 1}

	args := KernelArgs{
		Q:          b.dQ,
		K:          b.dK,
		V:          b.dV,
		Out:        b.dOut,
		BatchHeads: int32(batchHeads),
		SeqLen:     int32(b.SeqLen),
		HeadDim:    int32(b.HeadDim),
	}
	b.driver.LaunchKernel(b.context, b.kernel,
		globalSize, localSize, &args)

	b.driver.MemCopyD2H(b.context, b.hOut, b.dOut)
}

func (b *Benchmark) computeScores(
	bh, qr int, scale float64,
) []float64 {
	qOff := (bh*b.SeqLen + qr) * b.HeadDim
	scores := make([]float64, b.SeqLen)
	rowMax := -1e30

	for kc := 0; kc < b.SeqLen; kc++ {
		kOff := (bh*b.SeqLen + kc) * b.HeadDim
		dot := 0.0
		for d := 0; d < b.HeadDim; d++ {
			dot += float64(b.hQ[qOff+d]) * float64(b.hK[kOff+d])
		}
		scores[kc] = dot * scale
		if scores[kc] > rowMax {
			rowMax = scores[kc]
		}
	}

	rowSum := 0.0
	for kc := range scores {
		scores[kc] = math.Exp(scores[kc] - rowMax)
		rowSum += scores[kc]
	}
	for kc := range scores {
		scores[kc] /= rowSum
	}

	return scores
}

func (b *Benchmark) verifyRow(
	bh, qr int, scores []float64,
) int {
	qOff := (bh*b.SeqLen + qr) * b.HeadDim
	errs := 0

	for d := 0; d < b.HeadDim; d++ {
		expected := 0.0
		for kc := 0; kc < b.SeqLen; kc++ {
			vOff := (bh*b.SeqLen + kc) * b.HeadDim
			expected += scores[kc] * float64(b.hV[vOff+d])
		}
		exp := float32(expected)
		got := b.hOut[qOff+d]
		diff := math.Abs(float64(got - exp))
		tol := 1e-2*math.Abs(float64(exp)) + 1e-4

		if diff > tol {
			errs++
		}
	}

	return errs
}

// Verify verifies the result.
func (b *Benchmark) Verify() {
	errors := 0
	batchHeads := b.Batch * b.NumHeads
	scale := 1.0 / math.Sqrt(float64(b.HeadDim))

	for bh := 0; bh < batchHeads; bh++ {
		for qr := 0; qr < b.SeqLen; qr++ {
			scores := b.computeScores(bh, qr, scale)
			errors += b.verifyRow(bh, qr, scores)
		}
	}

	if errors > 0 {
		log.Panicf("FAIL: %d errors in naive_attention verification", errors)
	}
	log.Printf("Passed!")
}
