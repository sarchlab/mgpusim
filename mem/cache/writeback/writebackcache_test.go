package writeback

import (
	"log"
	"testing"

	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/mem/mem"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/mgpusim/v3/mem/cache"
	"github.com/sarchlab/mgpusim/v3/mem/idealmemcontroller"
)

//go:generate mockgen -destination "mock_cache_test.go" -package $GOPACKAGE  -write_package_comment=false github.com/sarchlab/mgpusim/v3/mem/cache Directory,MSHR
//go:generate mockgen -destination "mock_mem_test.go" -package $GOPACKAGE  -write_package_comment=false github.com/sarchlab/mgpusim/v3/mem/mem LowModuleFinder
//go:generate mockgen -destination "mock_sim_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v3/sim Port,Engine,Buffer,BufferedSender
//go:generate mockgen -destination "mock_pipelining_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v3/pipelining Pipeline

func TestCache(t *testing.T) {
	log.SetOutput(GinkgoWriter)
	RegisterFailHandler(Fail)
	RunSpecs(t, "Write-Back Suite")
}

var _ = Describe("Write-Back Cache Integration", func() {

	var (
		mockCtrl         *gomock.Controller
		engine           sim.Engine
		victimFinder     *cache.LRUVictimFinder
		directory        *cache.DirectoryImpl
		lowModuleFinder  *mem.SingleLowModuleFinder
		storage          *mem.Storage
		cacheModule      *Cache
		dram             *idealmemcontroller.Comp
		conn             *sim.DirectConnection
		agentPort        *MockPort
		controlAgentPort *MockPort
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		agentPort = NewMockPort(mockCtrl)
		agentPort.EXPECT().SetConnection(gomock.Any()).AnyTimes()
		controlAgentPort = NewMockPort(mockCtrl)
		controlAgentPort.EXPECT().SetConnection(gomock.Any()).AnyTimes()

		engine = sim.NewSerialEngine()
		directory = cache.NewDirectory(1024, 4, 64, victimFinder)
		lowModuleFinder = &mem.SingleLowModuleFinder{}
		storage = mem.NewStorage(1024 * 4 * 64)

		builder := MakeBuilder().
			WithEngine(engine).
			WithByteSize(1024 * 4 * 64).
			WithNumReqPerCycle(4).
			WithLowModuleFinder(lowModuleFinder)
		cacheModule = builder.Build("Cache")
		cacheModule.directory = directory
		cacheModule.storage = storage

		dram = idealmemcontroller.New("Dram", engine, 4*mem.GB)
		dram.Freq = 1 * sim.GHz
		dram.Latency = 200

		lowModuleFinder.LowModule = dram.GetPortByName("Top")

		conn = sim.NewDirectConnection("Connection", engine, 1*sim.GHz)
		conn.PlugIn(cacheModule.topPort, 10)
		conn.PlugIn(cacheModule.bottomPort, 10)
		conn.PlugIn(cacheModule.controlPort, 10)
		conn.PlugIn(dram.GetPortByName("Top"), 10)
		conn.PlugIn(agentPort, 10)
		conn.PlugIn(controlAgentPort, 10)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do read hit", func() {
		block := directory.Sets[0].Blocks[0]
		block.Tag = 0x10000
		block.IsValid = true
		storage.Write(block.CacheAddress, []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		})

		read := mem.ReadReqBuilder{}.
			WithSendTime(10).
			WithSrc(agentPort).
			WithDst(cacheModule.topPort).
			WithAddress(0x10004).
			WithByteSize(4).
			Build()
		read.RecvTime = 10
		cacheModule.topPort.Recv(read)

		agentPort.EXPECT().Recv(gomock.Any()).
			Do(func(dr *mem.DataReadyRsp) {
				Expect(dr.Data).To(Equal([]byte{5, 6, 7, 8}))
				Expect(dr.RespondTo).To(Equal(read.ID))
			})

		engine.Run()

		Expect(directory.Sets[0].LRUQueue[3]).To(BeIdenticalTo(block))
	})

	It("should write hit", func() {
		block := directory.Sets[0].Blocks[0]
		block.Tag = 0x10000
		block.IsValid = true
		storage.Write(block.CacheAddress, []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		})

		write := mem.WriteReqBuilder{}.
			WithSendTime(10).
			WithSrc(agentPort).
			WithDst(cacheModule.topPort).
			WithAddress(0x10004).
			WithData([]byte{9, 9, 9, 9}).
			Build()
		write.RecvTime = 10
		cacheModule.topPort.Recv(write)

		agentPort.EXPECT().Recv(gomock.Any()).
			Do(func(done *mem.WriteDoneRsp) {
				Expect(done.RespondTo).To(Equal(write.ID))
			})

		engine.Run()

		retData, _ := storage.Read(0x4, 4)
		Expect(retData).To(Equal(write.Data))
		Expect(block.Tag).To(Equal(uint64(0x10000)))
		Expect(block.IsValid).To(BeTrue())
		Expect(block.IsDirty).To(BeTrue())
		Expect(directory.Sets[0].LRUQueue[3]).To(BeIdenticalTo(block))
	})

	It("should handle read miss, mshr hit", func() {
		dram.Storage.Write(0x10000, []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		})

		read1 := mem.ReadReqBuilder{}.
			WithSendTime(10).
			WithSrc(agentPort).
			WithDst(cacheModule.topPort).
			WithAddress(0x10004).
			WithByteSize(4).
			Build()
		read1.RecvTime = 10
		cacheModule.topPort.Recv(read1)

		read2 := mem.ReadReqBuilder{}.
			WithSendTime(10).
			WithSrc(agentPort).
			WithDst(cacheModule.topPort).
			WithAddress(0x10008).
			WithByteSize(4).
			Build()
		read2.RecvTime = 10
		cacheModule.topPort.Recv(read2)

		agentPort.EXPECT().Recv(gomock.Any()).
			Do(func(dr *mem.DataReadyRsp) {
				Expect(dr.Data).To(Equal([]byte{5, 6, 7, 8}))
				Expect(dr.RespondTo).To(Equal(read1.ID))
			})

		agentPort.EXPECT().Recv(gomock.Any()).
			Do(func(dr *mem.DataReadyRsp) {
				Expect(dr.Data).To(Equal([]byte{1, 2, 3, 4}))
				Expect(dr.RespondTo).To(Equal(read2.ID))
			})

		engine.Run()

		block := directory.Sets[0].Blocks[0]
		Expect(directory.Sets[0].LRUQueue[3]).To(BeIdenticalTo(block))
	})

	It("should handle write miss, mshr hit", func() {
		dram.Storage.Write(0x10000,
			[]byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			})

		read1 := mem.ReadReqBuilder{}.
			WithSendTime(10).
			WithSrc(agentPort).
			WithDst(cacheModule.topPort).
			WithAddress(0x10004).
			WithByteSize(4).
			Build()
		read1.RecvTime = 10
		cacheModule.topPort.Recv(read1)

		write := mem.WriteReqBuilder{}.
			WithSendTime(10).
			WithSrc(agentPort).
			WithDst(cacheModule.topPort).
			WithAddress(0x10008).
			WithData([]byte{9, 9, 9, 9}).
			Build()
		write.RecvTime = 10
		cacheModule.topPort.Recv(write)

		read2 := mem.ReadReqBuilder{}.
			WithSendTime(10).
			WithSrc(agentPort).
			WithDst(cacheModule.topPort).
			WithAddress(0x10008).
			WithByteSize(4).
			Build()
		read2.RecvTime = 10
		cacheModule.topPort.Recv(read2)

		agentPort.EXPECT().Recv(gomock.Any()).
			Do(func(dr *mem.DataReadyRsp) {
				Expect(dr.Data).To(Equal([]byte{5, 6, 7, 8}))
				Expect(dr.RespondTo).To(Equal(read1.ID))
			})

		agentPort.EXPECT().Recv(gomock.Any()).
			Do(func(done *mem.WriteDoneRsp) {
				Expect(done.RespondTo).To(Equal(write.ID))
			})

		agentPort.EXPECT().Recv(gomock.Any()).
			Do(func(dr *mem.DataReadyRsp) {
				Expect(dr.Data).To(Equal([]byte{9, 9, 9, 9}))
				Expect(dr.RespondTo).To(Equal(read2.ID))
			})

		engine.Run()

		block := directory.Sets[0].Blocks[0]
		Expect(directory.Sets[0].LRUQueue[3]).To(BeIdenticalTo(block))
	})

	It("should do read miss, mshr miss, w/ fetch, w/o eviction", func() {
		dram.Storage.Write(0x10000, []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		})

		read := mem.ReadReqBuilder{}.
			WithSendTime(10).
			WithSrc(agentPort).
			WithDst(cacheModule.topPort).
			WithAddress(0x10004).
			WithByteSize(4).
			Build()
		read.RecvTime = 10
		cacheModule.topPort.Recv(read)

		agentPort.EXPECT().Recv(gomock.Any()).Do(func(dr *mem.DataReadyRsp) {
			Expect(dr.Data).To(Equal([]byte{5, 6, 7, 8}))
			Expect(dr.RespondTo).To(Equal(read.ID))
		})

		engine.Run()

		block := directory.Sets[0].Blocks[0]
		Expect(directory.Sets[0].LRUQueue[3]).To(BeIdenticalTo(block))
	})

	It("should do write miss, mshr miss, w/ fetch, w/o eviction", func() {
		dram.Storage.Write(0x10000, []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		})

		write := mem.WriteReqBuilder{}.
			WithSendTime(10).
			WithSrc(agentPort).
			WithDst(cacheModule.topPort).
			WithAddress(0x10004).
			WithData([]byte{9, 9, 9, 9}).
			Build()
		write.RecvTime = 10
		cacheModule.topPort.Recv(write)

		read := mem.ReadReqBuilder{}.
			WithSendTime(10).
			WithSrc(agentPort).
			WithDst(cacheModule.topPort).
			WithAddress(0x10000).
			WithByteSize(8).
			Build()
		read.RecvTime = 10
		cacheModule.topPort.Recv(read)

		agentPort.EXPECT().Recv(gomock.Any()).
			Do(func(done *mem.WriteDoneRsp) {
				Expect(done.RespondTo).To(Equal(write.ID))
			})
		agentPort.EXPECT().Recv(gomock.Any()).
			Do(func(dr *mem.DataReadyRsp) {
				Expect(dr.Data).To(Equal([]byte{1, 2, 3, 4, 9, 9, 9, 9}))
				Expect(dr.RespondTo).To(Equal(read.ID))
			})

		engine.Run()

		block := directory.Sets[0].Blocks[0]
		Expect(directory.Sets[0].LRUQueue[3]).To(BeIdenticalTo(block))
	})

	It("should handle write miss, mshr miss, w/o fetch, w/o eviction", func() {
		write := mem.WriteReqBuilder{}.
			WithSendTime(10).
			WithSrc(agentPort).
			WithDst(cacheModule.topPort).
			WithAddress(0x10000).
			WithData([]byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			}).
			Build()
		write.RecvTime = 10
		cacheModule.topPort.Recv(write)

		read := mem.ReadReqBuilder{}.
			WithSendTime(10).
			WithSrc(agentPort).
			WithDst(cacheModule.topPort).
			WithAddress(0x10004).
			WithByteSize(4).
			Build()
		read.RecvTime = 10
		cacheModule.topPort.Recv(read)

		agentPort.EXPECT().Recv(gomock.Any()).
			Do(func(done *mem.WriteDoneRsp) {
				Expect(done.RespondTo).To(Equal(write.ID))
			})

		agentPort.EXPECT().Recv(gomock.Any()).Do(func(dr *mem.DataReadyRsp) {
			Expect(dr.Data).To(Equal([]byte{5, 6, 7, 8}))
			Expect(dr.RespondTo).To(Equal(read.ID))
		})

		engine.Run()

		retData, _ := storage.Read(0x0, 64)
		Expect(retData).To(Equal(write.Data))
		block := directory.Sets[0].Blocks[0]
		Expect(block.Tag).To(Equal(uint64(0x10000)))
		Expect(block.IsValid).To(BeTrue())
		Expect(block.IsDirty).To(BeTrue())
		Expect(directory.Sets[0].LRUQueue[3]).To(BeIdenticalTo(block))
	})

	It("should handle read miss, mshr miss, w/ fetch, w/ eviction", func() {
		dram.Storage.Write(0x10000, []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		})

		set := directory.Sets[0]
		for i := 0; i < directory.WayAssociativity(); i++ {
			set.Blocks[i].IsValid = true
			set.Blocks[i].IsDirty = true
		}

		read := mem.ReadReqBuilder{}.
			WithSendTime(10).
			WithSrc(agentPort).
			WithDst(cacheModule.topPort).
			WithAddress(0x10004).
			WithByteSize(4).
			Build()
		read.RecvTime = 10
		cacheModule.topPort.Recv(read)

		agentPort.EXPECT().Recv(gomock.Any()).Do(func(dr *mem.DataReadyRsp) {
			Expect(dr.Data).To(Equal([]byte{5, 6, 7, 8}))
			Expect(dr.RespondTo).To(Equal(read.ID))
		})

		engine.Run()
	})

	It("should handle write miss, mshr miss, w/ fetch, w/ eviction", func() {
		dram.Storage.Write(0x10000, []byte{
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
			1, 2, 3, 4, 5, 6, 7, 8,
		})

		set := directory.Sets[0]
		for i := 0; i < directory.WayAssociativity(); i++ {
			set.Blocks[i].IsValid = true
			set.Blocks[i].IsDirty = true
		}
		write := mem.WriteReqBuilder{}.
			WithSendTime(10).
			WithSrc(agentPort).
			WithDst(cacheModule.topPort).
			WithAddress(0x10004).
			WithData([]byte{9, 9, 9, 9}).
			Build()
		write.RecvTime = 10
		cacheModule.topPort.Recv(write)

		read := mem.ReadReqBuilder{}.
			WithSendTime(10).
			WithSrc(agentPort).
			WithDst(cacheModule.topPort).
			WithAddress(0x10000).
			WithByteSize(8).
			Build()
		read.RecvTime = 10
		cacheModule.topPort.Recv(read)

		agentPort.EXPECT().Recv(gomock.Any()).Do(func(done *mem.WriteDoneRsp) {
			Expect(done.RespondTo).To(Equal(write.ID))
		})

		agentPort.EXPECT().Recv(gomock.Any()).Do(func(dr *mem.DataReadyRsp) {
			Expect(dr.Data).To(Equal([]byte{1, 2, 3, 4, 9, 9, 9, 9}))
			Expect(dr.RespondTo).To(Equal(read.ID))
		})

		engine.Run()
	})

	It("should handle write miss, mshr miss, w/ fetch, w/o eviction", func() {
		set := directory.Sets[0]
		for i := 0; i < directory.WayAssociativity(); i++ {
			set.Blocks[i].IsValid = true
			set.Blocks[i].IsDirty = false
		}

		write := mem.WriteReqBuilder{}.
			WithSendTime(10).
			WithSrc(agentPort).
			WithDst(cacheModule.topPort).
			WithAddress(0x10000).
			WithData([]byte{
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
				1, 2, 3, 4, 5, 6, 7, 8,
			}).
			Build()
		write.RecvTime = 10
		cacheModule.topPort.Recv(write)

		read := mem.ReadReqBuilder{}.
			WithSendTime(10).
			WithSrc(agentPort).
			WithDst(cacheModule.topPort).
			WithAddress(0x10000).
			WithByteSize(8).
			Build()
		read.RecvTime = 10
		cacheModule.topPort.Recv(read)

		agentPort.EXPECT().Recv(gomock.Any()).Do(func(done *mem.WriteDoneRsp) {
			Expect(done.RespondTo).To(Equal(write.ID))
		})

		agentPort.EXPECT().Recv(gomock.Any()).Do(func(dr *mem.DataReadyRsp) {
			Expect(dr.Data).To(Equal([]byte{1, 2, 3, 4, 5, 6, 7, 8}))
			Expect(dr.RespondTo).To(Equal(read.ID))
		})

		engine.Run()
	})

	It("should flush", func() {
		write1 := mem.WriteReqBuilder{}.
			WithSendTime(10).
			WithSrc(agentPort).
			WithDst(cacheModule.topPort).
			WithAddress(0x100000).
			WithData([]byte{1, 2, 3, 4}).
			Build()
		write1.RecvTime = 10
		cacheModule.topPort.Recv(write1)

		write2 := mem.WriteReqBuilder{}.
			WithSendTime(10).
			WithSrc(agentPort).
			WithDst(cacheModule.topPort).
			WithAddress(0x100000).
			WithData([]byte{1, 2, 3, 4}).
			Build()
		write2.RecvTime = 10
		cacheModule.topPort.Recv(write2)

		flush := cache.FlushReqBuilder{}.
			WithSendTime(10).
			WithSrc(controlAgentPort).
			WithDst(cacheModule.controlPort).
			Build()
		flush.RecvTime = 10
		cacheModule.controlPort.Recv(flush)

		agentPort.EXPECT().Recv(gomock.Any()).AnyTimes()

		controlAgentPort.EXPECT().Recv(gomock.Any()).
			Do(func(rsp *cache.FlushRsp) {
				Expect(rsp.RspTo).To(Equal(flush.ID))
			})

		engine.Run()
	})
})
