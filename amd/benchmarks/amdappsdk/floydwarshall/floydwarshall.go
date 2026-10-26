// Package floydwarshall implements the Floyd-Warshall benchmark from AMDAPPSDK.
package floydwarshall

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

// GCN3KernelArgs defines kernel arguments for GCN3 architecture
type GCN3KernelArgs struct {
	OutputPathDistanceMatrix driver.Ptr
	OutputPathMatrix         driver.Ptr
	NumNodes                 uint32
	Pass                     uint32
}

// CDNA3KernelArgs defines kernel arguments for CDNA3 architecture (GFX942)
type CDNA3KernelArgs struct {
	OutputPathDistanceMatrix driver.Ptr
	OutputPathMatrix         driver.Ptr
	NumNodes                 uint32
	Pass                     uint32
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
	benchmarks.Saved[checkpointConfig, State]

	driver *driver.Runtime

	kernel *insts.KernelCodeObject
}

// NewBenchmark creates a new benchmark
func NewBenchmark(driver *driver.Runtime) *Benchmark {
	b := new(Benchmark)
	b.State = &State{}
	b.Saved = benchmarks.NewSaved(b.State,
		b.checkpointConfig, b.bind)

	b.driver = driver
	b.Context = driver.Init()
	return b
}

// SelectGPU selects gpu
func (b *Benchmark) SelectGPU(gpus []int) {
	b.Gpus = gpus
}

// SetUnifiedMemory Use Unified Memory
func (b *Benchmark) SetUnifiedMemory() {
	b.UseUnifiedMemory = true
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
	b.kernel = insts.LoadKernelCodeObjectFromBytes(hsacoBytes, "floydWarshallPass")
	if b.kernel == nil {
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
		b.Queues = append(b.Queues, b.driver.CreateCommandQueue(b.Context))
	}

	if b.NumIterations == 0 || b.NumIterations > b.NumNodes {
		b.NumIterations = b.NumNodes
	}

	b.initMem()
}

func (b *Benchmark) initMem() {
	// Use a local random source so the input is reproducible. rand.Seed has
	// been a no-op since Go 1.24, so seeding the global generator no longer
	// produces a deterministic sequence.
	rng := rand.New(rand.NewSource(1))

	numNodes := b.NumNodes
	b.HOutputPathMatrix = make([]uint32, numNodes*numNodes)
	b.HOutputPathDistanceMatrix = make([]uint32, numNodes*numNodes)

	for i := uint32(0); i < numNodes; i++ {
		for j := uint32(0); j < i; j++ {
			temp := uint32(rng.Int31n(10))
			b.HOutputPathDistanceMatrix[i*numNodes+j] = temp
			b.HOutputPathDistanceMatrix[j*numNodes+i] = temp
		}
	}

	for i := uint32(0); i < numNodes; i++ {
		iXWidth := i * numNodes
		b.HOutputPathDistanceMatrix[iXWidth+i] = 0
	}

	for i := uint32(0); i < numNodes; i++ {
		for j := uint32(0); j < i; j++ {
			b.HOutputPathMatrix[i*numNodes+j] = i
			b.HOutputPathMatrix[j*numNodes+i] = j
		}
		b.HOutputPathMatrix[i*numNodes+i] = i
	}

	b.HVerificationPathMatrix = make([]uint32, numNodes*numNodes)
	b.HVerificationPathDistanceMatrix = make([]uint32, numNodes*numNodes)

	copy(b.HVerificationPathDistanceMatrix, b.HOutputPathDistanceMatrix)
	copy(b.HVerificationPathMatrix, b.HOutputPathMatrix)

	if b.UseUnifiedMemory {
		b.DOutputPathMatrix = b.driver.AllocateUnifiedMemory(
			b.Context,
			uint64(numNodes*numNodes*4))
		b.DOutputPathDistanceMatrix = b.driver.AllocateUnifiedMemory(
			b.Context,
			uint64(numNodes*numNodes*4))
	} else {
		b.DOutputPathMatrix = b.driver.AllocateMemory(
			b.Context,
			uint64(numNodes*numNodes*4))
		b.DOutputPathDistanceMatrix = b.driver.AllocateMemory(
			b.Context,
			uint64(numNodes*numNodes*4))
	}

	b.driver.EnqueueMemCopyH2D(b.Queue, b.DOutputPathMatrix, b.HOutputPathMatrix)
	b.driver.EnqueueMemCopyH2D(b.Queue, b.DOutputPathDistanceMatrix, b.HOutputPathDistanceMatrix)
}

func printMatrix(matrix []uint32, n uint32) {
	for i := uint32(0); i < n; i++ {
		for j := uint32(0); j < n; j++ {
			fmt.Printf("%d ", matrix[i*n+j])
		}
		fmt.Printf("\n")
	}
}

