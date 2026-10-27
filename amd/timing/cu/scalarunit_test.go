package cu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/mgpusim/v5/amd/emu"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

type mockALU struct {
	wfExecuted emu.InstEmuState
}

func (alu *mockALU) SetLDS(lds []byte) {
}

func (alu *mockALU) LDS() []byte {
	return nil
}

func (alu *mockALU) Run(wf emu.InstEmuState) {
	alu.wfExecuted = wf
}

func (alu *mockALU) ArchName() string {
	return "Mock"
}

// mockRegFileAccessor implements wavefront.RegFileAccessor for tests
type mockRegFileAccessor struct {
	storage map[string][]byte
}

func newMockRegFileAccessor() *mockRegFileAccessor {
	return &mockRegFileAccessor{
		storage: make(map[string][]byte),
	}
}

func regKey(reg *insts.Reg, regCount int, laneID int, waveOffset int) string {
	return reg.Name + ":" + string(rune(regCount)) + ":" +
		string(rune(laneID)) + ":" + string(rune(waveOffset))
}

func (m *mockRegFileAccessor) ReadReg(
	reg *insts.Reg, regCount int, laneID int, waveOffset int,
) []byte {
	key := regKey(reg, regCount, laneID, waveOffset)
	if data, ok := m.storage[key]; ok {
		result := make([]byte, len(data))
		copy(result, data)
		return result
	}
	return make([]byte, reg.ByteSize*regCount)
}

func (m *mockRegFileAccessor) WriteReg(
	reg *insts.Reg, regCount int, laneID int, waveOffset int, data []byte,
) {
	key := regKey(reg, regCount, laneID, waveOffset)
	stored := make([]byte, len(data))
	copy(stored, data)
	m.storage[key] = stored
}

func (m *mockRegFileAccessor) setRegValue(
	reg *insts.Reg, regCount int, laneID int, waveOffset int, data []byte,
) {
	key := regKey(reg, regCount, laneID, waveOffset)
	stored := make([]byte, len(data))
	copy(stored, data)
	m.storage[key] = stored
}

