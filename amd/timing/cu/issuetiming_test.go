package cu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/emu"
	"github.com/sarchlab/mgpusim/v5/amd/emu/cdna3"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

// jumpingALU writes the PC, moved by a fixed offset, as a taken branch does.
// With skip set it leaves the PC alone, as a branch that is not taken does.
type jumpingALU struct {
	mockALU
	offset uint64
	skip   bool
}

func (alu *jumpingALU) Run(wf emu.InstEmuState) {
	if !alu.skip {
		wf.SetPC(wf.PC() + alu.offset)
	}
}

func readyWaveWithNext(unit insts.ExeUnit) *Wavefront {
	wf := newTestWave()
	wf.execution().State = wavefront.WfReady
	description := insts.NewInst()
	description.ExeUnit = unit
	wf.setTestNextInst(testInst(testIDs.NewID(), description))

	return wf
}

var _ = Describe("Issue rotation", func() {
	var (
		engine  *fakeEngine
		cu      *ComputeUnit
		arbiter *IssueArbiter
		wfPools []*WavefrontPool
		waves   []*Wavefront
		period  timing.VTimeInPicoSec
	)

	BeforeEach(func() {
		engine = newFakeEngine()
		cu = newTestComputeUnit("CU", engine)
		period = cu.comp.Spec.Freq.Period()
		arbiter = NewIssueArbiter(cu)
		arbiter.rotation = true

		wfPools = nil
		waves = nil
		for i := 0; i < 4; i++ {
			pool := NewWavefrontPool(cu, i, 10)
			wf := readyWaveWithNext(insts.ExeUnitVALU)
			testAddWave(pool, wf)
			wfPools = append(wfPools, pool)
			waves = append(waves, wf)
		}
	})

	It("should serve one SIMD per cycle, in turn", func() {
		for cycle := 0; cycle < 8; cycle++ {
			engine.now = timing.VTimeInPicoSec(cycle) * period

			Expect(arbiter.Arbitrate(wfPools)).To(Equal([]*Wavefront{waves[cycle%4]}))
		}
	})

	It("should serve one wavefront per execution unit in the served SIMD", func() {
		scalar := readyWaveWithNext(insts.ExeUnitScalar)
		secondVALU := readyWaveWithNext(insts.ExeUnitVALU)
		testAddWave(wfPools[2], scalar)
		testAddWave(wfPools[2], secondVALU)
		engine.now = 2 * period

		Expect(arbiter.Arbitrate(wfPools)).To(Equal([]*Wavefront{waves[2], scalar}))
	})

	It("should hold back a branch on SCC after a scalar ALU instruction", func() {
		arbiter.scoreboardEnabled = true
		description := insts.NewInst()
		description.ExeUnit = insts.ExeUnitBranch
		description.FormatType = insts.SOPP
		description.InstName = "s_cbranch_scc0"
		waves[0].setTestNextInst(testInst(testIDs.NewID(), description))
		waves[0].state().HasScoreboard = true
		waves[0].scoreboard().MarkSCCBranchBusy(8)
		engine.now = 4 * period

		Expect(arbiter.Arbitrate(wfPools)).To(BeEmpty())

		for i := 0; i < 8; i++ {
			waves[0].scoreboard().Tick()
		}

		Expect(arbiter.Arbitrate(wfPools)).To(Equal([]*Wavefront{waves[0]}))
	})

	It("should not hold back other branches after a scalar ALU instruction", func() {
		arbiter.scoreboardEnabled = true
		description := insts.NewInst()
		description.ExeUnit = insts.ExeUnitBranch
		description.FormatType = insts.SOPP
		description.InstName = "s_cbranch_vccz"
		waves[0].setTestNextInst(testInst(testIDs.NewID(), description))
		waves[0].state().HasScoreboard = true
		waves[0].scoreboard().MarkSCCBranchBusy(8)
		engine.now = 4 * period

		Expect(arbiter.Arbitrate(wfPools)).To(Equal([]*Wavefront{waves[0]}))
	})
})

