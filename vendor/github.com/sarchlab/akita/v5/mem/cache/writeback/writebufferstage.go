package writeback

import (
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

type writeBufferStage struct {
	cache *pipelineMW
}

func (wb *writeBufferStage) Tick() bool {
	madeProgress := false

	madeProgress = wb.write() || madeProgress
	madeProgress = wb.processReturnRsp() || madeProgress
	madeProgress = wb.processNewTransaction() || madeProgress

	return madeProgress
}

func (wb *writeBufferStage) processNewTransaction() bool {
	next := wb.cache.comp.GetNextState()
	wbBuf := &next.WriteBufferBuf

	if wbBuf.Size() == 0 {
		return false
	}

	transIdx := wbBuf.Elements[0]
	trans := &next.Transactions[transIdx]

	switch trans.Action {
	case writeBufferFetch:
		return wb.processWriteBufferFetch(transIdx, trans)
	case writeBufferEvictAndWrite:
		return wb.processWriteBufferEvictAndWrite(transIdx, trans)
	case writeBufferEvictAndFetch:
		return wb.processWriteBufferFetchAndEvict(transIdx, trans)
	case writeBufferFlush:
		return wb.processWriteBufferFlush(transIdx, trans, true)
	default:
		panic("unknown transaction action")
	}
}

func (wb *writeBufferStage) processWriteBufferFetch(
	transIdx int,
	trans *transactionState,
) bool {
	if wb.findDataLocally(trans) {
		return wb.sendFetchedDataToBank(transIdx, trans)
	}

	return wb.fetchFromBottom(transIdx, trans)
}

func (wb *writeBufferStage) findDataLocally(trans *transactionState) bool {
	next := wb.cache.comp.GetNextState()

	for _, eIdx := range next.InflightEvictionIndices {
		e := &next.Transactions[eIdx]
		if e.EvictingAddr == trans.FetchAddress {
			trans.FetchedData = e.EvictingData
			return true
		}
	}

	for _, eIdx := range next.PendingEvictionIndices {
		e := &next.Transactions[eIdx]
		if e.EvictingAddr == trans.FetchAddress {
			trans.FetchedData = e.EvictingData
			return true
		}
	}

	return false
}

func (wb *writeBufferStage) sendFetchedDataToBank(
	transIdx int,
	trans *transactionState,
) bool {
	spec := wb.cache.comp.GetSpec()
	next := wb.cache.comp.GetNextState()
	bankNum := bankID(trans.BlockSetID, trans.BlockWayID,
		spec.WayAssociativity,
		len(next.WriteBufferToBankBufs))
	bankBuf := &next.WriteBufferToBankBufs[bankNum]

	if !bankBuf.CanPush() {
		trans.FetchedData = nil
		return false
	}

	if !trans.HasMSHREntry {
		panic("sendFetchedDataToBank without MSHR entry")
	}

	mshrIdx := wb.lookupMSHRIndex(trans)
	mshrEntry := &next.MSHRState.Entries[mshrIdx]
	mshrEntry.Data = trans.FetchedData
	trans.Action = bankWriteFetched
	wb.combineData(mshrIdx)

	// Resolve MSHR transaction pointers before removal
	trans.MSHRData = make([]byte, len(mshrEntry.Data))
	copy(trans.MSHRData, mshrEntry.Data)
	trans.MSHRTransactionIndices = wb.resolveEntryTransactionIndices(mshrEntry)

	cache.MSHRRemove(&next.MSHRState,
		vm.PID(mshrEntry.PID), mshrEntry.Address)

	bankBuf.PushTyped(transIdx)

	next.WriteBufferBuf.Elements = next.WriteBufferBuf.Elements[1:]

	return true
}

func (wb *writeBufferStage) fetchFromBottom(
	transIdx int,
	trans *transactionState,
) bool {
	next := wb.cache.comp.GetNextState()

	if wb.tooManyInflightFetches() {
		return false
	}

	if !wb.cache.bottomPort.CanSend() {
		return false
	}

	spec := wb.cache.comp.GetSpec()
	lowModulePort := wb.cache.findPort(trans.FetchAddress)
	read := &mem.ReadReq{}
	read.ID = sim.GetIDGenerator().Generate()
	read.Src = wb.cache.bottomPort.AsRemote()
	read.Dst = lowModulePort
	read.PID = trans.FetchPID
	read.Address = trans.FetchAddress
	read.AccessByteSize = 1 << spec.Log2BlockSize
	read.TrafficBytes = 12
	read.TrafficClass = "mem.ReadReq"
	wb.cache.bottomPort.Send(read)

	trans.HasFetchReadReq = true
	trans.FetchReadReqMeta = read.MsgMeta
	next.InflightFetchIndices = append(next.InflightFetchIndices, transIdx)

	next.WriteBufferBuf.Elements = next.WriteBufferBuf.Elements[1:]

	reqMeta := trans.reqMeta()
	tracing.TraceReqInitiate(read, wb.cache.comp,
		tracing.MsgIDAtReceiver(&reqMeta, wb.cache.comp))

	return true
}

func (wb *writeBufferStage) processWriteBufferEvictAndWrite(
	transIdx int,
	trans *transactionState,
) bool {
	if wb.writeBufferFull() {
		return false
	}

	spec := wb.cache.comp.GetSpec()
	next := wb.cache.comp.GetNextState()
	bankNum := bankID(
		trans.BlockSetID, trans.BlockWayID,
		spec.WayAssociativity,
		len(next.WriteBufferToBankBufs),
	)
	bankBuf := &next.WriteBufferToBankBufs[bankNum]

	if !bankBuf.CanPush() {
		return false
	}

	trans.Action = bankWriteHit
	bankBuf.PushTyped(transIdx)

	next.PendingEvictionIndices = append(next.PendingEvictionIndices, transIdx)
	next.WriteBufferBuf.Elements = next.WriteBufferBuf.Elements[1:]

	return true
}

func (wb *writeBufferStage) processWriteBufferFetchAndEvict(
	transIdx int,
	trans *transactionState,
) bool {
	ok := wb.processWriteBufferFlush(transIdx, trans, false)
	if ok {
		trans.Action = writeBufferFetch
		return true
	}

	return false
}

func (wb *writeBufferStage) processWriteBufferFlush(
	transIdx int,
	trans *transactionState,
	popAfterDone bool,
) bool {
	if wb.writeBufferFull() {
		return false
	}

	next := wb.cache.comp.GetNextState()
	next.PendingEvictionIndices = append(next.PendingEvictionIndices, transIdx)

	if popAfterDone {
		next.WriteBufferBuf.Elements = next.WriteBufferBuf.Elements[1:]
	}

	return true
}

func (wb *writeBufferStage) write() bool {
	next := wb.cache.comp.GetNextState()

	if len(next.PendingEvictionIndices) == 0 {
		return false
	}

	transIdx := next.PendingEvictionIndices[0]
	trans := &next.Transactions[transIdx]

	if wb.tooManyInflightEvictions() {
		return false
	}

	if !wb.cache.bottomPort.CanSend() {
		return false
	}

	lowModulePort := wb.cache.findPort(trans.EvictingAddr)
	write := &mem.WriteReq{}
	write.ID = sim.GetIDGenerator().Generate()
	write.Src = wb.cache.bottomPort.AsRemote()
	write.Dst = lowModulePort
	write.PID = trans.EvictingPID
	write.Address = trans.EvictingAddr
	write.Data = trans.EvictingData
	write.DirtyMask = trans.EvictingDirtyMask
	write.TrafficBytes = len(trans.EvictingData) + 12
	write.TrafficClass = "mem.WriteReq"
	wb.cache.bottomPort.Send(write)

	trans.HasEvictionWriteReq = true
	trans.EvictionWriteReqMeta = write.MsgMeta
	next.PendingEvictionIndices = next.PendingEvictionIndices[1:]
	next.InflightEvictionIndices = append(next.InflightEvictionIndices, transIdx)

	reqMeta := trans.reqMeta()
	tracing.TraceReqInitiate(write, wb.cache.comp,
		tracing.MsgIDAtReceiver(&reqMeta, wb.cache.comp))

	return true
}

func (wb *writeBufferStage) processReturnRsp() bool {
	msg := wb.cache.bottomPort.PeekIncoming()
	if msg == nil {
		return false
	}

	switch msg := msg.(type) {
	case *mem.DataReadyRsp:
		return wb.processDataReadyRsp(msg)
	case *mem.WriteDoneRsp:
		return wb.processWriteDoneRsp(msg)
	default:
		panic("unknown msg type")
	}
}

func (wb *writeBufferStage) processDataReadyRsp(
	msg *mem.DataReadyRsp,
) bool {
	spec := wb.cache.comp.GetSpec()
	next := wb.cache.comp.GetNextState()

	transIdx := wb.findInflightFetchIdxByFetchReadReqID(msg.RspTo)
	trans := &next.Transactions[transIdx]
	bankIndex := bankID(
		trans.BlockSetID, trans.BlockWayID,
		spec.WayAssociativity,
		len(next.WriteBufferToBankBufs),
	)
	bankBuf := &next.WriteBufferToBankBufs[bankIndex]

	if !bankBuf.CanPush() {
		return false
	}

	if !trans.HasMSHREntry {
		panic("processDataReadyRsp without MSHR entry")
	}

	mshrIdx := wb.lookupMSHRIndex(trans)
	trans.FetchedData = msg.Data
	trans.Action = bankWriteFetched
	mshrEntry := &next.MSHRState.Entries[mshrIdx]
	mshrEntry.Data = msg.Data
	wb.combineData(mshrIdx)

	// Resolve MSHR transaction pointers before removal
	trans.MSHRData = make([]byte, len(mshrEntry.Data))
	copy(trans.MSHRData, mshrEntry.Data)
	trans.MSHRTransactionIndices = wb.resolveEntryTransactionIndices(mshrEntry)

	cache.MSHRRemove(&next.MSHRState,
		vm.PID(mshrEntry.PID), mshrEntry.Address)

	bankBuf.PushTyped(transIdx)

	wb.removeInflightFetch(transIdx)
	wb.cache.bottomPort.RetrieveIncoming()

	tracing.TraceReqFinalize(&trans.FetchReadReqMeta, wb.cache.comp)

	return true
}

func (wb *writeBufferStage) combineData(mshrIdx int) {
	spec := wb.cache.comp.GetSpec()
	next := wb.cache.comp.GetNextState()
	mshrEntry := &next.MSHRState.Entries[mshrIdx]
	block := &next.DirectoryState.Sets[mshrEntry.BlockSetID].Blocks[mshrEntry.BlockWayID]

	block.DirtyMask = make([]bool, 1<<spec.Log2BlockSize)

	for _, transIdx := range mshrEntry.TransactionIndices {
		if transIdx < 0 || transIdx >= len(next.Transactions) {
			continue
		}

		trans := &next.Transactions[transIdx]
		if trans.HasRead {
			continue
		}

		block.IsDirty = true
		_, offset := getCacheLineID(trans.WriteAddress, spec.Log2BlockSize)

		for i := 0; i < len(trans.WriteData); i++ {
			if trans.WriteDirtyMask == nil || trans.WriteDirtyMask[i] {
				index := offset + uint64(i)
				mshrEntry.Data[index] = trans.WriteData[i]
				block.DirtyMask[index] = true
			}
		}
	}
}

func (wb *writeBufferStage) findInflightFetchIdxByFetchReadReqID(
	id uint64,
) int {
	next := wb.cache.comp.GetNextState()

	for _, tIdx := range next.InflightFetchIndices {
		t := &next.Transactions[tIdx]
		if t.FetchReadReqMeta.ID == id {
			return tIdx
		}
	}

	panic("inflight read not found")
}

func (wb *writeBufferStage) removeInflightFetch(transIdx int) {
	next := wb.cache.comp.GetNextState()

	for i, idx := range next.InflightFetchIndices {
		if idx == transIdx {
			next.InflightFetchIndices = append(
				next.InflightFetchIndices[:i],
				next.InflightFetchIndices[i+1:]...,
			)

			return
		}
	}

	panic("not found")
}

func (wb *writeBufferStage) processWriteDoneRsp(
	msg *mem.WriteDoneRsp,
) bool {
	next := wb.cache.comp.GetNextState()

	for i := len(next.InflightEvictionIndices) - 1; i >= 0; i-- {
		eIdx := next.InflightEvictionIndices[i]
		e := &next.Transactions[eIdx]
		if e.EvictionWriteReqMeta.ID == msg.RspTo {
			next.InflightEvictionIndices = append(
				next.InflightEvictionIndices[:i],
				next.InflightEvictionIndices[i+1:]...,
			)
			wb.cache.bottomPort.RetrieveIncoming()
			tracing.TraceReqFinalize(&e.EvictionWriteReqMeta, wb.cache.comp)

			return true
		}
	}

	panic("write request not found")
}

func (wb *writeBufferStage) writeBufferFull() bool {
	next := wb.cache.comp.GetNextState()
	spec := wb.cache.comp.GetSpec()
	numEntry := len(next.PendingEvictionIndices) + len(next.InflightEvictionIndices)
	return numEntry >= spec.WriteBufferCapacity
}

func (wb *writeBufferStage) tooManyInflightFetches() bool {
	next := wb.cache.comp.GetNextState()
	spec := wb.cache.comp.GetSpec()
	return len(next.InflightFetchIndices) >= spec.MaxInflightFetch
}

func (wb *writeBufferStage) tooManyInflightEvictions() bool {
	next := wb.cache.comp.GetNextState()
	spec := wb.cache.comp.GetSpec()
	return len(next.InflightEvictionIndices) >= spec.MaxInflightEviction
}

func (wb *writeBufferStage) Reset() {
	next := wb.cache.comp.GetNextState()
	next.WriteBufferBuf.Clear()
}

// lookupMSHRIndex finds the current index of the MSHR entry for this
// transaction.  The stored MSHREntryIndex may be stale because MSHRRemove
// shifts entries, so we re-query by PID+Address.
func (wb *writeBufferStage) lookupMSHRIndex(trans *transactionState) int {
	next := wb.cache.comp.GetNextState()
	idx, found := cache.MSHRQuery(&next.MSHRState, trans.FetchPID, trans.FetchAddress)
	if !found {
		panic("lookupMSHRIndex: MSHR entry not found")
	}
	return idx
}

// resolveEntryTransactionIndices collects the transaction indices from
// the MSHR entry's TransactionIndices.
func (wb *writeBufferStage) resolveEntryTransactionIndices(
	entry *cache.MSHREntryState,
) []int {
	next := wb.cache.comp.GetNextState()
	result := make([]int, 0, len(entry.TransactionIndices))
	for _, transIdx := range entry.TransactionIndices {
		if transIdx >= 0 && transIdx < len(next.Transactions) {
			result = append(result, transIdx)
		}
	}
	return result
}
