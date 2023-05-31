package vm

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("PageTable", func() {

	var (
		pageTable PageTable
		page      Page
	)

	BeforeEach(func() {
		page = Page{
			PID:      1,
			PAddr:    0x0,
			VAddr:    0x1000,
			PageSize: 4096,
			Valid:    true,
		}
		pageTable = NewPageTable(12)
	})

	It("should panic when inserting a page that is already exist", func() {
		pageTable.Insert(page)
		Expect(func() {
			pageTable.Insert(page)
		}).To(Panic())
	})

	It("should find page", func() {
		pageTable.Insert(page)
		retPage, found := pageTable.Find(1, 0x1000)
		Expect(found).To(BeTrue())
		Expect(retPage).To(Equal(page))
	})

	It("should find page if address is not aligned", func() {
		pageTable.Insert(page)
		retPage, found := pageTable.Find(1, 0x1024)
		Expect(found).To(BeTrue())
		Expect(retPage).To(Equal(page))
	})

	It("should remove page", func() {
		page1 := Page{PID: 1, VAddr: 0x1000, PageSize: 4096, Valid: true}
		pageTable.Insert(page1)
		page2 := Page{PID: 2, VAddr: 0x2000, PageSize: 4096, Valid: true}
		pageTable.Insert(page2)

		pageTable.Remove(2, 0x2000)

		retPage1, found1 := pageTable.Find(1, 0x1000)
		Expect(found1).To(BeTrue())
		Expect(retPage1).To(Equal(page1))
		_, found2 := pageTable.Find(2, 0x2000)
		Expect(found2).To(BeFalse())
	})
})
