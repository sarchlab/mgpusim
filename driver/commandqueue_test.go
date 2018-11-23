package driver

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
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
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		driver = NewDriver(nil)
		toGPU = mock_akita.NewMockPort(mockCtrl)
		driver.ToGPUs = toGPU
		gpu := gcn3.NewGPU("gpu", nil)
		driver.gpus = append(driver.gpus, gpu)

		cq = driver.CreateCommandQueue()
		drainer = &defaultCommandQueueDrainer{driver, nil}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should run memory copy host to device command", func() {
		c := &MemoryCopyD2HCommand{int64(4), GPUPtr(0)}
		cq.Commands = append(cq.Commands, c)

		toGPU.EXPECT().Send(gomock.Any()).Return(nil)

		drainer.scan()
	})
})
