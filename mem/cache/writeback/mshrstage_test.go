package writeback

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/mgpusim/v3/mem/cache"
	"github.com/sarchlab/mgpusim/v3/mem/mem"
)

var _ = Describe("MSHR Stage", func() {
	var (
		mockCtrl    *gomock.Controller
		cacheModule *Cache
		ms          *mshrStage
		inBuf       *MockBuffer
		mshr        *MockMSHR
		topSender   *MockBufferedSender
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		inBuf = NewMockBuffer(mockCtrl)
		mshr = NewMockMSHR(mockCtrl)
		topSender = NewMockBufferedSender(mockCtrl)

		builder := MakeBuilder()
		cacheModule = builder.Build("Cache")
		cacheModule.mshr = mshr
		cacheModule.topSender = topSender
		cacheModule.mshrStageBuffer = inBuf
		cacheModule.inFlightTransactions = nil

		ms = &mshrStage{
			cache: cacheModule,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do nothing if there is no entry in input buffer", func() {
		inBuf.EXPECT().Pop().Return(nil)
		ret := ms.Tick(10)
		Expect(ret).To(BeFalse())
	})

	It("should stall if topSender is busy", func() {
		read := mem.ReadReqBuilder{}.
			WithSendTime(6).
			WithAddress(0x104).
			WithByteSize(4).
			Build()
		mshrEntry := &cache.MSHREntry{
			Requests: []interface{}{read},
			Data: []byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			},
		}
		inBuf.EXPECT().Pop().Return(mshrEntry)
		topSender.EXPECT().CanSend(1).Return(false)

		ret := ms.Tick(10)

		Expect(ret).To(BeFalse())
		Expect(ms.processingMSHREntry).To(BeIdenticalTo(mshrEntry))
	})

	It("should send data ready to top", func() {
		block := &cache.Block{Tag: 0x100}
		read := mem.ReadReqBuilder{}.
			WithSendTime(6).
			WithAddress(0x104).
			WithByteSize(4).
			Build()
		trans := &transaction{read: read}
		cacheModule.inFlightTransactions = append(
			cacheModule.inFlightTransactions, trans)
		mshrEntry := &cache.MSHREntry{
			Requests: []interface{}{trans},
			Block:    block,
			Data: []byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			},
		}
		inBuf.EXPECT().Pop().Return(mshrEntry)
		topSender.EXPECT().CanSend(1).Return(true)
		topSender.EXPECT().Send(gomock.Any()).
			Do(func(dr *mem.DataReadyRsp) {
				Expect(dr.Data).To(Equal([]byte{5, 6, 7, 8}))
			})

		ret := ms.Tick(10)

		Expect(ret).To(BeTrue())
		Expect(ms.processingMSHREntry).To(BeNil())
		Expect(cacheModule.inFlightTransactions).NotTo(ContainElement(trans))
	})

	It("should send write done to top", func() {
		block := &cache.Block{Tag: 0x100}
		write := mem.WriteReqBuilder{}.
			WithSendTime(6).
			WithAddress(0x104).
			WithData([]byte{9, 9, 9, 9}).
			Build()
		trans := &transaction{write: write}
		cacheModule.inFlightTransactions = append(
			cacheModule.inFlightTransactions, trans)
		mshrEntry := &cache.MSHREntry{
			PID:      1,
			Address:  0x100,
			Requests: []interface{}{trans},
			Block:    block,
			Data: []byte{
				1, 2, 3, 4, 9, 9, 9, 9,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			},
		}
		ms.processingMSHREntry = mshrEntry
		topSender.EXPECT().CanSend(1).Return(true)
		topSender.EXPECT().Send(gomock.Any()).
			Do(func(done *mem.WriteDoneRsp) {
				Expect(done.RespondTo).To(Equal(write.ID))
			})

		ret := ms.Tick(10)

		Expect(ret).To(BeTrue())
		Expect(ms.processingMSHREntry).To(BeNil())
		Expect(cacheModule.inFlightTransactions).NotTo(ContainElement(trans))
	})

	It("should discard the request if it is no longer inflight", func() {
		block := &cache.Block{Tag: 0x100}
		read := mem.ReadReqBuilder{}.
			WithSendTime(6).
			WithAddress(0x104).
			WithByteSize(4).
			Build()
		trans := &transaction{read: read}
		mshrEntry := &cache.MSHREntry{
			Requests: []interface{}{trans},
			Block:    block,
			Data: []byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			},
		}
		inBuf.EXPECT().Pop().Return(mshrEntry)
		topSender.EXPECT().CanSend(1).Return(true)

		ret := ms.Tick(10)

		Expect(ret).To(BeTrue())
		Expect(ms.processingMSHREntry).To(BeNil())
	})
})
