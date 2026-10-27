package cu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/queueing"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

type fakeCoalescer struct {
	toReturn []VectorMemAccessInfo
	received []*Wavefront
}

func (c *fakeCoalescer) generateMemTransactions(
	wf *Wavefront,
) []VectorMemAccessInfo {
	c.received = append(c.received, wf)
	return c.toReturn
}

var _ = Describe("Vector Memory Unit", func() {
	var (
		cu          *ComputeUnit
		coalescer   *fakeCoalescer
		vecMemUnit  *VectorMemoryUnit
		toVectorMem *fakePort
	)

	BeforeEach(func() {
		engine := newFakeEngine()
		cu = newTestComputeUnit("CU", engine)
		cu.comp.Resources.VectorMemModules = mem.AddressMapper{Destinations: []messaging.RemotePort{"VectorMem"}}

		cu.InFlightVectorMemAccessLimit = 128

		coalescer = new(fakeCoalescer)
		vecMemUnit = NewVectorMemoryUnit(cu, coalescer)
		toVectorMem = newFakePort("CU.VectorMem")
		cu.ToVectorMem = toVectorMem

		vecMemUnit.state().InstructionsPipeline =
			queueing.MakePipeline[vectorMemInst](1, 6)
		vecMemUnit.state().InstructionsOutput =
			queueing.MakeBuffer[vectorMemInst](16)
		vecMemUnit.state().TransactionsPipeline =
			queueing.MakePipeline[uint64](2, 10)
		vecMemUnit.state().TransactionsOutput =
			queueing.MakeBuffer[uint64](8)
	})

	It("should allow accepting wavefront", func() {
		Expect(vecMemUnit.CanAcceptWave()).To(BeTrue())
	})

	It("should not allow accepting wavefront if the instruction pipeline "+
		"entry stage is occupied", func() {
		vecMemUnit.state().InstructionsPipeline.Accept(vectorMemInst{})
		Expect(vecMemUnit.CanAcceptWave()).To(BeFalse())
	})

	It("should accept wave", func() {
		wave := newTestRawWave(kernels.NewWavefront(testIDs.NewID()))

		registerTestWave(cu, wave)
		vecMemUnit.AcceptWave(wave)

		Expect(vecMemUnit.state().Instructions).To(Equal(uint64(1)))
		Expect(vecMemUnit.state().InstructionsPipeline.Stages()).To(HaveLen(1))
	})

	It("should run flat_load_dword", func() {
		kernelWave := kernels.NewWavefront(testIDs.NewID())
		wave := newTestRawWave(kernelWave)
		inst := testInst(testIDs.NewID(), insts.NewInst())
		inst.Format = insts.FormatTable[insts.FLAT]
		inst.Opcode = 20
		inst.Dst = insts.NewVRegOperand(0, 0, 1)
		wave.SetDynamicInst(inst)

		transactions := make([]VectorMemAccessInfo, 4)
		for i := 0; i < 4; i++ {
			read := messaging.Msg{ID: testIDs.NewID(), Payload: memprotocol.ReadReq{Address: 0x100,
				AccessByteSize: 4}}

			transactions[i].Read = read
		}
		coalescer.toReturn = transactions

		registerTestWave(cu, wave)
		vecMemUnit.state().InstructionsOutput.Push(vectorMemInst{WavefrontID: wfID(wave)})
		vecMemUnit.state().Instructions = 1

		madeProgress := vecMemUnit.instToTransaction()

		Expect(madeProgress).To(BeTrue())
		Expect(coalescer.received).To(ContainElement(wave))
		Expect(wave.execution().State).To(Equal(wavefront.WfReady))
		Expect(wave.execution().OutstandingVectorMemAccess).To(Equal(1))
		Expect(wave.execution().OutstandingScalarMemAccess).To(Equal(1))
		Expect(cu.comp.State.Vector).To(HaveLen(4))
		Expect(vecMemUnit.state().Waiting).To(HaveLen(4))
		Expect(vecMemUnit.state().InstructionsOutput.Size()).To(Equal(0))
		Expect(vecMemUnit.state().Instructions).To(Equal(uint64(0)))
	})

	It("should run flat_store_dword", func() {
		kernelWave := kernels.NewWavefront(testIDs.NewID())
		wave := newTestRawWave(kernelWave)
		inst := testInst(testIDs.NewID(), insts.NewInst())
		inst.Format = insts.FormatTable[insts.FLAT]
		inst.Opcode = 28
		inst.Dst = insts.NewVRegOperand(0, 0, 1)
		wave.SetDynamicInst(inst)

		transactions := make([]VectorMemAccessInfo, 4)
		for i := 0; i < 4; i++ {
			write := messaging.Msg{ID: testIDs.NewID(), Payload: memprotocol.WriteReq{Address: 0x100}}

			transactions[i].Write = write
		}
		coalescer.toReturn = transactions

		registerTestWave(cu, wave)
		vecMemUnit.state().InstructionsOutput.Push(vectorMemInst{WavefrontID: wfID(wave)})
		vecMemUnit.state().Instructions = 1

		madeProgress := vecMemUnit.instToTransaction()

		Expect(madeProgress).To(BeTrue())
		Expect(wave.execution().State).To(Equal(wavefront.WfReady))
		Expect(wave.execution().OutstandingVectorMemAccess).To(Equal(1))
		Expect(wave.execution().OutstandingScalarMemAccess).To(Equal(1))
		Expect(cu.comp.State.Vector).To(HaveLen(4))
		Expect(vecMemUnit.state().Waiting).To(HaveLen(4))
	})

	It("should add transactions to pipeline", func() {
		transactions := make([]VectorMemAccessInfo, 4)
		for i := 0; i < 4; i++ {
			write := messaging.Msg{ID: testIDs.NewID(), Payload: memprotocol.WriteReq{Address: 0x100}}

			transactions[i].Write = write
		}
		vecMemUnit.state().Waiting = testVectorStates(cu, transactions)

		// The transaction pipeline has width 2, so only 2 transactions can
		// enter stage 0 in one cycle.
		madeProgress := vecMemUnit.instToTransaction()

		Expect(madeProgress).To(BeTrue())
		Expect(vecMemUnit.state().Waiting).To(HaveLen(2))
		Expect(vecMemUnit.state().TransactionsPipeline.Stages()).To(HaveLen(2))
	})

	It("should send memory access requests", func() {
		inst := testInst(testIDs.NewID(), nil)
		loadReq := messaging.Msg{ID: testIDs.NewID(),
			Src: cu.ToVectorMem.AsRemote(),
			Dst: "VectorMem", Payload: memprotocol.ReadReq{Address: 0,
				AccessByteSize: 4}}

		trans := VectorMemAccessInfo{
			Read: loadReq,
			Inst: inst,
		}
		vecMemUnit.state().Transactions = 1

		cu.comp.State.Vector = append(cu.comp.State.Vector, testVectorState(cu, trans))
		vecMemUnit.state().TransactionsOutput.Push(trans.Read.ID)

		vecMemUnit.sendRequest()

		Expect(toVectorMem.sent).To(HaveLen(1))
		Expect(toVectorMem.sent[0]).To(Equal(loadReq))
		Expect(vecMemUnit.state().Transactions).To(Equal(uint64(0)))
		Expect(vecMemUnit.state().TransactionsOutput.Size()).To(Equal(0))
	})

})
