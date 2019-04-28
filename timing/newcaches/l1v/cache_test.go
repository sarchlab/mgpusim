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

	It("should read", func() {
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

})
