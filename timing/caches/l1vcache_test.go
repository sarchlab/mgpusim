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
		storage   *mem.Storage
		directory *cache.MockDirectory
		l1v       *L1VCache
	)

	BeforeEach(func() {
		engine = core.NewMockEngine()
		storage = mem.NewStorage(16 * mem.KB)
		directory = new(cache.MockDirectory)
		l1v = NewL1VCache("l1v", engine, 1)
		l1v.Directory = directory
		l1v.Storage = storage
		l1v.Latency = 8
	})

	Context("read hit", func() {
		var (
			block *cache.Block
			read  *mem.ReadReq
		)

		BeforeEach(func() {
			block = new(cache.Block)
			directory.ExpectLookup(0x100, block)

			read = mem.NewReadReq(10, nil, l1v.ToCU, 0x100, 64)
			l1v.ToCU.Recv(read)
		})

		It("should move req to directory", func() {
			l1v.parseFromCU(11)

			Expect(l1v.ToCU.Buf).To(HaveLen(0))
			Expect(l1v.cycleLeft).To(Equal(8))
			Expect(l1v.isBusy).To(BeTrue())
			Expect(l1v.reading).To(BeIdenticalTo(read))
			Expect(l1v.isStorageBusy).To(BeTrue())
			Expect(l1v.busyBlock).To(BeIdenticalTo(block))
			Expect(l1v.NeedTick).To(BeTrue())
		})

		It("should stall if cache is busy", func() {
			l1v.isBusy = true

			l1v.parseFromCU(11)

			Expect(l1v.ToCU.Buf).To(HaveLen(1))
			Expect(l1v.NeedTick).To(BeFalse())
		})

		It("should decrease cycleLeft", func() {
			l1v.isStorageBusy = true
			l1v.cycleLeft = 10

			l1v.doReadWrite(10)

			Expect(l1v.cycleLeft).To(Equal(9))
			Expect(l1v.NeedTick).To(BeTrue())
		})

		It("should finish read", func() {
			l1v.isStorageBusy = true
			l1v.cycleLeft = 1
			l1v.reading = read
			l1v.busyBlock = block

			l1v.doReadWrite(10)

			Expect(l1v.NeedTick).To(BeTrue())
			Expect(l1v.isStorageBusy).To(BeFalse())
			Expect(l1v.isBusy).To(BeFalse())
			Expect(l1v.toCUBuffer).To(HaveLen(1))
		})
	})

	Context("read miss", func() {
		var (
			block *cache.Block
			read  *mem.ReadReq
		)

		BeforeEach(func() {
			block = nil
			directory.ExpectLookup(0x100, block)

			read = mem.NewReadReq(10, nil, l1v.ToCU, 0x100, 64)
			l1v.ToCU.Recv(read)
		})

		It("should send read request to bottom", func() {
			l1v.parseFromCU(11)

			Expect(l1v.isBusy).To(BeTrue())
			Expect(l1v.reading).To(BeIdenticalTo(read))
			Expect(l1v.pendingDownGoingRead).To(HaveLen(1))
			Expect(l1v.toL2Buffer).To(HaveLen(1))
		})

		It("should not do read and write", func() {
			l1v.isStorageBusy = false
			l1v.cycleLeft = 10

			l1v.doReadWrite(10)

			Expect(l1v.cycleLeft).To(Equal(10))
			Expect(l1v.NeedTick).To(BeFalse())
		})
	})
})
