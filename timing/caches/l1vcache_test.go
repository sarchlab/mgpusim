package caches

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/mem"
	"gitlab.com/yaotsu/mem/cache"
)

var _ = Describe("L1V Cache", func() {
	var (
		engine    *core.MockEngine
		directory *cache.MockDirectory
		l1v       *L1VCache
	)

	BeforeEach(func() {
		engine = core.NewMockEngine()
		directory = new(cache.MockDirectory)
		l1v = NewL1VCache("l1v", engine, 1)
		l1v.Directory = directory
		l1v.NumBank = 2
		l1v.BankInterleaving = 64
		l1v.DirectoryLookupLatency = 1
		l1v.DirectoryBusy = make([]directoryBusy, 2)
		l1v.ReadLatency = 4
		l1v.WriteLatency = 8
		l1v.CacheRamBusy = make([]cacheRamBusy, 2)
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

	Context("after directory lookup", func() {
		Context("read hit", func() {
			var block *cache.Block

			BeforeEach(func() {
				block = new(cache.Block)
				directory.ExpectLookup(0x100, block)
			})

			It("should move req to cache ram", func() {
				read := mem.NewReadReq(10, nil, l1v.ToCU, 0x100, 64)

				l1v.DirectoryBusy[0].req = read
				l1v.DirectoryBusy[0].cycleLeft = 0

				l1v.lookupDirectory(11, 0)

				Expect(directory.AllExpectedCalled()).To(BeTrue())
				Expect(l1v.DirectoryBusy[0].req).To(BeNil())
				Expect(l1v.CacheRamBusy[0].req).To(BeIdenticalTo(read))
				Expect(l1v.CacheRamBusy[0].cycleLeft).To(Equal(4))
				Expect(l1v.NeedTick).To(BeTrue())
			})
		})

		Context("read miss", func() {
			var block *cache.Block

			BeforeEach(func() {
				block = nil
				directory.ExpectLookup(0x100, block)
			})

			It("read from bottom", func() {
			})
		})
	})

})
