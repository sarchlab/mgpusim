package cu

import (
	"math"

	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
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
	ahead := fetchesAhead(wf, a.FetchAheadLines)
	if !ahead && state.IsFetching {
		return false
	}

	if state.State == wavefront.WfCompleted {
		return false
	}

	linesInFlight, bufferSize := a.limits(ahead)

	nextFetchPC := nextFetchPC(state)
	bufferEnd := state.InstBufferStartPC + uint64(len(state.InstBuffer))
	if nextFetchPC-bufferEnd >= uint64(linesInFlight)*InstFetchByteSize {
		return false
	}

	if nextFetchPC-state.InstBufferStartPC >= uint64(bufferSize) {
		return false
	}

	if codeEnd, known := codeEnd(wf); known && nextFetchPC >= codeEnd {
		return false
	}

	return true
}

// limits returns the number of fetches a wavefront may have in flight and the
// number of bytes, buffered and in flight, it may hold.
func (a *FetchArbiter) limits(fetchesAhead bool) (linesInFlight, bufferSize int) {
	if fetchesAhead {
		return a.FetchAheadLines, a.InstBufByteSize
	}

	// A wavefront that cannot fetch ahead in a compute unit that does keeps
	// the default buffer, so that it reads no farther past its instructions
	// than it would without fetch-ahead.
	if a.FetchAheadLines > 1 {
		return 1, min(a.InstBufByteSize, DefaultInstBufByteSize)
	}

	return 1, a.InstBufByteSize
}

// fetchesAhead reports whether the wavefront fetches ahead, given the number
// of lines its compute unit lets a wavefront keep in flight. Lines fetched
// ahead may never execute, so the wavefront must know exactly where its code
// ends. Otherwise it fetches one line at a time, in every respect: one fetch
// outstanding, and a returned line is used whenever it continues the buffer.
func fetchesAhead(wf *Wavefront, lines int) bool {
	if lines <= 1 {
		return false
	}

	// A V2 or V3 symbol also covers the 256-byte header that is not part of
	// the uploaded code, so its end is past the code.
	co := &wf.group().raw().CodeObject
	return co.Present && co.HasSymbol && co.Version == insts.CodeObjectV5
}

// codeEnd returns the address after the last instruction of the wavefront's
// kernel, when the code object records it. For a V2 or V3 code object it is
// 256 bytes past the uploaded code.
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
