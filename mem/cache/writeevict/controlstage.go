package writeevict

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/mem/cache"
)

type controlStage struct {
	ctrlPort     sim.Port
	transactions *[]*transaction
	directory    cache.Directory
	cache        *Cache
	coalescer    *coalescer
	bankStages   []*bankStage

	currFlushReq *cache.FlushReq
}

func (s *controlStage) Tick(now sim.VTimeInSec) bool {
	madeProgress := false

	madeProgress = s.processNewRequest(now) || madeProgress
	madeProgress = s.processCurrentFlush(now) || madeProgress

	return madeProgress
}

func (s *controlStage) processCurrentFlush(now sim.VTimeInSec) bool {
	if s.currFlushReq == nil {
		return false
	}

	if s.shouldWaitForInFlightTransactions() {
		return false
	}

	rsp := cache.FlushRspBuilder{}.
		WithSendTime(now).
		WithSrc(s.ctrlPort).
		WithDst(s.currFlushReq.Src).
		WithRspTo(s.currFlushReq.ID).
		Build()
	err := s.ctrlPort.Send(rsp)
	if err != nil {
		return false
	}

	s.hardResetCache(now)
	s.currFlushReq = nil

	return true
}

func (s *controlStage) hardResetCache(now sim.VTimeInSec) {
	s.flushPort(s.cache.topPort, now)
	s.flushPort(s.cache.bottomPort, now)
	s.flushBuffer(s.cache.dirBuf)
	for _, bankBuf := range s.cache.bankBufs {
		s.flushBuffer(bankBuf)
	}

	s.directory.Reset()
	s.cache.mshr.Reset()
	s.cache.coalesceStage.Reset()
	for _, bankStage := range s.cache.bankStages {
		bankStage.Reset()
	}

	s.cache.transactions = nil
	s.cache.postCoalesceTransactions = nil

	if s.currFlushReq.PauseAfterFlushing {
		s.cache.isPaused = true
	}
}

func (s *controlStage) flushPort(port sim.Port, now sim.VTimeInSec) {
	for port.Peek() != nil {
		port.Retrieve(now)
	}
}

func (s *controlStage) flushBuffer(buffer sim.Buffer) {
	for buffer.Pop() != nil {
	}
}

func (s *controlStage) processNewRequest(now sim.VTimeInSec) bool {
	req := s.ctrlPort.Peek()
	if req == nil {
		return false
	}

	switch req := req.(type) {
	case *cache.FlushReq:
		return s.startCacheFlush(now, req)
	case *cache.RestartReq:
		return s.doCacheRestart(now, req)
	default:
		log.Panicf("cannot handle request of type %s ",
			reflect.TypeOf(req))
	}
	panic("never")
}

func (s *controlStage) startCacheFlush(
	now sim.VTimeInSec,
	req *cache.FlushReq,
) bool {
	if s.currFlushReq != nil {
		return false
	}

	s.currFlushReq = req
	s.ctrlPort.Retrieve(now)

	return true
}

func (s *controlStage) doCacheRestart(now sim.VTimeInSec, req *cache.RestartReq) bool {
	s.cache.isPaused = false

	s.ctrlPort.Retrieve(now)

	for s.cache.topPort.Peek() != nil {
		s.cache.topPort.Retrieve(now)
	}

	for s.cache.bottomPort.Peek() != nil {
		s.cache.bottomPort.Retrieve(now)
	}

	rsp := cache.RestartRspBuilder{}.
		WithSendTime(now).
		WithSrc(s.ctrlPort).
		WithDst(req.Src).
		Build()

	err := s.ctrlPort.Send(rsp)
	if err != nil {
		log.Panic("Unable to send restart rsp")
	}

	return true
}

func (s *controlStage) shouldWaitForInFlightTransactions() bool {
	if s.currFlushReq.DiscardInflight == false {
		if len(s.cache.transactions) != 0 {
			return true
		}
	}
	return false
}
