package cu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
	"github.com/sarchlab/mgpusim/v4/amd/timing/wavefront"
)

var _ = Describe("SIMD Unit", func() {

	var (
		cu   *ComputeUnit
		bu   *SIMDUnit
		sp   *mockScratchpadPreparer
		alu  *mockALU
		name string
	)

	BeforeEach(func() {
		cu = NewComputeUnit("CU", nil)
		sp = new(mockScratchpadPreparer)
		alu = new(mockALU)
		name = "simd"
		bu = NewSIMDUnit(cu, name, sp, alu)

	})

	It("should allow accepting wavefront", func() {
		// wave := new(Wavefront)
		bu.toExec = nil
		Expect(bu.CanAcceptWave()).To(BeTrue())
	})

	It("should not allow accepting wavefront if the SIMD is executing an instruction", func() {
		bu.toExec = new(wavefront.Wavefront)
		Expect(bu.CanAcceptWave()).To(BeFalse())
	})

	It("should accept wave", func() {
		wave := new(wavefront.Wavefront)
		inst := wavefront.NewInst(insts.NewInst())
		wave.SetDynamicInst(inst)
		bu.AcceptWave(wave)
		Expect(bu.toExec).To(BeIdenticalTo(wave))
		Expect(bu.cycleLeft).To(Equal(4))
	})

	It("should run", func() {
		wave := new(wavefront.Wavefront)
		inst := wavefront.NewInst(insts.NewInst())
		inst.FormatType = insts.VOPC
		inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		inst.ByteSize = 4
		wave.InstBuffer = make([]byte, 256)
		wave.InstBufferStartPC = 0x100
		wave.SetDynamicInst(inst)

		wave.PC = 0x13C

		wave.State = wavefront.WfRunning

		bu.toExec = wave
		bu.cycleLeft = 1

		bu.Run()

		Expect(wave.State).To(Equal(wavefront.WfReady))
		Expect(wave.PC).To(Equal(uint64(0x140)))

		Expect(bu.toExec).To(BeNil())
		Expect(bu.cycleLeft).To(Equal(0))

		Expect(sp.wfPrepared).To(BeIdenticalTo(wave))
		Expect(alu.wfExecuted).To(BeIdenticalTo(wave))
		Expect(sp.wfCommitted).To(BeIdenticalTo(wave))

		Expect(wave.InstBuffer).To(HaveLen(192))

	})

	It("should flush SIMD", func() {
		wave := new(wavefront.Wavefront)
		inst := wavefront.NewInst(insts.NewInst())
		inst.FormatType = insts.VOPC
		inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		inst.ByteSize = 4
		wave.InstBuffer = make([]byte, 256)
		wave.InstBufferStartPC = 0x100
		wave.SetDynamicInst(inst)
		wave.PC = 0x13C

		wave.State = wavefront.WfRunning

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
