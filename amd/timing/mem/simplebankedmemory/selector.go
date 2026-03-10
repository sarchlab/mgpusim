package simplebankedmemory

// bankSelector decides which bank should serve a request.
type bankSelector interface {
	Select(address uint64, numBanks int) int
}

// interleavedBankSelector assigns addresses to banks in an interleaved fashion.
type interleavedBankSelector struct {
	Log2InterleaveSize uint64
}

// Select returns the bank index for the given address.
func (s interleavedBankSelector) Select(address uint64, numBanks int) int {
	if numBanks == 0 {
		return 0
	}

	interleaveSize := uint64(1) << s.Log2InterleaveSize
	if interleaveSize == 0 {
		panic("simplebankedmemory.interleavedBankSelector: invalid interleave size")
	}

	return int((address / interleaveSize) % uint64(numBanks))
}
