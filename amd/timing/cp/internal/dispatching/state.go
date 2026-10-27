package dispatching

import (
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
)

type Location struct {
	Valid     bool
	CUID      int
	CU        messaging.RemotePort
	HasWG     bool
	WG        kernels.WorkGroupValue
	Locations []protocol.WfDispatchLocation
}

func saveLocation(l dispatchLocation) Location {
	s := Location{Valid: l.valid, CUID: l.cuID, CU: l.cu, Locations: l.locations, HasWG: l.wg != nil}
	if l.wg != nil {
		s.WG = kernels.WorkGroupSnapshot(l.wg)
	}
	return s
}
func (s Location) restore() (dispatchLocation, error) {
	l := dispatchLocation{valid: s.Valid, cuID: s.CUID, cu: s.CU, locations: s.Locations}
	if s.HasWG {
		var err error
		l.wg, err = s.WG.Restore()
		if err != nil {
			return l, err
		}
	}
	return l, nil
}

type PartitionState struct {
	Grid                               kernels.GridBuilderState
	Current                            Location
	Dispatched, Count, FirstCU, NextCU int
}

type AlgorithmState struct {
	Grid                                                                kernels.GridBuilderState
	Current                                                             Location
	NextCU, Dispatched, NextPartition, NumWG, WGPerPartition, CUsPerDie int
	Partitions                                                          []PartitionState
}

type State struct {
	Algorithm           AlgorithmState
	Current             Location
	Inflight            map[uint64]Location
	DieCyclesLeft       []int
	Dispatching         messaging.Msg
	IsDispatching       bool
	CycleLeft           int
	NumDispatchedWGs    int
	NumCompletedWGs     int
	WgTaskIDs           map[uint64]uint64
	LaunchTaskID        uint64
	FirstKernelLaunched bool
	PrevKernelWGCount   int
}

func mustRestore(l Location) dispatchLocation {
	v, err := l.restore()
	if err != nil {
		panic(err)
	}
	return v
}
func currentWG(l Location) *kernels.WorkGroup { return mustRestore(l).wg }
func retainWG(wg *kernels.WorkGroup) Location { return saveLocation(dispatchLocation{wg: wg}) }
