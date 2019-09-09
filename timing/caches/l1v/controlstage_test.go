package l1v

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem/cache"
)

var _ = Describe("Control Stage", func() {

	var (
		mockCtrl     *gomock.Controller
		ctrlPort     *MockPort
		transactions []*transaction
		directory    *MockDirectory
		s            *controlStage
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		ctrlPort = NewMockPort(mockCtrl)
		directory = NewMockDirectory(mockCtrl)

		transactions = nil

		s = &controlStage{
			ctrlPort:     ctrlPort,
			transactions: &transactions,
			directory:    directory,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do nothing if no request", func() {
		ctrlPort.EXPECT().Peek().Return(nil)

		madeProgress := s.Tick(10)

		Expect(madeProgress).To(BeFalse())
	})

	It("should wait for the cache to finish transactions", func() {
		transactions = []*transaction{{}}
		flushReq := cache.FlushReqBuilder{}.Build()
		s.currReq = flushReq

		madeProgress := s.Tick(10)

		Expect(madeProgress).To(BeFalse())
	})

	It("should reset directory", func() {
		flushReq := cache.FlushReqBuilder{}.Build()
		ctrlPort.EXPECT().Peek().Return(flushReq)
		ctrlPort.EXPECT().Retrieve(gomock.Any())
		directory.EXPECT().Reset()
		ctrlPort.EXPECT().Send(gomock.Any()).Do(func(rsp *cache.FlushRsp) {
			Expect(rsp.RspTo).To(Equal(flushReq.ID))
		})

		madeProgress := s.Tick(10)

		Expect(madeProgress).To(BeTrue())
		Expect(s.currReq).To(BeNil())
	})

	It("should stall if send rsp failed", func() {
		flushReq := cache.FlushReqBuilder{}.Build()
		ctrlPort.EXPECT().Peek().Return(flushReq)
		ctrlPort.EXPECT().Retrieve(gomock.Any())
		ctrlPort.EXPECT().Send(gomock.Any()).
			Return(&akita.SendError{})

		madeProgress := s.Tick(10)

		Expect(madeProgress).To(BeFalse())
		Expect(s.currReq).NotTo(BeNil())
	})

})
