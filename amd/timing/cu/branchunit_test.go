package cu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

var _ = Describe("Branch Unit", func() {
	var (
		cu  *ComputeUnit
		bu  *BranchUnit
		alu *mockALU
	)

	BeforeEach(func() {
		cu = newTestComputeUnit("CU", nil)
		alu = new(mockALU)
		bu = NewBranchUnit(cu, alu)
	})

	It("should allow accepting wavefront", func() {
		// wave := newTestWave()
		bu.state().Read = 0
		Expect(bu.CanAcceptWave()).To(BeTrue())
	})

	It("should not allow accepting wavefront is the read stage buffer is occupied", func() {
		bu.state().Read = testWaveID(cu, newTestWave())
		Expect(bu.CanAcceptWave()).To(BeFalse())
	})

	It("should accept wave", func() {
		wave := newTestWave()
		registerTestWave(cu, wave)
		bu.AcceptWave(wave)
		Expect(cu.wave(bu.state().Read)).To(Equal(wave))
	})

	It("should run", func() {
		wave1 := newTestWave()
		wave2 := newTestWave()
		wave3 := newTestWave()
		wave3.execution().State = wavefront.WfRunning
		wave3.execution().InstBuffer = make([]byte, 256)
		wave3.execution().InstBufferStartPC = 0x100
		inst := testInst(testIDs.NewID(), insts.NewInst())
		inst.FormatType = insts.SOPP
		inst.SImm16 = insts.NewIntOperand(1, 1)
		inst.ByteSize = 4
		wave3.SetDynamicInst(inst)
		wave3.SetPC(0x13C)

		bu.state().Read = testWaveID(cu, wave1)
		bu.state().Exec = testWaveID(cu, wave2)
		bu.state().Write = testWaveID(cu, wave3)

		bu.Run()

		Expect(wave3.execution().State).To(Equal(wavefront.WfReady))
		Expect(wave3.PC()).To(Equal(uint64(0x140)))

		Expect(cu.wave(bu.state().Write)).To(Equal(wave2))
		Expect(cu.wave(bu.state().Exec)).To(Equal(wave1))
		Expect(cu.wave(bu.state().Read)).To(BeNil())

		Expect(alu.wfExecuted.(*operationWave).value).To(BeIdenticalTo(wave2.execution()))
		Expect(wave3.execution().InstBuffer).To(HaveLen(0))
	})

	It("should flush", func() {
		wave1 := newTestWave()
		wave2 := newTestWave()
		wave3 := newTestWave()
		wave3.execution().State = wavefront.WfRunning
		wave3.execution().InstBuffer = make([]byte, 256)
		wave3.execution().InstBufferStartPC = 0x100
		inst := testInst(testIDs.NewID(), insts.NewInst())
		inst.FormatType = insts.SOPP
		inst.SImm16 = insts.NewIntOperand(1, 1)
		inst.ByteSize = 4
		wave3.SetDynamicInst(inst)

		bu.state().Read = testWaveID(cu, wave1)
		bu.state().Exec = testWaveID(cu, wave2)
		bu.state().Write = testWaveID(cu, wave3)

		bu.Flush()

		Expect(cu.wave(bu.state().Read)).To(BeNil())
		Expect(cu.wave(bu.state().Write)).To(BeNil())
		Expect(cu.wave(bu.state().Exec)).To(BeNil())

	})
})
