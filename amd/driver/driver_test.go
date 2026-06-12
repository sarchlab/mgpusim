package driver

import (
	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	"go.uber.org/mock/gomock"
)

var _ = ginkgo.Describe("Driver", func() {

	var (
		mockCtrl     *gomock.Controller
		pageTable    *MockPageTable
		driver       *Driver
		engine       *MockEngine
		toGPUs       *MockPort
		context      *Context
		cmdQueue     *CommandQueue
		memAllocator *MockMemoryAllocator
		log2PageSize uint64
	)

	ginkgo.BeforeEach(func() {
		mockCtrl = gomock.NewController(ginkgo.GinkgoT())
		engine = NewMockEngine(mockCtrl)
		toGPUs = NewMockPort(mockCtrl)
		pageTable = NewMockPageTable(mockCtrl)
		memAllocator = NewMockMemoryAllocator(mockCtrl)
		memAllocator.EXPECT().RegisterDevice(gomock.Any()).AnyTimes()
		log2PageSize = 12

		toGPUs.EXPECT().AsRemote().AnyTimes()

		driver = MakeBuilder().
			WithEngine(engine).
			WithLog2PageSize(log2PageSize).
			WithPageTable(pageTable).
			WithD2HCycles(1).
			WithH2DCycles(1).
			Build("Driver")
		driver.gpuPort = toGPUs
		driver.memAllocator = memAllocator

		for i := 0; i < 2; i++ {
			gpu := NewMockPort(mockCtrl)
			gpu.EXPECT().AsRemote().AnyTimes()
			driver.RegisterGPU(gpu,
				DeviceProperties{
					CUCount:  4,
					DRAMSize: 4 * mem.GB,
				})
		}

		context = driver.Init()
		context.pid = 1
		cmdQueue = driver.CreateCommandQueue(context)
	})

	ginkgo.AfterEach(func() {
		mockCtrl.Finish()
	})

	ginkgo.Context("process MemCopyH2D command", func() {
		ginkgo.It("should send request", func() {
			srcData := make([]byte, 0x2200)
			cmd := &MemCopyH2DCommand{
				Dst: Ptr(0x200000100),
				Src: srcData,
			}
			cmdQueue.Enqueue(cmd)
			cmdQueue.IsRunning = false

			pageTable.EXPECT().
				Find(vm.PID(1), uint64(0x200000100)).
				Return(vm.Page{
					PID:      1,
					VAddr:    0x200000000,
					PAddr:    0x100000000,
					PageSize: 0x800,
					Valid:    true,
				}, true)
			pageTable.EXPECT().
				Find(vm.PID(1), uint64(0x200000800)).
				Return(vm.Page{
					PID:      1,
					VAddr:    0x200000800,
					PAddr:    0x100000800,
					PageSize: 0x800,
					Valid:    true,
				}, true)
			pageTable.EXPECT().
				Find(vm.PID(1), uint64(0x200001000)).
				Return(vm.Page{
					PID:      1,
					VAddr:    0x200001000,
					PAddr:    0x100001000,
					PageSize: 0x1000,
					Valid:    true,
				}, true)
			pageTable.EXPECT().
				Find(vm.PID(1), uint64(0x200002000)).
				Return(vm.Page{
					PID:      1,
					VAddr:    0x200002000,
					PAddr:    0x100002000,
					PageSize: 0x1000,
					Valid:    true,
				}, true)
			memAllocator.EXPECT().
				GetDeviceIDByPAddr(uint64(0x1_0000_0100)).
				Return(1)
			memAllocator.EXPECT().
				GetDeviceIDByPAddr(uint64(0x1_0000_0800)).
				Return(1)
			memAllocator.EXPECT().
				GetDeviceIDByPAddr(uint64(0x1_0000_1000)).
				Return(1)
			memAllocator.EXPECT().
				GetDeviceIDByPAddr(uint64(0x1_0000_2000)).
				Return(1)

			toGPUs.EXPECT().PeekIncoming().Return(nil).AnyTimes()
			toGPUs.EXPECT().PeekIncoming().Return(nil).AnyTimes()
			toGPUs.EXPECT().PeekIncoming().Return(nil).AnyTimes()

			engine.EXPECT().Schedule(gomock.AssignableToTypeOf(sim.TickEvent{}))
			engine.EXPECT().Schedule(gomock.AssignableToTypeOf(sim.TickEvent{}))
			engine.EXPECT().Schedule(gomock.AssignableToTypeOf(sim.TickEvent{}))

			engine.EXPECT().CurrentTime().Return(sim.VTimeInSec(11))
			engine.EXPECT().CurrentTime().Return(sim.VTimeInSec(12))
			engine.EXPECT().CurrentTime().Return(sim.VTimeInSec(13))

			driver.Handle(sim.MakeTickEvent(nil, 11))
			driver.Handle(sim.MakeTickEvent(nil, 12))
			driver.Handle(sim.MakeTickEvent(nil, 13))

			Expect(driver.requestsToSend).To(HaveLen(4))
			Expect(cmdQueue.IsRunning).To(BeTrue())
			Expect(cmd.Reqs).To(HaveLen(4))
		})
	})

	ginkgo.Context("process MemCopyH2D return", func() {
		ginkgo.It("should remove one request", func() {
			nilPort := NewMockPort(mockCtrl)
			nilPort.EXPECT().AsRemote().AnyTimes()

			req := protocol.NewMemCopyH2DReq(toGPUs, nilPort,
				make([]byte, 4), 0x104)
			req2 := protocol.NewMemCopyH2DReq(toGPUs, nilPort,
				make([]byte, 4), 0x100)
			cmd := &MemCopyH2DCommand{
				Dst:  Ptr(0x100),
				Src:  uint32(1),
				Reqs: []sim.Msg{req, req2},
			}
			cmdQueue.Enqueue(cmd)
			cmdQueue.IsRunning = true

			rsp := sim.GeneralRspBuilder{}.WithOriginalReq(req).Build()
			toGPUs.EXPECT().PeekIncoming().Return(rsp)
			toGPUs.EXPECT().PeekIncoming().Return(nil)
			toGPUs.EXPECT().
				RetrieveIncoming().
				Return(req)

			engine.EXPECT().
				Schedule(gomock.AssignableToTypeOf(sim.TickEvent{}))

			engine.EXPECT().CurrentTime().Return(sim.VTimeInSec(11))

			driver.Handle(sim.MakeTickEvent(nil, 11))

			Expect(cmdQueue.IsRunning).To(BeTrue())
			Expect(cmdQueue.commands).To(HaveLen(1))
			Expect(cmd.Reqs).NotTo(ContainElement(req))
			Expect(cmd.Reqs).To(ContainElement(req2))
		})

		ginkgo.It("should remove command from queue if no more pending request", func() {
			nilPort := NewMockPort(mockCtrl)
			nilPort.EXPECT().AsRemote().AnyTimes()

			req := protocol.NewMemCopyH2DReq(toGPUs, nilPort,
				make([]byte, 4), 0x100)
			cmd := &MemCopyH2DCommand{
				Dst:  Ptr(0x100),
				Src:  uint32(1),
				Reqs: []sim.Msg{req},
			}
			cmdQueue.Enqueue(cmd)
			cmdQueue.IsRunning = true

			rsp := sim.GeneralRspBuilder{}.WithOriginalReq(req).Build()
			toGPUs.EXPECT().PeekIncoming().Return(rsp)
			toGPUs.EXPECT().PeekIncoming().Return(nil)
			toGPUs.EXPECT().
				RetrieveIncoming().
				Return(req)

			engine.EXPECT().Schedule(
				gomock.AssignableToTypeOf(sim.TickEvent{}))

			engine.EXPECT().CurrentTime().Return(sim.VTimeInSec(11))

			driver.Handle(sim.MakeTickEvent(nil, 11))

			Expect(cmdQueue.IsRunning).To(BeFalse())
			Expect(cmdQueue.NumCommand()).To(Equal(0))
		})

	})

	ginkgo.Context("process MemCopyD2HCommand", func() {
		ginkgo.It("should send request", func() {
			data := uint32(1)
			cmd := &MemCopyD2HCommand{
				Dst: &data,
				Src: Ptr(0x2_0000_0100),
			}
			cmdQueue.Enqueue(cmd)
			cmdQueue.IsRunning = false

			pageTable.EXPECT().Find(vm.PID(1), uint64(0x2_0000_0100)).
				Return(vm.Page{
					PID:      1,
					VAddr:    0x2_0000_0000,
					PAddr:    0x1_0000_0000,
					PageSize: 0x1000,
					Valid:    true,
				}, true)
			memAllocator.EXPECT().
				GetDeviceIDByPAddr(uint64(0x1_0000_0100)).
				Return(1)

			toGPUs.EXPECT().PeekIncoming().Return(nil).AnyTimes()
			toGPUs.EXPECT().PeekIncoming().Return(nil).AnyTimes()
			toGPUs.EXPECT().PeekIncoming().Return(nil).AnyTimes()

			engine.EXPECT().Schedule(
				gomock.AssignableToTypeOf(sim.TickEvent{}))
			engine.EXPECT().Schedule(
				gomock.AssignableToTypeOf(sim.TickEvent{}))
			engine.EXPECT().Schedule(
				gomock.AssignableToTypeOf(sim.TickEvent{}))

			engine.EXPECT().CurrentTime().Return(sim.VTimeInSec(11))
			engine.EXPECT().CurrentTime().Return(sim.VTimeInSec(12))
			engine.EXPECT().CurrentTime().Return(sim.VTimeInSec(13))

			driver.Handle(sim.MakeTickEvent(nil, 11))
			driver.Handle(sim.MakeTickEvent(nil, 12))
			driver.Handle(sim.MakeTickEvent(nil, 13))

			Expect(cmdQueue.IsRunning).To(BeTrue())
			Expect(cmd.Reqs).To(HaveLen(1))
			Expect(driver.requestsToSend).To(HaveLen(1))
		})
	})

	ginkgo.Context("process MemCopyD2H return", func() {
		ginkgo.It("should remove request", func() {
			nilPort := NewMockPort(mockCtrl)
			nilPort.EXPECT().AsRemote().AnyTimes()

			data := uint64(0)
			req := protocol.NewMemCopyD2HReq(
				nilPort, toGPUs, 0x100, []byte{1, 0, 0, 0})
			req2 := protocol.NewMemCopyD2HReq(
				nilPort, toGPUs, 0x104, []byte{1, 0, 0, 0})
			cmd := &MemCopyD2HCommand{
				Dst:  &data,
				Src:  Ptr(0x100),
				Reqs: []sim.Msg{req, req2},
			}
			cmdQueue.Enqueue(cmd)
			cmdQueue.IsRunning = true

			rsp := sim.GeneralRspBuilder{}.WithOriginalReq(req).Build()
			toGPUs.EXPECT().PeekIncoming().Return(rsp)
			toGPUs.EXPECT().PeekIncoming().Return(nil)
			toGPUs.EXPECT().
				RetrieveIncoming().
				Return(req)

			engine.EXPECT().Schedule(
				gomock.AssignableToTypeOf(sim.TickEvent{}))

			engine.EXPECT().CurrentTime().Return(sim.VTimeInSec(11))

			driver.Handle(sim.MakeTickEvent(nil, 11))

			Expect(cmdQueue.IsRunning).To(BeTrue())
			Expect(cmdQueue.commands).To(HaveLen(1))
			Expect(cmd.Reqs).To(ContainElement(req2))
			Expect(cmd.Reqs).NotTo(ContainElement(req))
		})

		ginkgo.It("should continue queue", func() {
			nilPort := NewMockPort(mockCtrl)
			nilPort.EXPECT().AsRemote().AnyTimes()

			data := uint32(0)
			req := protocol.NewMemCopyD2HReq(nilPort, toGPUs,
				0x100,
				[]byte{1, 0, 0, 0})
			cmd := &MemCopyD2HCommand{
				Dst:     &data,
				RawData: []byte{1, 0, 0, 0},
				Src:     Ptr(0x100),
				Reqs:    []sim.Msg{req},
			}
			cmdQueue.Enqueue(cmd)
			cmdQueue.IsRunning = true

			rsp := sim.GeneralRspBuilder{}.WithOriginalReq(req).Build()
			toGPUs.EXPECT().PeekIncoming().Return(rsp)
			toGPUs.EXPECT().PeekIncoming().Return(nil)
			toGPUs.EXPECT().
				RetrieveIncoming().
				Return(req)

			engine.EXPECT().Schedule(gomock.AssignableToTypeOf(sim.TickEvent{}))

			engine.EXPECT().CurrentTime().Return(sim.VTimeInSec(11))

			driver.Handle(sim.MakeTickEvent(nil, 11))

			Expect(cmdQueue.IsRunning).To(BeFalse())
			Expect(cmdQueue.commands).To(HaveLen(0))
			Expect(data).To(Equal(uint32(1)))
		})

	})

	ginkgo.Context("process LaunchKernelCommand", func() {
		ginkgo.It("should send request to GPU", func() {
			cmd := &LaunchKernelCommand{
				CodeObject: nil,
				GridSize:   [3]uint32{256, 1, 1},
				WGSize:     [3]uint16{64, 1, 1},
				KernelArgs: nil,
			}
			cmdQueue.Enqueue(cmd)
			cmdQueue.IsRunning = false

			toGPUs.EXPECT().PeekIncoming().Return(nil).AnyTimes()

			engine.EXPECT().Schedule(
				gomock.AssignableToTypeOf(sim.TickEvent{}))

			engine.EXPECT().CurrentTime().Return(sim.VTimeInSec(11))

			driver.Handle(sim.MakeTickEvent(nil, 11))

			Expect(cmdQueue.IsRunning).To(BeTrue())
			Expect(cmd.Reqs).To(HaveLen(1))
			req := cmd.Reqs[0].(*protocol.LaunchKernelReq)
			Expect(req.PID).To(Equal(vm.PID(1)))
			Expect(driver.requestsToSend).To(HaveLen(1))
		})
	})

	ginkgo.It("should process LaunchKernel return", func() {
		nilPort := NewMockPort(mockCtrl)
		nilPort.EXPECT().AsRemote().AnyTimes()

		req := protocol.NewLaunchKernelReq(toGPUs, nilPort)
		cmd := &LaunchKernelCommand{
			Reqs: []sim.Msg{req},
		}
		cmdQueue.Enqueue(cmd)
		cmdQueue.IsRunning = true
		rsp := protocol.NewLaunchKernelRsp("", "", req.ID)

		toGPUs.EXPECT().PeekIncoming().Return(rsp).Times(2)
		toGPUs.EXPECT().
			RetrieveIncoming().
			Return(rsp)

		engine.EXPECT().Schedule(gomock.AssignableToTypeOf(sim.TickEvent{}))

		engine.EXPECT().CurrentTime().Return(sim.VTimeInSec(11))

		driver.Handle(sim.MakeTickEvent(nil, 11))

		Expect(cmdQueue.IsRunning).To(BeFalse())
		Expect(cmdQueue.commands).To(HaveLen(0))
	})

})
