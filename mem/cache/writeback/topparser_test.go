package writeback

import (
	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/mem/mem"
)

var _ = Describe("TopParser", func() {
	var (
		mockCtrl *gomock.Controller
		cache    *Cache
		parser   *topParser
		port     *MockPort
		buf      *MockBuffer
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		port = NewMockPort(mockCtrl)
		buf = NewMockBuffer(mockCtrl)

		builder := MakeBuilder()
		cache = builder.Build("Cache")

		parser = &topParser{
			cache: cache,
		}
		cache.state = cacheStateRunning
		cache.topPort = port
		cache.dirStageBuffer = buf
		cache.inFlightTransactions = nil
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should return if no req to parse", func() {
		port.EXPECT().Peek().Return(nil)
		ret := parser.Tick(10)
		Expect(ret).To(BeFalse())
	})

	It("should return if the cache is not in running stage", func() {
		cache.state = cacheStateFlushing
		ret := parser.Tick(10)
		Expect(ret).To(BeFalse())
	})

	It("should return if the dir buf is full", func() {
		read := mem.ReadReqBuilder{}.
			WithSendTime(6).
			WithAddress(0x100).
			WithByteSize(64).
			Build()
		port.EXPECT().Peek().Return(read)
		buf.EXPECT().CanPush().Return(false)

		ret := parser.Tick(10)

		Expect(ret).To(BeFalse())
	})

	It("should parse read from top", func() {
		read := mem.ReadReqBuilder{}.
			WithSendTime(10).
			WithAddress(0x100).
			WithByteSize(64).
			Build()

		port.EXPECT().Peek().Return(read)
		buf.EXPECT().CanPush().Return(true)
		buf.EXPECT().Push(gomock.Any()).Do(func(t *transaction) {
			Expect(t.read).To(BeIdenticalTo(read))
		})
		port.EXPECT().Retrieve(sim.VTimeInSec(10)).Return(read)

		parser.Tick(10)

		Expect(cache.inFlightTransactions).To(HaveLen(1))
	})

	It("should parse write from top", func() {
		write := mem.WriteReqBuilder{}.
			WithSendTime(10).
			WithAddress(0x100).
			Build()

		port.EXPECT().Peek().Return(write)
		buf.EXPECT().CanPush().Return(true)
		buf.EXPECT().Push(gomock.Any()).Do(func(t *transaction) {
			Expect(t.write).To(BeIdenticalTo(write))
		})
		port.EXPECT().Retrieve(sim.VTimeInSec(10)).Return(write)

		parser.Tick(10)

		Expect(cache.inFlightTransactions).To(HaveLen(1))
	})

})
