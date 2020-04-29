package internal

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Buddy Allocator Metadata Structures", func() {

	var (
		listElement *freeListElement
	)

	BeforeEach(func() {
		e1 := &freeListElement{uint64(0x_0000_1000), nil,}
		e2 := &freeListElement{uint64(0x_0000_2000), nil,}
		e3 := &freeListElement{uint64(0x_0000_3000), nil,}
		e1.next = e2
		e2.next = e3
		listElement = e1
	})

	It("should push to back of the list", func() {
		pushBack(listElement,0x_0000_4000)

		Expect(listElement.freeAddr).To(Equal(uint64(0x_0000_1000)))
		listElement = listElement.next
		Expect(listElement.freeAddr).To(Equal(uint64(0x_0000_2000)))
		listElement = listElement.next
		Expect(listElement.freeAddr).To(Equal(uint64(0x_0000_3000)))
		listElement = listElement.next
		Expect(listElement.freeAddr).To(Equal(uint64(0x_0000_4000)))
		Expect(listElement.next).To(BeNil())

	})
})