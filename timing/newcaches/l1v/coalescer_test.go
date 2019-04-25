package l1v

import (
	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/mem"
)

var _ = Describe("Coalescer", func() {
	var (
		mockCtrl     *gomock.Controller
		topPort      *MockPort
		transactions []*transaction
		dirBuf       *MockBuffer
		c            coalescer
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		topPort = NewMockPort(mockCtrl)
		dirBuf = NewMockBuffer(mockCtrl)
		transactions = nil
		c = coalescer{
			log2BlockSize: 6,
			topPort:       topPort,
			transactions:  &transactions,
			dirBuf:        dirBuf,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do nothing if no request", func() {
		topPort.EXPECT().Peek().Return(nil)
		madeProgress := c.Tick(10)
		Expect(madeProgress).To(BeFalse())
	})

	It("should create transaction", func() {
		read := mem.NewReadReq(10, nil, nil, 0x100, 4)
		topPort.EXPECT().Peek().Return(read)
		topPort.EXPECT().Retrieve(gomock.Any())

		madeProgress := c.Tick(10)

		Expect(madeProgress).To(BeTrue())
		Expect(transactions).To(HaveLen(1))
		Expect(c.toCoalesce).To(HaveLen(1))
	})

	It("should wait in coalesce list if coalescing is possible", func() {
		read1 := mem.NewReadReq(10, nil, nil, 0x100, 4)
		read2 := mem.NewReadReq(10, nil, nil, 0x104, 4)

		topPort.EXPECT().Peek().Return(read1)
		topPort.EXPECT().Retrieve(gomock.Any())
		c.Tick(10)

		topPort.EXPECT().Peek().Return(read2)
		topPort.EXPECT().Retrieve(gomock.Any())
		madeProgress := c.Tick(11)

		Expect(madeProgress).To(BeTrue())
		Expect(transactions).To(HaveLen(2))
		Expect(c.toCoalesce).To(HaveLen(2))
	})

	It("should trigger coalesing if a request is not coalescable", func() {
		read1 := mem.NewReadReq(10, nil, nil, 0x100, 4)
		read2 := mem.NewReadReq(10, nil, nil, 0x104, 4)
		read3 := mem.NewReadReq(10, nil, nil, 0x144, 4)

		topPort.EXPECT().Peek().Return(read1)
		topPort.EXPECT().Peek().Return(read2)
		topPort.EXPECT().Retrieve(gomock.Any()).Times(2)
		c.Tick(10)
		c.Tick(12)

		dirBuf.EXPECT().CanPush().Return(true)
		dirBuf.EXPECT().Push(gomock.Any()).
			Do(func(trans *transaction) {
				Expect(trans.preCoalesceTransactions).To(HaveLen(2))
			})

		topPort.EXPECT().Peek().Return(read3)
		topPort.EXPECT().Retrieve(gomock.Any())
		madeProgress := c.Tick(13)

		Expect(madeProgress).To(BeTrue())
		Expect(transactions).To(HaveLen(3))
		Expect(c.toCoalesce).To(HaveLen(1))
	})

	It("should stall if cannot send request to directory stage", func() {
		read1 := mem.NewReadReq(10, nil, nil, 0x100, 4)
		read2 := mem.NewReadReq(10, nil, nil, 0x104, 4)
		read3 := mem.NewReadReq(10, nil, nil, 0x144, 4)

		topPort.EXPECT().Peek().Return(read1)
		topPort.EXPECT().Peek().Return(read2)
		topPort.EXPECT().Retrieve(gomock.Any()).Times(2)
		c.Tick(10)
		c.Tick(12)

		dirBuf.EXPECT().CanPush().Return(false)

		topPort.EXPECT().Peek().Return(read3)
		madeProgress := c.Tick(13)

		Expect(madeProgress).To(BeFalse())
		Expect(transactions).To(HaveLen(2))
		Expect(c.toCoalesce).To(HaveLen(2))
	})

})
