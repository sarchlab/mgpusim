package l1v

import (
	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
)

var _ = Describe("Bankstage", func() {
	var (
		mockCtrl          *gomock.Controller
		inBuf             *MockBuffer
		storage           *mem.Storage
		postCTransactions []*transaction
		s                 *bankStage
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		inBuf = NewMockBuffer(mockCtrl)
		storage = mem.NewStorage(4 * mem.KB)
		postCTransactions = nil
		s = &bankStage{
			inBuf:             inBuf,
			storage:           storage,
			postCTransactions: &postCTransactions,
			latency:           10,
			log2BlockSize:     6,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do nothing if no request", func() {
		inBuf.EXPECT().Peek().Return(nil)
		madeProgress := s.Tick(10)
		Expect(madeProgress).To(BeFalse())
	})

	It("should start count down", func() {
		trans := &transaction{}

		inBuf.EXPECT().Peek().Return(trans)
		inBuf.EXPECT().Pop()

		madeProgress := s.Tick(10)

		Expect(madeProgress).To(BeTrue())
		Expect(s.currTrans).To(BeIdenticalTo(trans))
		Expect(s.cycleLeft).To(Equal(10))
	})

	It("should count down", func() {
		trans := &transaction{}
		s.currTrans = trans
		s.cycleLeft = 10

		madeProgress := s.Tick(10)

		Expect(madeProgress).To(BeTrue())
		Expect(s.cycleLeft).To(Equal(9))
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
			preCRead1 = mem.NewReadReq(1, nil, nil, 0x104, 4)
			preCRead2 = mem.NewReadReq(2, nil, nil, 0x108, 8)
			postCRead = mem.NewReadReq(0, nil, nil, 0x100, 64)
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
			postCTransactions = append(postCTransactions, postCTrans)

			s.currTrans = postCTrans
			s.cycleLeft = 0
		})

		It("should read", func() {
			madeProgress := s.Tick(10)

			Expect(madeProgress).To(BeTrue())
			Expect(s.currTrans).To(BeNil())
			Expect(preCTrans1.data).To(Equal([]byte{5, 6, 7, 8}))
			Expect(preCTrans1.done).To(BeTrue())
			Expect(preCTrans2.data).To(Equal([]byte{1, 2, 3, 4, 5, 6, 7, 8}))
			Expect(preCTrans2.done).To(BeTrue())
			Expect(block.ReadCount).To(Equal(0))
			Expect(postCTransactions).NotTo(ContainElement(postCTrans))
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

			write = mem.NewWriteReq(1, nil, nil, 0x100)
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
			write.DirtyMask = []bool{
				false, false, false, false, false, false, false, false,
				true, true, true, true, true, true, true, true,
				false, false, false, false, false, false, false, false,
				false, false, false, false, false, false, false, false,
				false, false, false, false, false, false, false, false,
				false, false, false, false, false, false, false, false,
				false, false, false, false, false, false, false, false,
				false, false, false, false, false, false, false, false,
			}
			trans = &transaction{
				write:      write,
				block:      block,
				bankAction: bankActionWrite,
			}

			s.currTrans = trans
			s.cycleLeft = 0
		})

		It("should write", func() {
			madeProgress := s.Tick(10)

			Expect(madeProgress).To(BeTrue())
			Expect(s.currTrans).To(BeNil())
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

			s.currTrans = trans
			s.cycleLeft = 0
		})

		It("should write fetched", func() {
			madeProgress := s.Tick(10)

			Expect(madeProgress).To(BeTrue())
			Expect(s.currTrans).To(BeNil())
			Expect(block.IsLocked).To(BeFalse())
			data, _ := storage.Read(0x400, 64)
			Expect(data).To(Equal(trans.data))
		})
	})

})
