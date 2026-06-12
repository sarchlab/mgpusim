package driver

import (
	"fmt"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
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
func (c *noopConn) PlugIn(port messaging.Port)       { port.SetConnection(c) }
func (c *noopConn) Unplug(_ messaging.Port)          {}
func (c *noopConn) NotifyAvailable(_ messaging.Port) {}
func (c *noopConn) NotifySend()                      {}

var _ = ginkgo.Describe("Driver", func() {
	var (
		mockCtrl     *gomock.Controller
		pageTable    *MockPageTable
		driver       *Driver
		engine       timing.Engine
		toGPUs       messaging.Port
		context      *Context
		cmdQueue     *CommandQueue
		memAllocator *MockMemoryAllocator
	)

	ginkgo.BeforeEach(func() {
		mockCtrl = gomock.NewController(ginkgo.GinkgoT())
		engine = timing.NewSerialEngine()
		pageTable = NewMockPageTable(mockCtrl)
		memAllocator = NewMockMemoryAllocator(mockCtrl)
		memAllocator.EXPECT().RegisterDevice(gomock.Any()).AnyTimes()

		spec := DefaultSpec()
		spec.Log2PageSize = 12
		spec.D2HCycles = 1
		spec.H2DCycles = 1

		driver = MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithSpec(spec).
			WithResources(Resources{PageTable: pageTable}).
			Build("Driver")
		driver.memAllocator = memAllocator

		toGPUs = messaging.NewPort(driver.Comp, 16, 16, "Driver.GPU")
		(&noopConn{}).PlugIn(toGPUs)
		driver.AssignPort(GPUPortName, toGPUs)

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
	})

	makeMemCopyH2DReq := func(dst uint64, data []byte) protocol.MemCopyH2DReq {
		return protocol.MemCopyH2DReq{
			MsgMeta: messaging.MsgMeta{
				ID:  timing.GetIDGenerator().Generate(),
				Src: toGPUs.AsRemote(),
				Dst: driver.GPUs[0],
			},
			SrcBuffer:  data,
			DstAddress: dst,
		}
	}

	makeMemCopyD2HReq := func(src uint64, data []byte) protocol.MemCopyD2HReq {
		return protocol.MemCopyD2HReq{
			MsgMeta: messaging.MsgMeta{
				ID:  timing.GetIDGenerator().Generate(),
				Src: toGPUs.AsRemote(),
				Dst: driver.GPUs[0],
			},
			SrcAddress: src,
			DstBuffer:  data,
		}
	}

	deliverGeneralRsp := func(rspTo uint64) {
		rsp := protocol.GeneralRsp{
			MsgMeta: messaging.MsgMeta{
				ID:    timing.GetIDGenerator().Generate(),
				Src:   driver.GPUs[0],
				Dst:   toGPUs.AsRemote(),
				RspTo: rspTo,
			},
		}
		toGPUs.Deliver(rsp)
	}

	ginkgo.Context("process MemCopyH2D command", func() {
		ginkgo.It("should send request", func() {
			srcData := make([]byte, 0x2200)
			cmd := &MemCopyH2DCommand{
				ID:  timing.GetIDGenerator().Generate(),
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

			driver.Tick()
			driver.Tick()
			driver.Tick()

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
				ID:   timing.GetIDGenerator().Generate(),
				Dst:  Ptr(0x100),
				Src:  uint32(1),
				Reqs: []messaging.Msg{req, req2},
			}
			cmdQueue.Enqueue(cmd)
			cmdQueue.IsRunning = true

			deliverGeneralRsp(req.ID)

			driver.Tick()

			Expect(cmdQueue.IsRunning).To(BeTrue())
			Expect(cmdQueue.commands).To(HaveLen(1))
			Expect(cmd.Reqs).NotTo(ContainElement(req))
			Expect(cmd.Reqs).To(ContainElement(req2))
		})

		ginkgo.It("should remove command from queue if no more pending request",
			func() {
				req := makeMemCopyH2DReq(0x100, make([]byte, 4))
				cmd := &MemCopyH2DCommand{
					ID:   timing.GetIDGenerator().Generate(),
					Dst:  Ptr(0x100),
					Src:  uint32(1),
					Reqs: []messaging.Msg{req},
				}
				cmdQueue.Enqueue(cmd)
				cmdQueue.IsRunning = true

				deliverGeneralRsp(req.ID)

				driver.Tick()

				Expect(cmdQueue.IsRunning).To(BeFalse())
				Expect(cmdQueue.NumCommand()).To(Equal(0))
			})
	})

	ginkgo.Context("process MemCopyD2HCommand", func() {
		ginkgo.It("should send request", func() {
			data := uint32(1)
			cmd := &MemCopyD2HCommand{
				ID:  timing.GetIDGenerator().Generate(),
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

			driver.Tick()
			driver.Tick()
			driver.Tick()

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
				ID:   timing.GetIDGenerator().Generate(),
				Dst:  &data,
				Src:  Ptr(0x100),
				Reqs: []messaging.Msg{req, req2},
			}
			cmdQueue.Enqueue(cmd)
			cmdQueue.IsRunning = true

			deliverGeneralRsp(req.ID)

			driver.Tick()

			Expect(cmdQueue.IsRunning).To(BeTrue())
			Expect(cmdQueue.commands).To(HaveLen(1))
			Expect(cmd.Reqs).To(ContainElement(req2))
			Expect(cmd.Reqs).NotTo(ContainElement(req))
		})

		ginkgo.It("should continue queue", func() {
			data := uint32(0)
			req := makeMemCopyD2HReq(0x100, []byte{1, 0, 0, 0})
			cmd := &MemCopyD2HCommand{
				ID:      timing.GetIDGenerator().Generate(),
				Dst:     &data,
				RawData: []byte{1, 0, 0, 0},
				Src:     Ptr(0x100),
				Reqs:    []messaging.Msg{req},
			}
			cmdQueue.Enqueue(cmd)
			cmdQueue.IsRunning = true

			deliverGeneralRsp(req.ID)

			driver.Tick()

			Expect(cmdQueue.IsRunning).To(BeFalse())
			Expect(cmdQueue.commands).To(HaveLen(0))
			Expect(data).To(Equal(uint32(1)))
		})
	})

	ginkgo.Context("process LaunchKernelCommand", func() {
		ginkgo.It("should send request to GPU", func() {
			cmd := &LaunchKernelCommand{
				ID:         timing.GetIDGenerator().Generate(),
				CodeObject: nil,
				GridSize:   [3]uint32{256, 1, 1},
				WGSize:     [3]uint16{64, 1, 1},
				KernelArgs: nil,
			}
			cmdQueue.Enqueue(cmd)
			cmdQueue.IsRunning = false

			driver.Tick()

			Expect(cmdQueue.IsRunning).To(BeTrue())
			Expect(cmd.Reqs).To(HaveLen(1))
			req := cmd.Reqs[0].(protocol.LaunchKernelReq)
			Expect(req.PID).To(Equal(vm.PID(1)))
			Expect(driver.requestsToSend).To(HaveLen(1))
		})
	})

	ginkgo.It("should process LaunchKernel return", func() {
		req := protocol.LaunchKernelReq{
			MsgMeta: messaging.MsgMeta{
				ID:  timing.GetIDGenerator().Generate(),
				Src: toGPUs.AsRemote(),
				Dst: driver.GPUs[0],
			},
		}
		cmd := &LaunchKernelCommand{
			ID:   timing.GetIDGenerator().Generate(),
			Reqs: []messaging.Msg{req},
		}
		cmdQueue.Enqueue(cmd)
		cmdQueue.IsRunning = true

		rsp := protocol.LaunchKernelRsp{
			MsgMeta: messaging.MsgMeta{
				ID:    timing.GetIDGenerator().Generate(),
				Src:   driver.GPUs[0],
				Dst:   toGPUs.AsRemote(),
				RspTo: req.ID,
			},
		}
		toGPUs.Deliver(rsp)

		driver.Tick()

		Expect(cmdQueue.IsRunning).To(BeFalse())
		Expect(cmdQueue.commands).To(HaveLen(0))
	})
})
