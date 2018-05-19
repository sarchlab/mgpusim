package timing

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/insts"
)

var _ = Describe("Vector Memory Unit", func() {

	var (
		cu        *ComputeUnit
		sp        *mockScratchpadPreparer
		bu        *VectorMemoryUnit
		alu       *mockALU
		vectorMem *core.MockComponent
		conn      *core.MockConnection
	)

	BeforeEach(func() {
		cu = NewComputeUnit("cu", nil)
		sp = new(mockScratchpadPreparer)
		alu = new(mockALU)
		bu = NewVectorMemoryUnit(cu, sp, alu)
		vectorMem = core.NewMockComponent("VectorMem")
		conn = core.NewMockConnection()

		cu.ScalarMem = vectorMem
		core.PlugIn(cu, "ToVectorMem", conn)
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
		inst := NewInst(insts.NewInst())
		inst.FormatType = insts.SOP2
		wave2.inst = inst
		wave3 := new(Wavefront)
		wave3.State = WfRunning

		bu.toRead = wave1
		bu.toExec = wave2
		bu.toWrite = wave3

		bu.Run(10)

		Expect(wave3.State).To(Equal(WfReady))
		Expect(bu.toWrite).To(BeIdenticalTo(wave2))
		Expect(bu.toExec).To(BeIdenticalTo(wave1))
		Expect(bu.toRead).To(BeNil())

		Expect(sp.wfPrepared).To(BeIdenticalTo(wave1))
		Expect(alu.wfExecuted).To(BeIdenticalTo(wave2))
		Expect(sp.wfCommitted).To(BeIdenticalTo(wave3))
	})

})
