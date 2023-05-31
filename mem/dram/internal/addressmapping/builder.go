package addressmapping

// Builder can build default address mappers.
type Builder struct {
	busWidth          int
	burstLength       int
	numChannel        int
	numRank           int
	numBankGroup      int
	numBank           int
	numRow            int
	numCol            int
	bitOrderHighToLow []LocationItem

	accessUnitBit uint64
	colBit        uint64
	colLoBit      uint64
	colHiBit      uint64
	rowBit        uint64
	bankBit       uint64
	bankGroupBit  uint64
	rankBit       uint64
	channelBit    uint64
}

// MakeBuilder creates a new builder with default configurations.
func MakeBuilder() Builder {
	return Builder{
		busWidth:     64,
		burstLength:  8,
		numChannel:   1,
		numRank:      1,
		numBankGroup: 1,
		numBank:      8,
		numRow:       65536,
		numCol:       2048,
		bitOrderHighToLow: []LocationItem{
			LocationItemRow,
			LocationItemChannel,
			LocationItemRank,
			LocationItemBank,
			LocationItemBankGroup,
			LocationItemColumn,
		},
	}
}

// WithBusWidth sets the number of bits can be transferred out of the banks
// at the same time.
func (b Builder) WithBusWidth(n int) Builder {
	b.busWidth = n
	return b
}

// WithBurstLength sets the number of access (each access manipulates the amount
// of data that equals the bus width) that takes place as one group.
func (b Builder) WithBurstLength(n int) Builder {
	b.burstLength = n
	return b
}

// WithNumChannel sets the channels that the memory controller controls.
func (b Builder) WithNumChannel(n int) Builder {
	b.numChannel = n
	return b
}

// WithNumRank sets the number of ranks in each channel.
func (b Builder) WithNumRank(n int) Builder {
	b.numRank = n
	return b
}

// WithNumBankGroup sets the number of bank groups in each rank.
func (b Builder) WithNumBankGroup(n int) Builder {
	b.numBankGroup = n
	return b
}

// WithNumBank sets the number of banks in each bank group.
func (b Builder) WithNumBank(n int) Builder {
	b.numBank = n
	return b
}

// WithNumRow sets the number of rows in each DRAM array.
func (b Builder) WithNumRow(n int) Builder {
	b.numRow = n
	return b
}

// WithNumCol sets the number of columns in each DRAM array.
func (b Builder) WithNumCol(n int) Builder {
	b.numCol = n
	return b
}

// Build builds a default memory mapper.
func (b Builder) Build() Mapper {
	m := DefaultMapper{}

	b.calculateBits()

	m.channelMask = (1 << b.channelBit) - 1
	m.rankMask = (1 << b.rankBit) - 1
	m.bankGroupMask = (1 << b.bankGroupBit) - 1
	m.bankMask = (1 << b.bankBit) - 1
	m.rowMask = (1 << b.rowBit) - 1
	m.colMask = (1 << b.colHiBit) - 1

	pos := b.accessUnitBit
	for len(b.bitOrderHighToLow) > 0 {
		curr := b.bitOrderHighToLow[len(b.bitOrderHighToLow)-1]
		b.bitOrderHighToLow =
			b.bitOrderHighToLow[0 : len(b.bitOrderHighToLow)-1]
		switch curr {
		case LocationItemChannel:
			m.channelPos = int(pos)
			pos += b.channelBit
		case LocationItemRank:
			m.rankPos = int(pos)
			pos += b.rankBit
		case LocationItemBankGroup:
			m.bankGroupPos = int(pos)
			pos += b.bankGroupBit
		case LocationItemBank:
			m.bankPos = int(pos)
			pos += b.bankBit
		case LocationItemRow:
			m.rowPos = int(pos)
			pos += b.rowBit
		case LocationItemColumn:
			m.colPos = int(pos)
			pos += b.colHiBit
		}
	}

	return m
}

func (b *Builder) calculateBits() {
	b.colLoBit, _ = log2(uint64(b.burstLength))
	b.colBit, _ = log2(uint64(b.numCol))
	b.colHiBit = b.colBit - b.colLoBit

	b.channelBit, _ = log2(uint64(b.numChannel))
	b.rankBit, _ = log2(uint64(b.numRank))
	b.bankGroupBit, _ = log2(uint64(b.numBankGroup))
	b.bankBit, _ = log2(uint64(b.numBank))
	b.rowBit, _ = log2(uint64(b.numRow))
	b.accessUnitBit, _ = log2(uint64(b.busWidth / 8 * b.burstLength))
}

// log2 returns the log2 of a number. It also returns false if it is not a log2
// number.
func log2(n uint64) (uint64, bool) {
	oneCount := 0
	onePos := uint64(0)
	for i := uint64(0); i < 64; i++ {
		if n&(1<<i) > 0 {
			onePos = i
			oneCount++
		}
	}

	return onePos, oneCount == 1
}
