package writeback

import (
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
)

// ctrlMiddleware owns every control verb except CmdFlush, which is
// handled by the legacy flusher pipeline. The verbs it owns are
// Pause, Drain, Enable, Reset, Invalidate. CmdFlush is left in the
// Control port's incoming queue for the flusher to pick up.
type ctrlMiddleware struct {
	pipeline *pipelineMW
}

// ctrlPort resolves the "Control" port by name. The port instance is assigned
// externally after Build, so it is resolved lazily on every use rather than
// cached at build time.
func (m *ctrlMiddleware) ctrlPort() messaging.Port {
	return m.pipeline.comp.GetPortByName("Control")
}

func (m *ctrlMiddleware) Tick() bool {
	madeProgress := false
	madeProgress = m.completePendingDrain() || madeProgress
	// Control commands are processed serially: while an async verb (Drain or
	// Flush) is in progress, the next command is not accepted — it stays queued
	// on the Control port and is handled once the component settles. Flush is
	// driven by the separate flusher middleware, which gates its own intake the
	// same way.
	next := &m.pipeline.comp.State
	if cacheState(next.CacheState) != cacheStateDraining &&
		!next.HasProcessingFlush {
		madeProgress = m.handleIncoming() || madeProgress
	}
	return madeProgress
}

// completePendingDrain finalizes a pending Drain once no transactions
// and no MSHR-stage work remain.
func (m *ctrlMiddleware) completePendingDrain() bool {
	next := &m.pipeline.comp.State
	if cacheState(next.CacheState) != cacheStateDraining {
		return false
	}

	if !cacheIsQuiescent(next) {
		return false
	}

	if !m.ctrlPort().CanSend() {
		return false
	}

	m.ctrlPort().Send(makeCtrlRsp(m.ctrlPort(), memcontrolprotocol.CmdDrain,
		next.CurrentCmdSrc, next.CurrentCmdID, true, ""))
	next.CacheState = int(cacheStatePaused)
	next.CurrentCmdID = 0
	next.CurrentCmdSrc = ""
	return true
}

func cacheIsQuiescent(state *State) bool {
	// Completed transactions are only marked Removed (the slice is compacted
	// at Reset, not on each completion), so quiescence means no transaction
	// is still live, not that the slice is empty.
	for i := range state.Transactions {
		if !state.Transactions[i].Removed {
			return false
		}
	}
	if state.WriteBufferBuf.Size() > 0 {
		return false
	}
	for _, c := range state.BankInflightTransCounts {
		if c > 0 {
			return false
		}
	}
	for _, c := range state.BankDownwardInflightTransCounts {
		if c > 0 {
			return false
		}
	}
	return true
}

func (m *ctrlMiddleware) handleIncoming() bool {
	msg := m.ctrlPort().PeekIncoming()
	if msg == nil {
		return false
	}

	req, ok := msg.(memcontrolprotocol.Req)
	if !ok {
		return false
	}

	switch req.Command {
	case memcontrolprotocol.CmdPause:
		return m.handlePause(req)
	case memcontrolprotocol.CmdDrain:
		return m.handleDrain(req)
	case memcontrolprotocol.CmdEnable:
		return m.handleEnable(req)
	case memcontrolprotocol.CmdReset:
		return m.handleReset(req)
	case memcontrolprotocol.CmdFlush:
		// Owned by flusher; leave in queue.
		return false
	case memcontrolprotocol.CmdInvalidate:
		return m.handleInvalidate(req)
	default:
		return m.handleUnsupported(req)
	}
}

// handleInvalidate drops cached blocks matching the request's
// address/PID filter (empty address list = all addresses, zero PID = all
// PIDs) WITHOUT writeback: matched blocks are marked invalid even if
// dirty. Per the resolved protocol decision, Invalidate discards dirty
// data silently; a caller that wants to keep it must Flush first.
// Invalidate is acknowledged synchronously but is only legal once the
// cache is paused (or drained, which lands in paused); issued while
// Running it is rejected with ErrMustBePausedOrDrained.
func (m *ctrlMiddleware) handleInvalidate(req memcontrolprotocol.Req) bool {
	next := &m.pipeline.comp.State
	if cacheState(next.CacheState) != cacheStatePaused {
		return m.rejectMustBePaused(req)
	}
	if !m.ctrlPort().CanSend() {
		return false
	}

	spec := m.pipeline.comp.Spec()
	blockSize := uint64(1) << spec.Log2BlockSize
	invalidateBlocks(next, blockSize, req.Addresses, req.PID)

	m.ctrlPort().Send(makeCtrlRsp(m.ctrlPort(), memcontrolprotocol.CmdInvalidate,
		req.Src, req.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()
	return true
}

// rejectMustBePaused responds that a conditional verb is illegal while
// the cache is Running (Enabled).
func (m *ctrlMiddleware) rejectMustBePaused(req memcontrolprotocol.Req) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}
	m.ctrlPort().Send(makeCtrlRsp(m.ctrlPort(), req.Command,
		req.Src, req.ID, false, memcontrolprotocol.ErrMustBePausedOrDrained))
	m.ctrlPort().RetrieveIncoming()
	return true
}

