package internal

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Buddy Allocator Metadata Structures", func() {

	var (
		listElement *freeListElement
		bitfield    *bitField
	)

	BeforeEach(func() {
		e1 := &freeListElement{uint64(0x_0000_1000), nil,}
		e2 := &freeListElement{uint64(0x_0000_2000), nil,}
		e3 := &freeListElement{uint64(0x_0000_3000), nil,}
		e1.next = e2
		e2.next = e3
		listElement = e1

		bitfield = newBitField(1 << 20)
	})

	It("should push to back of the list", func() {
		pushBack(&listElement,0x_0000_4000)

		Expect(listElement.freeAddr).To(Equal(uint64(0x_0000_1000)))
		listElement = listElement.next
		Expect(listElement.freeAddr).To(Equal(uint64(0x_0000_2000)))
		listElement = listElement.next
		Expect(listElement.freeAddr).To(Equal(uint64(0x_0000_3000)))
		listElement = listElement.next
		Expect(listElement.freeAddr).To(Equal(uint64(0x_0000_4000)))
		Expect(listElement.next).To(BeNil())
	})

	It("should push add an element to a nil list", func() {
		var list *freeListElement
		Expect(list).To(BeNil())

		pushBack(&list, 0x0_0000_1000)

		Expect(list).To(Not(BeNil()))
	})

	It("should pop off the first element", func() {
		val := popFront(&listElement)

		Expect(val).To(Equal(uint64(0x_0000_1000)))
		Expect(listElement.freeAddr).To(Equal(uint64(0x_0000_2000)))
	})

	It("should remove a list element by value", func() {
		ok := removeByValue(&listElement, 0x_0000_1000)
		l := listElement
		Expect(ok).To(BeTrue())
		Expect(listElement.freeAddr).To(Equal(uint64(0x_0000_2000)))
		listElement = listElement.next
		Expect(listElement.freeAddr).To(Equal(uint64(0x_0000_3000)))
		Expect(listElement.next).To(BeNil())

		ok = removeByValue(&l, 0x_0000_1000)
		Expect(ok).To(BeFalse())
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