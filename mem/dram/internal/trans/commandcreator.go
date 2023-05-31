package trans

import (
	"github.com/sarchlab/mgpusim/v3/mem/dram/internal/signal"
)

// A CommandCreator can convert a subtransaction to a command.
type CommandCreator interface {
	Create(subTrans *signal.SubTransaction) *signal.Command
}
