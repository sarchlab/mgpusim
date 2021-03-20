package dispatching

import (
	"gitlab.com/akita/mgpusim/v2/kernels"
	"gitlab.com/akita/mgpusim/v2/protocol"
	"gitlab.com/akita/mgpusim/v2/timing/cp/internal/resource"
)

// greedyAlgorithm fills a CU before moving to another GPU.
type greedyAlgorithm struct {
	gridBuilder kernels.GridBuilder
	cuPool      resource.CUResourcePool

	currWG           *kernels.WorkGroup
	numDispatchedWGs int
}

// RegisterCU allows the greedyAlgorithm to dispatch work-group to the CU.
func (a *greedyAlgorithm) RegisterCU(cu resource.DispatchableCU) {
	a.cuPool.RegisterCU(cu)
}

// StartNewKernel lets the algorithms to start dispatching a new kernel.
func (a *greedyAlgorithm) StartNewKernel(info kernels.KernelLaunchInfo) {
	a.numDispatchedWGs = 0
	a.gridBuilder.SetKernel(info)
}

// NumWG returns the number of work-groups in the currently-dispatching
// work-group.
func (a *greedyAlgorithm) NumWG() int {
	return a.gridBuilder.NumWG()
}

// HasNext check if there are more work-groups to dispatch.
func (a *greedyAlgorithm) HasNext() bool {
	return a.numDispatchedWGs < a.gridBuilder.NumWG()
}

// Next finds the location to dispatch the next work-group.
func (a *greedyAlgorithm) Next() (location dispatchLocation) {
	if a.currWG == nil {
		a.currWG = a.gridBuilder.NextWG()
	}

	for i := 0; i < a.cuPool.NumCU(); i++ {
		cuID := i
		cu := a.cuPool.GetCU(cuID)

		locations, ok := cu.ReserveResourceForWG(a.currWG)
		if ok {
			dispatch := dispatchLocation{
				valid: true,
				cu:    cu.DispatchingPort(),
				cuID:  cuID,
				wg:    a.currWG,
			}
			dispatch.locations =
				make([]protocol.WfDispatchLocation, len(locations))
			for i, localtion := range locations {
				dispatch.locations[i] = protocol.WfDispatchLocation(localtion)
			}

			a.currWG = nil
			a.numDispatchedWGs++

			return dispatch
		}
	}

	return dispatchLocation{}
}

// FreeResources marks the dispatched location to be available.
func (a *greedyAlgorithm) FreeResources(location dispatchLocation) {
	a.cuPool.GetCU(location.cuID).FreeResourcesForWG(location.wg)
}
