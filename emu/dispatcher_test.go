package emu_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/emu"
	"gitlab.com/yaotsu/gcn3/kernels"
)

type MockGridBuilder struct {
	GridToBuild *kernels.Grid
}

func (b *MockGridBuilder) Build(req *kernels.LaunchKernelReq) *kernels.Grid {
	return b.GridToBuild
}

var _ = Describe("Dispatcher", func() {
	var (
		dispatcher  *emu.Dispatcher
		cu          [4]*core.MockComponent
		connection  *core.DirectConnection
		gridBuilder *MockGridBuilder
	)

	BeforeEach(func() {
		gridBuilder = new(MockGridBuilder)
		dispatcher = emu.NewDispatcher("dispatcher", gridBuilder)

		cu[0] = core.NewMockComponent("cu[0]")
		cu[0].AddPort("ToDispatcher")
		cu[1] = core.NewMockComponent("cu[1]")
		cu[1].AddPort("ToDispatcher")
		cu[2] = core.NewMockComponent("cu[2]")
		cu[2].AddPort("ToDispatcher")
		cu[3] = core.NewMockComponent("cu[3]")
		cu[3].AddPort("ToDispatcher")
		connection = core.NewDirectConnection()

		dispatcher.RegisterCU(cu[0])
		dispatcher.RegisterCU(cu[1])
		dispatcher.RegisterCU(cu[2])
		dispatcher.RegisterCU(cu[3])

		core.PlugIn(dispatcher, "ToComputeUnits", connection)
		core.PlugIn(cu[0], "ToDispatcher", connection)
		core.PlugIn(cu[1], "ToDispatcher", connection)
		core.PlugIn(cu[2], "ToDispatcher", connection)
		core.PlugIn(cu[3], "ToDispatcher", connection)
	})

	It("should dispatch", func() {
		wg1 := kernels.NewWorkGroup()
		dispatcher.PendingWGs = append(dispatcher.PendingWGs, wg1)

		req := emu.NewMapWGReq()
		req.SetSrc(dispatcher)
		req.SetDst(cu[0])
		req.WG = wg1
		cu[0].ToReceiveReq(req, nil)

		dispatcher.Dispatch(0)

		Expect(cu[0].AllReqReceived()).To(BeTrue())

		Expect(req.SendTime()).To(BeNumerically("~", 0, 1e-12))
		Expect(req.Src()).To(BeIdenticalTo(dispatcher))
		Expect(req.Dst()).To(BeIdenticalTo(cu[0]))
		Expect(req.WG).To(BeIdenticalTo(wg1))
	})

	It("should only dispatch to idle compute units", func() {
		wgs := make([]*kernels.WorkGroup, 5)
		reqs := make([]*emu.MapWgReq, 5)
		for i := 0; i < 5; i++ {
			wgs[i] = kernels.NewWorkGroup()
			reqs[i] = emu.NewMapWGReq()
			reqs[i].SetSrc(dispatcher)
			reqs[i].SetDst(cu[i%4])
			reqs[i].WG = wgs[i]
		}
		dispatcher.PendingWGs = append(dispatcher.PendingWGs, wgs...)

		cu[0].ToReceiveReq(reqs[0], nil)
		cu[1].ToReceiveReq(reqs[1], nil)
		cu[2].ToReceiveReq(reqs[2], nil)
		cu[3].ToReceiveReq(reqs[3], nil)

		dispatcher.Dispatch(0)

		Expect(cu[0].AllReqReceived()).To(BeTrue())
		Expect(cu[1].AllReqReceived()).To(BeTrue())
		Expect(cu[2].AllReqReceived()).To(BeTrue())
		Expect(cu[3].AllReqReceived()).To(BeTrue())
	})

	It("should expand grids to dispatch", func() {
		grid := kernels.NewGrid()
		wgs := make([]*kernels.WorkGroup, 5)
		reqs := make([]*emu.MapWgReq, 5)
		for i := 0; i < 5; i++ {
			wgs[i] = kernels.NewWorkGroup()
			reqs[i] = emu.NewMapWGReq()
			reqs[i].SetSrc(dispatcher)
			reqs[i].SetDst(cu[i%4])
			reqs[i].WG = wgs[i]
		}
		grid.WorkGroups = append(grid.WorkGroups, wgs...)
		dispatcher.PendingGrids = append(dispatcher.PendingGrids, grid)

		cu[0].ToReceiveReq(reqs[0], nil)
		cu[1].ToReceiveReq(reqs[1], nil)
		cu[2].ToReceiveReq(reqs[2], nil)
		cu[3].ToReceiveReq(reqs[3], nil)

		dispatcher.Dispatch(0)

		Expect(cu[0].AllReqReceived()).To(BeTrue())
		Expect(cu[1].AllReqReceived()).To(BeTrue())
		Expect(cu[2].AllReqReceived()).To(BeTrue())
		Expect(cu[3].AllReqReceived()).To(BeTrue())
	})

})
