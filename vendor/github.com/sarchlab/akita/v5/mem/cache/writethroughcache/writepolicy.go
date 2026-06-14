package writethroughcache

import (
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/tracing"
)

// writeTransIsReady reports whether a write transaction has met every
// completion dependency it has (bank pipeline work, fetch-and-write fill,
// lower-memory ack, MSHR fill) and can be returned to the requester.
//
// The result depends only on per-transaction flags so it works uniformly
// across write-through, write-around, and write-evict — including the
// MSHR-coalesced case where the write never visits the bank stage but
// still depends on the fetcher's merged-line write reaching storage.
func writeTransIsReady(trans *transactionState) bool {
	if trans.HasWriteToBottom && !trans.BottomWriteDone {
		return false
	}

	needsBank := trans.FetchAndWrite ||
		trans.BankAction == bankActionWrite ||
		trans.BankAction == bankActionWriteFetched
	if needsBank && !trans.BankDone {
		return false
	}

	if trans.WaitForMSHRFill && !trans.MSHRFillDone {
		return false
	}

	return true
}

// handleWriteHit dispatches to the correct write-hit handler based on
// the Spec.WritePolicyType.
func (d *directory) handleWriteHit(
	trans *transactionState,
	setID, wayID int,
	postCoalesceIdx int,
) bool {
	spec := d.cache.comp.Spec()
	switch spec.WritePolicyType {
	case "write-around":
		return d.writearoundWriteHit(trans, setID, wayID, postCoalesceIdx)
	case "write-evict":
		return d.writeevictWriteHit(trans, setID, wayID, postCoalesceIdx)
	case "write-through":
		return d.writethroughWriteHit(trans, setID, wayID, postCoalesceIdx)
	default:
		panic("unknown write policy type: " + spec.WritePolicyType)
	}
}

// handleWriteMiss dispatches to the correct write-miss handler based on
// the Spec.WritePolicyType.
func (d *directory) handleWriteMiss(
	trans *transactionState,
	postCoalesceIdx int,
) bool {
	spec := d.cache.comp.Spec()
	switch spec.WritePolicyType {
	case "write-around":
		return d.writearoundWriteMiss(trans, postCoalesceIdx)
	case "write-evict":
		return d.writeevictWriteMiss(trans, postCoalesceIdx)
	case "write-through":
		return d.writethroughWriteMiss(trans, postCoalesceIdx)
	default:
		panic("unknown write policy type: " + spec.WritePolicyType)
	}
}

// --- write-around policy ---

func (d *directory) writearoundWriteHit(
	trans *transactionState,
	setID, wayID int,
	postCoalesceIdx int,
) bool {
	next := &d.cache.comp.State
	block := &next.DirectoryState.Sets[setID].Blocks[wayID]
	if block.IsLocked || block.ReadCount > 0 {
		return false
	}

	bankBuf := d.getBankBuf(setID, wayID)
	if !bankBuf.CanPush() {
		return false
	}

	if !trans.HasWriteToBottom {
		ok := d.writeBottom(trans)
		if !ok {
			return false
		}
	}

	addr := trans.WriteAddress
	spec := d.cache.comp.Spec()
	blockSize := uint64(1 << spec.Log2BlockSize)
	cacheLineID := addr / blockSize * blockSize

	nextBlock := &next.DirectoryState.Sets[setID].Blocks[wayID]
	nextBlock.IsLocked = true
	nextBlock.IsValid = true
	nextBlock.Tag = cacheLineID
	cache.DirectoryVisit(&next.DirectoryState, setID, wayID)

	trans.BankAction = bankActionWrite
	trans.BlockSetID = setID
	trans.BlockWayID = wayID
	trans.HasBlock = true

	bankBuf.PushTyped(postCoalesceIdx)

	tracing.AddTaskTag(d.cache.comp, tracing.TaskTag{
		TaskID: trans.ID,
		What:   "write-hit",
	})

	dirPostBuf := &next.DirPostBuf
	dirPostBuf.Pop()

	return true
}

func (d *directory) writearoundWriteMiss(
	trans *transactionState,
	postCoalesceIdx int,
) bool {
	if ok := d.writeBottom(trans); ok {
		tracing.AddTaskTag(d.cache.comp, tracing.TaskTag{
			TaskID: trans.ID,
			What:   "write-miss",
		})

		next := &d.cache.comp.State
		dirPostBuf := &next.DirPostBuf
		dirPostBuf.Pop()

		return true
	}

	return false
}

// --- write-evict policy ---

func (d *directory) writeevictWriteHit(
	trans *transactionState,
	setID, wayID int,
	postCoalesceIdx int,
) bool {
	next := &d.cache.comp.State
	block := &next.DirectoryState.Sets[setID].Blocks[wayID]
	if block.IsLocked || block.ReadCount > 0 {
		return false
	}

	bankBuf := d.getBankBuf(setID, wayID)
	if !bankBuf.CanPush() {
		return false
	}

	if !trans.HasWriteToBottom {
		ok := d.writeBottom(trans)
		if !ok {
			return false
		}
	}

	nextBlock := &next.DirectoryState.Sets[setID].Blocks[wayID]
	nextBlock.IsValid = false

	tracing.AddTaskTag(d.cache.comp, tracing.TaskTag{
		TaskID: trans.ID,
		What:   "write-hit",
	})

	dirPostBuf := &next.DirPostBuf
	dirPostBuf.Pop()

	return true
}