func (b *Benchmark) launchKernelIteration(numNodes, blockSize, pass uint32) {
	if b.Arch == arch.CDNA3 {
		wgSizeX := uint16(blockSize)
		wgSizeY := uint16(blockSize)
		wgSizeZ := uint16(1)

		kernArg := CDNA3KernelArgs{
			OutputPathDistanceMatrix: b.DOutputPathDistanceMatrix,
			OutputPathMatrix:         b.DOutputPathMatrix,
			NumNodes:                 numNodes,
			Pass:                     pass,
			// Hidden kernel arguments for GFX942
			HiddenBlockCountX:   numNodes / uint32(wgSizeX),
			HiddenBlockCountY:   numNodes / uint32(wgSizeY),
			HiddenBlockCountZ:   1,
			HiddenGroupSizeX:    wgSizeX,
			HiddenGroupSizeY:    wgSizeY,
			HiddenGroupSizeZ:    wgSizeZ,
			HiddenRemainderX:    uint16(numNodes % uint32(wgSizeX)),
			HiddenRemainderY:    uint16(numNodes % uint32(wgSizeY)),
			HiddenRemainderZ:    0,
			HiddenGlobalOffsetX: 0,
			HiddenGlobalOffsetY: 0,
			HiddenGlobalOffsetZ: 0,
			HiddenGridDims:      2,
		}

		b.driver.EnqueueLaunchKernel(b.Queue,
			b.kernel,
			[3]uint32{numNodes, numNodes, 1},
			[3]uint16{uint16(blockSize), uint16(blockSize), 1},
			&kernArg,
		)
	} else {
		kernArg := GCN3KernelArgs{
			OutputPathDistanceMatrix: b.DOutputPathDistanceMatrix,
			OutputPathMatrix:         b.DOutputPathMatrix,
			NumNodes:                 numNodes,
			Pass:                     pass,
		}

		b.driver.EnqueueLaunchKernel(b.Queue,
			b.kernel,
			[3]uint32{numNodes, numNodes, 1},
			[3]uint16{uint16(blockSize), uint16(blockSize), 1},
			&kernArg,
		)
	}
}

func (b *Benchmark) exec() {
	numNodes := b.NumNodes
	blockSize := uint32(8)

	if numNodes%blockSize != 0 {
		numNodes = (numNodes/blockSize + 1) * blockSize
	}

	for k := uint32(0); k < b.NumIterations; k++ {
		b.launchKernelIteration(numNodes, blockSize, k)
	}

	b.Readbacks = append(b.Readbacks, benchmarks.Readback{Source: b.DOutputPathMatrix, Target: 0})
	b.Readbacks = append(b.Readbacks, benchmarks.Readback{Source: b.DOutputPathDistanceMatrix, Target: 1})
}

// Verify verifies
func (b *Benchmark) verify() error {
	numNodes := b.NumNodes
	var distanceYtoX, distanceYtoK, distanceKtoX, indirectDistance uint32
	width := numNodes
	var yXwidth uint32

	for k := uint32(0); k < b.NumIterations; k++ {
		for y := uint32(0); y < numNodes; y++ {
			yXwidth = y * numNodes
			for x := uint32(0); x < numNodes; x++ {
				distanceYtoX = b.HVerificationPathDistanceMatrix[yXwidth+x]
				distanceYtoK = b.HVerificationPathDistanceMatrix[yXwidth+k]
				distanceKtoX = b.HVerificationPathDistanceMatrix[k*width+x]

				indirectDistance = distanceYtoK + distanceKtoX

				if indirectDistance < distanceYtoX {
					b.HVerificationPathDistanceMatrix[yXwidth+x] = indirectDistance
					b.HVerificationPathMatrix[yXwidth+x] = k
				}
			}
		}
	}

	n := numNodes
	for i := uint32(0); i < n; i++ {
		for j := uint32(0); j < n; j++ {
			if b.HOutputPathMatrix[i*n+j] != b.HVerificationPathMatrix[i*n+j] {
				return fmt.Errorf("%s", fmt.Sprint(fmt.Sprintf("Mismatch at row %d col %d, expected %d got %d", i, j,
					b.HVerificationPathMatrix[i*n+j],
					b.HOutputPathMatrix[i*n+j])))
			}
			if b.HOutputPathDistanceMatrix[i*n+j] != b.HVerificationPathDistanceMatrix[i*n+j] {
				return fmt.Errorf("%s", fmt.Sprint(fmt.Sprintf("Mismatch at row %d col %d, expected %d got %d", i, j,
					b.HVerificationPathDistanceMatrix[i*n+j],
					b.HOutputPathDistanceMatrix[i*n+j])))
			}
		}
	}

	log.Printf("Passed!\n")

	return nil
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
				dst = b.HOutputPathMatrix
			case 1:
				dst = b.HOutputPathDistanceMatrix
			}
			b.Readbacks[i].Command = b.driver.EnqueueMemCopyD2H(b.Queue, b.Readbacks[i].Source, binary.Size(dst))
		}
		b.Phase = 3
	case 3:
		for _, copy := range b.Readbacks {
			var dst any
			switch copy.Target {
			case 0:
				dst = b.HOutputPathMatrix
			case 1:
				dst = b.HOutputPathDistanceMatrix
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
