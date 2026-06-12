package cu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
	"github.com/sarchlab/mgpusim/v4/amd/kernels"
	"github.com/sarchlab/mgpusim/v4/amd/timing/wavefront"
	"go.uber.org/mock/gomock"
)

var _ = Describe("Vector Memory Unit", func() {

	var (
		mockCtrl  *gomock.Controller
		cu        *ComputeUnit
		coalescer *Mockcoalescer
		vecMemUnit *VectorMemoryUnit
		vectorMem  *MockPort
		toVectorMem *MockPort
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		cu = NewComputeUnit("CU", nil)
		coalescer = NewMockcoalescer(mockCtrl)
		vecMemUnit = NewVectorMemoryUnit(cu, coalescer)
		toVectorMem = NewMockPort(mockCtrl)
		cu.ToVectorMem = toVectorMem
		cu.VectorMemModules = new(mem.SinglePortMapper)
		cu.InFlightVectorMemAccessLimit = 128
		vectorMem = NewMockPort(mockCtrl)

		vecMemUnit.instructionPipeline = queueing.Pipeline[vectorMemInst]{
			Width: 1, NumStages: 1,
		}
		vecMemUnit.postInstructionPipelineBuffer = queueing.Buffer[vectorMemInst]{
			Cap: 128,
		}
		vecMemUnit.transactionPipeline = queueing.Pipeline[VectorMemAccessInfo]{
			Width: 1, NumStages: 1,
		}
		vecMemUnit.postTransactionPipelineBuffer = queueing.Buffer[VectorMemAccessInfo]{
			Cap: 128,
		}

		toVectorMem.EXPECT().AsRemote().AnyTimes()
		vectorMem.EXPECT().AsRemote().AnyTimes()
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should allow accepting wavefront", func() {
		Expect(vecMemUnit.CanAcceptWave()).To(BeTrue())
	})

	It("should not allow accepting wavefront if the pipeline is full", func() {
		// Fill the pipeline stage 0 to capacity (width=1)
		vecMemUnit.instructionPipeline.Accept(vectorMemInst{})
		Expect(vecMemUnit.CanAcceptWave()).To(BeFalse())
	})

	It("should accept wave", func() {
		wave := new(wavefront.Wavefront)

		vecMemUnit.AcceptWave(wave)

		Expect(vecMemUnit.numInstInFlight).To(Equal(uint64(1)))
		Expect(vecMemUnit.instructionPipeline.Stages).To(HaveLen(1))
	})

	It("should run flat_load_dword", func() {
		kernelWave := kernels.NewWavefront()
		wave := wavefront.NewWavefront(kernelWave)
		inst := wavefront.NewInst(insts.NewInst())
		inst.Format = insts.FormatTable[insts.FLAT]
		inst.Opcode = 20
		inst.Dst = insts.NewVRegOperand(0, 0, 1)
		wave.SetDynamicInst(inst)

		transactions := make([]VectorMemAccessInfo, 4)
		for i := 0; i < 4; i++ {
			read := &mem.ReadReq{Address: 0x100, AccessByteSize: 4}
			read.ID = sim.GetIDGenerator().Generate()
			transactions[i].Read = read
		}
		coalescer.EXPECT().generateMemTransactions(wave).Return(transactions)

		// Put item in the post-instruction buffer (simulates pipeline output)
		vecMemUnit.postInstructionPipelineBuffer.PushTyped(
			vectorMemInst{wavefront: wave})

		madeProgress := vecMemUnit.instToTransaction()

		Expect(madeProgress).To(BeTrue())
		Expect(wave.State).To(Equal(wavefront.WfReady))
		Expect(wave.OutstandingVectorMemAccess).To(Equal(1))
		Expect(wave.OutstandingScalarMemAccess).To(Equal(1))
		Expect(cu.InFlightVectorMemAccess).To(HaveLen(4))
		Expect(cu.InFlightVectorMemAccess[3].Read.CanWaitForCoalesce).
			To(BeFalse())
		Expect(vecMemUnit.transactionsWaiting).To(HaveLen(4))
	})

	It("should run flat_store_dword", func() {
		kernelWave := kernels.NewWavefront()
		wave := wavefront.NewWavefront(kernelWave)
		inst := wavefront.NewInst(insts.NewInst())
		inst.Format = insts.FormatTable[insts.FLAT]
		inst.Opcode = 28
		inst.Dst = insts.NewVRegOperand(0, 0, 1)
		wave.SetDynamicInst(inst)

		transactions := make([]VectorMemAccessInfo, 4)
		for i := 0; i < 4; i++ {
			write := &mem.WriteReq{Address: 0x100}
			write.ID = sim.GetIDGenerator().Generate()
			transactions[i].Write = write
		}
		coalescer.EXPECT().generateMemTransactions(wave).Return(transactions)

		// Put item in the post-instruction buffer
		vecMemUnit.postInstructionPipelineBuffer.PushTyped(
			vectorMemInst{wavefront: wave})

		madeProgress := vecMemUnit.instToTransaction()

		Expect(madeProgress).To(BeTrue())
		Expect(wave.State).To(Equal(wavefront.WfReady))
		Expect(wave.OutstandingVectorMemAccess).To(Equal(1))
		Expect(wave.OutstandingScalarMemAccess).To(Equal(1))
		Expect(cu.InFlightVectorMemAccess).To(HaveLen(4))
		Expect(cu.InFlightVectorMemAccess[3].Write.CanWaitForCoalesce).
			To(BeFalse())
		Expect(vecMemUnit.transactionsWaiting).To(HaveLen(4))
	})

	It("should add transactions to pipeline", func() {
		transactions := make([]VectorMemAccessInfo, 4)
		for i := 0; i < 4; i++ {
			write := &mem.WriteReq{Address: 0x100}
			write.ID = sim.GetIDGenerator().Generate()
			transactions[i].Write = write
		}
		vecMemUnit.transactionsWaiting = transactions

		// Pipeline width=1, so each call to instToTransaction inserts one
		// transaction into the pipeline. After 1 accept, pipeline is full
		// (width=1, stage=0 occupied). We need a wider pipeline for this test.
		vecMemUnit.transactionPipeline = queueing.Pipeline[VectorMemAccessInfo]{
			Width: 2, NumStages: 1,
		}

		madeProgress := vecMemUnit.instToTransaction()

		Expect(madeProgress).To(BeTrue())
		// 2 accepted (pipeline width=2), 2 remaining
		Expect(vecMemUnit.transactionsWaiting).To(HaveLen(2))
	})

	It("should send memory access requests", func() {
		inst := wavefront.NewInst(nil)
		loadReq := &mem.ReadReq{Address: 0, AccessByteSize: 4}
		loadReq.ID = sim.GetIDGenerator().Generate()
		loadReq.Src = cu.ToVectorMem.AsRemote()
		loadReq.Dst = vectorMem.AsRemote()

		trans := VectorMemAccessInfo{
			Read: loadReq,
			Inst: inst,
		}
		vecMemUnit.numTransactionInFlight = 1

		// Put the transaction in the post-transaction buffer
		vecMemUnit.postTransactionPipelineBuffer.PushTyped(trans)
		toVectorMem.EXPECT().Send(loadReq)

		vecMemUnit.sendRequest()

		Expect(vecMemUnit.numTransactionInFlight).To(Equal(uint64(0)))
		Expect(vecMemUnit.postTransactionPipelineBuffer.Size()).To(Equal(0))
	})

	It("should flush the vector memory unit", func() {
		vecMemUnit.numInstInFlight = 1
		vecMemUnit.numTransactionInFlight = 1
		vecMemUnit.transactionsWaiting = append(vecMemUnit.transactionsWaiting,
			VectorMemAccessInfo{})

		// Add items to pipelines and buffers to verify they get cleared
		vecMemUnit.instructionPipeline.Accept(vectorMemInst{})
		vecMemUnit.postInstructionPipelineBuffer.PushTyped(vectorMemInst{})
		vecMemUnit.transactionPipeline.Accept(VectorMemAccessInfo{})
		vecMemUnit.postTransactionPipelineBuffer.PushTyped(VectorMemAccessInfo{})

		vecMemUnit.Flush()

		Expect(vecMemUnit.numInstInFlight).To(Equal(uint64(0)))
		Expect(vecMemUnit.numTransactionInFlight).To(Equal(uint64(0)))
		Expect(vecMemUnit.transactionsWaiting).To(BeEmpty())
		Expect(vecMemUnit.instructionPipeline.Stages).To(BeNil())
		Expect(vecMemUnit.transactionPipeline.Stages).To(BeNil())
		Expect(vecMemUnit.postInstructionPipelineBuffer.Size()).To(Equal(0))
		Expect(vecMemUnit.postTransactionPipelineBuffer.Size()).To(Equal(0))
	})
})