func (d *directory) writeevictWriteMiss(
	trans *transactionState,
	postCoalesceIdx int,
) bool {
	if ok := d.writeBottom(trans); ok {
		tracing.AddTaskTag(d.cache.comp, tracing.TaskTag{
			TaskID: trans.ID,
			What:   "write-miss",
		})

		next := &d.cache.comp.State
		dirPostBuf := &next.DirPostBuf
		dirPostBuf.Pop()

		return true
	}

	return false
}

// --- write-through policy ---

func (d *directory) writethroughWriteHit(
	trans *transactionState,
	setID, wayID int,
	postCoalesceIdx int,
) bool {
	next := &d.cache.comp.State
	block := &next.DirectoryState.Sets[setID].Blocks[wayID]
	if block.IsLocked || block.ReadCount > 0 {
		return false
	}

	bankBuf := d.getBankBuf(setID, wayID)
	if !bankBuf.CanPush() {
		return false
	}

	if !trans.HasWriteToBottom {
		ok := d.writeBottom(trans)
		if !ok {
			return false
		}
	}

	addr := trans.WriteAddress
	spec := d.cache.comp.Spec()
	blockSize := uint64(1 << spec.Log2BlockSize)
	cacheLineID := addr / blockSize * blockSize

	nextBlock := &next.DirectoryState.Sets[setID].Blocks[wayID]
	nextBlock.IsLocked = true
	nextBlock.IsValid = true
	nextBlock.Tag = cacheLineID
	cache.DirectoryVisit(&next.DirectoryState, setID, wayID)

	trans.BankAction = bankActionWrite
	trans.BlockSetID = setID
	trans.BlockWayID = wayID
	trans.HasBlock = true

	bankBuf.PushTyped(postCoalesceIdx)

	dirPostBuf := &next.DirPostBuf
	dirPostBuf.Pop()

	return true
}

func (d *directory) writethroughWriteMiss(
	trans *transactionState,
	postCoalesceIdx int,
) bool {
	if d.writethroughIsPartialWrite(trans) {
		return d.writethroughPartialWriteMiss(trans, postCoalesceIdx)
	}

	ok := d.writethroughFullLineWriteMiss(trans, postCoalesceIdx)
	if ok {
		tracing.AddTaskTag(d.cache.comp, tracing.TaskTag{
			TaskID: trans.ID,
			What:   "write-miss",
		})
	}

	return ok
}

func (d *directory) writethroughIsPartialWrite(
	trans *transactionState,
) bool {
	spec := d.cache.comp.Spec()
	if len(trans.WriteData) < (1 << spec.Log2BlockSize) {
		return true
	}

	if trans.WriteDirtyMask != nil {
		for _, byteDirty := range trans.WriteDirtyMask {
			if !byteDirty {
				return true
			}
		}
	}

	return false
}

func (d *directory) writethroughPartialWriteMiss(
	trans *transactionState,
	postCoalesceIdx int,
) bool {
	addr := trans.WriteAddress
	spec := d.cache.comp.Spec()
	blockSize := uint64(1 << spec.Log2BlockSize)
	cacheLineID := addr / blockSize * blockSize
	trans.FetchAndWrite = true

	next := &d.cache.comp.State

	if cache.MSHRIsFull(&next.MSHRState, spec.NumMSHREntry) {
		return false
	}

	victimSetID, victimWayID := cache.DirectoryFindVictim(
		&next.DirectoryState, spec.NumSets, int(blockSize), cacheLineID)
	victim := &next.DirectoryState.Sets[victimSetID].Blocks[victimWayID]
	if victim.ReadCount > 0 || victim.IsLocked {
		return false
	}

	sentThisCycle := false

	if !trans.HasWriteToBottom {
		ok := d.writeBottom(trans)
		if !ok {
			return false
		}

		sentThisCycle = true
	}

	ok := d.fetchFromBottom(trans, victimSetID, victimWayID, postCoalesceIdx)
	if !ok {
		return sentThisCycle
	}

	dirPostBuf := &next.DirPostBuf
	dirPostBuf.Pop()
	tracing.AddTaskTag(d.cache.comp, tracing.TaskTag{
		TaskID: trans.ID,
		What:   "write-miss",
	})

	return true
}

func (d *directory) writethroughFullLineWriteMiss(
	trans *transactionState,
	postCoalesceIdx int,
) bool {
	addr := trans.WriteAddress
	spec := d.cache.comp.Spec()
	blockSize := uint64(1 << spec.Log2BlockSize)
	cacheLineID := addr / blockSize * blockSize
	next := &d.cache.comp.State

	victimSetID, victimWayID := cache.DirectoryFindVictim(
		&next.DirectoryState, spec.NumSets, int(blockSize), cacheLineID)

	_ = next // suppress unused warning

	return d.writethroughWriteHit(trans, victimSetID, victimWayID, postCoalesceIdx)
}