var _ = Describe("Pipelined SIMD unit", func() {
	var (
		cu   *ComputeUnit
		simd *SIMDUnit
		alu  *mockALU
		wave *Wavefront
		id   uint64
	)

	BeforeEach(func() {
		cu = newTestComputeUnit("CU", nil)
		alu = new(mockALU)
		simd = NewSIMDUnit(cu, "simd", alu)
		simd.scoreboardEnabled = true
		simd.pipelineCapacity = 1

		wave = newTestWave()
		description := insts.NewInst()
		description.InstName = "v_fma_f64"
		description.ByteSize = 8
		id = testIDs.NewID()
		wave.setTestInst(testInst(id, description))
		wave.execution().State = wavefront.WfRunning
		wave.execution().InstBuffer = make([]byte, 64)
		wave.execution().InstBufferStartPC = 0x100
		wave.SetPC(0x100)
		registerTestWave(cu, wave)
		cu.beginInstruction(wave, id)
	})

	It("should take twice as long for f64 instructions by default", func() {
		simd.AcceptWave(wave)

		Expect(simd.state().Slots[0].Cycles).To(Equal(8))
	})

	It("should take the same time for f64 instructions with uniform cycles", func() {
		simd.uniformCycles = true

		simd.AcceptWave(wave)

		Expect(simd.state().Slots[0].Cycles).To(Equal(4))
	})

	It("should let the wavefront issue again before the instruction leaves", func() {
		simd.uniformCycles = true
		simd.issueOverlap = true
		simd.AcceptWave(wave)

		simd.Run()
		simd.Run()

		Expect(wave.execution().State).To(Equal(wavefront.WfRunning))
		Expect(alu.wfExecuted).To(BeNil())

		simd.Run()

		Expect(wave.execution().State).To(Equal(wavefront.WfReady))
		Expect(wave.PC()).To(Equal(uint64(0x108)))
		Expect(alu.wfExecuted).NotTo(BeNil())
		Expect(simd.CanAcceptWave()).To(BeFalse())
		Expect(cu.comp.State.InstTasks).To(HaveKey(id))

		next := testIDs.NewID()
		wave.setTestInst(testInst(next, insts.NewInst()))
		cu.beginInstruction(wave, next)
		alu.wfExecuted = nil

		simd.Run()

		Expect(simd.CanAcceptWave()).To(BeTrue())
		Expect(alu.wfExecuted).To(BeNil())
		Expect(cu.comp.State.InstTasks).NotTo(HaveKey(id))
		Expect(cu.comp.State.InstTasks).To(HaveKey(next))
		Expect(wave.execution().InFlightInsts).To(Equal(1))
	})

	It("should release the wavefront only at the end without overlap", func() {
		simd.uniformCycles = true
		simd.AcceptWave(wave)

		simd.Run()
		simd.Run()
		simd.Run()

		Expect(wave.execution().State).To(Equal(wavefront.WfRunning))

		simd.Run()

		Expect(wave.execution().State).To(Equal(wavefront.WfReady))
		Expect(simd.CanAcceptWave()).To(BeTrue())
		Expect(cu.comp.State.InstTasks).NotTo(HaveKey(id))
	})
})

var _ = Describe("Branch unit with block fetch", func() {
	var (
		engine *fakeEngine
		cu     *ComputeUnit
		bu     *BranchUnit
		alu    *jumpingALU
		wave   *Wavefront
		period timing.VTimeInPicoSec
	)

	// branchInst is a SOPP branch with the given opcode and displacement.
	branchInst := func(opcode insts.Opcode, displacement int64) *testInstruction {
		description := insts.NewInst()
		description.FormatType = insts.SOPP
		description.Opcode = opcode
		description.SImm16 = insts.NewIntOperand(0, displacement)
		description.ByteSize = 4

		return testInst(testIDs.NewID(), description)
	}

	BeforeEach(func() {
		engine = newFakeEngine()
		cu = newTestComputeUnit("CU", engine)
		cu.comp.Spec.InstBlockByteSize = 32
		period = cu.comp.Spec.Freq.Period()
		alu = new(jumpingALU)
		bu = NewBranchUnit(cu, alu)

		wave = newTestWave()
		wave.execution().State = wavefront.WfRunning
		wave.execution().InstBuffer = make([]byte, 256)
		wave.execution().InstBufferStartPC = 0x100
		wave.execution().NextFetchPC = 0x200
		wave.execution().IssuedEnd = 0x140
		wave.execution().PrefetchPC = 0x280
		wave.execution().BlockFetchNotBefore = 20 * period
		wave.execution().SIMDID = 1
		wave.setTestInst(branchInst(2, 0))
		wave.SetPC(0x13C)
		bu.state().Exec = testWaveID(cu, wave)
		engine.now = 10 * period
	})

	It("should start a new fetch stream after a taken branch, without a hold", func() {
		alu.offset = ^uint64(0x3b) // back by 60 bytes, to 0x104

		bu.Run()
		bu.Run()

		state := wave.execution()
		Expect(state.State).To(Equal(wavefront.WfReady))
		Expect(wave.PC()).To(Equal(uint64(0x104)))
		Expect(state.InstBuffer).To(BeEmpty())
		Expect(state.InstBufferStartPC).To(Equal(uint64(0x100)))
		Expect(state.FetchEpoch).To(Equal(uint64(1)))
		Expect(state.NextFetchPC).To(BeZero())
		Expect(state.IssuedEnd).To(BeZero())
		Expect(state.PrefetchPC).To(BeZero())
		Expect(state.BlockFetchNotBefore).To(BeZero())
		Expect(engine.scheduled).To(BeEmpty())
	})

	It("should keep the instruction buffer when the branch is not taken", func() {
		alu.skip = true

		bu.Run()
		bu.Run()

		state := wave.execution()
		Expect(state.State).To(Equal(wavefront.WfReady))
		Expect(wave.PC()).To(Equal(uint64(0x140)))
		Expect(state.InstBuffer).To(HaveLen(192))
		Expect(state.InstBufferStartPC).To(Equal(uint64(0x140)))
		Expect(state.FetchEpoch).To(BeZero())
		Expect(state.NextFetchPC).To(Equal(uint64(0x200)))
		Expect(engine.scheduled).To(BeEmpty())
	})

	It("should flush on every branch without block fetch", func() {
		cu.comp.Spec.InstBlockByteSize = 0
		alu.skip = true

		bu.Run()
		bu.Run()

		Expect(wave.PC()).To(Equal(uint64(0x140)))
		Expect(wave.execution().InstBuffer).To(BeEmpty())
		Expect(wave.execution().FetchEpoch).To(Equal(uint64(1)))
	})

	Context("with the CDNA3 ALU and a zero displacement", func() {
		BeforeEach(func() {
			bu.alu = cdna3.NewALU(nil)
		})

		expectTaken := func() {
			Expect(wave.PC()).To(Equal(uint64(0x140)))
			Expect(wave.execution().InstBuffer).To(BeEmpty())
			Expect(wave.execution().FetchEpoch).To(Equal(uint64(1)))
		}

		expectNotTaken := func() {
			Expect(wave.PC()).To(Equal(uint64(0x140)))
			Expect(wave.execution().InstBuffer).To(HaveLen(192))
			Expect(wave.execution().FetchEpoch).To(BeZero())
		}

		It("should treat an unconditional branch as taken", func() {
			wave.setTestInst(branchInst(2, 0)) // s_branch 0

			bu.Run()
			bu.Run()

			expectTaken()
		})

		It("should treat a conditional branch whose condition holds as taken", func() {
			wave.setTestInst(branchInst(4, 0)) // s_cbranch_scc0 0
			wave.SetSCC(0)

			bu.Run()
			bu.Run()

			expectTaken()
		})

		It("should treat a conditional branch whose condition fails as not taken", func() {
			wave.setTestInst(branchInst(4, 0)) // s_cbranch_scc0 0
			wave.SetSCC(1)

			bu.Run()
			bu.Run()

			expectNotTaken()
		})
	})
})

