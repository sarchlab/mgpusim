package l1v

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
		d = &directory{
			inBuf:           inBuf,
			dir:             dir,
			mshr:            mshr,
			bankBufs:        []util.Buffer{bankBuf},
			bottomPort:      bottomPort,
			lowModuleFinder: lowModuleFinder,
			log2BlockSize:   6,
		}
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
			read = mem.NewReadReq(6, nil, nil, 0x104, 4)
			trans = &transaction{
				read: read,
			}
			inBuf.EXPECT().Peek().Return(trans)
		})

		It("Should add to mshr entry", func() {
			mshrEntry := &cache.MSHREntry{}
			mshr.EXPECT().Query(uint64(0x100)).Return(mshrEntry)
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
			read = mem.NewReadReq(6, nil, nil, 0x104, 4)
			trans = &transaction{
				read: read,
			}
			inBuf.EXPECT().Peek().Return(trans)
			mshr.EXPECT().Query(gomock.Any()).Return(nil)
		})

		It("should send transaction to bank", func() {
			dir.EXPECT().Lookup(uint64(0x100)).Return(block)
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
			dir.EXPECT().Lookup(uint64(0x100)).Return(block)
			bankBuf.EXPECT().CanPush().Return(false)

			madeProgress := d.Tick(10)

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if block is locked", func() {
			block.IsLocked = true
			dir.EXPECT().Lookup(uint64(0x100)).Return(block)
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
			read = mem.NewReadReq(6, nil, nil, 0x104, 4)
			read.PID = 1
			trans = &transaction{
				read: read,
			}
			inBuf.EXPECT().Peek().Return(trans)
			mshr.EXPECT().Query(gomock.Any()).Return(nil)
		})

		It("should send request to bottom", func() {
			var readToBottom *mem.ReadReq
			dir.EXPECT().Lookup(uint64(0x100)).Return(nil)
			dir.EXPECT().FindVictim(uint64(0x100)).Return(block)
			dir.EXPECT().Visit(block)
			lowModuleFinder.EXPECT().Find(uint64(0x100)).Return(nil)
			bottomPort.EXPECT().Send(gomock.Any()).Do(func(read *mem.ReadReq) {
				readToBottom = read
				Expect(read.Address).To(Equal(uint64(0x100)))
				Expect(read.MemByteSize).To(Equal(uint64(64)))
				Expect(read.PID).To(Equal(ca.PID(1)))
			})
			mshr.EXPECT().IsFull().Return(false)
			mshr.EXPECT().Add(uint64(0x100)).Return(mshrEntry)
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
			dir.EXPECT().Lookup(uint64(0x100)).Return(nil)
			dir.EXPECT().FindVictim(uint64(0x100)).Return(block)

			madeProgress := d.Tick(10)

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall is victim block is being read", func() {
			block.ReadCount = 1
			dir.EXPECT().Lookup(uint64(0x100)).Return(nil)
			dir.EXPECT().FindVictim(uint64(0x100)).Return(block)

			madeProgress := d.Tick(10)

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall is mshr is full", func() {
			dir.EXPECT().Lookup(uint64(0x100)).Return(nil)
			dir.EXPECT().FindVictim(uint64(0x100)).Return(block)
			mshr.EXPECT().IsFull().Return(true)

			madeProgress := d.Tick(10)

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if send to bottom failed", func() {
			dir.EXPECT().Lookup(uint64(0x100)).Return(nil)
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
			write = mem.NewWriteReq(10, nil, nil, 0x104)
			write.Data = []byte{1, 2, 3, 4}
			write.PID = 1
			trans = &transaction{
				write: write,
			}
			mshrEntry = &cache.MSHREntry{}
		})

		It("should add to mshr entry", func() {
			var writeToBottom *mem.WriteReq
			inBuf.EXPECT().Peek().Return(trans)
			inBuf.EXPECT().Pop()
			mshr.EXPECT().Query(uint64(0x100)).Return(mshrEntry)
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
			write = mem.NewWriteReq(10, nil, nil, 0x104)
			write.Data = []byte{1, 2, 3, 4}
			write.PID = 1
			trans = &transaction{
				write: write,
			}
			block = &cache.Block{IsValid: true}
		})

		It("should send to bank", func() {
			inBuf.EXPECT().Peek().Return(trans)
			inBuf.EXPECT().Pop()
			mshr.EXPECT().Query(uint64(0x100)).Return(nil)
			dir.EXPECT().Lookup(uint64(0x100)).Return(block)
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
			mshr.EXPECT().Query(uint64(0x100)).Return(nil)
			dir.EXPECT().Lookup(uint64(0x100)).Return(block)

			madeProgress := d.Tick(10)

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall is the block is being read", func() {
			block.ReadCount = 1

			inBuf.EXPECT().Peek().Return(trans)
			mshr.EXPECT().Query(uint64(0x100)).Return(nil)
			dir.EXPECT().Lookup(uint64(0x100)).Return(block)

			madeProgress := d.Tick(10)

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall if bank buf is full", func() {
			inBuf.EXPECT().Peek().Return(trans)
			mshr.EXPECT().Query(uint64(0x100)).Return(nil)
			dir.EXPECT().Lookup(uint64(0x100)).Return(block)
			bankBuf.EXPECT().CanPush().Return(false)

			madeProgress := d.Tick(10)

			Expect(madeProgress).To(BeFalse())
		})

		It("should stall is send to bottom failed", func() {
			inBuf.EXPECT().Peek().Return(trans)
			mshr.EXPECT().Query(uint64(0x100)).Return(nil)
			dir.EXPECT().Lookup(uint64(0x100)).Return(block)
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
			block *cache.Block
		)

		BeforeEach(func() {
			write = mem.NewWriteReq(10, nil, nil, 0x104)
			write.Data = []byte{1, 2, 3, 4}
			write.PID = 1
			trans = &transaction{
				write: write,
			}
			block = &cache.Block{IsValid: true}
		})

		It("should write partial block", func() {
			inBuf.EXPECT().Peek().Return(trans)
			inBuf.EXPECT().Pop()
			mshr.EXPECT().Query(uint64(0x100)).Return(nil)
			dir.EXPECT().Lookup(uint64(0x100)).Return(nil)
			lowModuleFinder.EXPECT().Find(uint64(0x104))
			bottomPort.EXPECT().Send(gomock.Any()).
				Do(func(write *mem.WriteReq) {
					Expect(write.Address).To(Equal(uint64(0x104)))
					Expect(write.Data).To(HaveLen(4))
					Expect(write.PID).To(Equal(ca.PID(1)))
				})

			madeProgress := d.Tick(10)

			Expect(madeProgress).To(BeTrue())
			Expect(trans.writeToBottom).NotTo(BeNil())
		})

		It("should write full block", func() {
			block.Tag = 0x200
			block.IsValid = false
			write.Address = 0x100
			write.Data = make([]byte, 64)

			inBuf.EXPECT().Peek().Return(trans)
			inBuf.EXPECT().Pop()
			mshr.EXPECT().Query(uint64(0x100)).Return(nil)
			dir.EXPECT().Lookup(uint64(0x100)).Return(nil)
			dir.EXPECT().FindVictim(uint64(0x100)).Return(block)
			dir.EXPECT().Visit(block)
			bankBuf.EXPECT().CanPush().Return(true)
			bankBuf.EXPECT().Push(gomock.Any()).
				Do(func(trans *transaction) {
					Expect(trans.bankAction).To(Equal(bankActionWrite))
					Expect(trans.block).To(BeIdenticalTo(block))
				})
			lowModuleFinder.EXPECT().Find(uint64(0x100))
			bottomPort.EXPECT().Send(gomock.Any()).
				Do(func(write *mem.WriteReq) {
					Expect(write.Address).To(Equal(uint64(0x100)))
					Expect(write.Data).To(HaveLen(64))
					Expect(write.PID).To(Equal(ca.PID(1)))
				})

			madeProgress := d.Tick(10)

			Expect(madeProgress).To(BeTrue())
			Expect(block.IsLocked).To(BeTrue())
			Expect(block.Tag).To(Equal(uint64(0x100)))
			Expect(block.IsValid).To(BeTrue())
			Expect(trans.writeToBottom).NotTo(BeNil())
		})
	})

})
