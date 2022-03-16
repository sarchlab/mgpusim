package driver

import (
	"github.com/golang/mock/gomock"
	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/akita/v3/sim"
	"gitlab.com/akita/mem/v3/mem"
	"gitlab.com/akita/mem/v3/vm"
	"gitlab.com/akita/mgpusim/v2/protocol"
)

var _ = ginkgo.Describe("Driver", func() {

	var (
		mockCtrl       *gomock.Controller
		pageTable      *MockPageTable
		driver         *Driver
		engine         *MockEngine
		toGPUs         *MockPort
		toMMU          *MockPort
		remotePMCPorts []*MockPort
		context        *Context
		cmdQueue       *CommandQueue
		memAllocator   *MockMemoryAllocator
		log2PageSize   uint64
	)

	ginkgo.BeforeEach(func() {
		mockCtrl = gomock.NewController(ginkgo.GinkgoT())
		engine = NewMockEngine(mockCtrl)
		toGPUs = NewMockPort(mockCtrl)
		pageTable = NewMockPageTable(mockCtrl)
		toMMU = NewMockPort(mockCtrl)
		memAllocator = NewMockMemoryAllocator(mockCtrl)
		memAllocator.EXPECT().RegisterDevice(gomock.Any()).AnyTimes()
		log2PageSize = 12

		driver = MakeBuilder().
			WithEngine(engine).
			WithLog2PageSize(log2PageSize).
			WithPageTable(pageTable).
			Build("Driver")
		driver.gpuPort = toGPUs
		driver.mmuPort = toMMU
		driver.memAllocator = memAllocator

		for i := 0; i < 2; i++ {
			gpu := NewMockPort(mockCtrl)
			remotePMCPorts = append(remotePMCPorts, NewMockPort(mockCtrl))
			driver.RemotePMCPorts = append(driver.RemotePMCPorts,
				sim.NewLimitNumMsgPort(driver, 1, ""))
			driver.RemotePMCPorts[i] = remotePMCPorts[i]
			driver.RegisterGPU(gpu, 4*mem.GB)
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

			toGPUs.EXPECT().Peek().Return(nil).AnyTimes()
			toMMU.EXPECT().Retrieve(sim.VTimeInSec(11)).Return(nil)

			engine.EXPECT().Schedule(gomock.AssignableToTypeOf(sim.TickEvent{}))

			driver.Handle(sim.MakeTickEvent(11, nil))

			Expect(driver.requestsToSend).To(HaveLen(4))
			Expect(cmdQueue.IsRunning).To(BeTrue())
			Expect(cmd.Reqs).To(HaveLen(4))
		})
	})

	ginkgo.Context("process MemCopyH2D return", func() {
		ginkgo.It("should remove one request", func() {
			req := protocol.NewMemCopyH2DReq(9, toGPUs, nil,
				make([]byte, 4), 0x104)
			req2 := protocol.NewMemCopyH2DReq(9, toGPUs, nil,
				make([]byte, 4), 0x100)
			cmd := &MemCopyH2DCommand{
				Dst:  Ptr(0x100),
				Src:  uint32(1),
				Reqs: []sim.Msg{req, req2},
			}
			cmdQueue.Enqueue(cmd)
			cmdQueue.IsRunning = true

			toGPUs.EXPECT().Peek().Return(req)
			toGPUs.EXPECT().Peek().Return(nil)
			toGPUs.EXPECT().
				Retrieve(sim.VTimeInSec(11)).
				Return(req)
			toMMU.EXPECT().Retrieve(sim.VTimeInSec(11)).Return(nil)

			engine.EXPECT().
				Schedule(gomock.AssignableToTypeOf(sim.TickEvent{}))

			driver.Handle(sim.MakeTickEvent(11, nil))

			Expect(cmdQueue.IsRunning).To(BeTrue())
			Expect(cmdQueue.commands).To(HaveLen(1))
			Expect(cmd.Reqs).NotTo(ContainElement(req))
			Expect(cmd.Reqs).To(ContainElement(req2))
		})

		ginkgo.It("should remove command from queue if no more pending request", func() {
			req := protocol.NewMemCopyH2DReq(9,
				toGPUs, nil,
				make([]byte, 4), 0x100)
			cmd := &MemCopyH2DCommand{
				Dst:  Ptr(0x100),
				Src:  uint32(1),
				Reqs: []sim.Msg{req},
			}
			cmdQueue.Enqueue(cmd)
			cmdQueue.IsRunning = true

			toGPUs.EXPECT().Peek().Return(req)
			toGPUs.EXPECT().Peek().Return(nil)
			toGPUs.EXPECT().
				Retrieve(sim.VTimeInSec(11)).
				Return(req)

			toMMU.EXPECT().Retrieve(sim.VTimeInSec(11)).Return(nil)

			engine.EXPECT().Schedule(
				gomock.AssignableToTypeOf(sim.TickEvent{}))

			driver.Handle(sim.MakeTickEvent(11, nil))

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

			toGPUs.EXPECT().Peek().Return(nil).AnyTimes()
			toMMU.EXPECT().Retrieve(sim.VTimeInSec(11)).Return(nil)

			engine.EXPECT().Schedule(
				gomock.AssignableToTypeOf(sim.TickEvent{}))

			driver.Handle(sim.MakeTickEvent(11, nil))

			Expect(cmdQueue.IsRunning).To(BeTrue())
			Expect(cmd.Reqs).To(HaveLen(1))
			Expect(driver.requestsToSend).To(HaveLen(1))
		})
	})

	ginkgo.Context("process MemCopyD2H return", func() {
		ginkgo.It("should remove request", func() {
			data := uint64(0)
			req := protocol.NewMemCopyD2HReq(
				9, nil, toGPUs, 0x100, []byte{1, 0, 0, 0})
			req2 := protocol.NewMemCopyD2HReq(
				9, nil, toGPUs, 0x104, []byte{1, 0, 0, 0})
			cmd := &MemCopyD2HCommand{
				Dst:  &data,
				Src:  Ptr(0x100),
				Reqs: []sim.Msg{req, req2},
			}
			cmdQueue.Enqueue(cmd)
			cmdQueue.IsRunning = true

			toGPUs.EXPECT().Peek().Return(req)
			toGPUs.EXPECT().Peek().Return(nil)
			toGPUs.EXPECT().
				Retrieve(sim.VTimeInSec(11)).
				Return(req)
			toMMU.EXPECT().Retrieve(sim.VTimeInSec(11)).Return(nil)

			engine.EXPECT().Schedule(
				gomock.AssignableToTypeOf(sim.TickEvent{}))

			driver.Handle(sim.MakeTickEvent(11, nil))

			Expect(cmdQueue.IsRunning).To(BeTrue())
			Expect(cmdQueue.commands).To(HaveLen(1))
			Expect(cmd.Reqs).To(ContainElement(req2))
			Expect(cmd.Reqs).NotTo(ContainElement(req))
		})

		ginkgo.It("should continue queue", func() {
			data := uint32(0)
			req := protocol.NewMemCopyD2HReq(9, nil, toGPUs,
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

			toGPUs.EXPECT().Peek().Return(req)
			toGPUs.EXPECT().Peek().Return(nil)
			toGPUs.EXPECT().
				Retrieve(sim.VTimeInSec(11)).
				Return(req)
			toMMU.EXPECT().Retrieve(sim.VTimeInSec(11)).Return(nil)

			engine.EXPECT().Schedule(gomock.AssignableToTypeOf(sim.TickEvent{}))

			driver.Handle(sim.MakeTickEvent(11, nil))

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

			toGPUs.EXPECT().Peek().Return(nil).AnyTimes()

			toMMU.EXPECT().Retrieve(sim.VTimeInSec(11)).Return(nil)

			engine.EXPECT().Schedule(
				gomock.AssignableToTypeOf(sim.TickEvent{}))

			driver.Handle(sim.MakeTickEvent(11, nil))

			Expect(cmdQueue.IsRunning).To(BeTrue())
			Expect(cmd.Reqs).To(HaveLen(1))
			req := cmd.Reqs[0].(*protocol.LaunchKernelReq)
			Expect(req.PID).To(Equal(vm.PID(1)))
			Expect(driver.requestsToSend).To(HaveLen(1))
		})
	})

	ginkgo.It("should process LaunchKernel return", func() {
		req := protocol.NewLaunchKernelReq(9, toGPUs, nil)
		cmd := &LaunchKernelCommand{
			Reqs: []sim.Msg{req},
		}
		cmdQueue.Enqueue(cmd)
		cmdQueue.IsRunning = true

		toGPUs.EXPECT().Peek().Return(req).Times(2)
		toGPUs.EXPECT().
			Retrieve(sim.VTimeInSec(11)).
			Return(req)

		toMMU.EXPECT().Retrieve(sim.VTimeInSec(11)).Return(nil)

		engine.EXPECT().Schedule(gomock.AssignableToTypeOf(sim.TickEvent{}))

		driver.Handle(sim.MakeTickEvent(11, nil))

		Expect(cmdQueue.IsRunning).To(BeFalse())
		Expect(cmdQueue.commands).To(HaveLen(0))
	})
	ginkgo.It("should handle page migration req from MMU ", func() {
		req := vm.NewPageMigrationReqToDriver(10, nil, driver.mmuPort)
		toMMU.EXPECT().Retrieve(sim.VTimeInSec(10)).Return(req)
		driver.isCurrentlyHandlingMigrationReq = false

		for i := 0; i < 2; i++ {
			rdmaDrainReq := protocol.NewRDMADrainCmdFromDriver(10, driver.gpuPort, driver.GPUs[i])
			driver.requestsToSend = append(driver.requestsToSend, rdmaDrainReq)

		}

		driver.parseFromMMU(10)

		Expect(driver.currentPageMigrationReq).To(Equal(req))
		Expect(driver.isCurrentlyHandlingMigrationReq).To(BeTrue())
		Expect(driver.numRDMADrainACK).To(Equal(uint64(2)))
	})

	ginkgo.It("should handle RDMA Drain RSP ", func() {
		req := protocol.NewRDMADrainRspToDriver(10, nil, driver.gpuPort)
		driver.numRDMADrainACK = 1

		pageMigrationReq := vm.NewPageMigrationReqToDriver(
			10, nil, driver.mmuPort)
		pageMigrationReq.PageSize = 4 * mem.KB
		pageMigrationReq.CurrPageHostGPU = 1
		pageMigrationReq.CurrAccessingGPUs = append(
			pageMigrationReq.CurrAccessingGPUs, 1)
		GPUReqToVAddrMap := make(map[uint64][]uint64)
		GPUReqToVAddrMap[2] = append(GPUReqToVAddrMap[2], 0x100)
		migrationInfo := new(vm.PageMigrationInfo)
		migrationInfo.GPUReqToVAddrMap = GPUReqToVAddrMap
		pageMigrationReq.MigrationInfo = migrationInfo

		driver.currentPageMigrationReq = pageMigrationReq

		toGPUs.EXPECT().Peek().Return(req)
		toGPUs.EXPECT().Retrieve(sim.VTimeInSec(10)).Return(req)

		madeProgress := driver.processReturnReq(10)

		Expect(driver.numShootDownACK).To(Equal(uint64(1)))
		Expect(madeProgress).To(BeTrue())
		Expect(len(driver.requestsToSend)).To(Equal(1))

	})

	ginkgo.It("should handle shootdown complete rsp", func() {
		req := protocol.NewShootdownCompleteRsp(10, nil, driver.gpuPort)

		pageMigrationReq := vm.NewPageMigrationReqToDriver(
			10, nil, driver.mmuPort)
		pageMigrationReq.PageSize = 4 * mem.KB
		pageMigrationReq.CurrPageHostGPU = 1
		pageMigrationReq.CurrAccessingGPUs =
			append(pageMigrationReq.CurrAccessingGPUs, 1)
		GPUReqToVaddrMap := make(map[uint64][]uint64)
		GPUReqToVaddrMap[2] = append(GPUReqToVaddrMap[2], 0x100)
		migrationInfo := new(vm.PageMigrationInfo)
		migrationInfo.GPUReqToVAddrMap = GPUReqToVaddrMap
		pageMigrationReq.MigrationInfo = migrationInfo
		driver.currentPageMigrationReq = pageMigrationReq
		driver.numShootDownACK = 1

		page2 := &vm.Page{
			PID:      0,
			VAddr:    0x100,
			PAddr:    8589934592,
			PageSize: 4096,
			Valid:    true,
			DeviceID: 2,
			Unified:  true,
		}

		pageTable.EXPECT().
			Find(vm.PID(0), uint64(0x100)).
			Return(vm.Page{
				PID:      0,
				VAddr:    0x100,
				PAddr:    4294967296,
				PageSize: 0x1000,
				Valid:    true,
				DeviceID: 1,
				Unified:  true,
			}, true)
		pageTable.EXPECT().Update(vm.Page{
			PID:         0,
			VAddr:       0x100,
			PAddr:       8589934592,
			PageSize:    0x1000,
			Valid:       true,
			DeviceID:    2,
			Unified:     true,
			IsMigrating: true,
		})
		memAllocator.EXPECT().
			AllocatePageWithGivenVAddr(vm.PID(0), 2, uint64(0x100), true).
			Return(*page2)

		toGPUs.EXPECT().Peek().Return(req)
		toGPUs.EXPECT().Retrieve(sim.VTimeInSec(10)).Return(req)

		driver.processReturnReq(10)

		Expect(driver.numPagesMigratingACK).
			To(Equal(uint64(1)))
		Expect(driver.migrationReqToSendToCP[0].Dst).
			To(Equal(driver.GPUs[1]))
		Expect(driver.migrationReqToSendToCP[0].DestinationPMCPort).
			To(Equal(driver.RemotePMCPorts[0]))
		Expect(driver.migrationReqToSendToCP[0].ToReadFromPhysicalAddress).
			To(Equal(uint64(4294967296)))
		Expect(driver.migrationReqToSendToCP[0].ToWriteToPhysicalAddress).
			To(Equal(uint64(8589934592)))
		Expect(driver.migrationReqToSendToCP[0].PageSize).
			To(Equal(4 * mem.KB))

	})

	ginkgo.It("should send migration req to CP", func() {
		migrationReqToCP :=
			protocol.NewPageMigrationReqToCP(10, driver.gpuPort,
				driver.GPUs[1])
		driver.migrationReqToSendToCP = append(driver.migrationReqToSendToCP, migrationReqToCP)

		toGPUs.EXPECT().Send(migrationReqToCP)

		madeProgress := driver.sendMigrationReqToCP(10)

		Expect(driver.isCurrentlyMigratingOnePage).To(BeTrue())
		Expect(madeProgress).To(BeTrue())
	})

	ginkgo.It("should process page migration rsp from CP", func() {
		req := protocol.NewPageMigrationRspToDriver(10, nil, driver.gpuPort)

		toGPUs.EXPECT().Peek().Return(req)
		toGPUs.EXPECT().Retrieve(sim.VTimeInSec(10)).Return(req)

		driver.numPagesMigratingACK = 2
		driver.processReturnReq(10)

		Expect(driver.numPagesMigratingACK).To(Equal(uint64(1)))
		Expect(driver.isCurrentlyMigratingOnePage).To(BeFalse())

	})

	ginkgo.It("should process page migration rsp from CP and send restart reqs to GPU and reply to MMU", func() {
		req := protocol.NewPageMigrationRspToDriver(10, nil, driver.gpuPort)
		toGPUs.EXPECT().Peek().Return(req)
		toGPUs.EXPECT().Retrieve(sim.VTimeInSec(10)).Return(req)

		driver.numPagesMigratingACK = 1

		pageMigrationReq := vm.NewPageMigrationReqToDriver(10, nil, driver.mmuPort)
		pageMigrationReq.PageSize = 4 * mem.KB
		pageMigrationReq.CurrPageHostGPU = 1
		pageMigrationReq.CurrAccessingGPUs = append(pageMigrationReq.CurrAccessingGPUs, 1)
		pageMigrationReq.RespondToTop = true
		GpuReqToVaddrMap := make(map[uint64][]uint64)
		GpuReqToVaddrMap[2] = append(GpuReqToVaddrMap[2], 0x100)
		migrationInfo := new(vm.PageMigrationInfo)
		migrationInfo.GPUReqToVAddrMap = GpuReqToVaddrMap
		pageMigrationReq.MigrationInfo = migrationInfo
		driver.currentPageMigrationReq = pageMigrationReq

		reqToMMU := vm.NewPageMigrationRspFromDriver(10,
			driver.mmuPort, pageMigrationReq.Src)
		reqToMMU.VAddr = append(reqToMMU.VAddr, 0x100)
		reqToMMU.RspToTop = true

		driver.processReturnReq(10)

		Expect(driver.toSendToMMU).To(BeEquivalentTo(reqToMMU))
		Expect(driver.requestsToSend).To(HaveLen(1))
	})

	ginkgo.It("should process GPU restart rsp and send restart req to RDMAs", func() {
		req := protocol.NewGPURestartRsp(10, nil, driver.gpuPort)
		toGPUs.EXPECT().Peek().Return(req)
		toGPUs.EXPECT().Retrieve(sim.VTimeInSec(10)).Return(req)

		driver.numRestartACK = 1

		pageMigrationReq := vm.NewPageMigrationReqToDriver(10, nil, driver.mmuPort)
		pageMigrationReq.PageSize = 4 * mem.KB
		pageMigrationReq.CurrPageHostGPU = 1
		pageMigrationReq.CurrAccessingGPUs = append(pageMigrationReq.CurrAccessingGPUs, 1)
		pageMigrationReq.RespondToTop = true
		GpuReqToVaddrMap := make(map[uint64][]uint64)
		GpuReqToVaddrMap[2] = append(GpuReqToVaddrMap[2], 0x100)
		migrationInfo := new(vm.PageMigrationInfo)
		migrationInfo.GPUReqToVAddrMap = GpuReqToVaddrMap
		pageMigrationReq.MigrationInfo = migrationInfo
		driver.currentPageMigrationReq = pageMigrationReq

		driver.processReturnReq(10)

		Expect(driver.requestsToSend).To(HaveLen(2))
	})

	ginkgo.It("should handle rdma restart rsp", func() {
		req := protocol.NewRDMARestartRspToDriver(10, nil, driver.gpuPort)
		toGPUs.EXPECT().Peek().Return(req)
		toGPUs.EXPECT().Retrieve(sim.VTimeInSec(10)).Return(req)

		driver.numRDMARestartACK = 1

		pageMigrationReq := vm.NewPageMigrationReqToDriver(10, nil, driver.mmuPort)
		pageMigrationReq.PageSize = 4 * mem.KB
		pageMigrationReq.CurrPageHostGPU = 1
		pageMigrationReq.CurrAccessingGPUs = append(pageMigrationReq.CurrAccessingGPUs, 1)
		pageMigrationReq.RespondToTop = true
		GpuReqToVaddrMap := make(map[uint64][]uint64)
		GpuReqToVaddrMap[2] = append(GpuReqToVaddrMap[2], 0x100)
		migrationInfo := new(vm.PageMigrationInfo)
		migrationInfo.GPUReqToVAddrMap = GpuReqToVaddrMap
		pageMigrationReq.MigrationInfo = migrationInfo
		driver.currentPageMigrationReq = pageMigrationReq

		driver.processReturnReq(10)

		Expect(driver.currentPageMigrationReq).To(BeNil())
		Expect(driver.isCurrentlyHandlingMigrationReq).To(BeFalse())
	})

	ginkgo.It("should send to MMU", func() {
		reqToMMU := vm.NewPageMigrationRspFromDriver(10, driver.mmuPort, nil)
		driver.toSendToMMU = reqToMMU

		toMMU.EXPECT().Send(reqToMMU)

		madeProgress := driver.sendToMMU(10)

		Expect(madeProgress).To(BeTrue())
		Expect(driver.toSendToMMU).To(BeNil())
	})
})
