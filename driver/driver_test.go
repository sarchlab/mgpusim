package driver

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/akita"
	"gitlab.com/akita/akita/mock_akita"
	"gitlab.com/akita/gcn3"
)

var _ = Describe("Driver", func() {

	var (
		mockCtrl *gomock.Controller
		gpu      *gcn3.GPU

		driver   *Driver
		engine   *mock_akita.MockEngine
		toGPUs   *mock_akita.MockPort
		cmdQueue *CommandQueue
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = mock_akita.NewMockEngine(mockCtrl)
		toGPUs = mock_akita.NewMockPort(mockCtrl)

		gpu = gcn3.NewGPU("GPU", engine)

		driver = NewDriver(engine)
		driver.ToGPUs = toGPUs
		cmdQueue = driver.CreateCommandQueue()
		driver.gpus = append(driver.gpus, gpu)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("process MemCopyH2D command", func() {
		It("should send request", func() {
			cmd := &MemCopyH2DCommand{
				Dst: GPUPtr(0x100),
				Src: uint32(1),
			}
			cmdQueue.Commands = append(cmdQueue.Commands, cmd)
			cmdQueue.IsRunning = false

			toGPUs.EXPECT().
				Retrieve(akita.VTimeInSec(11)).
				Return(nil)

			toGPUs.EXPECT().
				Send(gomock.AssignableToTypeOf(&gcn3.MemCopyH2DReq{})).
				Return(nil)

			engine.EXPECT().Schedule(gomock.AssignableToTypeOf(akita.TickEvent{}))

			driver.Handle(*akita.NewTickEvent(11, nil))
		})

	})

	It("should process MemCopyH2D return", func() {
		req := gcn3.NewMemCopyH2DReq(9, toGPUs, nil, make([]byte, 4), 0x100)
		cmd := &MemCopyH2DCommand{
			Dst: GPUPtr(0x100),
			Src: uint32(1),
			Req: req,
		}
		cmdQueue.Commands = append(cmdQueue.Commands, cmd)
		cmdQueue.IsRunning = true

		toGPUs.EXPECT().
			Retrieve(akita.VTimeInSec(11)).
			Return(req)

		engine.EXPECT().Schedule(gomock.AssignableToTypeOf(akita.TickEvent{}))

		driver.Handle(*akita.NewTickEvent(11, nil))

		Expect(cmdQueue.IsRunning).To(BeFalse())
		Expect(cmdQueue.Commands).To(HaveLen(0))
	})

	It("should process MemCopyD2H return", func() {
		data := uint32(0)
		req := gcn3.NewMemCopyD2HReq(9, nil, toGPUs, 0x100, []byte{1, 0, 0, 0})
		cmd := &MemCopyD2HCommand{
			Dst: &data,
			Src: GPUPtr(0x100),
			Req: req,
		}
		cmdQueue.Commands = append(cmdQueue.Commands, cmd)
		cmdQueue.IsRunning = true

		toGPUs.EXPECT().
			Retrieve(akita.VTimeInSec(11)).
			Return(req)

		engine.EXPECT().Schedule(gomock.AssignableToTypeOf(akita.TickEvent{}))

		driver.Handle(*akita.NewTickEvent(11, nil))

		Expect(cmdQueue.IsRunning).To(BeFalse())
		Expect(cmdQueue.Commands).To(HaveLen(0))
		Expect(data).To(Equal(uint32(1)))
	})

})
