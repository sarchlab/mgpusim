package timing

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/kernels"
	"gitlab.com/akita/mem"
)

type mockWfArbitor struct {
	wfsToReturn [][]*Wavefront
}

func newMockWfArbitor() *mockWfArbitor {
	a := new(mockWfArbitor)
	a.wfsToReturn = make([][]*Wavefront, 0)
	return a
}

func (m *mockWfArbitor) Arbitrate([]*WavefrontPool) []*Wavefront {
	if len(m.wfsToReturn) == 0 {
		return nil
	}
	wfs := m.wfsToReturn[0]
	m.wfsToReturn = m.wfsToReturn[1:]
	return wfs
}

type mockCUComponent struct {
	canAccept    bool
	acceptedWave []*Wavefront
}

func (c *mockCUComponent) CanAcceptWave() bool {
	return c.canAccept
}

func (c *mockCUComponent) AcceptWave(wave *Wavefront, now akita.VTimeInSec) {
	c.acceptedWave = append(c.acceptedWave, wave)
}

func (c *mockCUComponent) Run(now akita.VTimeInSec) bool {
	return true
}

var _ = Describe("Scheduler", func() {
	var (
		toInstMemConn    *akita.MockConnection
		engine           *akita.MockEngine
		cu               *ComputeUnit
		branchUnit       *mockCUComponent
		ldsDecoder       *mockCUComponent
		vectorMemDecoder *mockCUComponent
		vectorDecoder    *mockCUComponent
		scalarDecoder    *mockCUComponent
		scheduler        *Scheduler
		fetchArbitor     *mockWfArbitor
		issueArbitor     *mockWfArbitor
		instMem          *akita.MockComponent
	)

	BeforeEach(func() {
		toInstMemConn = akita.NewMockConnection()
		engine = akita.NewMockEngine()
		cu = NewComputeUnit("cu", engine)
		cu.Freq = 1

		vectorDecoder = new(mockCUComponent)
		cu.VectorDecoder = vectorDecoder
		scalarDecoder = new(mockCUComponent)
		cu.ScalarDecoder = scalarDecoder
		branchUnit = new(mockCUComponent)
		cu.BranchUnit = branchUnit
		vectorMemDecoder = new(mockCUComponent)
		cu.VectorMemDecoder = vectorMemDecoder
		ldsDecoder = new(mockCUComponent)
		cu.LDSDecoder = ldsDecoder
		cu.VRegFile = append(cu.VRegFile, NewSimpleRegisterFile(16384, 1024))
		cu.VRegFile = append(cu.VRegFile, NewSimpleRegisterFile(16384, 1024))
		cu.VRegFile = append(cu.VRegFile, NewSimpleRegisterFile(16384, 1024))
		cu.VRegFile = append(cu.VRegFile, NewSimpleRegisterFile(16384, 1024))
		cu.SRegFile = NewSimpleRegisterFile(16384, 0)

		instMem = akita.NewMockComponent("InstMem")

		cu.InstMem = instMem.ToOutside
		toInstMemConn.PlugIn(cu.ToInstMem)

		fetchArbitor = newMockWfArbitor()
		issueArbitor = newMockWfArbitor()
		scheduler = NewScheduler(cu, fetchArbitor, issueArbitor)
	})

	It("should always fetch 64 bytes", func() {
		wf := new(Wavefront)
		wf.InstBufferStartPC = 0x100
		wf.InstBuffer = make([]byte, 0x80)

		fetchArbitor.wfsToReturn = append(fetchArbitor.wfsToReturn,
			[]*Wavefront{wf})

		reqToExpect := mem.NewReadReq(10, cu.ToInstMem, instMem.ToOutside, 0x180, 64)
		toInstMemConn.ExpectSend(reqToExpect, nil)

		scheduler.DoFetch(10)
		Expect(toInstMemConn.AllExpectedSent()).To(BeTrue())
		Expect(cu.inFlightMemAccess).To(HaveLen(1))
		Expect(wf.IsFetching).To(BeTrue())
	})

	It("should wait if fetch failed", func() {
		wf := new(Wavefront)
		wf.InstBufferStartPC = 0x100
		wf.InstBuffer = make([]byte, 0x80)
		fetchArbitor.wfsToReturn = append(fetchArbitor.wfsToReturn,
			[]*Wavefront{wf})

		reqToExpect := mem.NewReadReq(10, cu.ToInstMem, instMem.ToOutside, 0x180, 64)
		toInstMemConn.ExpectSend(reqToExpect, akita.NewSendError())

		scheduler.DoFetch(10)

		Expect(toInstMemConn.AllExpectedSent()).To(BeTrue())
		Expect(cu.inFlightMemAccess).To(HaveLen(0))
		Expect(wf.IsFetching).To(BeFalse())
	})

	It("should issue", func() {
		wfs := make([]*Wavefront, 0)
		issueDirs := []insts.ExeUnit{
			insts.ExeUnitBranch,
			insts.ExeUnitLDS,
			insts.ExeUnitVMem,
			insts.ExeUnitVALU,
			insts.ExeUnitScalar,
		}
		branchUnit.canAccept = true
		ldsDecoder.canAccept = true
		vectorDecoder.canAccept = true
		vectorMemDecoder.canAccept = true
		scalarDecoder.canAccept = false

		for i := 0; i < 5; i++ {
			wf := new(Wavefront)
			wf.PC = 0x120
			wf.InstBuffer = make([]byte, 256)
			wf.InstBufferStartPC = 0x100
			wf.State = WfReady
			wf.InstToIssue = NewInst(insts.NewInst())
			wf.InstToIssue.ExeUnit = issueDirs[i]
			wf.InstToIssue.ByteSize = 4
			wfs = append(wfs, wf)
		}
		wfs[0].PC = 0x13C
		issueArbitor.wfsToReturn = append(issueArbitor.wfsToReturn, wfs)

		scheduler.DoIssue(10)

		Expect(len(branchUnit.acceptedWave)).To(Equal(1))
		Expect(len(ldsDecoder.acceptedWave)).To(Equal(1))
		Expect(len(vectorDecoder.acceptedWave)).To(Equal(1))
		Expect(len(vectorMemDecoder.acceptedWave)).To(Equal(1))
		Expect(len(scalarDecoder.acceptedWave)).To(Equal(0))

		Expect(wfs[0].State).To(Equal(WfRunning))
		Expect(wfs[1].State).To(Equal(WfRunning))
		Expect(wfs[2].State).To(Equal(WfRunning))
		Expect(wfs[3].State).To(Equal(WfRunning))
		Expect(wfs[4].State).To(Equal(WfReady))

		Expect(wfs[0].PC).To(Equal(uint64(0x140)))
		Expect(wfs[1].PC).To(Equal(uint64(0x124)))
		Expect(wfs[2].PC).To(Equal(uint64(0x124)))
		Expect(wfs[3].PC).To(Equal(uint64(0x124)))
		Expect(wfs[4].PC).To(Equal(uint64(0x120)))

		Expect(wfs[0].InstToIssue).To(BeNil())
		Expect(wfs[1].InstToIssue).To(BeNil())
		Expect(wfs[2].InstToIssue).To(BeNil())
		Expect(wfs[3].InstToIssue).To(BeNil())
		Expect(wfs[4].InstToIssue).NotTo(BeNil())

		Expect(wfs[0].InstBufferStartPC).To(Equal(uint64(0x140)))
		Expect(wfs[0].InstBuffer).To(HaveLen(192))
	})

	It("should issue internal instruction", func() {
		wfs := make([]*Wavefront, 0)
		wf := new(Wavefront)
		wf.InstToIssue = NewInst(insts.NewInst())
		wf.InstToIssue.ExeUnit = insts.ExeUnitSpecial
		wf.InstToIssue.ByteSize = 4
		wf.PC = 10
		wf.State = WfReady
		wfs = append(wfs, wf)

		issueArbitor.wfsToReturn = append(issueArbitor.wfsToReturn, wfs)
		scheduler.internalExecuting = nil

		scheduler.DoIssue(10)

		Expect(scheduler.internalExecuting).To(BeIdenticalTo(wf))
		Expect(wf.State).To(Equal(WfRunning))
		Expect(wf.PC).To(Equal(uint64(14)))
		Expect(wf.InstToIssue).To(BeNil())
	})

	It("should evaluate internal executing insts", func() {
		wf := new(Wavefront)
		wf.CodeObject = insts.NewHsaCo()
		wf.SIMDID = 0
		wf.inst = NewInst(insts.NewInst())
		wf.inst.Format = insts.FormatTable[insts.SOPP]
		wf.inst.Opcode = 1 // S_ENDPGM

		scheduler.internalExecuting = wf
		scheduler.EvaluateInternalInst(10)

		Expect(len(engine.ScheduledEvent)).To(Equal(1))
	})

	It("should wait for memory access when running wait_cnt", func() {
		wf := new(Wavefront)
		wf.inst = NewInst(insts.NewInst())
		wf.inst.Format = insts.FormatTable[insts.SOPP]
		wf.inst.Opcode = 12 // WAIT_CNT
		wf.inst.LKGMCNT = 0
		wf.State = WfRunning
		wf.OutstandingScalarMemAccess = 1

		scheduler.internalExecuting = wf
		scheduler.EvaluateInternalInst(10)

		Expect(scheduler.internalExecuting).NotTo(BeNil())
		Expect(wf.State).To(Equal(WfRunning))
	})

	It("should wait for memory access when running wait_cnt", func() {
		wf := new(Wavefront)
		wf.inst = NewInst(insts.NewInst())
		wf.inst.Format = insts.FormatTable[insts.SOPP]
		wf.inst.Opcode = 12 // WAIT_CNT
		wf.inst.VMCNT = 0
		wf.State = WfRunning
		wf.OutstandingVectorMemAccess = 1

		scheduler.internalExecuting = wf
		scheduler.EvaluateInternalInst(10)

		Expect(scheduler.internalExecuting).NotTo(BeNil())
		Expect(wf.State).To(Equal(WfRunning))
	})

	It("should pass if memory returns when running wait_cnt", func() {
		wf := new(Wavefront)
		wf.inst = NewInst(insts.NewInst())
		wf.inst.Format = insts.FormatTable[insts.SOPP]
		wf.inst.Opcode = 12 // WAIT_CNT
		wf.inst.LKGMCNT = 0
		wf.inst.VMCNT = 0
		wf.State = WfRunning
		wf.OutstandingScalarMemAccess = 0
		wf.OutstandingVectorMemAccess = 0

		scheduler.internalExecuting = wf
		scheduler.EvaluateInternalInst(10)

		Expect(scheduler.internalExecuting).To(BeNil())
		Expect(wf.State).To(Equal(WfReady))
	})

	It("should not terminate wavefront if there are pending memory requests", func() {
		wf := new(Wavefront)
		wf.inst = NewInst(insts.NewInst())
		wf.inst.Format = insts.FormatTable[insts.SOPP]
		wf.inst.Opcode = 1 // WAIT_CNT
		wf.State = WfRunning
		wf.OutstandingScalarMemAccess = 1
		wf.OutstandingVectorMemAccess = 1

		scheduler.internalExecuting = wf
		scheduler.EvaluateInternalInst(10)

		Expect(scheduler.internalExecuting).NotTo(BeNil())
		Expect(len(engine.ScheduledEvent)).To(Equal(0))
	})

	It("should put wavefront in barrier buffer", func() {
		wg := new(WorkGroup)
		for i := 0; i < 4; i++ {
			wf := NewWavefront(kernels.NewWavefront())
			wf.State = WfRunning
			wf.inst = NewInst(insts.NewInst())
			wf.inst.Format = insts.FormatTable[insts.SOPP]
			wf.inst.Opcode = 10
			wf.WG = wg
			wg.Wfs = append(wg.Wfs, wf)
		}
		wf := wg.Wfs[0]

		scheduler.internalExecuting = wf
		scheduler.EvaluateInternalInst(10)

		Expect(wf.State).To(Equal(WfAtBarrier))
		Expect(len(scheduler.barrierBuffer)).To(Equal(1))
		Expect(scheduler.barrierBuffer[0]).To(BeIdenticalTo(wf))
		Expect(scheduler.internalExecuting).To(BeNil())
	})

	It("should wait if barrier buffer is full", func() {
		wg := new(WorkGroup)
		for i := 0; i < 4; i++ {
			wf := NewWavefront(kernels.NewWavefront())
			wf.State = WfRunning
			wf.inst = NewInst(insts.NewInst())
			wf.inst.Format = insts.FormatTable[insts.SOPP]
			wf.inst.Opcode = 10
			wf.WG = wg
			wg.Wfs = append(wg.Wfs, wf)
		}
		wf := wg.Wfs[0]

		scheduler.barrierBuffer = make([]*Wavefront, 0, scheduler.barrierBufferSize)
		for i := 0; i < 16; i++ {
			wf := NewWavefront(kernels.NewWavefront())
			wf.State = WfAtBarrier
			scheduler.barrierBuffer = append(scheduler.barrierBuffer, wf)
		}
		scheduler.internalExecuting = wf
		scheduler.EvaluateInternalInst(10)

		Expect(wf.State).To(Equal(WfRunning))
		Expect(len(scheduler.barrierBuffer)).To(Equal(scheduler.barrierBufferSize))
		Expect(scheduler.internalExecuting).NotTo(BeNil())
	})

	It("should continue execution if all wavefronts from a workgroup hits barrier", func() {
		wg := new(WorkGroup)
		for i := 0; i < 3; i++ {
			wf := NewWavefront(kernels.NewWavefront())
			wf.State = WfAtBarrier
			wf.WG = wg
			wg.Wfs = append(wg.Wfs, wf)
			scheduler.barrierBuffer = append(scheduler.barrierBuffer, wf)
		}

		wf := wg.Wfs[0]
		wf.State = WfRunning
		wf.inst = NewInst(insts.NewInst())
		wf.inst.Format = insts.FormatTable[insts.SOPP]
		wf.inst.Opcode = 10
		wg.Wfs = append(wg.Wfs, wf)

		scheduler.internalExecuting = wf
		scheduler.EvaluateInternalInst(10)

		Expect(scheduler.internalExecuting).To(BeNil())
		Expect(len(scheduler.barrierBuffer)).To(Equal(0))
		for i := 0; i < 4; i++ {
			wf := wg.Wfs[i]
			Expect(wf.State).To(Equal(WfReady))
		}

	})
})
