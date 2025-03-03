package cu

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/v4/insts"
	"github.com/sarchlab/mgpusim/v4/kernels"
	"github.com/sarchlab/mgpusim/v4/protocol"
	"github.com/sarchlab/mgpusim/v4/timing/wavefront"
)

type mockWfArbitor struct {
	wfsToReturn [][]*wavefront.Wavefront
}

func newMockWfArbitor() *mockWfArbitor {
	a := new(mockWfArbitor)
	a.wfsToReturn = make([][]*wavefront.Wavefront, 0)
	return a
}

func (m *mockWfArbitor) Arbitrate([]*WavefrontPool) []*wavefront.Wavefront {
	if len(m.wfsToReturn) == 0 {
		return nil
	}
	wfs := m.wfsToReturn[0]
	m.wfsToReturn = m.wfsToReturn[1:]
	return wfs
}

type mockCUComponent struct {
	canAccept    bool
	isIdle       bool
	acceptedWave []*wavefront.Wavefront
}

func (c *mockCUComponent) CanAcceptWave() bool {
	return c.canAccept
}

func (c *mockCUComponent) AcceptWave(wave *wavefront.Wavefront) {
	c.acceptedWave = append(c.acceptedWave, wave)
}

func (c *mockCUComponent) Run() bool {
	return true
}

func (c *mockCUComponent) IsIdle() bool {
	return c.isIdle
}

func (c *mockCUComponent) Flush() {

}

