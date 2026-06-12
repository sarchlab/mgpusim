// Package cutcp implements the Parboil Cutoff Potential benchmark.
// Computes short-range electrostatic potential at grid points from atoms
// within a cutoff distance (direct Coulomb summation with cutoff).
package cutcp

import (
	"log"
	"math"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// Matches: cutoff_potential(DATA_TYPE *potential, Atom *atoms,
//
//	int num_atoms, int grid_size,
//	DATA_TYPE spacing, DATA_TYPE cutoff2)
//
// KernelArgs defines kernel arguments.
type KernelArgs struct {
	Potential           driver.Ptr
	Atoms               driver.Ptr
	NumAtoms            int32
	GridSize            int32
	Spacing             float32
	Cutoff2             float32
	HiddenBlockCountX   uint32
	HiddenBlockCountY   uint32
	HiddenBlockCountZ   uint32
	HiddenGroupSizeX    uint16
	HiddenGroupSizeY    uint16
	HiddenGroupSizeZ    uint16
	HiddenRemainderX    uint16
	HiddenRemainderY    uint16
	HiddenRemainderZ    uint16
	Pad0                [16]byte
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
	HiddenGridDims      uint16
}

// Benchmark defines the Cutoff Potential benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	cutcpKernel *insts.KernelCodeObject

	NumAtoms  int
	GridSize  int
	Spacing   float32
	Cutoff    float32
	BlockSize int

	useUnifiedMemory bool

	hAtoms     []float32 // 4 floats per atom: x, y, z, charge
	hPotential []float32
	hRef       []float32

	dAtoms     driver.Ptr
	dPotential driver.Ptr
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

// NewBenchmark creates a new cutcp benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.NumAtoms = 4096
	b.GridSize = 32
	b.Spacing = 0.5
	b.Cutoff = 1.2
	b.BlockSize = 256
	return b
}

// SelectGPU selects GPU.
func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
}

// SetUnifiedMemory uses Unified Memory.
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

func (b *Benchmark) loadProgram() {
	b.cutcpKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "cutoff_potential")
	if b.cutcpKernel == nil {
		log.Panic("Failed to load cutoff_potential binary")
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
	b.initHostData()
	b.allocGPUMem()

	b.driver.MemCopyH2D(b.context, b.dAtoms, b.hAtoms)
	b.driver.MemCopyH2D(b.context, b.dPotential, b.hPotential)
}

func (b *Benchmark) initHostData() {
	gridExtent := float32(b.GridSize-1) * b.Spacing

	// Initialize atoms: 4 floats per atom (x, y, z, charge)
	b.hAtoms = make([]float32, b.NumAtoms*4)
	for i := 0; i < b.NumAtoms; i++ {
		b.hAtoms[i*4+0] = float32(((i*7+13)%10000)%10000) / 10000.0 * gridExtent
		b.hAtoms[i*4+1] = float32(((i*11+37)%10000)%10000) / 10000.0 * gridExtent
		b.hAtoms[i*4+2] = float32(((i*13+59)%10000)%10000) / 10000.0 * gridExtent
		b.hAtoms[i*4+3] = float32(((i*17+83)%2000))/1000.0 - 1.0 // [-1, 1]
	}

	// Initialize potential grid to zero
	totalPoints := b.GridSize * b.GridSize * b.GridSize
	b.hPotential = make([]float32, totalPoints)
	b.hRef = make([]float32, totalPoints)
}

func (b *Benchmark) allocGPUMem() {
	bytesAtoms := uint64(b.NumAtoms*4) * 4
	totalPoints := b.GridSize * b.GridSize * b.GridSize
	bytesPotential := uint64(totalPoints) * 4

	if b.useUnifiedMemory {
		b.dAtoms = b.driver.AllocateUnifiedMemory(b.context, bytesAtoms)
		b.dPotential = b.driver.AllocateUnifiedMemory(b.context, bytesPotential)
	} else {
		b.dAtoms = b.driver.AllocateMemory(b.context, bytesAtoms)
		b.dPotential = b.driver.AllocateMemory(b.context, bytesPotential)
	}
}

