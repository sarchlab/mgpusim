package simplebankedmemory

import (
	"log"

	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/modeling"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/tracing"
)

type dispatchMW struct {
	comp *modeling.Component[Spec, State, Resources]
}

func (m *dispatchMW) topPort() messaging.Port {
	return m.comp.GetPortByName("Top")
}

func (m *dispatchMW) Tick() bool {
	if m.comp.State.ControlState != memcontrolprotocol.StateEnabled {
		return false
	}
	return m.dispatchFromTopPort()
}

func (m *dispatchMW) dispatchFromTopPort() bool {
	madeProgress := false
	spec := m.comp.Spec()
	next := &m.comp.State

	for {
		msgI := m.topPort().PeekIncoming()
		if msgI == nil {
			break
		}

		msg, ok := msgI.(memprotocol.AccessReq)
		if !ok {
			log.Panicf("simplebankedmemory: unsupported message type %T", msgI)
		}

		if spec.NumBanks == 0 {
			log.Panic("simplebankedmemory: no banks configured")
		}

		bankID := selectBank(spec, bankSelectionAddress(spec, msg.GetAddress()))
		if bankID < 0 || bankID >= spec.NumBanks {
			log.Panicf("simplebankedmemory: bank selector returned %d", bankID)
		}

		if !pipelineCanAccept(next.Banks[bankID], spec) {
			break
		}

		m.topPort().RetrieveIncoming()
		tracing.TraceReqReceive(m.comp, msg)

		item := m.msgToItem(msg)
		pipelineAccept(&next.Banks[bankID], spec, item)
		madeProgress = true
	}

	return madeProgress
}

func (m *dispatchMW) msgToItem(msg messaging.Msg) bankPipelineItemState {
	switch r := msg.(type) {
	case memprotocol.ReadReq:
		return bankPipelineItemState{
			IsRead:  true,
			ReadMsg: r,
		}
	case memprotocol.WriteReq:
		return bankPipelineItemState{
			IsRead:   false,
			WriteMsg: r,
		}
	default:
		log.Panicf("simplebankedmemory: unsupported request type %T", msg)
		return bankPipelineItemState{}
	}
}
