package timing

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/emu"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/mem"
)

type mockScratchpadPreparer struct {
	wfPrepared  *Wavefront
	wfCommitted *Wavefront
}

func (sp *mockScratchpadPreparer) Prepare(
	instEmuState emu.InstEmuState,
	wf *Wavefront,
) {
	sp.wfPrepared = wf
}

func (sp *mockScratchpadPreparer) Commit(
	instEmuState emu.InstEmuState,
	wf *Wavefront,
) {
	sp.wfCommitted = wf
}

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

var _ = Describe("Scalar Unit", func() {

	var (
		cu        *ComputeUnit
		sp        *mockScratchpadPreparer
		bu        *ScalarUnit
		alu       *mockALU
		scalarMem *core.MockComponent
		conn      *core.MockConnection
	)

	BeforeEach(func() {
		cu = NewComputeUnit("cu", nil)
		sp = new(mockScratchpadPreparer)
		alu = new(mockALU)
		bu = NewScalarUnit(cu, sp, alu)
		scalarMem = core.NewMockComponent("ScalarMem")
		conn = core.NewMockConnection()

		cu.ScalarMem = scalarMem
		core.PlugIn(cu, "ToScalarMem", conn)
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

	It("should run s_load_dword", func() {
		wave := NewWavefront(nil)
		bu.toExec = wave

		inst := NewInst(insts.NewInst())
		inst.FormatType = insts.SMEM
		inst.Opcode = 0
		inst.Data = insts.NewSRegOperand(0, 0, 1)
		wave.inst = inst

		sp := wave.scratchpad.AsSMEM()
		sp.Base = 0x1000
		sp.Offset = 0x24

		expectedReq := mem.NewAccessReq()
		expectedReq.Address = 0x1024
		expectedReq.Type = mem.Read
		expectedReq.ByteSize = 4
		expectedReq.SetSendTime(10)
		expectedReq.SetDst(scalarMem)
		expectedReq.SetSrc(cu)
		info := new(MemAccessInfo)
		info.Wf = wave
		info.Action = MemAccessScalarDataLoad
		info.Dst = insts.SReg(0)
		info.Inst = inst
		expectedReq.Info = info
		conn.ExpectSend(expectedReq, nil)

		bu.Run(10)

		Expect(conn.AllExpectedSent()).To(BeTrue())

	})
})
