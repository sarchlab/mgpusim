package addressmapping

// An LocationItem is a field of the location.
type LocationItem int

// A list of all location items
const (
	LocationItemInvalid LocationItem = iota
	LocationItemChannel
	LocationItemRank
	LocationItemBankGroup
	LocationItemBank
	LocationItemRow
	LocationItemColumn
)

// A Location determines where to find the data to access.
type Location struct {
	Channel   uint64
	Rank      uint64
	BankGroup uint64
	Bank      uint64
	Row       uint64
	Column    uint64
}
