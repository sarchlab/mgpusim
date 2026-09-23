package driver

import (
	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/mgpusim/v5/amd/driver/internal"
)

func enqueueNoopCommand(d *Driver, q *CommandQueue) {
	c := &NoopCommand{
		ID: timing.GetIDGenerator().Generate(),
	}
	d.Enqueue(q, c)
}

var _ = ginkgo.Describe("Driver async API execution", func() {
	var (
		engine    timing.Engine
		pageTable vm.PageTable
		driver    *Driver
	)

	ginkgo.BeforeEach(func() {
		log2PageSize := uint64(12)
		engine = timing.NewSerialEngine()
		pageTable = vm.NewPageTable(log2PageSize)

		spec := DefaultSpec()
		spec.Log2PageSize = log2PageSize

		driver = MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithSpec(spec).
			WithResources(Resources{PageTable: pageTable}).
			Build("Driver")

		gpuPort := messaging.NewPort(driver.Comp, 16, 16, "Driver.GPU")
		(&noopConn{}).PlugIn(gpuPort)
		driver.AssignPort(GPUPortName, gpuPort)

		gpuDevice := &internal.Device{
			ID:       1,
			Type:     internal.DeviceTypeCPU,
			MemState: internal.NewDeviceMemoryState(log2PageSize),
		}
		gpuDevice.SetTotalMemSize(1 * mem.GB)
		driver.memAllocator.RegisterDevice(gpuDevice)
		driver.Run()
	})

	ginkgo.AfterEach(func() {
		driver.Terminate()
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
})
