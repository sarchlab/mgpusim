package trans

import (
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/mem/dram/internal/signal"
)

// A SubTransactionQueue is a queue for subtransactions.
type SubTransactionQueue interface {
	CanPush(n int) bool
	Push(t *signal.Transaction)
	Tick(now sim.VTimeInSec) bool
}
