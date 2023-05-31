package addressmapping

// DefaultMapper implements the default address mapping scheme.
type DefaultMapper struct {
	channelPos    int
	channelMask   uint64
	rankPos       int
	rankMask      uint64
	bankGroupPos  int
	bankGroupMask uint64
	bankPos       int
	bankMask      uint64
	rowPos        int
	rowMask       uint64
	colPos        int
	colMask       uint64
}

// Map returns the location  (i.e., channel, rank, bank-group, bank, row, col)
// that can find the given address.
func (m DefaultMapper) Map(addr uint64) Location {
	l := Location{}

	l.Channel = (addr >> m.channelPos) & m.channelMask
	l.Rank = (addr >> m.rankPos) & m.rankMask
	l.BankGroup = (addr >> m.bankGroupPos) & m.bankGroupMask
	l.Bank = (addr >> m.bankPos) & m.bankMask
	l.Row = (addr >> m.rowPos) & m.rowMask
	l.Column = (addr >> m.colPos) & m.colMask

	return l
}
