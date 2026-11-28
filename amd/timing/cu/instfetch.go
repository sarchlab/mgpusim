package cu

import (
	"math"

	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

// doBlockFetch runs instruction fetch with block fetch (Spec.InstBlockByteSize).
// In one cycle the compute unit requests at most one block into a
// wavefront's instruction buffer and reads at most one line ahead into the
// instruction cache. Prefetches use their own port, so that their responses,
// which are dropped, do not hold up the in-order return of the blocks.
func (s *SchedulerImpl) doBlockFetch() bool {
	madeProgress := false

	if wf := s.blockFetchCandidate(); wf != nil && s.cu.instMemPort().CanSend() {
		s.sendBlockFetch(wf)
		madeProgress = true
	}

	if s.cu.comp.Spec.InstPrefetchLines > 0 && s.cu.instPrefetchPort().CanSend() {
		if wf, addr, ok := s.prefetchCandidate(); ok {
			line := s.cu.instLineByteSize()
			s.sendInstFetch(wf, addr, line, true)
			wf.execution().PrefetchPC = addr + line
			madeProgress = true
		}
	}

	return madeProgress
}

// blockFetchCandidate returns the wavefront that requests a block in this
// cycle: of those with a free buffer block, the one that has waited longest
// for an instruction fetch. A wavefront held back only by the least time
// between its block requests wakes the compute unit when that time is up.
func (s *SchedulerImpl) blockFetchCandidate() *Wavefront {
	now := s.cu.CurrentTime()
	oldestTime := timing.VTimeInPicoSec(math.MaxUint64)
	var toFetch *Wavefront

	for _, wfPool := range s.cu.WfPools {
		for _, handle := range wfPool.ids() {
			wf := wfPool.cu.wave(handle)
			if !s.hasFreeBlock(wf) {
				continue
			}

			state := wf.execution()
			if now < state.BlockFetchNotBefore {
				s.cu.comp.TickAt(state.BlockFetchNotBefore)
				continue
			}

			if state.LastFetchTime < oldestTime {
				toFetch = wf
				oldestTime = state.LastFetchTime
			}
		}
	}

	return toFetch
}

// hasFreeBlock reports whether the wavefront's instruction buffer has a free
// block for the next block of its fetch stream. A block is taken from when it
// is requested until the wavefront issues the instruction holding its last
// byte.
func (s *SchedulerImpl) hasFreeBlock(wf *Wavefront) bool {
	state := wf.execution()
	if wf.state().StopFetch || state.State == wavefront.WfCompleted {
		return false
	}

	next := s.nextBlockPC(wf)
	if codeEnd, known := codeEnd(wf); known && next >= codeEnd {
		return false
	}

	// Once the stream has issued anything, the blocks before the end of the
	// last issued instruction are free. The PC only seeds an empty stream: a
	// taken branch writes its target into the PC before it redirects the
	// stream, and that target frees nothing.
	blockSize := s.cu.fetchUnitByteSize()
	issued := state.IssuedEnd
	if issued == 0 {
		issued = wf.PC()
	}
	firstTaken := issued &^ (blockSize - 1)

	return next-firstTaken < uint64(s.cu.comp.Spec.InstBufByteSize)
}

// nextBlockPC returns the address of the next block the wavefront requests in
// its fetch stream: the block holding the PC when the stream has requested
// nothing yet.
func (s *SchedulerImpl) nextBlockPC(wf *Wavefront) uint64 {
	if next := wf.execution().NextFetchPC; next != 0 {
		return next
	}

	return wf.PC() &^ (s.cu.fetchUnitByteSize() - 1)
}

// sendBlockFetch requests the next block of the wavefront's fetch stream.
func (s *SchedulerImpl) sendBlockFetch(wf *Wavefront) {
	state := wf.execution()
	blockSize := s.cu.fetchUnitByteSize()
	addr := s.nextBlockPC(wf)
	if state.NextFetchPC == 0 && len(state.InstBuffer) == 0 {
		state.InstBufferStartPC = addr
	}

	s.sendInstFetch(wf, addr, blockSize, false)

	state.NextFetchPC = addr + blockSize
	state.BlockFetchNotBefore = s.cu.comp.Spec.Freq.NCyclesLater(
		s.cu.comp.Spec.InstBlockFetchCycles, s.cu.CurrentTime())
}

// prefetchCandidate returns a wavefront and the line it reads ahead into the
// instruction cache in this cycle, if any: of the wavefronts with a line left
// to prefetch, the one that has waited longest for an instruction fetch.
func (s *SchedulerImpl) prefetchCandidate() (*Wavefront, uint64, bool) {
	oldestTime := timing.VTimeInPicoSec(math.MaxUint64)
	var toFetch *Wavefront
	var toFetchAddr uint64

	for _, wfPool := range s.cu.WfPools {
		for _, handle := range wfPool.ids() {
			wf := wfPool.cu.wave(handle)
			addr, ok := s.nextPrefetchLine(wf)
			if !ok {
				continue
			}

			if wf.execution().LastFetchTime < oldestTime {
				toFetch = wf
				toFetchAddr = addr
				oldestTime = wf.execution().LastFetchTime
			}
		}
	}

	return toFetch, toFetchAddr, toFetch != nil
}

// nextPrefetchLine returns the next line the wavefront reads ahead into the
// instruction cache. The wavefront reads up to Spec.InstPrefetchLines lines
// past the line of the next block it requests, never past the end of its
// code. It skips the lines that the compute unit has a request in flight for
// and records them as read.
func (s *SchedulerImpl) nextPrefetchLine(wf *Wavefront) (uint64, bool) {
	state := wf.execution()
	if wf.state().StopFetch || state.State == wavefront.WfCompleted || !knowsCodeEnd(wf) {
		return 0, false
	}

	codeEnd, _ := codeEnd(wf)
	line := s.cu.instLineByteSize()
	demandLine := s.nextBlockPC(wf) &^ (line - 1)
	last := demandLine + uint64(s.cu.comp.Spec.InstPrefetchLines)*line

	addr := max(state.PrefetchPC, demandLine+line)
	for ; addr <= last && addr < codeEnd; addr += line {
		if !s.cu.lineInFlight(wf.PID(), addr) {
			state.PrefetchPC = addr
			return addr, true
		}
	}

	state.PrefetchPC = addr

	return 0, false
}

// instLineByteSize is the size of a line of the instruction cache, which
// prefetches read whole.
func (cu *ComputeUnit) instLineByteSize() uint64 {
	return 1 << cu.comp.Spec.Log2CachelineSize
}

// lineInFlight reports whether the compute unit has an instruction-memory
// request in flight that reads the line in the address space of the process.
func (cu *ComputeUnit) lineInFlight(pid vm.PID, line uint64) bool {
	lineSize := cu.instLineByteSize()
	for _, info := range cu.comp.State.InstFetch {
		read := info.Req.Payload.(memprotocol.ReadReq)
		if read.PID == pid && info.Address&^(lineSize-1) == line {
			return true
		}
	}

	return false
}
