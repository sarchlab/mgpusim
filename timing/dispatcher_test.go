package timing_test

import (
	. "github.com/onsi/ginkgo"
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
		gridBuilder *MockGridBuilder
		dispatcher  *timing.Dispatcher
		cu0         *core.MockComponent
		cu1         *core.MockComponent
	)

	BeforeEach(func() {
		gridBuilder = new(MockGridBuilder)
		dispatcher = timing.NewDispatcher("dispatcher", gridBuilder)
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

	})
})
