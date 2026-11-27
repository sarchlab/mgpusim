package cu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/emu"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

// jumpingALU moves the PC by a fixed offset, as a taken branch does.
type jumpingALU struct {
	mockALU
	offset uint64
}

func (alu *jumpingALU) Run(wf emu.InstEmuState) {
	wf.SetPC(wf.PC() + alu.offset)
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

	It("should skip a wavefront that is held back", func() {
		engine.now = 4 * period
		waves[0].execution().IssueNotBefore = 8 * period

		Expect(arbiter.Arbitrate(wfPools)).To(BeEmpty())

		engine.now = 8 * period

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
		simd.overlapCycles = 1
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

var _ = Describe("Branch unit with a taken-branch issue gap", func() {
	var (
		engine *fakeEngine
		cu     *ComputeUnit
		bu     *BranchUnit
		alu    *jumpingALU
		wave   *Wavefront
		period timing.VTimeInPicoSec
	)

	BeforeEach(func() {
		engine = newFakeEngine()
		cu = newTestComputeUnit("CU", engine)
		period = cu.comp.Spec.Freq.Period()
		alu = new(jumpingALU)
		bu = NewBranchUnit(cu, alu)
		bu.takenIssueGap = 24

		wave = newTestWave()
		wave.execution().State = wavefront.WfRunning
		wave.execution().InstBuffer = make([]byte, 256)
		wave.execution().InstBufferStartPC = 0x100
		description := insts.NewInst()
		description.FormatType = insts.SOPP
		description.ByteSize = 4
		wave.setTestInst(testInst(testIDs.NewID(), description))
		wave.SetPC(0x13C)
		bu.state().Exec = testWaveID(cu, wave)
		engine.now = 10 * period
	})

	It("should flush and hold back the wavefront after a taken branch", func() {
		alu.offset = ^uint64(0x3f) // back by 64 bytes

		bu.Run()
		bu.Run()

		Expect(wave.execution().State).To(Equal(wavefront.WfReady))
		Expect(wave.PC()).To(Equal(uint64(0x100)))
		Expect(wave.execution().InstBuffer).To(BeEmpty())
		Expect(wave.execution().InstBufferStartPC).To(Equal(uint64(0x100)))
		Expect(wave.execution().IssueNotBefore).To(
			Equal(timing.VTimeInPicoSec(10+24-branchPipelineCycles) * period))
		Expect(cu.issueHeldBack(wave)).To(BeTrue())
	})

	It("should keep the instruction buffer when the branch is not taken", func() {
		bu.Run()
		bu.Run()

		Expect(wave.execution().State).To(Equal(wavefront.WfReady))
		Expect(wave.PC()).To(Equal(uint64(0x140)))
		Expect(wave.execution().InstBuffer).To(HaveLen(192))
		Expect(wave.execution().InstBufferStartPC).To(Equal(uint64(0x140)))
		Expect(wave.execution().IssueNotBefore).To(BeZero())
		Expect(cu.issueHeldBack(wave)).To(BeFalse())
	})

	It("should flush without a hold when the gap is not configured", func() {
		bu.takenIssueGap = 0

		bu.Run()
		bu.Run()

		Expect(wave.PC()).To(Equal(uint64(0x140)))
		Expect(wave.execution().InstBuffer).To(BeEmpty())
		Expect(wave.execution().IssueNotBefore).To(BeZero())
	})
})

var _ = Describe("Scheduler with timed issue", func() {
	var (
		engine        *fakeEngine
		cu            *ComputeUnit
		vectorDecoder *mockCUComponent
		scheduler     *SchedulerImpl
		wave          *Wavefront
		period        timing.VTimeInPicoSec
	)

	BeforeEach(func() {
		engine = newFakeEngine()
		cu = newTestComputeUnit("CU", engine)
		period = cu.comp.Spec.Freq.Period()
		for i := 0; i < 4; i++ {
			cu.WfPools = append(cu.WfPools, NewWavefrontPool(cu, i, 10))
		}
		vectorDecoder = &mockCUComponent{canAccept: true}
		cu.VectorDecoder = vectorDecoder

		scheduler = NewScheduler(cu, newMockWfArbitor(), newMockWfArbitor())
		scheduler.timedIssue = true

		wave = readyWaveWithNext(insts.ExeUnitVALU)
		testAddWave(cu.WfPools[1], wave)
	})

	runUntilIdle := func() int {
		for i := 0; i < 20; i++ {
			if !scheduler.Run() {
				return i
			}
		}
		return 20
	}

	It("should keep ticking while a wavefront waits out a hold", func() {
		wave.execution().IssueNotBefore = 100 * period

		Expect(runUntilIdle()).To(Equal(20))
	})

	It("should keep ticking while a wavefront waits for its SIMD's turn", func() {
		cu.comp.Spec.IssueRotation = true
		engine.now = 2 * period

		Expect(runUntilIdle()).To(Equal(20))
	})

	It("should go idle when the execution unit cannot accept the instruction", func() {
		wave.execution().IssueNotBefore = 100 * period
		vectorDecoder.canAccept = false

		Expect(runUntilIdle()).To(BeNumerically("<", 20))
	})

	It("should go idle when nothing is waiting for a known cycle", func() {
		Expect(runUntilIdle()).To(BeNumerically("<", 20))
	})
})