// invalidateBlocks marks every valid directory block matching the filter
// invalid and clean. An empty address list matches every block address; a
// zero PID matches every PID. Block addresses are cache-line aligned in
// Tag, so the requested addresses are aligned to the block before
// matching.
func invalidateBlocks(
	state *State,
	blockSize uint64,
	addresses []uint64,
	pid vm.PID,
) {
	matchAddr := make(map[uint64]bool, len(addresses))
	for _, a := range addresses {
		matchAddr[a/blockSize*blockSize] = true
	}

	for si := range state.DirectoryState.Sets {
		set := &state.DirectoryState.Sets[si]
		for wi := range set.Blocks {
			block := &set.Blocks[wi]
			if !block.IsValid {
				continue
			}
			if pid != 0 && vm.PID(block.PID) != pid {
				continue
			}
			if len(addresses) > 0 && !matchAddr[block.Tag] {
				continue
			}

			block.IsValid = false
			block.IsDirty = false
			block.DirtyMask = nil
		}
	}
}

func (m *ctrlMiddleware) handlePause(req memcontrolprotocol.Req) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}
	next := &m.pipeline.comp.State
	next.CacheState = int(cacheStatePaused)
	m.ctrlPort().Send(makeCtrlRsp(m.ctrlPort(), memcontrolprotocol.CmdPause,
		req.Src, req.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()
	return true
}

func (m *ctrlMiddleware) handleDrain(req memcontrolprotocol.Req) bool {
	next := &m.pipeline.comp.State
	next.CacheState = int(cacheStateDraining)
	next.CurrentCmdID = req.ID
	next.CurrentCmdSrc = req.Src
	m.ctrlPort().RetrieveIncoming()
	return true
}

func (m *ctrlMiddleware) handleEnable(req memcontrolprotocol.Req) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}
	// Enable resumes from Paused; it must not discard traffic queued while
	// paused (e.g. bottom responses for frozen in-flight transactions),
	// which the pipeline processes once it runs again.
	m.pipeline.comp.State.CacheState = int(cacheStateRunning)
	m.ctrlPort().Send(makeCtrlRsp(m.ctrlPort(), memcontrolprotocol.CmdEnable,
		req.Src, req.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()
	return true
}

// handleReset wipes the cache back to a freshly-built shape: empty
// directory, empty MSHR, empty buffers, empty in-flight transactions,
// empty port queues. No writeback happens — dirty data is discarded
// per the resolved-decision policy ("Invalidate-on-dirty: drop
// silently" generalized to Reset).
func (m *ctrlMiddleware) handleReset(req memcontrolprotocol.Req) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}

	next := &m.pipeline.comp.State
	spec := m.pipeline.comp.Spec()
	blockSize := 1 << spec.Log2BlockSize
	cache.DirectoryReset(
		&next.DirectoryState, spec.NumSets, spec.WayAssociativity, blockSize)
	next.MSHRState = cache.MSHRState{}
	next.Transactions = nil
	next.EvictingList = map[uint64]bool{}

	clearCachePipelinesAndBuffers(next)

	next.FlusherBlockToEvictRefs = nil
	next.HasProcessingFlush = false
	next.ProcessingFlush = flushReqState{}
	next.CurrentCmdID = 0
	next.CurrentCmdSrc = ""
	next.CacheState = int(cacheStateRunning)

	clearPort(m.pipeline.topPort())
	clearPort(m.pipeline.bottomPort())

	m.ctrlPort().Send(makeCtrlRsp(m.ctrlPort(), memcontrolprotocol.CmdReset,
		req.Src, req.ID, true, ""))
	m.ctrlPort().RetrieveIncoming()
	return true
}

// clearCachePipelinesAndBuffers empties every stage buffer, pipeline, and
// per-bank in-flight counter, and clears the MSHR/write-buffer stage
// bookkeeping. It does not touch the directory, MSHR contents, or the
// transaction table.
func clearCachePipelinesAndBuffers(next *State) {
	next.DirStageBuf.Clear()
	for i := range next.DirToBankBufs {
		next.DirToBankBufs[i].Clear()
	}
	for i := range next.WriteBufferToBankBufs {
		next.WriteBufferToBankBufs[i].Clear()
	}
	next.MSHRStageBuf.Clear()
	next.WriteBufferBuf.Clear()
	next.DirPipeline.Clear()
	next.DirPostPipelineBuf.Clear()
	for i := range next.BankPipelines {
		next.BankPipelines[i].Clear()
	}
	for i := range next.BankPostPipelineBufs {
		next.BankPostPipelineBufs[i].Clear()
	}
	for i := range next.BankInflightTransCounts {
		next.BankInflightTransCounts[i] = 0
	}
	for i := range next.BankDownwardInflightTransCounts {
		next.BankDownwardInflightTransCounts[i] = 0
	}
	next.PendingEvictionIndices = nil
	next.InflightFetchIndices = nil
	next.InflightEvictionIndices = nil
	next.HasProcessingMSHREntry = false
	next.ProcessingMSHREntryIdx = 0
}

func (m *ctrlMiddleware) handleUnsupported(req memcontrolprotocol.Req) bool {
	if !m.ctrlPort().CanSend() {
		return false
	}
	m.ctrlPort().Send(makeCtrlRsp(m.ctrlPort(), req.Command,
		req.Src, req.ID, false, memcontrolprotocol.ErrUnsupported))
	m.ctrlPort().RetrieveIncoming()
	return true
}

func makeCtrlRsp(
	port messaging.Port,
	cmd memcontrolprotocol.Command,
	dst messaging.RemotePort,
	rspTo uint64,
	success bool,
	errStr string,
) memcontrolprotocol.Rsp {
	rsp := memcontrolprotocol.Rsp{
		Command: cmd,
		Success: success,
		Error:   errStr,
	}
	rsp.ID = timing.GetIDGenerator().Generate()
	rsp.Src = port.AsRemote()
	rsp.Dst = dst
	rsp.RspTo = rspTo
	rsp.TrafficClass = "memcontrolprotocol.Rsp"
	return rsp
}
