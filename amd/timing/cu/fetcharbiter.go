package cu

import (
	"math"

	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

// A FetchArbiter decides which wavefront fetches the next line of
// instructions, when the compute unit fetches whole lines (no block fetch).
type FetchArbiter struct {
	InstBufByteSize int
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

// canFetchFromWF reports whether the wavefront can request its next line. A
// wavefront has one fetch outstanding at a time, including one made obsolete
// by a taken branch.
func (a *FetchArbiter) canFetchFromWF(wf *Wavefront) bool {
	state := wf.execution()
	if wf.state().StopFetch || state.IsFetching {
		return false
	}

	if state.State == wavefront.WfCompleted {
		return false
	}

	nextFetchPC := nextFetchPC(state)
	bufferEnd := state.InstBufferStartPC + uint64(len(state.InstBuffer))
	if nextFetchPC-bufferEnd >= InstFetchByteSize {
		return false
	}

	if nextFetchPC-state.InstBufferStartPC >= uint64(a.InstBufByteSize) {
		return false
	}

	if codeEnd, known := codeEnd(wf); known && nextFetchPC >= codeEnd {
		return false
	}

	return true
}

// knowsCodeEnd reports whether the wavefront's code object records exactly
// where its code ends, so that lines read ahead stay within the code. A V2 or
// V3 symbol also covers the 256-byte header that is not part of the uploaded
// code, so its end is past the code.
func knowsCodeEnd(wf *Wavefront) bool {
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

// InstFetchByteSize is the size of one instruction fetch without block fetch,
// and of one line read ahead into the instruction cache with it.
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

// redirectFetch starts a new fetch stream at the wavefront's PC, whose
// buffer starts at the fetch unit (line or block) holding the PC. The
// instruction buffer is emptied, and the fetches still in flight are dropped
// when they return.
func (cu *ComputeUnit) redirectFetch(wf *Wavefront) {
	state := wf.execution()
	state.InstBuffer = nil
	state.InstBufferStartPC = wf.PC() &^ (cu.fetchUnitByteSize() - 1)
	state.FetchEpoch++
	state.NextFetchPC = 0
	state.IssuedEnd = 0
	state.PrefetchPC = 0
	state.BlockFetchNotBefore = 0
}

// fetchUnitByteSize is the size of one instruction-buffer fetch: a block
// with block fetch, otherwise a line.
func (cu *ComputeUnit) fetchUnitByteSize() uint64 {
	if b := cu.comp.Spec.InstBlockByteSize; b > 0 {
		return uint64(b)
	}

	return InstFetchByteSize
}
