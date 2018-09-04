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
		engine     *core.MockEngine
		storage    *mem.Storage
		directory  *cache.MockDirectory
		l1v        *L1VCache
		connection *core.MockConnection
	)

	BeforeEach(func() {
		engine = core.NewMockEngine()
		connection = core.NewMockConnection()
		storage = mem.NewStorage(16 * mem.KB)
		directory = new(cache.MockDirectory)
		l1v = NewL1VCache("l1v", engine, 1)
		l1v.Directory = directory
		l1v.Storage = storage
		l1v.Latency = 8
		l1v.BlockSizeAsPowerOf2 = 6

		connection.PlugIn(l1v.ToCU)
		connection.PlugIn(l1v.ToL2)
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
		})

		It("should start local read", func() {
			l1v.ToCU.Recv(read)

			l1v.parseFromCU(11)

			Expect(l1v.ToCU.Buf).To(HaveLen(0))
			Expect(l1v.cycleLeft).To(Equal(8))
			Expect(l1v.isBusy).To(BeTrue())
			Expect(l1v.reading).To(BeIdenticalTo(read))
			Expect(l1v.isStorageBusy).To(BeTrue())
			Expect(l1v.busyBlock).To(BeIdenticalTo(block))
			Expect(l1v.NeedTick).To(BeTrue())
		})

		It("should also start local read if reading partial line", func() {
			read.Address = 0x104
			read.MemByteSize = 4
			l1v.ToCU.Recv(read)

			l1v.parseFromCU(11)

			Expect(directory.AllExpectedCalled()).To(BeTrue())
			Expect(l1v.ToCU.Buf).To(HaveLen(0))
			Expect(l1v.cycleLeft).To(Equal(8))
			Expect(l1v.isBusy).To(BeTrue())
			Expect(l1v.reading).To(BeIdenticalTo(read))
			Expect(l1v.isStorageBusy).To(BeTrue())
			Expect(l1v.busyBlock).To(BeIdenticalTo(block))
			Expect(l1v.NeedTick).To(BeTrue())
		})

		It("should stall if cache is busy", func() {
			l1v.ToCU.Recv(read)
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

		It("always read a whole cache line from bottom", func() {
			read.Address = 0x104
			read.MemByteSize = 4

			l1v.parseFromCU(11)

			Expect(l1v.isBusy).To(BeTrue())
			Expect(l1v.reading).To(BeIdenticalTo(read))
			Expect(l1v.pendingDownGoingRead).To(HaveLen(1))
			Expect(l1v.toL2Buffer).To(HaveLen(1))
			Expect(l1v.toL2Buffer[0].(*mem.ReadReq).Address).To(
				Equal(uint64(0x100)))
			Expect(l1v.toL2Buffer[0].(*mem.ReadReq).MemByteSize).To(
				Equal(uint64(64)))
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
			block.IsValid = true
			block.CacheAddress = 0x200
			storage.Write(0x200, []byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			})

			read = mem.NewReadReq(10, nil, l1v.ToCU, 0x100, 64)

			l1v.isStorageBusy = true
			l1v.cycleLeft = 1
			l1v.reading = read
			l1v.busyBlock = block
		})

		It("should finish read", func() {
			l1v.doReadWrite(10)

			Expect(l1v.NeedTick).To(BeTrue())
			Expect(l1v.isStorageBusy).To(BeFalse())
			Expect(l1v.isBusy).To(BeFalse())
			Expect(l1v.toCUBuffer).To(HaveLen(1))
			Expect(l1v.toCUBuffer[0].(*mem.DataReadyRsp).Data).To(Equal([]byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			}))
		})

		It("should finish read if not accessing the whole cache line", func() {
			read.Address = 0x104
			read.MemByteSize = 4

			l1v.doReadWrite(10)

			Expect(l1v.NeedTick).To(BeTrue())
			Expect(l1v.isStorageBusy).To(BeFalse())
			Expect(l1v.isBusy).To(BeFalse())
			Expect(l1v.toCUBuffer).To(HaveLen(1))
			Expect(l1v.toCUBuffer[0].(*mem.DataReadyRsp).Data).To(
				Equal([]byte{5, 6, 7, 8}))
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
			Expect(l1v.toCUBuffer[0].(*mem.DataReadyRsp).Data).To(Equal([]byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			}))
			Expect(l1v.pendingDownGoingRead).To(HaveLen(0))
			Expect(l1v.isBusy).To(BeFalse())
			Expect(l1v.NeedTick).To(BeTrue())

			data, _ := storage.Read(0x200, 64)
			Expect(data).To(Equal(dataReadyFromBottom.Data))
			Expect(block.IsValid).To(BeTrue())
		})

		It("should return partial data if read from top is not reading full line", func() {
			readFromTop.Address = 0x104
			readFromTop.MemByteSize = 4

			l1v.parseFromL2(11)

			Expect(l1v.toCUBuffer).To(HaveLen(1))
			Expect(l1v.toCUBuffer[0].(*mem.DataReadyRsp).Data).To(Equal([]byte{
				5, 6, 7, 8,
			}))
			Expect(l1v.ToL2.Buf).To(HaveLen(0))
			Expect(l1v.pendingDownGoingRead).To(HaveLen(0))
			Expect(l1v.isBusy).To(BeFalse())
			Expect(l1v.NeedTick).To(BeTrue())

			data, _ := storage.Read(0x200, 64)
			Expect(data).To(Equal(dataReadyFromBottom.Data))
			Expect(block.IsValid).To(BeTrue())
		})
	})

	It("should handle write", func() {
		write := mem.NewWriteReq(10, nil, l1v.ToCU, 0x100)
		write.Data = []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		}

		block := new(cache.Block)
		block.CacheAddress = 0x200
		block.IsValid = false
		directory.ExpectEvict(0x100, block)

		l1v.ToCU.Recv(write)

		l1v.parseFromCU(11)

		Expect(l1v.isBusy).To(BeTrue())
		Expect(l1v.NeedTick).To(BeTrue())
		Expect(l1v.writing).To(BeIdenticalTo(write))
		Expect(l1v.toL2Buffer).To(HaveLen(1))
		Expect(l1v.pendingDownGoingWrite).To(HaveLen(1))
		Expect(l1v.ToCU.Buf).To(HaveLen(0))

		data, _ := storage.Read(0x200, 64)
		Expect(data).To(Equal([]byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		}))
		Expect(block.IsValid).To(BeTrue())
		Expect(block.Tag).To(Equal(uint64(0x100)))
	})

	It("should handle write when writing partial line", func() {
		write := mem.NewWriteReq(10, nil, l1v.ToCU, 0x104)
		write.Data = []byte{5, 6, 7, 8}

		block := new(cache.Block)
		block.CacheAddress = 0x200
		block.IsValid = false
		directory.ExpectEvict(0x100, block)

		l1v.ToCU.Recv(write)

		l1v.parseFromCU(11)

		Expect(l1v.isBusy).To(BeTrue())
		Expect(l1v.NeedTick).To(BeTrue())
		Expect(l1v.writing).To(BeIdenticalTo(write))
		Expect(l1v.toL2Buffer).To(HaveLen(1))
		Expect(l1v.pendingDownGoingWrite).To(HaveLen(1))
		Expect(l1v.ToCU.Buf).To(HaveLen(0))
		Expect(block.IsValid).To(BeFalse())
	})

	It("should handle done rsp", func() {
		writeFromTop := mem.NewWriteReq(6, nil, l1v.ToCU, 0x104)
		writeToBottom := mem.NewWriteReq(8, l1v.ToL2, nil, 0x104)
		doneRsp := mem.NewDoneRsp(10, nil, l1v.ToL2, writeToBottom.ID)

		l1v.writing = writeFromTop
		l1v.pendingDownGoingWrite = append(l1v.pendingDownGoingWrite, writeToBottom)

		l1v.ToL2.Recv(doneRsp)

		l1v.parseFromL2(11)

		Expect(l1v.isBusy).To(BeFalse())
		Expect(l1v.ToL2.Buf).To(HaveLen(0))
		Expect(l1v.pendingDownGoingWrite).To(HaveLen(0))
		Expect(l1v.NeedTick).To(BeTrue())
		Expect(l1v.writing).To(BeNil())
		Expect(l1v.toCUBuffer).To(HaveLen(1))
	})

	It("should send request to CU", func() {
		dataReady := mem.NewDataReadyRsp(10, l1v.ToCU, nil, "")
		l1v.toCUBuffer = append(l1v.toCUBuffer, dataReady)

		connection.ExpectSend(dataReady, nil)

		l1v.sendToCU(11)

		Expect(connection.AllExpectedSent()).To(BeTrue())
		Expect(l1v.NeedTick).To(BeTrue())
	})

	It("should send request to L2", func() {
		read := mem.NewReadReq(10, l1v.ToL2, nil, 0x100, 64)
		l1v.toL2Buffer = append(l1v.toL2Buffer, read)

		connection.ExpectSend(read, nil)

		l1v.sendToL2(11)

		Expect(connection.AllExpectedSent()).To(BeTrue())
		Expect(l1v.NeedTick).To(BeTrue())
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
		l1v.ToCU.BufCapacity = 2
		l1v.BlockSizeAsPowerOf2 = 6

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
		lowModule.Latency = 300
		lowModule.Freq = 1
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
		Expect(cu.ReceivedReqs[0].(*mem.DataReadyRsp).Data).To(Equal([]byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		}))
	})

	It("should read miss on partial line read", func() {
		read := mem.NewReadReq(10, cu.ToOutside, l1v.ToCU, 0x104, 4)
		l1v.ToCU.Recv(read)

		engine.Run()

		Expect(cu.ReceivedReqs).To(HaveLen(1))
		Expect(cu.ReceivedReqs[0].(*mem.DataReadyRsp).Data).To(
			Equal([]byte{5, 6, 7, 8}))
	})

	It("should read hit", func() {
		read := mem.NewReadReq(10, cu.ToOutside, l1v.ToCU, 0x100, 64)
		read1 := mem.NewReadReq(10, cu.ToOutside, l1v.ToCU, 0x100, 64)

		l1v.ToCU.Recv(read)
		l1v.ToCU.Recv(read1)

		engine.Run()

		Expect(cu.ReceivedReqs).To(HaveLen(2))

		Expect(cu.ReceivedReqs[0].(*mem.DataReadyRsp).Data).To(Equal([]byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		}))
		Expect(cu.ReceivedReqs[0].(*mem.DataReadyRsp).RespondTo).To(
			Equal(read.ID))

		Expect(cu.ReceivedReqs[1].(*mem.DataReadyRsp).Data).To(Equal([]byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		}))
		Expect(cu.ReceivedReqs[1].(*mem.DataReadyRsp).RespondTo).To(
			Equal(read1.ID))
	})

	It("should read hit on partial line read", func() {
		read := mem.NewReadReq(10, cu.ToOutside, l1v.ToCU, 0x104, 4)
		read1 := mem.NewReadReq(11, cu.ToOutside, l1v.ToCU, 0x108, 4)
		l1v.ToCU.Recv(read)
		l1v.ToCU.Recv(read1)

		engine.Run()

		Expect(cu.ReceivedReqs).To(HaveLen(2))
		Expect(cu.ReceivedReqs[0].(*mem.DataReadyRsp).Data).To(
			Equal([]byte{5, 6, 7, 8}))
		Expect(cu.ReceivedReqs[1].(*mem.DataReadyRsp).Data).To(
			Equal([]byte{1, 2, 3, 4}))
	})

	It("should write", func() {
		write := mem.NewWriteReq(10, cu.ToOutside, l1v.ToCU, 0x100)
		write.Data = []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		}
		l1v.ToCU.Recv(write)

		engine.Run()

		Expect(cu.ReceivedReqs).To(HaveLen(1))
		Expect(cu.ReceivedReqs[0].(*mem.DoneRsp).RespondTo).To(Equal(write.ID))
		data, _ := lowModule.Storage.Read(0x100, 64)
		Expect(data).To(Equal([]byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		}))
	})

	It("should read hit after writing a full cache line", func() {
		startTime := engine.CurrentTime()

		read := mem.NewReadReq(0, cu.ToOutside, l1v.ToCU, 0x104, 4)
		read.SetRecvTime(0)
		l1v.ToCU.Recv(read)
		engine.Run()
		Expect(cu.ReceivedReqs).To(HaveLen(1))
		time1 := engine.CurrentTime()
		duration1 := time1 - startTime

		write := mem.NewWriteReq(10, cu.ToOutside, l1v.ToCU, 0x140)
		write.SetRecvTime(time1 + 1)
		write.Data = []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		}
		l1v.ToCU.Recv(write)
		engine.Run()
		Expect(cu.ReceivedReqs).To(HaveLen(2))
		time2 := engine.CurrentTime()

		read.Address = 0x144
		read.SetRecvTime(time2 + 1)
		l1v.ToCU.Recv(read)
		engine.Run()
		Expect(cu.ReceivedReqs).To(HaveLen(3))
		time3 := engine.CurrentTime()
		duration2 := time3 - time2

		Expect(duration2).To(BeNumerically("<", duration1))

	})

})
