package l1v

import (
	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/util/akitaext"
	"gitlab.com/akita/util/ca"
)

var _ = Describe("Coalescer", func() {
	var (
		mockCtrl *gomock.Controller
		cache    *Cache
		topPort  *MockPort
		dirBuf   *MockBuffer
		c        coalescer
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		topPort = NewMockPort(mockCtrl)
		dirBuf = NewMockBuffer(mockCtrl)
		cache = &Cache{
			log2BlockSize: 6,
			TopPort:       topPort,
			dirBuf:        dirBuf,
		}
		cache.TickingComponent = akitaext.NewTickingComponent(
			"cache", nil, 1, cache)
		c = coalescer{cache: cache}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do nothing if no req", func() {
		topPort.EXPECT().Peek().Return(nil)
		madeProgress := c.Tick(10)
		Expect(madeProgress).To(BeFalse())
	})

	Context("read", func() {
		var (
			read1 *mem.ReadReq
			read2 *mem.ReadReq
		)

		BeforeEach(func() {
			read1 = mem.ReadReqBuilder{}.
				WithSendTime(10).
				WithAddress(0x100).
				WithPID(1).
				WithByteSize(4).
				CanWaitForCoalesce().
				Build()
			read2 = mem.ReadReqBuilder{}.
				WithSendTime(1).
				WithAddress(0x104).
				WithPID(1).
				WithByteSize(4).
				CanWaitForCoalesce().
				Build()

			topPort.EXPECT().Peek().Return(read1)
			topPort.EXPECT().Retrieve(gomock.Any())
			topPort.EXPECT().Peek().Return(read2)
			topPort.EXPECT().Retrieve(gomock.Any())
			c.Tick(10)
			c.Tick(11)
		})

		Context("not coalescable", func() {
			It("should send to dir stage", func() {
				read3 := mem.ReadReqBuilder{}.
					WithSendTime(1).
					WithAddress(0x148).
					WithPID(1).
					WithByteSize(4).
					CanWaitForCoalesce().
					Build()

				dirBuf.EXPECT().CanPush().
					Return(true)
				dirBuf.EXPECT().Push(gomock.Any()).
					Do(func(trans *transaction) {
						Expect(trans.preCoalesceTransactions).To(HaveLen(2))
					})
				topPort.EXPECT().Peek().Return(read3)
				topPort.EXPECT().Retrieve(gomock.Any())

				madeProgress := c.Tick(13)

				Expect(madeProgress).To(BeTrue())
				Expect(cache.transactions).To(HaveLen(3))
				Expect(c.toCoalesce).To(HaveLen(1))
				Expect(cache.postCoalesceTransactions).To(HaveLen(1))
			})

			It("should stall if cannot send to dir", func() {
				read3 := mem.ReadReqBuilder{}.
					WithSendTime(10).
					WithAddress(0x148).
					WithPID(1).
					WithByteSize(4).
					Build()

				dirBuf.EXPECT().CanPush().
					Return(false)
				topPort.EXPECT().Peek().Return(read3)

				madeProgress := c.Tick(13)

				Expect(madeProgress).To(BeFalse())
				Expect(cache.transactions).To(HaveLen(2))
				Expect(c.toCoalesce).To(HaveLen(2))
			})
		})

		Context("last in wave, coalescable", func() {
			It("should send to dir stage", func() {
				read3 := mem.ReadReqBuilder{}.
					WithSendTime(1).
					WithAddress(0x108).
					WithPID(1).
					WithByteSize(4).
					Build()

				dirBuf.EXPECT().
					CanPush().
					Return(true)
				dirBuf.EXPECT().
					Push(gomock.Any()).
					Do(func(trans *transaction) {
						Expect(trans.preCoalesceTransactions).To(HaveLen(3))
						Expect(trans.read.Address).To(Equal(uint64(0x100)))
						Expect(trans.read.PID).To(Equal(ca.PID(1)))
						Expect(trans.read.AccessByteSize).To(Equal(uint64(64)))
					})
				topPort.EXPECT().Peek().Return(read3)
				topPort.EXPECT().Retrieve(gomock.Any())

				madeProgress := c.Tick(13)

				Expect(madeProgress).To(BeTrue())
				Expect(cache.transactions).To(HaveLen(3))
				Expect(c.toCoalesce).To(HaveLen(0))
				Expect(cache.postCoalesceTransactions).To(HaveLen(1))
			})

			It("should stall if cannot send", func() {
				read3 := mem.ReadReqBuilder{}.
					WithSendTime(10).
					WithAddress(0x108).
					WithPID(1).
					WithByteSize(4).
					Build()

				dirBuf.EXPECT().CanPush().
					Return(false)
				topPort.EXPECT().Peek().Return(read3)

				madeProgress := c.Tick(13)

				Expect(madeProgress).To(BeFalse())
				Expect(cache.transactions).To(HaveLen(2))
				Expect(c.toCoalesce).To(HaveLen(2))
			})
		})

		Context("last in wave, not coalescable", func() {
			It("should send to dir stage", func() {
				read3 := mem.ReadReqBuilder{}.
					WithSendTime(10).
					WithAddress(0x148).
					WithPID(1).
					WithByteSize(4).
					Build()

				dirBuf.EXPECT().CanPush().
					Return(true).Times(2)
				dirBuf.EXPECT().Push(gomock.Any()).
					Do(func(trans *transaction) {
						Expect(trans.preCoalesceTransactions).To(HaveLen(2))
					})
				dirBuf.EXPECT().Push(gomock.Any()).
					Do(func(trans *transaction) {
						Expect(trans.preCoalesceTransactions).To(HaveLen(1))
					})

				topPort.EXPECT().Peek().Return(read3)
				topPort.EXPECT().Retrieve(gomock.Any())
				madeProgress := c.Tick(13)

				Expect(madeProgress).To(BeTrue())
				Expect(cache.transactions).To(HaveLen(3))
				Expect(c.toCoalesce).To(HaveLen(0))
				Expect(cache.postCoalesceTransactions).To(HaveLen(2))
			})

			It("should stall is cannot send to dir stage", func() {
				read3 := mem.ReadReqBuilder{}.
					WithSendTime(10).
					WithAddress(0x148).
					WithPID(1).
					WithByteSize(4).
					Build()

				dirBuf.EXPECT().CanPush().
					Return(false)

				topPort.EXPECT().Peek().Return(read3)
				madeProgress := c.Tick(13)

				Expect(madeProgress).To(BeFalse())
				Expect(cache.transactions).To(HaveLen(2))
				Expect(c.toCoalesce).To(HaveLen(2))
			})

			It("should stall if cannot send to dir stage in the second time",
				func() {
					read3 := mem.ReadReqBuilder{}.
						WithSendTime(10).
						WithAddress(0x148).
						WithPID(1).
						WithByteSize(4).
						Build()

					dirBuf.EXPECT().CanPush().Return(true)
					dirBuf.EXPECT().
						Push(gomock.Any()).
						Do(func(trans *transaction) {
							Expect(trans.preCoalesceTransactions).To(HaveLen(2))
						})
					dirBuf.EXPECT().CanPush().Return(false)
					topPort.EXPECT().Peek().Return(read3)

					madeProgress := c.Tick(13)

					Expect(madeProgress).To(BeTrue())
					Expect(cache.transactions).To(HaveLen(2))
					Expect(c.toCoalesce).To(HaveLen(0))
					Expect(cache.postCoalesceTransactions).To(HaveLen(1))
				})
		})
	})

	Context("write", func() {
		It("should coalesce write", func() {
			write1 := mem.WriteReqBuilder{}.
				WithSendTime(10).
				WithAddress(0x104).
				WithPID(1).
				WithData([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 9, 9, 9}).
				WithDirtyMask([]bool{
					true, true, true, true,
					false, false, false, false,
					true, true, true, true,
				}).
				CanWaitForCoalesce().
				Build()

			write2 := mem.WriteReqBuilder{}.
				WithSendTime(10).
				WithAddress(0x108).
				WithPID(1).
				WithData([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 9, 9, 9}).
				WithDirtyMask([]bool{
					true, true, true, true,
					true, true, true, true,
					false, false, false, false,
				}).
				Build()

			topPort.EXPECT().Peek().Return(write1)
			topPort.EXPECT().Peek().Return(write2)
			topPort.EXPECT().Retrieve(gomock.Any()).Times(2)
			dirBuf.EXPECT().CanPush().Return(true)
			dirBuf.EXPECT().Push(gomock.Any()).Do(func(trans *transaction) {
				Expect(trans.write.Address).To(Equal(uint64(0x100)))
				Expect(trans.write.PID).To(Equal(ca.PID(1)))
				Expect(trans.write.Data).To(Equal([]byte{
					0, 0, 0, 0,
					1, 2, 3, 4,
					1, 2, 3, 4,
					5, 6, 7, 8,
					0, 0, 0, 0, 0, 0, 0, 0,
					0, 0, 0, 0, 0, 0, 0, 0,
					0, 0, 0, 0, 0, 0, 0, 0,
					0, 0, 0, 0, 0, 0, 0, 0,
					0, 0, 0, 0, 0, 0, 0, 0,
					0, 0, 0, 0, 0, 0, 0, 0,
				}))
				Expect(trans.write.DirtyMask).To(Equal([]bool{
					false, false, false, false, true, true, true, true,
					true, true, true, true, true, true, true, true,
					false, false, false, false, false, false, false, false,
					false, false, false, false, false, false, false, false,
					false, false, false, false, false, false, false, false,
					false, false, false, false, false, false, false, false,
					false, false, false, false, false, false, false, false,
					false, false, false, false, false, false, false, false,
				}))
			})

			madeProgress := c.Tick(10)
			Expect(madeProgress).To(BeTrue())

			madeProgress = c.Tick(11)
			Expect(madeProgress).To(BeTrue())

			Expect(cache.postCoalesceTransactions).To(HaveLen(1))
		})
	})
})
