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

	It("should add PAddrs to regular DMS", func() {
		buddyDMS.addSinglePAddr(0x0_0000_1000)
		buddyDMS.addSinglePAddr(0x0_0000_2000)
		buddyDMS.addSinglePAddr(0x0_0000_3000)
		buddyDMS.addSinglePAddr(0x0_0000_4000)

	})

	It("should get next available PAddrs", func() {
		//if last test didn't work this probably won't either
		buddyDMS.addSinglePAddr(0x0_0000_1000)
		buddyDMS.addSinglePAddr(0x0_0000_2000)
		buddyDMS.addSinglePAddr(0x0_0000_3000)
		buddyDMS.addSinglePAddr(0x0_0000_4000)

		//addr1 := buddyDMS.popNextAvailablePAddrs()
		//addr2 := buddyDMS.popNextAvailablePAddrs()


	})

	It("should have no available PAddrs", func() {
		ok := buddyDMS.noAvailablePAddrs()
		Expect(ok).To(BeTrue())

		buddyDMS.addSinglePAddr(0x0_0000_1000)
		ok = buddyDMS.noAvailablePAddrs()
		Expect(ok).To(BeFalse())
	})

})