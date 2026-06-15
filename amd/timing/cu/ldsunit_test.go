package cu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
	"github.com/sarchlab/mgpusim/v4/amd/timing/wavefront"
)

var _ = Describe("LDS Unit", func() {

	var (
		cu  *ComputeUnit
		bu  *LDSUnit
		alu *mockALU
	)

	BeforeEach(func() {
		cu = NewComputeUnit("CU", nil)
		alu = new(mockALU)
		bu = NewLDSUnit(cu, alu)
	})

	It("should allow accepting wavefront", func() {
		// wave := new(Wavefront)
		bu.toRead = nil
		Expect(bu.CanAcceptWave()).To(BeTrue())
	})

	It("should not allow accepting wavefront is the read stage buffer is occupied", func() {
		bu.toRead = new(wavefront.Wavefront)
		Expect(bu.CanAcceptWave()).To(BeFalse())
	})

	It("should accept wave", func() {
		wave := new(wavefront.Wavefront)
		bu.AcceptWave(wave)
		Expect(bu.toRead).To(BeIdenticalTo(wave))
	})

	It("should run", func() {
		wave1 := new(wavefront.Wavefront)
		wave2 := new(wavefront.Wavefront)
		wave2.WG = wavefront.NewWorkGroup(nil, nil)
		wave2.WG.LDS = make([]byte, 0)
		wave3 := new(wavefront.Wavefront)
		inst := wavefront.NewInst(insts.NewInst())
		inst.FormatType = insts.DS
		inst.Opcode = 0
		inst.Addr = insts.NewVRegOperand(0, 0, 1)
		inst.Data = insts.NewVRegOperand(2, 2, 2)
		inst.Data1 = insts.NewVRegOperand(4, 4, 2)
		inst.ByteSize = 4
		wave3.SetDynamicInst(inst)
		wave3.SetPC(0x13C)
		wave3.InstBuffer = make([]byte, 256)
		wave3.InstBufferStartPC = 0x100

		wave3.State = wavefront.WfRunning

		bu.toRead = wave1
		bu.toExec = wave2
		bu.toWrite = wave3

		bu.Run()

		// wave3 completes write stage
		Expect(wave3.State).To(Equal(wavefront.WfReady))
		Expect(wave3.PC()).To(Equal(uint64(0x140)))
		Expect(wave3.InstBuffer).To(HaveLen(192))

		// wave2: ALU runs, cycleLeft set to 14, stays in toExec
		Expect(bu.toExec).To(BeIdenticalTo(wave2))
		Expect(bu.toWrite).To(BeNil())
		Expect(alu.wfExecuted).To(BeIdenticalTo(wave2))

		// wave1: can't move from toRead because toExec is occupied
		Expect(bu.toRead).To(BeIdenticalTo(wave1))

		// Run 14 more cycles to drain cycleLeft for wave2
		for i := 0; i < 13; i++ {
			bu.Run()
		}

		// After 13 more runs, cycleLeft should be 1, wave2 still in toExec
		Expect(bu.toExec).To(BeIdenticalTo(wave2))
		Expect(bu.toWrite).To(BeNil())

		// 14th run: cycleLeft reaches 0, wave2 moves to toWrite
		bu.Run()
		Expect(bu.toWrite).To(BeIdenticalTo(wave2))
		Expect(bu.toExec).To(BeIdenticalTo(wave1))
		Expect(bu.toRead).To(BeNil())

	})
	It("should flush the LDS", func() {

		wave1 := new(wavefront.Wavefront)
		wave2 := new(wavefront.Wavefront)
		wave2.WG = wavefront.NewWorkGroup(nil, nil)
		wave2.WG.LDS = make([]byte, 0)
		wave3 := new(wavefront.Wavefront)
		inst := wavefront.NewInst(insts.NewInst())
		inst.FormatType = insts.DS
		inst.Opcode = 0
		inst.Addr = insts.NewVRegOperand(0, 0, 1)
		inst.Data = insts.NewVRegOperand(2, 2, 2)
		inst.Data1 = insts.NewVRegOperand(4, 4, 2)
		inst.ByteSize = 4
		wave3.SetDynamicInst(inst)
		wave3.SetPC(0x13C)
		wave3.InstBuffer = make([]byte, 256)
		wave3.InstBufferStartPC = 0x100

		wave3.State = wavefront.WfRunning

		bu.toRead = wave1
		bu.toExec = wave2
		bu.toWrite = wave3

		bu.Flush()

		Expect(bu.toRead).To(BeNil())
		Expect(bu.toWrite).To(BeNil())
		Expect(bu.toExec).To(BeNil())
		Expect(bu.cycleLeft).To(Equal(0))

	})
})
