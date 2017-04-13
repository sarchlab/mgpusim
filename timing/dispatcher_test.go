package timing_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/gcn3/kernels"
	"gitlab.com/yaotsu/gcn3/timing"
)

type MockGridBuilder struct {
	Grid *kernels.Grid
}

func (b *MockGridBuilder) Build(req *kernels.LaunchKernelReq) *kernels.Grid {
	return b.Grid
}

func prepareGrid() *kernels.Grid {
	// Prepare a mock grid that is expanded
	grid := kernels.NewGrid()
	for i := 0; i < 5; i++ {
		wg := kernels.NewWorkGroup()
		grid.WorkGroups = append(grid.WorkGroups, wg)
		for j := 0; j < 10; j++ {
			wf := kernels.NewWavefront()
			wg.Wavefronts = append(wg.Wavefronts, wf)
		}
	}
	return grid
}

var _ = Describe("Dispatcher", func() {
	var (
		engine      *core.MockEngine
		grid        *kernels.Grid
		codeObject  *insts.HsaCo
		packet      *kernels.HsaKernelDispatchPacket
		gridBuilder *MockGridBuilder
		dispatcher  *timing.Dispatcher
		connection  *core.MockConnection
		cu0         *core.MockComponent
		cu1         *core.MockComponent
	)

	BeforeEach(func() {
		engine = core.NewMockEngine()
		grid = prepareGrid()
		codeObject = insts.NewHsaCo()
		codeObject.KernelCodeEntryByteOffset = 256
		packet = new(kernels.HsaKernelDispatchPacket)
		packet.KernelObject = 6000
		grid.CodeObject = codeObject
		grid.Packet = packet

		gridBuilder = new(MockGridBuilder)
		gridBuilder.Grid = grid
		dispatcher = timing.NewDispatcher("dispatcher", engine, gridBuilder)
		dispatcher.Freq = 1 * core.GHz

		connection = core.NewMockConnection()
		cu0 = core.NewMockComponent("mockCU0")
		cu1 = core.NewMockComponent("mockCU1")

		dispatcher.CUs = append(dispatcher.CUs, cu0)
		dispatcher.CUs = append(dispatcher.CUs, cu1)

		core.PlugIn(dispatcher, "ToCUs", connection)
		core.PlugIn(dispatcher, "ToCommandProcessor", connection)
	})

	It("should process launch kernel request", func() {
		req := kernels.NewLaunchKernelReq()
		req.Packet = new(kernels.HsaKernelDispatchPacket)
		req.SetRecvTime(0)

		dispatcher.Recv(req)

		Expect(len(engine.ScheduledEvent)).To(Equal(1))
		evt := engine.ScheduledEvent[0].(*timing.KernelDispatchEvent)
		Expect(evt.Time()).To(BeNumerically("~", 1e-9, 1e-12))
		Expect(evt.Status.Packet).To(BeIdenticalTo(req.Packet))
		Expect(evt.Status.Grid).To(BeIdenticalTo(grid))
		Expect(len(evt.Status.WGs)).To(Equal(len(grid.WorkGroups)))
	})

	It("should process MapWGReq", func() {
		mapWGReq := timing.NewMapWGReq(nil, nil, 0, grid.WorkGroups[0],
			timing.NewKernelDispatchStatus())
		mapWGReq.Ok = true

		dispatcher.Recv(mapWGReq)

		Expect(engine.ScheduledEvent).NotTo(BeEmpty())
		evt := engine.ScheduledEvent[0].(*timing.KernelDispatchEvent)
		Expect(evt.Time()).To(BeNumerically("~", 1e-9, 1e-12))
		Expect(mapWGReq.KernelStatus.Mapped).To(BeTrue())
	})

	It("should mark CU busy if the MapWGReq is failed", func() {
		mapWGReq := timing.NewMapWGReq(nil, nil, 0, grid.WorkGroups[0],
			timing.NewKernelDispatchStatus())
		mapWGReq.Ok = false
		mapWGReq.KernelStatus.CUBusy = make([]bool, 2)

		dispatcher.Recv(mapWGReq)

		Expect(engine.ScheduledEvent).NotTo(BeEmpty())
		evt := engine.ScheduledEvent[0].(*timing.KernelDispatchEvent)
		Expect(evt.Time()).To(BeNumerically("~", 1e-9, 1e-12))

		Expect(mapWGReq.KernelStatus.CUBusy[0]).To(BeTrue())
		Expect(mapWGReq.KernelStatus.Mapped).To(BeFalse())
	})

	Context("when handling LaunchKernelEvent", func() {

		It("should start to map work-group", func() {
			evt := timing.NewKernelDispatchEvent()
			status := timing.NewKernelDispatchStatus()
			evt.Status = status
			evt.SetTime(10)

			status.WGs = append(status.WGs, grid.WorkGroups...)
			status.Grid = grid
			status.CUBusy = make([]bool, 2)
			status.DispatchingCUID = -1

			mapWGReq := timing.NewMapWGReq(dispatcher, cu0, 10, grid.WorkGroups[0],
				status)
			connection.ExpectSend(mapWGReq, nil)

			dispatcher.Handle(evt)

			Expect(connection.AllExpectedSent()).To(BeTrue())
		})

		It("should dispatch wavefront", func() {
			evt := timing.NewKernelDispatchEvent()
			status := timing.NewKernelDispatchStatus()
			evt.Status = status
			evt.SetTime(10)

			status.WGs = append(status.WGs, grid.WorkGroups[1:]...)
			status.DispatchingWfs = append(status.DispatchingWfs,
				grid.WorkGroups[0].Wavefronts...)
			status.Grid = grid
			status.DispatchingCUID = 0
			status.Mapped = true

			wf := status.DispatchingWfs[0]
			req := timing.NewDispatchWfReq(dispatcher, cu0, 10, wf, 6256)

			connection.ExpectSend(req, nil)

			dispatcher.Handle(evt)

			Expect(connection.AllExpectedSent()).To(BeTrue())
			Expect(status.DispatchingWfs).NotTo(ContainElement(
				BeIdenticalTo(wf)))
		})

		It("should map another workgroud after dispatching wavefronts", func() {
			evt := timing.NewKernelDispatchEvent()
			status := timing.NewKernelDispatchStatus()
			evt.Status = status
			evt.SetTime(10)

			status.WGs = append(status.WGs, grid.WorkGroups[1:]...)
			status.DispatchingWfs = append(status.DispatchingWfs,
				grid.WorkGroups[0].Wavefronts[0])
			status.Grid = grid
			status.DispatchingCUID = 0
			status.Mapped = true

			wf := status.DispatchingWfs[0]
			req := timing.NewDispatchWfReq(dispatcher, cu0, 10, wf, 6256)

			connection.ExpectSend(req, nil)

			dispatcher.Handle(evt)

			Expect(connection.AllExpectedSent()).To(BeTrue())
			Expect(status.DispatchingWfs).To(BeEmpty())
			Expect(status.Mapped).To(BeFalse())

		})

		It("should find not busy CUs to dispatch", func() {
			evt := timing.NewKernelDispatchEvent()
			status := timing.NewKernelDispatchStatus()
			evt.Status = status
			evt.SetTime(10)

			status.WGs = append(status.WGs, grid.WorkGroups...)
			status.Grid = grid
			status.CUBusy = make([]bool, 2)
			status.CUBusy[0] = true
			status.DispatchingCUID = -1

			mapWGReq := timing.NewMapWGReq(dispatcher, cu1, 10, grid.WorkGroups[0],
				status)
			connection.ExpectSend(mapWGReq, nil)

			dispatcher.Handle(evt)

			Expect(connection.AllExpectedSent()).To(BeTrue())
		})

		It("should wait if all the cus are busy", func() {
			evt := timing.NewKernelDispatchEvent()
			status := timing.NewKernelDispatchStatus()
			evt.Status = status
			evt.SetTime(10)

			status.WGs = append(status.WGs, grid.WorkGroups...)
			status.Grid = grid
			status.CUBusy = make([]bool, 2)
			status.CUBusy[0] = true
			status.CUBusy[1] = true
			status.DispatchingCUID = -1

			dispatcher.Handle(evt)

			Expect(connection.AllExpectedSent()).To(BeTrue())
		})

		It("should do nothing if no more pending work-groups", func() {
			evt := timing.NewKernelDispatchEvent()
			status := timing.NewKernelDispatchStatus()
			evt.Status = status
			evt.SetTime(10)

			status.Grid = grid
			status.CUBusy = make([]bool, 2)
			status.DispatchingCUID = 2

			dispatcher.Handle(evt)

			Expect(connection.AllExpectedSent()).To(BeTrue())
		})

		It("should process WGFinishMesg", func() {
			status := timing.NewKernelDispatchStatus()
			status.Grid = grid

			req := timing.NewWGFinishMesg(cu0, dispatcher, 10,
				grid.WorkGroups[0], status)

			dispatcher.Recv(req)

			Expect(status.CompletedWGs).To(ContainElement(grid.WorkGroups[0]))
		})

		It("should send back the LaunchKernelReq to the driver", func() {
			launchReq := kernels.NewLaunchKernelReq()
			launchReq.SetSrc(nil)
			launchReq.SetDst(dispatcher)

			status := timing.NewKernelDispatchStatus()
			status.Grid = grid
			status.CompletedWGs = append(status.CompletedWGs,
				status.Grid.WorkGroups[1:]...)
			status.Req = launchReq

			req := timing.NewWGFinishMesg(cu0, dispatcher, 10,
				grid.WorkGroups[0], status)

			connection.ExpectSend(launchReq, nil)

			dispatcher.Recv(req)

			Expect(connection.AllExpectedSent()).To(BeTrue())
		})
	})

})
