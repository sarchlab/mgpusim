package cu

import (
	"math"

	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

// A FetchArbiter can decide which wavefront in a scheduler can fetch
// instructions
type FetchArbiter struct {
	InstBufByteSize int

	// FetchAheadLines is the number of lines a wavefront may have in flight.
	// Zero or one fetches one line at a time.
	FetchAheadLines int
}

// Arbitrate decide which wavefront can fetch the next instruction
func (a *FetchArbiter) Arbitrate(
	wfPools []*WavefrontPool,
) []*Wavefront {
	list := make([]*Wavefront, 0, 1)

	oldestTime := timing.VTimeInPicoSec(math.MaxUint64)
	var toFetch *Wavefront
	for _, wfPool := range wfPools {
		for _, handle := range wfPool.ids() {
			wf := wfPool.cu.wave(handle)
			if !a.canFetchFromWF(wf) {
				continue
			}

			if wf.execution().LastFetchTime < oldestTime {
				toFetch = wf
				oldestTime = wf.execution().LastFetchTime
			}
		}
	}

	if toFetch != nil {
		list = append(list, toFetch)
	}

	return list
}

func (a *FetchArbiter) canFetchFromWF(wf *Wavefront) bool {
	state := wf.execution()
	if wf.state().StopFetch {
		return false
	}

	// Without fetch-ahead a wavefront has one fetch outstanding at a time,
	// including one made obsolete by a taken branch.
	if a.FetchAheadLines <= 1 && state.IsFetching {
		return false
	}

	if state.State == wavefront.WfCompleted {
		return false
	}

	nextFetchPC := nextFetchPC(state)
	bufferEnd := state.InstBufferStartPC + uint64(len(state.InstBuffer))
	if nextFetchPC-bufferEnd >= uint64(max(a.FetchAheadLines, 1))*instFetchByteSize {
		return false
	}

	if nextFetchPC-state.InstBufferStartPC >= uint64(a.InstBufByteSize) {
		return false
	}

	group := wf.group()
	co := &group.raw().CodeObject
	if co.Present && co.HasSymbol {
		lastPCInBinary := co.Symbol.Size +
			group.packet().KernelObject + co.Meta.KernelCodeEntryByteOffset
		if nextFetchPC >= lastPCInBinary {
			return false
		}
	}

	return true
}

// instFetchByteSize is the size of one instruction fetch, a cache line.
const instFetchByteSize = 64

// nextFetchPC returns the address of the next line the wavefront requests:
// the line after those buffered and in flight in the current fetch stream.
func nextFetchPC(state *wavefront.ExecutionState) uint64 {
	bufferEnd := state.InstBufferStartPC + uint64(len(state.InstBuffer))
	bufferEnd &^= instFetchByteSize - 1

	return max(state.NextFetchPC, bufferEnd)
}

// redirectFetch starts a new fetch stream at the wavefront's PC. The
// instruction buffer is emptied, and the fetches still in flight are dropped
// when they return.
func redirectFetch(wf *Wavefront) {
	state := wf.execution()
	state.InstBuffer = nil
	state.InstBufferStartPC = wf.PC() &^ (instFetchByteSize - 1)
	state.FetchEpoch++
	state.NextFetchPC = 0
}
