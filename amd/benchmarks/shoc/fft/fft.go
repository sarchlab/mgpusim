// Package fft include the benchmark of Fourier
package fft

import (
	"encoding/binary"
	"fmt"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"log"
	"math/rand"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// Float2 is two floats
type Float2 struct {
	X, Y float32
}

// KernelArgs defines kernel arguments for GCN3
type KernelArgs struct {
	Work                driver.Ptr
	Smem                driver.LocalPtr
	Paddinng            int32
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

// CDNA3KernelArgs defines kernel arguments for CDNA3 (GFX942).
// The CDNA3 kernel uses __shared__ memory, so no Smem parameter.
//
//	offset 0x00: Work               (8 bytes, global_buffer)
//	offset 0x08: HiddenBlockCountX  (4 bytes)
//	offset 0x0c: HiddenBlockCountY  (4 bytes)
//	offset 0x10: HiddenBlockCountZ  (4 bytes)
//	offset 0x14: HiddenGroupSizeX   (2 bytes)
//	offset 0x16: HiddenGroupSizeY   (2 bytes)
//	offset 0x18: HiddenGroupSizeZ   (2 bytes)
//	offset 0x1a: HiddenRemainderX   (2 bytes)
//	offset 0x1c: HiddenRemainderY   (2 bytes)
//	offset 0x1e: HiddenRemainderZ   (2 bytes)
//	offset 0x20: Padding2           (16 bytes)
//	offset 0x30: HiddenGlobalOffsetX (8 bytes)
//	offset 0x38: HiddenGlobalOffsetY (8 bytes)
//	offset 0x40: HiddenGlobalOffsetZ (8 bytes)
//	offset 0x48: HiddenGridDims     (2 bytes)
type CDNA3KernelArgs struct {
	Work                driver.Ptr
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

// Benchmark defines a benchmark
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
	fftKernel        *insts.KernelCodeObject

	Arch       arch.Type
	Bytes      int64
	BytesMode  bool
	Passes     int32
	halfNFfts  int64
	nFfts      int64
	halfNCmplx int64
	usedBytes  uint64
	dSource    driver.Ptr
	source     []Float2
	result     []Float2
}

// NewBenchmark returns a benchmark
func NewBenchmark(driver *driver.Runtime) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
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

//go:embed fft.hsaco
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

	b.fftKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "fft1D_512")
	if b.fftKernel == nil {
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
		b.queues = append(b.queues,
			b.driver.CreateCommandQueue(b.context))
	}

	b.initMem()
}

func (b *Benchmark) initMem() {
	if !b.BytesMode {
		b.Bytes = b.Bytes * 1024 * 1024
	}
	b.halfNFfts = b.Bytes / (512 * 4 * 2 * 2)
	b.nFfts = b.halfNFfts * 2
	b.halfNCmplx = b.halfNFfts * 512
	b.usedBytes = uint64(b.halfNCmplx) * 2 * 4 * 2

	b.source = make([]Float2, b.usedBytes>>3)
	b.result = make([]Float2, b.usedBytes>>3)
	b.fill()

	if b.useUnifiedMemory {
		b.dSource = b.driver.AllocateUnifiedMemory(b.context,
			b.usedBytes)
	} else {
		b.dSource = b.driver.AllocateMemory(b.context,
			b.usedBytes)
	}
	b.driver.EnqueueMemCopyH2D(b.queue, b.dSource, b.source)
}

func (b *Benchmark) exec() {
	localWorkSize := int64(64)
	vectorGlobalWSize := localWorkSize * b.nFfts

	globalSize := [3]uint32{uint32(vectorGlobalWSize), 1, 1}
	localSize := [3]uint16{uint16(localWorkSize), 1, 1}

	for k := int32(0); k < b.Passes; k++ {
		if b.Arch == arch.CDNA3 {
			b.execCDNA3(globalSize, localSize)
		} else {
			b.execGCN3(globalSize, localSize)
		}
	}
	b.readbacks = append(b.readbacks, benchmarks.Readback{Source: b.dSource, Target: 0})
}

func (b *Benchmark) execGCN3(
	globalSize [3]uint32,
	localSize [3]uint16,
) {
	args := KernelArgs{
		Work:                b.dSource,
		Smem:                8 * 8 * 9 * 8,
		Paddinng:            0,
		HiddenGlobalOffsetX: 0,
		HiddenGlobalOffsetY: 0,
		HiddenGlobalOffsetZ: 0,
	}

	b.driver.EnqueueLaunchKernel(b.queue,
		b.fftKernel,
		globalSize, localSize,
		&args,
	)
}

func (b *Benchmark) execCDNA3(
	globalSize [3]uint32,
	localSize [3]uint16,
) {
	args := CDNA3KernelArgs{
		Work: b.dSource,
	}

	b.driver.EnqueueLaunchKernel(b.queue,
		b.fftKernel,
		globalSize, localSize,
		&args,
	)
}

// Verify verifies
func (b *Benchmark) verify() error {
	mismatch := false

	if b.fftCPU() == 1 {
		mismatch = true
	}

	if mismatch {
		return fmt.Errorf("Mismatch!\n")
	}
	log.Printf("Passed!\n")

	return nil
}

func (b *Benchmark) fftCPU() int32 {
	fail := int32(0)
	fst := make([]Float2, b.nFfts<<6)
	snd := make([]Float2, b.nFfts<<6)
	for i := int64(0); i < (b.nFfts << 6); i++ {
		fst[i] = b.source[i]
	}

	for i := int64(0); i < (b.nFfts << 6); i++ {
		snd[i] = b.source[b.halfNCmplx+i]
	}

	for i := int64(0); i < (b.nFfts << 6); i++ {
		if fst[i].X != snd[i].X || fst[i].Y != snd[i].Y {
			fail = 1
		}
	}
	return fail
}

func (b *Benchmark) fill() {
	// Use a local random source so the input is reproducible. rand.Seed has
	// been a no-op since Go 1.24, so seeding the global generator no longer
	// produces a deterministic sequence.
	rng := rand.New(rand.NewSource(1))

	for i := int64(0); i < b.halfNCmplx; i++ {
		b.source[i].X = (rng.Float32())*2 - 1
		b.source[i].Y = (rng.Float32())*2 - 1
		b.source[i+b.halfNCmplx].X = b.source[i].X
		b.source[i+b.halfNCmplx].Y = b.source[i].Y
	}
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
		if !b.Verification {
			b.readbacks = nil
			b.Ended = true
			return nil
		}
		for i := range b.readbacks {
			var dst any
			switch b.readbacks[i].Target {
			case 0:
				dst = b.result
			}
			b.readbacks[i].Command = b.driver.EnqueueMemCopyD2H(b.queue, b.readbacks[i].Source, binary.Size(dst))
		}
		b.Phase = 3
	case 3:
		for _, copy := range b.readbacks {
			var dst any
			switch copy.Target {
			case 0:
				dst = b.result
			}
			if err := b.driver.ReadResult(copy.Command, dst); err != nil {
				return err
			}
		}
		if err := b.verify(); err != nil {
			return err
		}
		b.readbacks = nil
		b.Ended = true
	}
	return nil
}
