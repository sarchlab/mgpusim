package l1v

import (
	"log"
	"reflect"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/mem/cache"
)

type controlStage struct {
	ctrlPort     akita.Port
	transactions *[]*transaction
	directory    cache.Directory
	cache        *Cache
	coalescer    *coalescer
	bankStages   []*bankStage

	currFlushReq *cache.FlushReq
}

func (s *controlStage) Tick(now akita.VTimeInSec) bool {

	req := s.ctrlPort.Peek()
	if req == nil {
		return false
	}

	req = s.ctrlPort.Retrieve(now)

	switch req := req.(type) {
	case *cache.FlushReq:
		s.currFlushReq = req
		return s.doCacheFlush(now, req)
	case *cache.RestartReq:
		return s.doCacheRestart(now, req)
	default:
		log.Panicf("cannot handle request of type %s ",
			reflect.TypeOf(req))

	}

	return true

}

func (s *controlStage) doCacheFlush(now akita.VTimeInSec, req *cache.FlushReq) bool {
	if req.DiscardInflight == false {
		if len(*s.transactions) > 0 {
			return false
		}
		s.directory.Reset()
		s.currFlushReq = nil
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

		return true

	} else {

		s.cache.transactions = nil

		//log.Printf("POST C TRANSACTIONS %d \n", len(s.cache.postCoalesceTransactions))
		s.cache.postCoalesceTransactions = nil

		//Bank Stage component reset
		for i := 0; i < len(s.bankStages); i++ {
			s.bankStages[i].currTrans = nil
			for {
				out := s.cache.bankBufs[i].Pop()
				if out == nil {
					break
				}
			}
			/*for {
				out := s.cache.bankStages[i].inBuf.Pop()
				if out == nil {
					break
				}
			}*/
		}

		//Bottom Parser Stage  Reset
		/*for i := 0; i < len(s.cache.parseBottomStage.bankBufs); i++ {
			for {
				out := s.cache.parseBottomStage.bankBufs[i].Pop()
				if out == nil {
					break
				}
			}
		}*/
		//s.cache.parseBottomStage.mshr.Reset()

		//Coalescer Stage Reset
		s.coalescer.toCoalesce = nil
		/*for {
			out := s.cache.coalesceStage.dirBuf.Pop()
			if out == nil {
				break
			}
		}*/

		//Directory component reset
		/*s.cache.directoryStage.mshr.Reset()
		for i := 0; i < len(s.cache.directoryStage.bankBufs); i++ {
			for {
				out := s.cache.directoryStage.bankBufs[i].Pop()
				if out == nil {
					break
				}
			}

		}
		for {
			out := s.cache.directoryStage.inBuf.Pop()
			if out == nil {
				break
			}
		}*/

		for {
			out := s.cache.dirBuf.Pop()
			if out == nil {
				break
			}
		}

		s.directory.Reset()
		s.cache.mshr.Reset()

		s.cache.isPaused = true
		s.currFlushReq = nil

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

		return true
	}
}

func (s *controlStage) doCacheRestart(now akita.VTimeInSec, req *cache.RestartReq) bool {
	s.cache.isPaused = false

	//log.Printf("Receiving cache restart")

	for s.cache.TopPort.Retrieve(now) != nil {
		s.cache.TopPort.Retrieve(now)
	}

	for s.cache.BottomPort.Retrieve(now) != nil {
		s.cache.BottomPort.Retrieve(now)
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
