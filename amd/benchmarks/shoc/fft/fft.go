// Package fft include the benchmark of Fourier
package fft

import (
	// embed hsaco files
	_ "embed"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"math/rand"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

// Float2 is two floats
type Float2 struct {
	X, Y float32
}

// KernelArgs defines kernel arguments for GCN3
type KernelArgs struct {
	Work                gpuruntime.Ptr
	Smem                gpuruntime.LocalPtr
	Paddinng            int32
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

type nativeFFTArgs struct {
	Work   gpuruntime.Ptr
	Launch benchmarks.GFX942LaunchDimensions
}

type Double2 struct{ X, Y float64 }

// Benchmark defines a benchmark
type Benchmark struct {
	*State
	benchmarks.Checkpoint[State]
	rt *gpuruntime.Runtime

	fftKernel, inverseKernel *insts.KernelCodeObject
}

// NewBenchmark returns a benchmark
func NewBenchmark(rt *gpuruntime.Runtime) *Benchmark {
	b := new(Benchmark)
	b.State = &State{}
	b.Checkpoint = benchmarks.NewCheckpoint(b.State, b.bind)

	b.rt = rt
	b.Context = rt.Init()
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
	if b.Arch != arch.CDNA3 {
		b.fftKernel = insts.LoadKernelCodeObjectFromBytes(gcn3HSACOBytes, "fft1D_512")
	} else {
		forward := "_Z13FFT512_deviceI15HIP_vector_typeIfLj2EEfEvPT_"
		inverse := "_Z14IFFT512_deviceI15HIP_vector_typeIfLj2EEfEvPT_"
		if b.Precision == "double" {
			forward = "_Z13FFT512_deviceI15HIP_vector_typeIdLj2EEdEvPT_"
			inverse = "_Z14IFFT512_deviceI15HIP_vector_typeIdLj2EEdEvPT_"
		}
		b.fftKernel = insts.LoadKernelCodeObjectFromBytes(cdna3HSACOBytes, forward)
		b.inverseKernel = insts.LoadKernelCodeObjectFromBytes(cdna3HSACOBytes, inverse)
	}
	if b.fftKernel == nil || (b.Arch == arch.CDNA3 && b.inverseKernel == nil) {
		log.Panic("failed to load the FFT kernel")
	}
}

// Prepare the inputs for the first queued phase.
func (b *Benchmark) setup() {
	b.rt.SelectGPU(b.Context, b.Gpus[len(b.Gpus)-1])
	b.Queue = b.rt.CreateCommandQueue(b.Context)

	b.loadProgram()

	b.initMem()
}

func (b *Benchmark) initMem() {
	if b.Arch == arch.CDNA3 {
		b.initNativeFFT()
		return
	}
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
		b.DSource = b.rt.AllocateUnifiedMemory(b.Context,
			b.UsedBytes)
	} else {
		b.DSource = b.rt.AllocateMemory(b.Context,
			b.UsedBytes)
	}
	b.Upload(b.rt, b.Queue, b.DSource, b.Source)
}

func (b *Benchmark) initNativeFFT() {
	if b.Precision != "" && b.Precision != "float" && b.Precision != "double" {
		panic("FFT precision must be float or double")
	}
	elementBytes := int64(8)
	if b.Precision == "double" {
		elementBytes = 16
	}
	b.NFfts = b.Transforms
	if b.NFfts == 0 {
		length := b.Bytes
		if !b.BytesMode {
			length *= 1024 * 1024
		}
		b.NFfts = length / (512 * elementBytes)
	}
	if b.NFfts <= 0 {
		panic("FFT configuration must contain at least one complete transform")
	}
	count := b.NFfts * 512
	b.UsedBytes = uint64(count * elementBytes)
	alloc := b.rt.AllocateMemory
	if b.UseUnifiedMemory {
		alloc = b.rt.AllocateUnifiedMemory
	}
	b.DSource = alloc(b.Context, b.UsedBytes)
	rand := benchmarks.NewGlibcRand(42)
	next := func() float32 { return float32(rand.Next())/2147483647*2 - 1 }
	if b.Precision == "double" {
		b.DoubleSource, b.DoubleResult = make([]Double2, count), make([]Double2, count)
		for i := range b.DoubleSource {
			b.DoubleSource[i] = Double2{float64(next()), float64(next())}
		}
		b.Upload(b.rt, b.Queue, b.DSource, b.DoubleSource)
		return
	}
	b.Source, b.Result = make([]Float2, count), make([]Float2, count)
	for i := range b.Source {
		b.Source[i] = Float2{next(), next()}
	}
	b.Upload(b.rt, b.Queue, b.DSource, b.Source)
}

func (b *Benchmark) launchNativeFFT() {
	slices := int64(1)
	for b.NFfts/slices > 65535 {
		slices *= 2
	}
	grid := [3]uint32{uint32(b.NFfts / slices * 64), uint32(slices), 1}
	block := [3]uint16{64, 1, 1}
	args := nativeFFTArgs{Work: b.DSource, Launch: benchmarks.NewGFX942LaunchDimensions(grid, block)}
	for k := int32(0); k < b.Passes; k++ {
		b.rt.EnqueueLaunchKernel(b.Queue, b.fftKernel, grid, block, &args)
		b.rt.EnqueueLaunchKernel(b.Queue, b.inverseKernel, grid, block, &args)
	}
}

func (b *Benchmark) exec() {
	if b.Arch == arch.CDNA3 {
		b.launchNativeFFT()
	} else {
		local := [3]uint16{64, 1, 1}
		grid := [3]uint32{uint32(64 * b.NFfts), 1, 1}
		for k := int32(0); k < b.Passes; k++ {
			b.execGCN3(grid, local)
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

	b.rt.EnqueueLaunchKernel(b.Queue,
		b.fftKernel,
		globalSize, localSize,
		&args,
	)
}

func (b *Benchmark) verifyRoundTrip() error {
	passed := func(dx, dy float64) bool {
		errorSum := math.Abs(dx) + math.Abs(dy)
		return !math.IsNaN(errorSum) && errorSum < 1e-2
	}
	if b.Precision == "double" {
		for i, gpu := range b.DoubleResult {
			source := b.DoubleSource[i]
			if !passed(gpu.X-source.X, gpu.Y-source.Y) {
				return fmt.Errorf("double FFT round-trip failed at sample %d", i)
			}
		}
	} else {
		for i, gpu := range b.Result {
			source := b.Source[i]
			if !passed(float64(gpu.X)-float64(source.X), float64(gpu.Y)-float64(source.Y)) {
				return fmt.Errorf("FFT round-trip failed at sample %d", i)
			}
		}
	}
	log.Printf("Passed!\n")
	return nil
}

func (b *Benchmark) verify() error {
	if b.Arch == arch.CDNA3 {
		return b.verifyRoundTrip()
	}
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
	if err := b.ReleaseUploads(b.rt); err != nil {
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
				if b.Arch == arch.CDNA3 && b.Precision == "double" {
					dst = b.DoubleResult
				}
			}
			b.Readbacks[i].Buffer = benchmarks.Download(b.rt, b.Queue, b.Readbacks[i].Source, binary.Size(dst))
		}
		b.Phase = 3
	case 3:
		for _, copy := range b.Readbacks {
			var dst any
			switch copy.Target {
			case 0:
				dst = b.Result
				if b.Arch == arch.CDNA3 && b.Precision == "double" {
					dst = b.DoubleResult
				}
			}
			if err := benchmarks.Read(b.rt, copy.Buffer, dst); err != nil {
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
