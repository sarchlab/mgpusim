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