package dispatching

import (
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	"github.com/sarchlab/mgpusim/v5/amd/timing/cp/internal/resource"
)

// roundRobinAlgorithm can dispatch workgroups to CUs in a round robin
// fasion.
type roundRobinAlgorithm struct {
	gridBuilder kernels.GridBuilder
	cuPool      resource.CUResourcePool

	state func() *AlgorithmState
}

// StartNewKernel lets the algorithms to start dispatching a new kernel.
func (a *roundRobinAlgorithm) StartNewKernel(info kernels.KernelLaunchInfo) {
	a.state().Dispatched = 0
	a.gridBuilder.SetKernel(info)
}

// NumWG returns the number of work-groups in the currently-dispatching
// work-group.
func (a *roundRobinAlgorithm) NumWG() int {
	return a.gridBuilder.NumWG()
}

// HasNext check if there are more work-groups to dispatch.
func (a *roundRobinAlgorithm) HasNext() bool {
	return a.state().Dispatched < a.gridBuilder.NumWG()
}

// Next finds the location to dispatch the next work-group.
func (a *roundRobinAlgorithm) Next() (location dispatchLocation) {
	wg := currentWG(a.state().Current)
	if wg == nil {
		wg = a.gridBuilder.NextWG()
		a.state().Current = retainWG(wg)
	}

	for i := 0; i < a.cuPool.NumCU(); i++ {
		cuID := (a.state().NextCU + i) % a.cuPool.NumCU()
		cu := a.cuPool.GetCU(cuID)

		locations, ok := cu.ReserveResourceForWG(wg)
		if ok {
			a.state().NextCU = (cuID + 1) % a.cuPool.NumCU()

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
func (a *roundRobinAlgorithm) FreeResources(location dispatchLocation) {
	a.cuPool.GetCU(location.cuID).FreeResourcesForWG(location.wg)
}
