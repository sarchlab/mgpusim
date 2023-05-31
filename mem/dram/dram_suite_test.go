package dram

import (
	"testing"

	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/mem/mem"

	"github.com/golang/mock/gomock"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//go:generate mockgen -destination "mock_sim_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/akita/v3/sim Port
//go:generate mockgen -destination "mock_trans_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/mgpusim/v3/mem/dram/internal/trans SubTransactionQueue,SubTransSplitter
//go:generate mockgen -destination "mock_addressmapping_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/mgpusim/v3/mem/dram/internal/addressmapping Mapper
//go:generate mockgen -destination "mock_cmdq_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/mgpusim/v3/mem/dram/internal/cmdq CommandQueue
//go:generate mockgen -destination "mock_org_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/mgpusim/v3/mem/dram/internal/org Channel
//go:generate mockgen -destination "mock_mem_test.go" -package $GOPACKAGE -write_package_comment=false github.com/sarchlab/mgpusim/v3/mem/mem AddressConverter

func TestDram(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Dram Suite")
}

var _ = Describe("DRAM Integration", func() {
	var (
		mockCtrl *gomock.Controller
		engine   sim.Engine
		srcPort  *MockPort
		memCtrl  *MemController
		conn     *sim.DirectConnection
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		engine = sim.NewSerialEngine()
		memCtrl = MakeBuilder().
			WithEngine(engine).
			Build("MemCtrl")
		srcPort = NewMockPort(mockCtrl)

		conn = sim.NewDirectConnection("Conn", engine, 1*sim.GHz)
		srcPort.EXPECT().SetConnection(conn)
		conn.PlugIn(memCtrl.topPort, 1)
		conn.PlugIn(srcPort, 1)
	})

	It("should read and write", func() {
		write := mem.WriteReqBuilder{}.
			WithAddress(0x40).
			WithData([]byte{1, 2, 3, 4}).
			WithSrc(srcPort).
			WithDst(memCtrl.topPort).
			WithSendTime(0).
			Build()

		read := mem.ReadReqBuilder{}.
			WithAddress(0x40).
			WithByteSize(4).
			WithSrc(srcPort).
			WithDst(memCtrl.topPort).
			WithSendTime(0).
			Build()

		memCtrl.topPort.Recv(write)
		memCtrl.topPort.Recv(read)

		ret1 := srcPort.EXPECT().
			Recv(gomock.Any()).
			Do(func(wd *mem.WriteDoneRsp) {
				Expect(wd.RespondTo).To(Equal(write.ID))
			})
		srcPort.EXPECT().
			Recv(gomock.Any()).
			Do(func(dr *mem.DataReadyRsp) {
				Expect(dr.RespondTo).To(Equal(read.ID))
				Expect(dr.Data).To(Equal([]byte{1, 2, 3, 4}))
			}).After(ret1)

		engine.Run()
	})
})
