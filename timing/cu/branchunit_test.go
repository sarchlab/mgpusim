package cu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/mgpusim/v3/insts"
	"gitlab.com/akita/mgpusim/v3/timing/wavefront"
)

var _ = Describe("Branch Unit", func() {

	var (
		cu  *ComputeUnit
		bu  *BranchUnit
		sp  *mockScratchpadPreparer
		alu *mockALU
	)

	BeforeEach(func() {
		cu = NewComputeUnit("CU", nil)
		sp = new(mockScratchpadPreparer)
		alu = new(mockALU)
		bu = NewBranchUnit(cu, sp, alu)
	})

	It("should allow accepting wavefront", func() {
		// wave := new(wavefront.Wavefront)
		bu.toRead = nil
		Expect(bu.CanAcceptWave()).To(BeTrue())
	})

	It("should not allow accepting wavefront is the read stage buffer is occupied", func() {
		bu.toRead = new(wavefront.Wavefront)
		Expect(bu.CanAcceptWave()).To(BeFalse())
	})

	It("should accept wave", func() {
		wave := new(wavefront.Wavefront)
		bu.AcceptWave(wave, 10)
		Expect(bu.toRead).To(BeIdenticalTo(wave))
	})

	It("should run", func() {
		wave1 := new(wavefront.Wavefront)
		wave2 := new(wavefront.Wavefront)
		wave3 := new(wavefront.Wavefront)
		wave3.State = wavefront.WfRunning
		wave3.InstBuffer = make([]byte, 256)
		wave3.InstBufferStartPC = 0x100
		inst := wavefront.NewInst(insts.NewInst())
		inst.FormatType = insts.SOPP
		inst.SImm16 = insts.NewIntOperand(1, 1)
		inst.ByteSize = 4
		wave3.SetDynamicInst(inst)
		wave3.PC = 0x13C

		bu.toRead = wave1
		bu.toExec = wave2
		bu.toWrite = wave3

		bu.Run(10)

		Expect(wave3.State).To(Equal(wavefront.WfReady))
		Expect(wave3.PC).To(Equal(uint64(0x140)))

		Expect(bu.toWrite).To(BeIdenticalTo(wave2))
		Expect(bu.toExec).To(BeIdenticalTo(wave1))
		Expect(bu.toRead).To(BeNil())

		Expect(sp.wfPrepared).To(BeIdenticalTo(wave1))
		Expect(alu.wfExecuted).To(BeIdenticalTo(wave2))
		Expect(sp.wfCommitted).To(BeIdenticalTo(wave3))
		Expect(wave3.InstBuffer).To(HaveLen(0))
	})

	It("should flush", func() {
		wave1 := new(wavefront.Wavefront)
		wave2 := new(wavefront.Wavefront)
		wave3 := new(wavefront.Wavefront)
		wave3.State = wavefront.WfRunning
		wave3.InstBuffer = make([]byte, 256)
		wave3.InstBufferStartPC = 0x100
		inst := wavefront.NewInst(insts.NewInst())
		inst.FormatType = insts.SOPP
		inst.SImm16 = insts.NewIntOperand(1, 1)
		inst.ByteSize = 4
		wave3.SetDynamicInst(inst)

		bu.toRead = wave1
		bu.toExec = wave2
		bu.toWrite = wave3

		bu.Flush()

		Expect(bu.toRead).To(BeNil())
		Expect(bu.toWrite).To(BeNil())
		Expect(bu.toExec).To(BeNil())

	})
})
