package dram

// mapAddress decomposes a global physical address into a Location (channel,
// rank, bank group, bank, row, column) using the position/mask parameters in
// Spec. Storage is global, so this operates directly on the request address;
// when several controllers are interleaved, choose the channel/rank/bank bit
// positions so they sit above the upstream controller-select bits.
func mapAddress(spec *Spec, addr uint64) location {
	l := location{}

	l.Channel = (addr >> spec.ChannelPos) & spec.ChannelMask
	l.Rank = (addr >> spec.RankPos) & spec.RankMask
	l.BankGroup = (addr >> spec.BankGroupPos) & spec.BankGroupMask
	l.Bank = (addr >> spec.BankPos) & spec.BankMask
	l.Row = (addr >> spec.RowPos) & spec.RowMask
	l.Column = (addr >> spec.ColPos) & spec.ColMask

	return l
}
