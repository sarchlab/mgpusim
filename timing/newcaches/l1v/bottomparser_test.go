package l1v

import (
	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
	"gitlab.com/akita/util"
)

var _ = Describe("Bottom Parser", func() {
	var (
		mockCtrl          *gomock.Controller
		bottomPort        *MockPort
		bankBuf           *MockBuffer
		mshr              *MockMSHR
		postCTransactions []*transaction
		p                 *bottomParser
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		bottomPort = NewMockPort(mockCtrl)
		bankBuf = NewMockBuffer(mockCtrl)
		mshr = NewMockMSHR(mockCtrl)
		postCTransactions = nil
		p = &bottomParser{
			bottomPort:       bottomPort,
			mshr:             mshr,
			bankBufs:         []util.Buffer{bankBuf},
			transactions:     &postCTransactions,
			log2BlockSize:    6,
			wayAssociativity: 4,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do nothing if no respond", func() {
		bottomPort.EXPECT().Peek().Return(nil)
		madeProgress := p.Tick(12)
		Expect(madeProgress).To(BeFalse())
	})

	Context("write done", func() {
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
			bottomPort.EXPECT().Retrieve(gomock.Any())

			madeProgress := p.Tick(12)

			Expect(madeProgress).To(BeTrue())
			Expect(preCTrans1.doneFromBottom).To(BeIdenticalTo(done))
			Expect(preCTrans2.doneFromBottom).To(BeIdenticalTo(done))
			Expect(postCTransactions).NotTo(ContainElement(postCTrans))
		})
	})

	Context("data ready", func() {
		var (
			read1, read2           *mem.ReadReq
			preCTrans1, preCTrans2 *transaction
			postCRead              *mem.ReadReq
			readToBottom           *mem.ReadReq
			block                  *cache.Block
			postCTrans             *transaction
			dataReady              *mem.DataReadyRsp
		)

		BeforeEach(func() {
			read1 = mem.NewReadReq(1, nil, nil, 0x100, 4)
			read2 = mem.NewReadReq(1, nil, nil, 0x104, 4)
			preCTrans1 = &transaction{read: read1}
			preCTrans2 = &transaction{read: read2}
			postCRead = mem.NewReadReq(0, nil, nil, 0x100, 64)
			readToBottom = mem.NewReadReq(2, nil, nil, 0x100, 64)
			dataReady = mem.NewDataReadyRsp(4, nil, nil, readToBottom.GetID())
			block = &cache.Block{}
			postCTrans = &transaction{
				block:        block,
				read:         postCRead,
				readToBottom: readToBottom,
				preCoalesceTransactions: []*transaction{
					preCTrans1,
					preCTrans2,
				},
			}
			postCTransactions = append(postCTransactions, postCTrans)
		})

		It("should stall is bank is busy", func() {
			bottomPort.EXPECT().Peek().Return(dataReady)
			bankBuf.EXPECT().CanPush().Return(false)

			madeProgress := p.Tick(12)

			Expect(madeProgress).To(BeFalse())
		})

		It("should send transaction to bank", func() {
			bottomPort.EXPECT().Peek().Return(dataReady)
			bottomPort.EXPECT().Retrieve(gomock.Any())
			bankBuf.EXPECT().CanPush().Return(true)
			bankBuf.EXPECT().Push(gomock.Any()).
				Do(func(trans *transaction) {
					Expect(trans.bankAction).To(Equal(bankActionWriteFetched))
				})
			mshr.EXPECT().Remove(uint64(0x100))

			madeProgress := p.Tick(12)

			Expect(madeProgress).To(BeTrue())
			Expect(preCTrans1.dataReadyFromBottom).To(BeIdenticalTo(dataReady))
			Expect(preCTrans2.dataReadyFromBottom).To(BeIdenticalTo(dataReady))
			Expect(postCTransactions).NotTo(ContainElement(postCTrans))
		})
	})

})
