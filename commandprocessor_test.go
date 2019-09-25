package gcn3

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3/pagemigrationcontroller"
	rdma2 "gitlab.com/akita/gcn3/rdma"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
	"gitlab.com/akita/mem/vm/addresstranslator"
	"gitlab.com/akita/mem/vm/tlb"
)

var _ = Describe("CommandProcessor", func() {

	var (
		mockCtrl            *gomock.Controller
		engine              *MockEngine
		driver              *MockPort
		dispatcher          *MockPort
		commandProcessor    *CommandProcessor
		toDriver            *MockPort
		toDispatcher        *MockPort
		cus                 []*MockPort
		toCU                *MockPort
		tlbs                []*MockPort
		toTLB               *MockPort
		addressTranslators  []*MockPort
		toAddressTranslator *MockPort
		toRDMA              *MockPort
		rdma                *MockPort
		toPMC               *MockPort
		pmc                 *MockPort
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		engine = NewMockEngine(mockCtrl)

		driver = NewMockPort(mockCtrl)
		dispatcher = NewMockPort(mockCtrl)
		toDriver = NewMockPort(mockCtrl)
		toDispatcher = NewMockPort(mockCtrl)
		commandProcessor = NewCommandProcessor("commandProcessor", engine)
		commandProcessor.numCUs = 10
		commandProcessor.ToDispatcher = toDispatcher
		commandProcessor.ToDriver = toDriver

		commandProcessor.Dispatcher = dispatcher
		commandProcessor.Driver = driver

		toCU = NewMockPort(mockCtrl)
		toTLB = NewMockPort(mockCtrl)
		toPMC = NewMockPort(mockCtrl)
		toRDMA = NewMockPort(mockCtrl)
		toAddressTranslator = NewMockPort(mockCtrl)

		commandProcessor.ToCUs = toCU
		commandProcessor.ToPMC = toPMC
		commandProcessor.ToAddressTranslators = toAddressTranslator
		commandProcessor.ToTLBs = toTLB
		commandProcessor.ToRDMA = toRDMA

		for i := 0; i < int(10); i++ {

			cus = append(cus, NewMockPort(mockCtrl))
			commandProcessor.CUs = append(commandProcessor.CUs, akita.NewLimitNumMsgPort(commandProcessor, 1))
			commandProcessor.CUs[i] = cus[i]

			tlbs = append(tlbs, NewMockPort(mockCtrl))
			commandProcessor.TLBs = append(commandProcessor.TLBs, akita.NewLimitNumMsgPort(commandProcessor, 1))
			commandProcessor.TLBs[i] = tlbs[i]

			addressTranslators = append(addressTranslators, NewMockPort(mockCtrl))
			commandProcessor.AddressTranslators = append(commandProcessor.AddressTranslators, akita.NewLimitNumMsgPort(commandProcessor, 1))
			commandProcessor.AddressTranslators[i] = addressTranslators[i]
		}

		rdma = NewMockPort(mockCtrl)
		commandProcessor.RDMA = rdma

		pmc = NewMockPort(mockCtrl)
		commandProcessor.PMC = pmc

	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should forward kernel launching request to Dispatcher", func() {
		req := NewLaunchKernelReq(10,
			driver, commandProcessor.ToDriver)
		req.EventTime = 10

		toDispatcher.EXPECT().Send(gomock.AssignableToTypeOf(req))

		commandProcessor.Handle(req)
	})

	It("should delay forward kernel launching request to the Driver", func() {
		req := NewLaunchKernelReq(10,
			dispatcher, commandProcessor.ToDispatcher)

		engine.EXPECT().Schedule(
			gomock.AssignableToTypeOf(&ReplyKernelCompletionEvent{}))

		commandProcessor.Handle(req)
	})
	It("should handle a RDMA drain req from driver", func() {
		req := NewRDMADrainCmdFromDriver(10, nil, commandProcessor.ToDriver)

		drainReq := rdma2.RDMADrainReqBuilder{}.Build()
		drainReq.SendTime = 10
		drainReq.Src = commandProcessor.ToRDMA
		drainReq.Dst = commandProcessor.RDMA
		toRDMA.EXPECT().Send(gomock.AssignableToTypeOf(drainReq))

		commandProcessor.Handle(req)
	})

	It("should handle a RDMA drain rsp from RDMA", func() {
		req := rdma2.RDMADrainRspBuilder{}.Build()
		req.SendTime = 10

		drainRsp := NewRDMADrainRspToDriver(10, commandProcessor.ToDriver, commandProcessor.Driver)
		toDriver.EXPECT().Send(gomock.AssignableToTypeOf(drainRsp))

		commandProcessor.Handle(req)
	})

	It("should handle a shootdown cmd from Driver", func() {
		vAddr := make([]uint64, 0)
		vAddr = append(vAddr, 100)
		req := NewShootdownCommand(10, nil, commandProcessor.ToDriver, vAddr, 1)

		for i := 0; i < 10; i++ {
			cuFlushReq := CUPipelineFlushReqBuilder{}.Build()
			cuFlushReq.SendTime = 10
			cuFlushReq.Src = commandProcessor.ToCUs
			cuFlushReq.Dst = commandProcessor.CUs[i]

			toCU.EXPECT().Send(gomock.AssignableToTypeOf(cuFlushReq))
		}

		commandProcessor.Handle(req)
		Expect(commandProcessor.numCUAck).To(Equal(uint64(10)))

	})

	It("should handle a CU pipeline flush rsp", func() {
		req := CUPipelineFlushRspBuilder{}.Build()
		req.SendTime = 10
		req.Dst = commandProcessor.ToCUs
		commandProcessor.numCUAck = 1

		for i := 0; i < 10; i++ {
			atFlushReq := addresstranslator.AddressTranslatorFlushReqBuilder{}.Build()
			atFlushReq.SendTime = 10
			atFlushReq.Src = commandProcessor.ToAddressTranslators
			atFlushReq.Dst = commandProcessor.AddressTranslators[i]

			toAddressTranslator.EXPECT().Send(gomock.AssignableToTypeOf(atFlushReq))
		}

		commandProcessor.Handle(req)
		Expect(commandProcessor.numAddrTranslationAck).To(Equal(uint64(10)))

	})

	It("should handle a AT  flush rsp", func() {

	})

	It("should handle a Cache flush rsp", func() {
		req := cache.FlushRspBuilder{}.Build()
		req.Dst = commandProcessor.ToDispatcher
		req.SendTime = 10
		vAddr := make([]uint64, 0)
		vAddr = append(vAddr, 100)
		shootDwnCmd := NewShootdownCommand(10, nil, commandProcessor.ToDriver, vAddr, 1)

		commandProcessor.numCacheACK = 1
		commandProcessor.shootDownInProcess = true
		commandProcessor.curShootdownRequest = shootDwnCmd

		for i := 0; i < 10; i++ {
			tlbFlushReq := tlb.TLBFlushReqBuilder{}.Build()
			tlbFlushReq.SendTime = 10
			tlbFlushReq.Src = commandProcessor.ToTLBs
			tlbFlushReq.Dst = commandProcessor.TLBs[i]

			toTLB.EXPECT().Send(gomock.AssignableToTypeOf(tlbFlushReq))
		}

		commandProcessor.Handle(req)

		Expect(commandProcessor.numTLBAck).To(Equal(uint64(10)))

	})

	It("should handle a TLB flush rsp", func() {
		req := tlb.TLBFlushRspBuilder{}.Build()
		req.SendTime = 10
		req.Dst = commandProcessor.ToTLBs

		commandProcessor.numTLBAck = 1

		rsp := NewShootdownCompleteRsp(10, commandProcessor.ToDriver, commandProcessor.Driver)
		toDriver.EXPECT().Send(gomock.AssignableToTypeOf(rsp))

		commandProcessor.Handle(req)
		Expect(commandProcessor.shootDownInProcess).To(BeFalse())

	})

	It("should handle a GPU restart req", func() {

	})

	It("should handle a cache restart rsp", func() {
		req := cache.RestartRspBuilder{}.Build()
		req.SendTime = 10
		req.Dst = commandProcessor.ToDispatcher

		commandProcessor.numCacheACK = 1

		for i := 0; i < 10; i++ {
			tlbRestartReq := tlb.TLBRestartReqBuilder{}.Build()
			tlbRestartReq.SendTime = 10
			tlbRestartReq.Src = commandProcessor.ToTLBs
			tlbRestartReq.Dst = commandProcessor.TLBs[i]

			toTLB.EXPECT().Send(gomock.AssignableToTypeOf(tlbRestartReq))
		}

		commandProcessor.Handle(req)
		Expect(commandProcessor.numTLBAck).To(Equal(uint64(10)))
	})
	It("should handle a TLB restart rsp", func() {
		req := tlb.TLBRestartRspBuilder{}.Build()
		req.SendTime = 10
		req.Dst = commandProcessor.ToTLBs

		commandProcessor.numTLBAck = 1

		for i := 0; i < 10; i++ {
			atRestartReq := addresstranslator.AddressTranslatorRestartReqBuilder{}.Build()
			atRestartReq.SendTime = 10
			atRestartReq.Src = commandProcessor.ToAddressTranslators
			atRestartReq.Dst = commandProcessor.AddressTranslators[i]

			toAddressTranslator.EXPECT().Send(gomock.AssignableToTypeOf(atRestartReq))
		}

		commandProcessor.Handle(req)
		Expect(commandProcessor.numAddrTranslationAck).To(Equal(uint64(10)))

	})
	It("should handle a AT restart rsp", func() {
		req := addresstranslator.AddressTranslatorRestartRspBuilder{}.Build()
		req.SendTime = 10
		req.Dst = commandProcessor.ToAddressTranslators

		commandProcessor.numAddrTranslationAck = 1

		for i := 0; i < 10; i++ {
			cuRestartReq := CUPipelineRestartReqBuilder{}.Build()
			cuRestartReq.SendTime = 10
			cuRestartReq.Src = commandProcessor.ToCUs
			cuRestartReq.Dst = commandProcessor.CUs[i]
			toCU.EXPECT().Send(gomock.AssignableToTypeOf(cuRestartReq))

		}

		commandProcessor.Handle(req)
		Expect(commandProcessor.numCUAck).To(Equal(uint64(10)))

	})

	It("should handle a CU pipeline restart rsp", func() {
		req := CUPipelineRestartRspBuilder{}.Build()
		req.SendTime = 10
		req.Dst = commandProcessor.ToCUs

		commandProcessor.numCUAck = 1

		gpuRestartRsp := NewGPURestartRsp(10, commandProcessor.ToDriver, commandProcessor.Driver)
		toDriver.EXPECT().Send(gomock.AssignableToTypeOf(gpuRestartRsp))

		commandProcessor.Handle(req)
	})

	It("should handle a page migration req", func() {

		req := NewPageMigrationReqToCP(10, nil, commandProcessor.ToDriver)
		req.ToWriteToPhysicalAddress = 0x100
		req.ToReadFromPhysicalAddress = 0x20
		remotePMC := NewMockPort(mockCtrl)
		req.DestinationPMCPort = remotePMC
		req.PageSize = 4 * mem.KB

		reqToPMC := pagemigrationcontroller.PageMigrationReqToPMCBuilder{}.Build()
		req.SendTime = 10
		reqToPMC.PageSize = req.PageSize
		reqToPMC.ToReadFromPhysicalAddress = req.ToReadFromPhysicalAddress
		reqToPMC.ToWriteToPhysicalAddress = req.ToWriteToPhysicalAddress
		reqToPMC.PMCPortOfRemoteGPU = req.DestinationPMCPort

		toPMC.EXPECT().Send(gomock.AssignableToTypeOf(reqToPMC))

		commandProcessor.Handle(req)

	})

	It("should handle a page migration rsp", func() {
		req := pagemigrationcontroller.PageMigrationRspFromPMCBuilder{}.Build()
		req.SendTime = 10
		req.Dst = commandProcessor.ToPMC

		rsp := NewPageMigrationRspToDriver(10, commandProcessor.ToDriver, commandProcessor.Driver)

		toDriver.EXPECT().Send(gomock.AssignableToTypeOf(rsp))

		commandProcessor.Handle(req)

	})

})
