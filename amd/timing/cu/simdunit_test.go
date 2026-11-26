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
		wave.setTestInst(inst)
		registerTestWave(cu, wave)
		bu.AcceptWave(wave)
		Expect(cu.wave(bu.state().Exec)).To(Equal(wave))
		Expect(bu.state().Cycles).To(Equal(4))
	})

	DescribeTable("should take twice the cycles on the double-width datapath",
		func(name string, cycles int) {
			wave := newTestWave()
			inst := testInst(testIDs.NewID(), insts.NewInst())
			inst.InstName = name
			wave.setTestInst(inst)
			registerTestWave(cu, wave)

			bu.AcceptWave(wave)

			Expect(bu.state().Cycles).To(Equal(cycles))
		},
		Entry("f32 FMA", "v_fma_f32", 4),
		Entry("f64 FMA", "v_fma_f64", 8),
		Entry("packed f32 FMA", "v_pk_fma_f32", 8),
		Entry("packed f32 multiply", "v_pk_mul_f32", 8),
		Entry("packed f32 add", "v_pk_add_f32", 8),
		Entry("packed b32 move", "v_pk_mov_b32", 8),
		Entry("b64 move", "v_mov_b64_e32", 8),
		Entry("64-bit shift-add stays narrow", "v_lshl_add_u64", 4),
	)

	It("should run", func() {
		wave := newTestWave()
		inst := testInst(testIDs.NewID(), insts.NewInst())
		inst.FormatType = insts.VOPC
		inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		inst.ByteSize = 4
		wave.execution().InstBuffer = make([]byte, 256)
		wave.execution().InstBufferStartPC = 0x100
		wave.setTestInst(inst)

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
