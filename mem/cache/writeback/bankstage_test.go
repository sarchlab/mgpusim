package writeback

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/mem/cache"
	"github.com/sarchlab/mgpusim/v3/mem/mem"
)

var _ = Describe("Bank Stage", func() {
	var (
		mockCtrl          *gomock.Controller
		cacheModule       *Cache
		pipeline          *MockPipeline
		postPipelineBuf   *bufferImpl
		dirInBuf          *MockBuffer
		writeBufferInBuf  *MockBuffer
		bs                *bankStage
		storage           *mem.Storage
		topSender         *MockBufferedSender
		writeBufferBuffer *MockBuffer
		mshrStageBuffer   *MockBuffer
		lowModuleFinder   *MockLowModuleFinder
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		pipeline = NewMockPipeline(mockCtrl)
		postPipelineBuf = &bufferImpl{capacity: 2}
		dirInBuf = NewMockBuffer(mockCtrl)
		writeBufferInBuf = NewMockBuffer(mockCtrl)
		mshrStageBuffer = NewMockBuffer(mockCtrl)
		topSender = NewMockBufferedSender(mockCtrl)
		writeBufferBuffer = NewMockBuffer(mockCtrl)
		lowModuleFinder = NewMockLowModuleFinder(mockCtrl)
		storage = mem.NewStorage(4 * mem.KB)

		builder := MakeBuilder()
		cacheModule = builder.Build("Cache")
		cacheModule.dirToBankBuffers = []sim.Buffer{dirInBuf}
		cacheModule.writeBufferToBankBuffers =
			[]sim.Buffer{writeBufferInBuf}
		cacheModule.mshrStageBuffer = mshrStageBuffer
		cacheModule.topSender = topSender
		cacheModule.writeBufferBuffer = writeBufferBuffer
		cacheModule.lowModuleFinder = lowModuleFinder
		cacheModule.storage = storage
		cacheModule.inFlightTransactions = nil

		bs = &bankStage{
			cache:           cacheModule,
			bankID:          0,
			pipeline:        pipeline,
			pipelineWidth:   4,
			postPipelineBuf: postPipelineBuf,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("No transaction running", func() {
		It("should do nothing if pipeline is full", func() {
			pipeline.EXPECT().Tick(sim.VTimeInSec(10))
			pipeline.EXPECT().CanAccept().Return(false)

			ret := bs.Tick(10)

			Expect(ret).To(BeFalse())
		})

		It("should do nothing if there is no transaction", func() {
			pipeline.EXPECT().Tick(sim.VTimeInSec(10))
			pipeline.EXPECT().CanAccept().Return(true)
			writeBufferInBuf.EXPECT().Pop().Return(nil)
			writeBufferBuffer.EXPECT().CanPush().Return(true)
			dirInBuf.EXPECT().Pop().Return(nil)

			ret := bs.Tick(10)

			Expect(ret).To(BeFalse())
		})

		It("should extract transactions from write buffer first", func() {
			trans := &transaction{}

			pipeline.EXPECT().Tick(sim.VTimeInSec(10))
			writeBufferInBuf.EXPECT().Pop().Return(trans)
			pipeline.EXPECT().CanAccept().Return(true)
			pipeline.EXPECT().Accept(sim.VTimeInSec(10), gomock.Any())
			ret := bs.Tick(10)

			Expect(ret).To(BeTrue())
			Expect(bs.inflightTransCount).To(Equal(1))
		})

		It("should stall if write buffer buffer is full", func() {
			pipeline.EXPECT().Tick(sim.VTimeInSec(10))
			pipeline.EXPECT().CanAccept().Return(true)
			writeBufferInBuf.EXPECT().Pop().Return(nil)
			writeBufferBuffer.EXPECT().CanPush().Return(false)

			ret := bs.Tick(10)

			Expect(ret).To(BeFalse())
		})

		It("should extract transactions from directory", func() {
			trans := &transaction{}

			pipeline.EXPECT().Tick(sim.VTimeInSec(10))
			pipeline.EXPECT().CanAccept().Return(true)
			pipeline.EXPECT().Accept(sim.VTimeInSec(10), gomock.Any())
			writeBufferInBuf.EXPECT().Pop().Return(nil)
			writeBufferBuffer.EXPECT().CanPush().Return(true)
			dirInBuf.EXPECT().Pop().Return(trans)

			ret := bs.Tick(10)

			Expect(ret).To(BeTrue())
			Expect(bs.inflightTransCount).To(Equal(1))
		})

		It("should directly forward fetch transaction to writebuffer", func() {
			trans := &transaction{
				action: writeBufferFetch,
			}

			pipeline.EXPECT().Tick(sim.VTimeInSec(10))
			pipeline.EXPECT().CanAccept().Return(true)
			writeBufferInBuf.EXPECT().Pop().Return(nil)
			writeBufferBuffer.EXPECT().CanPush().Return(true)
			writeBufferBuffer.EXPECT().Push(trans)
			dirInBuf.EXPECT().Pop().Return(trans)
			ret := bs.Tick(10)

			Expect(ret).To(BeTrue())
		})
	})

	Context("completing a read hit transaction", func() {
		var (
			read  *mem.ReadReq
			block *cache.Block
			trans *transaction
		)

		BeforeEach(func() {
			storage.Write(0x40, []byte{1, 2, 3, 4, 5, 6, 7, 8})
			read = mem.ReadReqBuilder{}.
				WithSendTime(6).
				WithAddress(0x104).
				WithByteSize(4).
				Build()
			block = &cache.Block{
				CacheAddress: 0x40,
				ReadCount:    1,
			}
			trans = &transaction{
				read:   read,
				block:  block,
				action: bankReadHit,
			}
			postPipelineBuf.Push(bankPipelineElem{trans: trans})
			cacheModule.inFlightTransactions = append(
				cacheModule.inFlightTransactions, trans)

			pipeline.EXPECT().Tick(sim.VTimeInSec(10))
			pipeline.EXPECT().CanAccept().Return(false)
			bs.inflightTransCount = 1
		})

		It("should stall if send buffer is full", func() {
			topSender.EXPECT().CanSend(1).Return(false)

			ret := bs.Tick(10)

			Expect(ret).To(BeFalse())
			Expect(bs.inflightTransCount).To(Equal(1))
			Expect(postPipelineBuf.Size()).To(Equal(1))
		})

		It("should read and send response", func() {
			topSender.EXPECT().CanSend(1).Return(true)
			topSender.EXPECT().Send(gomock.Any()).
				Do(func(dr *mem.DataReadyRsp) {
					Expect(dr.RespondTo).To(Equal(read.ID))
					Expect(dr.Data).To(Equal([]byte{5, 6, 7, 8}))
				})

			ret := bs.Tick(10)

			Expect(ret).To(BeTrue())
			Expect(block.ReadCount).To(Equal(0))
			Expect(cacheModule.inFlightTransactions).
				NotTo(ContainElement(trans))
			Expect(bs.inflightTransCount).To(Equal(0))
			Expect(postPipelineBuf.Size()).To(Equal(0))
		})
	})

	Context("completing a write-hit transaction", func() {
		var (
			write *mem.WriteReq
			block *cache.Block
			trans *transaction
		)

		BeforeEach(func() {
			write = mem.WriteReqBuilder{}.
				WithSendTime(6).
				WithAddress(0x104).
				WithData([]byte{5, 6, 7, 8}).
				Build()
			block = &cache.Block{
				CacheAddress: 0x40,
				ReadCount:    1,
				IsLocked:     true,
			}
			trans = &transaction{
				write:  write,
				block:  block,
				action: bankWriteHit,
			}
			cacheModule.inFlightTransactions = append(
				cacheModule.inFlightTransactions, trans)
			postPipelineBuf.Push(bankPipelineElem{trans: trans})
			pipeline.EXPECT().Tick(sim.VTimeInSec(10))
			pipeline.EXPECT().CanAccept().Return(false)
			bs.inflightTransCount = 1
		})

		It("should stall if send buffer is full", func() {
			topSender.EXPECT().CanSend(1).Return(false)

			ret := bs.Tick(10)

			Expect(ret).To(BeFalse())
			Expect(bs.inflightTransCount).To(Equal(1))
			Expect(postPipelineBuf.Size()).To(Equal(1))
		})

		It("should write and send response", func() {
			topSender.EXPECT().CanSend(1).Return(true)
			topSender.EXPECT().Send(gomock.Any()).
				Do(func(done *mem.WriteDoneRsp) {
					Expect(done.RespondTo).To(Equal(write.ID))
				})

			ret := bs.Tick(10)

			Expect(ret).To(BeTrue())
			data, _ := storage.Read(0x44, 4)
			Expect(data).To(Equal([]byte{5, 6, 7, 8}))
			Expect(block.IsValid).To(BeTrue())
			Expect(block.IsLocked).To(BeFalse())
			Expect(block.IsDirty).To(BeTrue())
			Expect(block.DirtyMask).To(Equal([]bool{
				false, false, false, false, true, true, true, true,
				false, false, false, false, false, false, false, false,
				false, false, false, false, false, false, false, false,
				false, false, false, false, false, false, false, false,
				false, false, false, false, false, false, false, false,
				false, false, false, false, false, false, false, false,
				false, false, false, false, false, false, false, false,
				false, false, false, false, false, false, false, false,
			}))
			Expect(cacheModule.inFlightTransactions).
				NotTo(ContainElement(trans))
			Expect(bs.inflightTransCount).To(Equal(0))
			Expect(postPipelineBuf.Size()).To(Equal(0))
		})
	})

	Context("completing a write fetched transaction", func() {
		var (
			block     *cache.Block
			mshrEntry *cache.MSHREntry
			trans     *transaction
		)

		BeforeEach(func() {
			block = &cache.Block{
				CacheAddress: 0x40,
				IsLocked:     true,
			}
			mshrEntry = &cache.MSHREntry{
				Data: []byte{
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
				},
				Block: block,
			}
			trans = &transaction{
				mshrEntry: mshrEntry,
				action:    bankWriteFetched,
			}
			postPipelineBuf.Push(bankPipelineElem{trans: trans})

			pipeline.EXPECT().Tick(sim.VTimeInSec(10))
			pipeline.EXPECT().CanAccept().Return(false)
			bs.inflightTransCount = 1
		})

		It("should stall if the mshr stage buffer is full", func() {
			mshrStageBuffer.EXPECT().CanPush().Return(false)

			ret := bs.Tick(10)

			Expect(ret).To(BeFalse())
			Expect(bs.inflightTransCount).To(Equal(1))
			Expect(postPipelineBuf.Size()).To(Equal(1))
		})

		It("should write to storage and send to mshr stage", func() {
			mshrStageBuffer.EXPECT().CanPush().Return(true)
			mshrStageBuffer.EXPECT().Push(mshrEntry)

			ret := bs.Tick(10)

			Expect(ret).To(BeTrue())
			writtenData, _ := storage.Read(0x40, 64)
			Expect(writtenData).To(Equal(mshrEntry.Data))
			Expect(block.IsLocked).To(BeFalse())
			Expect(block.IsValid).To(BeTrue())
			Expect(bs.inflightTransCount).To(Equal(0))
			Expect(postPipelineBuf.Size()).To(Equal(0))
		})
	})

	Context("finalizing a read for eviction action", func() {
		var (
			victim *cache.Block
			trans  *transaction
		)

		BeforeEach(func() {
			victim = &cache.Block{
				Tag:          0x200,
				CacheAddress: 0x300,
				DirtyMask: []bool{
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
					true, true, true, true, false, false, false, false,
				},
			}
			trans = &transaction{
				victim: victim,
				action: bankEvictAndFetch,
			}
			postPipelineBuf.Push(bankPipelineElem{trans: trans})
			pipeline.EXPECT().Tick(sim.VTimeInSec(10))
			pipeline.EXPECT().CanAccept().Return(false)
			bs.inflightTransCount = 1
		})

		It("should stall if the bottom sender is busy", func() {
			writeBufferBuffer.EXPECT().CanPush().Return(false)

			ret := bs.Tick(10)

			Expect(ret).To(BeFalse())
			Expect(bs.inflightTransCount).To(Equal(1))
			Expect(postPipelineBuf.Size()).To(Equal(1))
		})

		It("should send write to bottom", func() {
			data := []byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			}
			storage.Write(0x300, data)
			writeBufferBuffer.EXPECT().CanPush().Return(true)
			writeBufferBuffer.EXPECT().Push(gomock.Any()).
				Do(func(eviction *transaction) {
					Expect(eviction.action).To(Equal(writeBufferEvictAndFetch))
					Expect(eviction.evictingData).To(Equal(data))
				})

			ret := bs.Tick(10)

			Expect(ret).To(BeTrue())
			Expect(bs.inflightTransCount).To(Equal(0))
			Expect(postPipelineBuf.Size()).To(Equal(0))
		})
	})
})
