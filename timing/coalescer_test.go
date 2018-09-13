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

		Expect(coalescedAddresses).To(Equal([]AddrSizePair{
			{0, 4},
			{0, 4},
			{0, 4},
			{0, 4},
			{0, 4},
			{0, 4},
			{0, 4},
			{0, 4},
			{0, 4},
			{0, 4},
			{0, 4},
			{0, 4},
			{0, 4},
			{0, 4},
			{0, 4},
			{0, 4},
		}))
	})

	It("should coalesced access to aligned addresses", func() {
		rawAddresses := make([]uint64, 64)
		for i := 0; i < 64; i++ {
			rawAddresses[i] = uint64(4 * i)
		}

		coalescedAddresses := coalescer.Coalesce(rawAddresses, 4)

		Expect(coalescedAddresses).To(Equal([]AddrSizePair{
			{0x00, 16},
			{0x10, 16},
			{0x20, 16},
			{0x30, 16},
			{0x40, 16},
			{0x50, 16},
			{0x60, 16},
			{0x70, 16},
			{0x80, 16},
			{0x90, 16},
			{0xa0, 16},
			{0xb0, 16},
			{0xc0, 16},
			{0xd0, 16},
			{0xe0, 16},
			{0xf0, 16},
		}))
	})

	It("should not coalesce in any other cases", func() {
		rawAddresses := make([]uint64, 64)
		for i := 0; i < 64; i++ {
			rawAddresses[i] = uint64(8 * i)
		}

		coalescedAddresses := coalescer.Coalesce(rawAddresses, 4)

		expectOutput := make([]AddrSizePair, 64)
		for i := 0; i < 64; i++ {
			expectOutput[i].Addr = rawAddresses[i]
			expectOutput[i].Size = 4
		}

		Expect(coalescedAddresses).To(Equal(expectOutput))
	})
})
