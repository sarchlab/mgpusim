package internal

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Buddy Allocator Metadata Structures", func() {

	var (
		bitfield    *bitField
	)

	BeforeEach(func() {
		bitfield = newBitField(1 << 20)
	})

	It("should update a bit in the field", func() {
		bitfield.updateBit(0)
		Expect(bitfield.field[0]).To(Equal(uint64(0b_0001)))

		bitfield.updateBit(64)
		Expect(bitfield.field[1]).To(Equal(uint64(0b_0001)))

		bitfield.updateBit(64)
		Expect(bitfield.field[1]).To(Equal(uint64(0b_0000)))
	})

	It("should check if a bit is true in the field", func() {
		answer := bitfield.checkBit(65)
		Expect(answer).To(BeFalse())

		bitfield.updateBit(64)

		answer = bitfield.checkBit(65)
		Expect(answer).To(BeFalse())

		answer = bitfield.checkBit(64)
		Expect(answer).To(BeTrue())
	})
})