var _ = Describe("Scalar Unit", func() {
	var (
		cu          *ComputeUnit
		bu          *ScalarUnit
		alu         *mockALU
		toScalarMem *fakePort
	)

	BeforeEach(func() {
		cu = newTestComputeUnit("CU", newFakeEngine())
		alu = new(mockALU)
		bu = NewScalarUnit(cu, alu)
		bu.log2CachelineSize = 6

		cu.comp.Spec.ScalarMem = "ScalarMem"

		toScalarMem = newFakePort("CU.ScalarMem")
		cu.ToScalarMem = toScalarMem
	})

	It("should allow accepting wavefront", func() {
		bu.state().Stages.Read = 0
		Expect(bu.CanAcceptWave()).To(BeTrue())
	})

	It("should not allow accepting wavefront is the read stage buffer "+
		"is occupied", func() {
		bu.state().Stages.Read = testWaveID(cu, newTestWave())
		Expect(bu.CanAcceptWave()).To(BeFalse())
	})

	It("should accept wave", func() {
		wave := newTestWave()
		registerTestWave(cu, wave)
		bu.AcceptWave(wave)
		Expect(cu.wave(bu.state().Stages.Read)).To(Equal(wave))
	})

	It("should run", func() {
		wave1 := newTestWave()
		wave2 := newTestWave()
		inst := testInst(testIDs.NewID(), insts.NewInst())
		inst.FormatType = insts.SOP2
		wave2.setTestInst(inst)
		wave3 := newTestWave()
		wave3.setTestInst(inst)
		wave3.execution().State = wavefront.WfRunning

		bu.state().Stages.Read = testWaveID(cu, wave1)
		bu.state().Stages.Exec = testWaveID(cu, wave2)
		bu.state().Stages.Write = testWaveID(cu, wave3)

		bu.Run()

		Expect(wave3.execution().State).To(Equal(wavefront.WfReady))
		Expect(cu.wave(bu.state().Stages.Write)).To(Equal(wave2))
		Expect(cu.wave(bu.state().Stages.Exec)).To(Equal(wave1))
		Expect(cu.wave(bu.state().Stages.Read)).To(BeNil())

		Expect(alu.wfExecuted.(*operationWave).value).To(BeIdenticalTo(wave2.execution()))
	})

	It("should run s_load_dword", func() {
		wave := newTestWave()

		regAccessor := newMockRegFileAccessor()
		setTestAccessor(wave, regAccessor)

		inst := testInst(testIDs.NewID(), insts.NewInst())
		inst.FormatType = insts.SMEM
		inst.Opcode = 0
		inst.Data = insts.NewSRegOperand(0, 0, 1)
		inst.Base = insts.NewSRegOperand(2, 2, 2)
		inst.Offset = insts.NewIntOperand(0, 0x24)
		wave.setTestInst(inst)

		// Set base register value = 0x1000
		baseReg := insts.SReg(2)
		regAccessor.setRegValue(baseReg, 2, 0, wave.execution().SRegOffset,
			insts.Uint64ToBytes(0x1000)[:8])

		bu.state().Stages.Exec = testWaveID(cu, wave)

		bu.Run()

		Expect(wave.execution().State).To(Equal(wavefront.WfReady))
		Expect(wave.execution().OutstandingScalarMemAccess).To(Equal(1))
		Expect(len(cu.comp.State.Scalar)).To(Equal(1))
		Expect(bu.state().Reads).To(HaveLen(1))
	})

	It("should run s_load_dwordx2", func() {
		wave := newTestWave()

		regAccessor := newMockRegFileAccessor()
		setTestAccessor(wave, regAccessor)

		inst := testInst(testIDs.NewID(), insts.NewInst())
		inst.FormatType = insts.SMEM
		inst.Opcode = 1
		inst.Data = insts.NewSRegOperand(0, 0, 1)
		inst.Base = insts.NewSRegOperand(2, 2, 2)
		inst.Offset = insts.NewIntOperand(0, 0x24)
		wave.setTestInst(inst)

		// Set base register value = 0x1000
		baseReg := insts.SReg(2)
		regAccessor.setRegValue(baseReg, 2, 0, wave.execution().SRegOffset,
			insts.Uint64ToBytes(0x1000)[:8])

		bu.state().Stages.Exec = testWaveID(cu, wave)

		bu.Run()

		Expect(wave.execution().State).To(Equal(wavefront.WfReady))
		Expect(wave.execution().OutstandingScalarMemAccess).To(Equal(1))
		Expect(bu.state().Reads).To(HaveLen(1))
	})

	It("should run s_load_dwordx4, access cross cacheline", func() {
		wave := newTestWave()

		regAccessor := newMockRegFileAccessor()
		setTestAccessor(wave, regAccessor)

		inst := testInst(testIDs.NewID(), insts.NewInst())
		inst.FormatType = insts.SMEM
		inst.Opcode = 2
		inst.Data = insts.NewSRegOperand(0, 0, 1)
		inst.Base = insts.NewSRegOperand(2, 2, 2)
		inst.Offset = insts.NewIntOperand(0, 56)
		wave.setTestInst(inst)

		// Set base register value = 0x1000
		baseReg := insts.SReg(2)
		regAccessor.setRegValue(baseReg, 2, 0, wave.execution().SRegOffset,
			insts.Uint64ToBytes(0x1000)[:8])

		bu.state().Stages.Exec = testWaveID(cu, wave)

		start := uint64(0x1000 + 56)
		bu.Run()
		Expect(bu.numCacheline(start, uint64(16))).To(Equal(2))
		Expect(wave.execution().State).To(Equal(wavefront.WfReady))
		Expect(wave.execution().OutstandingScalarMemAccess).To(Equal(1))
		Expect(bu.state().Reads).To(HaveLen(2))
		Expect(bu.state().Reads[0].Payload.(memprotocol.ReadReq).Address).To(Equal(uint64(0x1038)))
		Expect(bu.state().Reads[0].Payload.(memprotocol.ReadReq).AccessByteSize).To(Equal(uint64(8)))
		Expect(bu.state().Reads[1].Payload.(memprotocol.ReadReq).Address).To(Equal(uint64(0x1040)))
		Expect(bu.state().Reads[1].Payload.(memprotocol.ReadReq).AccessByteSize).To(Equal(uint64(8)))
	})

	It("should send request out", func() {
		req := messaging.Msg{ID: testIDs.NewID(),
			Src: cu.ToScalarMem.AsRemote(),
			Dst: cu.comp.Spec.ScalarMem, Payload: memprotocol.ReadReq{Address: 1024,
				AccessByteSize: 4}}

		bu.state().Reads = append(bu.state().Reads, req)

		bu.Run()

		Expect(toScalarMem.sent).To(HaveLen(1))
		sentReq := toScalarMem.sent[0]
		Expect(sentReq.Src).To(Equal(cu.ToScalarMem.AsRemote()))
		Expect(sentReq.Dst).To(
			Equal(messaging.RemotePort("ScalarMem")))
		Expect(sentReq.Payload.(memprotocol.ReadReq).Address).To(Equal(uint64(1024)))
		Expect(sentReq.Payload.(memprotocol.ReadReq).AccessByteSize).To(Equal(uint64(4)))

		Expect(bu.state().Reads).To(HaveLen(0))
	})

	It("should retry if send request failed", func() {
		req := messaging.Msg{ID: testIDs.NewID(),
			Src: cu.ToScalarMem.AsRemote(),
			Dst: cu.comp.Spec.ScalarMem, Payload: memprotocol.ReadReq{Address: 1024,
				AccessByteSize: 4}}

		bu.state().Reads = append(bu.state().Reads, req)

		toScalarMem.full = true

		bu.Run()

		Expect(bu.state().Reads).To(HaveLen(1))
	})

	It("should return correct num of cacheline", func() {
		Expect(bu.numCacheline(0x1038, uint64(80))).To(Equal(3))
	})
})
