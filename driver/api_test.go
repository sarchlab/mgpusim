package driver

import (
	"github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/rs/xid"
	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/vm/mmu"
)

func enqueueNoopCommand(d *Driver, q *CommandQueue) {
	c := &NoopCommand{
		ID: xid.New().String(),
	}
	d.Enqueue(q, c)
}

var _ = ginkgo.Describe("Driver async API execution", func() {
	var (
		engine  akita.Engine
		mmuComp *mmu.MMUImpl
		driver  *Driver
	)

	ginkgo.BeforeEach(func() {
		engine = akita.NewSerialEngine()
		mmuComp = mmu.MakeBuilder().
			WithLog2PageSize(12).
			Build("mmu")
		driver = NewDriver(engine, mmuComp, 12)
		driver.memAllocator.RegisterStorage(1 * mem.GB)
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
