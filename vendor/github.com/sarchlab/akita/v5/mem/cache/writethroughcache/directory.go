package writethroughcache

import (
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/mem/memprotocol"

	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

type directory struct {
	cache *pipelineMW
}

func (d *directory) Tick() (madeProgress bool) {
	spec := d.cache.comp.Spec()
	next := &d.cache.comp.State
	dirBuf := &next.DirBuf
	dirPipeline := &next.DirPipeline
	dirPostBuf := &next.DirPostBuf

	// Accept from dirBuf into pipeline
	for i := 0; i < spec.NumReqPerCycle; i++ {
		if !dirPipeline.CanAccept() {
			break
		}

		if dirBuf.Size() == 0 {
			break
		}

		transIdx := dirBuf.Pop()
		dirPipeline.Accept(transIdx)

		madeProgress = true
	}

	// Tick pipeline
	madeProgress = dirPipeline.Tick(dirPostBuf) || madeProgress

	// Process items from post-pipeline buffer
	for i := 0; i < spec.NumReqPerCycle; i++ {
		if dirPostBuf.Size() == 0 {
			break
		}

		transIdx := dirPostBuf.Peek()
		trans := &next.Transactions[transIdx]

		var processed bool
		if trans.HasRead {
			processed = d.processRead(trans, transIdx)
		} else {
			processed = d.processWrite(trans, transIdx)
		}

		if !processed {
			break
		}

		madeProgress = true
	}

	return madeProgress
}

func (d *directory) processRead(trans *transactionState, transIdx int) bool {
	addr := trans.ReadAddress
	pid := trans.ReadPID
	spec := d.cache.comp.Spec()
	blockSize := uint64(1 << spec.Log2BlockSize)
	cacheLineID := addr / blockSize * blockSize
	next := &d.cache.comp.State

	entryIdx, mshrFound := cache.MSHRQuery(
		&next.MSHRState, pid, cacheLineID)
	if mshrFound {
		return d.processMSHRHit(trans, entryIdx, transIdx)
	}

	setID, wayID, found := cache.DirectoryLookup(
		&next.DirectoryState, spec.NumSets, int(blockSize),
		pid, cacheLineID)
	if found && next.DirectoryState.Sets[setID].Blocks[wayID].IsValid {
		return d.processReadHit(trans, setID, wayID, transIdx)
	}

	return d.processReadMiss(trans, transIdx)
}

func (d *directory) processMSHRHit(
	trans *transactionState,
	entryIdx int,
	transIdx int,
) bool {
	next := &d.cache.comp.State

	next.MSHRState.Entries[entryIdx].TransactionIndices =
		append(next.MSHRState.Entries[entryIdx].TransactionIndices,
			transIdx)

	if trans.HasRead {
		tracing.AddTaskTag(d.cache.comp, tracing.TaskTag{
			TaskID: trans.ID,
			What:   "read-mshr-hit",
		})
	} else {
		tracing.AddTaskTag(d.cache.comp, tracing.TaskTag{
			TaskID: trans.ID,
			What:   "write-mshr-hit",
		})
	}

	dirPostBuf := &next.DirPostBuf
	dirPostBuf.Pop()

	return true
}

func (d *directory) processReadHit(
	trans *transactionState,
	setID, wayID int,
	transIdx int,
) bool {
	next := &d.cache.comp.State
	block := &next.DirectoryState.Sets[setID].Blocks[wayID]
	if block.IsLocked {
		return false
	}

	bankBuf := d.getBankBuf(setID, wayID)
	if !bankBuf.CanPush() {
		return false
	}

	trans.BlockSetID = setID
	trans.BlockWayID = wayID
	trans.HasBlock = true
	trans.BankAction = bankActionReadHit

	nextBlock := &next.DirectoryState.Sets[setID].Blocks[wayID]
	nextBlock.ReadCount++
	cache.DirectoryVisit(&next.DirectoryState, setID, wayID)

	bankBuf.PushTyped(transIdx)

	dirPostBuf := &next.DirPostBuf
	dirPostBuf.Pop()
	tracing.AddTaskTag(d.cache.comp, tracing.TaskTag{
		TaskID: trans.ID,
		What:   "read-hit",
	})

	return true
}

func (d *directory) processReadMiss(trans *transactionState, transIdx int) bool {
	addr := trans.ReadAddress
	spec := d.cache.comp.Spec()
	blockSize := uint64(1 << spec.Log2BlockSize)
	cacheLineID := addr / blockSize * blockSize
	next := &d.cache.comp.State

	victimSetID, victimWayID := cache.DirectoryFindVictim(
		&next.DirectoryState, spec.NumSets, int(blockSize), cacheLineID)
	victim := &next.DirectoryState.Sets[victimSetID].Blocks[victimWayID]
	if victim.IsLocked || victim.ReadCount > 0 {
		return false
	}

	if cache.MSHRIsFull(&next.MSHRState, spec.NumMSHREntry) {
		return false
	}

	if !d.fetchFromBottom(trans, victimSetID, victimWayID, transIdx) {
		return false
	}

	dirPostBuf := &next.DirPostBuf
	dirPostBuf.Pop()
	tracing.AddTaskTag(d.cache.comp, tracing.TaskTag{
		TaskID: trans.ID,
		What:   "read-miss",
	})

	return true
}

func (d *directory) processWrite(trans *transactionState, transIdx int) bool {
	addr := trans.WriteAddress
	pid := trans.WritePID
	spec := d.cache.comp.Spec()
	blockSize := uint64(1 << spec.Log2BlockSize)
	cacheLineID := addr / blockSize * blockSize
	next := &d.cache.comp.State

	entryIdx, mshrFound := cache.MSHRQuery(
		&next.MSHRState, pid, cacheLineID)
	if mshrFound {
		ok := d.writeBottom(trans)
		if ok {
			// The coalesced write's data is merged into the fetcher's
			// fill data (see mergeMSHRData) and reaches storage only when
			// the fetcher's bankActionWriteFetched stage runs. Record the
			// fetcher so writeTransIsReady can gate completion on that.
			entry := &next.MSHRState.Entries[entryIdx]
			trans.WaitForMSHRFill = true
			trans.MSHRFillFetcherIdx = entry.TransactionIndices[0]

			return d.processMSHRHit(trans, entryIdx, transIdx)
		}

		return false
	}

	setID, wayID, found := cache.DirectoryLookup(
		&next.DirectoryState, spec.NumSets, int(blockSize),
		pid, cacheLineID)
	if found && next.DirectoryState.Sets[setID].Blocks[wayID].IsValid {
		return d.handleWriteHit(trans, setID, wayID, transIdx)
	}

	return d.handleWriteMiss(trans, transIdx)
}

func (d *directory) writeBottom(trans *transactionState) bool {
	addr := trans.WriteAddress
	spec := d.cache.comp.Spec()
	blockSize := uint64(1 << spec.Log2BlockSize)
	cacheLineID := addr / blockSize * blockSize

	writeToBottom := memprotocol.WriteReq{}
	writeToBottom.ID = timing.GetIDGenerator().Generate()
	writeToBottom.Src = d.cache.bottomPort().AsRemote()
	// Route by cache-line ID so the write-through write and the
	// corresponding read-fill always target the same lower-memory port,
	// preserving per-line ordering.
	writeToBottom.Dst = d.cache.findPort(cacheLineID)
	writeToBottom.Address = addr
	writeToBottom.PID = trans.WritePID
	writeToBottom.Data = trans.WriteData
	writeToBottom.DirtyMask = trans.WriteDirtyMask
	writeToBottom.TrafficBytes = len(trans.WriteData) + 12
	writeToBottom.TrafficClass = "req"

	if !d.cache.bottomPort().CanSend() {
		return false
	}

	d.cache.bottomPort().Send(writeToBottom)

	trans.HasWriteToBottom = true
	trans.WriteToBottomMeta = writeToBottom.MsgMeta
	trans.WriteToBottomPID = trans.WritePID
	trans.WriteToBottomData = trans.WriteData
	trans.WriteToBottomDirtyMask = trans.WriteDirtyMask

	tracing.TraceReqInitiate(d.cache.comp, writeToBottom, trans.ID)

	return true
}

func (d *directory) fetchFromBottom(
	trans *transactionState,
	victimSetID, victimWayID int,
	transIdx int,
) bool {
	addr := trans.Address()
	pid := trans.PID()
	spec := d.cache.comp.Spec()
	blockSize := uint64(1 << spec.Log2BlockSize)
	cacheLineID := addr / blockSize * blockSize
	next := &d.cache.comp.State

	bottomModule := d.cache.findPort(cacheLineID)
	readToBottom := memprotocol.ReadReq{
		Address:        cacheLineID,
		PID:            pid,
		AccessByteSize: blockSize,
	}
	readToBottom.ID = timing.GetIDGenerator().Generate()
	readToBottom.Src = d.cache.bottomPort().AsRemote()
	readToBottom.Dst = bottomModule
	readToBottom.TrafficBytes, readToBottom.TrafficClass = 12, "req"

	if !d.cache.bottomPort().CanSend() {
		return false
	}

	d.cache.bottomPort().Send(readToBottom)

	tracing.TraceReqInitiate(d.cache.comp, readToBottom, trans.ID)

	trans.HasReadToBottom = true
	trans.ReadToBottomMeta = readToBottom.MsgMeta
	trans.ReadToBottomPID = pid
	trans.BlockSetID = victimSetID
	trans.BlockWayID = victimWayID
	trans.HasBlock = true

	entryIdx := cache.MSHRAdd(
		&next.MSHRState, spec.NumMSHREntry, pid, cacheLineID)
	entry := &next.MSHRState.Entries[entryIdx]
	entry.TransactionIndices = append(entry.TransactionIndices,
		transIdx)
	entry.HasReadReq = true
	entry.ReadReq = readToBottom.MsgMeta
	entry.HasBlock = true
	entry.BlockSetID = victimSetID
	entry.BlockWayID = victimWayID

	victim := &next.DirectoryState.Sets[victimSetID].Blocks[victimWayID]
	victim.Tag = cacheLineID
	victim.PID = uint32(pid)
	victim.IsValid = true
	victim.IsLocked = true
	cache.DirectoryVisit(&next.DirectoryState, victimSetID, victimWayID)

	return true
}

func (d *directory) getBankBuf(setID, wayID int) *queueing.Buffer[int] {
	next := &d.cache.comp.State
	numWaysPerSet := d.cache.comp.Spec().WayAssociativity
	blockID := setID*numWaysPerSet + wayID
	bankID := blockID % len(next.BankBufs)

	return &next.BankBufs[bankID]
}
