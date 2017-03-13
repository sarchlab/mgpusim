package emu_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/emu"
)

type MockMapWorkGroupReqFactory struct {
	Reqs []*emu.MapWgReq
}

func NewMockMapWorkGroupReqFactory() *MockMapWorkGroupReqFactory {
	return &MockMapWorkGroupReqFactory{make([]*emu.MapWgReq, 0)}
}

func (f *MockMapWorkGroupReqFactory) Create() *emu.MapWgReq {
	req := f.Reqs[0]
	f.Reqs = f.Reqs[1:]
	return req
}

var _ = Describe("Dispatcher", func() {
	var (
		dispatcher             *emu.Dispatcher
		cu0                    *core.MockComponent
		cu1                    *core.MockComponent
		cu2                    *core.MockComponent
		cu3                    *core.MockComponent
		connection             *core.DirectConnection
		mapWorkGroupReqFactory *MockMapWorkGroupReqFactory
	)

	BeforeEach(func() {
		mapWorkGroupReqFactory = NewMockMapWorkGroupReqFactory()
		dispatcher = emu.NewDispatcher("dispatcher", mapWorkGroupReqFactory)
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
		req := emu.NewLaunchKernelReq()
		packet := new(emu.HsaKernelDispatchPacket)
		packet.WorkgroupSizeX = 256
		packet.WorkgroupSizeY = 1
		packet.WorkgroupSizeZ = 1
		packet.GridSizeX = 1025
		packet.GridSizeY = 1
		packet.GridSizeZ = 1
		req.Packet = packet
		req.SetSendTime(0)

		mapReq0 := emu.NewMapWGReq()
		mapReq1 := emu.NewMapWGReq()
		mapReq2 := emu.NewMapWGReq()
		mapReq3 := emu.NewMapWGReq()
		mapWorkGroupReqFactory.Reqs = append(mapWorkGroupReqFactory.Reqs, mapReq0)
		mapWorkGroupReqFactory.Reqs = append(mapWorkGroupReqFactory.Reqs, mapReq1)
		mapWorkGroupReqFactory.Reqs = append(mapWorkGroupReqFactory.Reqs, mapReq2)
		mapWorkGroupReqFactory.Reqs = append(mapWorkGroupReqFactory.Reqs, mapReq3)

		cu0.ToReceiveReq(mapReq0, nil)
		cu1.ToReceiveReq(mapReq1, nil)
		cu2.ToReceiveReq(mapReq2, nil)
		cu3.ToReceiveReq(mapReq3, nil)

		dispatcher.Receive(req)

		Expect(cu0.AllReqReceived()).To(BeTrue())
		Expect(cu1.AllReqReceived()).To(BeTrue())
		Expect(cu2.AllReqReceived()).To(BeTrue())
		Expect(cu3.AllReqReceived()).To(BeTrue())

	})
})
