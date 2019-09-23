package l1v

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	cache2 "gitlab.com/akita/mem/cache"
	"gitlab.com/akita/util/akitaext"
)

var _ = Describe("Control Stage", func() {

	var (
		mockCtrl     *gomock.Controller
		ctrlPort     *MockPort
		topPort      *MockPort
		bottomPort   *MockPort
		transactions []*transaction
		directory    *MockDirectory
		s            *controlStage
		cache        *Cache
		inBuf        *MockBuffer
		mshr         *MockMSHR
		c            *coalescer
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		ctrlPort = NewMockPort(mockCtrl)
		topPort = NewMockPort(mockCtrl)
		bottomPort = NewMockPort(mockCtrl)
		directory = NewMockDirectory(mockCtrl)
		inBuf = NewMockBuffer(mockCtrl)
		mshr = NewMockMSHR(mockCtrl)
		c = &coalescer{cache: cache}

		transactions = nil

		cache = &Cache{
			TopPort:       topPort,
			BottomPort:    bottomPort,
			dirBuf:        inBuf,
			mshr:          mshr,
			coalesceStage: c,
		}
		cache.TickingComponent = akitaext.NewTickingComponent(
			"cache", nil, 1, cache)

		s = &controlStage{
			ctrlPort:     ctrlPort,
			transactions: &transactions,
			directory:    directory,
			cache:        cache,
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
		s.cache.transactions = transactions
		flushReq := cache2.FlushReqBuilder{}.Build()
		flushReq.DiscardInflight = false
		s.currFlushReq = flushReq
		ctrlPort.EXPECT().Peek().Return(flushReq)

		madeProgress := s.Tick(10)

		Expect(madeProgress).To(BeFalse())
	})

	It("should reset directory", func() {
		flushReq := cache2.FlushReqBuilder{}.
			InvalidateAllCacheLines().
			DiscardInflight().
			PauseAfterFlushing().
			Build()
		s.currFlushReq = flushReq
		ctrlPort.EXPECT().Send(gomock.Any()).Do(func(rsp *cache2.FlushRsp) {
			Expect(rsp.RspTo).To(Equal(flushReq.ID))
		})

		topPort.EXPECT().Peek().Return(nil)
		bottomPort.EXPECT().Peek().Return(nil)
		inBuf.EXPECT().Pop()
		directory.EXPECT().Reset()
		mshr.EXPECT().Reset()

		ctrlPort.EXPECT().Peek().Return(flushReq)

		madeProgress := s.Tick(10)

		Expect(madeProgress).To(BeTrue())
		Expect(s.currFlushReq).To(BeNil())
	})

})
