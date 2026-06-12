// Package mri_q implements the Parboil MRI-Q benchmark.
// Computes the Q matrix for MRI reconstruction using a single kernel.
// Kernel: ComputeQ(x, y, z, kx, ky, kz, phi_r, phi_i, Qr, Qi, num_voxels, num_k)
package mri_q

import (
	"log"
	"math"
	"math/rand"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// KernelArgs defines kernel arguments.
type KernelArgs struct {
	X                   driver.Ptr
	Y                   driver.Ptr
	Z                   driver.Ptr
	Kx                  driver.Ptr
	Ky                  driver.Ptr
	Kz                  driver.Ptr
	PhiR                driver.Ptr
	PhiI                driver.Ptr
	Qr                  driver.Ptr
	Qi                  driver.Ptr
	NumVoxels           int32
	NumK                int32
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

// Benchmark defines the Parboil MRI-Q benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	computeQKernel *insts.KernelCodeObject

	NumVoxels int
	NumK      int

	useUnifiedMemory bool

	hX    []float32
	hY    []float32
	hZ    []float32
	hKx   []float32
	hKy   []float32
	hKz   []float32
	hPhiR []float32
	hPhiI []float32
	hQr   []float32
	hQi   []float32

	dX    driver.Ptr
	dY    driver.Ptr
	dZ    driver.Ptr
	dKx   driver.Ptr
	dKy   driver.Ptr
	dKz   driver.Ptr
	dPhiR driver.Ptr
	dPhiI driver.Ptr
	dQr   driver.Ptr
	dQi   driver.Ptr
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

// NewBenchmark creates a new Parboil MRI-Q benchmark.
func NewBenchmark(drv *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = drv
	b.context = drv.Init()
	b.NumVoxels = 65536
	b.NumK = 256
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
	b.computeQKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "ComputeQ")
	if b.computeQKernel == nil {
		log.Panic("Failed to load ComputeQ kernel binary")
	}
}

// Run runs the benchmark.
func (b *Benchmark) Run() {
	for _, gpu := range b.gpus {
		b.driver.SelectGPU(b.context, gpu)
		b.queues = append(b.queues, b.driver.CreateCommandQueue(b.context))
	}
	b.loadProgram()
	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	rand.Seed(42)
	b.initHostMem()
	b.allocDeviceMem()
	b.copyHostToDevice()
}

func (b *Benchmark) initHostMem() {
	b.hX = make([]float32, b.NumVoxels)
	b.hY = make([]float32, b.NumVoxels)
	b.hZ = make([]float32, b.NumVoxels)
	b.hQr = make([]float32, b.NumVoxels)
	b.hQi = make([]float32, b.NumVoxels)

	b.hKx = make([]float32, b.NumK)
	b.hKy = make([]float32, b.NumK)
	b.hKz = make([]float32, b.NumK)
	b.hPhiR = make([]float32, b.NumK)
	b.hPhiI = make([]float32, b.NumK)

	for i := 0; i < b.NumVoxels; i++ {
		b.hX[i] = rand.Float32()*2.0 - 1.0
		b.hY[i] = rand.Float32()*2.0 - 1.0
		b.hZ[i] = rand.Float32()*2.0 - 1.0
		b.hQr[i] = 0.0
		b.hQi[i] = 0.0
	}

	for i := 0; i < b.NumK; i++ {
		b.hKx[i] = rand.Float32()*2.0 - 1.0
		b.hKy[i] = rand.Float32()*2.0 - 1.0
		b.hKz[i] = rand.Float32()*2.0 - 1.0
		b.hPhiR[i] = rand.Float32()
		b.hPhiI[i] = rand.Float32()
	}
}

func (b *Benchmark) allocDeviceMem() {
	voxelBytes := uint64(b.NumVoxels) * 4
	kBytes := uint64(b.NumK) * 4

	if b.useUnifiedMemory {
		b.dX = b.driver.AllocateUnifiedMemory(b.context, voxelBytes)
		b.dY = b.driver.AllocateUnifiedMemory(b.context, voxelBytes)
		b.dZ = b.driver.AllocateUnifiedMemory(b.context, voxelBytes)
		b.dQr = b.driver.AllocateUnifiedMemory(b.context, voxelBytes)
		b.dQi = b.driver.AllocateUnifiedMemory(b.context, voxelBytes)
		b.dKx = b.driver.AllocateUnifiedMemory(b.context, kBytes)
		b.dKy = b.driver.AllocateUnifiedMemory(b.context, kBytes)
		b.dKz = b.driver.AllocateUnifiedMemory(b.context, kBytes)
		b.dPhiR = b.driver.AllocateUnifiedMemory(b.context, kBytes)
		b.dPhiI = b.driver.AllocateUnifiedMemory(b.context, kBytes)
	} else {
		b.dX = b.driver.AllocateMemory(b.context, voxelBytes)
		b.dY = b.driver.AllocateMemory(b.context, voxelBytes)
		b.dZ = b.driver.AllocateMemory(b.context, voxelBytes)
		b.dQr = b.driver.AllocateMemory(b.context, voxelBytes)
		b.dQi = b.driver.AllocateMemory(b.context, voxelBytes)
		b.dKx = b.driver.AllocateMemory(b.context, kBytes)
		b.dKy = b.driver.AllocateMemory(b.context, kBytes)
		b.dKz = b.driver.AllocateMemory(b.context, kBytes)
		b.dPhiR = b.driver.AllocateMemory(b.context, kBytes)
		b.dPhiI = b.driver.AllocateMemory(b.context, kBytes)
	}
}

func (b *Benchmark) copyHostToDevice() {
	b.driver.MemCopyH2D(b.context, b.dX, b.hX)
	b.driver.MemCopyH2D(b.context, b.dY, b.hY)
	b.driver.MemCopyH2D(b.context, b.dZ, b.hZ)
	b.driver.MemCopyH2D(b.context, b.dQr, b.hQr)
	b.driver.MemCopyH2D(b.context, b.dQi, b.hQi)
	b.driver.MemCopyH2D(b.context, b.dKx, b.hKx)
	b.driver.MemCopyH2D(b.context, b.dKy, b.hKy)
	b.driver.MemCopyH2D(b.context, b.dKz, b.hKz)
	b.driver.MemCopyH2D(b.context, b.dPhiR, b.hPhiR)
	b.driver.MemCopyH2D(b.context, b.dPhiI, b.hPhiI)
}

func (b *Benchmark) exec() {
	blockSize := 256
	numBlocks := (b.NumVoxels + blockSize - 1) / blockSize

	globalSize := [3]uint32{uint32(numBlocks * blockSize), 1, 1}
	localSize := [3]uint16{uint16(blockSize), 1, 1}

	b.launch(globalSize, localSize)

	b.driver.MemCopyD2H(b.context, b.hQr, b.dQr)
	b.driver.MemCopyD2H(b.context, b.hQi, b.dQi)
}

func (b *Benchmark) launch(
	globalSize [3]uint32, localSize [3]uint16,
) {
	args := KernelArgs{
		X:                 b.dX,
		Y:                 b.dY,
		Z:                 b.dZ,
		Kx:                b.dKx,
		Ky:                b.dKy,
		Kz:                b.dKz,
		PhiR:              b.dPhiR,
		PhiI:              b.dPhiI,
		Qr:                b.dQr,
		Qi:                b.dQi,
		NumVoxels:         int32(b.NumVoxels),
		NumK:              int32(b.NumK),
		HiddenBlockCountX: globalSize[0] / uint32(localSize[0]),
		HiddenBlockCountY: 1,
		HiddenBlockCountZ: 1,
		HiddenGroupSizeX:  localSize[0],
		HiddenGroupSizeY:  1,
		HiddenGroupSizeZ:  1,
		HiddenRemainderX:  uint16(globalSize[0] % uint32(localSize[0])),
		HiddenRemainderY:  0,
		HiddenRemainderZ:  0,
		HiddenGridDims:    1,
	}
	b.driver.LaunchKernel(b.context,
		b.computeQKernel,
		globalSize, localSize,
		&args,
	)
}

const mPi = math.Pi

// Verify checks the result against a CPU reference for the first 16 voxels.
func (b *Benchmark) Verify() {
	checkCount := 16
	if b.NumVoxels < checkCount {
		checkCount = b.NumVoxels
	}

	errors := 0
	for v := 0; v < checkCount; v++ {
		xv := b.hX[v]
		yv := b.hY[v]
		zv := b.hZ[v]

		var qrRef, qiRef float64
		for k := 0; k < b.NumK; k++ {
			phase := 2.0 * mPi * float64(b.hKx[k]*xv+b.hKy[k]*yv+b.hKz[k]*zv)
			cosVal := math.Cos(phase)
			sinVal := math.Sin(phase)
			qrRef += float64(b.hPhiR[k])*cosVal - float64(b.hPhiI[k])*sinVal
			qiRef += float64(b.hPhiR[k])*sinVal + float64(b.hPhiI[k])*cosVal
		}

		diffR := math.Abs(float64(b.hQr[v]) - qrRef)
		diffI := math.Abs(float64(b.hQi[v]) - qiRef)
		tolR := 1e-3*math.Abs(qrRef) + 1e-3
		tolI := 1e-3*math.Abs(qiRef) + 1e-3

		if diffR > tolR || diffI > tolI {
			if errors < 10 {
				log.Printf("Mismatch at voxel %d: Qr got %f expected %f, Qi got %f expected %f",
					v, b.hQr[v], qrRef, b.hQi[v], qiRef)
			}
			errors++
		}
	}

	if errors > 0 {
		log.Panicf("FAIL: %d errors in MRI-Q verification", errors)
	}

	log.Printf("Passed!")
}
