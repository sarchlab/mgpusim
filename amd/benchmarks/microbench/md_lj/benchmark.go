// Package mdlj implements the SHOC MD Lennard-Jones molecular dynamics benchmark.
package mdlj

import (
	"log"
	"math"
	"math/rand"

	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// KernelArgs defines kernel arguments for md_lj_kernel.
type KernelArgs struct {
	PosX         driver.Ptr
	PosY         driver.Ptr
	PosZ         driver.Ptr
	Energy       driver.Ptr
	NeighborList driver.Ptr
	NumNeighbors driver.Ptr
	N            int32
	MaxNeighbors int32
}

// Benchmark defines the MD Lennard-Jones benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	kernel *insts.KernelCodeObject

	NumAtoms     int
	MaxNeighbors int
	BlockSize    int

	useUnifiedMemory bool

	hPosX         []float32
	hPosY         []float32
	hPosZ         []float32
	hEnergy       []float32
	hNeighborList []int32
	hNumNeighbors []int32

	dPosX         driver.Ptr
	dPosY         driver.Ptr
	dPosZ         driver.Ptr
	dEnergy       driver.Ptr
	dNeighborList driver.Ptr
	dNumNeighbors driver.Ptr
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

// NewBenchmark creates a new MD Lennard-Jones benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.NumAtoms = 4096
	b.MaxNeighbors = 128
	b.BlockSize = 256
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

// SetNumElements sets the number of atoms.
func (b *Benchmark) SetNumElements(n int) {
	b.NumAtoms = n
}

func (b *Benchmark) loadProgram() {
	b.kernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "md_lj_kernel")
	if b.kernel == nil {
		log.Panic("Failed to load md_lj_kernel")
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
	n := b.NumAtoms
	maxN := b.MaxNeighbors
	rng := rand.New(rand.NewSource(42))

	// Initialize random atom positions in a box
	b.hPosX = make([]float32, n)
	b.hPosY = make([]float32, n)
	b.hPosZ = make([]float32, n)
	b.hEnergy = make([]float32, n)

	boxSize := float32(math.Cbrt(float64(n)) * 2.0)
	for i := 0; i < n; i++ {
		b.hPosX[i] = rng.Float32() * boxSize
		b.hPosY[i] = rng.Float32() * boxSize
		b.hPosZ[i] = rng.Float32() * boxSize
	}

	// Build neighbor lists: each atom gets random neighbors
	b.hNeighborList = make([]int32, n*maxN)
	b.hNumNeighbors = make([]int32, n)
	for i := 0; i < n; i++ {
		nNeigh := maxN / 2
		if nNeigh > n-1 {
			nNeigh = n - 1
		}
		b.hNumNeighbors[i] = int32(nNeigh)
		for k := 0; k < nNeigh; k++ {
			j := rng.Intn(n)
			for j == i {
				j = rng.Intn(n)
			}
			b.hNeighborList[i*maxN+k] = int32(j)
		}
	}

	// Allocate GPU memory
	floatBytes := uint64(n * 4)
	neighborBytes := uint64(n * maxN * 4)
	countBytes := uint64(n * 4)

	if b.useUnifiedMemory {
		b.dPosX = b.driver.AllocateUnifiedMemory(b.context, floatBytes)
		b.dPosY = b.driver.AllocateUnifiedMemory(b.context, floatBytes)
		b.dPosZ = b.driver.AllocateUnifiedMemory(b.context, floatBytes)
		b.dEnergy = b.driver.AllocateUnifiedMemory(b.context, floatBytes)
		b.dNeighborList = b.driver.AllocateUnifiedMemory(b.context, neighborBytes)
		b.dNumNeighbors = b.driver.AllocateUnifiedMemory(b.context, countBytes)
	} else {
		b.dPosX = b.driver.AllocateMemory(b.context, floatBytes)
		b.dPosY = b.driver.AllocateMemory(b.context, floatBytes)
		b.dPosZ = b.driver.AllocateMemory(b.context, floatBytes)
		b.dEnergy = b.driver.AllocateMemory(b.context, floatBytes)
		b.dNeighborList = b.driver.AllocateMemory(b.context, neighborBytes)
		b.dNumNeighbors = b.driver.AllocateMemory(b.context, countBytes)
	}

	b.driver.MemCopyH2D(b.context, b.dPosX, b.hPosX)
	b.driver.MemCopyH2D(b.context, b.dPosY, b.hPosY)
	b.driver.MemCopyH2D(b.context, b.dPosZ, b.hPosZ)
	b.driver.MemCopyH2D(b.context, b.dNeighborList, b.hNeighborList)
	b.driver.MemCopyH2D(b.context, b.dNumNeighbors, b.hNumNeighbors)
}

func (b *Benchmark) exec() {
	n := b.NumAtoms
	blockSize := b.BlockSize
	grid := (n + blockSize - 1) / blockSize

	globalSize := [3]uint32{uint32(grid * blockSize), 1, 1}
	localSize := [3]uint16{uint16(blockSize), 1, 1}

	args := KernelArgs{
		PosX:         b.dPosX,
		PosY:         b.dPosY,
		PosZ:         b.dPosZ,
		Energy:       b.dEnergy,
		NeighborList: b.dNeighborList,
		NumNeighbors: b.dNumNeighbors,
		N:            int32(n),
		MaxNeighbors: int32(b.MaxNeighbors),
	}
	b.driver.LaunchKernel(b.context, b.kernel,
		globalSize, localSize, &args)

	b.driver.MemCopyD2H(b.context, b.hEnergy, b.dEnergy)
}

// Verify verifies the result.
func (b *Benchmark) Verify() {
	n := b.NumAtoms
	maxN := b.MaxNeighbors
	errors := 0

	for i := 0; i < n; i++ {
		xi := float64(b.hPosX[i])
		yi := float64(b.hPosY[i])
		zi := float64(b.hPosZ[i])

		eSum := 0.0
		nNeigh := int(b.hNumNeighbors[i])
		base := i * maxN

		for k := 0; k < nNeigh; k++ {
			j := int(b.hNeighborList[base+k])
			dx := xi - float64(b.hPosX[j])
			dy := yi - float64(b.hPosY[j])
			dz := zi - float64(b.hPosZ[j])

			r2 := dx*dx + dy*dy + dz*dz
			r2inv := 1.0 / r2
			r6inv := r2inv * r2inv * r2inv
			r12inv := r6inv * r6inv

			eSum += 4.0 * (r12inv - r6inv)
		}

		expected := float32(eSum)
		got := b.hEnergy[i]
		diff := math.Abs(float64(got - expected))
		tol := 1e-1*math.Abs(float64(expected)) + 1e-3

		if diff > tol {
			if errors < 10 {
				log.Printf("Mismatch at atom %d: got %f, expected %f",
					i, got, expected)
			}
			errors++
		}
	}

	if errors > 0 {
		log.Panicf("FAIL: %d errors in md_lj verification", errors)
	}
	log.Printf("Passed!")
}
