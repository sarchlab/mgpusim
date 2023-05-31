package writearound_test

import (
	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sarchlab/akita/v3/sim"
	. "github.com/sarchlab/mgpusim/v3/mem/cache/writearound"
	"github.com/sarchlab/mgpusim/v3/mem/idealmemcontroller"

	"github.com/sarchlab/mgpusim/v3/mem/mem"
)

var _ = Describe("Cache", func() {
	var (
		mockCtrl        *gomock.Controller
		engine          sim.Engine
		connection      sim.Connection
		lowModuleFinder mem.LowModuleFinder
		dram            *idealmemcontroller.Comp
		cuPort          *MockPort
		c               *Cache
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		cuPort = NewMockPort(mockCtrl)
		engine = sim.NewSerialEngine()
		connection = sim.NewDirectConnection("Conn", engine, 1*sim.GHz)
		dram = idealmemcontroller.New("DRAM", engine, 4*mem.GB)
		lowModuleFinder = &mem.SingleLowModuleFinder{
			LowModule: dram.GetPortByName("Top"),
		}
		c = NewBuilder().
			WithEngine(engine).
			WithLowModuleFinder(lowModuleFinder).
			Build("Cache")

		connection.PlugIn(dram.GetPortByName("Top"), 64)
		connection.PlugIn(c.GetPortByName("Top"), 4)
		connection.PlugIn(c.GetPortByName("Bottom"), 16)
		cuPort.EXPECT().SetConnection(connection)
		connection.PlugIn(cuPort, 4)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do read miss", func() {
		dram.Storage.Write(0x100, []byte{1, 2, 3, 4})
		read := mem.ReadReqBuilder{}.
			WithSendTime(1).
			WithSrc(cuPort).
			WithDst(c.GetPortByName("Top")).
			WithAddress(0x100).
			WithByteSize(4).
			Build()
		c.GetPortByName("Top").Recv(read)

		cuPort.EXPECT().Recv(gomock.Any()).
			Do(func(dr *mem.DataReadyRsp) {
				Expect(dr.Data).To(Equal([]byte{1, 2, 3, 4}))
			})

		engine.Run()
	})

	It("should do read miss coalesce", func() {
		dram.Storage.Write(0x100, []byte{1, 2, 3, 4, 5, 6, 7, 8})
		read1 := mem.ReadReqBuilder{}.
			WithSendTime(1).
			WithSrc(cuPort).
			WithDst(c.GetPortByName("Top")).
			WithAddress(0x100).
			WithByteSize(4).
			Build()
		c.GetPortByName("Top").Recv(read1)

		read2 := mem.ReadReqBuilder{}.
			WithSendTime(1).
			WithSrc(cuPort).
			WithDst(c.GetPortByName("Top")).
			WithAddress(0x104).
			WithByteSize(4).
			Build()
		c.GetPortByName("Top").Recv(read2)

		cuPort.EXPECT().Recv(gomock.Any()).
			Do(func(dr *mem.DataReadyRsp) {
				Expect(dr.Data).To(Equal([]byte{1, 2, 3, 4}))
			})
		cuPort.EXPECT().Recv(gomock.Any()).
			Do(func(dr *mem.DataReadyRsp) {
				Expect(dr.Data).To(Equal([]byte{5, 6, 7, 8}))
			})

		engine.Run()
	})

	It("should do read hit", func() {
		dram.Storage.Write(0x100, []byte{1, 2, 3, 4, 5, 6, 7, 8})
		read1 := mem.ReadReqBuilder{}.
			WithSendTime(1).
			WithSrc(cuPort).
			WithDst(c.GetPortByName("Top")).
			WithAddress(0x100).
			WithByteSize(4).
			Build()
		read1.RecvTime = 0
		c.GetPortByName("Top").Recv(read1)
		cuPort.EXPECT().Recv(gomock.Any()).
			Do(func(dr *mem.DataReadyRsp) {
				Expect(dr.Data).To(Equal([]byte{1, 2, 3, 4}))
			})
		engine.Run()
		t1 := engine.CurrentTime()

		read2 := mem.ReadReqBuilder{}.
			WithSendTime(t1).
			WithSrc(cuPort).
			WithDst(c.GetPortByName("Top")).
			WithAddress(0x104).
			WithByteSize(4).
			Build()
		read2.RecvTime = t1
		c.GetPortByName("Top").Recv(read2)
		cuPort.EXPECT().Recv(gomock.Any()).
			Do(func(dr *mem.DataReadyRsp) {
				Expect(dr.Data).To(Equal([]byte{5, 6, 7, 8}))
			})
		engine.Run()
		t2 := engine.CurrentTime()

		Expect(t2 - t1).To(BeNumerically("<", t1))
	})

	It("should write partial line", func() {
		write := mem.WriteReqBuilder{}.
			WithSendTime(0).
			WithSrc(cuPort).
			WithDst(c.GetPortByName("Top")).
			WithAddress(0x100).
			WithData([]byte{1, 2, 3, 4}).
			Build()
		write.RecvTime = 0
		c.GetPortByName("Top").Recv(write)
		cuPort.EXPECT().Recv(gomock.Any()).
			Do(func(done *mem.WriteDoneRsp) {
				Expect(done.RespondTo).To(Equal(write.ID))
			})

		engine.Run()

		data, _ := dram.Storage.Read(0x100, 4)
		Expect(data).To(Equal([]byte{1, 2, 3, 4}))
	})

	It("should write full line", func() {
		write := mem.WriteReqBuilder{}.
			WithSendTime(0).
			WithSrc(cuPort).
			WithDst(c.GetPortByName("Top")).
			WithAddress(0x100).
			WithData(
				[]byte{
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
		write.RecvTime = 0
		c.GetPortByName("Top").Recv(write)
		cuPort.EXPECT().Recv(gomock.Any()).
			Do(func(done *mem.WriteDoneRsp) {
				Expect(done.RespondTo).To(Equal(write.ID))
			})
		engine.Run()

		data, _ := dram.Storage.Read(0x100, 4)
		Expect(data).To(Equal([]byte{1, 2, 3, 4}))
	})

})
