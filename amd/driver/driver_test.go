package driver

import (
	"fmt"

	"github.com/sarchlab/mgpusim/v5/amd/kernels"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	"go.uber.org/mock/gomock"
)

// noopConn is a minimal messaging.Connection used to drive the driver's real
// GPU port in isolation. Tests feed responses via Deliver and read sent
// requests via RetrieveOutgoing.
type noopConn struct {
	hooking.HookableBase
}

func (c *noopConn) Name() string                     { return "NoopConn" }
func (c *noopConn) BindPort(port messaging.Port)     { port.BindConnection(c) }
func (c *noopConn) Unplug(_ messaging.Port)          {}
func (c *noopConn) NotifyAvailable(_ messaging.Port) {}
func (c *noopConn) NotifySend(messaging.Port)        {}

var _ = ginkgo.Describe("Driver", func() {
	var (
		mockCtrl     *gomock.Controller
		pageTable    *vm.PageTable
		driver       *Driver
		simulation   *sim.Simulation
		toGPUs       messaging.Port
		context      *Context
		cmdQueue     *CommandQueue
		memAllocator *MockMemoryAllocator
	)

	ginkgo.BeforeEach(func() {
		mockCtrl = gomock.NewController(ginkgo.GinkgoT())
		simulation = sim.MakeBuilder().Build()
		pageTable = vm.NewPageTable(11) // Exercise the existing mixed 2KB/4KB page metadata fixture.
		memAllocator = NewMockMemoryAllocator(mockCtrl)
		memAllocator.EXPECT().RegisterDevice(gomock.Any()).AnyTimes()

		spec := DefaultSpec()
		spec.Log2PageSize = 12
		spec.D2HCycles = 1
		spec.H2DCycles = 1

		driver = MakeBuilder().
			WithSimulation(simulation).
			WithSpec(spec).
			WithResources(Resources{PageTable: pageTable}).
			Build("Driver")
		driver.memAllocator = memAllocator

		toGPUs = twowaybuffered.MakeBuilder().
			WithSimulation(simulation).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: 16, OutgoingCapacity: 16}).
			Build()
		driver.BindPort(GPUPortName, toGPUs)
		(&noopConn{}).BindPort(toGPUs)
		Expect(simulation.Initialize()).To(Succeed())

		for i := 0; i < 2; i++ {
			driver.RegisterGPU(
				messaging.RemotePort(
					fmt.Sprintf("GPU[%d].CommandProcessor", i+1)),
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
		Expect(simulation.Terminate()).To(Succeed())
	})

	makeMemCopyH2DReq := func(dst uint64, data []byte) messaging.Msg {
		return messaging.Msg{ID: driver.NewID(),
			Src: toGPUs.AsRemote(),
			Dst: driver.GPUs[0], Payload: protocol.MemCopyH2DReq{SrcBuffer: data,
				DstAddress: dst}}

	}

	makeMemCopyD2HReq := func(src uint64, data []byte) messaging.Msg {
		return messaging.Msg{ID: driver.NewID(),
			Src: toGPUs.AsRemote(),
			Dst: driver.GPUs[0], Payload: protocol.MemCopyD2HReq{SrcAddress: src,
				ByteSize: uint64(len(data))}}

	}

	deliverGeneralRsp := func(rspTo uint64) {
		rsp := messaging.Msg{ID: driver.NewID(),
			Src:   driver.GPUs[0],
			Dst:   toGPUs.AsRemote(),
			RspTo: rspTo, Payload: protocol.GeneralRsp{}}

		toGPUs.Deliver(rsp)
	}

	ginkgo.Context("process MemCopyH2D command", func() {
		ginkgo.It("should send request", func() {
			srcData := make([]byte, 0x2200)
			cmd := &MemCopyH2DCommand{
				ID:  driver.NewID(),
				Dst: Ptr(0x200000100),
				Src: srcData,
			}
			cmdQueue.Enqueue(cmd)
			cmdQueue.IsRunning = false

			pageTable.Insert(vm.Page{
				PID:      1,
				VAddr:    0x200000000,
				PAddr:    0x100000000,
				PageSize: 0x800,
				Valid:    true,
			})
			pageTable.Insert(vm.Page{
				PID:      1,
				VAddr:    0x200000800,
				PAddr:    0x100000800,
				PageSize: 0x800,
				Valid:    true,
			})
			pageTable.Insert(vm.Page{
				PID:      1,
				VAddr:    0x200001000,
				PAddr:    0x100001000,
				PageSize: 0x1000,
				Valid:    true,
			})
			pageTable.Insert(vm.Page{
				PID:      1,
				VAddr:    0x200002000,
				PAddr:    0x100002000,
				PageSize: 0x1000,
				Valid:    true,
			})
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

			tickDriver(driver)
			tickDriver(driver)
			tickDriver(driver)

			Expect(driver.requestsToSend).To(HaveLen(4))
			Expect(cmdQueue.IsRunning).To(BeTrue())
			Expect(cmd.Reqs).To(HaveLen(4))
		})
	})

	ginkgo.Context("process MemCopyH2D return", func() {
		ginkgo.It("should remove one request", func() {
			req := makeMemCopyH2DReq(0x104, make([]byte, 4))
			req2 := makeMemCopyH2DReq(0x100, make([]byte, 4))
			cmd := &MemCopyH2DCommand{
				ID:   driver.NewID(),
				Dst:  Ptr(0x100),
				Src:  uint32(1),
				Reqs: []messaging.Msg{req, req2},
			}
			cmdQueue.Enqueue(cmd)
			cmdQueue.IsRunning = true

			deliverGeneralRsp(req.ID)

			tickDriver(driver)

			Expect(cmdQueue.IsRunning).To(BeTrue())
			Expect(cmdQueue.commands).To(HaveLen(1))
			Expect(cmd.Reqs).NotTo(ContainElement(req))
			Expect(cmd.Reqs).To(ContainElement(req2))
		})

		ginkgo.It("should remove command from queue if no more pending request",
			func() {
				req := makeMemCopyH2DReq(0x100, make([]byte, 4))
				cmd := &MemCopyH2DCommand{
					ID:   driver.NewID(),
					Dst:  Ptr(0x100),
					Src:  uint32(1),
					Reqs: []messaging.Msg{req},
				}
				cmdQueue.Enqueue(cmd)
				cmdQueue.IsRunning = true

				deliverGeneralRsp(req.ID)

				tickDriver(driver)

				Expect(cmdQueue.IsRunning).To(BeFalse())
				Expect(cmdQueue.NumCommand()).To(Equal(0))
			})
	})

	ginkgo.Context("process MemCopyD2HCommand", func() {
		ginkgo.It("should send request", func() {
			data := uint32(1)
			cmd := &MemCopyD2HCommand{
				ID:  driver.NewID(),
				Dst: &data,
				Src: Ptr(0x2_0000_0100),
			}
			cmdQueue.Enqueue(cmd)
			cmdQueue.IsRunning = false

			pageTable.Insert(vm.Page{
				PID:      1,
				VAddr:    0x2_0000_0000,
				PAddr:    0x1_0000_0000,
				PageSize: 0x1000,
				Valid:    true,
			})
			memAllocator.EXPECT().
				GetDeviceIDByPAddr(uint64(0x1_0000_0100)).
				Return(1)

			tickDriver(driver)
			tickDriver(driver)
			tickDriver(driver)

			Expect(cmdQueue.IsRunning).To(BeTrue())
			Expect(cmd.Reqs).To(HaveLen(1))
			Expect(driver.requestsToSend).To(HaveLen(1))
		})
	})

	ginkgo.Context("process MemCopyD2H return", func() {
		ginkgo.It("should remove request", func() {
			data := uint64(0)
			req := makeMemCopyD2HReq(0x100, []byte{1, 0, 0, 0})
			req2 := makeMemCopyD2HReq(0x104, []byte{1, 0, 0, 0})
			cmd := &MemCopyD2HCommand{
				ID:      driver.NewID(),
				Dst:     &data,
				Src:     Ptr(0x100),
				Reqs:    []messaging.Msg{req, req2},
				RawData: make([]byte, 8),
			}
			cmdQueue.Enqueue(cmd)
			cmdQueue.IsRunning = true

			toGPUs.Deliver(
				messaging.Msg{
					ID:      driver.NewID(),
					RspTo:   req.ID,
					Payload: protocol.MemCopyD2HRsp{Data: []byte{1, 0, 0, 0}},
				},
			)

			tickDriver(driver)

			Expect(cmdQueue.IsRunning).To(BeTrue())
			Expect(cmdQueue.commands).To(HaveLen(1))
			Expect(cmd.Reqs).To(ContainElement(req2))
			Expect(cmd.Reqs).NotTo(ContainElement(req))
		})

		ginkgo.It("should continue queue", func() {
			data := uint32(0)
			req := makeMemCopyD2HReq(0x100, []byte{1, 0, 0, 0})
			cmd := &MemCopyD2HCommand{
				ID:      driver.NewID(),
				Dst:     &data,
				RawData: []byte{1, 0, 0, 0},
				Src:     Ptr(0x100),
				Reqs:    []messaging.Msg{req},
			}
			cmdQueue.Enqueue(cmd)
			cmdQueue.IsRunning = true

			toGPUs.Deliver(
				messaging.Msg{
					ID:      driver.NewID(),
					RspTo:   req.ID,
					Payload: protocol.MemCopyD2HRsp{Data: []byte{1, 0, 0, 0}},
				},
			)

			tickDriver(driver)

			Expect(cmdQueue.IsRunning).To(BeFalse())
			Expect(cmdQueue.commands).To(HaveLen(0))
			Expect(data).To(Equal(uint32(1)))
		})
	})

	ginkgo.DescribeTable("complete memory copy regardless of response order",
		func(deviceToHost bool, order []int) {
			data := uint32(0)
			payload := []byte{0x78, 0x56, 0x34, 0x12}
			var copyReq messaging.Msg
			var cmd Command
			if deviceToHost {
				copyReq = makeMemCopyD2HReq(0x100, payload)
				cmd = &MemCopyD2HCommand{
					ID:  driver.NewID(),
					Src: Ptr(0x100), Dst: &data, RawData: payload,
				}
			} else {
				copyReq = makeMemCopyH2DReq(0x100, payload)
				cmd = &MemCopyH2DCommand{
					ID:  driver.NewID(),
					Dst: Ptr(0x100), Src: uint32(0x12345678),
				}
			}
			reqs := []messaging.Msg{copyReq}
			for _, gpu := range driver.GPUs {
				reqs = append(
					reqs,
					messaging.Msg{ID: driver.NewID(), Src: toGPUs.AsRemote(), Dst: gpu, Payload: protocol.FlushReq{}},
				)
			}
			for _, req := range reqs {
				cmd.AddReq(req)
			}
			cmdQueue.Enqueue(cmd)
			cmdQueue.Enqueue(&NoopCommand{ID: driver.NewID()})
			cmdQueue.IsRunning = true

			for i, index := range order {
				if deviceToHost && index == 0 {
					toGPUs.Deliver(
						messaging.Msg{
							ID:      driver.NewID(),
							RspTo:   reqs[index].ID,
							Payload: protocol.MemCopyD2HRsp{Data: payload},
						},
					)
				} else {
					deliverGeneralRsp(reqs[index].ID)
				}
				tickDriver(driver)
				Expect(cmd.GetReqs()).To(HaveLen(len(order) - i - 1))
				if i < len(order)-1 {
					Expect(cmdQueue.IsRunning).To(BeTrue())
					Expect(cmdQueue.NumCommand()).To(Equal(2))
					Expect(data).To(BeZero())
				}
			}
			// The last reply must release the copy and allow the next command
			// to run, including when the last reply acknowledges a remote flush.
			Expect(cmdQueue.IsRunning).To(BeFalse())
			Expect(cmdQueue.NumCommand()).To(BeZero())
			if deviceToHost {
				Expect(data).To(Equal(uint32(0x12345678)))
			}
		},
		ginkgo.Entry("H2D copy, flush 1, flush 2", false, []int{0, 1, 2}),
		ginkgo.Entry("H2D copy, flush 2, flush 1", false, []int{0, 2, 1}),
		ginkgo.Entry("H2D flush 1, copy, flush 2", false, []int{1, 0, 2}),
		ginkgo.Entry("H2D flush 2, copy, flush 1", false, []int{2, 0, 1}),
		ginkgo.Entry("H2D flush 1, flush 2, copy", false, []int{1, 2, 0}),
		ginkgo.Entry("H2D flush 2, flush 1, copy", false, []int{2, 1, 0}),
		ginkgo.Entry("D2H copy, flush 1, flush 2", true, []int{0, 1, 2}),
		ginkgo.Entry("D2H copy, flush 2, flush 1", true, []int{0, 2, 1}),
		ginkgo.Entry("D2H flush 1, copy, flush 2", true, []int{1, 0, 2}),
		ginkgo.Entry("D2H flush 2, copy, flush 1", true, []int{2, 0, 1}),
		ginkgo.Entry("D2H flush 1, flush 2, copy", true, []int{1, 2, 0}),
		ginkgo.Entry("D2H flush 2, flush 1, copy", true, []int{2, 1, 0}),
	)

	ginkgo.Context("process LaunchKernelCommand", func() {
		ginkgo.It("should send request to GPU", func() {
			cmd := &LaunchKernelCommand{
				ID:         driver.NewID(),
				CodeObject: nil,
				GridSize:   [3]uint32{256, 1, 1},
				WGSize:     [3]uint16{64, 1, 1},
				KernelArgs: nil,
				Packet:     &kernels.HsaKernelDispatchPacket{},
			}
			cmdQueue.Enqueue(cmd)
			cmdQueue.IsRunning = false

			tickDriver(driver)

			Expect(cmdQueue.IsRunning).To(BeTrue())
			Expect(cmd.Reqs).To(HaveLen(1))
			req := cmd.Reqs[0]
			Expect(req.Payload.(protocol.LaunchKernelReq).PID).To(Equal(vm.PID(1)))
			Expect(driver.requestsToSend).To(HaveLen(1))
		})
	})

	ginkgo.It("should process LaunchKernel return", func() {
		req := messaging.Msg{ID: driver.NewID(),
			Src: toGPUs.AsRemote(),
			Dst: driver.GPUs[0], Payload: protocol.LaunchKernelReq{}}

		cmd := &LaunchKernelCommand{
			ID:   driver.NewID(),
			Reqs: []messaging.Msg{req},
		}
		cmdQueue.Enqueue(cmd)
		cmdQueue.IsRunning = true

		rsp := messaging.Msg{ID: driver.NewID(),
			Src:   driver.GPUs[0],
			Dst:   toGPUs.AsRemote(),
			RspTo: req.ID, Payload: protocol.LaunchKernelRsp{}}

		toGPUs.Deliver(rsp)

		tickDriver(driver)

		Expect(cmdQueue.IsRunning).To(BeFalse())
		Expect(cmdQueue.commands).To(HaveLen(0))
	})
})

func tickDriver(d *Driver) bool {
	a := d.Middlewares.Send.Tick()
	b := d.Middlewares.Memory.Tick()
	c := d.Middlewares.Command.Tick()
	return a || b || c
}
