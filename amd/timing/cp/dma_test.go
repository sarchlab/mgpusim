package cp

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/mgpusim/v4/amd/protocol"
	"go.uber.org/mock/gomock"
)

var _ = Describe("DMAEngine", func() {
	var (
		mockCtrl          *gomock.Controller
		engine            *MockEngine
		toCP              *MockPort
		toMem             *MockPort
		localModuleFinder *mem.SinglePortMapper
		dmaEngine         *DMAEngine
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = NewMockEngine(mockCtrl)
		toCP = NewMockPort(mockCtrl)
		toMem = NewMockPort(mockCtrl)

		toCP.EXPECT().AsRemote().AnyTimes()
		toMem.EXPECT().AsRemote().AnyTimes()

		localModuleFinder = new(mem.SinglePortMapper)
		dmaEngine = NewDMAEngine("DMA", engine, localModuleFinder)
		dmaEngine.ToCP = toCP
		dmaEngine.ToMem = toMem
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should stall if dma is processing max request number", func() {
		nilPort := NewMockPort(mockCtrl)
		nilPort.EXPECT().AsRemote().AnyTimes()

		for i := 0; i < int(dmaEngine.maxRequestCount); i++ {
			srcBuf := make([]byte, 128)
			req := protocol.NewMemCopyH2DReq(nilPort, toCP, srcBuf, uint64(20+128*i))
			rqC := NewRequestCollection(req)

			dmaEngine.processingReqs = append(dmaEngine.processingReqs, rqC)
		}

		madeProgress := dmaEngine.parseFromCP()

		Expect(dmaEngine.toSendToMem).To(HaveLen(0))
		Expect(madeProgress).To(BeFalse())
	})

	It("should parse MemCopyH2D from CP", func() {
		nilPort := NewMockPort(mockCtrl)
		nilPort.EXPECT().AsRemote().AnyTimes()

		srcBuf := make([]byte, 128)
		req := protocol.NewMemCopyH2DReq(nilPort, toCP, srcBuf, 20)

		toCP.EXPECT().RetrieveIncoming().Return(req)

		madeProgress := dmaEngine.parseFromCP()

		Expect(dmaEngine.processingReqs[0].superiorRequest).To(BeIdenticalTo(req))
		Expect(dmaEngine.toSendToMem).To(HaveLen(3))
		Expect(dmaEngine.toSendToMem[0].(*mem.WriteReq).Address).
			To(Equal(uint64(20)))
		Expect(dmaEngine.toSendToMem[1].(*mem.WriteReq).Address).
			To(Equal(uint64(64)))
		Expect(dmaEngine.toSendToMem[2].(*mem.WriteReq).Address).
			To(Equal(uint64(128)))
		Expect(madeProgress).To(BeTrue())
		Expect(dmaEngine.pendingReqs).To(HaveLen(3))
	})

	It("should parse MemCopyD2H from CP", func() {
		nilPort := NewMockPort(mockCtrl)
		nilPort.EXPECT().AsRemote().AnyTimes()

		dstBuf := make([]byte, 128)
		req := protocol.NewMemCopyD2HReq(nilPort, toCP, 20, dstBuf)

		toCP.EXPECT().RetrieveIncoming().Return(req)

		madeProgress := dmaEngine.parseFromCP()

		Expect(dmaEngine.processingReqs[0].superiorRequest).To(BeIdenticalTo(req))
		Expect(dmaEngine.toSendToMem).To(HaveLen(3))
		Expect(dmaEngine.toSendToMem[0].(*mem.ReadReq).Address).
			To(Equal(uint64(20)))
		Expect(dmaEngine.toSendToMem[1].(*mem.ReadReq).Address).
			To(Equal(uint64(64)))
		Expect(dmaEngine.toSendToMem[2].(*mem.ReadReq).Address).
			To(Equal(uint64(128)))
		Expect(madeProgress).To(BeTrue())
		Expect(dmaEngine.pendingReqs).To(HaveLen(3))
	})

	It("should parse DataReady from mem", func() {
		nilPort := NewMockPort(mockCtrl)
		nilPort.EXPECT().AsRemote().AnyTimes()

		dstBuf := make([]byte, 128)
		req := protocol.NewMemCopyD2HReq(nilPort, toCP, 20, dstBuf)
		rqC := NewRequestCollection(req)
		dmaEngine.processingReqs = append(dmaEngine.processingReqs, rqC)

		reqToBottom1 := &mem.ReadReq{Address: 20, AccessByteSize: 64}
		reqToBottom1.ID = sim.GetIDGenerator().Generate()
		reqToBottom1.Src = toMem.AsRemote()

		reqToBottom2 := &mem.ReadReq{Address: 64, AccessByteSize: 64}
		reqToBottom2.ID = sim.GetIDGenerator().Generate()
		reqToBottom2.Src = toMem.AsRemote()

		reqToBottom3 := &mem.ReadReq{Address: 128, AccessByteSize: 64}
		reqToBottom3.ID = sim.GetIDGenerator().Generate()
		reqToBottom3.Src = toMem.AsRemote()

		dmaEngine.pendingReqs = append(dmaEngine.pendingReqs, reqToBottom1)
		rqC.appendSubordinateID(reqToBottom1.Meta().ID)
		dmaEngine.pendingReqs = append(dmaEngine.pendingReqs, reqToBottom2)
		rqC.appendSubordinateID(reqToBottom2.Meta().ID)
		dmaEngine.pendingReqs = append(dmaEngine.pendingReqs, reqToBottom3)
		rqC.appendSubordinateID(reqToBottom3.Meta().ID)

		dataReady := &mem.DataReadyRsp{
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
		dataReady.ID = sim.GetIDGenerator().Generate()
		dataReady.Dst = toMem.AsRemote()
		dataReady.RspTo = reqToBottom2.ID
		toMem.EXPECT().RetrieveIncoming().Return(dataReady)

		madeProgress := dmaEngine.parseFromMem()

		Expect(madeProgress).To(BeTrue())
		Expect(dmaEngine.processingReqs[0].superiorRequest).To(BeIdenticalTo(req))
		Expect(dmaEngine.processingReqs[0]).To(BeIdenticalTo(rqC))
		Expect(dmaEngine.pendingReqs).NotTo(ContainElement(reqToBottom2))
		Expect(dmaEngine.pendingReqs).To(ContainElement(reqToBottom1))
		Expect(dmaEngine.pendingReqs).To(ContainElement(reqToBottom3))
		Expect(dstBuf[44:108]).To(Equal(dataReady.Data))
	})

	It("should respond MemCopyD2H", func() {
		nilPort := NewMockPort(mockCtrl)
		nilPort.EXPECT().AsRemote().AnyTimes()

		dstBuf := make([]byte, 128)
		req := protocol.NewMemCopyD2HReq(nilPort, toCP, 20, dstBuf)
		rqC := NewRequestCollection(req)
		dmaEngine.processingReqs = append(dmaEngine.processingReqs, rqC)

		reqToBottom2 := &mem.ReadReq{Address: 64, AccessByteSize: 64}
		reqToBottom2.ID = sim.GetIDGenerator().Generate()
		reqToBottom2.Src = toMem.AsRemote()

		dmaEngine.pendingReqs = append(dmaEngine.pendingReqs, reqToBottom2)
		rqC.appendSubordinateID(reqToBottom2.Meta().ID)

		dataReady := &mem.DataReadyRsp{
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
		dataReady.ID = sim.GetIDGenerator().Generate()
		dataReady.Dst = toMem.AsRemote()
		dataReady.RspTo = reqToBottom2.ID
		toMem.EXPECT().RetrieveIncoming().Return(dataReady)

		madeProgress := dmaEngine.parseFromMem()

		Expect(madeProgress).To(BeTrue())
		Expect(dmaEngine.processingReqs).To(BeEmpty())
		Expect(dmaEngine.pendingReqs).NotTo(ContainElement(reqToBottom2))
		Expect(dstBuf[44:108]).To(Equal(dataReady.Data))
		rspMsg := dmaEngine.toSendToCP[0].(*protocol.MemCopyD2HReq)
		Expect(rspMsg.RspTo).To(Equal(req.Meta().ID))
	})

	It("should parse Done from mem", func() {
		nilPort := NewMockPort(mockCtrl)
		nilPort.EXPECT().AsRemote().AnyTimes()

		srcBuf := make([]byte, 128)
		req := protocol.NewMemCopyH2DReq(nilPort, toCP, srcBuf, 20)
		rqC := NewRequestCollection(req)
		dmaEngine.processingReqs = append(dmaEngine.processingReqs, rqC)

		reqToBottom1 := &mem.WriteReq{Address: 20}
		reqToBottom1.ID = sim.GetIDGenerator().Generate()
		reqToBottom1.Src = toMem.AsRemote()

		reqToBottom2 := &mem.WriteReq{Address: 64}
		reqToBottom2.ID = sim.GetIDGenerator().Generate()
		reqToBottom2.Src = toMem.AsRemote()

		reqToBottom3 := &mem.WriteReq{Address: 128}
		reqToBottom3.ID = sim.GetIDGenerator().Generate()
		reqToBottom3.Src = toMem.AsRemote()

		dmaEngine.pendingReqs = append(dmaEngine.pendingReqs, reqToBottom1)
		rqC.appendSubordinateID(reqToBottom1.Meta().ID)
		dmaEngine.pendingReqs = append(dmaEngine.pendingReqs, reqToBottom2)
		rqC.appendSubordinateID(reqToBottom2.Meta().ID)
		dmaEngine.pendingReqs = append(dmaEngine.pendingReqs, reqToBottom3)
		rqC.appendSubordinateID(reqToBottom3.Meta().ID)

		done := &mem.WriteDoneRsp{}
		done.ID = sim.GetIDGenerator().Generate()
		done.Dst = toMem.AsRemote()
		done.RspTo = reqToBottom2.ID

		toMem.EXPECT().RetrieveIncoming().Return(done)

		madeProgress := dmaEngine.parseFromMem()

		Expect(madeProgress).To(BeTrue())
		Expect(dmaEngine.processingReqs[0].superiorRequest).To(BeIdenticalTo(req))
		Expect(dmaEngine.processingReqs[0]).To(BeIdenticalTo(rqC))
		Expect(dmaEngine.pendingReqs).NotTo(ContainElement(reqToBottom2))
		Expect(dmaEngine.pendingReqs).To(ContainElement(reqToBottom1))
		Expect(dmaEngine.pendingReqs).To(ContainElement(reqToBottom3))
	})

	It("should send MemCopyH2D to top", func() {
		nilPort := NewMockPort(mockCtrl)
		nilPort.EXPECT().AsRemote().AnyTimes()

		srcBuf := make([]byte, 128)
		req := protocol.NewMemCopyH2DReq(nilPort, toCP, srcBuf, 20)
		rqC := NewRequestCollection(req)
		dmaEngine.processingReqs = append(dmaEngine.processingReqs, rqC)

		reqToBottom2 := &mem.WriteReq{Address: 64}
		reqToBottom2.ID = sim.GetIDGenerator().Generate()
		reqToBottom2.Src = toMem.AsRemote()

		dmaEngine.pendingReqs = append(dmaEngine.pendingReqs, reqToBottom2)
		rqC.appendSubordinateID(reqToBottom2.Meta().ID)

		done := &mem.WriteDoneRsp{}
		done.ID = sim.GetIDGenerator().Generate()
		done.Dst = toMem.AsRemote()
		done.RspTo = reqToBottom2.ID

		toMem.EXPECT().RetrieveIncoming().Return(done)

		madeProgress := dmaEngine.parseFromMem()

		Expect(madeProgress).To(BeTrue())
		Expect(dmaEngine.processingReqs).To(BeEmpty())
		Expect(dmaEngine.pendingReqs).NotTo(ContainElement(reqToBottom2))
		rspMsg := dmaEngine.toSendToCP[0].(*protocol.MemCopyH2DReq)
		Expect(rspMsg.RspTo).To(Equal(req.Meta().ID))
	})
})
