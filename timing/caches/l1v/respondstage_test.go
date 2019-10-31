package l1v

import (
	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem"
)

var _ = Describe("Respond Stage", func() {
	var (
		mockCtrl *gomock.Controller
		cache    *Cache
		topPort  *MockPort
		s        *respondStage
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		topPort = NewMockPort(mockCtrl)
		cache = &Cache{
			TopPort: topPort,
		}
		cache.TickingComponent = akita.NewTickingComponent(
			"cache", nil, 1, cache)
		s = &respondStage{cache: cache}

	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do nothing if there is no transaction", func() {
		madeProgress := s.Tick(10)
		Expect(madeProgress).To(BeFalse())
	})

	Context("read", func() {
		var (
			read  *mem.ReadReq
			trans *transaction
		)

		BeforeEach(func() {
			read = mem.ReadReqBuilder{}.
				WithSendTime(5).
				WithAddress(0x100).
				WithPID(1).
				WithByteSize(4).
				Build()
			trans = &transaction{read: read}
			cache.transactions = append(cache.transactions, trans)
		})

		It("should do nothing if the transaction is not ready", func() {
			madeProgress := s.Tick(10)
			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if cannot send to top", func() {
			trans.data = []byte{1, 2, 3, 4}
			trans.done = true
			topPort.EXPECT().Send(gomock.Any()).Return(&akita.SendError{})

			madeProgress := s.Tick(10)

			Expect(madeProgress).To(BeFalse())
		})

		It("should send data ready to top", func() {
			trans.data = []byte{1, 2, 3, 4}
			trans.done = true
			topPort.EXPECT().Send(gomock.Any()).
				Do(func(dr *mem.DataReadyRsp) {
					Expect(dr.RespondTo).To(Equal(read.ID))
					Expect(dr.Data).To(Equal([]byte{1, 2, 3, 4}))
				})

			madeProgress := s.Tick(10)

			Expect(madeProgress).To(BeTrue())
			Expect(cache.transactions).NotTo(ContainElement((trans)))
		})
	})

	Context("write", func() {
		var (
			write *mem.WriteReq
			trans *transaction
		)

		BeforeEach(func() {
			write = mem.WriteReqBuilder{}.
				WithSendTime(5).
				WithAddress(0x100).
				WithPID(1).
				Build()
			trans = &transaction{write: write}
			cache.transactions = append(cache.transactions, trans)
		})

		It("should do nothing if the transaction is not ready", func() {
			madeProgress := s.Tick(10)
			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if cannot send to top", func() {
			trans.done = true
			topPort.EXPECT().Send(gomock.Any()).Return(&akita.SendError{})

			madeProgress := s.Tick(10)

			Expect(madeProgress).To(BeFalse())
		})

		It("should send data ready to top", func() {
			trans.data = []byte{1, 2, 3, 4}
			trans.done = true
			topPort.EXPECT().Send(gomock.Any()).
				Do(func(done *mem.WriteDoneRsp) {
					Expect(done.RespondTo).To(Equal(write.ID))
				})

			madeProgress := s.Tick(10)

			Expect(madeProgress).To(BeTrue())
			Expect(cache.transactions).NotTo(ContainElement((trans)))
		})
	})

})
