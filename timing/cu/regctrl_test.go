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
		engine  *core.MockEngine
		regCtrl *cu.RegCtrl
	)

	BeforeEach(func() {
		engine = core.NewMockEngine()
		regCtrl = cu.NewRegCtrl("SRegFile", 8*mem.KB, engine)
	})

	Context("when processing ReadRegEvent", func() {
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
})
