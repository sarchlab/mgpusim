package writeback

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/akita/v3/tracing"
	"github.com/sarchlab/mgpusim/v3/mem/cache"
)

type flusher struct {
	cache *Cache

	blockToEvict    []*cache.Block
	processingFlush *cache.FlushReq
}

func (f *flusher) Tick(now sim.VTimeInSec) bool {
	if f.processingFlush != nil && f.cache.state == cacheStatePreFlushing {
		return f.processPreFlushing(now)
	}

	madeProgress := false
	if f.processingFlush != nil && f.cache.state == cacheStateFlushing {
		madeProgress = f.finalizeFlushing(now) || madeProgress
		madeProgress = f.processFlush(now) || madeProgress
		return madeProgress
	}

	return f.extractFromPort(now)
}

func (f *flusher) processPreFlushing(now sim.VTimeInSec) bool {
	if f.existInflightTransaction() {
		return false
	}

	f.prepareBlockToFlushList()
	f.cache.state = cacheStateFlushing

	return true
}

func (f *flusher) existInflightTransaction() bool {
	return len(f.cache.inFlightTransactions) > 0
}

func (f *flusher) prepareBlockToFlushList() {
	sets := f.cache.directory.GetSets()
	for _, set := range sets {
		for _, block := range set.Blocks {
			if block.ReadCount > 0 || block.IsLocked {
				panic("all the blocks should be unlocked before flushing")
			}

			if block.IsValid && block.IsDirty {
				f.blockToEvict = append(f.blockToEvict, block)
			}
		}
	}
}

func (f *flusher) processFlush(now sim.VTimeInSec) bool {
	if len(f.blockToEvict) == 0 {
		return false
	}

	block := f.blockToEvict[0]
	bankNum := bankID(
		block,
		f.cache.directory.WayAssociativity(),
		len(f.cache.dirToBankBuffers))
	bankBuf := f.cache.dirToBankBuffers[bankNum]

	if !bankBuf.CanPush() {
		return false
	}

	trans := &transaction{
		flush:             f.processingFlush,
		victim:            block,
		action:            bankEvict,
		evictingAddr:      block.Tag,
		evictingDirtyMask: block.DirtyMask,
	}
	bankBuf.Push(trans)

	f.blockToEvict = f.blockToEvict[1:]

	return true
}

func (f *flusher) extractFromPort(now sim.VTimeInSec) bool {
	item := f.cache.controlPort.Peek()
	if item == nil {
		return false
	}

	switch req := item.(type) {
	case *cache.FlushReq:
		return f.startProcessingFlush(now, req)
	case *cache.RestartReq:
		return f.handleCacheRestart(now, req)
	default:
		log.Panicf("Cannot process request of %s", reflect.TypeOf(req))
	}

	return true
}

func (f *flusher) startProcessingFlush(
	now sim.VTimeInSec,
	req *cache.FlushReq,
) bool {
	f.processingFlush = req
	if req.DiscardInflight {
		f.cache.discardInflightTransactions(now)
	}

	f.cache.state = cacheStatePreFlushing
	f.cache.controlPort.Retrieve(now)

	tracing.TraceReqReceive(req, f.cache)

	return true
}

func (f *flusher) handleCacheRestart(
	now sim.VTimeInSec,
	req *cache.RestartReq,
) bool {
	if !f.cache.controlPortSender.CanSend(1) {
		return false
	}

	clearPort(f.cache.topPort, now)
	clearPort(f.cache.bottomPort, now)

	f.cache.state = cacheStateRunning

	rsp := cache.RestartRspBuilder{}.
		WithSendTime(now).
		WithSrc(f.cache.controlPort).
		WithDst(req.Src).
		WithRspTo(req.ID).
		Build()
	f.cache.controlPortSender.Send(rsp)

	f.cache.controlPort.Retrieve(now)

	return true
}

func (f *flusher) finalizeFlushing(now sim.VTimeInSec) bool {
	if len(f.blockToEvict) > 0 {
		return false
	}

	if !f.flushCompleted() {
		return false
	}

	if !f.cache.controlPortSender.CanSend(1) {
		return false
	}

	rsp := cache.FlushRspBuilder{}.
		WithSendTime(now).
		WithSrc(f.cache.controlPort).
		WithDst(f.processingFlush.Src).
		WithRspTo(f.processingFlush.ID).
		Build()
	f.cache.controlPortSender.Send(rsp)

	f.cache.mshr.Reset()
	f.cache.directory.Reset()

	if f.processingFlush.PauseAfterFlushing {
		f.cache.state = cacheStatePaused
	} else {
		f.cache.state = cacheStateRunning
	}

	tracing.TraceReqComplete(f.processingFlush, f.cache)
	f.processingFlush = nil

	return true
}

func (f *flusher) flushCompleted() bool {
	for _, b := range f.cache.dirToBankBuffers {
		if b.Size() > 0 {
			return false
		}
	}

	for _, b := range f.cache.bankStages {
		if b.inflightTransCount > 0 {
			return false
		}
	}

	if f.cache.writeBufferBuffer.Size() > 0 {
		return false
	}

	if len(f.cache.writeBuffer.inflightFetch) > 0 ||
		len(f.cache.writeBuffer.inflightEviction) > 0 ||
		len(f.cache.writeBuffer.pendingEvictions) > 0 {
		return false
	}

	return true
}
