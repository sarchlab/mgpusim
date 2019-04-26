package l1v

import (
	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
	"gitlab.com/akita/util"
)

var _ = Describe("Directory", func() {
	var (
		mockCtrl *gomock.Controller
		inBuf    *MockBuffer
		dir      *MockDirectory
		mshr     *MockMSHR
		bankBuf  *MockBuffer
		d        *directory
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		inBuf = NewMockBuffer(mockCtrl)
		dir = NewMockDirectory(mockCtrl)
		dir.EXPECT().WayAssociativity().Return(4).AnyTimes()
		mshr = NewMockMSHR(mockCtrl)
		bankBuf = NewMockBuffer(mockCtrl)
		d = &directory{
			inBuf:         inBuf,
			dir:           dir,
			mshr:          mshr,
			bankBufs:      []util.Buffer{bankBuf},
			log2BlockSize: 6,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do nothing if no transaction", func() {
		inBuf.EXPECT().Peek().Return(nil)
		madeProgress := d.Tick(10)
		Expect(madeProgress).To(BeFalse())
	})

	Context("read mshr hit", func() {
		var (
			read  *mem.ReadReq
			trans *transaction
		)

		BeforeEach(func() {
			read = mem.NewReadReq(6, nil, nil, 0x104, 4)
			trans = &transaction{
				read: read,
			}
			inBuf.EXPECT().Peek().Return(trans)
		})

		It("Should add to mshr entry", func() {
			mshrEntry := &cache.MSHREntry{}
			mshr.EXPECT().Query(uint64(0x100)).Return(mshrEntry)
			inBuf.EXPECT().Pop()

			madeProgress := d.Tick(10)

			Expect(madeProgress).To(BeTrue())
			Expect(mshrEntry.Requests).To(ContainElement(trans))
		})
	})

	Context("read hit", func() {
		var (
			block *cache.Block
			read  *mem.ReadReq
			trans *transaction
		)

		BeforeEach(func() {
			block = &cache.Block{
				IsValid: true,
			}
			read = mem.NewReadReq(6, nil, nil, 0x104, 4)
			trans = &transaction{
				read: read,
			}
			inBuf.EXPECT().Peek().Return(trans)
			mshr.EXPECT().Query(gomock.Any()).Return(nil)
		})

		It("should send transaction to bank", func() {
			dir.EXPECT().Lookup(uint64(0x100)).Return(block)
			dir.EXPECT().Visit(block)
			bankBuf.EXPECT().CanPush().Return(true)
			bankBuf.EXPECT().Push(gomock.Any()).
				Do(func(t *transaction) {
					Expect(t.block).To(BeIdenticalTo(block))
					Expect(t.bankAction).To(Equal(bankActionReadHit))
				})

			madeProgress := d.Tick(10)

			Expect(madeProgress).To(BeTrue())
			Expect(block.ReadCount).To(Equal(1))
		})

		It("should stall if cannot send to bank", func() {
			dir.EXPECT().Lookup(uint64(0x100)).Return(block)
			bankBuf.EXPECT().CanPush().Return(false)

			madeProgress := d.Tick(10)

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if block is locked", func() {
			block.IsLocked = true
			dir.EXPECT().Lookup(uint64(0x100)).Return(block)
			madeProgress := d.Tick(10)
			Expect(madeProgress).To(BeFalse())
		})

	})

})
