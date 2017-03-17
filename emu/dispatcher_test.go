package emu_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/emu"
)

type MockGridBuilder struct {
	GridToBuild *emu.Grid
}

func (b *MockGridBuilder) Build(req *emu.LaunchKernelReq) *emu.Grid {
	return b.GridToBuild
}

type MockMapWGReqFactory struct {
	Reqs []*emu.MapWgReq
}

func NewMockMapWGReqFactory() *MockMapWGReqFactory {
	return &MockMapWGReqFactory{make([]*emu.MapWgReq, 0)}
}
func (f *MockMapWGReqFactory) Create() *emu.MapWgReq {
	req := f.Reqs[0]
	f.Reqs = f.Reqs[1:]
	return req
}

var _ = Describe("Dispatcher", func() {
	var (
		dispatcher      *emu.Dispatcher
		cu0             *core.MockComponent
		cu1             *core.MockComponent
		cu2             *core.MockComponent
		cu3             *core.MockComponent
		connection      *core.DirectConnection
		gridBuilder     *MockGridBuilder
		mapWGReqFactory *MockMapWGReqFactory
	)

	BeforeEach(func() {
		gridBuilder = new(MockGridBuilder)
		mapWGReqFactory = NewMockMapWGReqFactory()
		dispatcher = emu.NewDispatcher("dispatcher",
			gridBuilder, mapWGReqFactory)

		cu0 = core.NewMockComponent("cu0")
		cu0.AddPort("ToDispatcher")
		cu1 = core.NewMockComponent("cu1")
		cu1.AddPort("ToDispatcher")
		cu2 = core.NewMockComponent("cu2")
		cu2.AddPort("ToDispatcher")
		cu3 = core.NewMockComponent("cu3")
		cu3.AddPort("ToDispatcher")
		connection = core.NewDirectConnection()

		dispatcher.RegisterCU(cu0)
		dispatcher.RegisterCU(cu1)
		dispatcher.RegisterCU(cu2)
		dispatcher.RegisterCU(cu3)

		core.PlugIn(dispatcher, "ToComputeUnits", connection)
		core.PlugIn(cu0, "ToDispatcher", connection)
		core.PlugIn(cu1, "ToDispatcher", connection)
		core.PlugIn(cu2, "ToDispatcher", connection)
		core.PlugIn(cu3, "ToDispatcher", connection)
	})

	It("should dispatch", func() {
		wg1 := emu.NewWorkGroup()
		dispatcher.PendingWGs = append(dispatcher.PendingWGs, wg1)

		req := emu.NewMapWGReq()
		mapWGReqFactory.Reqs = append(mapWGReqFactory.Reqs, req)
		cu0.ToReceiveReq(req, nil)

		dispatcher.Dispatch(0)

		Expect(cu0.AllReqReceived()).To(BeTrue())

		Expect(req.SendTime()).To(BeNumerically("~", 0, 1e-12))
		Expect(req.Source()).To(BeIdenticalTo(dispatcher))
		Expect(req.Destination()).To(BeIdenticalTo(cu0))
		Expect(req.WG).To(BeIdenticalTo(wg1))
	})

	It("should only dispatch to idle cus", func() {
		wgs := make([]*emu.WorkGroup, 5)
		reqs := make([]*emu.MapWgReq, 5)
		for i := 0; i < 5; i++ {
			wgs[i] = emu.NewWorkGroup()
			reqs[i] = emu.NewMapWGReq()
		}
		dispatcher.PendingWGs = append(dispatcher.PendingWGs, wgs...)
		mapWGReqFactory.Reqs = append(mapWGReqFactory.Reqs, reqs...)

		cu0.ToReceiveReq(reqs[0], nil)
		cu1.ToReceiveReq(reqs[1], nil)
		cu2.ToReceiveReq(reqs[2], nil)
		cu3.ToReceiveReq(reqs[3], nil)

		dispatcher.Dispatch(0)

		Expect(cu0.AllReqReceived()).To(BeTrue())
		Expect(cu1.AllReqReceived()).To(BeTrue())
		Expect(cu2.AllReqReceived()).To(BeTrue())
		Expect(cu3.AllReqReceived()).To(BeTrue())
	})

	It("should expand grids to dispatch", func() {
		grid := emu.NewGrid()
		wgs := make([]*emu.WorkGroup, 5)
		reqs := make([]*emu.MapWgReq, 5)
		for i := 0; i < 5; i++ {
			wgs[i] = emu.NewWorkGroup()
			reqs[i] = emu.NewMapWGReq()
		}
		grid.WorkGroups = append(grid.WorkGroups, wgs...)
		mapWGReqFactory.Reqs = append(mapWGReqFactory.Reqs, reqs...)
		dispatcher.PendingGrids = append(dispatcher.PendingGrids, grid)

		cu0.ToReceiveReq(reqs[0], nil)
		cu1.ToReceiveReq(reqs[1], nil)
		cu2.ToReceiveReq(reqs[2], nil)
		cu3.ToReceiveReq(reqs[3], nil)

		dispatcher.Dispatch(0)

		Expect(cu0.AllReqReceived()).To(BeTrue())
		Expect(cu1.AllReqReceived()).To(BeTrue())
		Expect(cu2.AllReqReceived()).To(BeTrue())
		Expect(cu3.AllReqReceived()).To(BeTrue())
	})

})
