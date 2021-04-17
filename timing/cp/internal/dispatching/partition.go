package dispatching

import (
	"gitlab.com/akita/mgpusim/v2/kernels"
	"gitlab.com/akita/mgpusim/v2/protocol"
	"gitlab.com/akita/mgpusim/v2/timing/cp/internal/resource"
)

type partition struct {
	gridBuilder  kernels.GridBuilder
	dispatchedWG int
}

// partitionAlgorithm can dispatch workgroups to CUs in a round robin
// fasion.
type partitionAlgorithm struct {
	partitions []*partition
	cuPool     resource.CUResourcePool

	nextPartition     int
	currWGs           []*kernels.WorkGroup
	numWG             int
	numDispatchedWG   int
	numWGPerPartition int

	initialized bool
}

// RegisterCU allows the partitionAlgorithm to dispatch work-group to the CU.
func (a *partitionAlgorithm) RegisterCU(cu resource.DispatchableCU) {
	a.cuPool.RegisterCU(cu)
}

// StartNewKernel lets the algorithms to start dispatching a new kernel.
func (a *partitionAlgorithm) StartNewKernel(info kernels.KernelLaunchInfo) {
	a.numDispatchedWG = 0

	gb := kernels.NewGridBuilder()
	gb.SetKernel(info)
	a.numWG = gb.NumWG()
	numCU := a.cuPool.NumCU()
	a.numWGPerPartition = (a.numWG-1)/numCU + 1

	a.partitions = nil
	for i := 0; i < numCU; i++ {
		p := &partition{
			gridBuilder: kernels.NewGridBuilder(),
		}

		p.gridBuilder.SetKernel(info)
		p.gridBuilder.Skip(i * a.numWGPerPartition)

		a.partitions = append(a.partitions, p)
	}

	a.currWGs = make([]*kernels.WorkGroup, numCU)
}

// NumWG returns the number of work-groups in the currently-dispatching
// work-group.
func (a *partitionAlgorithm) NumWG() int {
	return a.numWG
}

// HasNext check if there are more work-groups to dispatch.
func (a *partitionAlgorithm) HasNext() bool {
	return a.numDispatchedWG < a.numWG
}

// Next finds the location to dispatch the next work-group.
func (a *partitionAlgorithm) Next() (location dispatchLocation) {
	if a.allWGDispatched() {
		return dispatchLocation{}
	}

	for index := range a.partitions {
		i := (index + a.nextPartition) % len(a.partitions)

		wgToDispatch, wgFromPartition := a.nextWG(i)
		if wgToDispatch == nil {
			continue
		}

		cu := a.cuPool.GetCU(i)
		locations, ok := cu.ReserveResourceForWG(wgToDispatch)
		if ok {
			dispatch := dispatchLocation{
				valid: true,
				cu:    cu.DispatchingPort(),
				cuID:  i,
				wg:    wgToDispatch,
			}

			dispatch.locations =
				make([]protocol.WfDispatchLocation, len(locations))
			for i, localtion := range locations {
				dispatch.locations[i] = protocol.WfDispatchLocation(localtion)
			}

			a.currWGs[wgFromPartition] = nil
			a.partitions[wgFromPartition].dispatchedWG++
			a.numDispatchedWG++

			a.nextPartition = i + 1

			return dispatch
		}
	}

	return dispatchLocation{}
}

func (a *partitionAlgorithm) nextWG(partitionIndex int) (
	*kernels.WorkGroup, int,
) {
	if a.noWGInPartition(partitionIndex) {
		for i := range a.partitions {
			if a.currWGs[i] != nil {
				return a.currWGs[i], i
			}
		}

		return nil, 0
	}

	if a.currWGs[partitionIndex] != nil {
		return a.currWGs[partitionIndex], partitionIndex
	}

	a.currWGs[partitionIndex] =
		a.partitions[partitionIndex].gridBuilder.NextWG()

	return a.currWGs[partitionIndex], partitionIndex
}

func (a *partitionAlgorithm) allWGDispatched() bool {
	return a.numDispatchedWG >= a.numWG
}

func (a *partitionAlgorithm) noWGInPartition(partitionIndex int) bool {
	p := a.partitions[partitionIndex]
	if p.dispatchedWG >= a.numWGPerPartition {
		return true
	}

	return false
}

// FreeResources marks the dispatched location to be available.
func (a *partitionAlgorithm) FreeResources(location dispatchLocation) {
	a.cuPool.GetCU(location.cuID).FreeResourcesForWG(location.wg)
}
