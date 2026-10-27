package dispatching

import (
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	"github.com/sarchlab/mgpusim/v5/amd/timing/cp/internal/resource"
)

// greedyAlgorithm fills a CU before moving to another GPU.
type greedyAlgorithm struct {
	gridBuilder kernels.GridBuilder
	cuPool      resource.CUResourcePool

	state func() *AlgorithmState
}

// StartNewKernel lets the algorithms to start dispatching a new kernel.
func (a *greedyAlgorithm) StartNewKernel(info kernels.KernelLaunchInfo) {
	a.state().Dispatched = 0
	a.gridBuilder.SetKernel(info)
}

// NumWG returns the number of work-groups in the currently-dispatching
// work-group.
func (a *greedyAlgorithm) NumWG() int {
	return a.gridBuilder.NumWG()
}

// HasNext check if there are more work-groups to dispatch.
func (a *greedyAlgorithm) HasNext() bool {
	return a.state().Dispatched < a.gridBuilder.NumWG()
}

// Next finds the location to dispatch the next work-group.
func (a *greedyAlgorithm) Next() (location dispatchLocation) {
	wg := currentWG(a.state().Current)
	if wg == nil {
		wg = a.gridBuilder.NextWG()
		a.state().Current = retainWG(wg)
	}

	for i := 0; i < a.cuPool.NumCU(); i++ {
		cuID := i
		cu := a.cuPool.GetCU(cuID)

		locations, ok := cu.ReserveResourceForWG(wg)
		if ok {
			dispatch := dispatchLocation{
				valid: true,
				cu:    cu.DispatchingPort(),
				cuID:  cuID,
				wg:    wg,
			}
			dispatch.locations =
				make([]protocol.WfDispatchLocation, len(locations))
			for i, localtion := range locations {
				dispatch.locations[i] = protocol.WfDispatchLocation(localtion)
			}

			a.state().Current = Location{}
			a.state().Dispatched++

			return dispatch
		}
	}

	return dispatchLocation{}
}

// FreeResources marks the dispatched location to be available.
func (a *greedyAlgorithm) FreeResources(location dispatchLocation) {
	a.cuPool.GetCU(location.cuID).FreeResourcesForWG(location.wg)
}
