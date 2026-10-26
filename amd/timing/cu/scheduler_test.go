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
	flushed      bool
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
	c.flushed = true
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
		wf.Wavefront = new(kernels.Wavefront)
		wf.InstBufferStartPC = 0x100
		wf.InstBuffer = make([]byte, 0x80)

		fetchArbitor.wfsToReturn = append(fetchArbitor.wfsToReturn,
			[]*wavefront.Wavefront{wf})

		registerTestWave(cu, wf)
		scheduler.DoFetch()

		Expect(toInstMem.sent).To(HaveLen(1))
		req := toInstMem.sent[0]
		Expect(req.Src).To(BeIdenticalTo(cu.ToInstMem.AsRemote()))
		Expect(req.Dst).To(BeIdenticalTo(messaging.RemotePort("InstMem")))
		Expect(req.Payload.(memprotocol.ReadReq).Address).To(Equal(uint64(0x180)))
		Expect(req.Payload.(memprotocol.ReadReq).AccessByteSize).To(Equal(uint64(64)))

		Expect(cu.comp.State.InstFetch).To(HaveLen(1))
		Expect(wf.IsFetching).To(BeTrue())
	})

	It("should wait if fetch failed", func() {
		wf := newTestWave()
		wf.InstBufferStartPC = 0x100
		wf.InstBuffer = make([]byte, 0x80)
		fetchArbitor.wfsToReturn = append(fetchArbitor.wfsToReturn,
			[]*wavefront.Wavefront{wf})

		toInstMem.full = true

		registerTestWave(cu, wf)
		scheduler.DoFetch()

		Expect(toInstMem.sent).To(HaveLen(0))
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
			wf := newTestWave()
			wf.Wavefront = kernels.NewWavefront(testIDs.NewID())
			wf.SetPC(0x120)
			wf.InstBuffer = make([]byte, 256)
			wf.InstBufferStartPC = 0x100
			wf.State = wavefront.WfReady
			wf.SetNextInst(wavefront.NewInst(testIDs.NewID(), insts.NewInst()))
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

		Expect(wfs[0].State).To(Equal(wavefront.WfRunning))
		Expect(wfs[1].State).To(Equal(wavefront.WfRunning))
		Expect(wfs[2].State).To(Equal(wavefront.WfRunning))
		Expect(wfs[3].State).To(Equal(wavefront.WfRunning))
		Expect(wfs[4].State).To(Equal(wavefront.WfReady))

		Expect(wfs[0].NextInst()).To(BeNil())
		Expect(wfs[1].NextInst()).To(BeNil())
		Expect(wfs[2].NextInst()).To(BeNil())
		Expect(wfs[3].NextInst()).To(BeNil())
		Expect(wfs[4].NextInst()).NotTo(BeNil())
	})

	It("should issue internal instruction", func() {
		wfs := make([]*wavefront.Wavefront, 0)
		wf := newTestWave()
		wf.Wavefront = kernels.NewWavefront(testIDs.NewID())
		wf.SetNextInst(wavefront.NewInst(testIDs.NewID(), insts.NewInst()))
		wf.NextInst().ExeUnit = insts.ExeUnitSpecial
		wf.NextInst().ByteSize = 4
		wf.SetPC(10)
		wf.State = wavefront.WfReady
		wfs = append(wfs, wf)

		issueArbitor.wfsToReturn = append(issueArbitor.wfsToReturn, wfs)
		scheduler.state().Internal = nil

		for _, batch := range issueArbitor.wfsToReturn {
			for _, w := range batch {
				registerTestWave(cu, w)
			}
		}
		scheduler.DoIssue()

		Expect(scheduler.state().Internal).To(ContainElement(wf.UID))
		Expect(wf.State).To(Equal(wavefront.WfRunning))
		Expect(wf.PC()).To(Equal(uint64(10)))
		Expect(wf.NextInst()).To(BeNil())
	})

	It("should wait for memory access when running wait_cnt", func() {
		wf := newTestWave()
		wf.SetDynamicInst(wavefront.NewInst(testIDs.NewID(), insts.NewInst()))
		wf.DynamicInst().Format = insts.FormatTable[insts.SOPP]
		wf.DynamicInst().Opcode = 12 // WAIT_CNT
		wf.DynamicInst().LKGMCNT = 0
		wf.State = wavefront.WfRunning
		wf.OutstandingScalarMemAccess = 1

		scheduler.state().Internal = []uint64{testWaveID(cu, wf)}
		scheduler.EvaluateInternalInst()

		Expect(scheduler.state().Internal).To(ContainElement(wf.UID))
		Expect(wf.State).To(Equal(wavefront.WfRunning))
	})

	It("should wait for memory access when running wait_cnt", func() {
		wf := newTestWave()
		wf.SetDynamicInst(wavefront.NewInst(testIDs.NewID(), insts.NewInst()))
		wf.DynamicInst().Format = insts.FormatTable[insts.SOPP]
		wf.DynamicInst().Opcode = 12 // WAIT_CNT
		wf.DynamicInst().VMCNT = 0
		wf.State = wavefront.WfRunning
		wf.OutstandingVectorMemAccess = 1

		scheduler.state().Internal = []uint64{testWaveID(cu, wf)}
		scheduler.EvaluateInternalInst()

		Expect(scheduler.state().Internal).To(ContainElement(wf.UID))
		Expect(wf.State).To(Equal(wavefront.WfRunning))
	})

	It("should pass if memory returns when running wait_cnt", func() {
		wf := newTestWave()
		wf.SetDynamicInst(wavefront.NewInst(testIDs.NewID(), insts.NewInst()))
		wf.DynamicInst().Format = insts.FormatTable[insts.SOPP]
		wf.DynamicInst().Opcode = 12 // WAIT_CNT
		wf.DynamicInst().LKGMCNT = 0
		wf.DynamicInst().VMCNT = 0
		wf.State = wavefront.WfRunning
		wf.OutstandingScalarMemAccess = 0
		wf.OutstandingVectorMemAccess = 0

		scheduler.state().Internal = []uint64{testWaveID(cu, wf)}
		scheduler.EvaluateInternalInst()

		Expect(scheduler.state().Internal).NotTo(ContainElement(wf.UID))
		Expect(wf.State).To(Equal(wavefront.WfReady))
	})

	Context("when running END_PGM", func() {
		It("should not terminate wavefront if there are pending memory "+
			"requests", func() {
			wf := newTestWave()
			wf.SetDynamicInst(wavefront.NewInst(testIDs.NewID(), insts.NewInst()))
			wf.DynamicInst().Format = insts.FormatTable[insts.SOPP]
			wf.DynamicInst().Opcode = 1 // S_ENDPGM
			wf.State = wavefront.WfRunning
			wf.OutstandingScalarMemAccess = 1
			wf.OutstandingVectorMemAccess = 1

			scheduler.state().Internal = []uint64{testWaveID(cu, wf)}
			scheduler.EvaluateInternalInst()

			Expect(scheduler.state().Internal).NotTo(BeNil())
		})

		It("should be marked completed if other wfs are still "+
			"running", func() {
			wg := wavefront.NewWorkGroup(kernels.NewWorkGroup(testIDs.NewID()), messaging.Msg{})
			co := &insts.KernelCodeObject{
				KernelCodeObjectMeta: &insts.KernelCodeObjectMeta{},
			}
			for i := 0; i < 3; i++ {
				wf := wavefront.NewWavefront(kernels.NewWavefront(testIDs.NewID()))
				wf.CodeObject = co
				wf.SetDynamicInst(wavefront.NewInst(testIDs.NewID(), insts.NewInst()))
				wf.DynamicInst().Format = insts.FormatTable[insts.SOPP]
				wf.DynamicInst().Opcode = 10
				wf.State = wavefront.WfRunning
				wf.WG = wg
				wg.Wfs = append(wg.Wfs, wf)
				scheduler.state().Barrier = append(
					scheduler.state().Barrier, testWaveID(cu, wf))
			}

			wf := wavefront.NewWavefront(kernels.NewWavefront(testIDs.NewID()))
			wf.CodeObject = co
			wf.SetDynamicInst(wavefront.NewInst(testIDs.NewID(), insts.NewInst()))
			wf.DynamicInst().Format = insts.FormatTable[insts.SOPP]
			wf.DynamicInst().Opcode = 1 // S_ENDPGM
			wf.WG = wg
			wf.State = wavefront.WfRunning
			wf.OutstandingScalarMemAccess = 0
			wf.OutstandingVectorMemAccess = 0
			wg.Wfs = append(wg.Wfs, wf)

			scheduler.state().Internal = []uint64{testWaveID(cu, wf)}
			scheduler.EvaluateInternalInst()

			Expect(scheduler.state().Internal).To(HaveLen(0))
			for i := 0; i < 3; i++ {
				wf := wg.Wfs[i]
				Expect(wf.State).To(Equal(wavefront.WfRunning))
			}
			Expect(wg.Wfs[3].State).To(Equal(wavefront.WfCompleted))
		})

		Context("all other wavefronts are at barrier", func() {
			It("should pass barrier", func() {
				wg := wavefront.NewWorkGroup(kernels.NewWorkGroup(testIDs.NewID()), messaging.Msg{})
				co := &insts.KernelCodeObject{
					KernelCodeObjectMeta: &insts.KernelCodeObjectMeta{},
				}
				for i := 0; i < 3; i++ {
					wf := wavefront.NewWavefront(kernels.NewWavefront(testIDs.NewID()))
					wf.CodeObject = co
					wf.SetDynamicInst(wavefront.NewInst(testIDs.NewID(), insts.NewInst()))
					wf.DynamicInst().Format = insts.FormatTable[insts.SOPP]
					wf.DynamicInst().Opcode = 10
					wf.State = wavefront.WfAtBarrier
					wf.WG = wg
					wg.Wfs = append(wg.Wfs, wf)
					scheduler.state().Barrier = append(
						scheduler.state().Barrier, testWaveID(cu, wf))
				}

				wf := wavefront.NewWavefront(kernels.NewWavefront(testIDs.NewID()))
				wf.CodeObject = co
				wf.SetDynamicInst(wavefront.NewInst(testIDs.NewID(), insts.NewInst()))
				wf.DynamicInst().Format = insts.FormatTable[insts.SOPP]
				wf.DynamicInst().Opcode = 1 // S_ENDPGM
				wf.WG = wg
				wf.State = wavefront.WfRunning
				wf.OutstandingScalarMemAccess = 0
				wf.OutstandingVectorMemAccess = 0
				wg.Wfs = append(wg.Wfs, wf)

				scheduler.state().Internal = []uint64{testWaveID(cu, wf)}
				scheduler.EvaluateInternalInst()

				Expect(scheduler.state().Internal).To(HaveLen(0))
				for i := 0; i < 3; i++ {
					wf := wg.Wfs[i]
					Expect(wf.State).To(Equal(wavefront.WfReady))
				}
				Expect(wg.Wfs[3].State).To(Equal(wavefront.WfCompleted))
			})
		})

		Context("all other wavefronts completed", func() {
			It("should wait if cannot send msg", func() {
				toACE.full = true

				wg := wavefront.NewWorkGroup(kernels.NewWorkGroup(testIDs.NewID()), messaging.Msg{})
				mapReq := messaging.Msg{ID: testIDs.NewID(), Payload: protocol.MapWGReq{}}

				wg.MapReq = mapReq
				co := &insts.KernelCodeObject{
					KernelCodeObjectMeta: &insts.KernelCodeObjectMeta{},
				}
				for i := 0; i < 3; i++ {
					wf := wavefront.NewWavefront(kernels.NewWavefront(testIDs.NewID()))
					wf.CodeObject = co
					wf.State = wavefront.WfCompleted
					wg.Wfs = append(wg.Wfs, wf)
				}

				wf := wavefront.NewWavefront(kernels.NewWavefront(testIDs.NewID()))
				wf.CodeObject = co
				wf.SetDynamicInst(wavefront.NewInst(testIDs.NewID(), insts.NewInst()))
				wf.DynamicInst().Format = insts.FormatTable[insts.SOPP]
				wf.DynamicInst().Opcode = 1 // S_ENDPGM
				wf.WG = wg
				wf.State = wavefront.WfRunning
				wf.OutstandingScalarMemAccess = 0
				wf.OutstandingVectorMemAccess = 0
				wg.Wfs = append(wg.Wfs, wf)

				scheduler.state().Internal = []uint64{testWaveID(cu, wf)}
				scheduler.EvaluateInternalInst()

				Expect(scheduler.state().Internal).To(ContainElement(wf.UID))
				for i := 0; i < 3; i++ {
					wf := wg.Wfs[i]
					Expect(wf.State).To(Equal(wavefront.WfCompleted))
				}
			})

			It("should clear resources", func() {
				wg := wavefront.NewWorkGroup(kernels.NewWorkGroup(testIDs.NewID()), messaging.Msg{})
				mapReq := messaging.Msg{ID: testIDs.NewID(), Payload: protocol.MapWGReq{}}

				wg.MapReq = mapReq
				co := &insts.KernelCodeObject{
					KernelCodeObjectMeta: &insts.KernelCodeObjectMeta{},
				}
				for i := 0; i < 3; i++ {
					wf := wavefront.NewWavefront(kernels.NewWavefront(testIDs.NewID()))
					wf.CodeObject = co
					wf.State = wavefront.WfCompleted
					wg.Wfs = append(wg.Wfs, wf)
					testAddWave(cu.WfPools[0], wf)
				}

				wf := wavefront.NewWavefront(kernels.NewWavefront(testIDs.NewID()))
				wf.CodeObject = co
				wf.SetDynamicInst(wavefront.NewInst(testIDs.NewID(), insts.NewInst()))
				wf.DynamicInst().Format = insts.FormatTable[insts.SOPP]
				wf.DynamicInst().Opcode = 1 // S_ENDPGM
				wf.WG = wg
				wf.State = wavefront.WfRunning
				wf.OutstandingScalarMemAccess = 0
				wf.OutstandingVectorMemAccess = 0
				wg.Wfs = append(wg.Wfs, wf)
				testAddWave(cu.WfPools[0], wf)

				scheduler.state().Internal = []uint64{testWaveID(cu, wf)}
				scheduler.EvaluateInternalInst()

				Expect(toACE.sent).To(HaveLen(1))
				Expect(scheduler.state().Internal).NotTo(ContainElement(wf.UID))
				Expect(cu.WfPools[0].waves()).To(HaveLen(0))
			})
		})
	})

	It("should put wavefront in barrier buffer", func() {
		wg := wavefront.NewWorkGroup(kernels.NewWorkGroup(testIDs.NewID()), messaging.Msg{})
		for i := 0; i < 4; i++ {
			wf := wavefront.NewWavefront(kernels.NewWavefront(testIDs.NewID()))
			wf.State = wavefront.WfRunning
			wf.SetDynamicInst(wavefront.NewInst(testIDs.NewID(), insts.NewInst()))
			wf.DynamicInst().Format = insts.FormatTable[insts.SOPP]
			wf.DynamicInst().Opcode = 10
			wf.WG = wg
			wg.Wfs = append(wg.Wfs, wf)
		}

		wf := wg.Wfs[0]

		scheduler.state().Internal = []uint64{testWaveID(cu, wf)}
		scheduler.EvaluateInternalInst()

		Expect(wf.State).To(Equal(wavefront.WfAtBarrier))
		Expect(len(scheduler.state().Barrier)).To(Equal(1))
		Expect(cu.wave(scheduler.state().Barrier[0])).To(BeIdenticalTo(wf))
		Expect(scheduler.state().Internal).NotTo(ContainElement(wf.UID))
	})

	It("should continue execution if all wavefronts from a workgroup hits "+
		"barrier", func() {
		wg := wavefront.NewWorkGroup(kernels.NewWorkGroup(testIDs.NewID()), messaging.Msg{})
		for i := 0; i < 3; i++ {
			wf := wavefront.NewWavefront(kernels.NewWavefront(testIDs.NewID()))
			wf.SetDynamicInst(wavefront.NewInst(testIDs.NewID(), insts.NewInst()))
			wf.DynamicInst().Format = insts.FormatTable[insts.SOPP]
			wf.DynamicInst().Opcode = 10
			wf.State = wavefront.WfAtBarrier
			wf.WG = wg
			wg.Wfs = append(wg.Wfs, wf)
			scheduler.state().Barrier = append(scheduler.state().Barrier, testWaveID(cu, wf))
		}

		wf := wg.Wfs[0]
		wf.State = wavefront.WfRunning
		wf.SetDynamicInst(wavefront.NewInst(testIDs.NewID(), insts.NewInst()))
		wf.DynamicInst().Format = insts.FormatTable[insts.SOPP]
		wf.DynamicInst().Opcode = 10
		wg.Wfs = append(wg.Wfs, wf)

		scheduler.state().Internal = []uint64{testWaveID(cu, wf)}
		scheduler.EvaluateInternalInst()

		Expect(scheduler.state().Internal).NotTo(ContainElement(wf.UID))
		Expect(len(scheduler.state().Barrier)).To(Equal(0))
		for i := 0; i < 4; i++ {
			wf := wg.Wfs[i]
			Expect(wf.State).To(Equal(wavefront.WfReady))
		}

	})

	It("should flush", func() {
		wg := wavefront.NewWorkGroup(kernels.NewWorkGroup(testIDs.NewID()), messaging.Msg{})
		for i := 0; i < 4; i++ {
			wf := wavefront.NewWavefront(kernels.NewWavefront(testIDs.NewID()))
			wf.State = wavefront.WfRunning
			wf.SetDynamicInst(wavefront.NewInst(testIDs.NewID(), insts.NewInst()))
			wf.DynamicInst().Format = insts.FormatTable[insts.SOPP]
			wf.DynamicInst().Opcode = 10
			wf.WG = wg
			wg.Wfs = append(wg.Wfs, wf)
		}
		wf := wg.Wfs[0]

		scheduler.state().Internal = []uint64{testWaveID(cu, wf)}
		scheduler.state().Barrier = append(scheduler.state().Barrier, testWaveID(cu, wf))

		scheduler.Flush()

		Expect(scheduler.state().Internal).NotTo(ContainElement(wf.UID))
		Expect(scheduler.state().Barrier).To(BeNil())

	})
})
