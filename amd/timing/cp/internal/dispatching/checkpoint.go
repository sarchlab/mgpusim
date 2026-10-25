package dispatching

import (
	"fmt"
	"io"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
)

type savedLocation struct {
	Valid     bool
	CUID      int
	CU        messaging.RemotePort
	HasWG     bool
	WG        kernels.WorkGroupValue
	Locations []protocol.WfDispatchLocation
}

func saveLocation(l dispatchLocation) savedLocation {
	s := savedLocation{Valid: l.valid, CUID: l.cuID, CU: l.cu, Locations: l.locations, HasWG: l.wg != nil}
	if l.wg != nil {
		s.WG = kernels.WorkGroupSnapshot(l.wg)
	}
	return s
}
func (s savedLocation) restore() (dispatchLocation, error) {
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

type savedPartition struct {
	Grid                               kernels.GridBuilderState
	Current                            savedLocation
	Dispatched, Count, FirstCU, NextCU int
}

type algorithmState struct {
	Kind                                                                string
	Grid                                                                kernels.GridBuilderState
	Current                                                             savedLocation
	NextCU, Dispatched, NextPartition, NumWG, WGPerPartition, CUsPerDie int
	Initialized                                                         bool
	Partitions                                                          []savedPartition
}

func (d *DispatcherImpl) snapshotAlgorithm() algorithmState {
	switch a := d.alg.(type) {
	case *roundRobinAlgorithm:
		return algorithmState{Kind: "round-robin",
			Grid:       a.gridBuilder.Snapshot(),
			Current:    saveLocation(dispatchLocation{wg: a.currWG}),
			NextCU:     a.nextCU,
			Dispatched: a.numDispatchedWGs}
	case *greedyAlgorithm:
		return algorithmState{Kind: "greedy",
			Grid:       a.gridBuilder.Snapshot(),
			Current:    saveLocation(dispatchLocation{wg: a.currWG}),
			Dispatched: a.numDispatchedWGs}
	case *partitionAlgorithm:
		s := algorithmState{Kind: "partition",
			NextPartition:  a.nextPartition,
			NumWG:          a.numWG,
			Dispatched:     a.numDispatchedWG,
			WGPerPartition: a.numWGPerPartition,
			Initialized:    a.initialized}
		for i, p := range a.partitions {
			s.Partitions = append(s.Partitions,
				savedPartition{Grid: p.gridBuilder.Snapshot(),
					Current:    saveLocation(dispatchLocation{wg: a.currWGs[i]}),
					Dispatched: p.dispatchedWG})
		}
		return s
	case *perDieAlgorithm:
		s := algorithmState{Kind: "per-die", NumWG: a.numWG, CUsPerDie: a.cusPerDie, Dispatched: a.numDispatchedWG}
		for _, p := range a.dies {
			s.Partitions = append(s.Partitions,
				savedPartition{Grid: p.gridBuilder.Snapshot(),
					Current:    saveLocation(dispatchLocation{wg: p.currWG}),
					Dispatched: p.dispatchedWG,
					Count:      p.numWGInDie,
					FirstCU:    p.firstCU,
					NextCU:     p.nextCUOffset})
		}
		return s
	default:
		panic("dispatching: unknown algorithm")
	}
}

func (d *DispatcherImpl) restoreAlgorithm(s algorithmState) error {
	current, err := s.Current.restore()
	if err != nil {
		return err
	}
	currents := make([]*kernels.WorkGroup, len(s.Partitions))
	for i, p := range s.Partitions {
		l, err := p.Current.restore()
		if err != nil {
			return err
		}
		currents[i] = l.wg
	}
	switch a := d.alg.(type) {
	case *roundRobinAlgorithm:
		a.gridBuilder.Restore(s.Grid)
		a.currWG = current.wg
		a.nextCU = s.NextCU
		a.numDispatchedWGs = s.Dispatched
	case *greedyAlgorithm:
		a.gridBuilder.Restore(s.Grid)
		a.currWG = current.wg
		a.numDispatchedWGs = s.Dispatched
	case *partitionAlgorithm:
		a.restorePartitions(s, currents)
	case *perDieAlgorithm:
		a.restoreDies(s, currents)
	default:
		return fmt.Errorf("dispatching: unknown algorithm")
	}
	return nil
}

type dispatcherState struct {
	Algorithm           algorithmState
	Current             savedLocation
	Inflight            map[uint64]savedLocation
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

type dispatcherConfig struct {
	Algorithm                                                            string
	NumDies                                                              int
	WavefrontCycles                                                      int
	Latency                                                              []int
	KernelOverhead, LaunchOverhead, SubsequentOverhead, ScalingThreshold int
}

func (d *DispatcherImpl) checkpointConfig() dispatcherConfig {
	n := 0
	if d.dieAware != nil {
		n = d.dieAware.NumDies()
	}
	return dispatcherConfig{
		d.snapshotAlgorithm().Kind,
		n,
		d.wavefrontDispatchCycles,
		d.latencyTable,
		d.constantKernelOverhead,
		d.constantKernelLaunchOverhead,
		d.subsequentKernelLaunchOverhead,
		d.wgScalingThreshold,
	}
}

// SaveCheckpoint preserves the complete dispatcher state at an engine boundary.
func (d *DispatcherImpl) SaveCheckpoint(w io.Writer) error {
	s := dispatcherState{
		Algorithm:           d.snapshotAlgorithm(),
		Current:             saveLocation(d.currWG),
		Inflight:            make(map[uint64]savedLocation),
		DieCyclesLeft:       d.dieCyclesLeft,
		Dispatching:         d.dispatching,
		IsDispatching:       d.isDispatching,
		CycleLeft:           d.cycleLeft,
		NumDispatchedWGs:    d.numDispatchedWGs,
		NumCompletedWGs:     d.numCompletedWGs,
		WgTaskIDs:           d.wgTaskIDs,
		LaunchTaskID:        d.launchTaskID,
		FirstKernelLaunched: d.firstKernelLaunched,
		PrevKernelWGCount:   d.prevKernelWGCount,
	}
	for id, l := range d.inflightWGs {
		s.Inflight[id] = saveLocation(l)
	}
	return modeling.WriteCheckpoint(w, d.checkpointConfig(), s, nil)
}

// LoadCheckpoint restores work-group identities without re-enumerating work or
// reserving resources. The separately saved shared pool owns the reservations.
func (d *DispatcherImpl) LoadCheckpoint(r io.Reader) error {
	var s dispatcherState
	if err := modeling.ReadCheckpoint(r, d.checkpointConfig(), &s, nil); err != nil {
		return err
	}
	current, err := s.Current.restore()
	if err != nil {
		return err
	}
	inflight := make(map[uint64]dispatchLocation, len(s.Inflight))
	for id, l := range s.Inflight {
		v, err := l.restore()
		if err != nil {
			return err
		}
		inflight[id] = v
	}
	if err := d.restoreAlgorithm(s.Algorithm); err != nil {
		return err
	}
	d.currWG = current
	d.inflightWGs = inflight
	d.dieCyclesLeft = s.DieCyclesLeft
	d.dispatching = s.Dispatching
	d.isDispatching = s.IsDispatching
	d.cycleLeft = s.CycleLeft
	d.numDispatchedWGs = s.NumDispatchedWGs
	d.numCompletedWGs = s.NumCompletedWGs
	d.wgTaskIDs = s.WgTaskIDs
	d.launchTaskID = s.LaunchTaskID
	d.firstKernelLaunched = s.FirstKernelLaunched
	d.prevKernelWGCount = s.PrevKernelWGCount

	d.progressBar = nil
	if d.isDispatching {
		d.initializeProgressBar(d.dispatching.ID)
		if d.progressBar != nil {
			d.progressBar.IncrementInProgress(uint64(d.numDispatchedWGs))
			d.progressBar.MoveInProgressToFinished(uint64(d.numCompletedWGs))
		}
	}
	return nil
}

func (a *partitionAlgorithm) restorePartitions(s algorithmState, currents []*kernels.WorkGroup) {
	a.nextPartition = s.NextPartition
	a.numWG = s.NumWG
	a.numDispatchedWG = s.Dispatched
	a.numWGPerPartition = s.WGPerPartition
	a.initialized = s.Initialized
	a.currWGs = currents
	a.partitions = make([]*partition, len(s.Partitions))
	for i, p := range s.Partitions {
		g := kernels.NewGridBuilder(a.ids)
		g.Restore(p.Grid)
		a.partitions[i] = &partition{gridBuilder: g, dispatchedWG: p.Dispatched}
	}
}

func (a *perDieAlgorithm) restoreDies(s algorithmState, currents []*kernels.WorkGroup) {
	a.numWG = s.NumWG
	a.cusPerDie = s.CUsPerDie
	a.numDispatchedWG = s.Dispatched
	a.dies = make([]*dieState, len(s.Partitions))
	for i, p := range s.Partitions {
		g := kernels.NewGridBuilder(a.ids)
		g.Restore(p.Grid)
		a.dies[i] = &dieState{
			gridBuilder:  g,
			currWG:       currents[i],
			numWGInDie:   p.Count,
			dispatchedWG: p.Dispatched,
			firstCU:      p.FirstCU,
			nextCUOffset: p.NextCU,
		}
	}
}
