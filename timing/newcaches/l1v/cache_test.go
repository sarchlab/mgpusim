package l1v_test

import (
	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "gitlab.com/akita/gcn3/timing/newcaches/l1v"
	"gitlab.com/akita/mem/cache"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem"
)

var _ = Describe("Cache", func() {
	var (
		mockCtrl        *gomock.Controller
		engine          akita.Engine
		connection      akita.Connection
		lowModuleFinder cache.LowModuleFinder
		dram            *mem.IdealMemController
		cuPort          *MockPort
		c               *Cache
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		cuPort = NewMockPort(mockCtrl)
		engine = akita.NewSerialEngine()
		connection = akita.NewDirectConnection(engine)
		dram = mem.NewIdealMemController("dram", engine, 4*mem.GB)
		lowModuleFinder = &cache.SingleLowModuleFinder{
			LowModule: dram.ToTop,
		}
		c = NewBuilder().
			WithEngine(engine).
			WithLowModuleFinder(lowModuleFinder).
			Build("cache")

		connection.PlugIn(dram.ToTop)
		connection.PlugIn(c.TopPort)
		connection.PlugIn(c.BottomPort)
		cuPort.EXPECT().SetConnection(connection)
		connection.PlugIn(cuPort)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should do read miss", func() {
		dram.Storage.Write(0x100, []byte{1, 2, 3, 4})
		read := mem.NewReadReq(1, cuPort, c.TopPort, 0x100, 4)
		read.IsLastInWave = true
		c.TopPort.Recv(read)

		cuPort.EXPECT().Recv(gomock.Any()).
			Do(func(dr *mem.DataReadyRsp) {
				Expect(dr.Data).To(Equal([]byte{1, 2, 3, 4}))
			})

		engine.Run()
	})

	It("should do read miss coalesce", func() {
		dram.Storage.Write(0x100, []byte{1, 2, 3, 4, 5, 6, 7, 8})
		read1 := mem.NewReadReq(1, cuPort, c.TopPort, 0x100, 4)
		c.TopPort.Recv(read1)

		read2 := mem.NewReadReq(1, cuPort, c.TopPort, 0x104, 4)
		read2.IsLastInWave = true
		c.TopPort.Recv(read2)

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
		read1 := mem.NewReadReq(0, cuPort, c.TopPort, 0x100, 4)
		read1.SetRecvTime(0)
		read1.IsLastInWave = true
		c.TopPort.Recv(read1)
		cuPort.EXPECT().Recv(gomock.Any()).
			Do(func(dr *mem.DataReadyRsp) {
				Expect(dr.Data).To(Equal([]byte{1, 2, 3, 4}))
			})
		engine.Run()
		t1 := engine.CurrentTime()

		read2 := mem.NewReadReq(t1, cuPort, c.TopPort, 0x104, 4)
		read2.SetRecvTime(t1)
		read2.IsLastInWave = true
		c.TopPort.Recv(read2)
		cuPort.EXPECT().Recv(gomock.Any()).
			Do(func(dr *mem.DataReadyRsp) {
				Expect(dr.Data).To(Equal([]byte{5, 6, 7, 8}))
			})
		engine.Run()
		t2 := engine.CurrentTime()

		Expect(t2 - t1).To(BeNumerically("<", t1))
	})

	It("should write partial line", func() {
		write := mem.NewWriteReq(0, cuPort, c.TopPort, 0x100)
		write.Data = []byte{1, 2, 3, 4}
		write.IsLastInWave = true
		write.SetRecvTime(0)
		c.TopPort.Recv(write)
		cuPort.EXPECT().Recv(gomock.Any()).
			Do(func(done *mem.DoneRsp) {
				Expect(done.RespondTo).To(Equal(write.ID))
			})

		engine.Run()

		data, _ := dram.Storage.Read(0x100, 4)
		Expect(data).To(Equal([]byte{1, 2, 3, 4}))
	})

	It("should write full line", func() {
		write := mem.NewWriteReq(0, cuPort, c.TopPort, 0x100)
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
		write.IsLastInWave = true
		write.SetRecvTime(0)
		c.TopPort.Recv(write)
		cuPort.EXPECT().Recv(gomock.Any()).
			Do(func(done *mem.DoneRsp) {
				Expect(done.RespondTo).To(Equal(write.ID))
			})
		engine.Run()

		data, _ := dram.Storage.Read(0x100, 4)
		Expect(data).To(Equal([]byte{1, 2, 3, 4}))
	})

})
