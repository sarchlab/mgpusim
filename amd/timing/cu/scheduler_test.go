package cu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
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
	isIdle       bool
	acceptedWave []*Wavefront
}

func (c *mockCUComponent) CanAcceptWave() bool {
	return c.canAccept
}

func (c *mockCUComponent) AcceptWave(wave *Wavefront) {
	c.acceptedWave = append(c.acceptedWave, wave)
}

func (c *mockCUComponent) Run() bool {
	return true
}

func (c *mockCUComponent) IsIdle() bool {
	return c.isIdle
}

var _ = Describe("Scheduler", func() {
	var (
		engine           *fakeEngine
		cu               *ComputeUnit
		branchUnit       *mockCUComponent
		ldsDecoder       *mockCUComponent
		vectorMemDecoder *mockCUComponent
		vectorDecoder    *mockCUComponent
		scalarDecoder    *mockCUComponent
		scheduler        *SchedulerImpl
		fetchArbitor     *mockWfArbitor
		issueArbitor     *mockWfArbitor
		toInstMem        *fakePort
		toACE            *fakePort
	)

	BeforeEach(func() {
		engine = newFakeEngine()

		cu = newTestComputeUnit("CU", engine)
		cu.WfPools = make([]*WavefrontPool, 1)
		cu.WfPools[0] = NewWavefrontPool(cu, 0, 10)

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

		cu.comp.Spec.InstMem = "InstMem"

		toInstMem = newFakePort("CU.InstMem")
		cu.ToInstMem = toInstMem

		toACE = newFakePort("CU.Top")
		cu.ToACE = toACE

		fetchArbitor = newMockWfArbitor()
		issueArbitor = newMockWfArbitor()
		scheduler = NewScheduler(cu, fetchArbitor, issueArbitor)
	})

	It("should always fetch 64 bytes", func() {
		wf := newTestWave()

		wf.execution().InstBufferStartPC = 0x100
		wf.execution().InstBuffer = make([]byte, 0x80)

		fetchArbitor.wfsToReturn = append(fetchArbitor.wfsToReturn,
			[]*Wavefront{wf})

		registerTestWave(cu, wf)
		scheduler.DoFetch()

		Expect(toInstMem.sent).To(HaveLen(1))
		req := toInstMem.sent[0]
		Expect(req.Src).To(Equal(cu.ToInstMem.AsRemote()))
		Expect(req.Dst).To(Equal(messaging.RemotePort("InstMem")))
		Expect(req.Payload.(memprotocol.ReadReq).Address).To(Equal(uint64(0x180)))
		Expect(req.Payload.(memprotocol.ReadReq).AccessByteSize).To(Equal(uint64(64)))

		Expect(cu.comp.State.InstFetch).To(HaveLen(1))
		Expect(wf.execution().IsFetching).To(BeTrue())
	})

	It("should wait if fetch failed", func() {
		wf := newTestWave()
		wf.execution().InstBufferStartPC = 0x100
		wf.execution().InstBuffer = make([]byte, 0x80)
		fetchArbitor.wfsToReturn = append(fetchArbitor.wfsToReturn,
			[]*Wavefront{wf})

		toInstMem.full = true

		registerTestWave(cu, wf)
		scheduler.DoFetch()

		Expect(toInstMem.sent).To(HaveLen(0))
		Expect(wf.execution().IsFetching).To(BeFalse())
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
			wf := newTestWave()

			wf.SetPC(0x120)
			wf.execution().InstBuffer = make([]byte, 256)
			wf.execution().InstBufferStartPC = 0x100
			wf.execution().State = wavefront.WfReady
			wf.SetNextInst(testInst(testIDs.NewID(), insts.NewInst()))
			wf.NextInst().ExeUnit = issueDirs[i]
			wf.NextInst().ByteSize = 4
			wfs = append(wfs, wf)
		}
		wfs[0].SetPC(0x13C)
		issueArbitor.wfsToReturn = append(issueArbitor.wfsToReturn, wfs)

		for _, batch := range issueArbitor.wfsToReturn {
			for _, w := range batch {
				registerTestWave(cu, w)
			}
		}
		scheduler.DoIssue()

		Expect(len(branchUnit.acceptedWave)).To(Equal(1))
		Expect(len(ldsDecoder.acceptedWave)).To(Equal(1))
		Expect(len(vectorDecoder.acceptedWave)).To(Equal(1))
		Expect(len(vectorMemDecoder.acceptedWave)).To(Equal(1))
		Expect(len(scalarDecoder.acceptedWave)).To(Equal(0))

		Expect(wfs[0].execution().State).To(Equal(wavefront.WfRunning))
		Expect(wfs[1].execution().State).To(Equal(wavefront.WfRunning))
		Expect(wfs[2].execution().State).To(Equal(wavefront.WfRunning))
		Expect(wfs[3].execution().State).To(Equal(wavefront.WfRunning))
		Expect(wfs[4].execution().State).To(Equal(wavefront.WfReady))

		Expect(wfs[0].NextInst()).To(BeNil())
		Expect(wfs[1].NextInst()).To(BeNil())
		Expect(wfs[2].NextInst()).To(BeNil())
		Expect(wfs[3].NextInst()).To(BeNil())
		Expect(wfs[4].NextInst()).NotTo(BeNil())
	})

	It("should issue internal instruction", func() {
		wfs := make([]*Wavefront, 0)
		wf := newTestWave()

		wf.SetNextInst(testInst(testIDs.NewID(), insts.NewInst()))
		wf.NextInst().ExeUnit = insts.ExeUnitSpecial
		wf.NextInst().ByteSize = 4
		wf.SetPC(10)
		wf.execution().State = wavefront.WfReady
		wfs = append(wfs, wf)

		issueArbitor.wfsToReturn = append(issueArbitor.wfsToReturn, wfs)
		scheduler.state().Internal = nil

		for _, batch := range issueArbitor.wfsToReturn {
			for _, w := range batch {
				registerTestWave(cu, w)
			}
		}
		scheduler.DoIssue()

		Expect(scheduler.state().Internal).To(ContainElement(wfID(wf)))
		Expect(wf.execution().State).To(Equal(wavefront.WfRunning))
		Expect(wf.PC()).To(Equal(uint64(10)))
		Expect(wf.NextInst()).To(BeNil())
	})

	It("should wait for memory access when running wait_cnt", func() {
		wf := newTestWave()
		wf.SetDynamicInst(testInst(testIDs.NewID(), insts.NewInst()))
		wf.DynamicInst().Format = insts.FormatTable[insts.SOPP]
		wf.DynamicInst().Opcode = 12 // WAIT_CNT
		wf.DynamicInst().LKGMCNT = 0
		wf.execution().State = wavefront.WfRunning
		wf.execution().OutstandingScalarMemAccess = 1

		scheduler.state().Internal = []uint64{testWaveID(cu, wf)}
		scheduler.EvaluateInternalInst()

		Expect(scheduler.state().Internal).To(ContainElement(wfID(wf)))
		Expect(wf.execution().State).To(Equal(wavefront.WfRunning))
	})

	It("should wait for memory access when running wait_cnt", func() {
		wf := newTestWave()
		wf.SetDynamicInst(testInst(testIDs.NewID(), insts.NewInst()))
		wf.DynamicInst().Format = insts.FormatTable[insts.SOPP]
		wf.DynamicInst().Opcode = 12 // WAIT_CNT
		wf.DynamicInst().VMCNT = 0
		wf.execution().State = wavefront.WfRunning
		wf.execution().OutstandingVectorMemAccess = 1

		scheduler.state().Internal = []uint64{testWaveID(cu, wf)}
		scheduler.EvaluateInternalInst()

		Expect(scheduler.state().Internal).To(ContainElement(wfID(wf)))
		Expect(wf.execution().State).To(Equal(wavefront.WfRunning))
	})

	It("should pass if memory returns when running wait_cnt", func() {
		wf := newTestWave()
		wf.SetDynamicInst(testInst(testIDs.NewID(), insts.NewInst()))
		wf.DynamicInst().Format = insts.FormatTable[insts.SOPP]
		wf.DynamicInst().Opcode = 12 // WAIT_CNT
		wf.DynamicInst().LKGMCNT = 0
		wf.DynamicInst().VMCNT = 0
		wf.execution().State = wavefront.WfRunning
		wf.execution().OutstandingScalarMemAccess = 0
		wf.execution().OutstandingVectorMemAccess = 0

		scheduler.state().Internal = []uint64{testWaveID(cu, wf)}
		scheduler.EvaluateInternalInst()

		Expect(scheduler.state().Internal).NotTo(ContainElement(wfID(wf)))
		Expect(wf.execution().State).To(Equal(wavefront.WfReady))
	})

	Context("when running END_PGM", func() {
		It("should not terminate wavefront if there are pending memory "+
			"requests", func() {
			wf := newTestWave()
			wf.SetDynamicInst(testInst(testIDs.NewID(), insts.NewInst()))
			wf.DynamicInst().Format = insts.FormatTable[insts.SOPP]
			wf.DynamicInst().Opcode = 1 // S_ENDPGM
			wf.execution().State = wavefront.WfRunning
			wf.execution().OutstandingScalarMemAccess = 1
			wf.execution().OutstandingVectorMemAccess = 1

			scheduler.state().Internal = []uint64{testWaveID(cu, wf)}
			scheduler.EvaluateInternalInst()

			Expect(scheduler.state().Internal).NotTo(BeNil())
		})

		It("should be marked completed if other wfs are still "+
			"running", func() {
			wg := newTestRawGroup(kernels.NewWorkGroup(testIDs.NewID()), messaging.Msg{})
			co := &insts.KernelCodeObject{
				KernelCodeObjectMeta: &insts.KernelCodeObjectMeta{},
			}
			for i := 0; i < 3; i++ {
				wf := newTestRawWave(kernels.NewWavefront(testIDs.NewID()))
				setTestCodeObject(wf.group(), co)
				wf.SetDynamicInst(testInst(testIDs.NewID(), insts.NewInst()))
				wf.DynamicInst().Format = insts.FormatTable[insts.SOPP]
				wf.DynamicInst().Opcode = 10
				wf.execution().State = wavefront.WfRunning
				setTestGroup(wf, wg)
				setTestWaves(wg, append(wg.waves(), wf))
				scheduler.state().Barrier = append(
					scheduler.state().Barrier, testWaveID(cu, wf))
			}

			wf := newTestRawWave(kernels.NewWavefront(testIDs.NewID()))
			setTestCodeObject(wf.group(), co)
			wf.SetDynamicInst(testInst(testIDs.NewID(), insts.NewInst()))
			wf.DynamicInst().Format = insts.FormatTable[insts.SOPP]
			wf.DynamicInst().Opcode = 1 // S_ENDPGM
			setTestGroup(wf, wg)
			wf.execution().State = wavefront.WfRunning
			wf.execution().OutstandingScalarMemAccess = 0
			wf.execution().OutstandingVectorMemAccess = 0
			setTestWaves(wg, append(wg.waves(), wf))

			scheduler.state().Internal = []uint64{testWaveID(cu, wf)}
			scheduler.EvaluateInternalInst()

			Expect(scheduler.state().Internal).To(HaveLen(0))
			for i := 0; i < 3; i++ {
				wf := wg.waves()[i]
				Expect(wf.execution().State).To(Equal(wavefront.WfRunning))
			}
			Expect(wg.waves()[3].execution().State).To(Equal(wavefront.WfCompleted))
		})

		Context("all other wavefronts are at barrier", func() {
			It("should pass barrier", func() {
				wg := newTestRawGroup(kernels.NewWorkGroup(testIDs.NewID()), messaging.Msg{})
				co := &insts.KernelCodeObject{
					KernelCodeObjectMeta: &insts.KernelCodeObjectMeta{},
				}
				for i := 0; i < 3; i++ {
					wf := newTestRawWave(kernels.NewWavefront(testIDs.NewID()))
					setTestCodeObject(wf.group(), co)
					wf.SetDynamicInst(testInst(testIDs.NewID(), insts.NewInst()))
					wf.DynamicInst().Format = insts.FormatTable[insts.SOPP]
					wf.DynamicInst().Opcode = 10
					wf.execution().State = wavefront.WfAtBarrier
					setTestGroup(wf, wg)
					setTestWaves(wg, append(wg.waves(), wf))
					scheduler.state().Barrier = append(
						scheduler.state().Barrier, testWaveID(cu, wf))
				}

				wf := newTestRawWave(kernels.NewWavefront(testIDs.NewID()))
				setTestCodeObject(wf.group(), co)
				wf.SetDynamicInst(testInst(testIDs.NewID(), insts.NewInst()))
				wf.DynamicInst().Format = insts.FormatTable[insts.SOPP]
				wf.DynamicInst().Opcode = 1 // S_ENDPGM
				setTestGroup(wf, wg)
				wf.execution().State = wavefront.WfRunning
				wf.execution().OutstandingScalarMemAccess = 0
				wf.execution().OutstandingVectorMemAccess = 0
				setTestWaves(wg, append(wg.waves(), wf))

				scheduler.state().Internal = []uint64{testWaveID(cu, wf)}
				scheduler.EvaluateInternalInst()

				Expect(scheduler.state().Internal).To(HaveLen(0))
				for i := 0; i < 3; i++ {
					wf := wg.waves()[i]
					Expect(wf.execution().State).To(Equal(wavefront.WfReady))
				}
				Expect(wg.waves()[3].execution().State).To(Equal(wavefront.WfCompleted))
			})
		})

		Context("all other wavefronts completed", func() {
			It("should wait if cannot send msg", func() {
				toACE.full = true

				wg := newTestRawGroup(kernels.NewWorkGroup(testIDs.NewID()), messaging.Msg{})
				mapReq := messaging.Msg{ID: testIDs.NewID(), Payload: protocol.MapWGReq{}}

				setTestRequest(wg, mapReq)
				co := &insts.KernelCodeObject{
					KernelCodeObjectMeta: &insts.KernelCodeObjectMeta{},
				}
				for i := 0; i < 3; i++ {
					wf := newTestRawWave(kernels.NewWavefront(testIDs.NewID()))
					setTestCodeObject(wf.group(), co)
					wf.execution().State = wavefront.WfCompleted
					setTestWaves(wg, append(wg.waves(), wf))
				}

				wf := newTestRawWave(kernels.NewWavefront(testIDs.NewID()))
				setTestCodeObject(wf.group(), co)
				wf.SetDynamicInst(testInst(testIDs.NewID(), insts.NewInst()))
				wf.DynamicInst().Format = insts.FormatTable[insts.SOPP]
				wf.DynamicInst().Opcode = 1 // S_ENDPGM
				setTestGroup(wf, wg)
				wf.execution().State = wavefront.WfRunning
				wf.execution().OutstandingScalarMemAccess = 0
				wf.execution().OutstandingVectorMemAccess = 0
				setTestWaves(wg, append(wg.waves(), wf))

				scheduler.state().Internal = []uint64{testWaveID(cu, wf)}
				scheduler.EvaluateInternalInst()

				cu.Scheduler = scheduler
				cu.retireGroups()
				Expect(scheduler.state().Internal).To(BeEmpty())
				Expect(toACE.sent).To(BeEmpty())
				Expect(wf.execution().State).To(Equal(wavefront.WfCompleted))
				for i := 0; i < 3; i++ {
					wf := wg.waves()[i]
					Expect(wf.execution().State).To(Equal(wavefront.WfCompleted))
				}
			})

			It("should clear resources", func() {
				wg := newTestRawGroup(kernels.NewWorkGroup(testIDs.NewID()), messaging.Msg{})
				mapReq := messaging.Msg{ID: testIDs.NewID(), Payload: protocol.MapWGReq{}}

				setTestRequest(wg, mapReq)
				co := &insts.KernelCodeObject{
					KernelCodeObjectMeta: &insts.KernelCodeObjectMeta{},
				}
				for i := 0; i < 3; i++ {
					wf := newTestRawWave(kernels.NewWavefront(testIDs.NewID()))
					setTestCodeObject(wf.group(), co)
					wf.execution().State = wavefront.WfCompleted
					setTestWaves(wg, append(wg.waves(), wf))
					testAddWave(cu.WfPools[0], wf)
				}

				wf := newTestRawWave(kernels.NewWavefront(testIDs.NewID()))
				setTestCodeObject(wf.group(), co)
				wf.SetDynamicInst(testInst(testIDs.NewID(), insts.NewInst()))
				wf.DynamicInst().Format = insts.FormatTable[insts.SOPP]
				wf.DynamicInst().Opcode = 1 // S_ENDPGM
				setTestGroup(wf, wg)
				wf.execution().State = wavefront.WfRunning
				wf.execution().OutstandingScalarMemAccess = 0
				wf.execution().OutstandingVectorMemAccess = 0
				setTestWaves(wg, append(wg.waves(), wf))
				testAddWave(cu.WfPools[0], wf)

				scheduler.state().Internal = []uint64{testWaveID(cu, wf)}
				scheduler.EvaluateInternalInst()

				cu.Scheduler = scheduler
				cu.retireGroups()
				Expect(toACE.sent).To(HaveLen(1))
				Expect(scheduler.state().Internal).NotTo(ContainElement(wfID(wf)))
				Expect(cu.WfPools[0].waves()).To(HaveLen(0))
			})
		})
	})

	It("should put wavefront in barrier buffer", func() {
		wg := newTestRawGroup(kernels.NewWorkGroup(testIDs.NewID()), messaging.Msg{})
		for i := 0; i < 4; i++ {
			wf := newTestRawWave(kernels.NewWavefront(testIDs.NewID()))
			wf.execution().State = wavefront.WfRunning
			wf.SetDynamicInst(testInst(testIDs.NewID(), insts.NewInst()))
			wf.DynamicInst().Format = insts.FormatTable[insts.SOPP]
			wf.DynamicInst().Opcode = 10
			setTestGroup(wf, wg)
			setTestWaves(wg, append(wg.waves(), wf))
		}

		wf := wg.waves()[0]

		scheduler.state().Internal = []uint64{testWaveID(cu, wf)}
		scheduler.EvaluateInternalInst()

		Expect(wf.execution().State).To(Equal(wavefront.WfAtBarrier))
		Expect(len(scheduler.state().Barrier)).To(Equal(1))
		Expect(cu.wave(scheduler.state().Barrier[0])).To(Equal(wf))
		Expect(scheduler.state().Internal).NotTo(ContainElement(wfID(wf)))
	})

	It("should continue execution if all wavefronts from a workgroup hits "+
		"barrier", func() {
		wg := newTestRawGroup(kernels.NewWorkGroup(testIDs.NewID()), messaging.Msg{})
		for i := 0; i < 3; i++ {
			wf := newTestRawWave(kernels.NewWavefront(testIDs.NewID()))
			wf.SetDynamicInst(testInst(testIDs.NewID(), insts.NewInst()))
			wf.DynamicInst().Format = insts.FormatTable[insts.SOPP]
			wf.DynamicInst().Opcode = 10
			wf.execution().State = wavefront.WfAtBarrier
			setTestGroup(wf, wg)
			setTestWaves(wg, append(wg.waves(), wf))
			scheduler.state().Barrier = append(scheduler.state().Barrier, testWaveID(cu, wf))
		}

		wf := wg.waves()[0]
		wf.execution().State = wavefront.WfRunning
		wf.SetDynamicInst(testInst(testIDs.NewID(), insts.NewInst()))
		wf.DynamicInst().Format = insts.FormatTable[insts.SOPP]
		wf.DynamicInst().Opcode = 10
		setTestWaves(wg, append(wg.waves(), wf))

		scheduler.state().Internal = []uint64{testWaveID(cu, wf)}
		scheduler.EvaluateInternalInst()

		Expect(scheduler.state().Internal).NotTo(ContainElement(wfID(wf)))
		Expect(len(scheduler.state().Barrier)).To(Equal(0))
		for _, wf := range wg.waves() {
			Expect(wf.execution().State).To(Equal(wavefront.WfReady))
		}

	})

})
