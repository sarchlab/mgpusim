package l1v

import (
	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
	"gitlab.com/akita/util"
	"gitlab.com/akita/util/akitaext"
	"gitlab.com/akita/util/ca"
)

var _ = Describe("Bottom Parser", func() {
	var (
		mockCtrl   *gomock.Controller
		bottomPort *MockPort
		bankBuf    *MockBuffer
		mshr       *MockMSHR
		p          *bottomParser
		c          *Cache
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		bottomPort = NewMockPort(mockCtrl)
		bankBuf = NewMockBuffer(mockCtrl)
		mshr = NewMockMSHR(mockCtrl)
		c = &Cache{
			log2BlockSize:    6,
			BottomPort:       bottomPort,
			mshr:             mshr,
			wayAssociativity: 4,
			bankBufs:         []util.Buffer{bankBuf},
		}
		c.TickingComponent = akitaext.NewTickingComponent(
			"cache", nil, 1, c)
		p = &bottomParser{cache: c}
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
			write1 := mem.WriteReqBuilder{}.
				WithSendTime(4).
				WithAddress(0x100).
				WithPID(1).
				Build()
			preCTrans1 := &transaction{
				write: write1,
			}
			write2 := mem.WriteReqBuilder{}.
				WithSendTime(4).
				WithAddress(0x104).
				WithPID(1).
				Build()
			preCTrans2 := &transaction{
				write: write2,
			}
			writeToBottom := mem.WriteReqBuilder{}.
				WithSendTime(4).
				WithAddress(0x100).
				WithPID(1).
				Build()
			postCTrans := &transaction{
				writeToBottom:           writeToBottom,
				preCoalesceTransactions: []*transaction{preCTrans1, preCTrans2},
			}
			c.postCoalesceTransactions = append(
				c.postCoalesceTransactions, postCTrans)
			done := mem.WriteDoneRspBuilder{}.
				WithSendTime(11).
				WithRspTo(writeToBottom.ID).
				Build()

			bottomPort.EXPECT().Peek().Return(done)
			bottomPort.EXPECT().Retrieve(gomock.Any())

			madeProgress := p.Tick(12)

			Expect(madeProgress).To(BeTrue())
			Expect(preCTrans1.done).To(BeTrue())
			Expect(preCTrans2.done).To(BeTrue())
			Expect(c.postCoalesceTransactions).NotTo(ContainElement(postCTrans))
		})
	})

	Context("data ready", func() {
		var (
			read1, read2             *mem.ReadReq
			write1, write2           *mem.WriteReq
			preCTrans1, preCTrans2   *transaction
			preCTrans3, preCTrans4   *transaction
			postCRead                *mem.ReadReq
			postCWrite               *mem.WriteReq
			readToBottom             *mem.ReadReq
			block                    *cache.Block
			postCTrans1, postCTrans2 *transaction
			mshrEntry                *cache.MSHREntry
			dataReady                *mem.DataReadyRsp
		)

		BeforeEach(func() {
			read1 = mem.NewReadReq(1, nil, nil, 0x100, 4)
			read1.PID = 1
			read2 = mem.NewReadReq(1, nil, nil, 0x104, 4)
			read2.PID = 1
			write1 = mem.NewWriteReq(1, nil, nil, 0x108)
			write1.Data = []byte{9, 9, 9, 9}
			write1.PID = 1
			write2 = mem.NewWriteReq(1, nil, nil, 0x10C)
			write2.Data = []byte{9, 9, 9, 9}
			write2.PID = 1
			preCTrans1 = &transaction{read: read1}
			preCTrans2 = &transaction{read: read2}
			preCTrans3 = &transaction{write: write1}
			preCTrans4 = &transaction{write: write2}

			postCRead = mem.NewReadReq(0, nil, nil, 0x100, 64)
			postCRead.PID = 1
			readToBottom = mem.NewReadReq(2, nil, nil, 0x100, 64)
			readToBottom.PID = 1
			dataReady = mem.NewDataReadyRsp(4, nil, nil, readToBottom.GetID())
			dataReady.Data = []byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			}
			block = &cache.Block{
				PID: 1,
				Tag: 0x100,
			}
			postCTrans1 = &transaction{
				block:        block,
				read:         postCRead,
				readToBottom: readToBottom,
				preCoalesceTransactions: []*transaction{
					preCTrans1,
					preCTrans2,
				},
			}
			c.postCoalesceTransactions = append(c.postCoalesceTransactions, postCTrans1)

			postCWrite = mem.NewWriteReq(0, nil, nil, 0x100)
			postCWrite.Data = []byte{
				0, 0, 0, 0, 0, 0, 0, 0,
				9, 9, 9, 9, 9, 9, 9, 9,
			}
			postCWrite.DirtyMask = []bool{
				false, false, false, false, false, false, false, false,
				true, true, true, true, true, true, true, true,
			}
			postCTrans2 = &transaction{
				write: postCWrite,
				preCoalesceTransactions: []*transaction{
					preCTrans3, preCTrans4,
				},
			}

			mshrEntry = &cache.MSHREntry{
				Block: block,
			}
			mshrEntry.Requests = append(mshrEntry.Requests, postCTrans1)
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
			mshr.EXPECT().Query(ca.PID(1), uint64(0x100)).Return(mshrEntry)
			mshr.EXPECT().Remove(ca.PID(1), uint64(0x100))
			bankBuf.EXPECT().CanPush().Return(true)
			bankBuf.EXPECT().Push(gomock.Any()).
				Do(func(trans *transaction) {
					Expect(trans.bankAction).To(Equal(bankActionWriteFetched))
				})

			madeProgress := p.Tick(12)

			Expect(madeProgress).To(BeTrue())
			Expect(preCTrans1.done).To(BeTrue())
			Expect(preCTrans1.data).To(Equal([]byte{1, 2, 3, 4}))
			Expect(preCTrans2.done).To(BeTrue())
			Expect(preCTrans2.data).To(Equal([]byte{5, 6, 7, 8}))
			Expect(c.postCoalesceTransactions).NotTo(ContainElement(postCTrans1))
		})

		It("should combine write", func() {
			mshrEntry.Requests = append(mshrEntry.Requests, postCTrans2)
			c.postCoalesceTransactions = append(c.postCoalesceTransactions, postCTrans2)

			bottomPort.EXPECT().Peek().Return(dataReady)
			bottomPort.EXPECT().Retrieve(gomock.Any())
			mshr.EXPECT().Query(ca.PID(1), uint64(0x100)).Return(mshrEntry)
			mshr.EXPECT().Remove(ca.PID(1), uint64(0x100))
			bankBuf.EXPECT().CanPush().Return(true)
			bankBuf.EXPECT().Push(gomock.Any()).
				Do(func(trans *transaction) {
					Expect(trans.bankAction).To(Equal(bankActionWriteFetched))
					Expect(trans.data).To(Equal([]byte{
						1, 2, 3, 4, 5, 6, 7, 8,
						9, 9, 9, 9, 9, 9, 9, 9,
						1, 2, 3, 4, 5, 6, 7, 8,
						1, 2, 3, 4, 5, 6, 7, 8,
						1, 2, 3, 4, 5, 6, 7, 8,
						1, 2, 3, 4, 5, 6, 7, 8,
						1, 2, 3, 4, 5, 6, 7, 8,
						1, 2, 3, 4, 5, 6, 7, 8,
					}))
					Expect(trans.writeFetchedDirtyMask).To(Equal([]bool{
						false, false, false, false, false, false, false, false,
						true, true, true, true, true, true, true, true,
						false, false, false, false, false, false, false, false,
						false, false, false, false, false, false, false, false,
						false, false, false, false, false, false, false, false,
						false, false, false, false, false, false, false, false,
						false, false, false, false, false, false, false, false,
						false, false, false, false, false, false, false, false,
					}))
				})

			madeProgress := p.Tick(12)

			Expect(madeProgress).To(BeTrue())
			Expect(preCTrans1.done).To(BeTrue())
			Expect(preCTrans1.data).To(Equal([]byte{1, 2, 3, 4}))
			Expect(preCTrans2.done).To(BeTrue())
			Expect(preCTrans2.data).To(Equal([]byte{5, 6, 7, 8}))
			Expect(preCTrans3.done).To(BeTrue())
			Expect(preCTrans4.done).To(BeTrue())
			Expect(c.postCoalesceTransactions).NotTo(ContainElement(postCTrans1))
			Expect(c.postCoalesceTransactions).NotTo(ContainElement(postCTrans2))
		})
	})

})
