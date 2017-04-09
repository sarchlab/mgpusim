package timing_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3/kernels"
	"gitlab.com/yaotsu/gcn3/timing"
)

type MockGridBuilder struct {
	Grid *kernels.Grid
}

func (b *MockGridBuilder) Build(req *kernels.LaunchKernelReq) *kernels.Grid {
	return b.Grid
}

var _ = Describe("Dispatcher", func() {
	var (
		engine      *core.MockEngine
		gridBuilder *MockGridBuilder
		dispatcher  *timing.Dispatcher
		cu0         *core.MockComponent
		cu1         *core.MockComponent
	)

	BeforeEach(func() {
		engine = core.NewMockEngine()
		gridBuilder = new(MockGridBuilder)
		dispatcher = timing.NewDispatcher("dispatcher", engine, gridBuilder)
		dispatcher.Freq = 1 * core.GHz
		cu0 = core.NewMockComponent("mockCU0")
		cu1 = core.NewMockComponent("mockCU1")

		dispatcher.CUs = append(dispatcher.CUs, cu0)
		dispatcher.CUs = append(dispatcher.CUs, cu1)
	})

	It("should process launch kernel request", func() {
		grid := kernels.NewGrid()
		for i := 0; i < 5; i++ {
			wg := kernels.NewWorkGroup()
			grid.WorkGroups = append(grid.WorkGroups, wg)
			for j := 0; j < 10; j++ {
				wf := kernels.NewWavefront()
				wg.Wavefronts = append(wg.Wavefronts, wf)
			}
		}
		gridBuilder.Grid = grid

		req := kernels.NewLaunchKernelReq()
		req.Packet = new(kernels.HsaKernelDispatchPacket)
		req.SetRecvTime(0)

		dispatcher.Recv(req)

		Expect(len(engine.ScheduledEvent)).To(Equal(1))
		evt := engine.ScheduledEvent[0].(*timing.KernelDispatchEvent)
		Expect(evt.Time()).To(BeNumerically("~", 1e-9, 1e-12))
	})
})
