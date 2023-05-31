package trans

import (
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/mgpusim/v3/mem/dram/internal/signal"
)

// A SubTransSplitter can split transactions into sub-transactions.
type SubTransSplitter interface {
	Split(t *signal.Transaction)
}

// NewSubTransSplitter creates a default SubTransSplitter
func NewSubTransSplitter(log2BankSize uint64) SubTransSplitter {
	s := &defaultSubTransSplitter{
		log2AccessUnitSize: log2BankSize,
	}

	return s
}

type defaultSubTransSplitter struct {
	log2AccessUnitSize uint64
}

func (s *defaultSubTransSplitter) Split(t *signal.Transaction) {
	addr, size := s.align(t)
	endAddr := addr + size

	unitSize := uint64(1 << s.log2AccessUnitSize)
	for addr < endAddr {
		st := &signal.SubTransaction{
			ID:          sim.GetIDGenerator().Generate(),
			Transaction: t,
			Address:     addr,
		}
		t.SubTransactions = append(t.SubTransactions, st)

		addr += unitSize
	}
}

func (s *defaultSubTransSplitter) align(
	t *signal.Transaction,
) (addr, size uint64) {
	addr = t.GlobalAddress()
	sizeLeft := t.AccessByteSize()
	endAddr := addr + sizeLeft
	unitSize := uint64(1 << s.log2AccessUnitSize)

	addrMask := ^(unitSize - 1)
	addr = addr & addrMask

	currAddr := addr
	for currAddr < endAddr {
		size += unitSize
		currAddr += unitSize
	}

	return addr, size
}
