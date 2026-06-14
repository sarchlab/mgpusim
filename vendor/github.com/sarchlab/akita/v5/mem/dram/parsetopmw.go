package dram

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/modeling"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/tracing"
)

type parseTopMW struct {
	comp *modeling.Component[Spec, State, Resources]
}

func (m *parseTopMW) topPort() messaging.Port {
	return m.comp.GetPortByName("Top")
}

// Tick runs the parseTop stage. Pause and Drain both stop accepting
// new traffic from the Top port; only Enabled DRAM accepts new
// transactions.
func (m *parseTopMW) Tick() bool {
	next := &m.comp.State
	if next.ControlState != memcontrolprotocol.StateEnabled {
		return false
	}
	spec := m.comp.Spec()

	return m.parseTop(&spec, next)
}

func (m *parseTopMW) parseTop(spec *Spec, next *State) bool {
	msgI := m.topPort().PeekIncoming()
	if msgI == nil {
		return false
	}

	ts := transactionState{}

	switch msg := msgI.(type) {
	case memprotocol.ReadReq:
		ts.HasRead = true
		ts.ReadMsg = msg
	case memprotocol.WriteReq:
		ts.HasWrite = true
		ts.WriteMsg = msg
	default:
		panic(fmt.Sprintf("dram parseTop: unsupported message type %T", msgI))
	}

	// Split into sub-transactions
	transIdx := len(next.Transactions)
	splitTransaction(spec, &ts, transIdx)

	if !canPushSubTrans(next, len(ts.SubTransactions),
		spec.TransactionQueueSize) {
		return false
	}

	ts.ArrivalTick = next.TickCount
	next.Transactions = append(next.Transactions, ts)
	pushSubTrans(next, transIdx)
	m.topPort().RetrieveIncoming()

	tracing.TraceReqReceive(m.comp, msgI)

	for _, st := range ts.SubTransactions {
		tracing.StartTask(m.comp, tracing.TaskStart{
			ID:       st.ID,
			ParentID: tracing.MsgIDAtReceiver(msgI, m.comp),
			Kind:     "sub-trans",
			What:     "sub-trans",
			Location: m.comp.Name() + ".SubTransQueue",
		})
	}

	return true
}
