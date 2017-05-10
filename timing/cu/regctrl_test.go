package cu_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/gcn3/timing/cu"
	"gitlab.com/yaotsu/mem"
)

var _ = Describe("RegCtrl", func() {

	var (
		engine     *core.MockEngine
		regCtrl    *cu.RegCtrl
		storage    *mem.Storage
		connection *core.MockConnection
	)

	BeforeEach(func() {
		engine = core.NewMockEngine()
		storage = mem.NewStorage(8 * mem.KB)
		regCtrl = cu.NewRegCtrl("SRegFile", storage, engine)
		connection = core.NewMockConnection()

		core.PlugIn(regCtrl, "ToOutside", connection)
	})

	Context("when processing request", func() {
		It("should schedule ReadRegEvent", func() {
			req := cu.NewReadRegReq(0, insts.SReg(0), 4, 0)
			regCtrl.Recv(req)

			Expect(engine.ScheduledEvent).NotTo(BeEmpty())
			evt := engine.ScheduledEvent[0].(*cu.ReadRegEvent)
			Expect(evt.Req).To(BeIdenticalTo(req))
		})

		It("should schedule WriteRegEvent", func() {
			data := []byte{0, 0, 0, 0}
			req := cu.NewWriteRegReq(0, insts.SReg(0), 0, data)
			regCtrl.Recv(req)

			Expect(engine.ScheduledEvent).NotTo(BeEmpty())
			evt := engine.ScheduledEvent[0].(*cu.WriteRegEvent)
			Expect(evt.Req).To(BeIdenticalTo(req))
		})
	})

	Context("when handling event", func() {
		It("shoud read register", func() {
			req := cu.NewReadRegReq(0, insts.SReg(0), 4, 100)
			evt := cu.NewReadRegEvent(0.1, regCtrl, req)

			storage.Write(100, insts.Uint32ToBytes(54))
			connection.ExpectSend(req, nil)

			regCtrl.Handle(evt)

			Expect(connection.AllExpectedSent()).To(BeTrue())
			Expect(insts.BytesToUint32(req.Buf)).To(Equal(uint32(54)))
			Expect(req.SendTime()).To(BeNumerically("~", 0.1, 1e-9))
		})

		It("shoud write register", func() {
			offset := 100
			req := cu.NewWriteRegReq(0, insts.SReg(0), offset,
				insts.Uint32ToBytes(54))
			evt := cu.NewWriteRegEvent(0.1, regCtrl, req)

			connection.ExpectSend(req, nil)

			regCtrl.Handle(evt)

			Expect(connection.AllExpectedSent()).To(BeTrue())
			data, _ := storage.Read(100, 4)
			Expect(insts.BytesToUint32(data)).To(Equal(uint32(54)))
			Expect(req.SendTime()).To(BeNumerically("~", 0.1, 1e-9))
		})

	})
})
