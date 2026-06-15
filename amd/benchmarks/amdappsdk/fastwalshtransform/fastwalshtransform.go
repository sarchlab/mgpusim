// Package fastwalshtransform implements the fastwalshtransform benchmark from
// AMDAPPSDK.
package fastwalshtransform

import (
	"fmt"
	"log"
	"math/rand"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/arch"
	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// GCN3KernelArgs defines kernel arguments for GCN3 architecture
type GCN3KernelArgs struct {
	TArray driver.Ptr
	Step   uint32
}

// CDNA3KernelArgs defines kernel arguments for CDNA3 architecture (GFX942)
type CDNA3KernelArgs struct {
	TArray driver.Ptr
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
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue
	kernel  *insts.KernelCodeObject

	Arch           arch.Type
	Length         uint32
	hInputArray    []float32
	hVerInputArray []float32
	dInputArray    driver.Ptr

	useUnifiedMemory bool
}

// NewBenchmark returns a benchmark
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	return b
}

// SelectGPU selects GPU
func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
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
	b.useUnifiedMemory = true
}

// Run runs
func (b *Benchmark) Run() {
	b.loadProgram()

	for _, gpu := range b.gpus {
		b.driver.SelectGPU(b.context, gpu)
		b.queues = append(b.queues, b.driver.CreateCommandQueue(b.context))
	}

	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	rand.Seed(123)

	b.hInputArray = make([]float32, b.Length)
	b.hVerInputArray = make([]float32, b.Length)

	for i := uint32(0); i < b.Length; i++ {
		temp := rand.Float32() + float32(rand.Int31n(255))
		b.hInputArray[i] = temp
		b.hVerInputArray[i] = temp
	}

	if b.useUnifiedMemory {
		b.dInputArray = b.driver.AllocateUnifiedMemory(b.context, uint64(b.Length*4))
	} else {
		b.dInputArray = b.driver.AllocateMemory(b.context, uint64(b.Length*4))
	}

	b.driver.MemCopyH2D(b.context, b.dInputArray, b.hInputArray)
}

func printArray(array []float32, n uint32) {
	for i := uint32(0); i < n; i++ {
		fmt.Printf("%f ", array[i])
	}
}

func (b *Benchmark) exec() {
	globalThreadSize := b.Length / 2
	localThreadSize := uint16(256)

	for _, queue := range b.queues {
		for step := uint32(1); step < b.Length; step <<= 1 {
			if b.Arch == arch.CDNA3 {
				wgSizeX := localThreadSize
				wgSizeY := uint16(1)
				wgSizeZ := uint16(1)

				kernArg := CDNA3KernelArgs{
					TArray: b.dInputArray,
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

				b.driver.EnqueueLaunchKernel(
					queue,
					b.kernel,
					[3]uint32{globalThreadSize, 1, 1},
					[3]uint16{localThreadSize, 1, 1},
					&kernArg,
				)
			} else {
				kernArg := GCN3KernelArgs{
					TArray: b.dInputArray,
					Step:   step,
				}

				b.driver.EnqueueLaunchKernel(
					queue,
					b.kernel,
					[3]uint32{globalThreadSize, 1, 1},
					[3]uint16{localThreadSize, 1, 1},
					&kernArg,
				)
			}
		}
	}

	for _, q := range b.queues {
		b.driver.DrainCommandQueue(q)
	}

	b.driver.MemCopyD2H(b.context, b.hInputArray, b.dInputArray)
}

// Verify verifies
func (b *Benchmark) Verify() {
	for step := uint32(1); step < b.Length; step <<= 1 {
		jump := step << 1
		for group := uint32(0); group < step; group++ {
			for pair := group; pair < b.Length; pair += jump {
				match := pair + step

				T1 := b.hVerInputArray[pair]
				T2 := b.hVerInputArray[match]

				b.hVerInputArray[pair] = T1 + T2
				b.hVerInputArray[match] = T1 - T2
			}
		}
	}

	for i := uint32(0); i < b.Length; i++ {
		if b.hInputArray[i] != b.hVerInputArray[i] {
			panic(fmt.Sprintf("Mismatch at %d, expected %f found %f",
				i, b.hInputArray[i], b.hVerInputArray[i]))
		}
	}

	log.Printf("Passed!\n")
}
