package timing

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/gcn3/insts"
)

var _ = Describe("LDS Unit", func() {

	var (
		cu  *ComputeUnit
		sp  *mockScratchpadPreparer
		bu  *LDSUnit
		alu *mockALU
	)

	BeforeEach(func() {
		cu = NewComputeUnit("cu", nil)
		sp = new(mockScratchpadPreparer)
		alu = new(mockALU)
		bu = NewLDSUnit(cu, sp, alu)
	})

	It("should allow accepting wavefront", func() {
		// wave := new(Wavefront)
		bu.toRead = nil
		Expect(bu.CanAcceptWave()).To(BeTrue())
	})

	It("should not allow accepting wavefront is the read stage buffer is occupied", func() {
		bu.toRead = new(Wavefront)
		Expect(bu.CanAcceptWave()).To(BeFalse())
	})

	It("should accept wave", func() {
		wave := new(Wavefront)
		bu.AcceptWave(wave, 10)
		Expect(bu.toRead).To(BeIdenticalTo(wave))
	})

	It("should run", func() {
		wave1 := new(Wavefront)
		wave2 := new(Wavefront)
		wave2.WG = NewWorkGroup(nil, nil)
		wave2.WG.LDS = make([]byte, 0)
		wave3 := new(Wavefront)
		inst := NewInst(insts.NewInst())
		inst.FormatType = insts.DS
		inst.Opcode = 0
		inst.Addr = insts.NewVRegOperand(0, 0, 1)
		inst.Data = insts.NewVRegOperand(2, 2, 2)
		inst.Data1 = insts.NewVRegOperand(4, 4, 2)
		inst.ByteSize = 4
		wave3.inst = inst
		wave3.PC = 0x13C
		wave3.InstBuffer = make([]byte, 256)
		wave3.InstBufferStartPC = 0x100

		wave3.State = WfRunning

		bu.toRead = wave1
		bu.toExec = wave2
		bu.toWrite = wave3

		bu.Run(10)

		Expect(wave3.State).To(Equal(WfReady))
		Expect(wave3.PC).To(Equal(uint64(0x140)))

		Expect(bu.toWrite).To(BeIdenticalTo(wave2))
		Expect(bu.toExec).To(BeIdenticalTo(wave1))
		Expect(bu.toRead).To(BeNil())

		Expect(sp.wfPrepared).To(BeIdenticalTo(wave1))
		Expect(alu.wfExecuted).To(BeIdenticalTo(wave2))
		Expect(sp.wfCommitted).To(BeIdenticalTo(wave3))

		Expect(wave3.InstBuffer).To(HaveLen(192))

	})
})
