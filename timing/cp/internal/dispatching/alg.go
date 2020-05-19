package dispatching

import (
	"gitlab.com/akita/akita"
	"gitlab.com/akita/mgpusim/kernels"
	"gitlab.com/akita/mgpusim/protocol"
	"gitlab.com/akita/mgpusim/timing/cp/internal/resource"
)

type dispatchLocation struct {
	valid     bool
	cuID      int
	cu        akita.Port
	wg        *kernels.WorkGroup
	locations []protocol.WfDispatchLocation
}

// algorithm defines the CTA scheduling scheme.
type algorithm interface {
	// RegisterCU notifies the algorithm about the existence of the a cu.
	RegisterCU(cu resource.DispatchableCU)

	// StartNewKernel allows the algorithm start to process another kernel.
	StartNewKernel(info kernels.KernelLaunchInfo)

	// NumWG returns the total number of work-groups.
	NumWG() int

	// HasNext checks if there are more work-groups need to dispatch
	HasNext() bool

	// Next returns the information about where the next workgroup can be
	// dispatched.
	Next() (location dispatchLocation)

	// FreeResources marks the dispatched resources available.
	FreeResources(location dispatchLocation)
}

// roundRobinAlgorithm can dispatch workgroups to CUs in a round robin
// fasion.
type roundRobinAlgorithm struct {
	gridBuilder kernels.GridBuilder
	cuPool      resource.CUResourcePool

	currWG           *kernels.WorkGroup
	nextCU           int
	numDispatchedWGs int
}

// RegisterCU allows the roundRobinAlgorithm to dispatch work-group to the CU.
func (a *roundRobinAlgorithm) RegisterCU(cu resource.DispatchableCU) {
	a.cuPool.RegisterCU(cu)
}

// StartNewKernel lets the algorithms to start dispatching a new kernel.
func (a *roundRobinAlgorithm) StartNewKernel(info kernels.KernelLaunchInfo) {
	a.numDispatchedWGs = 0
	a.gridBuilder.SetKernel(info)
}

// NumWG returns the number of work-groups in the currently-dispatching
// work-group.
func (a *roundRobinAlgorithm) NumWG() int {
	return a.gridBuilder.NumWG()
}

// HasNext check if there are more work-groups to dispatch.
func (a *roundRobinAlgorithm) HasNext() bool {
	return a.numDispatchedWGs < a.gridBuilder.NumWG()
}

// Next finds the location to dispatch the next work-group.
func (a *roundRobinAlgorithm) Next() (location dispatchLocation) {
	if a.currWG == nil {
		a.currWG = a.gridBuilder.NextWG()
	}

	for i := 0; i < a.cuPool.NumCU(); i++ {
		cuID := (a.nextCU + i) % a.cuPool.NumCU()
		cu := a.cuPool.GetCU(cuID)

		locations, ok := cu.ReserveResourceForWG(a.currWG)
		if ok {
			a.nextCU = (cuID + 1) % a.cuPool.NumCU()

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
func (a *roundRobinAlgorithm) FreeResources(location dispatchLocation) {
	a.cuPool.GetCU(location.cuID).FreeResourcesForWG(location.wg)
}
