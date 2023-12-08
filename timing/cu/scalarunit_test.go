package cu

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v3/mem/mem"
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/emu"
	"github.com/sarchlab/mgpusim/v3/insts"
	"github.com/sarchlab/mgpusim/v3/timing/wavefront"
)

type mockScratchpadPreparer struct {
	wfPrepared  *wavefront.Wavefront
	wfCommitted *wavefront.Wavefront
}

func (sp *mockScratchpadPreparer) Prepare(
	instEmuState emu.InstEmuState,
	wf *wavefront.Wavefront,
) {
	sp.wfPrepared = wf
}

func (sp *mockScratchpadPreparer) Commit(
	instEmuState emu.InstEmuState,
	wf *wavefront.Wavefront,
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
		scalarMem   *MockPort
		toScalarMem *MockPort
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		cu = NewComputeUnit("CU", nil)
		sp = new(mockScratchpadPreparer)
		alu = new(mockALU)
		bu = NewScalarUnit(cu, sp, alu)
		bu.log2CachelineSize = 6

		scalarMem = NewMockPort(mockCtrl)
		cu.ScalarMem = scalarMem

		toScalarMem = NewMockPort(mockCtrl)
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
		bu.toRead = new(wavefront.Wavefront)
		Expect(bu.CanAcceptWave()).To(BeFalse())
	})

	It("should accept wave", func() {
		wave := new(wavefront.Wavefront)
		bu.AcceptWave(wave, 10)
		Expect(bu.toRead).To(BeIdenticalTo(wave))
	})

	It("should run", func() {
		wave1 := new(wavefront.Wavefront)
		wave2 := new(wavefront.Wavefront)
		inst := wavefront.NewInst(insts.NewInst())
		inst.FormatType = insts.SOP2
		wave2.SetDynamicInst(inst)
		wave3 := new(wavefront.Wavefront)
		wave3.SetDynamicInst(inst)
		wave3.State = wavefront.WfRunning

		bu.toRead = wave1
		bu.toExec = wave2
		bu.toWrite = wave3

		bu.Run(10)

		Expect(wave3.State).To(Equal(wavefront.WfReady))
		Expect(bu.toWrite).To(BeIdenticalTo(wave2))
		Expect(bu.toExec).To(BeIdenticalTo(wave1))
		Expect(bu.toRead).To(BeNil())

		Expect(sp.wfPrepared).To(BeIdenticalTo(wave1))
		Expect(alu.wfExecuted).To(BeIdenticalTo(wave2))
		Expect(sp.wfCommitted).To(BeIdenticalTo(wave3))
	})

	It("should run s_load_dword", func() {
		wave := wavefront.NewWavefront(nil)
		bu.toExec = wave

		inst := wavefront.NewInst(insts.NewInst())
		inst.FormatType = insts.SMEM
		inst.Opcode = 0
		inst.Data = insts.NewSRegOperand(0, 0, 1)
		wave.SetDynamicInst(inst)

		sp := wave.Scratchpad().AsSMEM()
		sp.Base = 0x1000
		sp.Offset = 0x24

		//expectedReq := mem.NewReadReq(10, cu, scalarMem, 0x1024, 4)
		//conn.ExpectSend(expectedReq, nil)

		bu.Run(10)

		Expect(wave.State).To(Equal(wavefront.WfReady))
		Expect(wave.OutstandingScalarMemAccess).To(Equal(1))
		Expect(len(cu.InFlightScalarMemAccess)).To(Equal(1))
		//Expect(conn.AllExpectedSent()).To(BeTrue())
		Expect(bu.readBuf).To(HaveLen(1))
	})

	It("should run s_load_dwordx2", func() {
		wave := wavefront.NewWavefront(nil)
		bu.toExec = wave

		inst := wavefront.NewInst(insts.NewInst())
		inst.FormatType = insts.SMEM
		inst.Opcode = 1
		inst.Data = insts.NewSRegOperand(0, 0, 1)
		wave.SetDynamicInst(inst)

		sp := wave.Scratchpad().AsSMEM()
		sp.Base = 0x1000
		sp.Offset = 0x24

		//expectedReq := mem.NewReadReq(10, cu, scalarMem, 0x1024, 8)
		//conn.ExpectSend(expectedReq, nil)

		bu.Run(10)

		Expect(wave.State).To(Equal(wavefront.WfReady))
		Expect(wave.OutstandingScalarMemAccess).To(Equal(1))
		//Expect(len(cu.inFlightMemAccess)).To(Equal(1))
		//Expect(conn.AllExpectedSent()).To(BeTrue())
		Expect(bu.readBuf).To(HaveLen(1))
	})

	It("should run s_load_dwordx4, access cross cacheline", func() {
		wave := wavefront.NewWavefront(nil)
		bu.toExec = wave

		inst := wavefront.NewInst(insts.NewInst())
		inst.FormatType = insts.SMEM
		inst.Opcode = 2
		inst.Data = insts.NewSRegOperand(0, 0, 1)
		wave.SetDynamicInst(inst)

		sp := wave.Scratchpad().AsSMEM()
		sp.Base = 0x1000
		sp.Offset = 56
		start := sp.Base + sp.Offset
		bu.Run(10)
		Expect(bu.numCacheline(start, uint64(16))).To(Equal(2))
		Expect(wave.State).To(Equal(wavefront.WfReady))
		Expect(wave.OutstandingScalarMemAccess).To(Equal(1))
		Expect(bu.readBuf).To(HaveLen(2))
		Expect(bu.readBuf[0].CanWaitForCoalesce).To(BeTrue())
		Expect(bu.readBuf[0].Address).To(Equal(uint64(0x1038)))
		Expect(bu.readBuf[0].AccessByteSize).To(Equal(uint64(8)))
		Expect(bu.readBuf[1].CanWaitForCoalesce).To(BeFalse())
		Expect(bu.readBuf[1].Address).To(Equal(uint64(0x1040)))
		Expect(bu.readBuf[1].AccessByteSize).To(Equal(uint64(8)))
	})

	It("should send request out", func() {
		req := mem.ReadReqBuilder{}.
			WithSendTime(10).
			WithSrc(cu.ToScalarMem).
			WithDst(scalarMem).
			WithAddress(1024).
			WithByteSize(4).
			Build()
		bu.readBuf = append(bu.readBuf, req)

		toScalarMem.EXPECT().Send(gomock.Any()).Do(func(r sim.Msg) {
			req := r.(*mem.ReadReq)
			Expect(req.Src).To(BeIdenticalTo(cu.ToScalarMem))
			Expect(req.Dst).To(BeIdenticalTo(scalarMem))
			Expect(req.Address).To(Equal(uint64(1024)))
			Expect(req.AccessByteSize).To(Equal(uint64(4)))
		})

		bu.Run(11)

		Expect(bu.readBuf).To(HaveLen(0))
	})

	It("should retry if send request failed", func() {
		req := mem.ReadReqBuilder{}.
			WithSendTime(10).
			WithSrc(cu.ToScalarMem).
			WithDst(scalarMem).
			WithAddress(1024).
			WithByteSize(4).
			Build()
		bu.readBuf = append(bu.readBuf, req)

		toScalarMem.EXPECT().Send(gomock.Any()).Do(func(r sim.Msg) {
			req := r.(*mem.ReadReq)
			Expect(req.Src).To(BeIdenticalTo(cu.ToScalarMem))
			Expect(req.Dst).To(BeIdenticalTo(scalarMem))
			Expect(req.Address).To(Equal(uint64(1024)))
			Expect(req.AccessByteSize).To(Equal(uint64(4)))
		}).Return(&sim.SendError{})

		bu.Run(11)

		Expect(bu.readBuf).To(HaveLen(1))
	})
	It("should flush the scalar unit", func() {
		wave := wavefront.NewWavefront(nil)
		inst := wavefront.NewInst(insts.NewInst())
		inst.FormatType = insts.SMEM
		inst.Opcode = 1
		inst.Data = insts.NewSRegOperand(0, 0, 1)
		wave.SetDynamicInst(inst)

		bu.toExec = wave
		bu.toWrite = wave
		bu.toRead = wave

		bu.Flush()

		Expect(bu.toRead).To(BeNil())
		Expect(bu.toWrite).To(BeNil())
		Expect(bu.toExec).To(BeNil())
	})

	It("should return correct num of cacheline", func() {
		Expect(bu.numCacheline(0x1038, uint64(80))).To(Equal(3))
	})
})
