// Package floydwarshall implements the Floyd-Warshall benchmark from AMDAPPSDK.
package floydwarshall

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
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue
	kernel  *insts.KernelCodeObject

	Arch                      arch.Type
	NumNodes                  uint32
	NumIterations             uint32
	hOutputPathMatrix         []uint32
	hOutputPathDistanceMatrix []uint32
	dOutputPathMatrix         driver.Ptr
	dOutputPathDistanceMatrix driver.Ptr

	hVerificationPathMatrix         []uint32
	hVerificationPathDistanceMatrix []uint32

	useUnifiedMemory bool
}

// NewBenchmark creates a new benchmark
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	return b
}

// SelectGPU selects gpu
func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
}

// SetUnifiedMemory Use Unified Memory
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
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

// Run runs the benchmark
func (b *Benchmark) Run() {
	b.loadProgram()

	for _, gpu := range b.gpus {
		b.driver.SelectGPU(b.context, gpu)
		b.queues = append(b.queues, b.driver.CreateCommandQueue(b.context))
	}

	if b.NumIterations == 0 || b.NumIterations > b.NumNodes {
		b.NumIterations = b.NumNodes
	}

	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	rand.Seed(1)

	numNodes := b.NumNodes
	b.hOutputPathMatrix = make([]uint32, numNodes*numNodes)
	b.hOutputPathDistanceMatrix = make([]uint32, numNodes*numNodes)

	for i := uint32(0); i < numNodes; i++ {
		for j := uint32(0); j < i; j++ {
			temp := uint32(rand.Int31n(10))
			b.hOutputPathDistanceMatrix[i*numNodes+j] = temp
			b.hOutputPathDistanceMatrix[j*numNodes+i] = temp
		}
	}

	for i := uint32(0); i < numNodes; i++ {
		iXWidth := i * numNodes
		b.hOutputPathDistanceMatrix[iXWidth+i] = 0
	}

	for i := uint32(0); i < numNodes; i++ {
		for j := uint32(0); j < i; j++ {
			b.hOutputPathMatrix[i*numNodes+j] = i
			b.hOutputPathMatrix[j*numNodes+i] = j
		}
		b.hOutputPathMatrix[i*numNodes+i] = i
	}

	b.hVerificationPathMatrix = make([]uint32, numNodes*numNodes)
	b.hVerificationPathDistanceMatrix = make([]uint32, numNodes*numNodes)

	copy(b.hVerificationPathDistanceMatrix, b.hOutputPathDistanceMatrix)
	copy(b.hVerificationPathMatrix, b.hOutputPathMatrix)

	if b.useUnifiedMemory {
		b.dOutputPathMatrix = b.driver.AllocateUnifiedMemory(
			b.context,
			uint64(numNodes*numNodes*4))
		b.dOutputPathDistanceMatrix = b.driver.AllocateUnifiedMemory(
			b.context,
			uint64(numNodes*numNodes*4))
	} else {
		b.dOutputPathMatrix = b.driver.AllocateMemory(
			b.context,
			uint64(numNodes*numNodes*4))
		b.dOutputPathDistanceMatrix = b.driver.AllocateMemory(
			b.context,
			uint64(numNodes*numNodes*4))
	}

	b.driver.MemCopyH2D(b.context, b.dOutputPathMatrix, b.hOutputPathMatrix)
	b.driver.MemCopyH2D(b.context, b.dOutputPathDistanceMatrix, b.hOutputPathDistanceMatrix)
}

func printMatrix(matrix []uint32, n uint32) {
	for i := uint32(0); i < n; i++ {
		for j := uint32(0); j < n; j++ {
			fmt.Printf("%d ", matrix[i*n+j])
		}
		fmt.Printf("\n")
	}
}

func (b *Benchmark) exec() {
	numNodes := b.NumNodes
	blockSize := uint32(8)

	if numNodes%blockSize != 0 {
		numNodes = (numNodes/blockSize + 1) * blockSize
	}

	for k := uint32(0); k < b.NumIterations; k++ {
		pass := k

		if b.Arch == arch.CDNA3 {
			wgSizeX := uint16(blockSize)
			wgSizeY := uint16(blockSize)
			wgSizeZ := uint16(1)

			kernArg := CDNA3KernelArgs{
				OutputPathDistanceMatrix: b.dOutputPathDistanceMatrix,
				OutputPathMatrix:         b.dOutputPathMatrix,
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

			b.driver.LaunchKernel(
				b.context,
				b.kernel,
				[3]uint32{numNodes, numNodes, 1},
				[3]uint16{uint16(blockSize), uint16(blockSize), 1},
				&kernArg,
			)
		} else {
			kernArg := GCN3KernelArgs{
				OutputPathDistanceMatrix: b.dOutputPathDistanceMatrix,
				OutputPathMatrix:         b.dOutputPathMatrix,
				NumNodes:                 numNodes,
				Pass:                     pass,
			}

			b.driver.LaunchKernel(
				b.context,
				b.kernel,
				[3]uint32{numNodes, numNodes, 1},
				[3]uint16{uint16(blockSize), uint16(blockSize), 1},
				&kernArg,
			)
		}
	}

	b.driver.MemCopyD2H(b.context, b.hOutputPathMatrix, b.dOutputPathMatrix)
	b.driver.MemCopyD2H(b.context,
		b.hOutputPathDistanceMatrix, b.dOutputPathDistanceMatrix)
}

// Verify verifies
func (b *Benchmark) Verify() {
	numNodes := b.NumNodes
	var distanceYtoX, distanceYtoK, distanceKtoX, indirectDistance uint32
	width := numNodes
	var yXwidth uint32

	for k := uint32(0); k < b.NumIterations; k++ {
		for y := uint32(0); y < numNodes; y++ {
			yXwidth = y * numNodes
			for x := uint32(0); x < numNodes; x++ {
				distanceYtoX = b.hVerificationPathDistanceMatrix[yXwidth+x]
				distanceYtoK = b.hVerificationPathDistanceMatrix[yXwidth+k]
				distanceKtoX = b.hVerificationPathDistanceMatrix[k*width+x]

				indirectDistance = distanceYtoK + distanceKtoX

				if indirectDistance < distanceYtoX {
					b.hVerificationPathDistanceMatrix[yXwidth+x] = indirectDistance
					b.hVerificationPathMatrix[yXwidth+x] = k
				}
			}
		}
	}

	n := numNodes
	for i := uint32(0); i < n; i++ {
		for j := uint32(0); j < n; j++ {
			if b.hOutputPathMatrix[i*n+j] != b.hVerificationPathMatrix[i*n+j] {
				panic(fmt.Sprintf("Mismatch at row %d col %d, expected %d got %d", i, j,
					b.hVerificationPathMatrix[i*n+j],
					b.hOutputPathMatrix[i*n+j]))
			}
			if b.hOutputPathDistanceMatrix[i*n+j] != b.hVerificationPathDistanceMatrix[i*n+j] {
				panic(fmt.Sprintf("Mismatch at row %d col %d, expected %d got %d", i, j,
					b.hVerificationPathDistanceMatrix[i*n+j],
					b.hOutputPathDistanceMatrix[i*n+j]))
			}
		}
	}

	log.Printf("Passed!\n")
}
