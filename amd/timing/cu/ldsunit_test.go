package cu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

var _ = Describe("LDS Unit", func() {
	var (
		cu  *ComputeUnit
		bu  *LDSUnit
		alu *mockALU
	)

	BeforeEach(func() {
		cu = newTestComputeUnit("CU", nil)
		alu = new(mockALU)
		bu = NewLDSUnit(cu, alu)
	})

	It("should allow accepting wavefront", func() {
		// wave := new(Wavefront)
		bu.state().Read = 0
		Expect(bu.CanAcceptWave()).To(BeTrue())
	})

	It("should not allow accepting wavefront is the read stage buffer is occupied", func() {
		bu.state().Read = testWaveID(cu, newTestWave())
		Expect(bu.CanAcceptWave()).To(BeFalse())
	})

	It("should accept wave", func() {
		wave := newTestWave()
		registerTestWave(cu, wave)
		bu.AcceptWave(wave)
		Expect(cu.wave(bu.state().Read)).To(BeIdenticalTo(wave))
	})

	It("should run", func() {
		wave1 := newTestWave()
		wave2 := newTestWave()
		wave2.WG = wavefront.NewWorkGroup(nil, messaging.Msg{Payload: protocol.MapWGReq{}})
		wave2.WG.LDS = make([]byte, 0)
		wave3 := newTestWave()
		inst := wavefront.NewInst(testIDs.NewID(), insts.NewInst())
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

		bu.state().Read = testWaveID(cu, wave1)
		bu.state().Exec = testWaveID(cu, wave2)
		bu.state().Write = testWaveID(cu, wave3)

		bu.Run()

		// wave3 completes write stage
		Expect(wave3.State).To(Equal(wavefront.WfReady))
		Expect(wave3.PC()).To(Equal(uint64(0x140)))
		Expect(wave3.InstBuffer).To(HaveLen(192))

		// wave2: ALU runs, cycleLeft set to 14, stays in toExec
		Expect(cu.wave(bu.state().Exec)).To(BeIdenticalTo(wave2))
		Expect(cu.wave(bu.state().Write)).To(BeNil())
		Expect(alu.wfExecuted).To(BeIdenticalTo(wave2))

		// wave1: can't move from toRead because toExec is occupied
		Expect(cu.wave(bu.state().Read)).To(BeIdenticalTo(wave1))

		// Run 14 more cycles to drain cycleLeft for wave2
		for i := 0; i < 13; i++ {
			bu.Run()
		}

		// After 13 more runs, cycleLeft should be 1, wave2 still in toExec
		Expect(cu.wave(bu.state().Exec)).To(BeIdenticalTo(wave2))
		Expect(cu.wave(bu.state().Write)).To(BeNil())

		// 14th run: cycleLeft reaches 0, wave2 moves to toWrite
		bu.Run()
		Expect(cu.wave(bu.state().Write)).To(BeIdenticalTo(wave2))
		Expect(cu.wave(bu.state().Exec)).To(BeIdenticalTo(wave1))
		Expect(cu.wave(bu.state().Read)).To(BeNil())

	})
	It("should flush the LDS", func() {
		wave1 := newTestWave()
		wave2 := newTestWave()
		wave2.WG = wavefront.NewWorkGroup(nil, messaging.Msg{Payload: protocol.MapWGReq{}})
		wave2.WG.LDS = make([]byte, 0)
		wave3 := newTestWave()
		inst := wavefront.NewInst(testIDs.NewID(), insts.NewInst())
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

		bu.state().Read = testWaveID(cu, wave1)
		bu.state().Exec = testWaveID(cu, wave2)
		bu.state().Write = testWaveID(cu, wave3)

		bu.Flush()

		Expect(cu.wave(bu.state().Read)).To(BeNil())
		Expect(cu.wave(bu.state().Write)).To(BeNil())
		Expect(cu.wave(bu.state().Exec)).To(BeNil())
		Expect(bu.state().Cycles).To(Equal(0))

	})
})
