package dispatching

import (
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	"github.com/sarchlab/mgpusim/v5/amd/timing/cp/internal/resource"
)

// partitionAlgorithm holds only behavior and resolves all cursors through State.
type partitionAlgorithm struct {
	gridForPartition func(int) kernels.GridBuilder
	ids              interface{ NewID() uint64 }
	cuPool           resource.CUResourcePool
	state            func() *AlgorithmState
}

func (a *partitionAlgorithm) grid(i int) kernels.GridBuilder {
	if a.gridForPartition != nil {
		return a.gridForPartition(i)
	}
	return &stateGrid{ids: a.ids, state: func() *kernels.GridBuilderState { return &a.state().Partitions[i].Grid }}
}

// StartNewKernel lets the algorithms to start dispatching a new kernel.
func (a *partitionAlgorithm) StartNewKernel(info kernels.KernelLaunchInfo) {
	a.state().Dispatched = 0

	gb := kernels.NewGridBuilder(a.ids)
	gb.SetKernel(info)
	a.state().NumWG = gb.NumWG()
	numCU := a.cuPool.NumCU()
	a.state().WGPerPartition = (a.state().NumWG-1)/numCU + 1

	a.state().Partitions = nil
	for i := 0; i < numCU; i++ {
		p := PartitionState{}
		grid := &stateGrid{ids: a.ids, state: func() *kernels.GridBuilderState { return &p.Grid }}

		grid.SetKernel(info)
		grid.Skip(i * a.state().WGPerPartition)

		a.state().Partitions = append(a.state().Partitions, p)
	}
}

// NumWG returns the number of work-groups in the currently-dispatching
// work-group.
func (a *partitionAlgorithm) NumWG() int {
	return a.state().NumWG
}

// HasNext check if there are more work-groups to dispatch.
func (a *partitionAlgorithm) HasNext() bool {
	return a.state().Dispatched < a.state().NumWG
}

// Next finds the location to dispatch the next work-group.
func (a *partitionAlgorithm) Next() (location dispatchLocation) {
	if a.allWGDispatched() {
		return dispatchLocation{}
	}

	for index := range a.state().Partitions {
		i := (index + a.state().NextPartition) % len(a.state().Partitions)

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

			a.state().Partitions[wgFromPartition].Current = Location{}
			a.state().Partitions[wgFromPartition].Dispatched++
			a.state().Dispatched++

			a.state().NextPartition = i + 1

			return dispatch
		}
	}

	return dispatchLocation{}
}

func (a *partitionAlgorithm) nextWG(partitionIndex int) (
	*kernels.WorkGroup, int,
) {
	if a.noWGInPartition(partitionIndex) {
		for i := range a.state().Partitions {
			if a.state().Partitions[i].Current.HasWG {
				return currentWG(a.state().Partitions[i].Current), i
			}
		}

		return nil, 0
	}

	if a.state().Partitions[partitionIndex].Current.HasWG {
		return currentWG(a.state().Partitions[partitionIndex].Current), partitionIndex
	}

	a.state().Partitions[partitionIndex].Current = retainWG(a.grid(partitionIndex).NextWG())

	return currentWG(a.state().Partitions[partitionIndex].Current), partitionIndex
}

func (a *partitionAlgorithm) allWGDispatched() bool {
	return a.state().Dispatched >= a.state().NumWG
}

func (a *partitionAlgorithm) noWGInPartition(partitionIndex int) bool {
	p := a.state().Partitions[partitionIndex]
	if p.Dispatched >= a.state().WGPerPartition {
		return true
	}

	return false
}

// FreeResources marks the dispatched location to be available.
func (a *partitionAlgorithm) FreeResources(location dispatchLocation) {
	a.cuPool.GetCU(location.cuID).FreeResourcesForWG(location.wg)
}
