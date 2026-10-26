// Package fft include the benchmark of Fourier
package fft

import (
	// embed hsaco files
	_ "embed"
	"encoding/binary"
	"fmt"
	"log"
	"math/rand"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
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
	*State
	benchmarks.Saved[checkpointConfig, State]

	driver *driver.Runtime

	fftKernel *insts.KernelCodeObject
}

// NewBenchmark returns a benchmark
func NewBenchmark(driver *driver.Runtime) *Benchmark {
	b := new(Benchmark)
	b.State = &State{}
	b.Saved = benchmarks.NewSaved(b.State, b.checkpointConfig, b.bind)

	b.driver = driver
	b.Context = driver.Init()
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
	b.driver.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.driver.CreateCommandQueue(b.Context)

	b.loadProgram()

	for _, gpu := range b.Gpus {
		b.driver.SelectGPU(b.Context, gpu)
		b.Queues = append(b.Queues,
			b.driver.CreateCommandQueue(b.Context))
	}

	b.initMem()
}

func (b *Benchmark) initMem() {
	if !b.BytesMode {
		b.Bytes = b.Bytes * 1024 * 1024
	}
	b.HalfNFfts = b.Bytes / (512 * 4 * 2 * 2)
	b.NFfts = b.HalfNFfts * 2
	b.HalfNCmplx = b.HalfNFfts * 512
	b.UsedBytes = uint64(b.HalfNCmplx) * 2 * 4 * 2

	b.Source = make([]Float2, b.UsedBytes>>3)
	b.Result = make([]Float2, b.UsedBytes>>3)
	b.fill()

	if b.UseUnifiedMemory {
		b.DSource = b.driver.AllocateUnifiedMemory(b.Context,
			b.UsedBytes)
	} else {
		b.DSource = b.driver.AllocateMemory(b.Context,
			b.UsedBytes)
	}
	b.driver.EnqueueMemCopyH2D(b.Queue, b.DSource, b.Source)
}

func (b *Benchmark) exec() {
	localWorkSize := int64(64)
	vectorGlobalWSize := localWorkSize * b.NFfts

	globalSize := [3]uint32{uint32(vectorGlobalWSize), 1, 1}
	localSize := [3]uint16{uint16(localWorkSize), 1, 1}

	for k := int32(0); k < b.Passes; k++ {
		if b.Arch == arch.CDNA3 {
			b.execCDNA3(globalSize, localSize)
		} else {
			b.execGCN3(globalSize, localSize)
		}
	}
	b.Readbacks = append(b.Readbacks, benchmarks.Readback{Source: b.DSource, Target: 0})
}

func (b *Benchmark) execGCN3(
	globalSize [3]uint32,
	localSize [3]uint16,
) {
	args := KernelArgs{
		Work:                b.DSource,
		Smem:                8 * 8 * 9 * 8,
		Paddinng:            0,
		HiddenGlobalOffsetX: 0,
		HiddenGlobalOffsetY: 0,
		HiddenGlobalOffsetZ: 0,
	}

	b.driver.EnqueueLaunchKernel(b.Queue,
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
		Work: b.DSource,
	}

	b.driver.EnqueueLaunchKernel(b.Queue,
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
	fst := make([]Float2, b.NFfts<<6)
	snd := make([]Float2, b.NFfts<<6)
	for i := int64(0); i < (b.NFfts << 6); i++ {
		fst[i] = b.Source[i]
	}

	for i := int64(0); i < (b.NFfts << 6); i++ {
		snd[i] = b.Source[b.HalfNCmplx+i]
	}

	for i := int64(0); i < (b.NFfts << 6); i++ {
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

	for i := int64(0); i < b.HalfNCmplx; i++ {
		b.Source[i].X = (rng.Float32())*2 - 1
		b.Source[i].Y = (rng.Float32())*2 - 1
		b.Source[i+b.HalfNCmplx].X = b.Source[i].X
		b.Source[i+b.HalfNCmplx].Y = b.Source[i].Y
	}
}

func (b *Benchmark) Step() error {
	if err := b.Prepare(); err != nil {
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
				dst = b.Result
			}
			b.Readbacks[i].Command = b.driver.EnqueueMemCopyD2H(b.Queue, b.Readbacks[i].Source, binary.Size(dst))
		}
		b.Phase = 3
	case 3:
		for _, copy := range b.Readbacks {
			var dst any
			switch copy.Target {
			case 0:
				dst = b.Result
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
		b.Readbacks = nil
		b.Ended = true
	}
	return nil
}
