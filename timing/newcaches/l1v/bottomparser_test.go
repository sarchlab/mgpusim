package l1v

import (
	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/mem"
)

var _ = Describe("Bottom Parser", func() {
	var (
		mockCtrl          *gomock.Controller
		bottomPort        *MockPort
		postCTransactions []*transaction
		p                 *bottomParser
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		bottomPort = NewMockPort(mockCtrl)
		postCTransactions = nil
		p = &bottomParser{
			bottomPort:   bottomPort,
			transactions: &postCTransactions,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should handle write done", func() {
		write1 := mem.NewWriteReq(4, nil, nil, 0x100)
		preCTrans1 := &transaction{
			write: write1,
		}
		write2 := mem.NewWriteReq(4, nil, nil, 0x104)
		preCTrans2 := &transaction{
			write: write2,
		}
		writeToBottom := mem.NewWriteReq(6, nil, nil, 0x100)
		postCTrans := &transaction{
			writeToBottom:           writeToBottom,
			preCoalesceTransactions: []*transaction{preCTrans1, preCTrans2},
		}
		postCTransactions = append(postCTransactions, postCTrans)
		done := mem.NewDoneRsp(11, nil, nil, writeToBottom.GetID())

		bottomPort.EXPECT().Peek().Return(done)

		madeProgress := p.Tick(12)

		Expect(madeProgress).To(BeTrue())
		Expect(preCTrans1.doneFromBottom).To(BeIdenticalTo(done))
		Expect(preCTrans2.doneFromBottom).To(BeIdenticalTo(done))
		Expect(postCTransactions).NotTo(ContainElement(postCTrans))
	})

})
