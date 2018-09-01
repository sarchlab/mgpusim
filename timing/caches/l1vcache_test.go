package caches

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/mem"
)

var _ = Describe("L1V Cache", func() {
	var (
		engine *core.MockEngine
		l1v    *L1VCache
	)

	BeforeEach(func() {
		engine = core.NewMockEngine()
		l1v = NewL1VCache("l1v", engine, 1)
		l1v.NumBank = 2
		l1v.BankInterleaving = 64
		l1v.DirectoryLookupLatency = 1
		l1v.DirectoryBusy = make([]directoryBusy, 2)
	})

	Context("when handling read req", func() {
		It("should move req to directory", func() {
			read := mem.NewReadReq(10, nil, l1v.ToCU, 0x100, 64)
			l1v.ToCU.Recv(read)

			l1v.parseFromCU(11)

			Expect(l1v.ToCU.Buf).To(HaveLen(0))
			Expect(l1v.DirectoryBusy[0].cycleLeft).To(Equal(1))
			Expect(l1v.DirectoryBusy[0].req).To(BeIdenticalTo(read))
			Expect(l1v.NeedTick).To(BeTrue())
		})

		It("should stall if directory is busy", func() {
			read1 := mem.NewReadReq(10, nil, l1v.ToCU, 0x100, 64)
			l1v.DirectoryBusy[0].req = read1

			read := mem.NewReadReq(10, nil, l1v.ToCU, 0x100, 64)
			l1v.ToCU.Recv(read)

			l1v.parseFromCU(11)

			Expect(l1v.ToCU.Buf).To(HaveLen(1))
			Expect(l1v.DirectoryBusy[0].req).To(BeIdenticalTo(read1))
			Expect(l1v.NeedTick).To(BeFalse())
		})
	})

})
