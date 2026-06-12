package writethroughcache

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

type intake struct {
	cache *pipelineMW
}

func (s *intake) Tick() bool {
	msg := s.cache.topPort.PeekIncoming()
	if msg == nil {
		return false
	}

	next := s.cache.comp.GetNextState()
	if s.countActive(next) >= s.cache.GetSpec().MaxNumConcurrentTrans {
		return false
	}

	dirBuf := &next.DirBuf
	if !dirBuf.CanPush() {
		return false
	}

	transIdx := s.createTransaction(msg)

	dirBuf.PushTyped(transIdx)

	s.cache.topPort.RetrieveIncoming()

	tracing.TraceReqReceive(msg, s.cache.comp)

	return true
}

func (s *intake) countActive(state *State) int {
	count := 0
	for i := range state.Transactions {
		if !state.Transactions[i].Removed {
			count++
		}
	}
	return count
}

func (s *intake) createTransaction(msg sim.Msg) int {
	next := s.cache.comp.GetNextState()

	var t transactionState
	switch m := msg.(type) {
	case *mem.ReadReq:
		t = transactionState{
			ID:                 sim.GetIDGenerator().Generate(),
			HasRead:            true,
			ReadMeta:           m.MsgMeta,
			ReadAddress:        m.Address,
			ReadAccessByteSize: m.AccessByteSize,
			ReadPID:            m.PID,
		}

		tracing.StartTaskWithSpecificLocation(t.ID,
			tracing.MsgIDAtReceiver(m, s.cache.comp),
			s.cache.comp, "cache_transaction", "read",
			s.cache.comp.Name()+".Local",
			nil)
	case *mem.WriteReq:
		t = transactionState{
			ID:             sim.GetIDGenerator().Generate(),
			HasWrite:       true,
			WriteMeta:      m.MsgMeta,
			WriteAddress:   m.Address,
			WriteData:      m.Data,
			WriteDirtyMask: m.DirtyMask,
			WritePID:       m.PID,
		}

		if t.WriteDirtyMask == nil {
			t.WriteDirtyMask = make([]bool, len(m.Data))
			for i := range t.WriteDirtyMask {
				t.WriteDirtyMask[i] = true
			}
		}

		tracing.StartTaskWithSpecificLocation(t.ID,
			tracing.MsgIDAtReceiver(m, s.cache.comp),
			s.cache.comp, "cache_transaction", "write",
			s.cache.comp.Name()+".Local",
			nil)
	default:
		log.Panicf("cannot process request of type %s\n",
			reflect.TypeOf(msg))
		return -1
	}

	next.Transactions = append(next.Transactions, t)
	return len(next.Transactions) - 1
}
