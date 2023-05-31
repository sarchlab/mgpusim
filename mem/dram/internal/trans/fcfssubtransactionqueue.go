package trans

import (
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/mem/dram/internal/cmdq"
	"github.com/sarchlab/mgpusim/v3/mem/dram/internal/signal"
)

// A FCFSSubTransactionQueue returns sub-transactions in a
// first-come-first-serve way.
type FCFSSubTransactionQueue struct {
	Capacity   int
	Queue      []*signal.SubTransaction
	CmdCreator CommandCreator
	CmdQueue   cmdq.CommandQueue
}

// CanPush returns true if there are enough slots to hold n subtransactions.
func (q *FCFSSubTransactionQueue) CanPush(n int) bool {
	if n >= q.Capacity {
		panic("queue size not large enough to handle a single transaction")
	}

	if len(q.Queue)+n > q.Capacity {
		return false
	}
	return true
}

// Push adds new transaction to the transaction queue.
func (q *FCFSSubTransactionQueue) Push(t *signal.Transaction) {
	if len(q.Queue)+len(t.SubTransactions) > q.Capacity {
		panic("pushing too many subtransactions into queue.")
	}

	q.Queue = append(q.Queue, t.SubTransactions...)
}

// Tick breaks down transactions to commands and dispatches the command to the
// command queues.
func (q *FCFSSubTransactionQueue) Tick(now sim.VTimeInSec) bool {
	for i, subTrans := range q.Queue {
		cmd := q.CmdCreator.Create(subTrans)

		if q.CmdQueue.CanAccept(cmd) {
			q.CmdQueue.Accept(cmd)
			q.Queue = append(q.Queue[:i], q.Queue[i+1:]...)

			// fmt.Printf("Command Pushed: %#v\n", cmd)

			return true
		}
	}

	return false
}
