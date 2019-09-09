package gcn3

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
)

var _ = Describe("DMAEngine", func() {
	var (
		mockCtrl          *gomock.Controller
		engine            *MockEngine
		toCP              *MockPort
		toMem             *MockPort
		localModuleFinder *cache.SingleLowModuleFinder
		dmaEngine         *DMAEngine
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = NewMockEngine(mockCtrl)
		toCP = NewMockPort(mockCtrl)
		toMem = NewMockPort(mockCtrl)

		localModuleFinder = new(cache.SingleLowModuleFinder)
		dmaEngine = NewDMAEngine("dma", engine, localModuleFinder)
		dmaEngine.ToCP = toCP
		dmaEngine.ToMem = toMem
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should stall if dma is processing another request", func() {
		srcBuf := make([]byte, 128)
		req := NewMemCopyH2DReq(5, nil, toCP, srcBuf, 20)
		dmaEngine.processingReq = req

		dmaEngine.parseFromCP(6)

		Expect(dmaEngine.toSendToMem).To(HaveLen(0))
		Expect(dmaEngine.NeedTick).To(BeFalse())
	})

	It("should parse MemCopyH2D from CP", func() {
		srcBuf := make([]byte, 128)
		req := NewMemCopyH2DReq(5, nil, toCP, srcBuf, 20)

		toCP.EXPECT().Retrieve(akita.VTimeInSec(6)).Return(req)

		dmaEngine.parseFromCP(6)

		Expect(dmaEngine.processingReq).To(BeIdenticalTo(req))
		Expect(dmaEngine.toSendToMem).To(HaveLen(3))
		Expect(dmaEngine.toSendToMem[0].(*mem.WriteReq).Address).
			To(Equal(uint64(20)))
		Expect(dmaEngine.toSendToMem[1].(*mem.WriteReq).Address).
			To(Equal(uint64(64)))
		Expect(dmaEngine.toSendToMem[2].(*mem.WriteReq).Address).
			To(Equal(uint64(128)))
		Expect(dmaEngine.NeedTick).To(BeTrue())
		Expect(dmaEngine.pendingReqs).To(HaveLen(3))
	})

	It("should parse MemCopyD2H from CP", func() {
		dstBuf := make([]byte, 128)
		req := NewMemCopyD2HReq(5, nil, toCP, 20, dstBuf)

		toCP.EXPECT().Retrieve(akita.VTimeInSec(6)).Return(req)

		dmaEngine.parseFromCP(6)

		Expect(dmaEngine.processingReq).To(BeIdenticalTo(req))
		Expect(dmaEngine.toSendToMem).To(HaveLen(3))
		Expect(dmaEngine.toSendToMem[0].(*mem.ReadReq).Address).
			To(Equal(uint64(20)))
		Expect(dmaEngine.toSendToMem[1].(*mem.ReadReq).Address).
			To(Equal(uint64(64)))
		Expect(dmaEngine.toSendToMem[2].(*mem.ReadReq).Address).
			To(Equal(uint64(128)))
		Expect(dmaEngine.NeedTick).To(BeTrue())
		Expect(dmaEngine.pendingReqs).To(HaveLen(3))
	})

	It("should parse DataReady from mem", func() {
		dstBuf := make([]byte, 128)
		req := NewMemCopyD2HReq(5, nil, toCP, 20, dstBuf)
		dmaEngine.processingReq = req

		reqToBottom1 := mem.NewReadReq(6, toMem, nil, 20, 44)
		reqToBottom2 := mem.NewReadReq(6, toMem, nil, 64, 64)
		reqToBottom3 := mem.NewReadReq(6, toMem, nil, 128, 20)
		dmaEngine.pendingReqs = append(dmaEngine.pendingReqs, reqToBottom1)
		dmaEngine.pendingReqs = append(dmaEngine.pendingReqs, reqToBottom2)
		dmaEngine.pendingReqs = append(dmaEngine.pendingReqs, reqToBottom3)

		dataReady := mem.NewDataReadyRsp(7, nil, toMem, reqToBottom2.ID)
		dataReady.Data = []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		}

		toMem.EXPECT().Retrieve(akita.VTimeInSec(10)).Return(dataReady)

		dmaEngine.parseFromMem(10)

		Expect(dmaEngine.NeedTick).To(BeTrue())
		Expect(dmaEngine.processingReq).To(BeIdenticalTo(req))
		Expect(dmaEngine.pendingReqs).NotTo(ContainElement(reqToBottom2))
		Expect(dmaEngine.pendingReqs).To(ContainElement(reqToBottom1))
		Expect(dmaEngine.pendingReqs).To(ContainElement(reqToBottom3))
		Expect(dstBuf[44:108]).To(Equal(dataReady.Data))
	})

	It("should respond MemCopyD2H", func() {
		dstBuf := make([]byte, 128)
		req := NewMemCopyD2HReq(5, nil, toCP, 20, dstBuf)
		dmaEngine.processingReq = req

		reqToBottom2 := mem.NewReadReq(6, toMem, nil, 64, 64)
		dmaEngine.pendingReqs = append(dmaEngine.pendingReqs, reqToBottom2)

		dataReady := mem.NewDataReadyRsp(7, nil, toMem, reqToBottom2.ID)
		dataReady.Data = []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		}

		toMem.EXPECT().Retrieve(akita.VTimeInSec(10)).Return(dataReady)

		dmaEngine.parseFromMem(10)

		Expect(dmaEngine.NeedTick).To(BeTrue())
		Expect(dmaEngine.processingReq).To(BeNil())
		Expect(dmaEngine.pendingReqs).NotTo(ContainElement(reqToBottom2))
		Expect(dstBuf[44:108]).To(Equal(dataReady.Data))
		Expect(dmaEngine.toSendToCP).To(ContainElement(req))
	})

	It("should parse Done from mem", func() {
		srcBuf := make([]byte, 128)
		req := NewMemCopyH2DReq(5, nil, toCP, srcBuf, 20)
		dmaEngine.processingReq = req

		reqToBottom1 := mem.NewWriteReq(6, toMem, nil, 20)
		reqToBottom2 := mem.NewWriteReq(6, toMem, nil, 64)
		reqToBottom3 := mem.NewWriteReq(6, toMem, nil, 128)
		dmaEngine.pendingReqs = append(dmaEngine.pendingReqs, reqToBottom1)
		dmaEngine.pendingReqs = append(dmaEngine.pendingReqs, reqToBottom2)
		dmaEngine.pendingReqs = append(dmaEngine.pendingReqs, reqToBottom3)

		done := mem.NewDoneRsp(7, nil, toMem, reqToBottom2.ID)

		toMem.EXPECT().Retrieve(akita.VTimeInSec(10)).Return(done)

		dmaEngine.parseFromMem(10)

		Expect(dmaEngine.NeedTick).To(BeTrue())
		Expect(dmaEngine.processingReq).To(BeIdenticalTo(req))
		Expect(dmaEngine.pendingReqs).NotTo(ContainElement(reqToBottom2))
		Expect(dmaEngine.pendingReqs).To(ContainElement(reqToBottom1))
		Expect(dmaEngine.pendingReqs).To(ContainElement(reqToBottom3))
	})

	It("should send MemCopyH2D to top", func() {
		srcBuf := make([]byte, 128)
		req := NewMemCopyH2DReq(5, nil, toCP, srcBuf, 20)
		dmaEngine.processingReq = req

		reqToBottom2 := mem.NewWriteReq(6, toMem, nil, 64)
		dmaEngine.pendingReqs = append(dmaEngine.pendingReqs, reqToBottom2)

		done := mem.NewDoneRsp(7, nil, toMem, reqToBottom2.ID)

		toMem.EXPECT().Retrieve(akita.VTimeInSec(10)).Return(done)

		dmaEngine.parseFromMem(10)

		Expect(dmaEngine.NeedTick).To(BeTrue())
		Expect(dmaEngine.processingReq).To(BeNil())
		Expect(dmaEngine.pendingReqs).NotTo(ContainElement(reqToBottom2))
		Expect(dmaEngine.toSendToCP).To(ContainElement(req))
	})
})
