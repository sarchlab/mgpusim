package internal

//linked list for a free list at each block order
type freeListElement struct {
	freeAddr uint64
	next *freeListElement
}

func pushBack(list **freeListElement, fAddr uint64) {
	if *list == nil {
		*list = &freeListElement{
			freeAddr: fAddr,
			next:     nil,
		}
		return
	}
	l := *list
	for l.next != nil {
		l = l.next
	}
	l.next = &freeListElement{fAddr,nil}
}

func popFront (list **freeListElement) uint64 {
	first := (*list).freeAddr
	*list = (*list).next
	return first
}

type blockTracker struct {
	numOfPages  int
	initialAddr uint64
}

func (bt *blockTracker) removePage() bool {
	bt.numOfPages--
	return bt.numOfPages == 0
}

type bitField struct {
	field []uint64
	size  uint64
}

func newBitField(size uint64) *bitField {
	n := size / 64 + 1
	bf := &bitField{
		field: make([]uint64, n),
		size:  size,
	}
	return bf
}

func (bf *bitField) updateBit(index uint64) {
	arrayIndex := index / 64
	bitIndex := index % 64
	bf.field[arrayIndex] ^= 1 << bitIndex
}

func (bf *bitField) checkBit(index uint64) bool {
	arrayIndex := index / 64
	bitIndex := index % 64
	bits := bf.field[arrayIndex]
	return bits & (1 << bitIndex) != 0
}