package caches

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/akita"
	"gitlab.com/akita/akita/mock_akita"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
)

var _ = Describe("L1V Cache", func() {
	var (
		mockCtrl  *gomock.Controller
		engine    *akita.MockEngine
		storage   *mem.Storage
		directory *cache.MockDirectory
		l1v       *L1VCache
		l2Finder  cache.LowModuleFinder

		toCU *mock_akita.MockPort
		toCP *mock_akita.MockPort
		toL2 *mock_akita.MockPort
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		engine = akita.NewMockEngine()
		storage = mem.NewStorage(16 * mem.KB)
		directory = new(cache.MockDirectory)
		l2Finder = new(cache.SingleLowModuleFinder)
		l1v = NewL1VCache("l1v", engine, 1)
		l1v.Directory = directory
		l1v.Storage = storage
		l1v.L2Finder = l2Finder
		l1v.InvalidationLatency = 64
		l1v.BlockSizeAsPowerOf2 = 6

		toCU = mock_akita.NewMockPort(mockCtrl)
		toCP = mock_akita.NewMockPort(mockCtrl)
		toL2 = mock_akita.NewMockPort(mockCtrl)
		l1v.ToCU = toCU
		l1v.ToCP = toCP
		l1v.ToL2 = toL2
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("read requests from CU", func() {

		It("should put transaction in reqBuf and preAddressTranslationBuf", func() {
			req := mem.NewReadReq(10, nil, nil, 0x100, 64)
			toCU.EXPECT().Retrieve(akita.VTimeInSec(10)).Return(req)

			l1v.parseFromCU(10)

			Expect(l1v.reqBuf).To(HaveLen(1))
			Expect(l1v.reqIDToTransactionMap).To(HaveLen(1))
			Expect(l1v.preAddrTranslationBuf).To(HaveLen(1))

			Expect(l1v.NeedTick).To(BeTrue())
		})

		It("should block preAddrTranslationBuf when receiving last req in insruction", func() {
			req := mem.NewReadReq(10, nil, nil, 0x100, 64)
			req.IsLastInWave = true
			toCU.EXPECT().Retrieve(akita.VTimeInSec(10)).Return(req)

			l1v.parseFromCU(10)

			Expect(l1v.reqBuf).To(HaveLen(1))
			Expect(l1v.reqIDToTransactionMap).To(HaveLen(1))
			Expect(l1v.preAddrTranslationBuf).To(HaveLen(1))

			Expect(l1v.NeedTick).To(BeTrue())
		})

		//It("should coalesce after receiving last in inst write", func() {
		//req0 := mem.NewWriteReq(10, nil, nil, 0x100)
		//req0.Data = []byte{0, 1, 2, 3}
		//toCU.EXPECT().Retrieve(akita.VTimeInSec(10)).Return(req0)
		//l1v.parseFromCU(10)

		//req := mem.NewWriteReq(10, nil, nil, 0x104)
		//req.Data = []byte{4, 5, 6, 7}
		//req.IsLastInWave = true
		//toCU.EXPECT().Retrieve(akita.VTimeInSec(10)).Return(req)
		//l1v.parseFromCU(10)

		//Expect(l1v.reqBuf).To(HaveLen(2))
		//Expect(l1v.reqIDToTransactionMap).To(HaveLen(2))
		//Expect(l1v.inPipeline).To(HaveLen(0))

		//Expect(l1v.preCoalesceWriteBuf).To(HaveLen(0))
		//Expect(l1v.postCoalesceWriteBuf).To(HaveLen(1))
		//Expect(l1v.postCoalesceWriteBuf[0].OriginalReqs).To(HaveLen(2))

		//Expect(l1v.NeedTick).To(BeTrue())
		//})
	})

	It("should process post coalescing write", func() {
		write := mem.NewWriteReq(10, nil, nil, 0x100)
		transaction := new(cacheTransaction)
		transaction.Req = write
		l1v.postCoalesceWriteBuf = append(l1v.postCoalesceWriteBuf, transaction)

		l1v.processPostCoalescingWrites(11)

		Expect(l1v.postCoalesceWriteBuf).To(HaveLen(0))
		Expect(l1v.inPipeline).To(HaveLen(1))
		Expect(l1v.NeedTick).To(BeTrue())
	})

	Context("count down pipeline", func() {
		It("should count down", func() {
			inPipeline := &inPipelineReqStatus{nil, 10}
			l1v.inPipeline = append(l1v.inPipeline, inPipeline)

			l1v.countDownPipeline(10)

			Expect(inPipeline.CycleLeft).To(Equal(9))
			Expect(l1v.NeedTick).To(BeTrue())
		})

		It("should insert into post pipeline buf", func() {
			req := mem.NewReadReq(10, nil, nil, 0x100, 64)
			transaction := l1v.createTransaction(req)
			inPipeline := &inPipelineReqStatus{transaction, 1}
			l1v.inPipeline = append(l1v.inPipeline, inPipeline)

			l1v.countDownPipeline(10)

			Expect(l1v.inPipeline).To(HaveLen(0))
			Expect(l1v.postPipelineBuf).To(HaveLen(1))
		})
	})

	Context("parse read hit", func() {
		var (
			block       *cache.Block
			read        *mem.ReadReq
			transaction *cacheTransaction
		)

		BeforeEach(func() {
			block = new(cache.Block)
			directory.ExpectLookup(0x100, block)

			read = mem.NewReadReq(10, nil, l1v.ToCU, 0x100, 64)
			transaction = l1v.createTransaction(read)
			l1v.postPipelineBuf = append(l1v.postPipelineBuf, transaction)
		})

		It("should start local read", func() {
			l1v.parseFromPostPipelineBuf(11)

			Expect(directory.AllExpectedCalled()).To(BeTrue())

			Expect(transaction.Block).To(BeIdenticalTo(block))
			Expect(block.IsLocked).To(BeTrue())

			Expect(l1v.NeedTick).To(BeTrue())

			Expect(l1v.storageTransaction).To(BeIdenticalTo(transaction))
			Expect(l1v.storageCycleLeft).To(Equal(1))
			Expect(l1v.postPipelineBuf).To(HaveLen(0))
		})

		It("should also start local read if reading partial line", func() {
			read.Address = 0x104
			read.MemByteSize = 4

			l1v.parseFromPostPipelineBuf(11)

			Expect(directory.AllExpectedCalled()).To(BeTrue())
			Expect(l1v.NeedTick).To(BeTrue())
			Expect(block.IsLocked).To(BeTrue())

			Expect(l1v.storageTransaction).To(BeIdenticalTo(transaction))
			Expect(l1v.storageCycleLeft).To(Equal(1))
			Expect(l1v.postPipelineBuf).To(HaveLen(0))
		})

		It("should stall if block is locked", func() {
			block.IsLocked = true

			l1v.parseFromPostPipelineBuf(11)

			Expect(directory.AllExpectedCalled()).To(BeTrue())
			Expect(l1v.NeedTick).To(BeFalse())
			Expect(l1v.storageTransaction).To(BeNil())
		})
	})

	Context("parse read miss", func() {
		var (
			block       *cache.Block
			read        *mem.ReadReq
			transaction *cacheTransaction
		)

		BeforeEach(func() {
			block = new(cache.Block)
			directory.ExpectLookup(0x100, nil)
			directory.ExpectEvict(0x100, block)

			read = mem.NewReadReq(10, nil, l1v.ToCU, 0x100, 64)
			transaction = l1v.createTransaction(read)
			l1v.postPipelineBuf = append(l1v.postPipelineBuf, transaction)
		})

		It("should send read request to bottom", func() {
			l1v.parseFromPostPipelineBuf(11)

			Expect(l1v.pendingDownGoingRead).To(HaveLen(1))
			Expect(l1v.toL2Buffer).To(HaveLen(1))
			Expect(transaction.Block).To(BeIdenticalTo(block))
			Expect(block.Tag).To(Equal(uint64(0x100)))
			Expect(block.IsValid).To(BeTrue())
			Expect(block.IsLocked).To(BeTrue())
			Expect(transaction.ReqToBottom).NotTo(BeNil())
			Expect(l1v.mshr).To(HaveLen(1))
		})

		It("should not read form bottom if the address is already being read", func() {
			readBottom := mem.NewReadReq(11, nil, nil, 0x100, 64)
			transaction.ReqToBottom = readBottom
			l1v.mshr = append(l1v.mshr, transaction)

			l1v.parseFromPostPipelineBuf(11)

			Expect(l1v.NeedTick).To(BeTrue())
			Expect(l1v.mshr).To(HaveLen(2))
		})

		It("always read a whole cache line from bottom", func() {
			read.Address = 0x104
			read.MemByteSize = 4

			l1v.parseFromPostPipelineBuf(11)

			Expect(l1v.pendingDownGoingRead).To(HaveLen(1))
			Expect(l1v.toL2Buffer).To(HaveLen(1))
			Expect(l1v.toL2Buffer[0].(*mem.ReadReq).Address).To(
				Equal(uint64(0x100)))
			Expect(l1v.toL2Buffer[0].(*mem.ReadReq).MemByteSize).To(
				Equal(uint64(64)))
			Expect(transaction.Block).To(BeIdenticalTo(block))
			Expect(block.Tag).To(Equal(uint64(0x100)))
			Expect(block.IsValid).To(BeTrue())
			Expect(block.IsLocked).To(BeTrue())
			Expect(transaction.ReqToBottom).NotTo(BeNil())
		})
	})

	Context("parse invalidate", func() {
		It("should schedule InvalidateCompleteEvent", func() {
			invalidReq := mem.NewInvalidReq(10, nil, l1v.ToCP)

			toCP.EXPECT().Retrieve(akita.VTimeInSec(11)).Return(invalidReq)

			l1v.parseFromCP(11)

			Expect(engine.ScheduledEvent).To(HaveLen(1))
			invalidCompEvent := engine.ScheduledEvent[0].(*invalidationCompleteEvent)
			Expect(invalidCompEvent.Time()).To(BeNumerically("~", 75))
		})
	})

	Context("complete invalidation", func() {
		It("should clear cache and respond", func() {
			invalidReq := mem.NewInvalidReq(10, nil, l1v.ToCP)
			invalidComplEvent := newInvalidationCompleteEvent(70,
				l1v, invalidReq, l1v.ToCP)

			expectRsp := mem.NewInvalidDoneRsp(70, l1v.ToCP, nil,
				invalidReq.GetID())
			toCP.EXPECT().Send(gomock.AssignableToTypeOf(expectRsp)).Return(nil)

			l1v.Handle(invalidComplEvent)

			Expect(directory.Resetted).To(BeTrue())
			Expect(engine.ScheduledEvent).To(HaveLen(1))
		})

		It("should retry if send failed", func() {
			invalidReq := mem.NewInvalidReq(10, nil, l1v.ToCP)
			invalidComplEvent := newInvalidationCompleteEvent(70,
				l1v, invalidReq, l1v.ToCP)

			expectRsp := mem.NewInvalidDoneRsp(70, l1v.ToCP, nil,
				invalidReq.GetID())
			toCP.EXPECT().Send(gomock.AssignableToTypeOf(expectRsp)).
				Return(akita.NewSendError())

			l1v.Handle(invalidComplEvent)

			Expect(engine.ScheduledEvent).To(HaveLen(1))
			evt := engine.ScheduledEvent[0].(*invalidationCompleteEvent)
			Expect(evt.Time()).To(BeNumerically("~", 71))
		})
	})

	Context("during local read or local write", func() {
		It("should reduce cycleLeft for all the request that is being processed by the storage", func() {
			l1v.storageTransaction = new(cacheTransaction)
			l1v.storageCycleLeft = 2

			l1v.doReadWrite(10)

			Expect(l1v.storageCycleLeft).To(Equal(1))
		})

	})

	Context("complete read hit", func() {
		var (
			block       *cache.Block
			read        *mem.ReadReq
			transaction *cacheTransaction
		)

		BeforeEach(func() {
			block = new(cache.Block)
			block.IsValid = true
			block.IsLocked = true
			block.CacheAddress = 0x200
			storage.Write(0x200, []byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			})

			read = mem.NewReadReq(10, nil, l1v.ToCU, 0x100, 64)
			transaction = l1v.createTransaction(read)
			transaction.Block = block
			l1v.storageTransaction = transaction
			l1v.storageCycleLeft = 1
		})

		It("should finish read", func() {
			l1v.doReadWrite(10)

			Expect(l1v.NeedTick).To(BeTrue())
			Expect(l1v.storageTransaction).To(BeNil())

			Expect(transaction.Rsp).NotTo(BeNil())
			Expect(transaction.Rsp.(*mem.DataReadyRsp).Data).To(Equal([]byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			}))
			Expect(block.IsLocked).To(BeFalse())

		})

		It("should finish read if not accessing the whole cache line", func() {
			read.Address = 0x104
			read.MemByteSize = 4

			l1v.doReadWrite(10)

			Expect(l1v.NeedTick).To(BeTrue())
			Expect(l1v.inPipeline).To(BeEmpty())

			rsp := l1v.reqBuf[0].Rsp
			Expect(rsp).NotTo(BeNil())
			Expect(rsp.(*mem.DataReadyRsp).Data).To(
				Equal([]byte{5, 6, 7, 8}))
			Expect(block.IsLocked).To(BeFalse())
		})
	})

	Context("handle data-ready", func() {
		var (
			readFromTop         *mem.ReadReq
			readToBottom        *mem.ReadReq
			dataReadyFromBottom *mem.DataReadyRsp
			block               *cache.Block
			transaction         *cacheTransaction
		)

		BeforeEach(func() {
			readFromTop = mem.NewReadReq(
				5, nil, l1v.ToCU, 0x100, 64)
			readToBottom = mem.NewReadReq(
				8, l1v.ToL2, nil, 0x100, 64)
			dataReadyFromBottom = mem.NewDataReadyRsp(
				10, nil, l1v.ToL2, readToBottom.GetID())
			dataReadyFromBottom.Data = []byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			}

			block = new(cache.Block)
			block.IsValid = false
			block.IsLocked = true
			block.CacheAddress = 0x200

			l1v.pendingDownGoingRead =
				append(l1v.pendingDownGoingRead, readToBottom)
			transaction = l1v.createTransaction(readFromTop)
			transaction.Block = block
			transaction.ReqToBottom = readToBottom
			l1v.mshr = append(l1v.mshr, transaction)
		})

		It("should respond data ready to cu and write to local", func() {
			toL2.EXPECT().Peek().Return(dataReadyFromBottom)
			toL2.EXPECT().
				Retrieve(akita.VTimeInSec(11)).
				Return(dataReadyFromBottom)

			l1v.parseFromL2(11)

			rsp := l1v.reqBuf[0].Rsp
			Expect(rsp).NotTo(BeNil())
			Expect(rsp.(*mem.DataReadyRsp).Data).To(Equal([]byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			}))
			Expect(l1v.pendingDownGoingRead).To(HaveLen(0))
			Expect(l1v.mshr).To(BeEmpty())
			Expect(l1v.NeedTick).To(BeTrue())

			data, _ := storage.Read(0x200, 64)
			Expect(data).To(Equal(dataReadyFromBottom.Data))
			Expect(block.IsValid).To(BeTrue())
			Expect(block.IsLocked).To(BeFalse())
		})

		It("should return partial data if read from top is not reading full line", func() {
			readFromTop.Address = 0x104
			readFromTop.MemByteSize = 4

			toL2.EXPECT().Peek().Return(dataReadyFromBottom)
			toL2.EXPECT().
				Retrieve(akita.VTimeInSec(11)).
				Return(dataReadyFromBottom)

			l1v.parseFromL2(11)

			rsp := l1v.reqBuf[0].Rsp
			Expect(rsp).NotTo(BeNil())
			Expect(rsp.(*mem.DataReadyRsp).Data).To(Equal([]byte{5, 6, 7, 8}))

			Expect(l1v.pendingDownGoingRead).To(HaveLen(0))
			Expect(l1v.mshr).To(BeEmpty())
			Expect(l1v.NeedTick).To(BeTrue())

			data, _ := storage.Read(0x200, 64)
			Expect(data).To(Equal(dataReadyFromBottom.Data))
			Expect(block.IsValid).To(BeTrue())
			Expect(block.IsLocked).To(BeFalse())
		})
	})

	Context("when handle write", func() {
		var (
			writeLine    *mem.WriteReq
			writePartial *mem.WriteReq
			transaction  *cacheTransaction
		)

		BeforeEach(func() {
			writeLine = mem.NewWriteReq(10, nil, l1v.ToCU, 0x100)
			writeLine.Data = []byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			}

			writePartial = mem.NewWriteReq(10, nil, l1v.ToCU, 0x104)
			writePartial.Data = []byte{5, 6, 7, 8}
		})

		Context("write hit", func() {
			var (
				block *cache.Block
			)

			BeforeEach(func() {
				block = new(cache.Block)
				block.Tag = 0x100
				block.CacheAddress = 0x200
				block.IsValid = true
				directory.ExpectLookup(0x100, block)
			})

			It("should stall if block is locked", func() {
				block.IsLocked = true
				transaction = l1v.createTransaction(writeLine)
				l1v.postPipelineBuf = append(l1v.postPipelineBuf, transaction)

				l1v.parseFromPostPipelineBuf(11)

				Expect(l1v.NeedTick).To(BeFalse())
				Expect(l1v.toL2Buffer).To(HaveLen(0))
				Expect(l1v.pendingDownGoingWrite).To(HaveLen(0))
				Expect(l1v.reqBufReadPtr).To(Equal(0))
			})

			It("should do full line write", func() {
				transaction = l1v.createTransaction(writeLine)
				l1v.postPipelineBuf = append(l1v.postPipelineBuf, transaction)

				l1v.parseFromPostPipelineBuf(11)

				Expect(l1v.NeedTick).To(BeTrue())
				Expect(l1v.toL2Buffer).To(HaveLen(1))
				Expect(l1v.pendingDownGoingWrite).To(HaveLen(1))
				Expect(transaction.Block).To(BeIdenticalTo(block))
				Expect(transaction.ReqToBottom).NotTo(BeNil())
				Expect(block.IsLocked).To(BeTrue())

				data, _ := storage.Read(0x200, 64)
				Expect(data).To(Equal([]byte{
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
				}))
			})

			It("should handle partial line write", func() {
				transaction = l1v.createTransaction(writePartial)
				l1v.postPipelineBuf = append(l1v.postPipelineBuf, transaction)

				l1v.parseFromPostPipelineBuf(11)

				Expect(l1v.NeedTick).To(BeTrue())
				Expect(l1v.toL2Buffer).To(HaveLen(1))
				Expect(l1v.pendingDownGoingWrite).To(HaveLen(1))
				Expect(transaction.Block).To(BeIdenticalTo(block))
				Expect(block.IsLocked).To(BeTrue())

				data, _ := storage.Read(0x204, 4)
				Expect(data).To(Equal([]byte{5, 6, 7, 8}))
			})
		})

		Context("write miss", func() {
			var (
				block       *cache.Block
				transaction *cacheTransaction
			)

			BeforeEach(func() {
				block = new(cache.Block)
				block.CacheAddress = 0x200
				block.IsValid = false
				directory.ExpectLookup(0x100, nil)
				directory.ExpectEvict(0x100, block)
			})

			It("should stall if block is locked", func() {
				block.IsLocked = true
				transaction = l1v.createTransaction(writeLine)
				l1v.postPipelineBuf = append(l1v.postPipelineBuf, transaction)

				l1v.parseFromPostPipelineBuf(11)

				Expect(l1v.NeedTick).To(BeFalse())
				Expect(l1v.toL2Buffer).To(HaveLen(0))
				Expect(l1v.pendingDownGoingWrite).To(HaveLen(0))
				Expect(l1v.reqBufReadPtr).To(Equal(0))
			})

			It("should do full line write", func() {
				transaction = l1v.createTransaction(writeLine)
				l1v.postPipelineBuf = append(l1v.postPipelineBuf, transaction)

				l1v.parseFromPostPipelineBuf(11)

				Expect(l1v.NeedTick).To(BeTrue())
				Expect(l1v.toL2Buffer).To(HaveLen(1))
				Expect(l1v.pendingDownGoingWrite).To(HaveLen(1))
				// Expect(l1v.ToCU.Buf).To(HaveLen(0))
				Expect(transaction.Block).To(BeIdenticalTo(block))
				Expect(block.IsLocked).To(BeTrue())

				data, _ := storage.Read(0x200, 64)
				Expect(data).To(Equal([]byte{
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
				}))
				Expect(block.IsValid).To(BeTrue())
				Expect(block.Tag).To(Equal(uint64(0x100)))
			})

			It("should handle partial line write", func() {
				transaction = l1v.createTransaction(writePartial)
				l1v.postPipelineBuf = append(l1v.postPipelineBuf, transaction)

				l1v.parseFromPostPipelineBuf(11)

				Expect(l1v.NeedTick).To(BeTrue())
				Expect(l1v.toL2Buffer).To(HaveLen(1))
				Expect(l1v.pendingDownGoingWrite).To(HaveLen(1))
				// Expect(l1v.ToCU.Buf).To(HaveLen(0))

				Expect(block.IsValid).To(BeFalse())
			})
		})
	})

	It("should handle done rsp", func() {
		writeFromTop1 := mem.NewWriteReq(6, nil, l1v.ToCU, 0x104)
		transaction1 := l1v.createTransaction(writeFromTop1)

		writeFromTop2 := mem.NewWriteReq(6, nil, l1v.ToCU, 0x108)
		transaction2 := l1v.createTransaction(writeFromTop2)

		writeToBottom := mem.NewWriteReq(8, l1v.ToL2, nil, 0x104)
		coalescedTransaction := new(cacheTransaction)
		coalescedTransaction.ReqToBottom = writeToBottom
		coalescedTransaction.OriginalReqs = append(
			coalescedTransaction.OriginalReqs, writeFromTop1)
		coalescedTransaction.OriginalReqs = append(
			coalescedTransaction.OriginalReqs, writeFromTop2)
		l1v.pendingDownGoingWrite = append(l1v.pendingDownGoingWrite,
			coalescedTransaction)

		block := new(cache.Block)
		block.IsLocked = true
		coalescedTransaction.Block = block

		doneRsp := mem.NewDoneRsp(10, nil, l1v.ToL2, writeToBottom.ID)

		toL2.EXPECT().Peek().Return(doneRsp)
		toL2.EXPECT().Retrieve(akita.VTimeInSec(11)).Return(doneRsp)

		l1v.parseFromL2(11)

		// Expect(l1v.ToL2.Buf).To(HaveLen(0))
		Expect(l1v.pendingDownGoingWrite).To(HaveLen(0))
		Expect(l1v.NeedTick).To(BeTrue())
		Expect(transaction1.Rsp).NotTo(BeNil())
		Expect(transaction2.Rsp).NotTo(BeNil())
		Expect(block.IsLocked).To(BeFalse())
	})

	It("should send request to CU", func() {
		read := mem.NewReadReq(8, nil, l1v.ToCU, 0x100, 64)
		transaction := l1v.createTransaction(read)
		dataReady := mem.NewDataReadyRsp(10, l1v.ToCU, nil, read.GetID())
		transaction.Rsp = dataReady

		toCU.EXPECT().Send(gomock.AssignableToTypeOf(dataReady)).Return(nil)

		l1v.sendToCU(11)

		Expect(l1v.reqBuf).To(HaveLen(0))
		Expect(l1v.reqIDToTransactionMap[read.GetID()]).To(BeNil())

		Expect(l1v.NeedTick).To(BeTrue())
	})

	It("should send request to L2", func() {
		read := mem.NewReadReq(10, l1v.ToL2, nil, 0x100, 64)
		l1v.toL2Buffer = append(l1v.toL2Buffer, read)

		toL2.EXPECT().Send(gomock.AssignableToTypeOf(read)).Return(nil)

		l1v.sendToL2(11)

		Expect(l1v.NeedTick).To(BeTrue())
	})
})

