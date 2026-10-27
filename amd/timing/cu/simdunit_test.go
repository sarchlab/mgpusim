package cu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

var _ = Describe("SIMD Unit", func() {
	var (
		cu   *ComputeUnit
		bu   *SIMDUnit
		alu  *mockALU
		name string
	)

	BeforeEach(func() {
		cu = newTestComputeUnit("CU", nil)
		alu = new(mockALU)
		name = "simd"
		bu = NewSIMDUnit(cu, name, alu)

	})

	It("should allow accepting wavefront", func() {
		// wave := new(Wavefront)
		bu.state().Exec = 0
		Expect(bu.CanAcceptWave()).To(BeTrue())
	})

	It("should not allow accepting wavefront if the SIMD is executing an instruction", func() {
		bu.state().Exec = testWaveID(cu, newTestWave())
		Expect(bu.CanAcceptWave()).To(BeFalse())
	})

	It("should accept wave", func() {
		wave := newTestWave()
		inst := testInst(testIDs.NewID(), insts.NewInst())
		wave.SetDynamicInst(inst)
		registerTestWave(cu, wave)
		bu.AcceptWave(wave)
		Expect(cu.wave(bu.state().Exec)).To(Equal(wave))
		Expect(bu.state().Cycles).To(Equal(4))
	})

	It("should run", func() {
		wave := newTestWave()
		inst := testInst(testIDs.NewID(), insts.NewInst())
		inst.FormatType = insts.VOPC
		inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		inst.ByteSize = 4
		wave.execution().InstBuffer = make([]byte, 256)
		wave.execution().InstBufferStartPC = 0x100
		wave.SetDynamicInst(inst)

		wave.SetPC(0x13C)

		wave.execution().State = wavefront.WfRunning

		bu.state().Exec = testWaveID(cu, wave)
		bu.state().Cycles = 1

		bu.Run()

		Expect(wave.execution().State).To(Equal(wavefront.WfReady))
		Expect(wave.PC()).To(Equal(uint64(0x140)))

		Expect(cu.wave(bu.state().Exec)).To(BeNil())
		Expect(bu.state().Cycles).To(Equal(0))

		Expect(alu.wfExecuted.(*operationWave).value).To(BeIdenticalTo(wave.execution()))

		Expect(wave.execution().InstBuffer).To(HaveLen(192))

	})

})
