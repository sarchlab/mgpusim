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

	codeEnd, codeEndKnown := codeEnd(wf)
	linesInFlight, bufferSize := a.limits(codeEndKnown)

	nextFetchPC := nextFetchPC(state)
	bufferEnd := state.InstBufferStartPC + uint64(len(state.InstBuffer))
	if nextFetchPC-bufferEnd >= uint64(linesInFlight)*InstFetchByteSize {
		return false
	}

	if nextFetchPC-state.InstBufferStartPC >= uint64(bufferSize) {
		return false
	}

	if codeEndKnown && nextFetchPC >= codeEnd {
		return false
	}

	return true
}

// limits returns the number of fetches a wavefront may have in flight and the
// number of bytes, buffered and in flight, it may hold.
func (a *FetchArbiter) limits(codeEndKnown bool) (linesInFlight, bufferSize int) {
	if a.FetchAheadLines <= 1 {
		return 1, a.InstBufByteSize
	}

	// Fetching ahead requests lines the wavefront may never execute, so it
	// needs to know where the code ends. A wavefront that does not know
	// fetches one line at a time into a buffer of the default size, which
	// bounds how far past its instructions it reads.
	if !codeEndKnown {
		return 1, min(a.InstBufByteSize, DefaultInstBufByteSize)
	}

	return a.FetchAheadLines, a.InstBufByteSize
}

// codeEnd returns the address after the last instruction of the wavefront's
// kernel, when the code object records it.
func codeEnd(wf *Wavefront) (uint64, bool) {
	group := wf.group()
	co := &group.raw().CodeObject
	if !co.Present || !co.HasSymbol {
		return 0, false
	}

	return co.Symbol.Size + group.packet().KernelObject + co.Meta.KernelCodeEntryByteOffset, true
}

// InstFetchByteSize is the size of one instruction fetch.
const InstFetchByteSize = 64

// DefaultInstBufByteSize is the default size of a wavefront's instruction
// buffer.
const DefaultInstBufByteSize = 256

// nextFetchPC returns the address of the next line the wavefront requests:
// the line after those buffered and in flight in the current fetch stream.
func nextFetchPC(state *wavefront.ExecutionState) uint64 {
	bufferEnd := state.InstBufferStartPC + uint64(len(state.InstBuffer))
	bufferEnd &^= InstFetchByteSize - 1

	return max(state.NextFetchPC, bufferEnd)
}

// redirectFetch starts a new fetch stream at the wavefront's PC. The
// instruction buffer is emptied, and the fetches still in flight are dropped
// when they return.
func redirectFetch(wf *Wavefront) {
	state := wf.execution()
	state.InstBuffer = nil
	state.InstBufferStartPC = wf.PC() &^ (InstFetchByteSize - 1)
	state.FetchEpoch++
	state.NextFetchPC = 0
}
