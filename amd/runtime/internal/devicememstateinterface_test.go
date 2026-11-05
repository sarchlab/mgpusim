package internal

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DeviceMemoryState", func() {
	var dms DeviceMemoryState

	BeforeEach(func() {
		dms = NewDeviceMemoryState(12)
		dms.setStorageSize(0x4000)
		dms.setInitialAddress(0x1000)
	})

	It("should allocate pages in increasing order", func() {
		Expect(dms.popNextAvailablePAddrs()).To(Equal(uint64(0x1000)))
		Expect(dms.allocateMultiplePages(2)).
			To(Equal([]uint64{0x2000, 0x3000}))
	})

	It("should never reuse a retired page", func() {
		addr := dms.popNextAvailablePAddrs()
		dms.retirePAddr(addr)

		Expect(dms.popNextAvailablePAddrs()).To(Equal(uint64(0x2000)))
	})

	It("should run out of memory at the end of the device", func() {
		Expect(dms.noAvailablePAddrs()).To(BeFalse())
		dms.allocateMultiplePages(4)
		Expect(dms.noAvailablePAddrs()).To(BeTrue())
	})
})
