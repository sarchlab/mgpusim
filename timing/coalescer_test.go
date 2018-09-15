package timing

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("DefaultCoalescer", func() {
	var (
		coalescer *DefaultCoalescer
	)

	BeforeEach(func() {
		coalescer = NewCoalescer()
	})

	It("should coalesce access to same address to 16 requests", func() {
		rawAddresses := []uint64{
			0, 0, 0, 0, 0, 0, 0, 0,
			0, 0, 0, 0, 0, 0, 0, 0,
			0, 0, 0, 0, 0, 0, 0, 0,
			0, 0, 0, 0, 0, 0, 0, 0,
			0, 0, 0, 0, 0, 0, 0, 0,
			0, 0, 0, 0, 0, 0, 0, 0,
			0, 0, 0, 0, 0, 0, 0, 0,
			0, 0, 0, 0, 0, 0, 0, 0,
		}

		coalescedAddresses := coalescer.Coalesce(rawAddresses, 4)

		expectedCoalescedAccesses := make([]CoalescedAccess, 0)
		for i := 0; i < 16; i++ {
			access := CoalescedAccess{0, 4,
				[]int{i * 4, i*4 + 1, i*4 + 2, i*4 + 3}}
			expectedCoalescedAccesses = append(
				expectedCoalescedAccesses, access)
		}
		Expect(coalescedAddresses).To(Equal(expectedCoalescedAccesses))
	})

	It("should coalesced access to aligned addresses", func() {
		rawAddresses := make([]uint64, 64)
		for i := 0; i < 64; i++ {
			rawAddresses[i] = uint64(4 * i)
		}

		coalescedAddresses := coalescer.Coalesce(rawAddresses, 4)

		expectedCoalescedAccesses := make([]CoalescedAccess, 0)
		for i := 0; i < 16; i++ {
			access := CoalescedAccess{uint64(0x10 * i), 16,
				[]int{i * 4, i*4 + 1, i*4 + 2, i*4 + 3}}
			expectedCoalescedAccesses = append(
				expectedCoalescedAccesses, access)
		}
		Expect(coalescedAddresses).To(Equal(expectedCoalescedAccesses))
	})

	It("should not coalesce in any other cases", func() {
		rawAddresses := make([]uint64, 64)
		for i := 0; i < 64; i++ {
			rawAddresses[i] = uint64(8 * i)
		}

		coalescedAddresses := coalescer.Coalesce(rawAddresses, 4)

		expectOutput := make([]CoalescedAccess, 64)
		for i := 0; i < 64; i++ {
			expectOutput[i].Addr = rawAddresses[i]
			expectOutput[i].Size = 4
			expectOutput[i].LaneIDs = []int{i}
		}

		Expect(coalescedAddresses).To(Equal(expectOutput))
	})
})
