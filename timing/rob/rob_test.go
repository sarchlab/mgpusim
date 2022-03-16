package rob

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/akita/v3/sim"
	"gitlab.com/akita/mem/v3/mem"
)

var _ = Describe("Reorder Buffer", func() {
	var (
		mockCtrl   *gomock.Controller
		rob        *ReorderBuffer
		topPort    *MockPort
		bottomPort *MockPort
		ctrlPort   *MockPort
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		topPort = NewMockPort(mockCtrl)
		bottomPort = NewMockPort(mockCtrl)
		ctrlPort = NewMockPort(mockCtrl)

		rob = MakeBuilder().
			WithBufferSize(10).
			Build("rob")
		rob.topPort = topPort
		rob.bottomPort = bottomPort
		rob.controlPort = ctrlPort
		rob.BottomUnit = NewMockPort(mockCtrl)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("top-down", func() {
		var (
			read *mem.ReadReq
		)

		BeforeEach(func() {
			read = mem.ReadReqBuilder{}.Build()
		})

		It("should do nothing if buffer is full", func() {
			for i := 0; i < 10; i++ {
				req := mem.ReadReqBuilder{}.Build()
				trans := rob.createTransaction(req)
				rob.addTransaction(trans)
			}

			madeProgress := rob.topDown(10)

			Expect(madeProgress).To(BeFalse())
		})

		It("should do nothing if no message arriving", func() {
			topPort.EXPECT().Peek().Return(nil)

			madeProgress := rob.topDown(10)

			Expect(madeProgress).To(BeFalse())
		})

		It("should not receive request if bottom port is busy", func() {
			topPort.EXPECT().Peek().Return(read)
			bottomPort.EXPECT().
				Send(gomock.Any()).
				Return(sim.NewSendError())

			madeProgress := rob.topDown(10)

			Expect(madeProgress).To(BeFalse())
		})

		It("should accept request from top and forward to bottom", func() {
			topPort.EXPECT().Peek().Return(read)
			topPort.EXPECT().Retrieve(sim.VTimeInSec(10))
			bottomPort.EXPECT().
				Send(gomock.Any()).
				Do(func(req *mem.ReadReq) {
					Expect(req.Src).To(BeIdenticalTo(rob.bottomPort))
					Expect(req.Dst).To(BeIdenticalTo(rob.BottomUnit))
					Expect(req.SendTime).To(Equal(sim.VTimeInSec(10)))
				}).
				Return(nil)

			madeProgress := rob.topDown(10)

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
			writeFromTop = mem.WriteReqBuilder{}.Build()
			transaction = rob.createTransaction(writeFromTop)
			rob.addTransaction(transaction)
		})

		It("should do nothing if no response in the Bottom Port", func() {
			bottomPort.EXPECT().Peek().Return(nil)

			madeProgress := rob.parseBottom(10)

			Expect(madeProgress).To(BeFalse())
		})

		It("should attach response to transaction", func() {
			rsp := mem.WriteDoneRspBuilder{}.
				WithRspTo(transaction.reqToBottom.Meta().ID).
				Build()

			bottomPort.EXPECT().Peek().Return(rsp)
			bottomPort.EXPECT().Retrieve(sim.VTimeInSec(10))

			madeProgress := rob.parseBottom(10)

			Expect(madeProgress).To(BeTrue())
			Expect(transaction.rspFromBottom).To(BeIdenticalTo(rsp))
		})
	})

	Context("bottom up", func() {
		var (
			topModule     sim.Port
			writeFromTop  *mem.WriteReq
			rspFromBottom *mem.WriteDoneRsp
			transaction   *transaction
		)

		BeforeEach(func() {
			topModule = NewMockPort(mockCtrl)
			writeFromTop = mem.WriteReqBuilder{}.
				WithSrc(topModule).
				Build()
			rspFromBottom = mem.WriteDoneRspBuilder{}.
				WithRspTo(writeFromTop.ID).
				Build()
			transaction = rob.createTransaction(writeFromTop)
			transaction.rspFromBottom = rspFromBottom
			rob.addTransaction(transaction)
		})

		It("should do nothing if there is no transaction", func() {
			rob.transactions.Remove(rob.transactions.Front())

			madeProgress := rob.bottomUp(10)

			Expect(madeProgress).To(BeFalse())
		})

		It("should do nothing if the transaction is not ready", func() {
			transaction.rspFromBottom = nil

			madeProgress := rob.bottomUp(10)

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if TopPort is busy", func() {
			topPort.EXPECT().Send(gomock.Any()).Return(sim.NewSendError())

			madeProgress := rob.bottomUp(10)

			Expect(madeProgress).To(BeFalse())
			Expect(rob.transactions.Len()).To(Equal(1))
			Expect(rob.toBottomReqIDToTransactionTable).To(HaveLen(1))
		})

		It("should send response to top", func() {
			topPort.EXPECT().
				Send(gomock.Any()).
				Do(func(rsp *mem.WriteDoneRsp) {
					Expect(rsp.Dst).To(BeIdenticalTo(topModule))
					Expect(rsp.Src).To(BeIdenticalTo(topPort))
					Expect(rsp.SendTime).To(Equal(sim.VTimeInSec(10)))
					Expect(rsp.RespondTo).To(Equal(writeFromTop.ID))
				}).
				Return(nil)

			madeProgress := rob.bottomUp(10)

			Expect(madeProgress).To(BeTrue())
			Expect(rob.transactions.Len()).To(Equal(0))
			Expect(rob.toBottomReqIDToTransactionTable).To(HaveLen(0))
		})
	})

	Context("when processing control messages", func() {
		It("should flush", func() {
			flush := mem.ControlMsgBuilder{}.
				ToDiscardTransactions().
				Build()

			ctrlPort.EXPECT().Peek().Return(flush)
			ctrlPort.EXPECT().Retrieve(sim.VTimeInSec(10))
			ctrlPort.EXPECT().Send(gomock.Any()).Return(nil)

			madeProgress := rob.processControlMsg(10)

			Expect(madeProgress).To(BeTrue())
			Expect(rob.isFlushing).To(BeTrue())
		})

		It("should restart", func() {
			restart := mem.ControlMsgBuilder{}.
				ToRestart().
				Build()

			ctrlPort.EXPECT().Peek().Return(restart)
			ctrlPort.EXPECT().Retrieve(sim.VTimeInSec(10))
			ctrlPort.EXPECT().Send(gomock.Any()).Return(nil)
			topPort.EXPECT().Retrieve(sim.VTimeInSec(10)).AnyTimes()
			bottomPort.EXPECT().Retrieve(sim.VTimeInSec(10)).AnyTimes()

			madeProgress := rob.processControlMsg(10)

			Expect(madeProgress).To(BeTrue())
			Expect(rob.isFlushing).To(BeFalse())
		})
	})

})
