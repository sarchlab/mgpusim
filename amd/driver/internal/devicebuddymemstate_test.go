package internal

import (
	"container/list"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Implementation of buddy allocation DeviceMemoryState", func() {

	buddyDMS := newDeviceBuddyMemoryState(12)

	BeforeEach(func() {
		buddyDMS.setStorageSize(0x1_0000_0000)
		buddyDMS.setInitialAddress(0x1_0000_1000)

	})

	It("should properly set up the storage size", func() {
		bDMS := newDeviceBuddyMemoryState(12)
		b := bDMS.(*deviceBuddyMemoryState)
		Expect(b.initFlag).To(BeFalse())
		Expect(b.freeList).To(HaveLen(0))
		bDMS.setStorageSize(0x1_0000_0000)
		Expect(b.freeList[0].Len()).To(BeZero())

		storagesize := bDMS.getStorageSize()

		listLength := len(b.freeList)

		Expect(1 << (listLength + 11)).To(Equal(int(storagesize)))

	})

	It("should set initial addr before storage size and add to free list", func() {
		bDMS := newDeviceBuddyMemoryState(12)

		bDMS.setInitialAddress(0x1_0000_1000)
		iAddr := bDMS.getInitialAddress()
		Expect(iAddr).To(Equal(uint64(0x1_0000_1000)))
		b := bDMS.(*deviceBuddyMemoryState)
		Expect(b.freeList).To(HaveLen(0))

		bDMS.setStorageSize(0x1_0000_0000)

		freeBlock := b.freeList[0].Front()
		Expect(freeBlock).To(Not(BeNil()))
		Expect(freeBlock.Value.(uint64)).To(Equal(iAddr))
	})

	It("should add PAddrs to buddy DMS", func() {
		addr1 := buddyDMS.popNextAvailablePAddrs()
		addr2 := buddyDMS.popNextAvailablePAddrs()

		buddyDMS.addSinglePAddr(addr1)

		bDMS := buddyDMS.(*deviceBuddyMemoryState)
		Expect(bDMS.freeList[0].Len()).To(BeZero())
		for i := len(bDMS.freeList) - 1; i > 0; i-- {
			Expect(bDMS.freeList[i].Len()).To(Not(BeZero()))
		}

		buddyDMS.addSinglePAddr(addr2)
		Expect(bDMS.freeList[0].Len()).To(Not(BeZero()))
		for i := len(bDMS.freeList) - 1; i > 0; i-- {
			Expect(bDMS.freeList[i].Len()).To(BeZero())
		}
	})

	It("should remove an value from the linked list", func() {
		l := list.New()
		addr1 := uint64(0x1000)
		addr2 := uint64(0x2000)
		addr3 := uint64(0x3000)
		addr4 := uint64(0x4000)
		l.PushBack(addr1)
		l.PushBack(addr2)
		l.PushBack(addr3)
		l.PushBack(addr4)
		Expect(l.Len()).To(Equal(4))
		removeByValue(l, addr2)
		Expect(l.Len()).To(Equal(3))
		removeByValue(l, addr3)
		Expect(l.Len()).To(Equal(2))
		removeByValue(l, addr1)
		Expect(l.Len()).To(Equal(1))
		ok := removeByValue(l, addr2)
		Expect(l.Len()).To(Equal(1))
		Expect(ok).To(BeFalse())
		removeByValue(l, addr4)
		Expect(l.Len()).To(Equal(0))
	})

	It("should get next available PAddrs", func() {
		addr1 := buddyDMS.popNextAvailablePAddrs()
		addr2 := buddyDMS.popNextAvailablePAddrs()

		Expect(addr1).To(Equal(uint64(0x1_0000_1000)))
		Expect(addr2).To(Equal(uint64(0x1_0000_2000)))

		bDMS := buddyDMS.(*deviceBuddyMemoryState)

		Expect(bDMS.freeList[0].Len()).To(BeZero())
		for i := len(bDMS.freeList) - 2; i > 0; i-- {
			Expect(bDMS.freeList[i].Len()).To(Not(BeZero()))
		}
		Expect(bDMS.freeList[len(bDMS.freeList)-1].Len()).To(BeZero())

		for i := 1; i < len(bDMS.freeList)-1; i++ {
			ok := bDMS.blockOrBuddyIsAllocated(addr1, i)
			Expect(ok).To(BeTrue())
		}
		ok := bDMS.blockOrBuddyIsAllocated(addr1, len(bDMS.freeList)-1)
		Expect(ok).To(BeFalse())
	})

	It("should allocate multiple PAddrs", func() {
		addrs := buddyDMS.allocateMultiplePages(3)

		Expect(addrs).To(HaveLen(3))
		Expect(addrs[0]).To(Equal(uint64(0x1_0000_1000)))
		Expect(addrs[1]).To(Equal(uint64(0x1_0000_2000)))
		Expect(addrs[2]).To(Equal(uint64(0x1_0000_3000)))

		bDMS := buddyDMS.(*deviceBuddyMemoryState)

		Expect(bDMS.freeList[0].Len()).To(BeZero())

		for i := len(bDMS.freeList) - 3; i > 0; i-- {
			Expect(bDMS.freeList[i].Len()).To(Not(BeZero()))
		}
		Expect(bDMS.freeList[len(bDMS.freeList)-2].Len()).To(BeZero())
		Expect(bDMS.freeList[len(bDMS.freeList)-1].Len()).To(BeZero())
	})

	It("should allocate the whole space", func() {
		addrs := buddyDMS.allocateMultiplePages(1048555)
		Expect(addrs).To(HaveLen(1048555))

		ok := buddyDMS.noAvailablePAddrs()
		Expect(ok).To(BeTrue())
	})

	It("should find the proper buddy of a block", func() {
		bDMS := buddyDMS.(*deviceBuddyMemoryState)
		block := uint64(0x1_0000_1000)

		buddy := bDMS.buddyOf(block, 20)
		Expect(buddy).To(Equal(uint64(0x1_0000_2000)))

		buddy = bDMS.buddyOf(block, 19)
		Expect(buddy).To(Equal(uint64(0x1_0000_3000)))

		buddy = bDMS.buddyOf(block, 18)
		Expect(buddy).To(Equal(uint64(0x1_0000_5000)))
	})

	It("should find the size of the level", func() {
		bDMS := buddyDMS.(*deviceBuddyMemoryState)

		answer := bDMS.sizeOfLevel(0)
		Expect(answer).To(Equal(bDMS.storageSize))

		answer = bDMS.sizeOfLevel(1)
		Expect(answer).To(Equal(bDMS.storageSize / 2))

		answer = bDMS.sizeOfLevel(2)
		Expect(answer).To(Equal(bDMS.storageSize / 4))
	})

	It("should find the index of a block in their level", func() {
		bDMS := buddyDMS.(*deviceBuddyMemoryState)

		answer := bDMS.indexInLevelOf(0x1_0000_1000, 0)
		Expect(answer).To(Equal(uint64(0)))

		answer = bDMS.indexInLevelOf(0x1_0000_1000, 1)
		Expect(answer).To(Equal(uint64(0)))

		answer = bDMS.indexInLevelOf(0x1_8000_1000, 1)
		Expect(answer).To(Equal(uint64(1)))
	})

	It("should find the overall index of a block", func() {
		bDMS := buddyDMS.(*deviceBuddyMemoryState)

		answer := bDMS.indexOfBlock(0x1_0000_1000, 0)
		Expect(answer).To(Equal(uint64(0)))

		answer = bDMS.indexOfBlock(0x1_0000_1000, 1)
		Expect(answer).To(Equal(uint64(1)))
		answer = bDMS.indexOfBlock(0x1_0000_2000, 1)
		Expect(answer).To(Equal(uint64(1)))

		answer = bDMS.indexOfBlock(0x1_0000_1000, 2)
		Expect(answer).To(Equal(uint64(3)))

		answer = bDMS.indexOfBlock(0x1_8000_1000, 2)
		Expect(answer).To(Equal(uint64(5)))
	})

	It("should update the bit field for which blocks are split", func() {
		bDMS := buddyDMS.(*deviceBuddyMemoryState)
		Expect(bDMS.bfBlockSplit.field[0]).To(Equal(uint64(0b_0000)))

		bDMS.updateSplitBlockBitField(0)
		Expect(bDMS.bfBlockSplit.field[0]).To(Equal(uint64(0b_0001)))

		bDMS.updateSplitBlockBitField(1)
		Expect(bDMS.bfBlockSplit.field[0]).To(Equal(uint64(0b_0011)))

		bDMS.updateSplitBlockBitField(2)
		Expect(bDMS.bfBlockSplit.field[0]).To(Equal(uint64(0b_0111)))

		bDMS.updateSplitBlockBitField(1)
		Expect(bDMS.bfBlockSplit.field[0]).To(Equal(uint64(0b_0101)))

		bDMS.updateSplitBlockBitField(1 << (len(bDMS.freeList) - 1))
	})

	It("should find the level of the block", func() {
		addr := buddyDMS.popNextAvailablePAddrs()

		bDMS := buddyDMS.(*deviceBuddyMemoryState)
		listLen := len(bDMS.freeList)

		level := bDMS.levelOfBlock(addr)
		Expect(level).To(Equal(listLen - 1))

		addrs := buddyDMS.allocateMultiplePages(2)
		level = bDMS.levelOfBlock(addrs[0])
		Expect(level).To(Equal(listLen - 2))
		level = bDMS.levelOfBlock(addrs[1])
		Expect(level).To(Equal(listLen - 2))
	})

	It("should check if block has been split", func() {
		addr := buddyDMS.popNextAvailablePAddrs()

		bDMS := buddyDMS.(*deviceBuddyMemoryState)
		listLen := len(bDMS.freeList)

		answer := bDMS.blockHasBeenSplit(addr, listLen-1)
		Expect(answer).To(BeFalse())

		answer = bDMS.blockHasBeenSplit(addr, listLen-2)
		Expect(answer).To(BeTrue())
	})

	It("should check if block or buddy is allocation", func() {
		bDMS := buddyDMS.(*deviceBuddyMemoryState)
		listLen := len(bDMS.freeList)

		answer := bDMS.blockOrBuddyIsAllocated(0x1_0000_1000, listLen-1)
		Expect(answer).To(BeFalse())
		answer = bDMS.blockOrBuddyIsAllocated(0x1_0000_2000, listLen-1)
		Expect(answer).To(BeFalse())
		answer = bDMS.blockOrBuddyIsAllocated(0x1_0000_1000, listLen-2)
		Expect(answer).To(BeFalse())
		answer = bDMS.blockOrBuddyIsAllocated(0x1_0000_2000, listLen-3)
		Expect(answer).To(BeFalse())

		_ = buddyDMS.popNextAvailablePAddrs()

		answer = bDMS.blockOrBuddyIsAllocated(0x1_0000_1000, listLen-1)
		Expect(answer).To(BeTrue())
		answer = bDMS.blockOrBuddyIsAllocated(0x1_0000_2000, listLen-1)
		Expect(answer).To(BeTrue())
		answer = bDMS.blockOrBuddyIsAllocated(0x1_0000_1000, listLen-2)
		Expect(answer).To(BeTrue())
		answer = bDMS.blockOrBuddyIsAllocated(0x1_0000_2000, listLen-3)
		Expect(answer).To(BeTrue())
	})

	It("should add one block then free that block", func() {
		addr := buddyDMS.popNextAvailablePAddrs()

		bDMS := buddyDMS.(*deviceBuddyMemoryState)
		bDMS.freeBlock(addr)

		Expect(bDMS.freeList[0].Len()).To(Not(BeZero()))
		for i := len(bDMS.freeList) - 1; i > 0; i-- {
			Expect(bDMS.freeList[i].Len()).To(BeZero())
		}

		for _, bits := range bDMS.bfMergeList.field {
			Expect(bits).To(Equal(uint64(0)))
		}
		for _, bits := range bDMS.bfBlockSplit.field {
			Expect(bits).To(Equal(uint64(0)))
		}
	})

	It("should add two blocks then free one block", func() {
		addr1 := buddyDMS.popNextAvailablePAddrs()
		_ = buddyDMS.popNextAvailablePAddrs()

		bDMS := buddyDMS.(*deviceBuddyMemoryState)
		bDMS.freeBlock(addr1)

		Expect(bDMS.freeList[0].Len()).To(BeZero())
		for i := len(bDMS.freeList) - 1; i > 0; i-- {
			Expect(bDMS.freeList[i].Len()).To(Not(BeZero()))
		}
	})

	It("should have no available PAddrs", func() {
		bDMS := buddyDMS.(*deviceBuddyMemoryState)
		bDMS.freeList[0].Init()
		ok := buddyDMS.noAvailablePAddrs()
		Expect(ok).To(BeTrue())

		bDMS.freeList[0].PushBack(0x1_0000_1000)
		ok = buddyDMS.noAvailablePAddrs()
		Expect(ok).To(BeFalse())
	})

})
