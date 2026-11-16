package dispatching

import (
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	"github.com/sarchlab/mgpusim/v5/amd/timing/cp/internal/resource"
)

// perDieAlgorithm dispatches a kernel across numDies XCDs in parallel. The grid
// is split into numDies contiguous blocks (die-level block assignment); within a
// die the block's work-groups are spread round-robin (interleaved) across that
// die's compute units. The dispatcher drives each die independently through
// NextForDie and gates each with its own per-wavefront rate, so the dies run in
// parallel. CUs are assumed to be registered in XCD order, so die d owns the
// contiguous CU range [d*cusPerDie, (d+1)*cusPerDie).
//
// NOTE: the per-WG -> die policy is die-level block + intra-die interleave. The
// HW-accurate cross-XCD distribution is undocumented; this is a modeling choice
// and is irrelevant to uniform grids (e.g. empty_kernel). See issue #282.
type perDieAlgorithm struct {
	gridForDie func(int) kernels.GridBuilder
	ids        interface{ NewID() uint64 }
	cuPool     resource.CUResourcePool
	numDies    int

	state func() *AlgorithmState
}

// NumDies returns the number of dies the algorithm dispatches across.
func (a *perDieAlgorithm) NumDies() int {
	return a.numDies
}

// StartNewKernel partitions the grid into one contiguous block per die.
func (a *perDieAlgorithm) StartNewKernel(info kernels.KernelLaunchInfo) {
	gb := kernels.NewGridBuilder(a.ids)
	gb.SetKernel(info)
	a.state().NumWG = gb.NumWG()
	a.state().Dispatched = 0

	a.state().CUsPerDie = a.cuPool.NumCU() / a.numDies
	numWGPerDie := (a.state().NumWG-1)/a.numDies + 1 // ceil; block size per die

	a.state().Partitions = make([]PartitionState, a.numDies)
	for d := 0; d < a.numDies; d++ {
		start := d * numWGPerDie
		n := a.state().NumWG - start
		if n < 0 {
			n = 0
		}
		if n > numWGPerDie {
			n = numWGPerDie
		}

		ds := PartitionState{Count: n, FirstCU: d * a.state().CUsPerDie}
		grid := &stateGrid{ids: a.ids, state: func() *kernels.GridBuilderState { return &ds.Grid }}
		grid.SetKernel(info)
		grid.Skip(start)
		a.state().Partitions[d] = ds
	}
}

// NumWG returns the total number of work-groups across all dies.
func (a *perDieAlgorithm) NumWG() int {
	return a.state().NumWG
}

// HasNext reports whether any die still has work-groups to dispatch.
func (a *perDieAlgorithm) HasNext() bool {
	return a.state().Dispatched < a.state().NumWG
}

// Next is unused on the per-die path (the dispatcher drives dies via
// NextForDie). It returns an invalid location.
func (a *perDieAlgorithm) Next() dispatchLocation {
	return dispatchLocation{}
}

// NextForDie returns the next work-group to dispatch on the given die, placed on
// the die's next compute unit (round-robin). It returns an invalid location when
// the die's block is exhausted or no CU on the die can currently host the WG (in
// which case the pulled WG is held and retried on a later call).
func (a *perDieAlgorithm) NextForDie(die int) dispatchLocation {
	ds := &a.state().Partitions[die]
	if ds.Dispatched >= ds.Count {
		return dispatchLocation{}
	}

	wg := currentWG(ds.Current)
	if wg == nil {
		if a.gridForDie != nil {
			wg = a.gridForDie(die).NextWG()
		} else {
			wg = kernels.NextWorkGroup(&ds.Grid, a.ids)
		}
		ds.Current = retainWG(wg)
	}

	for k := 0; k < a.state().CUsPerDie; k++ {
		cuOffset := (ds.NextCU + k) % a.state().CUsPerDie
		cuID := ds.FirstCU + cuOffset
		cu := a.cuPool.GetCU(cuID)

		locations, ok := cu.ReserveResourceForWG(wg)
		if !ok {
			continue
		}

		dispatch := dispatchLocation{
			valid: true,
			cu:    cu.DispatchingPort(),
			cuID:  cuID,
			wg:    wg,
		}
		dispatch.locations = make([]protocol.WfDispatchLocation, len(locations))
		for i, location := range locations {
			dispatch.locations[i] = protocol.WfDispatchLocation(location)
		}

		ds.Current = Location{}
		ds.Dispatched++
		ds.NextCU = cuOffset + 1
		a.state().Dispatched++

		return dispatch
	}

	return dispatchLocation{}
}

// FreeResources marks the dispatched location's resources available.
func (a *perDieAlgorithm) FreeResources(location dispatchLocation) {
	a.cuPool.GetCU(location.cuID).FreeResourcesForWG(location.wg)
}
