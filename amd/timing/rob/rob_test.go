package rob

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/sim"
	"go.uber.org/mock/gomock"
)

var _ = Describe("Reorder Buffer", func() {
	var (
		mockCtrl       *gomock.Controller
		rob            *ReorderBuffer
		topPort        *MockPort
		bottomPort     *MockPort
		ctrlPort       *MockPort
		bottomUnitPort *MockPort
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		topPort = NewMockPort(mockCtrl)
		bottomPort = NewMockPort(mockCtrl)
		ctrlPort = NewMockPort(mockCtrl)

		topPort.EXPECT().AsRemote().AnyTimes()
		topPort.EXPECT().Name().AnyTimes()
		bottomPort.EXPECT().AsRemote().AnyTimes()
		bottomPort.EXPECT().Name().AnyTimes()
		ctrlPort.EXPECT().AsRemote().AnyTimes()
		ctrlPort.EXPECT().Name().AnyTimes()

		rob = MakeBuilder().
			WithBufferSize(10).
			Build("ROB")
		rob.topPort = topPort
		rob.bottomPort = bottomPort
		rob.controlPort = ctrlPort
		bottomUnitPort = NewMockPort(mockCtrl)
		bottomUnitPort.EXPECT().AsRemote().AnyTimes()
		rob.BottomUnit = bottomUnitPort.AsRemote()
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("top-down", func() {
		var (
			read *mem.ReadReq
		)

		BeforeEach(func() {
			read = &mem.ReadReq{}
			read.ID = sim.GetIDGenerator().Generate()
		})

		It("should do nothing if buffer is full", func() {
			topPort.EXPECT().PeekIncoming().Return(read)

			for i := 0; i < 10; i++ {
				req := &mem.ReadReq{}
				req.ID = sim.GetIDGenerator().Generate()
				trans := rob.createTransaction(req)
				rob.addTransaction(trans)
			}

			madeProgress := rob.topDown()

			Expect(madeProgress).To(BeFalse())
		})

		It("should do nothing if no message arriving", func() {
			topPort.EXPECT().PeekIncoming().Return(nil)

			madeProgress := rob.topDown()

			Expect(madeProgress).To(BeFalse())
		})

		It("should not receive request if bottom port is busy", func() {
			topPort.EXPECT().PeekIncoming().Return(read)
			bottomPort.EXPECT().
				Send(gomock.Any()).
				Return(sim.NewSendError())

			madeProgress := rob.topDown()

			Expect(madeProgress).To(BeFalse())
		})

		It("should accept request from top and forward to bottom", func() {
			topPort.EXPECT().PeekIncoming().Return(read)
			topPort.EXPECT().RetrieveIncoming()
			bottomPort.EXPECT().
				Send(gomock.Any()).
				Do(func(req *mem.ReadReq) {
					Expect(req.Src).To(BeIdenticalTo(rob.bottomPort.AsRemote()))
					Expect(req.Dst).To(BeIdenticalTo(rob.BottomUnit))
				}).
				Return(nil)

			madeProgress := rob.topDown()

			Expect(madeProgress).To(BeTrue())
			Expect(rob.transactions.Len()).To(Equal(1))
			Expect(rob.toBottomReqIDToTransactionTable).To(HaveLen(1))
		})
	})

	Context("parse bottom", func() {
		var (
			writeFromTop *mem.WriteReq
			transaction  *transaction
		)

		BeforeEach(func() {
			writeFromTop = &mem.WriteReq{}
			writeFromTop.ID = sim.GetIDGenerator().Generate()
			transaction = rob.createTransaction(writeFromTop)
			rob.addTransaction(transaction)
		})

		It("should do nothing if no response in the Bottom Port", func() {
			bottomPort.EXPECT().PeekIncoming().Return(nil)

			madeProgress := rob.parseBottom()

			Expect(madeProgress).To(BeFalse())
		})

		It("should attach response to transaction", func() {
			rsp := &mem.WriteDoneRsp{}
			rsp.ID = sim.GetIDGenerator().Generate()
			rsp.RspTo = transaction.reqToBottom.Meta().ID

			bottomPort.EXPECT().PeekIncoming().Return(rsp)
			bottomPort.EXPECT().RetrieveIncoming()

			madeProgress := rob.parseBottom()

			Expect(madeProgress).To(BeTrue())
			Expect(transaction.rspFromBottom).To(BeIdenticalTo(rsp))
		})
	})

	Context("bottom up", func() {
		var (
			topModule     *MockPort
			writeFromTop  *mem.WriteReq
			rspFromBottom *mem.WriteDoneRsp
			transaction   *transaction
		)

		BeforeEach(func() {
			topModule = NewMockPort(mockCtrl)
			topModule.EXPECT().AsRemote().AnyTimes()
			writeFromTop = &mem.WriteReq{}
			writeFromTop.ID = sim.GetIDGenerator().Generate()
			writeFromTop.Src = topModule.AsRemote()

			rspFromBottom = &mem.WriteDoneRsp{}
			rspFromBottom.ID = sim.GetIDGenerator().Generate()
			rspFromBottom.RspTo = writeFromTop.ID
			transaction = rob.createTransaction(writeFromTop)
			transaction.rspFromBottom = rspFromBottom
			rob.addTransaction(transaction)
		})

		It("should do nothing if there is no transaction", func() {
			rob.transactions.Remove(rob.transactions.Front())

			madeProgress := rob.bottomUp()

			Expect(madeProgress).To(BeFalse())
		})

		It("should do nothing if the transaction is not ready", func() {
			transaction.rspFromBottom = nil

			madeProgress := rob.bottomUp()

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if TopPort is busy", func() {
			topPort.EXPECT().Send(gomock.Any()).Return(sim.NewSendError())

			madeProgress := rob.bottomUp()

			Expect(madeProgress).To(BeFalse())
			Expect(rob.transactions.Len()).To(Equal(1))
			Expect(rob.toBottomReqIDToTransactionTable).To(HaveLen(1))
		})

		It("should send response to top", func() {
			topPort.EXPECT().
				Send(gomock.Any()).
				Do(func(rsp *mem.WriteDoneRsp) {
					Expect(rsp.Dst).To(BeIdenticalTo(topModule.AsRemote()))
					Expect(rsp.Src).To(BeIdenticalTo(topPort.AsRemote()))
					Expect(rsp.RspTo).To(Equal(writeFromTop.ID))
				}).
				Return(nil)

			madeProgress := rob.bottomUp()

			Expect(madeProgress).To(BeTrue())
			Expect(rob.transactions.Len()).To(Equal(0))
			Expect(rob.toBottomReqIDToTransactionTable).To(HaveLen(0))
		})
	})

	Context("when processing control messages", func() {
		It("should flush", func() {
			flush := &mem.ControlReq{Command: mem.CmdFlush}
			flush.ID = sim.GetIDGenerator().Generate()

			ctrlPort.EXPECT().PeekIncoming().Return(flush)
			ctrlPort.EXPECT().RetrieveIncoming()
			ctrlPort.EXPECT().Send(gomock.Any()).Return(nil)

			madeProgress := rob.processControlMsg()

			Expect(madeProgress).To(BeTrue())
			Expect(rob.isFlushing).To(BeTrue())
		})

		It("should restart", func() {
			restart := &mem.ControlReq{Command: mem.CmdEnable}
			restart.ID = sim.GetIDGenerator().Generate()

			ctrlPort.EXPECT().PeekIncoming().Return(restart)
			ctrlPort.EXPECT().RetrieveIncoming()
			ctrlPort.EXPECT().Send(gomock.Any()).Return(nil)
			topPort.EXPECT().RetrieveIncoming().AnyTimes()
			bottomPort.EXPECT().RetrieveIncoming().AnyTimes()

			madeProgress := rob.processControlMsg()

			Expect(madeProgress).To(BeTrue())
			Expect(rob.isFlushing).To(BeFalse())
		})
	})

})
