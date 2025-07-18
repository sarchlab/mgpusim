package cu

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
	"github.com/sarchlab/mgpusim/v4/amd/kernels"
	"github.com/sarchlab/mgpusim/v4/amd/timing/wavefront"
	"go.uber.org/mock/gomock"
)

var _ = Describe("Vector Memory Unit", func() {

	var (
		mockCtrl            *gomock.Controller
		cu                  *ComputeUnit
		sp                  *mockScratchpadPreparer
		coalescer           *Mockcoalescer
		vecMemUnit          *VectorMemoryUnit
		vectorMem           *MockPort
		toVectorMem         *MockPort
		instPipeline        *MockPipeline
		instBuffer          *MockBuffer
		transactionPipeline *MockPipeline
		transactionBuffer   *MockBuffer
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		cu = NewComputeUnit("CU", nil)
		sp = new(mockScratchpadPreparer)
		coalescer = NewMockcoalescer(mockCtrl)
		vecMemUnit = NewVectorMemoryUnit(cu, sp, coalescer)
		toVectorMem = NewMockPort(mockCtrl)
		instPipeline = NewMockPipeline(mockCtrl)
		instBuffer = NewMockBuffer(mockCtrl)
		transactionPipeline = NewMockPipeline(mockCtrl)
		transactionBuffer = NewMockBuffer(mockCtrl)
		cu.ToVectorMem = toVectorMem
		cu.VectorMemModules = new(mem.SinglePortMapper)
		cu.InFlightVectorMemAccessLimit = 128
		vectorMem = NewMockPort(mockCtrl)

		vecMemUnit.instructionPipeline = instPipeline
		vecMemUnit.postInstructionPipelineBuffer = instBuffer
		vecMemUnit.transactionPipeline = transactionPipeline
		vecMemUnit.postTransactionPipelineBuffer = transactionBuffer

		toVectorMem.EXPECT().AsRemote().AnyTimes()
		vectorMem.EXPECT().AsRemote().AnyTimes()
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should allow accepting wavefront", func() {
		instPipeline.EXPECT().CanAccept().Return(true)
		Expect(vecMemUnit.CanAcceptWave()).To(BeTrue())
	})

	It("should not allow accepting wavefront if the read stage buffer is occupied", func() {
		instPipeline.EXPECT().CanAccept().Return(false)
		Expect(vecMemUnit.CanAcceptWave()).To(BeFalse())
	})

	It("should accept wave", func() {
		wave := new(wavefront.Wavefront)

		instPipeline.EXPECT().Accept(gomock.Any())

		vecMemUnit.AcceptWave(wave)

		Expect(vecMemUnit.numInstInFlight).To(Equal(uint64(1)))
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
			read := mem.ReadReqBuilder{}.
				WithAddress(0x100).
				WithByteSize(4).
				Build()
			transactions[i].Read = read
		}
		coalescer.EXPECT().generateMemTransactions(wave).Return(transactions)
		instBuffer.EXPECT().Peek().Return(vectorMemInst{wavefront: wave})
		instBuffer.EXPECT().Pop().Return(vectorMemInst{wavefront: wave})

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
			write := mem.WriteReqBuilder{}.
				WithAddress(0x100).
				Build()
			transactions[i].Write = write
		}
		coalescer.EXPECT().generateMemTransactions(wave).Return(transactions)
		instBuffer.EXPECT().Peek().Return(vectorMemInst{wavefront: wave})
		instBuffer.EXPECT().Pop().Return(vectorMemInst{wavefront: wave})

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
			write := mem.WriteReqBuilder{}.
				WithAddress(0x100).
				Build()
			transactions[i].Write = write
		}
		vecMemUnit.transactionsWaiting = transactions

		transactionPipeline.EXPECT().CanAccept().Return(true)
		transactionPipeline.EXPECT().Accept(gomock.Any())

		madeProgress := vecMemUnit.instToTransaction()

		Expect(madeProgress).To(BeTrue())
		Expect(vecMemUnit.transactionsWaiting).To(HaveLen(3))
	})

	It("should send memory access requests", func() {
		inst := wavefront.NewInst(nil)
		loadReq := mem.ReadReqBuilder{}.
			WithSrc(cu.ToVectorMem.AsRemote()).
			WithDst(vectorMem.AsRemote()).
			WithAddress(0).
			WithByteSize(4).
			Build()
		trans := VectorMemAccessInfo{
			Read: loadReq,
			Inst: inst,
		}
		vecMemUnit.numTransactionInFlight = 1

		transactionBuffer.EXPECT().Peek().Return(trans)
		transactionBuffer.EXPECT().Pop()
		toVectorMem.EXPECT().Send(loadReq)

		vecMemUnit.sendRequest()

		Expect(vecMemUnit.numTransactionInFlight).To(Equal(uint64(0)))
	})

	It("should flush the vector memory unit", func() {
		vecMemUnit.numInstInFlight = 1
		vecMemUnit.numTransactionInFlight = 1
		vecMemUnit.transactionsWaiting = append(vecMemUnit.transactionsWaiting,
			VectorMemAccessInfo{})

		instPipeline.EXPECT().Clear()
		instBuffer.EXPECT().Clear()
		transactionPipeline.EXPECT().Clear()
		transactionBuffer.EXPECT().Clear()

		vecMemUnit.Flush()

		Expect(vecMemUnit.numInstInFlight).To(Equal(uint64(0)))
		Expect(vecMemUnit.numTransactionInFlight).To(Equal(uint64(0)))
		Expect(vecMemUnit.transactionsWaiting).To(BeEmpty())
	})
})
