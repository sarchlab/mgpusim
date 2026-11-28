package cu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
)

var _ = Describe("Builder", func() {
	It("should build a fully equipped compute unit", func() {
		reg := sim.MakeBuilder().Build()
		DeferCleanup(func() { Expect(reg.Terminate()).To(Succeed()) })

		spec := DefaultSpec()
		spec.VectorMemModules = mem.AddressMapper{Destinations: []messaging.RemotePort{"Memory.Top"}}
		comp := MakeBuilder().WithSpec(spec).
			WithSimulation(reg).
			Build("GPU.CU")

		// All four ports must be declared so external code can assign them.
		for _, portName := range []string{
			DispatchPortName,
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
	})

	It("should reject issue rotation without four SIMDs", func() {
		reg := sim.MakeBuilder().Build()
		DeferCleanup(func() { Expect(reg.Terminate()).To(Succeed()) })

		spec := DefaultSpec()
		spec.IssueRotation = true
		spec.SIMDCount = 2
		spec.VGPRCounts = []int{16384, 16384}

		Expect(func() {
			MakeBuilder().WithSpec(spec).WithSimulation(reg).Build("GPU.CU")
		}).To(PanicWith(ContainSubstring("IssueRotation requires SIMDCount")))
	})

	It("should require a fetch-ahead buffer of at least two whole fetches", func() {
		reg := sim.MakeBuilder().Build()
		DeferCleanup(func() { Expect(reg.Terminate()).To(Succeed()) })

		build := func(name string, lines, bufferSize int) func() {
			spec := DefaultSpec()
			spec.InstFetchAheadLines = lines
			spec.InstBufByteSize = bufferSize
			return func() { MakeBuilder().WithSpec(spec).WithSimulation(reg).Build(name) }
		}

		Expect(build("GPU.CU[0]", 2, 100)).To(PanicWith(ContainSubstring("InstBufByteSize must be a multiple")))
		Expect(build("GPU.CU[1]", 2, 64)).To(PanicWith(ContainSubstring("InstBufByteSize must be a multiple")))
		Expect(build("GPU.CU[2]", 2, 128)).NotTo(Panic())
		// One line at a time keeps its capacity rule.
		Expect(build("GPU.CU[3]", 1, 100)).NotTo(Panic())
	})
})
