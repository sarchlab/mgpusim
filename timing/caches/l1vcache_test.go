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

	Context("parse read hit", func() {
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

	})

	Context("parse read miss", func() {
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
	})

	Context("during local read or local write", func() {
		It("should decrease cycleLeft", func() {
			l1v.isStorageBusy = true
			l1v.cycleLeft = 10

			l1v.doReadWrite(10)

			Expect(l1v.cycleLeft).To(Equal(9))
			Expect(l1v.NeedTick).To(BeTrue())
		})

		It("should do nothing if storage is not busy", func() {
			l1v.isStorageBusy = false
			l1v.cycleLeft = 10

			l1v.doReadWrite(10)

			Expect(l1v.cycleLeft).To(Equal(10))
			Expect(l1v.NeedTick).To(BeFalse())
		})
	})

	Context("complete read hit", func() {
		var (
			block *cache.Block
			read  *mem.ReadReq
		)

		BeforeEach(func() {
			block = new(cache.Block)
			read = mem.NewReadReq(10, nil, l1v.ToCU, 0x100, 64)
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

	Context("handle data-ready", func() {
		var (
			readFromTop         *mem.ReadReq
			readToBottom        *mem.ReadReq
			dataReadyFromBottom *mem.DataReadyRsp
			block               *cache.Block
		)

		BeforeEach(func() {
			readFromTop = mem.NewReadReq(
				5, nil, l1v.ToCU, 0x100, 64)
			readToBottom = mem.NewReadReq(
				8, l1v.ToL2, nil, 0x100, 64)
			dataReadyFromBottom = mem.NewDataReadyRsp(
				10, nil, l1v.ToL2, readToBottom.GetID())
			dataReadyFromBottom.Data = []byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			}

			block = new(cache.Block)
			block.IsValid = false
			block.CacheAddress = 0x200
			directory.ExpectEvict(0x100, block)

			l1v.pendingDownGoingRead = append(l1v.pendingDownGoingRead,
				readToBottom)
			l1v.reading = readFromTop
			l1v.isBusy = true

			l1v.ToL2.Recv(dataReadyFromBottom)
		})

		It("should respond data ready to cu and write to local", func() {
			l1v.parseFromL2(11)

			Expect(l1v.toCUBuffer).To(HaveLen(1))
			Expect(l1v.ToL2.Buf).To(HaveLen(0))
			Expect(l1v.pendingDownGoingRead).To(HaveLen(0))
			Expect(l1v.isBusy).To(BeFalse())
			Expect(l1v.NeedTick).To(BeTrue())

			data, _ := storage.Read(0x200, 64)
			Expect(data).To(Equal(dataReadyFromBottom.Data))
			Expect(block.IsValid).To(BeTrue())
		})
	})
})

var _ = Describe("L1VCache black box", func() {
	var (
		engine     core.Engine
		evictor    cache.Evictor
		storage    *mem.Storage
		directory  cache.Directory
		l1v        *L1VCache
		cu         *core.MockComponent
		lowModule  *mem.IdealMemController
		connection *core.DirectConnection
	)

	BeforeEach(func() {
		engine = core.NewSerialEngine()
		storage = mem.NewStorage(16 * mem.KB)
		evictor = cache.NewLRUEvictor()
		directory = cache.NewDirectory(64, 4, 64, evictor)
		l1v = NewL1VCache("l1v", engine, 1)
		l1v.Directory = directory
		l1v.Storage = storage
		l1v.Latency = 8

		lowModule = mem.NewIdealMemController("lowModule", engine, 4*mem.GB)
		lowModule.Storage.Write(0x100, []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		})
		l1v.L2 = lowModule.ToTop

		cu = core.NewMockComponent("cu")

		connection = core.NewDirectConnection(engine)
		connection.PlugIn(l1v.ToCU)
		connection.PlugIn(l1v.ToL2)
		connection.PlugIn(lowModule.ToTop)
		connection.PlugIn(cu.ToOutside)
	})

	It("should read miss", func() {
		read := mem.NewReadReq(10, cu.ToOutside, l1v.ToCU, 0x100, 64)
		l1v.ToCU.Recv(read)

		engine.Run()

		Expect(cu.ReceivedReqs).To(HaveLen(1))
	})
})
