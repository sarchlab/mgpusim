package driver

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/akita"
	"gitlab.com/akita/akita/mock_akita"
	"gitlab.com/akita/gcn3"
)

var _ = Describe("Command Queue", func() {
	It("should create command queue", func() {
		driver := NewDriver(nil)
		driver.usingGPU = 1

		q := driver.CreateCommandQueue()

		Expect(driver.CommandQueues).To(HaveLen(1))
		Expect(q.GPUID).To(Equal(1))
	})
})

var _ = Describe("Default Command Queue Drainer", func() {
	var (
		mockCtrl *gomock.Controller

		driver  *Driver
		cq      *CommandQueue
		drainer *defaultCommandQueueDrainer
		toGPU   *mock_akita.MockPort
		engine  *mock_akita.MockEngine
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = mock_akita.NewMockEngine(mockCtrl)

		driver = NewDriver(engine)
		toGPU = mock_akita.NewMockPort(mockCtrl)
		driver.ToGPUs = toGPU
		gpu := gcn3.NewGPU("gpu", engine)
		driver.gpus = append(driver.gpus, gpu)

		cq = driver.CreateCommandQueue()
		drainer = &defaultCommandQueueDrainer{driver, engine}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should run memory copy host to device command", func() {
		c := &MemCopyD2HCommand{int64(4), GPUPtr(0), nil}
		cq.Commands = append(cq.Commands, c)

		engine.EXPECT().CurrentTime().Return(akita.VTimeInSec(11))
		toGPU.EXPECT().Send(gomock.Any()).Return(nil)

		drainer.scan()
	})
})
