package timing

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3/insts"
	"gitlab.com/akita/gcn3/timing/wavefront"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
)

var _ = Describe("Vector Memory Unit", func() {

	var (
		mockCtrl    *gomock.Controller
		cu          *ComputeUnit
		sp          *mockScratchpadPreparer
		coalescer   *Mockcoalescer
		bu          *VectorMemoryUnit
		vectorMem   *MockPort
		toVectorMem *MockPort
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		cu = NewComputeUnit("cu", nil)
		sp = new(mockScratchpadPreparer)
		coalescer = NewMockcoalescer(mockCtrl)
		bu = NewVectorMemoryUnit(cu, sp, coalescer)
		//vectorMem = NewMockPort(mockCtrl)
		toVectorMem = NewMockPort(mockCtrl)
		cu.ToVectorMem = toVectorMem

		cu.VectorMemModules = new(cache.SingleLowModuleFinder)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should allow accepting wavefront", func() {
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

	It("should read", func() {
		wave := new(wavefront.Wavefront)
		bu.toRead = wave

		madeProgress := bu.runReadStage(10)

		Expect(madeProgress).To(BeTrue())
		Expect(bu.toExec).To(BeIdenticalTo(wave))
		Expect(bu.toRead).To(BeNil())
		Expect(bu.AddrCoalescingCycleLeft).To(Equal(bu.AddrCoalescingLatency))
	})

	It("should reduce cycle left when executing", func() {
		wave := new(wavefront.Wavefront)
		bu.toExec = wave
		bu.AddrCoalescingCycleLeft = 40

		madeProgress := bu.runExecStage(10)

		Expect(madeProgress).To(BeTrue())
		Expect(bu.toExec).To(BeIdenticalTo(wave))
		Expect(bu.AddrCoalescingCycleLeft).To(Equal(39))
	})

	It("should run flat_load_dword", func() {
		wave := wavefront.NewWavefront(nil)
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

		bu.toExec = wave

		bu.Run(10)

		Expect(wave.State).To(Equal(wavefront.WfReady))
		Expect(wave.OutstandingVectorMemAccess).To(Equal(1))
		Expect(wave.OutstandingScalarMemAccess).To(Equal(1))
		Expect(cu.InFlightVectorMemAccess).To(HaveLen(4))
		Expect(cu.InFlightVectorMemAccess[3].Read.CanWaitForCoalesce).
			To(BeFalse())
		Expect(bu.SendBuf).To(HaveLen(4))
	})

	It("should run flat_store_dword", func() {
		wave := wavefront.NewWavefront(nil)
		inst := wavefront.NewInst(insts.NewInst())
		inst.Format = insts.FormatTable[insts.FLAT]
		inst.Opcode = 28
		inst.Dst = insts.NewVRegOperand(0, 0, 1)
		wave.SetDynamicInst(inst)
		bu.toExec = wave

		transactions := make([]VectorMemAccessInfo, 4)
		for i := 0; i < 4; i++ {
			write := mem.WriteReqBuilder{}.
				WithAddress(0x100).
				Build()
			transactions[i].Write = write
		}
		coalescer.EXPECT().generateMemTransactions(wave).Return(transactions)

		bu.Run(10)

		Expect(wave.State).To(Equal(wavefront.WfReady))
		Expect(wave.OutstandingVectorMemAccess).To(Equal(1))
		Expect(wave.OutstandingScalarMemAccess).To(Equal(1))
		Expect(cu.InFlightVectorMemAccess).To(HaveLen(4))
		Expect(cu.InFlightVectorMemAccess[3].Write.CanWaitForCoalesce).
			To(BeFalse())
		Expect(bu.SendBuf).To(HaveLen(4))
	})

	It("should send memory access requests", func() {
		loadReq := mem.ReadReqBuilder{}.
			WithSendTime(10).
			WithSrc(cu.ToVectorMem).
			WithDst(vectorMem).
			WithAddress(0).
			WithByteSize(4).
			Build()
		loadReq.RecvTime = 10
		bu.SendBuf = append(bu.SendBuf, loadReq)

		toVectorMem.EXPECT().Send(loadReq)

		bu.Run(10)
		Expect(len(bu.SendBuf)).To(Equal(0))
	})

	It("should not remove request from read buffer, if send fails", func() {
		loadReq := mem.ReadReqBuilder{}.
			WithSendTime(10).
			WithSrc(cu.ToVectorMem).
			WithDst(vectorMem).
			WithAddress(0).
			WithByteSize(4).
			Build()
		bu.SendBuf = append(bu.SendBuf, loadReq)

		toVectorMem.EXPECT().Send(loadReq).Return(&akita.SendError{})

		bu.Run(10)

		Expect(len(bu.SendBuf)).To(Equal(1))
	})

	It("should flush the vector memory unit", func() {
		wave := wavefront.NewWavefront(nil)
		inst := wavefront.NewInst(insts.NewInst())
		inst.Format = insts.FormatTable[insts.FLAT]
		inst.Opcode = 28
		inst.Dst = insts.NewVRegOperand(0, 0, 1)
		wave.SetDynamicInst(inst)

		sp := wave.Scratchpad().AsFlat()
		for i := 0; i < 64; i++ {
			sp.ADDR[i] = uint64(4096 + i*4)
			sp.DATA[i*4] = uint32(i)
		}
		sp.EXEC = 0xffffffffffffffff

		bu.toExec = wave
		bu.toRead = wave
		bu.toWrite = wave

		loadReq := mem.ReadReqBuilder{}.
			WithSendTime(10).
			WithSrc(cu.ToVectorMem).
			WithDst(vectorMem).
			WithAddress(0).
			WithByteSize(4).
			Build()
		bu.SendBuf = append(bu.SendBuf, loadReq)

		bu.Flush()

		Expect(bu.SendBuf).To(BeNil())
		Expect(bu.toWrite).To(BeNil())
		Expect(bu.toRead).To(BeNil())
		Expect(bu.toExec).To(BeNil())
	})
})
