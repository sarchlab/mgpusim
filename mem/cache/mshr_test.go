package cache

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type mockAccessReq struct {
	address uint64
}

func (r *mockAccessReq) GetAddress() uint64 {
	return r.address
}

var _ = Describe("MSHRImpl", func() {

	var (
		m *mshrImpl
	)

	BeforeEach(func() {
		m = NewMSHR(4).(*mshrImpl)
	})

	It("should add an entry", func() {
		entry := m.Add(1, 0x00)
		Expect(entry).NotTo(BeNil())
	})

	It("should panic if adding an address that is already in MSHR", func() {
		m.Add(1, 0x00)
		Expect(func() { m.Add(1, 0x00) }).To(Panic())
	})

	It("should panic if adding to a full MSHR", func() {
		m.Add(1, 0x00)
		m.Add(1, 0x40)
		m.Add(1, 0x80)
		m.Add(1, 0xc0)
		Expect(func() { m.Add(1, 0x100) }).To(Panic())
	})

	It("should say full if the MSHR is full", func() {
		m.Add(1, 0x00)
		m.Add(1, 0x40)
		m.Add(1, 0x80)
		m.Add(1, 0xc0)
		Expect(m.IsFull()).To(BeTrue())
	})

	It("should say not full if the MSHR is not full", func() {
		m.Add(1, 0x00)
		m.Add(1, 0x40)
		m.Add(1, 0xc0)
		Expect(m.IsFull()).To(BeFalse())
	})

	It("should send back the entry if querying a address in MSHR", func() {
		entry := m.Add(1, 0x40)
		entryQuery := m.Query(1, 0x40)
		Expect(entry).To(BeIdenticalTo(entryQuery))
	})

	It("should send nil the entry if querying another address in MSHR", func() {
		m.Add(1, 0x20)
		entryQuery := m.Query(1, 0x40)
		Expect(entryQuery).To(BeNil())
	})

	It("should send nil the entry if querying another pid in MSHR", func() {
		m.Add(1, 0x20)
		entryQuery := m.Query(2, 0x20)
		Expect(entryQuery).To(BeNil())
	})

	It("should remove the mshr entry", func() {
		entry := m.Add(1, 0x40)
		entryRemove := m.Remove(1, 0x40)
		Expect(len(m.entries)).To(Equal(0))
		Expect(entry).To(BeIdenticalTo(entryRemove))
	})

	It("should panic on removing an non-exist entry", func() {
		m.Add(1, 0x80)
		Expect(func() { m.Remove(1, 0x40) }).To(Panic())
	})

})
