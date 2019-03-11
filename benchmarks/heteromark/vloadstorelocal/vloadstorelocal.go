package vloadstorelocal

import (
	"log"

	"gitlab.com/akita/gcn3/kernels"

	"gitlab.com/akita/gcn3/driver"
	"gitlab.com/akita/gcn3/insts"
)

type Benchmark struct {
    driver *driver.Driver
    context *driver.Context
    hsaco  *insts.HsaCo
    queue  *driver.CommandQueue

    count    int
    h_a    []int32
    h_c    []int32
    d_a    driver.GPUPtr
    d_c    driver.GPUPtr
}

type KernelArgs struct {
    A                   driver.GPUPtr
    C                   driver.GPUPtr
    A_tmp               driver.LocalPtr
    Count               uint32
    Padding             uint32
    HiddenGlobalOffsetX int64
    HiddenGlobalOffsetY int64
    HiddenGlobalOffsetZ int64
}

func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.context = driver.Init()
	b.queue = driver.CreateCommandQueue(b.context)

	hsacoBytes, err := Asset("vloadstorelocal.hsaco")
	if err != nil {
		log.Panic(err)
	}
	b.hsaco = kernels.LoadProgramFromMemory(hsacoBytes, "vlsl")

	return b
}

func (b *Benchmark) Run() {
	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	b.count = 1024

	b.h_a = make([]int32, b.count)
	b.h_c = make([]int32, b.count)

	for i:=0; i < b.count; i++ {
		b.h_a[i] = 3
	}

        b.d_a = b.driver.AllocateMemory(b.context, uint64(b.count * 4))
        b.d_c = b.driver.AllocateMemory(b.context, uint64(b.count * 4))

	b.driver.EnqueueMemCopyH2D(b.queue, b.d_a, b.h_a)
}


func (b *Benchmark) exec() {
	kernArg := KernelArgs{
		b.d_a,
		b.d_c,
		driver.LocalPtr(64*16*4),
		uint32(b.count),
		0,
		0,0,0,
	}

	b.driver.EnqueueLaunchKernel(
		b.queue,
		b.hsaco,
		[3]uint32{uint32(b.count/16), 1, 1},
		[3]uint16{64, 1, 1},
		&kernArg,
	)

	b.driver.DrainCommandQueue(b.queue)
}

func (b *Benchmark) Verify() {
	gpuOutput := make([]int32, b.count)
	b.driver.MemCopyD2H(b.context, gpuOutput, b.d_c)

	for i := 0; i < b.count; i++ {

		if gpuOutput[i] != 3 {
			log.Fatalf("At position %d, expected %d, but get %d.\n",
				i, 3, gpuOutput[i])
		}
	}

	log.Printf("Passed!\n")
}
