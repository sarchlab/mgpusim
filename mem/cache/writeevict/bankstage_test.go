package writeevict

import (
	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/mem/cache"
	"github.com/sarchlab/mgpusim/v3/mem/mem"
)

var _ = Describe("Bankstage", func() {
	var (
		mockCtrl        *gomock.Controller
		inBuf           *MockBuffer
		storage         *mem.Storage
		pipeline        *MockPipeline
		postPipelineBuf *MockBuffer
		s               *bankStage
		c               *Cache
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		inBuf = NewMockBuffer(mockCtrl)
		storage = mem.NewStorage(4 * mem.KB)
		pipeline = NewMockPipeline(mockCtrl)
		postPipelineBuf = NewMockBuffer(mockCtrl)
		c = &Cache{
			bankLatency:   10,
			bankBufs:      []sim.Buffer{inBuf},
			storage:       storage,
			log2BlockSize: 6,
		}
		c.TickingComponent = sim.NewTickingComponent(
			"Cache", nil, 1, c)
		s = &bankStage{
			cache:           c,
			bankID:          0,
			numReqPerCycle:  1,
			pipeline:        pipeline,
			postPipelineBuf: postPipelineBuf,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do nothing if no request", func() {
		pipeline.EXPECT().Tick(sim.VTimeInSec(10)).Return(false)
		inBuf.EXPECT().Peek().Return(nil)
		postPipelineBuf.EXPECT().Peek().Return(nil)

		madeProgress := s.Tick(10)

		Expect(madeProgress).To(BeFalse())
	})

	It("should insert transactions into pipeline", func() {
		trans := &transaction{}

		inBuf.EXPECT().Peek().Return(trans)
		inBuf.EXPECT().Pop()
		pipeline.EXPECT().Tick(sim.VTimeInSec(10)).Return(false)
		pipeline.EXPECT().CanAccept().Return(true)
		pipeline.EXPECT().
			Accept(sim.VTimeInSec(10), gomock.Any()).
			Do(func(now sim.VTimeInSec, t *bankTransaction) {
				Expect(t.transaction).To(BeIdenticalTo(trans))
			})
		postPipelineBuf.EXPECT().Peek().Return(nil)

		madeProgress := s.Tick(10)

		Expect(madeProgress).To(BeTrue())
	})

	Context("read hit", func() {
		var (
			preCRead1, preCRead2, postCRead    *mem.ReadReq
			preCTrans1, preCTrans2, postCTrans *transaction
			block                              *cache.Block
		)

		BeforeEach(func() {
			storage.Write(0x400, []byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			})
			block = &cache.Block{
				Tag:          0x100,
				CacheAddress: 0x400,
				ReadCount:    1,
			}
			preCRead1 = mem.ReadReqBuilder{}.
				WithSendTime(1).
				WithAddress(0x104).
				WithByteSize(4).
				Build()
			preCRead2 = mem.ReadReqBuilder{}.
				WithSendTime(2).
				WithAddress(0x108).
				WithByteSize(8).
				Build()
			postCRead = mem.ReadReqBuilder{}.
				WithAddress(0x100).
				WithByteSize(64).
				Build()
			preCTrans1 = &transaction{read: preCRead1}
			preCTrans2 = &transaction{read: preCRead2}
			postCTrans = &transaction{
				read:       postCRead,
				block:      block,
				bankAction: bankActionReadHit,
				preCoalesceTransactions: []*transaction{
					preCTrans1, preCTrans2,
				},
			}
			c.postCoalesceTransactions = append(
				c.postCoalesceTransactions, postCTrans)

			postPipelineBuf.EXPECT().Peek().Return(&bankTransaction{
				transaction: postCTrans,
			})
		})

		It("should read", func() {
			pipeline.EXPECT().Tick(sim.VTimeInSec(10))
			inBuf.EXPECT().Peek().Return(nil)
			postPipelineBuf.EXPECT().Pop()

			madeProgress := s.Tick(10)

			Expect(madeProgress).To(BeTrue())
			Expect(preCTrans1.data).To(Equal([]byte{5, 6, 7, 8}))
			Expect(preCTrans1.done).To(BeTrue())
			Expect(preCTrans2.data).To(Equal([]byte{1, 2, 3, 4, 5, 6, 7, 8}))
			Expect(preCTrans2.done).To(BeTrue())
			Expect(block.ReadCount).To(Equal(0))
			Expect(c.postCoalesceTransactions).NotTo(ContainElement(postCTrans))
		})
	})

	Context("write", func() {
		var (
			write *mem.WriteReq
			trans *transaction
			block *cache.Block
		)

		BeforeEach(func() {
			block = &cache.Block{
				Tag:          0x100,
				CacheAddress: 0x400,
				IsLocked:     true,
			}

			write = mem.WriteReqBuilder{}.
				WithSendTime(1).
				WithAddress(0x100).
				WithData([]byte{
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
					1, 2, 3, 4, 5, 6, 7, 8,
				}).
				WithDirtyMask([]bool{
					false, false, false, false, false, false, false, false,
					true, true, true, true, true, true, true, true,
					false, false, false, false, false, false, false, false,
					false, false, false, false, false, false, false, false,
					false, false, false, false, false, false, false, false,
					false, false, false, false, false, false, false, false,
					false, false, false, false, false, false, false, false,
					false, false, false, false, false, false, false, false,
				}).
				Build()
			trans = &transaction{
				write:      write,
				block:      block,
				bankAction: bankActionWrite,
			}

			postPipelineBuf.EXPECT().
				Peek().
				Return(&bankTransaction{transaction: trans})
		})

		It("should write", func() {
			pipeline.EXPECT().Tick(sim.VTimeInSec(10))
			inBuf.EXPECT().Peek().Return(nil)
			postPipelineBuf.EXPECT().Pop()

			madeProgress := s.Tick(10)

			Expect(madeProgress).To(BeTrue())
			Expect(block.IsLocked).To(BeFalse())
			data, _ := storage.Read(0x400, 64)
			Expect(data).To(Equal([]byte{
				0, 0, 0, 0, 0, 0, 0, 0,
				1, 2, 3, 4, 5, 6, 7, 8,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
			}))
		})
	})

	Context("write fetched", func() {
		var (
			trans *transaction
			block *cache.Block
		)

		BeforeEach(func() {
			block = &cache.Block{
				Tag:          0x100,
				CacheAddress: 0x400,
				IsLocked:     true,
			}

			trans = &transaction{
				block:      block,
				bankAction: bankActionWriteFetched,
			}
			trans.data = []byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			}
			trans.writeFetchedDirtyMask = make([]bool, 64)

			postPipelineBuf.EXPECT().
				Peek().
				Return(&bankTransaction{transaction: trans})
		})

		It("should write fetched", func() {
			pipeline.EXPECT().Tick(sim.VTimeInSec(10))
			inBuf.EXPECT().Peek().Return(nil)
			postPipelineBuf.EXPECT().Pop()

			madeProgress := s.Tick(10)

			Expect(madeProgress).To(BeTrue())
			// Expect(s.currTrans).To(BeNil())
			Expect(block.IsLocked).To(BeFalse())
			data, _ := storage.Read(0x400, 64)
			Expect(data).To(Equal(trans.data))
		})
	})
})
