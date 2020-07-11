package driver

import (
	"github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/rs/xid"
	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/vm"
	"gitlab.com/akita/mgpusim/driver/internal"
)

func enqueueNoopCommand(d *Driver, q *CommandQueue) {
	c := &NoopCommand{
		ID: xid.New().String(),
	}
	d.Enqueue(q, c)
}

var _ = ginkgo.Describe("Driver async API execution", func() {
	var (
		engine    akita.Engine
		pageTable vm.PageTable
		driver    *Driver
	)

	ginkgo.BeforeEach(func() {
		log2PageSize := uint64(12)
		engine = akita.NewSerialEngine()
		pageTable = vm.NewPageTable(log2PageSize)
		driver = NewDriver(engine, pageTable, log2PageSize)
		gpuDevice := &internal.Device{
			ID:       1,
			Type:     internal.DeviceTypeCPU,
			MemState: internal.NewDeviceMemoryState(log2PageSize),
		}
		gpuDevice.SetTotalMemSize(1 * mem.GB)
		driver.memAllocator.RegisterDevice(gpuDevice)
		driver.Run()
	})

	ginkgo.It("should drain queues", func() {
		context := driver.Init()
		q := driver.CreateCommandQueue(context)
		enqueueNoopCommand(driver, q)

		driver.DrainCommandQueue(q)

		Expect(q.commands).To(HaveLen(0))
	})

	ginkgo.It("should drain queues", func() {
		context := driver.Init()
		q := driver.CreateCommandQueue(context)
		enqueueNoopCommand(driver, q)
		enqueueNoopCommand(driver, q)
		enqueueNoopCommand(driver, q)

		driver.DrainCommandQueue(q)

		Expect(q.commands).To(HaveLen(0))
	})

	ginkgo.Measure("Memory allocation", func(b ginkgo.Benchmarker) {
		context := driver.Init()
		b.Time("runtime", func() {
			driver.AllocateMemory(context, 400*mem.MB)
		})
	}, 10)
})
