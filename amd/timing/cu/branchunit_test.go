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

var _ = Describe("Branch Unit", func() {
	var (
		cu  *ComputeUnit
		bu  *BranchUnit
		alu *mockALU
	)

	BeforeEach(func() {
		cu = newTestComputeUnit("CU", nil)
		alu = new(mockALU)
		bu = NewBranchUnit(cu, alu)
	})

	It("should allow accepting wavefront", func() {
		// wave := newTestWave()
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
		Expect(cu.wave(bu.state().Read)).To(Equal(wave))
	})

	It("should run", func() {
		wave1 := newTestWave()
		wave2 := newTestWave()
		wave3 := newTestWave()
		wave3.execution().State = wavefront.WfRunning
		wave3.execution().InstBuffer = make([]byte, 256)
		wave3.execution().InstBufferStartPC = 0x100
		inst := testInst(testIDs.NewID(), insts.NewInst())
		inst.FormatType = insts.SOPP
		inst.SImm16 = insts.NewIntOperand(1, 1)
		inst.ByteSize = 4
		wave3.setTestInst(inst)
		wave3.SetPC(0x13C)

		bu.state().Read = testWaveID(cu, wave1)
		bu.state().Exec = testWaveID(cu, wave2)
		bu.state().Write = testWaveID(cu, wave3)

		bu.Run()

		Expect(wave3.execution().State).To(Equal(wavefront.WfReady))
		Expect(wave3.PC()).To(Equal(uint64(0x140)))

		Expect(cu.wave(bu.state().Write)).To(Equal(wave2))
		Expect(cu.wave(bu.state().Exec)).To(Equal(wave1))
		Expect(cu.wave(bu.state().Read)).To(BeNil())

		Expect(alu.wfExecuted.(*operationWave).value).To(BeIdenticalTo(wave2.execution()))
		Expect(wave3.execution().InstBuffer).To(HaveLen(0))
	})

	It("should redirect fetch to a target beyond the buffered lines", func() {
		wave := newTestWave()
		wave.execution().State = wavefront.WfRunning
		wave.execution().InstBuffer = make([]byte, 128)
		wave.execution().InstBufferStartPC = 0x100
		wave.execution().NextFetchPC = 0x200
		inst := testInst(testIDs.NewID(), insts.NewInst())
		inst.FormatType = insts.SOPP
		inst.ByteSize = 4
		wave.setTestInst(inst)
		// A taken branch: the PC is one instruction short of its target.
		wave.SetPC(0x1ffc)
		bu.state().Write = testWaveID(cu, wave)

		bu.Run()

		Expect(wave.PC()).To(Equal(uint64(0x2000)))
		Expect(wave.execution().InstBuffer).To(HaveLen(0))
		Expect(wave.execution().InstBufferStartPC).To(Equal(uint64(0x2000)))
		Expect(wave.execution().FetchEpoch).To(Equal(uint64(1)))
		Expect(nextFetchPC(wave.execution())).To(Equal(uint64(0x2000)))
	})

	Context("when s_setpc_b64 names an address", func() {
		var wave *Wavefront

		BeforeEach(func() {
			wave = newTestWave()
			wave.execution().State = wavefront.WfRunning
			wave.execution().InstBuffer = make([]byte, 64)
			wave.execution().InstBufferStartPC = 0x2000
			inst := testInst(testIDs.NewID(), insts.NewInst())
			inst.FormatType = insts.SOP1
			inst.Opcode = 29
			inst.ByteSize = 4
			wave.setTestInst(inst)
			wave.SetPC(0x2010)
			bu = NewBranchUnit(cu, &jumpALU{target: 0x1004})
			bu.state().Exec = testWaveID(cu, wave)
		})

		It("should hold that address from execution through writeback", func() {
			bu.Run()

			// The instruction completes at the start of the write stage, so
			// this is the PC that completion observers see.
			Expect(wave.PC()).To(Equal(uint64(0x1004)))
			Expect(wave.execution().State).To(Equal(wavefront.WfRunning))

			bu.Run()

			Expect(wave.execution().State).To(Equal(wavefront.WfReady))
			Expect(wave.PC()).To(Equal(uint64(0x1004)))
			Expect(wave.execution().InstBuffer).To(HaveLen(0))
			Expect(wave.execution().InstBufferStartPC).To(Equal(uint64(0x1000)))
		})

		It("should drop every fetch in flight, even one for the target line", func() {
			cu.comp.Spec.InstFetchAheadLines = 4
			recordCodeEnd(wave, 0x1000, 0x2000)
			var reqs []messaging.Msg
			wave.execution().IsFetching = true
			wave.execution().NextFetchPC = 0x2100
			for _, addr := range []uint64{0x2040, 0x1000, 0x20c0} {
				req := messaging.Msg{ID: testIDs.NewID(), Payload: memprotocol.ReadReq{
					Address: addr, AccessByteSize: 64}}
				reqs = append(reqs, req)
				cu.comp.State.InstFetch = append(cu.comp.State.InstFetch,
					testFetchState(cu, &InstFetchReqInfo{Req: req, Wavefront: wave, Address: addr}))
			}

			bu.Run()
			bu.Run()

			Expect(wave.execution().FetchEpoch).To(Equal(uint64(1)))
			Expect(wave.execution().NextFetchPC).To(Equal(uint64(0)))
			for i, req := range reqs {
				cu.handleFetchReturn(messaging.Msg{ID: testIDs.NewID(), RspTo: req.ID,
					Payload: memprotocol.DataReadyRsp{Data: make([]byte, 64)}})
				Expect(wave.execution().InstBuffer).To(HaveLen(0))
				Expect(wave.execution().IsFetching).To(Equal(i < len(reqs)-1))
			}
			Expect(cu.comp.State.InstFetch).To(HaveLen(0))
			Expect(wave.execution().InstBufferStartPC).To(Equal(uint64(0x1000)))
		})

		It("should take the lines of the new stream in order", func() {
			cu.comp.Spec.InstFetchAheadLines = 4
			recordCodeEnd(wave, 0x1000, 0x2000)
			bu.Run()
			bu.Run()

			var reqs []messaging.Msg
			for _, addr := range []uint64{0x1000, 0x1040} {
				req := messaging.Msg{ID: testIDs.NewID(), Payload: memprotocol.ReadReq{
					Address: addr, AccessByteSize: 64}}
				reqs = append(reqs, req)
				cu.comp.State.InstFetch = append(cu.comp.State.InstFetch, testFetchState(cu,
					&InstFetchReqInfo{Req: req, Wavefront: wave, Address: addr,
						Epoch: wave.execution().FetchEpoch}))
			}
			wave.execution().IsFetching = true
			wave.execution().State = wavefront.WfDraining

			cu.handleFetchReturn(messaging.Msg{ID: testIDs.NewID(), RspTo: reqs[0].ID,
				Payload: memprotocol.DataReadyRsp{Data: make([]byte, 64)}})
			Expect(wave.execution().InstBuffer).To(HaveLen(64))
			Expect(wave.execution().IsFetching).To(BeTrue())
			Expect(wave.execution().State).To(Equal(wavefront.WfDraining))

			cu.handleFetchReturn(messaging.Msg{ID: testIDs.NewID(), RspTo: reqs[1].ID,
				Payload: memprotocol.DataReadyRsp{Data: make([]byte, 64)}})
			Expect(wave.execution().InstBuffer).To(HaveLen(128))
			Expect(wave.execution().IsFetching).To(BeFalse())
			Expect(wave.execution().State).To(Equal(wavefront.WfCompleted))
		})

		It("should drop a fall-through fetch that returns after the jump", func() {
			req := messaging.Msg{ID: testIDs.NewID(), Payload: memprotocol.ReadReq{
				Address: 0x2040, AccessByteSize: 64}}
			wave.execution().IsFetching = true
			cu.comp.State.InstFetch = append(cu.comp.State.InstFetch,
				testFetchState(cu, &InstFetchReqInfo{Req: req, Wavefront: wave, Address: 0x2040}))

			bu.Run()
			bu.Run()
			cu.handleFetchReturn(messaging.Msg{ID: testIDs.NewID(), RspTo: req.ID,
				Payload: memprotocol.DataReadyRsp{Data: make([]byte, 64)}})

			Expect(wave.execution().IsFetching).To(BeFalse())
			Expect(cu.comp.State.InstFetch).To(HaveLen(0))
			Expect(wave.execution().InstBuffer).To(HaveLen(0))
			Expect(wave.execution().InstBufferStartPC).To(Equal(uint64(0x1000)))
			Expect(wave.PC()).To(Equal(uint64(0x1004)))
		})
	})

})

// jumpALU sets the PC to an absolute address, as s_setpc_b64 does.
type jumpALU struct {
	mockALU
	target uint64
}

func (alu *jumpALU) Run(wf emu.InstEmuState) { wf.SetPC(alu.target) }
