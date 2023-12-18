package cu

import (
	"math"

	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/timing/wavefront"
)

// A FetchArbiter can decide which wavefront in a scheduler can fetch
// instructions
type FetchArbiter struct {
	InstBufByteSize int
}

// Arbitrate decide which wavefront can fetch the next instruction
func (a *FetchArbiter) Arbitrate(
	wfPools []*WavefrontPool,
) []*wavefront.Wavefront {
	list := make([]*wavefront.Wavefront, 0, 1)

	oldestTime := sim.VTimeInSec(math.MaxFloat64)
	var toFetch *wavefront.Wavefront
	for _, wfPool := range wfPools {
		for _, wf := range wfPool.wfs {
			wf.RLock()
			if !a.canFetchFromWF(wf) {
				continue
			}

			if wf.LastFetchTime < oldestTime {
				toFetch = wf
				oldestTime = wf.LastFetchTime
			}
			wf.RUnlock()
		}
	}

	if toFetch != nil {
		list = append(list, toFetch)
	}

	return list
}

func (a *FetchArbiter) canFetchFromWF(wf *wavefront.Wavefront) bool {
	if wf.IsFetching {
		return false
	}

	if wf.State == wavefront.WfCompleted {
		return false
	}

	if len(wf.InstBuffer) >= a.InstBufByteSize {
		return false
	}

	if wf.CodeObject != nil && wf.CodeObject.Symbol != nil {
		lastPCInBinary := wf.CodeObject.Symbol.Size +
			wf.WG.Packet.KernelObject
		lastPCInInstBuffer := wf.InstBufferStartPC +
			uint64(len(wf.InstBuffer))
		if lastPCInInstBuffer >= lastPCInBinary {
			// fmt.Printf("lastInstPCInBinary: %016X, lastPCInInstBuffer: %016X, PC: %016X\n",
			// 	lastPCInBinary, lastPCInInstBuffer, wf.PC)
			return false
		}
	}

	return true
}