var _ = Describe("Scheduler", func() {
	var (
		mockCtrl         *gomock.Controller
		engine           *MockEngine
		cu               *ComputeUnit
		branchUnit       *mockCUComponent
		ldsDecoder       *mockCUComponent
		vectorMemDecoder *mockCUComponent
		vectorDecoder    *mockCUComponent
		scalarDecoder    *mockCUComponent
		scheduler        *SchedulerImpl
		fetchArbitor     *mockWfArbitor
		issueArbitor     *mockWfArbitor
		instMem          *MockPort
		toInstMem        *MockPort
		toACE            *MockPort
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		engine = NewMockEngine(mockCtrl)
		engine.EXPECT().
			CurrentTime().
			AnyTimes().
			Return(sim.VTimeInSec(0)).
			AnyTimes()

		cu = NewComputeUnit("CU", engine)
		cu.Freq = 1
		cu.WfPools = make([]*WavefrontPool, 1)
		cu.WfPools[0] = NewWavefrontPool(10)

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

		instMem = NewMockPort(mockCtrl)
		cu.InstMem = instMem

		toInstMem = NewMockPort(mockCtrl)
		cu.ToInstMem = toInstMem

		toACE = NewMockPort(mockCtrl)
		cu.ToACE = toACE

		instMem.EXPECT().AsRemote().AnyTimes()
		toInstMem.EXPECT().AsRemote().AnyTimes()
		toACE.EXPECT().AsRemote().AnyTimes()

		fetchArbitor = newMockWfArbitor()
		issueArbitor = newMockWfArbitor()
		scheduler = NewScheduler(cu, fetchArbitor, issueArbitor)
	})

	It("should always fetch 64 bytes", func() {
		wf := new(wavefront.Wavefront)
		wf.Wavefront = new(kernels.Wavefront)
		wf.InstBufferStartPC = 0x100
		wf.InstBuffer = make([]byte, 0x80)

		fetchArbitor.wfsToReturn = append(fetchArbitor.wfsToReturn,
			[]*wavefront.Wavefront{wf})

		toInstMem.EXPECT().Send(gomock.Any()).Do(func(r sim.Msg) {
			req := r.(*mem.ReadReq)
			Expect(req.Src).To(BeIdenticalTo(cu.ToInstMem.AsRemote()))
			Expect(req.Dst).To(BeIdenticalTo(instMem.AsRemote()))
			Expect(req.Address).To(Equal(uint64(0x180)))
			Expect(req.AccessByteSize).To(Equal(uint64(64)))
		})

		scheduler.DoFetch()

		Expect(cu.InFlightInstFetch).To(HaveLen(1))
		Expect(wf.IsFetching).To(BeTrue())
	})

	It("should wait if fetch failed", func() {
		wf := new(wavefront.Wavefront)
		wf.InstBufferStartPC = 0x100
		wf.InstBuffer = make([]byte, 0x80)
		fetchArbitor.wfsToReturn = append(fetchArbitor.wfsToReturn,
			[]*wavefront.Wavefront{wf})

		toInstMem.EXPECT().Send(gomock.Any()).Do(func(r sim.Msg) {
			req := r.(*mem.ReadReq)
			Expect(req.Src).To(BeIdenticalTo(cu.ToInstMem.AsRemote()))
			Expect(req.Dst).To(BeIdenticalTo(instMem.AsRemote()))
			Expect(req.Address).To(Equal(uint64(0x180)))
			Expect(req.AccessByteSize).To(Equal(uint64(64)))
		}).Return(&sim.SendError{})

		scheduler.DoFetch()

		//Expect(cu.inFlightMemAccess).To(HaveLen(0))
		Expect(wf.IsFetching).To(BeFalse())
	})

	It("should issue", func() {
		wfs := make([]*wavefront.Wavefront, 0)
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
			wf := new(wavefront.Wavefront)
			wf.Wavefront = kernels.NewWavefront()
			wf.PC = 0x120
			wf.InstBuffer = make([]byte, 256)
			wf.InstBufferStartPC = 0x100
			wf.State = wavefront.WfReady
			wf.InstToIssue = wavefront.NewInst(insts.NewInst())
			wf.InstToIssue.ExeUnit = issueDirs[i]
			wf.InstToIssue.ByteSize = 4
			wfs = append(wfs, wf)
		}
		wfs[0].PC = 0x13C
		issueArbitor.wfsToReturn = append(issueArbitor.wfsToReturn, wfs)

		scheduler.DoIssue()

		Expect(len(branchUnit.acceptedWave)).To(Equal(1))
		Expect(len(ldsDecoder.acceptedWave)).To(Equal(1))
		Expect(len(vectorDecoder.acceptedWave)).To(Equal(1))
		Expect(len(vectorMemDecoder.acceptedWave)).To(Equal(1))
		Expect(len(scalarDecoder.acceptedWave)).To(Equal(0))

		Expect(wfs[0].State).To(Equal(wavefront.WfRunning))
		Expect(wfs[1].State).To(Equal(wavefront.WfRunning))
		Expect(wfs[2].State).To(Equal(wavefront.WfRunning))
		Expect(wfs[3].State).To(Equal(wavefront.WfRunning))
		Expect(wfs[4].State).To(Equal(wavefront.WfReady))

		Expect(wfs[0].InstToIssue).To(BeNil())
		Expect(wfs[1].InstToIssue).To(BeNil())
		Expect(wfs[2].InstToIssue).To(BeNil())
		Expect(wfs[3].InstToIssue).To(BeNil())
		Expect(wfs[4].InstToIssue).NotTo(BeNil())
	})

	It("should issue internal instruction", func() {
		wfs := make([]*wavefront.Wavefront, 0)
		wf := new(wavefront.Wavefront)
		wf.Wavefront = kernels.NewWavefront()
		wf.InstToIssue = wavefront.NewInst(insts.NewInst())
		wf.InstToIssue.ExeUnit = insts.ExeUnitSpecial
		wf.InstToIssue.ByteSize = 4
		wf.PC = 10
		wf.State = wavefront.WfReady
		wfs = append(wfs, wf)

		issueArbitor.wfsToReturn = append(issueArbitor.wfsToReturn, wfs)
		scheduler.internalExecuting = nil

		scheduler.DoIssue()

		Expect(scheduler.internalExecuting).To(ContainElement(wf))
		Expect(wf.State).To(Equal(wavefront.WfRunning))
		Expect(wf.PC).To(Equal(uint64(10)))
		Expect(wf.InstToIssue).To(BeNil())
	})

	It("should wait for memory access when running wait_cnt", func() {
		wf := new(wavefront.Wavefront)
		wf.SetDynamicInst(wavefront.NewInst(insts.NewInst()))
		wf.DynamicInst().Format = insts.FormatTable[insts.SOPP]
		wf.DynamicInst().Opcode = 12 // WAIT_CNT
		wf.DynamicInst().LKGMCNT = 0
		wf.State = wavefront.WfRunning
		wf.OutstandingScalarMemAccess = 1

		scheduler.internalExecuting = []*wavefront.Wavefront{wf}
		scheduler.EvaluateInternalInst()

		Expect(scheduler.internalExecuting).To(ContainElement(wf))
		Expect(wf.State).To(Equal(wavefront.WfRunning))
	})

	It("should wait for memory access when running wait_cnt", func() {
		wf := new(wavefront.Wavefront)
		wf.SetDynamicInst(wavefront.NewInst(insts.NewInst()))
		wf.DynamicInst().Format = insts.FormatTable[insts.SOPP]
		wf.DynamicInst().Opcode = 12 // WAIT_CNT
		wf.DynamicInst().VMCNT = 0
		wf.State = wavefront.WfRunning
		wf.OutstandingVectorMemAccess = 1

		scheduler.internalExecuting = []*wavefront.Wavefront{wf}
		scheduler.EvaluateInternalInst()

		Expect(scheduler.internalExecuting).To(ContainElement(wf))
		Expect(wf.State).To(Equal(wavefront.WfRunning))
	})

	It("should pass if memory returns when running wait_cnt", func() {
		wf := new(wavefront.Wavefront)
		wf.SetDynamicInst(wavefront.NewInst(insts.NewInst()))
		wf.DynamicInst().Format = insts.FormatTable[insts.SOPP]
		wf.DynamicInst().Opcode = 12 // WAIT_CNT
		wf.DynamicInst().LKGMCNT = 0
		wf.DynamicInst().VMCNT = 0
		wf.State = wavefront.WfRunning
		wf.OutstandingScalarMemAccess = 0
		wf.OutstandingVectorMemAccess = 0

		scheduler.internalExecuting = []*wavefront.Wavefront{wf}
		scheduler.EvaluateInternalInst()

		Expect(scheduler.internalExecuting).NotTo(ContainElement(wf))
		Expect(wf.State).To(Equal(wavefront.WfReady))
	})

	Context("when running END_PGM", func() {
		It("should not terminate wavefront if there are pending memory requests", func() {
			wf := new(wavefront.Wavefront)
			wf.SetDynamicInst(wavefront.NewInst(insts.NewInst()))
			wf.DynamicInst().Format = insts.FormatTable[insts.SOPP]
			wf.DynamicInst().Opcode = 1 // S_ENDPGM
			wf.State = wavefront.WfRunning
			wf.OutstandingScalarMemAccess = 1
			wf.OutstandingVectorMemAccess = 1

			scheduler.internalExecuting = []*wavefront.Wavefront{wf}
			scheduler.EvaluateInternalInst()

			Expect(scheduler.internalExecuting).NotTo(BeNil())
		})

		It("should be marked completed if other wfs are still "+
			"running", func() {
			wg := new(wavefront.WorkGroup)
			co := new(insts.HsaCo)
			co.HsaCoHeader = new(insts.HsaCoHeader)
			for i := 0; i < 3; i++ {
				wf := wavefront.NewWavefront(kernels.NewWavefront())
				wf.CodeObject = co
				wf.SetDynamicInst(wavefront.NewInst(insts.NewInst()))
				wf.DynamicInst().Format = insts.FormatTable[insts.SOPP]
				wf.DynamicInst().Opcode = 10
				wf.State = wavefront.WfRunning
				wf.WG = wg
				wg.Wfs = append(wg.Wfs, wf)
				scheduler.barrierBuffer = append(
					scheduler.barrierBuffer, wf)
			}

			wf := wavefront.NewWavefront(kernels.NewWavefront())
			wf.CodeObject = co
			wf.SetDynamicInst(wavefront.NewInst(insts.NewInst()))
			wf.DynamicInst().Format = insts.FormatTable[insts.SOPP]
			wf.DynamicInst().Opcode = 1 // S_ENDPGM
			wf.WG = wg
			wf.State = wavefront.WfRunning
			wf.OutstandingScalarMemAccess = 0
			wf.OutstandingVectorMemAccess = 0
			wg.Wfs = append(wg.Wfs, wf)

			scheduler.internalExecuting = []*wavefront.Wavefront{wf}
			scheduler.EvaluateInternalInst()

			Expect(scheduler.internalExecuting).To(HaveLen(0))
			for i := 0; i < 3; i++ {
				wf := wg.Wfs[i]
				Expect(wf.State).To(Equal(wavefront.WfRunning))
			}
			Expect(wg.Wfs[3].State).To(Equal(wavefront.WfCompleted))
		})

		Context("all other wavefronts are at barrier", func() {
			It("should pass barrier", func() {
				wg := new(wavefront.WorkGroup)
				co := new(insts.HsaCo)
				co.HsaCoHeader = new(insts.HsaCoHeader)
				for i := 0; i < 3; i++ {
					wf := wavefront.NewWavefront(kernels.NewWavefront())
					wf.CodeObject = co
					wf.SetDynamicInst(wavefront.NewInst(insts.NewInst()))
					wf.DynamicInst().Format = insts.FormatTable[insts.SOPP]
					wf.DynamicInst().Opcode = 10
					wf.State = wavefront.WfAtBarrier
					wf.WG = wg
					wg.Wfs = append(wg.Wfs, wf)
					scheduler.barrierBuffer = append(
						scheduler.barrierBuffer, wf)
				}

				wf := wavefront.NewWavefront(kernels.NewWavefront())
				wf.CodeObject = co
				wf.SetDynamicInst(wavefront.NewInst(insts.NewInst()))
				wf.DynamicInst().Format = insts.FormatTable[insts.SOPP]
				wf.DynamicInst().Opcode = 1 // S_ENDPGM
				wf.WG = wg
				wf.State = wavefront.WfRunning
				wf.OutstandingScalarMemAccess = 0
				wf.OutstandingVectorMemAccess = 0
				wg.Wfs = append(wg.Wfs, wf)

				scheduler.internalExecuting = []*wavefront.Wavefront{wf}
				scheduler.EvaluateInternalInst()

				Expect(scheduler.internalExecuting).To(HaveLen(0))
				for i := 0; i < 3; i++ {
					wf := wg.Wfs[i]
					Expect(wf.State).To(Equal(wavefront.WfReady))
				}
				Expect(wg.Wfs[3].State).To(Equal(wavefront.WfCompleted))
			})
		})

		Context("all other wavefronts completed", func() {
			It("should wait if cannot send msg", func() {
				toACE.EXPECT().Send(gomock.Any()).Return(sim.NewSendError())

				wg := new(wavefront.WorkGroup)
				mapReq := protocol.MapWGReqBuilder{}.Build()
				wg.MapReq = mapReq
				co := new(insts.HsaCo)
				co.HsaCoHeader = new(insts.HsaCoHeader)
				for i := 0; i < 3; i++ {
					wf := wavefront.NewWavefront(kernels.NewWavefront())
					wf.CodeObject = co
					wf.State = wavefront.WfCompleted
					wg.Wfs = append(wg.Wfs, wf)
				}

				wf := wavefront.NewWavefront(kernels.NewWavefront())
				wf.CodeObject = co
				wf.SetDynamicInst(wavefront.NewInst(insts.NewInst()))
				wf.DynamicInst().Format = insts.FormatTable[insts.SOPP]
				wf.DynamicInst().Opcode = 1 // S_ENDPGM
				wf.WG = wg
				wf.State = wavefront.WfRunning
				wf.OutstandingScalarMemAccess = 0
				wf.OutstandingVectorMemAccess = 0
				wg.Wfs = append(wg.Wfs, wf)

				scheduler.internalExecuting = []*wavefront.Wavefront{wf}
				scheduler.EvaluateInternalInst()

				Expect(scheduler.internalExecuting).To(ContainElement(wf))
				for i := 0; i < 3; i++ {
					wf := wg.Wfs[i]
					Expect(wf.State).To(Equal(wavefront.WfCompleted))
				}
			})

			It("should clear resources", func() {
				toACE.EXPECT().Send(gomock.Any()).Return(nil)

				wg := new(wavefront.WorkGroup)
				mapReq := protocol.MapWGReqBuilder{}.Build()
				wg.MapReq = mapReq
				co := new(insts.HsaCo)
				co.HsaCoHeader = new(insts.HsaCoHeader)
				for i := 0; i < 3; i++ {
					wf := wavefront.NewWavefront(kernels.NewWavefront())
					wf.CodeObject = co
					wf.State = wavefront.WfCompleted
					wg.Wfs = append(wg.Wfs, wf)
					cu.WfPools[0].wfs = append(cu.WfPools[0].wfs, wf)
				}

				wf := wavefront.NewWavefront(kernels.NewWavefront())
				wf.CodeObject = co
				wf.SetDynamicInst(wavefront.NewInst(insts.NewInst()))
				wf.DynamicInst().Format = insts.FormatTable[insts.SOPP]
				wf.DynamicInst().Opcode = 1 // S_ENDPGM
				wf.WG = wg
				wf.State = wavefront.WfRunning
				wf.OutstandingScalarMemAccess = 0
				wf.OutstandingVectorMemAccess = 0
				wg.Wfs = append(wg.Wfs, wf)
				cu.WfPools[0].wfs = append(cu.WfPools[0].wfs, wf)

				scheduler.internalExecuting = []*wavefront.Wavefront{wf}
				scheduler.EvaluateInternalInst()

				Expect(scheduler.internalExecuting).NotTo(ContainElement(wf))
				Expect(cu.WfPools[0].wfs).To(HaveLen(0))
			})
		})
	})

	It("should put wavefront in barrier buffer", func() {
		wg := new(wavefront.WorkGroup)
		for i := 0; i < 4; i++ {
			wf := wavefront.NewWavefront(kernels.NewWavefront())
			wf.State = wavefront.WfRunning
			wf.SetDynamicInst(wavefront.NewInst(insts.NewInst()))
			wf.DynamicInst().Format = insts.FormatTable[insts.SOPP]
			wf.DynamicInst().Opcode = 10
			wf.WG = wg
			wg.Wfs = append(wg.Wfs, wf)
		}

		wf := wg.Wfs[0]

		scheduler.internalExecuting = []*wavefront.Wavefront{wf}
		scheduler.EvaluateInternalInst()

		Expect(wf.State).To(Equal(wavefront.WfAtBarrier))
		Expect(len(scheduler.barrierBuffer)).To(Equal(1))
		Expect(scheduler.barrierBuffer[0]).To(BeIdenticalTo(wf))
		Expect(scheduler.internalExecuting).NotTo(ContainElement(wf))
	})

	It("should continue execution if all wavefronts from a workgroup hits barrier", func() {
		wg := new(wavefront.WorkGroup)
		for i := 0; i < 3; i++ {
			wf := wavefront.NewWavefront(kernels.NewWavefront())
			wf.SetDynamicInst(wavefront.NewInst(insts.NewInst()))
			wf.DynamicInst().Format = insts.FormatTable[insts.SOPP]
			wf.DynamicInst().Opcode = 10
			wf.State = wavefront.WfAtBarrier
			wf.WG = wg
			wg.Wfs = append(wg.Wfs, wf)
			scheduler.barrierBuffer = append(scheduler.barrierBuffer, wf)
		}

		wf := wg.Wfs[0]
		wf.State = wavefront.WfRunning
		wf.SetDynamicInst(wavefront.NewInst(insts.NewInst()))
		wf.DynamicInst().Format = insts.FormatTable[insts.SOPP]
		wf.DynamicInst().Opcode = 10
		wg.Wfs = append(wg.Wfs, wf)

		scheduler.internalExecuting = []*wavefront.Wavefront{wf}
		scheduler.EvaluateInternalInst()

		Expect(scheduler.internalExecuting).NotTo(ContainElement(wf))
		Expect(len(scheduler.barrierBuffer)).To(Equal(0))
		for i := 0; i < 4; i++ {
			wf := wg.Wfs[i]
			Expect(wf.State).To(Equal(wavefront.WfReady))
		}

	})

	It("should flush", func() {
		wg := new(wavefront.WorkGroup)
		for i := 0; i < 4; i++ {
			wf := wavefront.NewWavefront(kernels.NewWavefront())
			wf.State = wavefront.WfRunning
			wf.SetDynamicInst(wavefront.NewInst(insts.NewInst()))
			wf.DynamicInst().Format = insts.FormatTable[insts.SOPP]
			wf.DynamicInst().Opcode = 10
			wf.WG = wg
			wg.Wfs = append(wg.Wfs, wf)
		}
		wf := wg.Wfs[0]

		scheduler.internalExecuting = []*wavefront.Wavefront{wf}
		scheduler.barrierBuffer = append(scheduler.barrierBuffer, wf)

		scheduler.Flush()

		Expect(scheduler.internalExecuting).NotTo(ContainElement(wf))
		Expect(scheduler.barrierBuffer).To(BeNil())

	})
})
