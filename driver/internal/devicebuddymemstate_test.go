package internal

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Implementation of buddy allocation deviceMemoryState", func() {

	buddyDMS := newDeviceBuddyMemoryState()

	BeforeEach(func() {
		buddyDMS.setStorageSize(0x1_0000_0000)
		buddyDMS.setInitialAddress(0x0_0000_1000)

	})

	It("should properly set up the storage size", func() {
		bDMS := newDeviceBuddyMemoryState()
		bDMS.setStorageSize(0x1_0000_0000)

		storagesize := bDMS.getStorageSize()
		b := bDMS.(*deviceBuddyMemoryState)
		listLength := len(b.freeList)

		Expect(1 << (listLength + 11)).To(Equal(int(storagesize)))

	})

	It("should set initial addr and add to free list", func() {
		bDMS := newDeviceBuddyMemoryState()
		bDMS.setStorageSize(0x1_0000_0000)

		bDMS.setInitialAddress(0x0_0000_1000)
		iAddr := bDMS.getInitialAddress()
		b := bDMS.(*deviceBuddyMemoryState)
		freeBlock := b.freeList[0]

		Expect(freeBlock).To(Not(BeNil()))
		Expect(freeBlock.freeAddr).To(Equal(iAddr))
	})

	It("should add PAddrs to regular DMS", func() {
		//buddyDMS.addSinglePAddr(0x0_0000_1000)
		//buddyDMS.addSinglePAddr(0x0_0000_2000)
		//buddyDMS.addSinglePAddr(0x0_0000_3000)
		//buddyDMS.addSinglePAddr(0x0_0000_4000)

	})

	It("should get next available PAddrs", func() {

		addr1 := buddyDMS.popNextAvailablePAddrs()
		addr2 := buddyDMS.popNextAvailablePAddrs()

		Expect(addr1).To(Equal(uint64(0x0_0000_1000)))
		Expect(addr2).To(Equal(uint64(0x0_0000_2000)))

		bDMS := buddyDMS.(*deviceBuddyMemoryState)

		Expect(bDMS.freeList[0]).To(BeNil())
		for i := len(bDMS.freeList)-2; i > 0; i-- {
			Expect(bDMS.freeList[i]).To(Not(BeNil()))
		}
		Expect(bDMS.freeList[len(bDMS.freeList)-1]).To(BeNil())
	})

	It("should allocate multiple PAddrs", func() {

		addrs := buddyDMS.allocateMultiplePages(3)

		Expect(addrs).To(HaveLen(3))
		Expect(addrs[0]).To(Equal(uint64(0x0_0000_1000)))
		Expect(addrs[1]).To(Equal(uint64(0x0_0000_2000)))
		Expect(addrs[2]).To(Equal(uint64(0x0_0000_3000)))

		bDMS := buddyDMS.(*deviceBuddyMemoryState)

		Expect(bDMS.freeList[0]).To(BeNil())

		for i := len(bDMS.freeList)-3; i > 0; i-- {
			Expect(bDMS.freeList[i]).To(Not(BeNil()))
		}
		Expect(bDMS.freeList[len(bDMS.freeList)-2]).To(BeNil())
		Expect(bDMS.freeList[len(bDMS.freeList)-1]).To(BeNil())
	})

	It("should allocate the whole space", func() {
		addrs := buddyDMS.allocateMultiplePages(1048555)
		Expect(addrs).To(HaveLen(1048555))

		ok := buddyDMS.noAvailablePAddrs()
		Expect(ok).To(BeTrue())
	})

	It("should find the proper buddy of a block", func() {
		bDMS := buddyDMS.(*deviceBuddyMemoryState)
		block := uint64(0x0_0000_1000)

		buddy := bDMS.buddyOf(block, 12)
		Expect(buddy).To(Equal(uint64(0x0_0000_2000)))

		buddy = bDMS.buddyOf(block, 13)
		Expect(buddy).To(Equal(uint64(0x0_0000_3000)))

		buddy = bDMS.buddyOf(block, 14)
		Expect(buddy).To(Equal(uint64(0x0_0000_5000)))
	})

	It("should find the size of the level", func() {
		bDMS := buddyDMS.(*deviceBuddyMemoryState)

		answer := bDMS.sizeOfLevel(0)
		Expect(answer).To(Equal(bDMS.storageSize))

		answer = bDMS.sizeOfLevel(1)
		Expect(answer).To(Equal(bDMS.storageSize/2))

		answer = bDMS.sizeOfLevel(2)
		Expect(answer).To(Equal(bDMS.storageSize/4))
	})

	It("should find the index of a block in their level", func() {
		bDMS := buddyDMS.(*deviceBuddyMemoryState)

		answer := bDMS.indexInLevelOf(0x0_0000_1000, 0)
		Expect(answer).To(Equal(uint64(0)))

		answer = bDMS.indexInLevelOf(0x0_0000_1000, 1)
		Expect(answer).To(Equal(uint64(0)))

		answer = bDMS.indexInLevelOf(0x0_8000_1000, 1)
		Expect(answer).To(Equal(uint64(1)))
	})

	It("should find the overall index of a block", func() {
		bDMS := buddyDMS.(*deviceBuddyMemoryState)

		answer := bDMS.indexOfBlock(0x0_0000_1000, 0)
		Expect(answer).To(Equal(uint64(0)))

		answer = bDMS.indexOfBlock(0x0_0000_1000, 1)
		Expect(answer).To(Equal(uint64(1)))

		answer = bDMS.indexOfBlock(0x0_0000_1000, 2)
		Expect(answer).To(Equal(uint64(3)))

		answer = bDMS.indexOfBlock(0x0_8000_1000, 2)
		Expect(answer).To(Equal(uint64(5)))
	})


	It("should have no available PAddrs", func() {
		bDMS := buddyDMS.(*deviceBuddyMemoryState)
		bDMS.freeList[0] = nil
		ok := buddyDMS.noAvailablePAddrs()
		Expect(ok).To(BeTrue())

		pushBack(&bDMS.freeList[0],0x0_0000_1000)
		ok = buddyDMS.noAvailablePAddrs()
		Expect(ok).To(BeFalse())
	})

})