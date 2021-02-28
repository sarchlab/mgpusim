package writearound

import (
	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
	"gitlab.com/akita/util"
	"gitlab.com/akita/util/ca"
)

var _ = Describe("Directory", func() {
	var (
		mockCtrl        *gomock.Controller
		inBuf           *MockBuffer
		dir             *MockDirectory
		mshr            *MockMSHR
		bankBuf         *MockBuffer
		bottomPort      *MockPort
		lowModuleFinder *MockLowModuleFinder
		d               *directory
		c               *Cache
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		inBuf = NewMockBuffer(mockCtrl)
		dir = NewMockDirectory(mockCtrl)
		dir.EXPECT().WayAssociativity().Return(4).AnyTimes()
		mshr = NewMockMSHR(mockCtrl)
		bankBuf = NewMockBuffer(mockCtrl)
		bottomPort = NewMockPort(mockCtrl)
		lowModuleFinder = NewMockLowModuleFinder(mockCtrl)
		c = &Cache{
			log2BlockSize:    6,
			BottomPort:       bottomPort,
			directory:        dir,
			dirBuf:           inBuf,
			lowModuleFinder:  lowModuleFinder,
			mshr:             mshr,
			wayAssociativity: 4,
			bankBufs:         []util.Buffer{bankBuf},
		}
		c.TickingComponent = akita.NewTickingComponent(
			"cache", nil, 1, c)
		d = &directory{cache: c}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do nothing if no transaction", func() {
		inBuf.EXPECT().Peek().Return(nil)
		madeProgress := d.Tick(10)
		Expect(madeProgress).To(BeFalse())
	})

	Context("read mshr hit", func() {
		var (
			read  *mem.ReadReq
			trans *transaction
		)

		BeforeEach(func() {
			read = mem.ReadReqBuilder{}.
				WithSendTime(6).
				WithAddress(0x104).
				WithPID(1).
				WithByteSize(4).
				Build()

			trans = &transaction{
				read: read,
			}
			inBuf.EXPECT().Peek().Return(trans)
		})

		It("Should add to mshr entry", func() {
			mshrEntry := &cache.MSHREntry{}
			mshr.EXPECT().Query(ca.PID(1), uint64(0x100)).Return(mshrEntry)
			inBuf.EXPECT().Pop()

			madeProgress := d.Tick(10)

			Expect(madeProgress).To(BeTrue())
			Expect(mshrEntry.Requests).To(ContainElement(trans))
		})
	})

	Context("read hit", func() {
		var (
			block *cache.Block
			read  *mem.ReadReq
			trans *transaction
		)

		BeforeEach(func() {
			block = &cache.Block{
				IsValid: true,
			}
			read = mem.ReadReqBuilder{}.
				WithSendTime(6).
				WithAddress(0x104).
				WithPID(1).
				WithByteSize(4).
				Build()
			trans = &transaction{
				read: read,
			}
			inBuf.EXPECT().Peek().Return(trans)
			mshr.EXPECT().Query(ca.PID(1), gomock.Any()).Return(nil)
		})

		It("should send transaction to bank", func() {
			dir.EXPECT().Lookup(ca.PID(1), uint64(0x100)).Return(block)
			dir.EXPECT().Visit(block)
			bankBuf.EXPECT().CanPush().Return(true)
			bankBuf.EXPECT().Push(gomock.Any()).
				Do(func(t *transaction) {
					Expect(t.block).To(BeIdenticalTo(block))
					Expect(t.bankAction).To(Equal(bankActionReadHit))
				})
			inBuf.EXPECT().Pop()

			madeProgress := d.Tick(10)

			Expect(madeProgress).To(BeTrue())
			Expect(block.ReadCount).To(Equal(1))
		})

		It("should stall if cannot send to bank", func() {
			dir.EXPECT().Lookup(ca.PID(1), uint64(0x100)).Return(block)
			bankBuf.EXPECT().CanPush().Return(false)

			madeProgress := d.Tick(10)

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if block is locked", func() {
			block.IsLocked = true
			dir.EXPECT().Lookup(ca.PID(1), uint64(0x100)).Return(block)
			madeProgress := d.Tick(10)
			Expect(madeProgress).To(BeFalse())
		})
	})

	Context("read miss", func() {
		var (
			block     *cache.Block
			read      *mem.ReadReq
			trans     *transaction
			mshrEntry *cache.MSHREntry
		)

		BeforeEach(func() {
			block = &cache.Block{
				IsValid: true,
			}
			mshrEntry = &cache.MSHREntry{}
			read = mem.ReadReqBuilder{}.
				WithSendTime(6).
				WithAddress(0x104).
				WithPID(1).
				WithByteSize(4).
				Build()
			trans = &transaction{
				read: read,
			}
			inBuf.EXPECT().Peek().Return(trans)
			mshr.EXPECT().Query(ca.PID(1), gomock.Any()).Return(nil)
		})

		It("should send request to bottom", func() {
			var readToBottom *mem.ReadReq
			dir.EXPECT().Lookup(ca.PID(1), uint64(0x100)).Return(nil)
			dir.EXPECT().FindVictim(uint64(0x100)).Return(block)
			dir.EXPECT().Visit(block)
			lowModuleFinder.EXPECT().Find(uint64(0x100)).Return(nil)
			bottomPort.EXPECT().Send(gomock.Any()).Do(func(read *mem.ReadReq) {
				readToBottom = read
				Expect(read.Address).To(Equal(uint64(0x100)))
				Expect(read.AccessByteSize).To(Equal(uint64(64)))
				Expect(read.PID).To(Equal(ca.PID(1)))
			})
			mshr.EXPECT().IsFull().Return(false)
			mshr.EXPECT().Add(ca.PID(1), uint64(0x100)).Return(mshrEntry)
			inBuf.EXPECT().Pop()

			madeProgress := d.Tick(10)

			Expect(madeProgress).To(BeTrue())
			Expect(mshrEntry.Requests).To(ContainElement(trans))
			Expect(mshrEntry.Block).To(BeIdenticalTo(block))
			Expect(mshrEntry.ReadReq).To(BeIdenticalTo(readToBottom))
			Expect(block.Tag).To(Equal(uint64(0x100)))
			Expect(block.IsLocked).To(BeTrue())
			Expect(block.IsValid).To(BeTrue())
			Expect(trans.readToBottom).To(BeIdenticalTo(readToBottom))
			Expect(trans.block).To(BeIdenticalTo(block))
		})

		It("should stall is victim block is locked", func() {
			block.IsLocked = true
			dir.EXPECT().Lookup(ca.PID(1), uint64(0x100)).Return(nil)
			dir.EXPECT().FindVictim(uint64(0x100)).Return(block)

			madeProgress := d.Tick(10)

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall is victim block is being read", func() {
			block.ReadCount = 1
			dir.EXPECT().Lookup(ca.PID(1), uint64(0x100)).Return(nil)
			dir.EXPECT().FindVictim(uint64(0x100)).Return(block)

			madeProgress := d.Tick(10)

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall is mshr is full", func() {
			dir.EXPECT().Lookup(ca.PID(1), uint64(0x100)).Return(nil)
			dir.EXPECT().FindVictim(uint64(0x100)).Return(block)
			mshr.EXPECT().IsFull().Return(true)

			madeProgress := d.Tick(10)

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if send to bottom failed", func() {
			dir.EXPECT().Lookup(ca.PID(1), uint64(0x100)).Return(nil)
			dir.EXPECT().FindVictim(uint64(0x100)).Return(block)
			lowModuleFinder.EXPECT().Find(uint64(0x100)).Return(nil)
			mshr.EXPECT().IsFull().Return(false)
			bottomPort.EXPECT().Send(gomock.Any()).Return(&akita.SendError{})

			madeProgress := d.Tick(10)

			Expect(madeProgress).To(BeFalse())
		})
	})

	Context("write mshr hit", func() {
		var (
			write     *mem.WriteReq
			trans     *transaction
			mshrEntry *cache.MSHREntry
		)

		BeforeEach(func() {
			write = mem.WriteReqBuilder{}.
				WithSendTime(10).
				WithAddress(0x104).
				WithPID(1).
				WithData([]byte{1, 2, 3, 4}).
				Build()
			trans = &transaction{
				write: write,
			}
			mshrEntry = &cache.MSHREntry{}
		})

		It("should add to mshr entry", func() {
			var writeToBottom *mem.WriteReq
			inBuf.EXPECT().Peek().Return(trans)
			inBuf.EXPECT().Pop()
			mshr.EXPECT().Query(ca.PID(1), uint64(0x100)).Return(mshrEntry)
			lowModuleFinder.EXPECT().Find(uint64(0x104))
			bottomPort.EXPECT().Send(gomock.Any()).
				Do(func(write *mem.WriteReq) {
					writeToBottom = write
					Expect(write.Address).To(Equal(uint64(0x104)))
					Expect(write.Data).To(Equal([]byte{1, 2, 3, 4}))
					Expect(write.PID).To(Equal(ca.PID(1)))
				})

			madeProgress := d.Tick(10)

			Expect(madeProgress).To(BeTrue())
			Expect(mshrEntry.Requests).To(ContainElement(trans))
			Expect(trans.writeToBottom).To(BeIdenticalTo(writeToBottom))
		})
	})

	Context("write hit", func() {
		var (
			write *mem.WriteReq
			trans *transaction
			block *cache.Block
		)

		BeforeEach(func() {
			write = mem.WriteReqBuilder{}.
				WithSendTime(10).
				WithAddress(0x104).
				WithPID(1).
				WithData([]byte{1, 2, 3, 4}).
				Build()
			trans = &transaction{
				write: write,
			}
			block = &cache.Block{IsValid: true}
		})

		It("should send to bank", func() {
			inBuf.EXPECT().Peek().Return(trans)
			inBuf.EXPECT().Pop()
			mshr.EXPECT().Query(ca.PID(1), uint64(0x100)).Return(nil)
			dir.EXPECT().Lookup(ca.PID(1), uint64(0x100)).Return(block)
			dir.EXPECT().Visit(block)
			lowModuleFinder.EXPECT().Find(uint64(0x104))
			bankBuf.EXPECT().CanPush().Return(true)
			bankBuf.EXPECT().Push(gomock.Any()).
				Do(func(trans *transaction) {
					Expect(trans.bankAction).To(Equal(bankActionWrite))
					Expect(trans.block).To(BeIdenticalTo(block))
				})
			bottomPort.EXPECT().Send(gomock.Any()).
				Do(func(write *mem.WriteReq) {
					Expect(write.Address).To(Equal(uint64(0x104)))
					Expect(write.Data).To(Equal([]byte{1, 2, 3, 4}))
					Expect(write.PID).To(Equal(ca.PID(1)))
				})

			madeProgress := d.Tick(10)

			Expect(madeProgress).To(BeTrue())
			Expect(block.IsLocked).To(BeTrue())
			Expect(trans.writeToBottom).NotTo(BeNil())
		})

		It("should stall is the block is locked", func() {
			block.IsLocked = true

			inBuf.EXPECT().Peek().Return(trans)
			mshr.EXPECT().Query(ca.PID(1), uint64(0x100)).Return(nil)
			dir.EXPECT().Lookup(ca.PID(1), uint64(0x100)).Return(block)

			madeProgress := d.Tick(10)

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall is the block is being read", func() {
			block.ReadCount = 1

			inBuf.EXPECT().Peek().Return(trans)
			mshr.EXPECT().Query(ca.PID(1), uint64(0x100)).Return(nil)
			dir.EXPECT().Lookup(ca.PID(1), uint64(0x100)).Return(block)

			madeProgress := d.Tick(10)

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if bank buf is full", func() {
			inBuf.EXPECT().Peek().Return(trans)
			mshr.EXPECT().Query(ca.PID(1), uint64(0x100)).Return(nil)
			dir.EXPECT().Lookup(ca.PID(1), uint64(0x100)).Return(block)
			bankBuf.EXPECT().CanPush().Return(false)

			madeProgress := d.Tick(10)

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall is send to bottom failed", func() {
			inBuf.EXPECT().Peek().Return(trans)
			mshr.EXPECT().Query(ca.PID(1), uint64(0x100)).Return(nil)
			dir.EXPECT().Lookup(ca.PID(1), uint64(0x100)).Return(block)
			bankBuf.EXPECT().CanPush().Return(true)
			lowModuleFinder.EXPECT().Find(uint64(0x104))
			bottomPort.EXPECT().Send(gomock.Any()).Return(&akita.SendError{})

			madeProgress := d.Tick(10)

			Expect(madeProgress).To(BeFalse())
		})
	})

	Context("write miss", func() {
		var (
			write *mem.WriteReq
			trans *transaction
		)

		BeforeEach(func() {
			write = mem.WriteReqBuilder{}.
				WithSendTime(10).
				WithAddress(0x100).
				WithPID(1).
				WithData(make([]byte, 64)).
				Build()
			trans = &transaction{
				write: write,
			}
		})

		It("should send to bottom", func() {
			inBuf.EXPECT().Peek().Return(trans)
			inBuf.EXPECT().Pop()
			mshr.EXPECT().Query(ca.PID(1), uint64(0x100)).Return(nil)
			dir.EXPECT().Lookup(ca.PID(1), uint64(0x100)).Return(nil)
			lowModuleFinder.EXPECT().Find(uint64(0x100))
			bottomPort.EXPECT().Send(gomock.Any()).
				Do(func(write *mem.WriteReq) {
					Expect(write.Address).To(Equal(uint64(0x100)))
					Expect(write.Data).To(HaveLen(64))
					Expect(write.PID).To(Equal(ca.PID(1)))
				})

			madeProgress := d.Tick(10)

			Expect(madeProgress).To(BeTrue())
			Expect(trans.writeToBottom).NotTo(BeNil())
		})
	})

})
