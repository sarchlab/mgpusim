// Package fastwalshtransform implements the fastwalshtransform benchmark from
// AMDAPPSDK.
package fastwalshtransform

import (
	// embed hsaco files
	_ "embed"
	"fmt"
	"log"
	"math/rand"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

// GCN3KernelArgs defines kernel arguments for GCN3 architecture
type GCN3KernelArgs struct {
	TArray gpuruntime.Ptr
	Step   uint32
}

// CDNA3KernelArgs defines kernel arguments for CDNA3 architecture (GFX942)
type CDNA3KernelArgs struct {
	TArray gpuruntime.Ptr
	Step   uint32
	// Padding to align hidden args to next 4-byte boundary
	Pad uint32
	// Hidden kernel arguments (required by HIP runtime for GFX942)
	HiddenBlockCountX   uint32   // number of workgroups in X
	HiddenBlockCountY   uint32   // number of workgroups in Y
	HiddenBlockCountZ   uint32   // number of workgroups in Z
	HiddenGroupSizeX    uint16   // workgroup size X
	HiddenGroupSizeY    uint16   // workgroup size Y
	HiddenGroupSizeZ    uint16   // workgroup size Z
	HiddenRemainderX    uint16   // grid size % workgroup size X
	HiddenRemainderY    uint16   // grid size % workgroup size Y
	HiddenRemainderZ    uint16   // grid size % workgroup size Z
	Padding             [16]byte // reserved
	HiddenGlobalOffsetX int64    // global offset X
	HiddenGlobalOffsetY int64    // global offset Y
	HiddenGlobalOffsetZ int64    // global offset Z
	HiddenGridDims      uint16   // grid dimensions
}

// Benchmark defines a benchmark
type Benchmark struct {
	*State
	benchmarks.Checkpoint[checkpointConfig, State]

	rt *gpuruntime.Runtime

	kernel *insts.KernelCodeObject
}

// NewBenchmark returns a benchmark
func NewBenchmark(rt *gpuruntime.Runtime) *Benchmark {
	b := new(Benchmark)
	b.State = &State{}
	b.Checkpoint = benchmarks.NewCheckpoint(b.State, b.checkpointConfig, b.bind)

	b.rt = rt
	b.Context = rt.Init()
	return b
}

// SelectGPU selects GPU
func (b *Benchmark) SelectGPU(gpus []int) {
	b.Gpus = gpus
}

//go:embed kernels.hsaco
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
	b.kernel = insts.LoadKernelCodeObjectFromBytes(hsacoBytes, "fastWalshTransform")
	if b.kernel == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// SetUnifiedMemory uses Unified Memory
func (b *Benchmark) SetUnifiedMemory() {
	b.UseUnifiedMemory = true
}

// Prepare the inputs for the first queued phase.
func (b *Benchmark) setup() {
	b.loadProgram()

	for _, gpu := range b.Gpus {
		b.rt.SelectGPU(b.Context, gpu)
		b.Queues = append(b.Queues, b.rt.CreateCommandQueue(b.Context))
	}

	b.initMem()
}

func (b *Benchmark) initMem() {
	rng := rand.New(rand.NewSource(123))

	b.HInputArray = make([]float32, b.Length)
	b.HVerInputArray = make([]float32, b.Length)

	for i := uint32(0); i < b.Length; i++ {
		temp := rng.Float32() + float32(rng.Int31n(255))
		b.HInputArray[i] = temp
		b.HVerInputArray[i] = temp
	}

	if b.UseUnifiedMemory {
		b.DInputArray = b.rt.AllocateUnifiedMemory(b.Context, uint64(b.Length*4))
	} else {
		b.DInputArray = b.rt.AllocateMemory(b.Context, uint64(b.Length*4))
	}

	b.Upload(b.rt, b.Queues[0], b.DInputArray, b.HInputArray)
}

func printArray(array []float32, n uint32) {
	for i := uint32(0); i < n; i++ {
		fmt.Printf("%f ", array[i])
	}
}

func (b *Benchmark) execStage(queue gpuruntime.CommandQueue, step uint32) {
	globalThreadSize := b.Length / 2
	localThreadSize := uint16(256)

	if b.Arch == arch.CDNA3 {
		wgSizeX := localThreadSize
		wgSizeY := uint16(1)
		wgSizeZ := uint16(1)

		kernArg := CDNA3KernelArgs{
			TArray: b.DInputArray,
			Step:   step,
			// Hidden kernel arguments for GFX942
			HiddenBlockCountX:   globalThreadSize / uint32(wgSizeX),
			HiddenBlockCountY:   1,
			HiddenBlockCountZ:   1,
			HiddenGroupSizeX:    wgSizeX,
			HiddenGroupSizeY:    wgSizeY,
			HiddenGroupSizeZ:    wgSizeZ,
			HiddenRemainderX:    uint16(globalThreadSize % uint32(wgSizeX)),
			HiddenRemainderY:    0,
			HiddenRemainderZ:    0,
			HiddenGlobalOffsetX: 0,
			HiddenGlobalOffsetY: 0,
			HiddenGlobalOffsetZ: 0,
			HiddenGridDims:      1,
		}

		b.rt.EnqueueLaunchKernel(
			queue,
			b.kernel,
			[3]uint32{globalThreadSize, 1, 1},
			[3]uint16{localThreadSize, 1, 1},
			&kernArg,
		)
	} else {
		kernArg := GCN3KernelArgs{
			TArray: b.DInputArray,
			Step:   step,
		}

		b.rt.EnqueueLaunchKernel(
			queue,
			b.kernel,
			[3]uint32{globalThreadSize, 1, 1},
			[3]uint16{localThreadSize, 1, 1},
			&kernArg,
		)
	}
}

// Verify verifies
func (b *Benchmark) verify() error {
	for step := uint32(1); step < b.Length; step <<= 1 {
		jump := step << 1
		for group := uint32(0); group < step; group++ {
			for pair := group; pair < b.Length; pair += jump {
				match := pair + step

				T1 := b.HVerInputArray[pair]
				T2 := b.HVerInputArray[match]

				b.HVerInputArray[pair] = T1 + T2
				b.HVerInputArray[match] = T1 - T2
			}
		}
	}

	for i := uint32(0); i < b.Length; i++ {
		if b.HInputArray[i] != b.HVerInputArray[i] {
			return fmt.Errorf("%s", fmt.Sprint(fmt.Sprintf("Mismatch at %d, expected %f found %f",
				i, b.HInputArray[i], b.HVerInputArray[i])))
		}
	}

	log.Printf("Passed!\n")

	return nil
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
		for _, queue := range b.Queues {
			for step := uint32(1); step < b.Length; step <<= 1 {
				b.execStage(queue, step)
			}
		}
		b.Phase = 2
	case 2:
		b.Readback = benchmarks.Download(b.rt, b.Queues[0], b.DInputArray, int(b.Length)*4)
		b.Phase = 3
	case 3:
		if err := benchmarks.Read(b.rt, b.Readback, b.HInputArray); err != nil {
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
