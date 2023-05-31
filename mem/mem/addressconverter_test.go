package mem

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Interleaving Address Converter", func() {

	var (
		converter InterleavingConverter
	)

	BeforeEach(func() {
		converter.InterleavingSize = 4096
		converter.TotalNumOfElements = 8
		converter.CurrentElementIndex = 1
	})

	It("should convert from external to internal", func() {
		Expect(converter.ConvertExternalToInternal(4096)).
			To(Equal(uint64(0)))
		Expect(converter.ConvertExternalToInternal(4096*8 + 4096)).
			To(Equal(uint64(4096)))
		Expect(converter.ConvertExternalToInternal(4096*8*2 + 4096)).
			To(Equal(uint64(8192)))
		Expect(converter.ConvertExternalToInternal(4096*8*2 + 4096 + 100)).
			To(Equal(uint64(8292)))
	})

	It("should panic when converting address does not belongs to current element", func() {
		Expect(func() {
			converter.ConvertExternalToInternal(0)
		}).
			Should(Panic())
	})

})
