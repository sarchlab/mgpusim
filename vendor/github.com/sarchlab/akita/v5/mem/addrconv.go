package mem

import "log"

// ConvertAddress converts an external address to an internal address using
// interleaving. If kind is empty, the address is returned unchanged.
func ConvertAddress(
	kind string,
	offset uint64,
	interleavingSize uint64,
	totalNumOfElements int,
	currentElementIndex int,
	addr uint64,
) uint64 {
	if kind == "" {
		return addr
	}

	if addr < offset {
		log.Panic("address is smaller than offset")
	}

	a := addr - offset
	roundSize := interleavingSize * uint64(totalNumOfElements)
	belongsTo := int(a % roundSize / interleavingSize)

	if belongsTo != currentElementIndex {
		log.Panicf("address 0x%x does not belong to current element %d",
			addr, currentElementIndex)
	}

	return a/roundSize*interleavingSize + addr%interleavingSize
}
