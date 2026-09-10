package cu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

var _ = Describe("Builder", func() {
	It("should build a fully equipped compute unit", func() {
		engine := timing.NewSerialEngine()
		reg := modeling.NewStandaloneRegistrar(engine)

		comp := MakeBuilder().
			WithRegistrar(reg).
			WithSpec(DefaultSpec()).
			Build("GPU.CU")

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

		Expect(comp.Resources().Decoder).NotTo(BeNil())
		Expect(comp.Resources().ALU).NotTo(BeNil())

		// All five ports must be declared so external code can assign them.
		for _, portName := range []string{
			DispatchPortName, CtrlPortName,
			InstMemPortName, ScalarMemPortName, VectorMemPortName,
		} {
			p := messaging.NewPort(comp, 4, 4, "GPU.CU."+portName)
			Expect(func() { comp.AssignPort(portName, p) }).NotTo(Panic())
		}

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

	It("should derive the lane stride from a 32768-VGPR SIMD", func() {
		engine := timing.NewSerialEngine()
		reg := modeling.NewStandaloneRegistrar(engine)
		spec := DefaultSpec()
		spec.WfPoolSize = 8
		spec.VGPRCounts = []int{32768, 32768, 32768, 32768}

		comp := MakeBuilder().
			WithRegistrar(reg).
			WithSpec(spec).
			Build("GPU.CU.VGPR32768")

		cuMW := MiddlewareOf(comp)
		registerFile, ok := cuMW.VRegFile[0].(*SimpleRegisterFile)
		Expect(ok).To(BeTrue())
		Expect(registerFile.ByteSizePerLane).To(Equal(2048))

		// A 34-VGPR kernel occupies 36 VGPRs after allocation rounding. The
		// eighth resident wave starts at byte offset 7*36*4 = 1008. Its v8
		// therefore crosses byte 1024, but must not alias lane 1's v4.
		eighthWaveV8 := RegisterAccess{
			Reg:        insts.VReg(8),
			RegCount:   1,
			LaneID:     0,
			WaveOffset: 7 * 36 * 4,
			Data:       insts.Uint32ToBytes(0x11223344),
		}
		nextLaneV4 := RegisterAccess{
			Reg:        insts.VReg(4),
			RegCount:   1,
			LaneID:     1,
			WaveOffset: 0,
			Data:       insts.Uint32ToBytes(0x55667788),
		}

		registerFile.Write(eighthWaveV8)
		registerFile.Write(nextLaneV4)

		eighthWaveV8.Data = make([]byte, 4)
		nextLaneV4.Data = make([]byte, 4)
		registerFile.Read(eighthWaveV8)
		registerFile.Read(nextLaneV4)

		Expect(insts.BytesToUint32(eighthWaveV8.Data)).To(
			Equal(uint32(0x11223344)))
		Expect(insts.BytesToUint32(nextLaneV4.Data)).To(
			Equal(uint32(0x55667788)))
	})

	It("should reject an invalid per-SIMD VGPR count", func() {
		engine := timing.NewSerialEngine()
		reg := modeling.NewStandaloneRegistrar(engine)

		for _, invalidCount := range []int{0, 32767} {
			spec := DefaultSpec()
			spec.VGPRCounts = []int{
				invalidCount, 16384, 16384, 16384,
			}

			Expect(func() {
				MakeBuilder().
					WithRegistrar(reg).
					WithSpec(spec).
					Build("GPU.CU.InvalidVGPRCount")
			}).To(PanicWith(
				"cu: VGPRCounts[0] must be positive and divisible by 64"))
		}
	})
})
