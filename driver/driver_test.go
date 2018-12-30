package driver

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/akita"
	"gitlab.com/akita/akita/mock_akita"
	"gitlab.com/akita/gcn3"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/vm"
	"gitlab.com/akita/mem/vm/mock_vm"
)

var _ = Describe("Driver", func() {

	var (
		mockCtrl *gomock.Controller
		gpu      *gcn3.GPU
		mmu      *mock_vm.MockMMU

		driver   *Driver
		engine   *mock_akita.MockEngine
		toGPUs   *mock_akita.MockPort
		cmdQueue *CommandQueue
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = mock_akita.NewMockEngine(mockCtrl)
		toGPUs = mock_akita.NewMockPort(mockCtrl)
		mmu = mock_vm.NewMockMMU(mockCtrl)

		gpu = gcn3.NewGPU("GPU", engine)

		driver = NewDriver(engine, mmu)
		driver.ToGPUs = toGPUs
		cmdQueue = driver.CreateCommandQueue()
		driver.RegisterGPU(gpu, 4*mem.GB)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("process MemCopyH2D command", func() {
		It("should send request", func() {
			srcData := make([]byte, 0x2200)
			cmd := &MemCopyH2DCommand{
				Dst: GPUPtr(0x100000100),
				Src: srcData,
			}
			cmdQueue.Commands = append(cmdQueue.Commands, cmd)
			cmdQueue.IsRunning = false

			mmu.EXPECT().
				Translate(vm.PID(1), uint64(0x100000100)).
				Return(uint64(0x100), &vm.Page{
					PID:      1,
					VAddr:    0x100000000,
					PAddr:    0x0,
					PageSize: 0x800,
					Valid:    true,
				})
			mmu.EXPECT().
				Translate(vm.PID(1), uint64(0x100000800)).
				Return(uint64(0x800), &vm.Page{
					PID:      1,
					VAddr:    0x100000800,
					PAddr:    0x0,
					PageSize: 0x800,
					Valid:    true,
				})
			mmu.EXPECT().
				Translate(vm.PID(1), uint64(0x100001000)).
				Return(uint64(0x1000), &vm.Page{
					PID:      1,
					VAddr:    0x100001000,
					PAddr:    0x0,
					PageSize: 0x1000,
					Valid:    true,
				})
			mmu.EXPECT().
				Translate(vm.PID(1), uint64(0x100002000)).
				Return(uint64(0x2000), &vm.Page{
					PID:      1,
					VAddr:    0x100002000,
					PAddr:    0x0,
					PageSize: 0x1000,
					Valid:    true,
				})

			toGPUs.EXPECT().
				Retrieve(akita.VTimeInSec(11)).
				Return(nil)

			engine.EXPECT().Schedule(gomock.AssignableToTypeOf(akita.TickEvent{}))

			driver.Handle(*akita.NewTickEvent(11, nil))

			Expect(driver.requestsToSend).To(HaveLen(4))
			Expect(cmdQueue.IsRunning).To(BeTrue())
			Expect(cmd.Reqs).To(HaveLen(4))
		})
	})

	Context("process MemCopyH2D return", func() {
		It("should remove one request", func() {
			req := gcn3.NewMemCopyH2DReq(9, toGPUs, nil, make([]byte, 4), 0x100)
			req2 := gcn3.NewMemCopyH2DReq(9, toGPUs, nil, make([]byte, 4), 0x100)
			cmd := &MemCopyH2DCommand{
				Dst:  GPUPtr(0x100),
				Src:  uint32(1),
				Reqs: []akita.Req{req, req2},
			}
			cmdQueue.Commands = append(cmdQueue.Commands, cmd)
			cmdQueue.IsRunning = true

			toGPUs.EXPECT().
				Retrieve(akita.VTimeInSec(11)).
				Return(req)

			engine.EXPECT().Schedule(gomock.AssignableToTypeOf(akita.TickEvent{}))

			driver.Handle(*akita.NewTickEvent(11, nil))

			Expect(cmdQueue.IsRunning).To(BeTrue())
			Expect(cmdQueue.Commands).To(HaveLen(1))
			Expect(cmd.Reqs).NotTo(ContainElement(req))
			Expect(cmd.Reqs).To(ContainElement(req2))
		})

		It("should remove command from queue if no more pending request", func() {
			req := gcn3.NewMemCopyH2DReq(9,
				toGPUs, nil,
				make([]byte, 4), 0x100)
			cmd := &MemCopyH2DCommand{
				Dst:  GPUPtr(0x100),
				Src:  uint32(1),
				Reqs: []akita.Req{req},
			}
			cmdQueue.Commands = append(cmdQueue.Commands, cmd)
			cmdQueue.IsRunning = true

			toGPUs.EXPECT().
				Retrieve(akita.VTimeInSec(11)).
				Return(req)

			engine.EXPECT().Schedule(
				gomock.AssignableToTypeOf(akita.TickEvent{}))

			driver.Handle(*akita.NewTickEvent(11, nil))

			Expect(cmdQueue.IsRunning).To(BeFalse())
			Expect(cmdQueue.Commands).To(HaveLen(0))
		})

	})

	Context("process MemCopyD2HCommand", func() {
		It("should send request", func() {
			data := uint32(1)
			cmd := &MemCopyD2HCommand{
				Dst: &data,
				Src: GPUPtr(0x100),
			}
			cmdQueue.Commands = append(cmdQueue.Commands, cmd)
			cmdQueue.IsRunning = false

			mmu.EXPECT().Translate(vm.PID(1), uint64(0x100)).
				Return(uint64(0x1100), &vm.Page{
					PID:      1,
					VAddr:    0x0,
					PAddr:    0x1000,
					PageSize: 0x1000,
					Valid:    true,
				})

			toGPUs.EXPECT().
				Retrieve(akita.VTimeInSec(11)).
				Return(nil)

			engine.EXPECT().Schedule(
				gomock.AssignableToTypeOf(akita.TickEvent{}))

			driver.Handle(*akita.NewTickEvent(11, nil))

			Expect(cmdQueue.IsRunning).To(BeTrue())
			Expect(cmd.Reqs).To(HaveLen(1))
			Expect(driver.requestsToSend).To(HaveLen(1))
		})
	})

	Context("process MemCopyD2H return", func() {
		It("should remove request", func() {
			data := uint64(0)
			req := gcn3.NewMemCopyD2HReq(
				9, nil, toGPUs, 0x100, []byte{1, 0, 0, 0})
			req2 := gcn3.NewMemCopyD2HReq(
				9, nil, toGPUs, 0x104, []byte{1, 0, 0, 0})
			cmd := &MemCopyD2HCommand{
				Dst:  &data,
				Src:  GPUPtr(0x100),
				Reqs: []akita.Req{req, req2},
			}
			cmdQueue.Commands = append(cmdQueue.Commands, cmd)
			cmdQueue.IsRunning = true

			toGPUs.EXPECT().
				Retrieve(akita.VTimeInSec(11)).
				Return(req)

			engine.EXPECT().Schedule(
				gomock.AssignableToTypeOf(akita.TickEvent{}))

			driver.Handle(*akita.NewTickEvent(11, nil))

			Expect(cmdQueue.IsRunning).To(BeTrue())
			Expect(cmdQueue.Commands).To(HaveLen(1))
			Expect(cmd.Reqs).To(ContainElement(req2))
			Expect(cmd.Reqs).NotTo(ContainElement(req))
		})

		It("should continue queue", func() {
			data := uint32(0)
			req := gcn3.NewMemCopyD2HReq(9, nil, toGPUs,
				0x100,
				[]byte{1, 0, 0, 0})
			cmd := &MemCopyD2HCommand{
				Dst:     &data,
				RawData: []byte{1, 0, 0, 0},
				Src:     GPUPtr(0x100),
				Reqs:    []akita.Req{req},
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

	Context("process LaunchKernelCommand", func() {
		It("should send request to GPU", func() {
			cmd := &LaunchKernelCommand{
				CodeObject: nil,
				GridSize:   [3]uint32{256, 1, 1},
				WGSize:     [3]uint16{64, 1, 1},
				KernelArgs: nil,
			}
			cmdQueue.Commands = append(cmdQueue.Commands, cmd)
			cmdQueue.IsRunning = false

			toGPUs.EXPECT().
				Retrieve(akita.VTimeInSec(11)).
				Return(nil)

			engine.EXPECT().Schedule(
				gomock.AssignableToTypeOf(akita.TickEvent{}))

			driver.Handle(*akita.NewTickEvent(11, nil))

			Expect(cmdQueue.IsRunning).To(BeTrue())
			Expect(cmd.Reqs).To(HaveLen(1))
			Expect(driver.requestsToSend).To(HaveLen(1))
		})
	})

	It("should process LaunchKernel return", func() {
		req := gcn3.NewLaunchKernelReq(9, toGPUs, nil)
		cmd := &LaunchKernelCommand{
			Reqs: []akita.Req{req},
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

	Context("process FlushCommand", func() {
		It("should send request to GPU", func() {
			cmd := &FlushCommand{}
			cmdQueue.Commands = append(cmdQueue.Commands, cmd)
			cmdQueue.IsRunning = false

			toGPUs.EXPECT().
				Retrieve(akita.VTimeInSec(11)).
				Return(nil)

			engine.EXPECT().Schedule(
				gomock.AssignableToTypeOf(akita.TickEvent{}))

			driver.Handle(*akita.NewTickEvent(11, nil))

			Expect(cmdQueue.IsRunning).To(BeTrue())
			Expect(cmd.Reqs).To(HaveLen(1))
			Expect(driver.requestsToSend).To(HaveLen(1))
		})
	})

	It("should process Flush return", func() {
		req := gcn3.NewFlushCommand(9, toGPUs, nil)
		cmd := &FlushCommand{
			Reqs: []akita.Req{req},
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
})
