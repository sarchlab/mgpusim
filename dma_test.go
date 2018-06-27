package gcn3

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/mem"
	"gitlab.com/yaotsu/mem/cache"
)

var _ = Describe("DMAEngine", func() {
	var (
		engine                 *core.MockEngine
		localModuleFinder      *cache.SingleLowModuleFinder
		dmaEngine              *DMAEngine
		toMemConn              *core.MockConnection
		toCommandProcessorConn *core.MockConnection
	)

	BeforeEach(func() {
		engine = core.NewMockEngine()
		localModuleFinder = new(cache.SingleLowModuleFinder)
		dmaEngine = NewDMAEngine("dma", engine, localModuleFinder)

		toMemConn = core.NewMockConnection()
		core.PlugIn(dmaEngine, "ToMem", toMemConn)

		toCommandProcessorConn = core.NewMockConnection()
		core.PlugIn(dmaEngine, "ToCommandProcessor", toCommandProcessorConn)
	})

	Context("when copy memory from host to device", func() {
		It("should only process one req", func() {
			dmaEngine.processingReq = core.NewReqBase()

			buf := make([]byte, 128)
			req := NewMemCopyH2DReq(10, nil, dmaEngine, buf, 1024)
			req.SetRecvTime(10)

			err := dmaEngine.Recv(req)

			Expect(err).NotTo(BeNil())
		})

		It("should process MemCopyH2DReq", func() {
			dmaEngine.progressOffset = 1024

			buf := make([]byte, 128)
			req := NewMemCopyH2DReq(10, nil, dmaEngine, buf, 1024)
			req.SetRecvTime(10)

			dmaEngine.Recv(req)

			Expect(engine.ScheduledEvent).To(HaveLen(1))
			Expect(dmaEngine.processingReq).To(BeIdenticalTo(req))
			Expect(dmaEngine.progressOffset).To(Equal(uint64(0)))
		})

		It("should send WriteReq", func() {
			buf := make([]byte, 132)
			req := NewMemCopyH2DReq(10, nil, dmaEngine, buf, 1024)
			req.SetRecvTime(10)
			dmaEngine.processingReq = req

			expectWriteReq := mem.NewWriteReq(10, dmaEngine, nil, 1024)
			expectWriteReq.Data = []byte{
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
			}
			toMemConn.ExpectSend(expectWriteReq, nil)

			tickEvent := core.NewTickEvent(10, dmaEngine)
			dmaEngine.Handle(tickEvent)

			Expect(toMemConn.AllExpectedSent()).To(BeTrue())
			Expect(dmaEngine.progressOffset).To(Equal(uint64(64)))
		})

		It("should send WriteReq if memory not aligned with cache line", func() {
			buf := make([]byte, 128)
			req := NewMemCopyH2DReq(10, nil, dmaEngine, buf, 1028)
			req.SetRecvTime(10)
			dmaEngine.processingReq = req

			expectWriteReq := mem.NewWriteReq(10, dmaEngine, nil, 1028)
			expectWriteReq.Data = []byte{
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0,
			}
			toMemConn.ExpectSend(expectWriteReq, nil)

			tickEvent := core.NewTickEvent(10, dmaEngine)
			dmaEngine.Handle(tickEvent)

			Expect(toMemConn.AllExpectedSent()).To(BeTrue())
			Expect(dmaEngine.progressOffset).To(Equal(uint64(60)))
		})

		It("should not make progress if failed", func() {
			buf := make([]byte, 132)
			req := NewMemCopyH2DReq(10, nil, dmaEngine, buf, 1024)
			req.SetRecvTime(10)
			dmaEngine.processingReq = req

			expectWriteReq := mem.NewWriteReq(10, dmaEngine, nil, 1024)
			expectWriteReq.Data = []byte{
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
				0, 0, 0, 0, 0, 0, 0, 0,
			}
			toMemConn.ExpectSend(expectWriteReq,
				core.NewError("Busy", true, 12))

			tickEvent := core.NewTickEvent(10, dmaEngine)
			dmaEngine.Handle(tickEvent)

			Expect(toMemConn.AllExpectedSent()).To(BeTrue())
			Expect(dmaEngine.progressOffset).To(Equal(uint64(0)))
			Expect(engine.ScheduledEvent).To(HaveLen(1))
		})

		It("should send WriteReq for last piece", func() {
			buf := make([]byte, 132)
			req := NewMemCopyH2DReq(10, nil, dmaEngine, buf, 1024)
			req.SetRecvTime(10)
			dmaEngine.processingReq = req
			dmaEngine.progressOffset = 128

			expectWriteReq := mem.NewWriteReq(10, dmaEngine, nil, 1152)
			expectWriteReq.Data = []byte{0, 0, 0, 0}
			toMemConn.ExpectSend(expectWriteReq, nil)

			tickEvent := core.NewTickEvent(10, dmaEngine)
			dmaEngine.Handle(tickEvent)

			Expect(toMemConn.AllExpectedSent()).To(BeTrue())
			Expect(dmaEngine.progressOffset).To(Equal(uint64(132)))
		})

		It("should reply MemCopyH2DReq when copy completed", func() {
			buf := make([]byte, 132)
			req := NewMemCopyH2DReq(10, nil, dmaEngine, buf, 1024)
			req.SetRecvTime(10)
			dmaEngine.processingReq = req
			dmaEngine.progressOffset = 132

			toCommandProcessorConn.ExpectSend(req, nil)

			tickEvent := core.NewTickEvent(12, dmaEngine)
			dmaEngine.Handle(tickEvent)

			Expect(toCommandProcessorConn.AllExpectedSent()).To(BeTrue())
			Expect(req.Src()).To(BeIdenticalTo(dmaEngine))
			Expect(req.SendTime()).To(Equal(core.VTimeInSec(12)))
			Expect(dmaEngine.processingReq).To(BeNil())
		})

		It("should retry if sending MemCopyH2DReq failed", func() {
			buf := make([]byte, 132)
			req := NewMemCopyH2DReq(10, nil, dmaEngine, buf, 1024)
			req.SetRecvTime(10)
			dmaEngine.processingReq = req
			dmaEngine.progressOffset = 132

			toCommandProcessorConn.ExpectSend(req,
				core.NewError("Busy", true, 13))

			tickEvent := core.NewTickEvent(12, dmaEngine)
			dmaEngine.Handle(tickEvent)

			Expect(toCommandProcessorConn.AllExpectedSent()).To(BeTrue())
			Expect(req.Src()).To(BeIdenticalTo(dmaEngine))
			Expect(req.SendTime()).To(Equal(core.VTimeInSec(12)))
			Expect(dmaEngine.processingReq).NotTo(BeNil())
			Expect(engine.ScheduledEvent).To(HaveLen(1))
		})

		It("should schedule next tick event after receiving DoneRsp", func() {
			doneRsp := mem.NewDoneRsp(10, nil, dmaEngine, "")

			dmaEngine.Recv(doneRsp)

			Expect(engine.ScheduledEvent).To(HaveLen(1))
		})
	})

	Context("when copy memory from device to host", func() {
		It("should only process one req", func() {
			dmaEngine.processingReq = core.NewReqBase()

			buf := make([]byte, 128)
			req := NewMemCopyD2HReq(10, nil, dmaEngine, 1024, buf)
			req.SetRecvTime(10)

			err := dmaEngine.Recv(req)

			Expect(err).NotTo(BeNil())
		})

		It("should process MemCopyD2HReq", func() {
			dmaEngine.progressOffset = 1024

			buf := make([]byte, 128)
			req := NewMemCopyD2HReq(10, nil, dmaEngine, 1024, buf)
			req.SetRecvTime(10)

			dmaEngine.Recv(req)

			Expect(engine.ScheduledEvent).To(HaveLen(1))
			Expect(dmaEngine.processingReq).To(BeIdenticalTo(req))
			Expect(dmaEngine.progressOffset).To(Equal(uint64(0)))
		})

		It("should send ReadReq", func() {
			buf := make([]byte, 132)
			req := NewMemCopyD2HReq(10, nil, dmaEngine, 1024, buf)
			req.SetRecvTime(10)
			dmaEngine.processingReq = req

			expectReadReq := mem.NewReadReq(10, dmaEngine, nil, 1024, 64)
			toMemConn.ExpectSend(expectReadReq, nil)

			tickEvent := core.NewTickEvent(10, dmaEngine)
			dmaEngine.Handle(tickEvent)

			Expect(toMemConn.AllExpectedSent()).To(BeTrue())
			Expect(dmaEngine.progressOffset).To(Equal(uint64(64)))
		})

		It("should send ReadReq if memory not aligned with cache line", func() {
			buf := make([]byte, 128)
			req := NewMemCopyD2HReq(10, nil, dmaEngine, 1028, buf)
			req.SetRecvTime(10)
			dmaEngine.processingReq = req

			expectReadReq := mem.NewReadReq(10, dmaEngine, nil, 1028, 60)
			toMemConn.ExpectSend(expectReadReq, nil)

			tickEvent := core.NewTickEvent(10, dmaEngine)
			dmaEngine.Handle(tickEvent)

			Expect(toMemConn.AllExpectedSent()).To(BeTrue())
			Expect(dmaEngine.progressOffset).To(Equal(uint64(60)))
		})

		It("should not make progress if failed", func() {
			buf := make([]byte, 132)
			req := NewMemCopyD2HReq(10, nil, dmaEngine, 1024, buf)
			req.SetRecvTime(10)
			dmaEngine.processingReq = req

			expectReadReq := mem.NewReadReq(10, dmaEngine, nil, 1024, 64)
			toMemConn.ExpectSend(expectReadReq,
				core.NewError("Busy", true, 12))

			tickEvent := core.NewTickEvent(10, dmaEngine)
			dmaEngine.Handle(tickEvent)

			Expect(toMemConn.AllExpectedSent()).To(BeTrue())
			Expect(dmaEngine.progressOffset).To(Equal(uint64(0)))
			Expect(engine.ScheduledEvent).To(HaveLen(1))
		})

		It("should send ReadReq for last piece", func() {
			buf := make([]byte, 132)
			req := NewMemCopyD2HReq(10, nil, dmaEngine, 1024, buf)
			req.SetRecvTime(10)
			dmaEngine.processingReq = req
			dmaEngine.progressOffset = 128

			expectReadReq := mem.NewReadReq(10, dmaEngine, nil, 1152, 4)
			toMemConn.ExpectSend(expectReadReq, nil)

			tickEvent := core.NewTickEvent(10, dmaEngine)
			dmaEngine.Handle(tickEvent)

			Expect(toMemConn.AllExpectedSent()).To(BeTrue())
			Expect(dmaEngine.progressOffset).To(Equal(uint64(132)))
		})

		It("should reply MemCopyD2HReq when copy completed", func() {
			buf := make([]byte, 132)
			req := NewMemCopyD2HReq(10, nil, dmaEngine, 1024, buf)
			req.SetRecvTime(10)
			dmaEngine.processingReq = req
			dmaEngine.progressOffset = 132

			toCommandProcessorConn.ExpectSend(req, nil)

			tickEvent := core.NewTickEvent(12, dmaEngine)
			dmaEngine.Handle(tickEvent)

			Expect(toCommandProcessorConn.AllExpectedSent()).To(BeTrue())
			Expect(req.Src()).To(BeIdenticalTo(dmaEngine))
			Expect(req.SendTime()).To(Equal(core.VTimeInSec(12)))
			Expect(dmaEngine.processingReq).To(BeNil())
		})

		It("should retry if sending MemCopyD2HReq failed", func() {
			buf := make([]byte, 132)
			req := NewMemCopyD2HReq(10, nil, dmaEngine, 1024, buf)
			req.SetRecvTime(10)
			dmaEngine.processingReq = req
			dmaEngine.progressOffset = 132

			toCommandProcessorConn.ExpectSend(req,
				core.NewError("Busy", true, 13))

			tickEvent := core.NewTickEvent(12, dmaEngine)
			dmaEngine.Handle(tickEvent)

			Expect(toCommandProcessorConn.AllExpectedSent()).To(BeTrue())
			Expect(req.Src()).To(BeIdenticalTo(dmaEngine))
			Expect(req.SendTime()).To(Equal(core.VTimeInSec(12)))
			Expect(dmaEngine.processingReq).NotTo(BeNil())
			Expect(engine.ScheduledEvent).To(HaveLen(1))
		})

		It("should schedule next tick event after receiving DataReadyRsp", func() {
			buf := make([]byte, 132)
			req := NewMemCopyD2HReq(10, nil, dmaEngine, 1024, buf)
			req.SetRecvTime(10)
			dmaEngine.processingReq = req
			dmaEngine.progressOffset = 132

			dataReadyRsp := mem.NewDataReadyRsp(10, nil, dmaEngine, "")
			dataReadyRsp.Data = []byte{1, 2, 3, 4}

			dmaEngine.Recv(dataReadyRsp)

			Expect(engine.ScheduledEvent).To(HaveLen(1))
			Expect(buf[128:132]).To(Equal([]byte{1, 2, 3, 4}))
		})
	})
})
