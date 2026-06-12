package writethroughcache

import (
	"github.com/sarchlab/akita/v5/mem/cache"
	"github.com/sarchlab/akita/v5/tracing"
)

// needsDualCompletion returns true if the current write policy requires both
// bank and bottom-write completion before a write-hit transaction is done.
func needsDualCompletion(policyType string) bool {
	switch policyType {
	case "write-around", "write-through":
		return true
	case "write-evict":
		return false
	default:
		panic("unknown write policy type: " + policyType)
	}
}

// handleWriteHit dispatches to the correct write-hit handler based on
// the Spec.WritePolicyType.
func (d *directory) handleWriteHit(
	trans *transactionState,
	setID, wayID int,
	postCoalesceIdx int,
) bool {
	spec := d.cache.GetSpec()
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
	spec := d.cache.GetSpec()
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
	next := d.cache.comp.GetNextState()
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
	spec := d.cache.GetSpec()
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

	tracing.AddTaskStep(trans.ID, d.cache.comp, "write-hit")

	dirPostBuf := &next.DirPostBuf
	dirPostBuf.Elements = dirPostBuf.Elements[1:]

	return true
}

func (d *directory) writearoundWriteMiss(
	trans *transactionState,
	postCoalesceIdx int,
) bool {
	if ok := d.writeBottom(trans); ok {
		tracing.AddTaskStep(trans.ID, d.cache.comp, "write-miss")

		next := d.cache.comp.GetNextState()
		dirPostBuf := &next.DirPostBuf
		dirPostBuf.Elements = dirPostBuf.Elements[1:]

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
	next := d.cache.comp.GetNextState()
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

	tracing.AddTaskStep(trans.ID, d.cache.comp, "write-hit")

	dirPostBuf := &next.DirPostBuf
	dirPostBuf.Elements = dirPostBuf.Elements[1:]

	return true
}

func (d *directory) writeevictWriteMiss(
	trans *transactionState,
	postCoalesceIdx int,
) bool {
	if ok := d.writeBottom(trans); ok {
		tracing.AddTaskStep(trans.ID, d.cache.comp, "write-miss")

		next := d.cache.comp.GetNextState()
		dirPostBuf := &next.DirPostBuf
		dirPostBuf.Elements = dirPostBuf.Elements[1:]

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
	next := d.cache.comp.GetNextState()
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
	spec := d.cache.GetSpec()
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
	dirPostBuf.Elements = dirPostBuf.Elements[1:]

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
		tracing.AddTaskStep(trans.ID, d.cache.comp, "write-miss")
	}

	return ok
}

func (d *directory) writethroughIsPartialWrite(
	trans *transactionState,
) bool {
	spec := d.cache.GetSpec()
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
	spec := d.cache.GetSpec()
	blockSize := uint64(1 << spec.Log2BlockSize)
	cacheLineID := addr / blockSize * blockSize
	trans.FetchAndWrite = true

	next := d.cache.comp.GetNextState()

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
	dirPostBuf.Elements = dirPostBuf.Elements[1:]
	tracing.AddTaskStep(trans.ID, d.cache.comp, "write-miss")

	return true
}

func (d *directory) writethroughFullLineWriteMiss(
	trans *transactionState,
	postCoalesceIdx int,
) bool {
	addr := trans.WriteAddress
	spec := d.cache.GetSpec()
	blockSize := uint64(1 << spec.Log2BlockSize)
	cacheLineID := addr / blockSize * blockSize
	next := d.cache.comp.GetNextState()

	victimSetID, victimWayID := cache.DirectoryFindVictim(
		&next.DirectoryState, spec.NumSets, int(blockSize), cacheLineID)

	_ = next // suppress unused warning

	return d.writethroughWriteHit(trans, victimSetID, victimWayID, postCoalesceIdx)
}