func (b *Benchmark) exec() {
	totalPoints := b.GridSize * b.GridSize * b.GridSize
	numBlocks := (totalPoints + b.BlockSize - 1) / b.BlockSize

	globalSize := [3]uint32{
		uint32(numBlocks * b.BlockSize),
		1,
		1,
	}
	localSize := [3]uint16{
		uint16(b.BlockSize),
		1,
		1,
	}

	cutoff2 := b.Cutoff * b.Cutoff

	b.launch(globalSize, localSize, cutoff2)

	b.driver.MemCopyD2H(b.context, b.hPotential, b.dPotential)
}

func (b *Benchmark) launch(
	globalSize [3]uint32, localSize [3]uint16, cutoff2 float32,
) {
	args := KernelArgs{
		Potential:          b.dPotential,
		Atoms:              b.dAtoms,
		NumAtoms:           int32(b.NumAtoms),
		GridSize:           int32(b.GridSize),
		Spacing:            b.Spacing,
		Cutoff2:            cutoff2,
		HiddenBlockCountX:  globalSize[0] / uint32(localSize[0]),
		HiddenBlockCountY:  1,
		HiddenBlockCountZ:  1,
		HiddenGroupSizeX:   localSize[0],
		HiddenGroupSizeY:   1,
		HiddenGroupSizeZ:   1,
		HiddenRemainderX:   uint16(globalSize[0] % uint32(localSize[0])),
		HiddenRemainderY:   0,
		HiddenRemainderZ:   0,
		HiddenGridDims:     1,
	}
	b.driver.LaunchKernel(b.context,
		b.cutcpKernel,
		globalSize, localSize,
		&args,
	)
}

// Verify verifies the result against CPU computation.
func (b *Benchmark) Verify() {
	b.cpuCutcp()
	b.checkResults()
}

func (b *Benchmark) cpuCutcp() {
	gs := b.GridSize
	cutoff2 := float64(b.Cutoff * b.Cutoff)

	for iz := 0; iz < gs; iz++ {
		for iy := 0; iy < gs; iy++ {
			for ix := 0; ix < gs; ix++ {
				gid := iz*gs*gs + iy*gs + ix
				gx := float64(ix) * float64(b.Spacing)
				gy := float64(iy) * float64(b.Spacing)
				gz := float64(iz) * float64(b.Spacing)

				var energy float64
				for a := 0; a < b.NumAtoms; a++ {
					ax := float64(b.hAtoms[a*4+0])
					ay := float64(b.hAtoms[a*4+1])
					az := float64(b.hAtoms[a*4+2])
					aq := float64(b.hAtoms[a*4+3])

					dx := gx - ax
					dy := gy - ay
					dz := gz - az
					dist2 := dx*dx + dy*dy + dz*dz

					if dist2 < cutoff2 && dist2 > 1e-12 {
						energy += aq / math.Sqrt(dist2)
					}
				}
				b.hRef[gid] = float32(energy)
			}
		}
	}
}

func (b *Benchmark) checkResults() {
	errors := 0
	totalPoints := b.GridSize * b.GridSize * b.GridSize

	for i := 0; i < totalPoints; i++ {
		diff := math.Abs(float64(b.hPotential[i] - b.hRef[i]))
		tol := 1e-3*math.Abs(float64(b.hRef[i])) + 1e-5

		if diff > tol {
			if errors < 10 {
				log.Printf("Mismatch at %d: got %f, expected %f",
					i, b.hPotential[i], b.hRef[i])
			}
			errors++
		}
	}

	if errors > 0 {
		log.Panicf("FAIL: %d errors in cutcp verification", errors)
	}

	log.Printf("Passed!")
}
