package l1v

import (
	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Bankstage", func() {
	var (
		mockCtrl *gomock.Controller
		inBuf    *MockBuffer
		s        *bankStage
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		inBuf = NewMockBuffer(mockCtrl)
		s = &bankStage{
			inBuf:   inBuf,
			latency: 10,
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
		// preCRead1, preCRead2, postCRead *mem.ReadReq
		)

		It("should read", func() {

		})
	})

})
