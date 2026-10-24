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
	received []*wavefront.Wavefront
}

func (c *fakeCoalescer) generateMemTransactions(
	wf *wavefront.Wavefront,
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

		vecMemUnit.instructionPipeline =
			queueing.MakePipeline[vectorMemInst](1, 6)
		vecMemUnit.postInstructionPipelineBuffer =
			queueing.MakeBuffer[vectorMemInst](16)
		vecMemUnit.transactionPipeline =
			queueing.MakePipeline[uint64](2, 10)
		vecMemUnit.postTransactionPipelineBuffer =
			queueing.MakeBuffer[uint64](8)
	})

	It("should allow accepting wavefront", func() {
		Expect(vecMemUnit.CanAcceptWave()).To(BeTrue())
	})

	It("should not allow accepting wavefront if the instruction pipeline "+
		"entry stage is occupied", func() {
		vecMemUnit.instructionPipeline.Accept(vectorMemInst{})
		Expect(vecMemUnit.CanAcceptWave()).To(BeFalse())
	})

	It("should accept wave", func() {
		wave := wavefront.NewWavefront(kernels.NewWavefront(testIDs.NewID()))

		vecMemUnit.AcceptWave(wave)

		Expect(vecMemUnit.numInstInFlight).To(Equal(uint64(1)))
		Expect(vecMemUnit.instructionPipeline.Stages()).To(HaveLen(1))
	})

	It("should run flat_load_dword", func() {
		kernelWave := kernels.NewWavefront(testIDs.NewID())
		wave := wavefront.NewWavefront(kernelWave)
		inst := wavefront.NewInst(testIDs.NewID(), insts.NewInst())
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
		vecMemUnit.postInstructionPipelineBuffer.Push(vectorMemInst{WavefrontID: wave.UID})
		vecMemUnit.numInstInFlight = 1

		madeProgress := vecMemUnit.instToTransaction()

		Expect(madeProgress).To(BeTrue())
		Expect(coalescer.received).To(ContainElement(wave))
		Expect(wave.State).To(Equal(wavefront.WfReady))
		Expect(wave.OutstandingVectorMemAccess).To(Equal(1))
		Expect(wave.OutstandingScalarMemAccess).To(Equal(1))
		Expect(cu.InFlightVectorMemAccess).To(HaveLen(4))
		Expect(vecMemUnit.transactionsWaiting).To(HaveLen(4))
		Expect(vecMemUnit.postInstructionPipelineBuffer.Size()).To(Equal(0))
		Expect(vecMemUnit.numInstInFlight).To(Equal(uint64(0)))
	})

	It("should run flat_store_dword", func() {
		kernelWave := kernels.NewWavefront(testIDs.NewID())
		wave := wavefront.NewWavefront(kernelWave)
		inst := wavefront.NewInst(testIDs.NewID(), insts.NewInst())
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
		vecMemUnit.postInstructionPipelineBuffer.Push(vectorMemInst{WavefrontID: wave.UID})
		vecMemUnit.numInstInFlight = 1

		madeProgress := vecMemUnit.instToTransaction()

		Expect(madeProgress).To(BeTrue())
		Expect(wave.State).To(Equal(wavefront.WfReady))
		Expect(wave.OutstandingVectorMemAccess).To(Equal(1))
		Expect(wave.OutstandingScalarMemAccess).To(Equal(1))
		Expect(cu.InFlightVectorMemAccess).To(HaveLen(4))
		Expect(vecMemUnit.transactionsWaiting).To(HaveLen(4))
	})

	It("should add transactions to pipeline", func() {
		transactions := make([]VectorMemAccessInfo, 4)
		for i := 0; i < 4; i++ {
			write := messaging.Msg{ID: testIDs.NewID(), Payload: memprotocol.WriteReq{Address: 0x100}}

			transactions[i].Write = write
		}
		vecMemUnit.transactionsWaiting = transactions

		// The transaction pipeline has width 2, so only 2 transactions can
		// enter stage 0 in one cycle.
		madeProgress := vecMemUnit.instToTransaction()

		Expect(madeProgress).To(BeTrue())
		Expect(vecMemUnit.transactionsWaiting).To(HaveLen(2))
		Expect(vecMemUnit.transactionPipeline.Stages()).To(HaveLen(2))
	})

	It("should send memory access requests", func() {
		inst := wavefront.NewInst(testIDs.NewID(), nil)
		loadReq := messaging.Msg{ID: testIDs.NewID(),
			Src: cu.ToVectorMem.AsRemote(),
			Dst: "VectorMem", Payload: memprotocol.ReadReq{Address: 0,
				AccessByteSize: 4}}

		trans := VectorMemAccessInfo{
			Read: loadReq,
			Inst: inst,
		}
		vecMemUnit.numTransactionInFlight = 1

		cu.InFlightVectorMemAccess = append(cu.InFlightVectorMemAccess, trans)
		vecMemUnit.postTransactionPipelineBuffer.Push(trans.Read.ID)

		vecMemUnit.sendRequest()

		Expect(toVectorMem.sent).To(HaveLen(1))
		Expect(toVectorMem.sent[0]).To(Equal(loadReq))
		Expect(vecMemUnit.numTransactionInFlight).To(Equal(uint64(0)))
		Expect(vecMemUnit.postTransactionPipelineBuffer.Size()).To(Equal(0))
	})

	It("should flush the vector memory unit", func() {
		vecMemUnit.numInstInFlight = 1
		vecMemUnit.numTransactionInFlight = 1
		vecMemUnit.transactionsWaiting = append(vecMemUnit.transactionsWaiting,
			VectorMemAccessInfo{})
		vecMemUnit.instructionPipeline.Accept(vectorMemInst{})
		vecMemUnit.postInstructionPipelineBuffer.Push(vectorMemInst{})

		vecMemUnit.Flush()

		Expect(vecMemUnit.numInstInFlight).To(Equal(uint64(0)))
		Expect(vecMemUnit.numTransactionInFlight).To(Equal(uint64(0)))
		Expect(vecMemUnit.transactionsWaiting).To(BeEmpty())
		Expect(vecMemUnit.instructionPipeline.Stages()).To(BeEmpty())
		Expect(vecMemUnit.postInstructionPipelineBuffer.Size()).To(Equal(0))
	})
})
