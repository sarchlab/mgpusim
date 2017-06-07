package timing

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/gcn3/kernels"
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
		dispatcher  *Dispatcher
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
		dispatcher = NewDispatcher("dispatcher", engine, gridBuilder)
		dispatcher.Freq = 1 * core.GHz

		connection = core.NewMockConnection()
		cu0 = core.NewMockComponent("mockCU0")
		cu1 = core.NewMockComponent("mockCU1")

		dispatcher.CUs = append(dispatcher.CUs, cu0)
		dispatcher.CUs = append(dispatcher.CUs, cu1)

		core.PlugIn(dispatcher, "ToCUs", connection)
		core.PlugIn(dispatcher, "ToCommandProcessor", connection)
	})

	It("should reject dispatching if it is dispatching another kernel", func() {
		dispatcher.dispatchingKernel = NewKernelDispatchStatus()
		req := kernels.NewLaunchKernelReq()

		err := dispatcher.Recv(req)

		Expect(err).NotTo(BeNil())
	})

	It("should process launch kernel request", func() {
		req := kernels.NewLaunchKernelReq()
		req.Packet = new(kernels.HsaKernelDispatchPacket)
		req.SetRecvTime(0)

		dispatcher.Recv(req)

		Expect(len(engine.ScheduledEvent)).To(Equal(1))
		evt := engine.ScheduledEvent[0].(*core.TickEvent)
		Expect(evt.Time()).To(BeNumerically("~", 1e-9, 1e-12))

		status := dispatcher.dispatchingKernel
		Expect(status.Packet).To(BeIdenticalTo(req.Packet))
		Expect(status.Grid).To(BeIdenticalTo(grid))
		Expect(len(status.WGs)).To(Equal(len(grid.WorkGroups)))

		Expect(dispatcher.running).To(BeTrue())
	})

	It("should continue dispatching after receiving MapWGReq", func() {
		status := NewKernelDispatchStatus()
		status.Mapped = false
		dispatcher.dispatchingKernel = status
		dispatcher.running = false

		wg := grid.WorkGroups[0]
		mapWGReq := NewMapWGReq(nil, nil, 0, wg, nil)
		mapWGReq.Ok = true
		status.WGs = append(status.WGs, wg)

		dispatcher.Recv(mapWGReq)

		Expect(engine.ScheduledEvent).NotTo(BeEmpty())
		Expect(status.Mapped).To(BeTrue())
		Expect(status.WGs).NotTo(ContainElement(BeIdenticalTo(wg)))
	})

	It("should mark CU busy if the MapWGReq is failed", func() {
		status := NewKernelDispatchStatus()
		status.DispatchingCUID = 0 // This is not the CU to be marked as busy
		dispatcher.dispatchingKernel = status

		wg := grid.WorkGroups[0]
		mapWGReq := NewMapWGReq(nil, nil, 0, wg, nil)
		mapWGReq.Ok = false
		mapWGReq.CUID = 1 // This is the CU to be marked as busy
		status.CUBusy = make([]bool, 2)
		status.WGs = append(status.WGs, wg)

		dispatcher.Recv(mapWGReq)

		Expect(engine.ScheduledEvent).NotTo(BeEmpty())
		Expect(status.CUBusy[1]).To(BeTrue())
		Expect(status.Mapped).To(BeFalse())
		Expect(status.WGs).To(ContainElement(BeIdenticalTo(wg)))
	})

	It("should start to map work-group", func() {
		status := NewKernelDispatchStatus()
		status.WGs = append(status.WGs, grid.WorkGroups...)
		status.Grid = grid
		status.CUBusy = make([]bool, 2)
		status.DispatchingCUID = -1
		dispatcher.dispatchingKernel = status

		mapWGReq := NewMapWGReq(dispatcher, cu0, 10, grid.WorkGroups[0], nil)
		connection.ExpectSend(mapWGReq, nil)

		dispatcher.Handle(core.NewTickEvent(10, dispatcher))

		Expect(connection.AllExpectedSent()).To(BeTrue())
		Expect(dispatcher.running).To(BeFalse())
		Expect(engine.ScheduledEvent).To(BeEmpty())
	})

	It("should dispatch wavefront", func() {
		status := NewKernelDispatchStatus()
		status.WGs = append(status.WGs, grid.WorkGroups[1:]...)
		wfDispatchInfo := &WfDispatchInfo{
			SIMDID: 1, VGPROffset: 0, SGPROffset: 0, LDSOffset: 0}
		for _, wf := range grid.WorkGroups[0].Wavefronts {
			status.DispatchingWfs[wf] = wfDispatchInfo
		}
		status.Grid = grid
		status.DispatchingCUID = 0
		status.Mapped = true
		dispatcher.dispatchingKernel = status
		dispatcher.running = true

		wf := grid.WorkGroups[0].Wavefronts[0]
		req := NewDispatchWfReq(dispatcher, cu0, 10, wf,
			wfDispatchInfo, 6256)

		connection.ExpectSend(req, nil)

		dispatcher.Handle(core.NewTickEvent(10, dispatcher))

		Expect(connection.AllExpectedSent()).To(BeTrue())
		Expect(status.DispatchingWfs).NotTo(ContainElement(
			BeIdenticalTo(wf)))
		Expect(engine.ScheduledEvent).NotTo(BeEmpty())
	})

	It("should map another workgroup after dispatching wavefronts", func() {
		status := NewKernelDispatchStatus()
		status.WGs = append(status.WGs, grid.WorkGroups[1:]...)
		wfDispatchInfo := &WfDispatchInfo{
			SIMDID: 1, VGPROffset: 0, SGPROffset: 0, LDSOffset: 0}
		status.DispatchingWfs[grid.WorkGroups[0].Wavefronts[0]] =
			wfDispatchInfo
		status.Grid = grid
		status.DispatchingCUID = 0
		status.Mapped = true
		dispatcher.dispatchingKernel = status
		dispatcher.running = true

		wf := grid.WorkGroups[0].Wavefronts[0]
		req := NewDispatchWfReq(dispatcher, cu0, 10, wf, wfDispatchInfo, 6256)

		connection.ExpectSend(req, nil)

		dispatcher.Handle(core.NewTickEvent(10, dispatcher))

		Expect(connection.AllExpectedSent()).To(BeTrue())
		Expect(status.DispatchingWfs).NotTo(ContainElement(BeIdenticalTo(wf)))
		Expect(status.DispatchingWfs).To(BeEmpty())
		Expect(status.Mapped).To(BeFalse())
		Expect(engine.ScheduledEvent).NotTo(BeEmpty())
	})

	It("should find not busy CUs to dispatch", func() {
		status := NewKernelDispatchStatus()
		status.WGs = append(status.WGs, grid.WorkGroups...)
		status.Grid = grid
		status.CUBusy = make([]bool, 2)
		status.CUBusy[0] = true
		status.DispatchingCUID = 1
		dispatcher.dispatchingKernel = status

		mapWGReq := NewMapWGReq(dispatcher, cu1, 10, grid.WorkGroups[0], nil)
		mapWGReq.CUID = 1
		connection.ExpectSend(mapWGReq, nil)

		dispatcher.Handle(core.NewTickEvent(10, dispatcher))

		Expect(connection.AllExpectedSent()).To(BeTrue())
	})

	It("should process WGFinishMesg", func() {
		status := NewKernelDispatchStatus()
		status.Grid = grid
		status.CUBusy = make([]bool, 4)
		status.CUBusy[0] = true
		dispatcher.dispatchingKernel = status

		req := NewWGFinishMesg(cu0, dispatcher, 10, grid.WorkGroups[0])
		req.CUID = 0

		dispatcher.Recv(req)

		Expect(status.CompletedWGs).To(ContainElement(grid.WorkGroups[0]))
		Expect(status.CUBusy[0]).To(BeFalse())
		Expect(engine.ScheduledEvent).NotTo(BeEmpty())
	})

	It("should not scheduler tick, if dispatcher is running, when processing "+
		"WGFinishMesg", func() {
		status := NewKernelDispatchStatus()
		status.Grid = grid
		status.CUBusy = make([]bool, 4)
		status.CUBusy[0] = true
		dispatcher.dispatchingKernel = status
		dispatcher.running = true

		req := NewWGFinishMesg(cu0, dispatcher, 10, grid.WorkGroups[0])
		req.CUID = 0

		dispatcher.Recv(req)

		Expect(status.CompletedWGs).To(ContainElement(grid.WorkGroups[0]))
		Expect(status.CUBusy[0]).To(BeFalse())
		Expect(engine.ScheduledEvent).To(BeEmpty())
	})

	It("should send back the LaunchKernelReq to the driver", func() {
		launchReq := kernels.NewLaunchKernelReq()
		launchReq.SetSrc(nil)
		launchReq.SetDst(dispatcher)

		status := NewKernelDispatchStatus()
		status.Grid = grid
		status.CompletedWGs = append(status.CompletedWGs,
			status.Grid.WorkGroups[1:]...)
		status.Req = launchReq
		dispatcher.dispatchingKernel = status

		req := NewWGFinishMesg(cu0, dispatcher, 10, grid.WorkGroups[0])

		connection.ExpectSend(launchReq, nil)

		dispatcher.Recv(req)

		Expect(connection.AllExpectedSent()).To(BeTrue())
	})

})