var _ = Describe("Scheduler with issue rotation", func() {
	var (
		engine        *fakeEngine
		cu            *ComputeUnit
		vectorDecoder *mockCUComponent
		scheduler     *SchedulerImpl
		wave          *Wavefront
		period        timing.VTimeInPicoSec
	)

	wakeTimes := func() []timing.VTimeInPicoSec {
		times := make([]timing.VTimeInPicoSec, 0, len(engine.scheduled))
		for _, evt := range engine.scheduled {
			times = append(times, evt.Time())
		}
		return times
	}

	runUntilIdle := func() int {
		for i := 0; i < 20; i++ {
			if !scheduler.Run() {
				return i
			}
		}
		return 20
	}

	BeforeEach(func() {
		engine = newFakeEngine()
		cu = newTestComputeUnit("CU", engine)
		cu.comp.Spec.IssueRotation = true
		period = cu.comp.Spec.Freq.Period()
		for i := 0; i < numWfPools; i++ {
			cu.WfPools = append(cu.WfPools, NewWavefrontPool(cu, i, 10))
		}
		vectorDecoder = &mockCUComponent{canAccept: true}
		cu.VectorDecoder = vectorDecoder

		scheduler = NewScheduler(cu, newMockWfArbitor(), newMockWfArbitor())
		scheduler.issueRotation = true

		wave = readyWaveWithNext(insts.ExeUnitVALU)
		testAddWave(cu.WfPools[1], wave)
	})

	It("should wake the compute unit at the next turn of a waiting SIMD", func() {
		engine.now = 2 * period

		scheduler.Run()

		Expect(wakeTimes()).To(Equal([]timing.VTimeInPicoSec{5 * period}))
	})

	It("should not report progress while an instruction waits for its turn", func() {
		engine.now = 2 * period

		Expect(runUntilIdle()).To(BeNumerically("<", 20))
	})

	It("should not wake for the SIMD that is served in this cycle", func() {
		engine.now = 1 * period

		scheduler.Run()

		Expect(wakeTimes()).To(BeEmpty())
	})

	It("should not wake when the execution unit cannot accept the instruction", func() {
		engine.now = 2 * period
		vectorDecoder.canAccept = false

		scheduler.Run()

		Expect(wakeTimes()).To(BeEmpty())
	})

	It("should not wake without issue rotation", func() {
		scheduler.issueRotation = false
		engine.now = 2 * period

		scheduler.Run()

		Expect(wakeTimes()).To(BeEmpty())
	})
})
