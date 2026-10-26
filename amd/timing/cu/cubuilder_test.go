package cu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
)

var _ = Describe("Builder", func() {
	It("should build a fully equipped compute unit", func() {
		reg := sim.MakeBuilder().Build()
		DeferCleanup(func() { Expect(reg.Terminate()).To(Succeed()) })

		comp := MakeBuilder().
			WithSimulation(reg).
			WithSpec(DefaultSpec()).
			Build("GPU.CU")

		// All five ports must be declared so external code can assign them.
		for _, portName := range []string{
			DispatchPortName, CtrlPortName,
			InstMemPortName, ScalarMemPortName, VectorMemPortName,
		} {
			p := twowaybuffered.MakeBuilder().
				WithSimulation(reg).
				WithSpec(twowaybuffered.Spec{IncomingCapacity: 4, OutgoingCapacity: 4}).
				Build()
			Expect(func() { comp.BindPort(portName, p) }).NotTo(Panic())
		}

		reg.Initialize()
		cuMW := MiddlewareOf(comp)

		Expect(cuMW.Scheduler).NotTo(BeNil())
		Expect(cuMW.BranchUnit).NotTo(BeNil())
		Expect(cuMW.ScalarUnit).NotTo(BeNil())
		Expect(cuMW.ScalarDecoder).NotTo(BeNil())
		Expect(cuMW.VectorDecoder).NotTo(BeNil())
		Expect(cuMW.LDSDecoder).NotTo(BeNil())
		Expect(cuMW.LDSUnit).NotTo(BeNil())
		Expect(cuMW.VectorMemDecoder).NotTo(BeNil())
		Expect(cuMW.VectorMemUnit).NotTo(BeNil())
		Expect(cuMW.SIMDUnit).To(HaveLen(4))
		Expect(cuMW.VRegFile).To(HaveLen(4))
		Expect(cuMW.SRegFile).NotTo(BeNil())
		Expect(cuMW.WfPools).To(HaveLen(4))
		Expect(cuMW.WfDispatcher).NotTo(BeNil())
		Expect(cuMW.Decoder).NotTo(BeNil())

		Expect(comp.Resources.Decoder).NotTo(BeNil())
		Expect(comp.Resources.ALU).NotTo(BeNil())

		view := DispatcherView{CU: comp}
		Expect(view.WfPoolSizes()).To(Equal([]int{10, 10, 10, 10}))
		Expect(view.VRegCounts()).To(
			Equal([]int{16384, 16384, 16384, 16384}))
		Expect(view.SRegCount()).To(Equal(3200))
		Expect(view.LDSBytes()).To(Equal(64 * 1024))
		Expect(view.DispatchingPort()).To(
			Equal(messaging.RemotePort("GPU.CU.Top")))
		Expect(view.ControlPort()).To(
			Equal(messaging.RemotePort("GPU.CU.Ctrl")))
	})
})
