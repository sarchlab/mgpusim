package gcn3

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/akita"
	"gitlab.com/akita/akita/mock_akita"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
)

var _ = Describe("DMAEngine", func() {
	var (
		mockCtrl          *gomock.Controller
		engine            *mock_akita.MockEngine
		toCP              *mock_akita.MockPort
		toMem             *mock_akita.MockPort
		localModuleFinder *cache.SingleLowModuleFinder
		dmaEngine         *DMAEngine
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = mock_akita.NewMockEngine(mockCtrl)
		toCP = mock_akita.NewMockPort(mockCtrl)
		toMem = mock_akita.NewMockPort(mockCtrl)

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

	//Context("when copy memory from host to device", func() {
	//	It("should only process one req", func() {
	//		dmaEngine.processingReq = akita.NewReqBase()
	//
	//		buf := make([]byte, 128)
	//		req := NewMemCopyH2DReq(10, nil, dmaEngine.ToCP, buf, 1024)
	//		req.SetRecvTime(10)
	//		dmaEngine.ToCP.Recv(req)
	//
	//		dmaEngine.acceptNewReq(10)
	//
	//		Expect(dmaEngine.processingReq).NotTo(BeIdenticalTo(req))
	//	})
	//
	//	It("should process MemCopyH2DReq", func() {
	//		dmaEngine.progressOffset = 1024
	//
	//		buf := make([]byte, 128)
	//		req := NewMemCopyH2DReq(10, nil, dmaEngine.ToCP, buf, 1024)
	//		req.SetRecvTime(10)
	//		dmaEngine.ToCP.Recv(req)
	//
	//		dmaEngine.acceptNewReq(10)
	//
	//		Expect(engine.ScheduledEvent).To(HaveLen(1))
	//		Expect(dmaEngine.processingReq).To(BeIdenticalTo(req))
	//		Expect(dmaEngine.progressOffset).To(Equal(uint64(0)))
	//	})
	//
	//	It("should send WriteReq", func() {
	//		buf := make([]byte, 132)
	//		req := NewMemCopyH2DReq(10, nil, dmaEngine.ToCP, buf, 1024)
	//		req.SetRecvTime(10)
	//		dmaEngine.processingReq = req
	//
	//		expectWriteReq := mem.NewWriteReq(10, dmaEngine.ToMem, nil, 1024)
	//		expectWriteReq.Data = []byte{
	//			0, 0, 0, 0, 0, 0, 0, 0,
	//			0, 0, 0, 0, 0, 0, 0, 0,
	//			0, 0, 0, 0, 0, 0, 0, 0,
	//			0, 0, 0, 0, 0, 0, 0, 0,
	//			0, 0, 0, 0, 0, 0, 0, 0,
	//			0, 0, 0, 0, 0, 0, 0, 0,
	//			0, 0, 0, 0, 0, 0, 0, 0,
	//			0, 0, 0, 0, 0, 0, 0, 0,
	//		}
	//		toMemConn.ExpectSend(expectWriteReq, nil)
	//
	//		tickEvent := akita.NewTickEvent(10, dmaEngine)
	//		dmaEngine.Handle(*tickEvent)
	//
	//		Expect(toMemConn.AllExpectedSent()).To(BeTrue())
	//		Expect(dmaEngine.progressOffset).To(Equal(uint64(64)))
	//	})
	//
	//	It("should send WriteReq if memory not aligned with cache line", func() {
	//		buf := make([]byte, 128)
	//		req := NewMemCopyH2DReq(10, nil, dmaEngine.ToMem, buf, 1028)
	//		req.SetRecvTime(10)
	//		dmaEngine.processingReq = req
	//
	//		expectWriteReq := mem.NewWriteReq(10, dmaEngine.ToMem, nil, 1028)
	//		expectWriteReq.Data = []byte{
	//			0, 0, 0, 0, 0, 0, 0, 0,
	//			0, 0, 0, 0, 0, 0, 0, 0,
	//			0, 0, 0, 0, 0, 0, 0, 0,
	//			0, 0, 0, 0, 0, 0, 0, 0,
	//			0, 0, 0, 0, 0, 0, 0, 0,
	//			0, 0, 0, 0, 0, 0, 0, 0,
	//			0, 0, 0, 0, 0, 0, 0, 0,
	//			0, 0, 0, 0,
	//		}
	//		toMemConn.ExpectSend(expectWriteReq, nil)
	//
	//		tickEvent := akita.NewTickEvent(10, dmaEngine)
	//		dmaEngine.Handle(*tickEvent)
	//
	//		Expect(toMemConn.AllExpectedSent()).To(BeTrue())
	//		Expect(dmaEngine.progressOffset).To(Equal(uint64(60)))
	//	})
	//
	//	It("should not make progress if failed", func() {
	//		buf := make([]byte, 132)
	//		req := NewMemCopyH2DReq(10, nil, dmaEngine.ToMem, buf, 1024)
	//		req.SetRecvTime(10)
	//		dmaEngine.processingReq = req
	//
	//		expectWriteReq := mem.NewWriteReq(10, dmaEngine.ToMem, nil, 1024)
	//		expectWriteReq.Data = []byte{
	//			0, 0, 0, 0, 0, 0, 0, 0,
	//			0, 0, 0, 0, 0, 0, 0, 0,
	//			0, 0, 0, 0, 0, 0, 0, 0,
	//			0, 0, 0, 0, 0, 0, 0, 0,
	//			0, 0, 0, 0, 0, 0, 0, 0,
	//			0, 0, 0, 0, 0, 0, 0, 0,
	//			0, 0, 0, 0, 0, 0, 0, 0,
	//			0, 0, 0, 0, 0, 0, 0, 0,
	//		}
	//		toMemConn.ExpectSend(expectWriteReq,
	//			akita.NewSendError())
	//
	//		tickEvent := akita.NewTickEvent(10, dmaEngine)
	//		dmaEngine.Handle(*tickEvent)
	//
	//		Expect(toMemConn.AllExpectedSent()).To(BeTrue())
	//		Expect(dmaEngine.progressOffset).To(Equal(uint64(0)))
	//	})
	//
	//	It("should send WriteReq for last piece", func() {
	//		buf := make([]byte, 132)
	//		req := NewMemCopyH2DReq(10, nil, dmaEngine.ToMem, buf, 1024)
	//		req.SetRecvTime(10)
	//		dmaEngine.processingReq = req
	//		dmaEngine.progressOffset = 128
	//
	//		expectWriteReq := mem.NewWriteReq(10, dmaEngine.ToMem, nil, 1152)
	//		expectWriteReq.Data = []byte{0, 0, 0, 0}
	//		toMemConn.ExpectSend(expectWriteReq, nil)
	//
	//		tickEvent := akita.NewTickEvent(10, dmaEngine)
	//		dmaEngine.Handle(*tickEvent)
	//
	//		Expect(toMemConn.AllExpectedSent()).To(BeTrue())
	//		Expect(dmaEngine.progressOffset).To(Equal(uint64(132)))
	//	})
	//
	//	It("should reply MemCopyH2DReq when copy completed", func() {
	//		buf := make([]byte, 132)
	//		req := NewMemCopyH2DReq(10, nil, dmaEngine.ToCP, buf, 1024)
	//		req.SetRecvTime(10)
	//		dmaEngine.processingReq = req
	//		dmaEngine.progressOffset = 132
	//
	//		toCommandProcessorConn.ExpectSend(req, nil)
	//
	//		tickEvent := akita.NewTickEvent(12, dmaEngine)
	//		dmaEngine.Handle(*tickEvent)
	//
	//		Expect(toCommandProcessorConn.AllExpectedSent()).To(BeTrue())
	//		Expect(req.Src()).To(BeIdenticalTo(dmaEngine.ToCP))
	//		Expect(req.SendTime()).To(Equal(akita.VTimeInSec(12)))
	//		Expect(dmaEngine.processingReq).To(BeNil())
	//	})
	//
	//	It("should retry if sending MemCopyH2DReq failed", func() {
	//		buf := make([]byte, 132)
	//		req := NewMemCopyH2DReq(10, nil, dmaEngine.ToCP, buf, 1024)
	//		req.SetRecvTime(10)
	//		dmaEngine.processingReq = req
	//		dmaEngine.progressOffset = 132
	//
	//		toCommandProcessorConn.ExpectSend(req,
	//			akita.NewSendError())
	//
	//		tickEvent := akita.NewTickEvent(12, dmaEngine)
	//		dmaEngine.Handle(*tickEvent)
	//
	//		Expect(toCommandProcessorConn.AllExpectedSent()).To(BeTrue())
	//		Expect(req.Src()).To(BeIdenticalTo(dmaEngine.ToCP))
	//		Expect(req.SendTime()).To(Equal(akita.VTimeInSec(12)))
	//		Expect(dmaEngine.processingReq).NotTo(BeNil())
	//	})
	//})
	//
	//Context("when copy memory from device to host", func() {
	//
	//	It("should process MemCopyD2HReq", func() {
	//		//dmaEngine.progressOffset = 1024
	//
	//		buf := make([]byte, 128)
	//		req := NewMemCopyD2HReq(10, nil, dmaEngine.ToCP, 1024, buf)
	//		req.SetRecvTime(10)
	//		dmaEngine.ToCP.Recv(req)
	//
	//		tick := akita.NewTickEvent(10, dmaEngine)
	//		dmaEngine.Handle(*tick)
	//
	//		Expect(engine.ScheduledEvent).To(HaveLen(1))
	//		Expect(dmaEngine.processingReq).To(BeIdenticalTo(req))
	//		Expect(dmaEngine.progressOffset).To(Equal(uint64(0)))
	//	})
	//
	//	It("should send ReadReq", func() {
	//		buf := make([]byte, 132)
	//		req := NewMemCopyD2HReq(10, nil, dmaEngine.ToMem, 1024, buf)
	//		req.SetRecvTime(10)
	//		dmaEngine.processingReq = req
	//
	//		expectReadReq := mem.NewReadReq(10, dmaEngine.ToMem, nil, 1024, 64)
	//		toMemConn.ExpectSend(expectReadReq, nil)
	//
	//		tickEvent := akita.NewTickEvent(10, dmaEngine)
	//		dmaEngine.Handle(*tickEvent)
	//
	//		Expect(toMemConn.AllExpectedSent()).To(BeTrue())
	//		Expect(dmaEngine.progressOffset).To(Equal(uint64(64)))
	//	})
	//
	//	It("should send ReadReq if memory not aligned with cache line", func() {
	//		buf := make([]byte, 128)
	//		req := NewMemCopyD2HReq(10, nil, dmaEngine.ToMem, 1028, buf)
	//		req.SetRecvTime(10)
	//		dmaEngine.processingReq = req
	//
	//		expectReadReq := mem.NewReadReq(10, dmaEngine.ToMem, nil, 1028, 60)
	//		toMemConn.ExpectSend(expectReadReq, nil)
	//
	//		tickEvent := akita.NewTickEvent(10, dmaEngine)
	//		dmaEngine.Handle(*tickEvent)
	//
	//		Expect(toMemConn.AllExpectedSent()).To(BeTrue())
	//		Expect(dmaEngine.progressOffset).To(Equal(uint64(60)))
	//	})
	//
	//	It("should not make progress if failed", func() {
	//		buf := make([]byte, 132)
	//		req := NewMemCopyD2HReq(10, nil, dmaEngine.ToMem, 1024, buf)
	//		req.SetRecvTime(10)
	//		dmaEngine.processingReq = req
	//
	//		expectReadReq := mem.NewReadReq(10, dmaEngine.ToMem, nil, 1024, 64)
	//		toMemConn.ExpectSend(expectReadReq, akita.NewSendError())
	//
	//		tickEvent := akita.NewTickEvent(10, dmaEngine)
	//		dmaEngine.Handle(*tickEvent)
	//
	//		Expect(toMemConn.AllExpectedSent()).To(BeTrue())
	//		Expect(dmaEngine.progressOffset).To(Equal(uint64(0)))
	//	})
	//
	//	It("should send ReadReq for last piece", func() {
	//		buf := make([]byte, 132)
	//		req := NewMemCopyD2HReq(10, nil, dmaEngine.ToMem, 1024, buf)
	//		req.SetRecvTime(10)
	//		dmaEngine.processingReq = req
	//		dmaEngine.progressOffset = 128
	//
	//		expectReadReq := mem.NewReadReq(10, dmaEngine.ToMem, nil, 1152, 4)
	//		toMemConn.ExpectSend(expectReadReq, nil)
	//
	//		tickEvent := akita.NewTickEvent(10, dmaEngine)
	//		dmaEngine.Handle(*tickEvent)
	//
	//		Expect(toMemConn.AllExpectedSent()).To(BeTrue())
	//		Expect(dmaEngine.progressOffset).To(Equal(uint64(132)))
	//	})
	//
	//	It("should reply MemCopyD2HReq when copy completed", func() {
	//		buf := make([]byte, 132)
	//		req := NewMemCopyD2HReq(10, nil, dmaEngine.ToCP, 1024, buf)
	//		req.SetRecvTime(10)
	//		dmaEngine.processingReq = req
	//		dmaEngine.progressOffset = 132
	//
	//		toCommandProcessorConn.ExpectSend(req, nil)
	//
	//		tickEvent := akita.NewTickEvent(12, dmaEngine)
	//		dmaEngine.Handle(*tickEvent)
	//
	//		Expect(toCommandProcessorConn.AllExpectedSent()).To(BeTrue())
	//		Expect(req.Src()).To(BeIdenticalTo(dmaEngine.ToCP))
	//		Expect(req.SendTime()).To(Equal(akita.VTimeInSec(12)))
	//		Expect(dmaEngine.processingReq).To(BeNil())
	//	})
	//
	//	It("should retry if sending MemCopyD2HReq failed", func() {
	//		buf := make([]byte, 132)
	//		req := NewMemCopyD2HReq(10, nil, dmaEngine.ToCP, 1024, buf)
	//		req.SetRecvTime(10)
	//		dmaEngine.processingReq = req
	//		dmaEngine.progressOffset = 132
	//
	//		toCommandProcessorConn.ExpectSend(req, akita.NewSendError())
	//
	//		tickEvent := akita.NewTickEvent(12, dmaEngine)
	//		dmaEngine.Handle(*tickEvent)
	//
	//		Expect(toCommandProcessorConn.AllExpectedSent()).To(BeTrue())
	//		Expect(req.Src()).To(BeIdenticalTo(dmaEngine.ToCP))
	//		Expect(req.SendTime()).To(Equal(akita.VTimeInSec(12)))
	//		Expect(dmaEngine.processingReq).NotTo(BeNil())
	//	})
	//})
})
