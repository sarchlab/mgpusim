package timing

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/akita"
	"gitlab.com/akita/akita/mock_akita"
	"gitlab.com/akita/gcn3/emu"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/mem"
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
		mockCtrl    *gomock.Controller
		cu          *ComputeUnit
		sp          *mockScratchpadPreparer
		bu          *ScalarUnit
		alu         *mockALU
		scalarMem   *mock_akita.MockPort
		toScalarMem *mock_akita.MockPort
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		cu = NewComputeUnit("cu", nil)
		sp = new(mockScratchpadPreparer)
		alu = new(mockALU)
		bu = NewScalarUnit(cu, sp, alu)

		scalarMem = mock_akita.NewMockPort(mockCtrl)
		cu.ScalarMem = scalarMem

		toScalarMem = mock_akita.NewMockPort(mockCtrl)
		cu.ToScalarMem = toScalarMem
	})

	AfterEach(func() {
		mockCtrl.Finish()
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
		wave3.inst = inst
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

		//expectedReq := mem.NewReadReq(10, cu, scalarMem, 0x1024, 4)
		//conn.ExpectSend(expectedReq, nil)

		bu.Run(10)

		Expect(wave.State).To(Equal(WfReady))
		Expect(wave.OutstandingScalarMemAccess).To(Equal(1))
		Expect(len(cu.InFlightScalarMemAccess)).To(Equal(1))
		//Expect(conn.AllExpectedSent()).To(BeTrue())
		Expect(bu.readBuf).To(HaveLen(1))
	})

	It("should run s_load_dwordx2", func() {
		wave := NewWavefront(nil)
		bu.toExec = wave

		inst := NewInst(insts.NewInst())
		inst.FormatType = insts.SMEM
		inst.Opcode = 1
		inst.Data = insts.NewSRegOperand(0, 0, 1)
		wave.inst = inst

		sp := wave.scratchpad.AsSMEM()
		sp.Base = 0x1000
		sp.Offset = 0x24

		//expectedReq := mem.NewReadReq(10, cu, scalarMem, 0x1024, 8)
		//conn.ExpectSend(expectedReq, nil)

		bu.Run(10)

		Expect(wave.State).To(Equal(WfReady))
		Expect(wave.OutstandingScalarMemAccess).To(Equal(1))
		//Expect(len(cu.inFlightMemAccess)).To(Equal(1))
		//Expect(conn.AllExpectedSent()).To(BeTrue())
		Expect(bu.readBuf).To(HaveLen(1))
	})

	It("should send request out", func() {
		req := mem.NewReadReq(10, cu.ToScalarMem, scalarMem, 1024, 4)
		bu.readBuf = append(bu.readBuf, req)

		toScalarMem.EXPECT().Send(gomock.Any()).Do(func(r akita.Req) {
			req := r.(*mem.ReadReq)
			Expect(req.Src()).To(BeIdenticalTo(cu.ToScalarMem))
			Expect(req.Dst()).To(BeIdenticalTo(scalarMem))
			Expect(req.Address).To(Equal(uint64(1024)))
			Expect(req.MemByteSize).To(Equal(uint64(4)))
		})

		bu.Run(11)

		Expect(bu.readBuf).To(HaveLen(0))
	})

	It("should retry if send request failed", func() {
		req := mem.NewReadReq(10, cu.ToScalarMem, scalarMem, 1024, 4)
		bu.readBuf = append(bu.readBuf, req)

		toScalarMem.EXPECT().Send(gomock.Any()).Do(func(r akita.Req) {
			req := r.(*mem.ReadReq)
			Expect(req.Src()).To(BeIdenticalTo(cu.ToScalarMem))
			Expect(req.Dst()).To(BeIdenticalTo(scalarMem))
			Expect(req.Address).To(Equal(uint64(1024)))
			Expect(req.MemByteSize).To(Equal(uint64(4)))
		}).Return(&akita.SendError{})

		bu.Run(11)

		Expect(bu.readBuf).To(HaveLen(1))
	})
	It("should flush the scalar unit", func() {
		wave := NewWavefront(nil)
		inst := NewInst(insts.NewInst())
		inst.FormatType = insts.SMEM
		inst.Opcode = 1
		inst.Data = insts.NewSRegOperand(0, 0, 1)
		wave.inst = inst

		bu.toExec = wave
		bu.toWrite = wave
		bu.toRead = wave

		bu.Flush()

		Expect(bu.toRead).To(BeNil())
		Expect(bu.toWrite).To(BeNil())
		Expect(bu.toExec).To(BeNil())

	})
})
