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

		Expect(coalescedAddresses).To(Equal([]uint64{
			0, 0, 0, 0, 0, 0, 0, 0,
			0, 0, 0, 0, 0, 0, 0, 0,
		}))
	})

	It("should coalesced access to aligned addresses", func() {
		rawAddresses := make([]uint64, 64)
		for i := 0; i < 64; i++ {
			rawAddresses[i] = uint64(4 * i)
		}

		coalescedAddresses := coalescer.Coalesce(rawAddresses, 4)

		Expect(coalescedAddresses).To(Equal([]uint64{
			0x00, 0x00, 0x00, 0x00,
			0x40, 0x40, 0x40, 0x40,
			0x80, 0x80, 0x80, 0x80,
			0xc0, 0xc0, 0xc0, 0xc0,
		}))
	})

	It("should not coalesce in any other cases", func() {
		rawAddresses := make([]uint64, 64)
		for i := 0; i < 64; i++ {
			rawAddresses[i] = uint64(8 * i)
		}

		coalescedAddresses := coalescer.Coalesce(rawAddresses, 4)

		Expect(coalescedAddresses).To(Equal(rawAddresses))
	})
})
