package trans

import (
	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/mgpusim/v3/mem/dram/internal/signal"
	"github.com/sarchlab/mgpusim/v3/mem/mem"
)

var _ = Describe("FCFSSubTransactionQueue", func() {
	var (
		mockCtrl   *gomock.Controller
		cmdQueue   *MockCommandQueue
		cmdCreator *MockCommandCreator
		queue      *FCFSSubTransactionQueue
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		cmdQueue = NewMockCommandQueue(mockCtrl)
		cmdCreator = NewMockCommandCreator(mockCtrl)
		queue = &FCFSSubTransactionQueue{
			Capacity:   4,
			CmdQueue:   cmdQueue,
			CmdCreator: cmdCreator,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should panic if the sub-trans count is larger than the queue size.",
		func() {
			Expect(func() { queue.CanPush(5) }).To(Panic())
		})

	It("should not allow pushing if there is no space in the queue.", func() {
		queue.Queue = make([]*signal.SubTransaction, 2)

		canPush := queue.CanPush(3)

		Expect(canPush).To(BeFalse())
	})

	It("should allow pushing if there is space", func() {
		queue.Queue = make([]*signal.SubTransaction, 1)

		canPush := queue.CanPush(3)

		Expect(canPush).To(BeTrue())
	})

	It("should panic if pushing too many sub-trans", func() {
		trans := &signal.Transaction{
			SubTransactions: make([]*signal.SubTransaction, 4),
		}
		queue.Queue = make([]*signal.SubTransaction, 2)

		Expect(func() { queue.Push(trans) }).To(Panic())
	})

	It("should push sub-transactions", func() {
		trans := &signal.Transaction{
			SubTransactions: make([]*signal.SubTransaction, 2),
		}
		queue.Queue = make([]*signal.SubTransaction, 2)

		queue.Push(trans)

		Expect(queue.Queue).To(HaveLen(4))
		Expect(queue.Queue[2]).To(BeIdenticalTo(trans.SubTransactions[0]))
		Expect(queue.Queue[3]).To(BeIdenticalTo(trans.SubTransactions[1]))
	})

	It("should add read command to queue", func() {
		read := mem.ReadReqBuilder{}.Build()
		trans := &signal.Transaction{
			Read: read,
		}
		subTrans := &signal.SubTransaction{
			Transaction: trans,
			Address:     0x40,
		}
		queue.Queue = []*signal.SubTransaction{subTrans}
		cmd := &signal.Command{}

		cmdCreator.EXPECT().Create(subTrans).Return(cmd)
		cmdQueue.EXPECT().CanAccept(cmd).Return(true)
		cmdQueue.EXPECT().Accept(cmd)

		madeProgress := queue.Tick(10)

		Expect(madeProgress).To(BeTrue())
		Expect(queue.Queue).NotTo(ContainElement(subTrans))
	})
})
