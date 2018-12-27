package matrixmultiplication

import (
	"log"

	"gitlab.com/akita/gcn3/insts"

	"gitlab.com/akita/gcn3/driver"
	"gitlab.com/akita/gcn3/kernels"
)

type Matrix struct {
	Data          []float32
	Width, Height uint32
}

func NewMatrix(width, height uint32) *Matrix {
	matrix := new(Matrix)
	matrix.Width = width
	matrix.Height = height
	matrix.Data = make([]float32, width*height)
	return matrix
}

type KernelArgs struct {
	MatrixA             driver.GPUPtr
	MatrixB             driver.GPUPtr
	MatrixC             driver.GPUPtr
	WidthA              uint32
	BlockA              driver.LocalPtr
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

func MatrixMultiplicationOnGPU(mA, mB *Matrix, gpuDriver *driver.Driver) *Matrix {
	mC := new(Matrix)
	mC.Width = mB.Width
	mC.Height = mA.Height
	mC.Data = make([]float32, mC.Width*mC.Height)

	kernel := loadKernel()

	gA, gB, gC := initMemory(gpuDriver, mA, mB, mC)

	launchKernel(gA, gB, gC, mA, gpuDriver, kernel, mC)

	copyDataBackFromGPU(gpuDriver, mC, gC)

	return mC
}

func launchKernel(gA driver.GPUPtr, gB driver.GPUPtr, gC driver.GPUPtr, mA *Matrix, gpuDriver *driver.Driver, kernel *insts.HsaCo, mC *Matrix) {
	numGPU := gpuDriver.GetNumGPUs()
	for i := 0; i < numGPU; i++ {
		kernArgs := &KernelArgs{
			gA, gB, gC,
			mA.Width,
			32 * 32 * 4,
			0, int64(int(mC.Height) / 4 / numGPU * i), 0,
		}
		gpuDriver.SelectGPU(i)
		gpuDriver.EnqueueLaunchKernel(
			gpuDriver.CreateCommandQueue(),
			kernel,
			[3]uint32{mC.Width / 4 / uint32(numGPU), mC.Height / 4 / uint32(numGPU), 1},
			[3]uint16{8, 8, 1},
			kernArgs,
		)
	}

	gpuDriver.ExecuteAllCommands()
}

func initMemory(gpuDriver *driver.Driver, mA *Matrix, mB *Matrix, mC *Matrix) (driver.GPUPtr, driver.GPUPtr, driver.GPUPtr) {
	gA := gpuDriver.AllocateMemory(uint64(mA.Width * mA.Height * 4))
	gB := gpuDriver.AllocateMemory(uint64(mB.Width * mB.Height * 4))
	gC := gpuDriver.AllocateMemory(uint64(mC.Width * mC.Height * 4))

	distributeMatrixMemToGPUs(gpuDriver, mA, gA)
	distributeMatrixMemToGPUs(gpuDriver, mB, gB)
	distributeMatrixMemToGPUs(gpuDriver, mC, gC)

	return gA, gB, gC
}

func distributeMatrixMemToGPUs(gpuDriver *driver.Driver, m *Matrix, gm driver.GPUPtr) {
	numGPU := gpuDriver.GetNumGPUs()
	for i := 0; i < numGPU; i++ {
		bytePerGPU := uint64(m.Width * m.Height * 4 / uint32(numGPU))
		addr := uint64(gm) + uint64(i)*bytePerGPU
		gpuDriver.Remap(addr, bytePerGPU, i)

		gpuDriver.MemCopyH2D(driver.GPUPtr(addr),
			m.Data[i*int(bytePerGPU)/4:(i+1)*int(bytePerGPU)/4])
	}
}

func copyDataBackFromGPU(gpuDriver *driver.Driver, m *Matrix, gm driver.GPUPtr) {
	numGPU := gpuDriver.GetNumGPUs()
	for i := 0; i < numGPU; i++ {
		bytePerGPU := uint64(m.Width * m.Height * 4 / uint32(numGPU))
		addr := uint64(gm) + uint64(i)*bytePerGPU

		gpuDriver.MemCopyD2H(
			m.Data[i*int(bytePerGPU)/4:(i+1)*int(bytePerGPU)/4],
			driver.GPUPtr(addr))
	}
}

func loadKernel() *insts.HsaCo {
	hsacoBytes, err := Asset("kernels.hsaco")
	if err != nil {
		log.Panic(err)
	}

	kernel := kernels.LoadProgramFromMemory(hsacoBytes, "mmmKernel_local")
	if kernel == nil {
		log.Panic("Failed to load kernel binary")
	}

	return kernel
}

func MatrixMultiplicationOnCPU(mA, mB *Matrix) *Matrix {
	if mA.Width != mB.Height {
		log.Panic("matrix dimension mismatch")
	}

	mC := new(Matrix)
	mC.Width = mB.Width
	mC.Height = mA.Height
	mC.Data = make([]float32, mC.Width*mC.Height)

	for x := uint32(0); x < mC.Width; x++ {
		for y := uint32(0); y < mC.Height; y++ {
			indexC := y*mC.Width + x

			sum := float32(0)
			for i := uint32(0); i < mA.Width; i++ {
				indexA := y*mA.Width + i
				indexB := i*mB.Width + x
				sum += mA.Data[indexA] * mB.Data[indexB]
			}

			mC.Data[indexC] = sum
		}
	}

	return mC
}
