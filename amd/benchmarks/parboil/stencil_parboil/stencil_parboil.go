// Package stencil_parboil implements the Parboil 3D Jacobi Stencil benchmark.
// 7-point stencil (center + 6 face neighbors) with ping-pong between two
// arrays across timesteps. Boundary cells stay fixed.
package stencil_parboil

import (
	"log"
	"math"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// Matches: stencil_kernel(DATA_TYPE *in, DATA_TYPE *out,
//
//	int nx, int ny, int nz,
//	DATA_TYPE coeff_center, DATA_TYPE coeff_neighbor)
// KernelArgs defines kernel arguments.
type KernelArgs struct {
	In                  driver.Ptr
	Out                 driver.Ptr
	Nx                  int32
	Ny                  int32
	Nz                  int32
	CoeffCenter         float32
	CoeffNeighbor       float32
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

// Benchmark defines the 3D Stencil benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	stencilKernel *insts.KernelCodeObject

	GridDim       int
	NumTimesteps  int
	BlockX        int
	BlockY        int
	CoeffCenter   float32
	CoeffNeighbor float32

	useUnifiedMemory bool

	hData []float32
	hOut  []float32
	hRef  []float32

	dA driver.Ptr
	dB driver.Ptr
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

// NewBenchmark creates a new stencil benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.GridDim = 32
	b.NumTimesteps = 10
	b.BlockX = 16
	b.BlockY = 16
	b.CoeffCenter = 0.6
	b.CoeffNeighbor = (1.0 - 0.6) / 6.0
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
	b.stencilKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "stencil_kernel")
	if b.stencilKernel == nil {
		log.Panic("Failed to load stencil_kernel binary")
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

	b.driver.MemCopyH2D(b.context, b.dA, b.hData)
	b.driver.MemCopyH2D(b.context, b.dB, b.hData)
}

func (b *Benchmark) initHostData() {
	n := b.GridDim
	total := n * n * n

	b.hData = make([]float32, total)
	for i := 0; i < total; i++ {
		b.hData[i] = float32(i%1000) * 0.001
	}

	b.hOut = make([]float32, total)
	b.hRef = make([]float32, total)
}

func (b *Benchmark) allocGPUMem() {
	total := b.GridDim * b.GridDim * b.GridDim
	bytes := uint64(total) * 4

	if b.useUnifiedMemory {
		b.dA = b.driver.AllocateUnifiedMemory(b.context, bytes)
		b.dB = b.driver.AllocateUnifiedMemory(b.context, bytes)
	} else {
		b.dA = b.driver.AllocateMemory(b.context, bytes)
		b.dB = b.driver.AllocateMemory(b.context, bytes)
	}
}

func (b *Benchmark) exec() {
	nx := b.GridDim
	ny := b.GridDim
	nz := b.GridDim

	gridX := (nx + b.BlockX - 1) / b.BlockX
	gridY := (ny + b.BlockY - 1) / b.BlockY

	globalSize := [3]uint32{
		uint32(gridX * b.BlockX),
		uint32(gridY * b.BlockY),
		uint32(nz),
	}
	localSize := [3]uint16{
		uint16(b.BlockX),
		uint16(b.BlockY),
		1,
	}

	// Ping-pong for NumTimesteps iterations
	for t := 0; t < b.NumTimesteps; t++ {
		if t%2 == 0 {
			b.launch(b.dA, b.dB, globalSize, localSize)
		} else {
			b.launch(b.dB, b.dA, globalSize, localSize)
		}
	}

	// Copy result from the correct buffer
	if b.NumTimesteps%2 == 0 {
		// Even timesteps: last write was to dA (or never ran), data is in dA
		b.driver.MemCopyD2H(b.context, b.hOut, b.dA)
	} else {
		// Odd timesteps: last write was to dB
		b.driver.MemCopyD2H(b.context, b.hOut, b.dB)
	}
}

func (b *Benchmark) launch(
	in, out driver.Ptr,
	globalSize [3]uint32, localSize [3]uint16,
) {
	args := KernelArgs{
		In:                 in,
		Out:                out,
		Nx:                 int32(b.GridDim),
		Ny:                 int32(b.GridDim),
		Nz:                 int32(b.GridDim),
		CoeffCenter:        b.CoeffCenter,
		CoeffNeighbor:      b.CoeffNeighbor,
		HiddenBlockCountX:  globalSize[0] / uint32(localSize[0]),
		HiddenBlockCountY:  globalSize[1] / uint32(localSize[1]),
		HiddenBlockCountZ:  globalSize[2],
		HiddenGroupSizeX:   localSize[0],
		HiddenGroupSizeY:   localSize[1],
		HiddenGroupSizeZ:   1,
		HiddenRemainderX:   uint16(globalSize[0] % uint32(localSize[0])),
		HiddenRemainderY:   uint16(globalSize[1] % uint32(localSize[1])),
		HiddenRemainderZ:   0,
		HiddenGridDims:     3,
	}
	b.driver.LaunchKernel(b.context,
		b.stencilKernel,
		globalSize, localSize,
		&args,
	)
}

// Verify verifies the result against CPU computation.
func (b *Benchmark) Verify() {
	b.cpuStencil()
	b.checkResults()
}

func (b *Benchmark) cpuStencil() {
	nx := b.GridDim
	ny := b.GridDim
	nz := b.GridDim
	total := nx * ny * nz

	// Use two arrays for ping-pong
	a := make([]float32, total)
	buf := make([]float32, total)
	copy(a, b.hData)
	copy(buf, b.hData)

	for t := 0; t < b.NumTimesteps; t++ {
		var in, out []float32
		if t%2 == 0 {
			in = a
			out = buf
		} else {
			in = buf
			out = a
		}

		for iz := 0; iz < nz; iz++ {
			for iy := 0; iy < ny; iy++ {
				for ix := 0; ix < nx; ix++ {
					idx := iz*ny*nx + iy*nx + ix

					if ix >= 1 && ix < nx-1 &&
						iy >= 1 && iy < ny-1 &&
						iz >= 1 && iz < nz-1 {
						out[idx] = b.CoeffCenter*in[idx] +
							b.CoeffNeighbor*(in[idx-1]+
								in[idx+1]+
								in[idx-nx]+
								in[idx+nx]+
								in[idx-ny*nx]+
								in[idx+ny*nx])
					}
					// Boundary cells stay unchanged
				}
			}
		}
	}

	if b.NumTimesteps%2 == 0 {
		copy(b.hRef, a)
	} else {
		copy(b.hRef, buf)
	}
}

func (b *Benchmark) checkResults() {
	errors := 0
	total := b.GridDim * b.GridDim * b.GridDim

	for i := 0; i < total; i++ {
		diff := math.Abs(float64(b.hOut[i] - b.hRef[i]))
		tol := 1e-3*math.Abs(float64(b.hRef[i])) + 1e-5

		if diff > tol {
			if errors < 10 {
				log.Printf("Mismatch at %d: got %f, expected %f",
					i, b.hOut[i], b.hRef[i])
			}
			errors++
		}
	}

	if errors > 0 {
		log.Panicf("FAIL: %d errors in stencil verification", errors)
	}

	log.Printf("Passed!")
}