var _ = XDescribe("L1VCache black box", func() {
	var (
		engine     akita.Engine
		evictor    cache.Evictor
		storage    *mem.Storage
		directory  cache.Directory
		l1v        *L1VCache
		l2Finder   *cache.SingleLowModuleFinder
		cu         *akita.MockComponent
		lowModule  *mem.IdealMemController
		connection *akita.DirectConnection
	)

	BeforeEach(func() {
		engine = akita.NewSerialEngine()
		storage = mem.NewStorage(16 * mem.KB)
		evictor = cache.NewLRUEvictor()
		directory = cache.NewDirectory(64, 4, 64, evictor)
		l2Finder = new(cache.SingleLowModuleFinder)
		l1v = NewL1VCache("l1v", engine, 1)
		l1v.Directory = directory
		l1v.Storage = storage
		l1v.L2Finder = l2Finder
		l1v.BlockSizeAsPowerOf2 = 6

		lowModule = mem.NewIdealMemController("lowModule", engine, 4*mem.GB)
		lowModule.Storage.Write(0x100, []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		})
		lowModule.Latency = 300
		lowModule.Freq = 1
		l2Finder.LowModule = lowModule.ToTop

		cu = akita.NewMockComponent("cu")

		connection = akita.NewDirectConnection(engine)
		connection.PlugIn(l1v.ToCU)
		connection.PlugIn(l1v.ToL2)
		connection.PlugIn(lowModule.ToTop)
		connection.PlugIn(cu.ToOutside)
	})

	It("should read miss", func() {
		read := mem.NewReadReq(10, cu.ToOutside, l1v.ToCU, 0x100, 64)
		l1v.ToCU.Recv(read)

		engine.Run()

		Expect(cu.ReceivedReqs).To(HaveLen(1))
		Expect(cu.ReceivedReqs[0].(*mem.DataReadyRsp).Data).To(Equal([]byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		}))
	})

	It("should read miss on partial line read", func() {
		read := mem.NewReadReq(10, cu.ToOutside, l1v.ToCU, 0x104, 4)
		l1v.ToCU.Recv(read)

		engine.Run()

		Expect(cu.ReceivedReqs).To(HaveLen(1))
		Expect(cu.ReceivedReqs[0].(*mem.DataReadyRsp).Data).To(
			Equal([]byte{5, 6, 7, 8}))
	})

	It("should read hit", func() {
		read := mem.NewReadReq(10, cu.ToOutside, l1v.ToCU, 0x100, 64)

		l1v.ToCU.Recv(read)
		engine.Run()
		Expect(cu.ReceivedReqs).To(HaveLen(1))
		Expect(cu.ReceivedReqs[0].(*mem.DataReadyRsp).Data).To(Equal([]byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		}))
		Expect(cu.ReceivedReqs[0].(*mem.DataReadyRsp).RespondTo).To(
			Equal(read.ID))

		read1 := mem.NewReadReq(10, cu.ToOutside, l1v.ToCU, 0x100, 64)
		read1.SetRecvTime(engine.CurrentTime() + akita.VTimeInSec(1))
		l1v.ToCU.Recv(read1)
		engine.Run()
		Expect(cu.ReceivedReqs).To(HaveLen(2))
		Expect(cu.ReceivedReqs[1].(*mem.DataReadyRsp).Data).To(Equal([]byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		}))
		Expect(cu.ReceivedReqs[1].(*mem.DataReadyRsp).RespondTo).To(
			Equal(read1.ID))
	})

	It("should read hit on partial line read", func() {
		read := mem.NewReadReq(10, cu.ToOutside, l1v.ToCU, 0x104, 4)
		l1v.ToCU.Recv(read)

		engine.Run()

		Expect(cu.ReceivedReqs).To(HaveLen(1))
		Expect(cu.ReceivedReqs[0].(*mem.DataReadyRsp).Data).To(
			Equal([]byte{5, 6, 7, 8}))

		read1 := mem.NewReadReq(11, cu.ToOutside, l1v.ToCU, 0x108, 4)
		read1.SetRecvTime(engine.CurrentTime() + 1)
		l1v.ToCU.Recv(read1)
		engine.Run()
		Expect(cu.ReceivedReqs).To(HaveLen(2))
		Expect(cu.ReceivedReqs[1].(*mem.DataReadyRsp).Data).To(
			Equal([]byte{1, 2, 3, 4}))
	})

	It("should write", func() {
		write := mem.NewWriteReq(10, cu.ToOutside, l1v.ToCU, 0x100)
		write.IsLastInWave = true
		write.Data = []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		}
		l1v.ToCU.Recv(write)

		engine.Run()

		Expect(cu.ReceivedReqs).To(HaveLen(1))
		Expect(cu.ReceivedReqs[0].(*mem.DoneRsp).RespondTo).To(Equal(write.ID))
		data, _ := lowModule.Storage.Read(0x100, 64)
		Expect(data).To(Equal([]byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		}))
	})

	It("should coalesce write", func() {
		write := mem.NewWriteReq(10, cu.ToOutside, l1v.ToCU, 0x100)
		write.Data = []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
		}
		l1v.ToCU.Recv(write)

		write2 := mem.NewWriteReq(10, cu.ToOutside, l1v.ToCU, 0x108)
		write2.IsLastInWave = true
		write2.Data = []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
		}
		l1v.ToCU.Recv(write2)

		engine.Run()

		Expect(cu.ReceivedReqs).To(HaveLen(2))
		Expect(cu.ReceivedReqs[0].(*mem.DoneRsp).RespondTo).To(Equal(write.ID))
		data, _ := lowModule.Storage.Read(0x100, 16)
		Expect(data).To(Equal([]byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		}))
	})

	//	It("should read hit after writing a full cache line", func() {
	//		startTime := engine.CurrentTime()
	//
	//		read := mem.NewReadReq(0, cu.ToOutside, l1v.ToCU, 0x104, 4)
	//		read.SetRecvTime(0)
	//		l1v.ToCU.Recv(read)
	//		engine.Run()
	//		Expect(cu.ReceivedReqs).To(HaveLen(1))
	//		time1 := engine.CurrentTime()
	//		duration1 := time1 - startTime
	//
	//		write := mem.NewWriteReq(10, cu.ToOutside, l1v.ToCU, 0x140)
	//		write.SetRecvTime(time1 + 1)
	//		write.Data = []byte{
	//			1, 2, 3, 4, 5, 6, 7, 8,
	//			1, 2, 3, 4, 5, 6, 7, 8,
	//			1, 2, 3, 4, 5, 6, 7, 8,
	//			1, 2, 3, 4, 5, 6, 7, 8,
	//			1, 2, 3, 4, 5, 6, 7, 8,
	//			1, 2, 3, 4, 5, 6, 7, 8,
	//			1, 2, 3, 4, 5, 6, 7, 8,
	//			1, 2, 3, 4, 5, 6, 7, 8,
	//		}
	//		l1v.ToCU.Recv(write)
	//		engine.Run()
	//		Expect(cu.ReceivedReqs).To(HaveLen(2))
	//		time2 := engine.CurrentTime()
	//
	//		read.Address = 0x144
	//		read.SetRecvTime(time2 + 1)
	//		l1v.ToCU.Recv(read)
	//		engine.Run()
	//		Expect(cu.ReceivedReqs).To(HaveLen(3))
	//		time3 := engine.CurrentTime()
	//		duration2 := time3 - time2
	//
	//		Expect(duration2).To(BeNumerically("<", duration1))
	//
	//	})
})
