package mem

import "log"

// AddressConverter can translate the address between two domains
type AddressConverter interface {
	ConvertExternalToInternal(external uint64) uint64
	ConvertInternalToExternal(internal uint64) uint64
}

// InterleavingConverter is an address converter that can converts the address
// from an continuous address space to interleaved addresses.
//
// For example, DRAM is divided into multiple banks, the internal address
// of each bank starts from 0, while the global address is continuous. In
// this case, we can use the InterleavingConverter to convert the
// external addresses from/ to internal addresses.
type InterleavingConverter struct {
	InterleavingSize    uint64
	TotalNumOfElements  int
	CurrentElementIndex int
	Offset              uint64
}

// ConvertExternalToInternal converts from external address to internal address
func (c InterleavingConverter) ConvertExternalToInternal(external uint64) uint64 {
	if external < c.Offset {
		log.Panic("address is smaller than offset")
	}

	addr := external - c.Offset
	roundSize := c.InterleavingSize * uint64(c.TotalNumOfElements)
	belongsTo := int(addr % roundSize / c.InterleavingSize)
	if belongsTo != c.CurrentElementIndex {
		log.Panicf("address 0x%x does not belongs to current element %d",
			external, c.CurrentElementIndex)
	}

	internal := addr/(roundSize)*c.InterleavingSize +
		external%c.InterleavingSize
	return internal
}

// ConvertInternalToExternal converts from internal address to external address
func (c InterleavingConverter) ConvertInternalToExternal(internal uint64) uint64 {
	panic("this function should never be called")
}
