package driver

import (
	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rs/xid"
	"gitlab.com/akita/akita/v3/sim"
	"gitlab.com/akita/mem/v3/mem"
	"gitlab.com/akita/mem/v3/vm"
	"gitlab.com/akita/mgpusim/v3/driver/internal"
)

func enqueueNoopCommand(d *Driver, q *CommandQueue) {
	c := &NoopCommand{
		ID: xid.New().String(),
	}
	d.Enqueue(q, c)
}

var _ = ginkgo.Describe("Driver async API execution", func() {
	var (
		engine    sim.Engine
		pageTable vm.PageTable
		driver    *Driver
	)

	ginkgo.BeforeEach(func() {
		log2PageSize := uint64(12)
		engine = sim.NewSerialEngine()
		pageTable = vm.NewPageTable(log2PageSize)

		driver = MakeBuilder().
			WithEngine(engine).
			WithLog2PageSize(log2PageSize).
			WithPageTable(pageTable).
			Build("Driver")
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

	ginkgo.It("should allocate memory", func() {
		context := driver.Init()

		ptr := driver.AllocateMemory(context, 1*mem.MB)

		Expect(context.buffers).To(HaveLen(1))
		Expect(context.buffers[0].size).To(Equal(1 * mem.MB))
		Expect(context.buffers[0].vAddr).To(Equal(ptr))
		Expect(context.buffers[0].freed).To(BeFalse())
		Expect(context.buffers[0].l2Dirty).To(BeFalse())
	})

	ginkgo.It("should allocate unified memory", func() {
		context := driver.Init()

		ptr := driver.AllocateUnifiedMemory(context, 1*mem.MB)

		Expect(context.buffers).To(HaveLen(1))
		Expect(context.buffers[0].size).To(Equal(1 * mem.MB))
		Expect(context.buffers[0].vAddr).To(Equal(ptr))
		Expect(context.buffers[0].freed).To(BeFalse())
		Expect(context.buffers[0].l2Dirty).To(BeFalse())
	})

	// ginkgo.Measure("Memory allocation", func(b ginkgo.Benchmarker) {
	// 	context := driver.Init()
	// 	b.Time("runtime", func() {
	// 		driver.AllocateMemory(context, 400*mem.MB)
	// 	})
	// }, 10)
})
