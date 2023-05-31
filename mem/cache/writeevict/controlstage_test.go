package writeevict

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/mem/cache"
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
		cacheComp    *Cache
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
		c = &coalescer{cache: cacheComp}

		transactions = nil

		cacheComp = &Cache{
			topPort:               topPort,
			bottomPort:            bottomPort,
			dirBuf:                inBuf,
			mshr:                  mshr,
			coalesceStage:         c,
			maxNumConcurrentTrans: 32,
		}
		cacheComp.TickingComponent = sim.NewTickingComponent(
			"Cache", nil, 1, cacheComp)

		s = &controlStage{
			ctrlPort:     ctrlPort,
			transactions: &transactions,
			directory:    directory,
			cache:        cacheComp,
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
		flushReq := cache.FlushReqBuilder{}.Build()
		flushReq.DiscardInflight = false
		s.currFlushReq = flushReq
		ctrlPort.EXPECT().Peek().Return(flushReq)

		madeProgress := s.Tick(10)

		Expect(madeProgress).To(BeFalse())
	})

	It("should reset directory", func() {
		flushReq := cache.FlushReqBuilder{}.
			InvalidateAllCacheLines().
			DiscardInflight().
			PauseAfterFlushing().
			Build()
		s.currFlushReq = flushReq
		ctrlPort.EXPECT().Send(gomock.Any()).Do(func(rsp *cache.FlushRsp) {
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
