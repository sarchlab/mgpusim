package gcn3

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/akita"
	"gitlab.com/akita/mgpusim/pagemigrationcontroller"
	rdma2 "gitlab.com/akita/mgpusim/rdma"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
	"gitlab.com/akita/mem/vm/addresstranslator"
	"gitlab.com/akita/mem/vm/tlb"
)

var _ = Describe("CommandProcessor", func() {

	var (
		mockCtrl           *gomock.Controller
		engine             *MockEngine
		driver             *MockPort
		dispatcher         *MockPort
		commandProcessor   *CommandProcessor
		cus                []*MockPort
		toCU               *MockPort
		tlbs               []*MockPort
		addressTranslators []*MockPort
		rdma               *MockPort
		pmc                *MockPort
		l1VCaches          []*MockPort
		l1SCaches          []*MockPort
		l1ICaches          []*MockPort
		l2Caches           []*MockPort

		toDriver            *MockPort
		toDispatcher        *MockPort
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
		commandProcessor.toDispatcherSender = toDispatcherSender
		commandProcessor.toCUsSender = toCUsSender
		commandProcessor.toAddressTranslatorsSender = toAddressTranslatorSender
		commandProcessor.toTLBsSender = toTLBSender
		commandProcessor.toCachesSender = toCachesSender
		commandProcessor.toPMCSender = toPMCSender
		commandProcessor.toRDMASender = toRDMASender

		for i := 0; i < int(10); i++ {
			cus = append(cus, NewMockPort(mockCtrl))
			commandProcessor.CUs = append(commandProcessor.CUs, akita.NewLimitNumMsgPort(commandProcessor, 1, ""))
			commandProcessor.CUs[i] = cus[i]

			tlbs = append(tlbs, NewMockPort(mockCtrl))
			commandProcessor.TLBs = append(commandProcessor.TLBs, akita.NewLimitNumMsgPort(commandProcessor, 1, ""))
			commandProcessor.TLBs[i] = tlbs[i]

			addressTranslators = append(addressTranslators,
				NewMockPort(mockCtrl))
			commandProcessor.AddressTranslators =
				append(commandProcessor.AddressTranslators,
					akita.NewLimitNumMsgPort(commandProcessor, 1, ""))
			commandProcessor.AddressTranslators[i] = addressTranslators[i]

			l1ICaches = append(l1ICaches, NewMockPort(mockCtrl))
			commandProcessor.L1ICaches = append(commandProcessor.L1ICaches, akita.NewLimitNumMsgPort(commandProcessor, 1, ""))
			commandProcessor.L1ICaches[i] = l1ICaches[i]

			l1SCaches = append(l1SCaches, NewMockPort(mockCtrl))
			commandProcessor.L1SCaches = append(commandProcessor.L1SCaches, akita.NewLimitNumMsgPort(commandProcessor, 1, ""))
			commandProcessor.L1SCaches[i] = l1SCaches[i]

			l1VCaches = append(l1VCaches, NewMockPort(mockCtrl))
			commandProcessor.L1VCaches = append(commandProcessor.L1VCaches, akita.NewLimitNumMsgPort(commandProcessor, 1, ""))
			commandProcessor.L1VCaches[i] = l1VCaches[i]

			l2Caches = append(l2Caches, NewMockPort(mockCtrl))
			commandProcessor.L2Caches = append(commandProcessor.L2Caches, akita.NewLimitNumMsgPort(commandProcessor, 1, ""))
			commandProcessor.L2Caches[i] = l2Caches[i]
		}

		rdma = NewMockPort(mockCtrl)
		commandProcessor.RDMA = rdma

		pmc = NewMockPort(mockCtrl)
		commandProcessor.PMC = pmc
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should forward kernel launching request to a Dispatcher", func() {
		req := NewLaunchKernelReq(10,
			driver, commandProcessor.ToDriver)

		toDispatcherSender.EXPECT().Send(gomock.AssignableToTypeOf(req))
		toDriver.EXPECT().Retrieve(akita.VTimeInSec(10))

		madeProgress := commandProcessor.processLaunchKernelReq(10, req)

		Expect(madeProgress).To(BeTrue())
	})

	It("should delay forward kernel launching request to the Driver", func() {
		req := NewLaunchKernelReq(10,
			dispatcher, commandProcessor.ToDispatcher)

		engine.EXPECT().Schedule(
			gomock.AssignableToTypeOf(&ReplyKernelCompletionEvent{}))
		toDispatcher.EXPECT().Retrieve(akita.VTimeInSec(10))

		madeProgress := commandProcessor.processLaunchKernelRsp(10, req)

		Expect(madeProgress).To(BeTrue())
	})

	It("should handle a RDMA drain req from driver", func() {
		cmd := NewRDMADrainCmdFromDriver(10, nil, commandProcessor.ToDriver)
		drainReq := rdma2.RDMADrainReqBuilder{}.Build()

		toRDMASender.EXPECT().Send(gomock.AssignableToTypeOf(drainReq))
		toDriver.EXPECT().Retrieve(akita.VTimeInSec(10))

		madeProgress := commandProcessor.processRDMADrainCmd(10, cmd)

		Expect(madeProgress).To(BeTrue())
	})

	It("should handle a RDMA drain rsp from RDMA", func() {
		req := rdma2.RDMADrainRspBuilder{}.Build()

		drainRsp := NewRDMADrainRspToDriver(10,
			commandProcessor.ToDriver, commandProcessor.Driver)
		toDriverSender.EXPECT().Send(gomock.AssignableToTypeOf(drainRsp))
		toRDMA.EXPECT().Retrieve(akita.VTimeInSec(10))

		madeProgress := commandProcessor.processRDMADrainRsp(10, req)

		Expect(madeProgress).To(BeTrue())
	})

	It("should handle a shootdown cmd from Driver", func() {
		vAddr := make([]uint64, 0)
		vAddr = append(vAddr, 100)
		cmd := NewShootdownCommand(10, nil, commandProcessor.ToDriver, vAddr, 1)

		for i := 0; i < 10; i++ {
			cuFlushReq := CUPipelineFlushReqBuilder{}.Build()
			cuFlushReq.SendTime = 10
			cuFlushReq.Src = commandProcessor.ToCUs
			cuFlushReq.Dst = commandProcessor.CUs[i]
			toCUsSender.EXPECT().Send(gomock.AssignableToTypeOf(cuFlushReq))
		}
		toDriver.EXPECT().Retrieve(akita.VTimeInSec(10))

		madeProgress := commandProcessor.processShootdownCommand(10, cmd)

		Expect(commandProcessor.numCUAck).To(Equal(uint64(10)))
		Expect(madeProgress).To(BeTrue())
	})

	It("should handle a CU pipeline flush rsp", func() {
		req := CUPipelineFlushRspBuilder{}.Build()
		req.SendTime = 10
		req.Dst = commandProcessor.ToCUs
		commandProcessor.numCUAck = 1

		for i := 0; i < 10; i++ {
			atFlushReq := addresstranslator.
				AddressTranslatorFlushReqBuilder{}.
				Build()
			atFlushReq.SendTime = 10
			atFlushReq.Src = commandProcessor.ToAddressTranslators
			atFlushReq.Dst = commandProcessor.AddressTranslators[i]

			toAddressTranslatorSender.EXPECT().
				Send(gomock.AssignableToTypeOf(atFlushReq))
		}
		toCU.EXPECT().Retrieve(akita.VTimeInSec(10))

		madeProgress := commandProcessor.processCUPipelineFlushRsp(10, req)

		Expect(commandProcessor.numAddrTranslationAck).To(Equal(uint64(10)))
		Expect(madeProgress).To(BeTrue())
	})

	It("should handle a AT flush rsp", func() {
		req := addresstranslator.AddressTranslatorFlushRspBuilder{}.Build()
		req.SendTime = 10
		req.Dst = commandProcessor.ToAddressTranslators
		commandProcessor.numAddrTranslationAck = 1

		for i := 0; i < 10; i++ {
			cacheFlushReq := cache.FlushReqBuilder{}.Build()
			cacheFlushReq.SendTime = 10
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
			cacheFlushReq.SendTime = 10
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
			cacheFlushReq.SendTime = 10
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
			cacheFlushReq.SendTime = 10
			cacheFlushReq.Src = commandProcessor.ToCaches
			cacheFlushReq.Dst = commandProcessor.L2Caches[i]
			cacheFlushReq.DiscardInflight = true
			cacheFlushReq.PauseAfterFlushing = true
			cacheFlushReq.InvalidateAllCachelines = true
			toCachesSender.EXPECT().
				Send(gomock.AssignableToTypeOf(cacheFlushReq))
		}

		toAddressTranslator.EXPECT().Retrieve(akita.VTimeInSec(10))

		madeProgress := commandProcessor.processAddressTranslatorFlushRsp(
			10, req)

		Expect(commandProcessor.numCacheACK).To(Equal(uint64(40)))
		Expect(madeProgress).To(BeTrue())
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
		commandProcessor.currShootdownRequest = shootDwnCmd

		for i := 0; i < 10; i++ {
			tlbFlushReq := tlb.TLBFlushReqBuilder{}.Build()
			tlbFlushReq.SendTime = 10
			tlbFlushReq.Src = commandProcessor.ToTLBs
			tlbFlushReq.Dst = commandProcessor.TLBs[i]
			toTLBSender.EXPECT().Send(gomock.AssignableToTypeOf(tlbFlushReq))
		}

		toCaches.EXPECT().Retrieve(akita.VTimeInSec(10))

		commandProcessor.processCacheFlushRsp(10, req)

		Expect(commandProcessor.numTLBAck).To(Equal(uint64(10)))
	})

	It("should handle a TLB flush rsp", func() {
		req := tlb.TLBFlushRspBuilder{}.Build()
		req.SendTime = 10
		req.Dst = commandProcessor.ToTLBs

		commandProcessor.numTLBAck = 1

		rsp := NewShootdownCompleteRsp(10, commandProcessor.ToDriver, commandProcessor.Driver)
		toDriverSender.EXPECT().Send(gomock.AssignableToTypeOf(rsp))
		toTLB.EXPECT().Retrieve(akita.VTimeInSec(10))

		madeProgress := commandProcessor.processTLBFlushRsp(10, req)

		Expect(madeProgress).To(BeTrue())
		Expect(commandProcessor.shootDownInProcess).To(BeFalse())
	})

	It("should handle a GPU restart req", func() {
		req := NewGPURestartReq(10, nil, commandProcessor.ToDriver)

		for i := 0; i < 10; i++ {
			cacheRestartReq := cache.RestartReqBuilder{}.Build()
			cacheRestartReq.SendTime = 10
			cacheRestartReq.Src = commandProcessor.ToCaches
			cacheRestartReq.Dst = commandProcessor.L1ICaches[i]
			toCachesSender.EXPECT().
				Send(gomock.AssignableToTypeOf(cacheRestartReq))
		}

		for i := 0; i < 10; i++ {
			cacheRestartReq := cache.RestartReqBuilder{}.Build()
			cacheRestartReq.SendTime = 10
			cacheRestartReq.Src = commandProcessor.ToCaches
			cacheRestartReq.Dst = commandProcessor.L1SCaches[i]
			toCachesSender.EXPECT().
				Send(gomock.AssignableToTypeOf(cacheRestartReq))
		}

		for i := 0; i < 10; i++ {
			cacheRestartReq := cache.RestartReqBuilder{}.Build()
			cacheRestartReq.SendTime = 10
			cacheRestartReq.Src = commandProcessor.ToCaches
			cacheRestartReq.Dst = commandProcessor.L1VCaches[i]
			toCachesSender.EXPECT().
				Send(gomock.AssignableToTypeOf(cacheRestartReq))
		}

		for i := 0; i < 10; i++ {
			cacheRestartReq := cache.RestartReqBuilder{}.Build()
			cacheRestartReq.SendTime = 10
			cacheRestartReq.Src = commandProcessor.ToCaches
			cacheRestartReq.Dst = commandProcessor.L2Caches[i]
			toCachesSender.EXPECT().
				Send(gomock.AssignableToTypeOf(cacheRestartReq))
		}

		toDriver.EXPECT().Retrieve(akita.VTimeInSec(10))

		madeProgress := commandProcessor.processGPURestartReq(10, req)

		Expect(madeProgress).To(BeTrue())
		Expect(commandProcessor.numCacheACK).To(Equal(uint64(40)))
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
			toTLBSender.EXPECT().Send(gomock.AssignableToTypeOf(tlbRestartReq))
		}
		toCaches.EXPECT().Retrieve(akita.VTimeInSec(10))

		madeProgress := commandProcessor.processCacheRestartRsp(10, req)

		Expect(madeProgress).To(BeTrue())
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

			toAddressTranslatorSender.EXPECT().
				Send(gomock.AssignableToTypeOf(atRestartReq))
		}
		toTLB.EXPECT().Retrieve(akita.VTimeInSec(10))

		madeProgress := commandProcessor.processTLBRestartRsp(10, req)

		Expect(madeProgress).To(BeTrue())
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
			toCUsSender.EXPECT().Send(gomock.AssignableToTypeOf(cuRestartReq))
		}
		toAddressTranslator.EXPECT().Retrieve(akita.VTimeInSec(10))

		madeProgress :=
			commandProcessor.processAddressTranslatorRestartRsp(10, req)

		Expect(commandProcessor.numCUAck).To(Equal(uint64(10)))
		Expect(madeProgress).To(BeTrue())
	})

	It("should handle a CU pipeline restart rsp", func() {
		req := CUPipelineRestartRspBuilder{}.Build()
		req.SendTime = 10
		req.Dst = commandProcessor.ToCUs

		commandProcessor.numCUAck = 1

		gpuRestartRsp := NewGPURestartRsp(10,
			commandProcessor.ToDriver, commandProcessor.Driver)
		toDriverSender.EXPECT().Send(gomock.AssignableToTypeOf(gpuRestartRsp))
		toCU.EXPECT().Retrieve(akita.VTimeInSec(10))

		madeProgress := commandProcessor.processCUPipelineRestartRsp(10, req)

		Expect(madeProgress).To(BeTrue())
	})

	It("should handle a page migration req", func() {
		req := NewPageMigrationReqToCP(10, nil, commandProcessor.ToDriver)
		req.ToWriteToPhysicalAddress = 0x100
		req.ToReadFromPhysicalAddress = 0x20
		remotePMC := NewMockPort(mockCtrl)
		req.DestinationPMCPort = remotePMC
		req.PageSize = 4 * mem.KB

		reqToPMC := pagemigrationcontroller.PageMigrationReqToPMCBuilder{}.
			Build()
		req.SendTime = 10
		reqToPMC.PageSize = req.PageSize
		reqToPMC.ToReadFromPhysicalAddress = req.ToReadFromPhysicalAddress
		reqToPMC.ToWriteToPhysicalAddress = req.ToWriteToPhysicalAddress
		reqToPMC.PMCPortOfRemoteGPU = req.DestinationPMCPort

		toPMCSender.EXPECT().Send(gomock.AssignableToTypeOf(reqToPMC))
		toDriver.EXPECT().Retrieve(akita.VTimeInSec(10))

		madeProgress :=
			commandProcessor.processPageMigrationReq(10, req)

		Expect(madeProgress).To(BeTrue())

	})

	It("should handle a page migration rsp", func() {
		req := pagemigrationcontroller.PageMigrationRspFromPMCBuilder{}.Build()
		req.SendTime = 10
		req.Dst = commandProcessor.ToPMC

		rsp := NewPageMigrationRspToDriver(10,
			commandProcessor.ToDriver, commandProcessor.Driver)

		toDriverSender.EXPECT().Send(gomock.AssignableToTypeOf(rsp))
		toPMC.EXPECT().Retrieve(akita.VTimeInSec(10))

		madeProgress := commandProcessor.processPageMigrationRsp(10, req)

		Expect(madeProgress).To(BeTrue())
	})

})
