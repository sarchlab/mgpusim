package cp

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/mem/cache"
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/mem/vm/tlb"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/v4/protocol"
	"github.com/sarchlab/mgpusim/v4/timing/cp/internal/dispatching"
	"github.com/sarchlab/mgpusim/v4/timing/pagemigrationcontroller"
	"github.com/sarchlab/mgpusim/v4/timing/rdma"
)

var _ = Describe("CommandProcessor", func() {

	var (
		mockCtrl           *gomock.Controller
		engine             *MockEngine
		driver             *MockPort
		dispatcher         *MockDispatcher
		commandProcessor   *CommandProcessor
		cus                []*MockPort
		toCU               *MockPort
		tlbs               []*MockPort
		addressTranslators []*MockPort
		rdmaPort           *MockPort
		pmc                *MockPort
		l1VCaches          []*MockPort
		l1SCaches          []*MockPort
		l1ICaches          []*MockPort
		l2Caches           []*MockPort

		toDriver            *MockPort
		toDMA               *MockPort
		toCaches            *MockPort
		toTLB               *MockPort
		toRDMA              *MockPort
		toAddressTranslator *MockPort
		toPMC               *MockPort

		toDriverSender            *MockBufferedSender
		toCUsSender               *MockBufferedSender
		toDispatcherSender        *MockBufferedSender
		toCachesSender            *MockBufferedSender
		toTLBSender               *MockBufferedSender
		toRDMASender              *MockBufferedSender
		toAddressTranslatorSender *MockBufferedSender
		toPMCSender               *MockBufferedSender
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		engine = NewMockEngine(mockCtrl)

		driver = NewMockPort(mockCtrl)
		dispatcher = NewMockDispatcher(mockCtrl)
		toDriver = NewMockPort(mockCtrl)
		toDMA = NewMockPort(mockCtrl)
		commandProcessor = MakeBuilder().
			WithEngine(engine).
			WithFreq(1).
			Build("CP")
		commandProcessor.ToDMA = toDMA
		commandProcessor.ToDriver = toDriver

		commandProcessor.Dispatchers = []dispatching.Dispatcher{dispatcher}
		commandProcessor.Driver = driver

		toCU = NewMockPort(mockCtrl)
		toTLB = NewMockPort(mockCtrl)
		toPMC = NewMockPort(mockCtrl)
		toRDMA = NewMockPort(mockCtrl)
		toAddressTranslator = NewMockPort(mockCtrl)
		toCaches = NewMockPort(mockCtrl)

		commandProcessor.ToCUs = toCU
		commandProcessor.ToPMC = toPMC
		commandProcessor.ToAddressTranslators = toAddressTranslator
		commandProcessor.ToTLBs = toTLB
		commandProcessor.ToRDMA = toRDMA
		commandProcessor.ToCaches = toCaches

		toDriverSender = NewMockBufferedSender(mockCtrl)
		toDispatcherSender = NewMockBufferedSender(mockCtrl)
		toCUsSender = NewMockBufferedSender(mockCtrl)
		toAddressTranslatorSender = NewMockBufferedSender(mockCtrl)
		toTLBSender = NewMockBufferedSender(mockCtrl)
		toCachesSender = NewMockBufferedSender(mockCtrl)
		toPMCSender = NewMockBufferedSender(mockCtrl)
		toRDMASender = NewMockBufferedSender(mockCtrl)

		commandProcessor.toDriverSender = toDriverSender
		commandProcessor.toDMASender = toDispatcherSender
		commandProcessor.toCUsSender = toCUsSender
		commandProcessor.toAddressTranslatorsSender = toAddressTranslatorSender
		commandProcessor.toTLBsSender = toTLBSender
		commandProcessor.toCachesSender = toCachesSender
		commandProcessor.toPMCSender = toPMCSender
		commandProcessor.toRDMASender = toRDMASender

		for i := 0; i < int(10); i++ {
			cus = append(cus, NewMockPort(mockCtrl))
			commandProcessor.CUs = append(commandProcessor.CUs,
				sim.NewLimitNumMsgPort(commandProcessor, 1, "CUPort"))
			commandProcessor.CUs[i] = cus[i]

			tlbs = append(tlbs, NewMockPort(mockCtrl))
			commandProcessor.TLBs = append(commandProcessor.TLBs,
				sim.NewLimitNumMsgPort(commandProcessor, 1, "TLBPort"))
			commandProcessor.TLBs[i] = tlbs[i]

			addressTranslators = append(addressTranslators,
				NewMockPort(mockCtrl))
			commandProcessor.AddressTranslators =
				append(commandProcessor.AddressTranslators,
					sim.NewLimitNumMsgPort(commandProcessor, 1, "ATPort"))
			commandProcessor.AddressTranslators[i] = addressTranslators[i]

			l1ICaches = append(l1ICaches, NewMockPort(mockCtrl))
			commandProcessor.L1ICaches = append(commandProcessor.L1ICaches,
				sim.NewLimitNumMsgPort(commandProcessor, 1, "L1ICachePort"))
			commandProcessor.L1ICaches[i] = l1ICaches[i]

			l1SCaches = append(l1SCaches, NewMockPort(mockCtrl))
			commandProcessor.L1SCaches = append(commandProcessor.L1SCaches,
				sim.NewLimitNumMsgPort(commandProcessor, 1, "L1SCachePort"))
			commandProcessor.L1SCaches[i] = l1SCaches[i]

			l1VCaches = append(l1VCaches, NewMockPort(mockCtrl))
			commandProcessor.L1VCaches = append(commandProcessor.L1VCaches,
				sim.NewLimitNumMsgPort(commandProcessor, 1, "L1VCachePort"))
			commandProcessor.L1VCaches[i] = l1VCaches[i]

			l2Caches = append(l2Caches, NewMockPort(mockCtrl))
			commandProcessor.L2Caches = append(commandProcessor.L2Caches,
				sim.NewLimitNumMsgPort(commandProcessor, 1, "L2CachePort"))
			commandProcessor.L2Caches[i] = l2Caches[i]
		}

		rdmaPort = NewMockPort(mockCtrl)
		commandProcessor.RDMA = rdmaPort

		pmc = NewMockPort(mockCtrl)
		commandProcessor.PMC = pmc
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should forward kernel launching request to a Dispatcher", func() {
		req := protocol.NewLaunchKernelReq(driver, commandProcessor.ToDriver)

		dispatcher.EXPECT().IsDispatching().Return(false)
		dispatcher.EXPECT().StartDispatching(req)
		toDriver.EXPECT().RetrieveIncoming()

		madeProgress := commandProcessor.processLaunchKernelReq(req)

		Expect(madeProgress).To(BeTrue())
	})

	It("should wait if there is no dispatcher available", func() {
		req := protocol.NewLaunchKernelReq(driver, commandProcessor.ToDriver)

		dispatcher.EXPECT().IsDispatching().Return(true)

		madeProgress := commandProcessor.processLaunchKernelReq(req)

		Expect(madeProgress).To(BeFalse())
	})

	It("should handle a RDMA drain req from driver", func() {
		cmd := protocol.NewRDMADrainCmdFromDriver(nil, commandProcessor.ToDriver)
		drainReq := rdma.DrainReqBuilder{}.Build()

		toRDMASender.EXPECT().Send(gomock.AssignableToTypeOf(drainReq))
		toDriver.EXPECT().RetrieveIncoming()

		madeProgress := commandProcessor.processRDMADrainCmd(cmd)

		Expect(madeProgress).To(BeTrue())
	})

	It("should handle a RDMA drain rsp from RDMA", func() {
		req := rdma.DrainRspBuilder{}.Build()

		drainRsp := protocol.NewRDMADrainRspToDriver(
			commandProcessor.ToDriver, commandProcessor.Driver)
		toDriverSender.EXPECT().Send(gomock.AssignableToTypeOf(drainRsp))
		toRDMA.EXPECT().RetrieveIncoming()

		madeProgress := commandProcessor.processRDMADrainRsp(req)

		Expect(madeProgress).To(BeTrue())
	})

	It("should handle a shootdown cmd from Driver", func() {
		vAddr := make([]uint64, 0)
		vAddr = append(vAddr, 100)
		cmd := protocol.NewShootdownCommand(
			nil, commandProcessor.ToDriver, vAddr, 1)

		for i := 0; i < 10; i++ {
			cuFlushReq := protocol.CUPipelineFlushReqBuilder{}.Build()
			cuFlushReq.Src = commandProcessor.ToCUs
			cuFlushReq.Dst = commandProcessor.CUs[i]
			toCUsSender.EXPECT().Send(gomock.AssignableToTypeOf(cuFlushReq))
		}
		toDriver.EXPECT().RetrieveIncoming()

		madeProgress := commandProcessor.processShootdownCommand(cmd)

		Expect(commandProcessor.numCUAck).To(Equal(uint64(10)))
		Expect(madeProgress).To(BeTrue())
	})

	It("should handle a CU pipeline flush rsp", func() {
		req := protocol.CUPipelineFlushRspBuilder{}.Build()
		req.Dst = commandProcessor.ToCUs
		commandProcessor.numCUAck = 1

		for i := 0; i < 10; i++ {
			toAddressTranslatorSender.EXPECT().Send(gomock.Any())
		}
		toCU.EXPECT().RetrieveIncoming()

		madeProgress := commandProcessor.processCUPipelineFlushRsp(req)

		Expect(commandProcessor.numAddrTranslationFlushAck).
			To(Equal(uint64(10)))
		Expect(madeProgress).To(BeTrue())
	})

	It("should handle a AT flush rsp", func() {
		req := mem.ControlMsgBuilder{}.
			WithDst(commandProcessor.ToAddressTranslators).
			ToNotifyDone().
			Build()
		commandProcessor.numAddrTranslationFlushAck = 1

		for i := 0; i < 10; i++ {
			cacheFlushReq := cache.FlushReqBuilder{}.Build()
			cacheFlushReq.Src = commandProcessor.ToCaches
			cacheFlushReq.Dst = commandProcessor.L1ICaches[i]
			cacheFlushReq.DiscardInflight = true
			cacheFlushReq.PauseAfterFlushing = true
			cacheFlushReq.InvalidateAllCachelines = true
			toCachesSender.EXPECT().
				Send(gomock.AssignableToTypeOf(cacheFlushReq))
		}

		for i := 0; i < 10; i++ {
			cacheFlushReq := cache.FlushReqBuilder{}.Build()
			cacheFlushReq.Src = commandProcessor.ToCaches
			cacheFlushReq.Dst = commandProcessor.L1VCaches[i]
			cacheFlushReq.DiscardInflight = true
			cacheFlushReq.PauseAfterFlushing = true
			cacheFlushReq.InvalidateAllCachelines = true
			toCachesSender.EXPECT().
				Send(gomock.AssignableToTypeOf(cacheFlushReq))
		}

		for i := 0; i < 10; i++ {
			cacheFlushReq := cache.FlushReqBuilder{}.Build()
			cacheFlushReq.Src = commandProcessor.ToCaches
			cacheFlushReq.Dst = commandProcessor.L1VCaches[i]
			cacheFlushReq.DiscardInflight = true
			cacheFlushReq.PauseAfterFlushing = true
			cacheFlushReq.InvalidateAllCachelines = true
			toCachesSender.EXPECT().
				Send(gomock.AssignableToTypeOf(cacheFlushReq))
		}

		for i := 0; i < 10; i++ {
			cacheFlushReq := cache.FlushReqBuilder{}.Build()
			cacheFlushReq.Src = commandProcessor.ToCaches
			cacheFlushReq.Dst = commandProcessor.L2Caches[i]
			cacheFlushReq.DiscardInflight = true
			cacheFlushReq.PauseAfterFlushing = true
			cacheFlushReq.InvalidateAllCachelines = true
			toCachesSender.EXPECT().
				Send(gomock.AssignableToTypeOf(cacheFlushReq))
		}

		toAddressTranslator.EXPECT().RetrieveIncoming()

		madeProgress := commandProcessor.processAddressTranslatorFlushRsp(
			req)

		Expect(commandProcessor.numCacheACK).To(Equal(uint64(40)))
		Expect(madeProgress).To(BeTrue())
	})

	It("should handle a Cache flush rsp", func() {
		req := cache.FlushRspBuilder{}.Build()
		req.Dst = commandProcessor.ToDMA
		vAddr := make([]uint64, 0)
		vAddr = append(vAddr, 100)
		shootDwnCmd := protocol.NewShootdownCommand(nil, commandProcessor.ToDriver, vAddr, 1)

		commandProcessor.numCacheACK = 1
		commandProcessor.shootDownInProcess = true
		commandProcessor.currShootdownRequest = shootDwnCmd

		for i := 0; i < 10; i++ {
			tlbFlushReq := tlb.FlushReqBuilder{}.Build()
			tlbFlushReq.Src = commandProcessor.ToTLBs
			tlbFlushReq.Dst = commandProcessor.TLBs[i]
			toTLBSender.EXPECT().Send(gomock.AssignableToTypeOf(tlbFlushReq))
		}

		toCaches.EXPECT().RetrieveIncoming()

		commandProcessor.processCacheFlushRsp(req)

		Expect(commandProcessor.numTLBAck).To(Equal(uint64(10)))
	})

	It("should handle a TLB flush rsp", func() {
		req := tlb.FlushRspBuilder{}.Build()
		req.Dst = commandProcessor.ToTLBs

		commandProcessor.numTLBAck = 1

		rsp := protocol.NewShootdownCompleteRsp(commandProcessor.ToDriver, commandProcessor.Driver)
		toDriverSender.EXPECT().Send(gomock.AssignableToTypeOf(rsp))
		toTLB.EXPECT().RetrieveIncoming()

		madeProgress := commandProcessor.processTLBFlushRsp(req)

		Expect(madeProgress).To(BeTrue())
		Expect(commandProcessor.shootDownInProcess).To(BeFalse())
	})

	It("should handle a GPU restart req", func() {
		req := protocol.NewGPURestartReq(nil, commandProcessor.ToDriver)

		for i := 0; i < 10; i++ {
			cacheRestartReq := cache.RestartReqBuilder{}.Build()
			cacheRestartReq.Src = commandProcessor.ToCaches
			cacheRestartReq.Dst = commandProcessor.L1ICaches[i]
			toCachesSender.EXPECT().
				Send(gomock.AssignableToTypeOf(cacheRestartReq))
		}

		for i := 0; i < 10; i++ {
			cacheRestartReq := cache.RestartReqBuilder{}.Build()
			cacheRestartReq.Src = commandProcessor.ToCaches
			cacheRestartReq.Dst = commandProcessor.L1SCaches[i]
			toCachesSender.EXPECT().
				Send(gomock.AssignableToTypeOf(cacheRestartReq))
		}

		for i := 0; i < 10; i++ {
			cacheRestartReq := cache.RestartReqBuilder{}.Build()
			cacheRestartReq.Src = commandProcessor.ToCaches
			cacheRestartReq.Dst = commandProcessor.L1VCaches[i]
			toCachesSender.EXPECT().
				Send(gomock.AssignableToTypeOf(cacheRestartReq))
		}

		for i := 0; i < 10; i++ {
			cacheRestartReq := cache.RestartReqBuilder{}.Build()
			cacheRestartReq.Src = commandProcessor.ToCaches
			cacheRestartReq.Dst = commandProcessor.L2Caches[i]
			toCachesSender.EXPECT().
				Send(gomock.AssignableToTypeOf(cacheRestartReq))
		}

		toDriver.EXPECT().RetrieveIncoming()

		madeProgress := commandProcessor.processGPURestartReq(req)

		Expect(madeProgress).To(BeTrue())
		Expect(commandProcessor.numCacheACK).To(Equal(uint64(40)))
	})

	It("should handle a cache restart rsp", func() {
		req := cache.RestartRspBuilder{}.Build()
		req.Dst = commandProcessor.ToDMA

		commandProcessor.numCacheACK = 1

		for i := 0; i < 10; i++ {
			tlbRestartReq := tlb.RestartReqBuilder{}.Build()
			tlbRestartReq.Src = commandProcessor.ToTLBs
			tlbRestartReq.Dst = commandProcessor.TLBs[i]
			toTLBSender.EXPECT().Send(gomock.AssignableToTypeOf(tlbRestartReq))
		}
		toCaches.EXPECT().RetrieveIncoming()

		madeProgress := commandProcessor.processCacheRestartRsp(req)

		Expect(madeProgress).To(BeTrue())
		Expect(commandProcessor.numTLBAck).To(Equal(uint64(10)))
	})

	It("should handle a TLB restart rsp", func() {
		req := tlb.RestartRspBuilder{}.Build()
		req.Dst = commandProcessor.ToTLBs

		commandProcessor.numTLBAck = 1

		for i := 0; i < 10; i++ {
			toAddressTranslatorSender.EXPECT().
				Send(gomock.Any())
		}
		toTLB.EXPECT().RetrieveIncoming()

		madeProgress := commandProcessor.processTLBRestartRsp(req)

		Expect(madeProgress).To(BeTrue())
		Expect(commandProcessor.numAddrTranslationRestartAck).
			To(Equal(uint64(10)))
	})

	It("should handle a AT restart rsp", func() {
		req := mem.ControlMsgBuilder{}.
			WithDst(commandProcessor.ToAddressTranslators).
			ToNotifyDone().
			Build()
		commandProcessor.numAddrTranslationRestartAck = 1

		for i := 0; i < 10; i++ {
			cuRestartReq := protocol.CUPipelineRestartReqBuilder{}.Build()
			cuRestartReq.Src = commandProcessor.ToCUs
			cuRestartReq.Dst = commandProcessor.CUs[i]
			toCUsSender.EXPECT().Send(gomock.AssignableToTypeOf(cuRestartReq))
		}
		toAddressTranslator.EXPECT().RetrieveIncoming()

		madeProgress :=
			commandProcessor.processAddressTranslatorRestartRsp(req)

		Expect(commandProcessor.numCUAck).To(Equal(uint64(10)))
		Expect(madeProgress).To(BeTrue())
	})

	It("should handle a CU pipeline restart rsp", func() {
		req := protocol.CUPipelineRestartRspBuilder{}.Build()
		req.Dst = commandProcessor.ToCUs

		commandProcessor.numCUAck = 1

		gpuRestartRsp := protocol.NewGPURestartRsp(
			commandProcessor.ToDriver, commandProcessor.Driver)
		toDriverSender.EXPECT().Send(gomock.AssignableToTypeOf(gpuRestartRsp))
		toCU.EXPECT().RetrieveIncoming()

		madeProgress := commandProcessor.processCUPipelineRestartRsp(req)

		Expect(madeProgress).To(BeTrue())
	})

	It("should handle a page migration req", func() {
		req := protocol.NewPageMigrationReqToCP(
			nil, commandProcessor.ToDriver)
		req.ToWriteToPhysicalAddress = 0x100
		req.ToReadFromPhysicalAddress = 0x20
		remotePMC := NewMockPort(mockCtrl)
		req.DestinationPMCPort = remotePMC
		req.PageSize = 4 * mem.KB

		reqToPMC := pagemigrationcontroller.PageMigrationReqToPMCBuilder{}.
			Build()
		reqToPMC.PageSize = req.PageSize
		reqToPMC.ToReadFromPhysicalAddress = req.ToReadFromPhysicalAddress
		reqToPMC.ToWriteToPhysicalAddress = req.ToWriteToPhysicalAddress
		reqToPMC.PMCPortOfRemoteGPU = req.DestinationPMCPort

		toPMCSender.EXPECT().Send(gomock.AssignableToTypeOf(reqToPMC))
		toDriver.EXPECT().RetrieveIncoming()

		madeProgress :=
			commandProcessor.processPageMigrationReq(req)

		Expect(madeProgress).To(BeTrue())

	})

	It("should handle a page migration rsp", func() {
		req := pagemigrationcontroller.PageMigrationRspFromPMCBuilder{}.Build()
		req.Dst = commandProcessor.ToPMC

		rsp := protocol.NewPageMigrationRspToDriver(
			commandProcessor.ToDriver, commandProcessor.Driver)

		toDriverSender.EXPECT().Send(gomock.AssignableToTypeOf(rsp))
		toPMC.EXPECT().RetrieveIncoming()

		madeProgress := commandProcessor.processPageMigrationRsp(req)

		Expect(madeProgress).To(BeTrue())
	})

})
