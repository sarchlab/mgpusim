package timing

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/gcn3/insts"
)

var _ = Describe("SIMD Unit", func() {

	var (
		cu  *ComputeUnit
		bu  *SIMDUnit
		sp  *mockScratchpadPreparer
		alu *mockALU
	)

	BeforeEach(func() {
		cu = NewComputeUnit("cu", nil)
		sp = new(mockScratchpadPreparer)
		alu = new(mockALU)
		bu = NewSIMDUnit(cu, sp, alu)
	})

	It("should allow accepting wavefront", func() {
		// wave := new(Wavefront)
		bu.toExec = nil
		Expect(bu.CanAcceptWave()).To(BeTrue())
	})

	It("should not allow accepting wavefront if the SIMD is executing an instruction", func() {
		bu.toExec = new(Wavefront)
		Expect(bu.CanAcceptWave()).To(BeFalse())
	})

	It("should accept wave", func() {
		wave := new(Wavefront)
		bu.AcceptWave(wave, 10)
		Expect(bu.toExec).To(BeIdenticalTo(wave))
		Expect(bu.cycleLeft).To(Equal(4))
	})

	It("should run", func() {
		wave := new(Wavefront)
		inst := NewInst(insts.NewInst())
		inst.FormatType = insts.VOPC
		inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		inst.ByteSize = 4
		wave.InstBuffer = make([]byte, 256)
		wave.InstBufferStartPC = 0x100
		wave.inst = inst
		wave.PC = 0x13C

		wave.State = WfRunning

		bu.toExec = wave
		bu.cycleLeft = 1

		bu.Run(10)

		Expect(wave.State).To(Equal(WfReady))
		Expect(wave.PC).To(Equal(uint64(0x140)))

		Expect(bu.toExec).To(BeNil())
		Expect(bu.cycleLeft).To(Equal(0))

		Expect(sp.wfPrepared).To(BeIdenticalTo(wave))
		Expect(alu.wfExecuted).To(BeIdenticalTo(wave))
		Expect(sp.wfCommitted).To(BeIdenticalTo(wave))

		Expect(wave.InstBuffer).To(HaveLen(192))

	})

	It("should flush SIMD", func() {
		wave := new(Wavefront)
		inst := NewInst(insts.NewInst())
		inst.FormatType = insts.VOPC
		inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		inst.ByteSize = 4
		wave.InstBuffer = make([]byte, 256)
		wave.InstBufferStartPC = 0x100
		wave.inst = inst
		wave.PC = 0x13C

		wave.State = WfRunning

		bu.toExec = wave

		bu.Flush()

		Expect(bu.toExec).To(BeNil())
	})

	//It("should spend 4 cycles in execution", func() {
	//	wave1 := new(Wavefront)
	//	wave2 := new(Wavefront)
	//	wave3 := new(Wavefront)
	//	wave3.State = WfRunning
	//
	//	bu.toRead = wave1
	//	bu.toExec = wave2
	//	bu.toWrite = wave3
	//	bu.cycleLeft = 4
	//
	//	bu.Run(10)
	//
	//	Expect(wave3.State).To(Equal(WfReady))
	//	Expect(bu.toWrite).To(BeNil())
	//	Expect(bu.toExec).To(BeIdenticalTo(wave2))
	//	Expect(bu.cycleLeft).To(Equal(3))
	//	Expect(bu.toRead).To(BeIdenticalTo(wave1))
	//})
})
