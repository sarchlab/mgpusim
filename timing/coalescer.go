package timing

import (
	"gitlab.com/akita/mem/cache"
)

type CoalescedAccess struct {
	Addr           uint64
	Size           uint64
	LaneIDs        []int
	LaneAddrOffset []uint64
}

// A Coalescer defines the algorithm on how addresses can be coalesced
type Coalescer interface {
	Coalesce(addresses []uint64, bytesPerWI int) []CoalescedAccess
}

func NewCoalescer() *DefaultCoalescer {
	c := new(DefaultCoalescer)
	c.CoalescingWidth = 4
	c.CacheLineSizeAsPowerOf2 = 6
	return c
}

// DefaultCoalescer provides the default coalescing algorithm.
type DefaultCoalescer struct {
	CoalescingWidth         int // Number of WIs that can be coalesced
	CacheLineSizeAsPowerOf2 uint64
}

func (c *DefaultCoalescer) Coalesce(
	addresses []uint64,
	bytesPerWI int,
) []CoalescedAccess {
	coalescedAddresses := make([]CoalescedAccess, 0, 64)

	numGroups := 64 / c.CoalescingWidth
	for i := 0; i < numGroups; i++ {
		startIndex := i * c.CoalescingWidth
		endIndex := startIndex + c.CoalescingWidth

		c.coaleseLaneGroups(
			&coalescedAddresses,
			addresses[startIndex:endIndex],
			bytesPerWI,
			i*c.CoalescingWidth,
		)
	}

	return coalescedAddresses
}

func (c *DefaultCoalescer) coaleseLaneGroups(
	coalescedAddresses *[]CoalescedAccess,
	addresses []uint64,
	bytesPerWI int,
	firstLaneID int,
) {
	if c.trySameAddressCoalesce(coalescedAddresses, addresses, bytesPerWI, firstLaneID) {
		return
	}

	if c.tryAdjacentAddressCoalesce(coalescedAddresses, addresses, bytesPerWI, firstLaneID) {
		return
	}

	c.doNotCoalesce(coalescedAddresses, addresses, bytesPerWI, firstLaneID)
}

func (c *DefaultCoalescer) trySameAddressCoalesce(
	coalescedAddresses *[]CoalescedAccess,
	addresses []uint64,
	bytesPerWI int,
	firstLaneID int,
) bool {
	if c.isSameAddress(addresses) {
		address := addresses[0]
		//address, _ = cache.GetCacheLineID(address, c.CacheLineSizeAsPowerOf2)
		access := CoalescedAccess{
			address, uint64(bytesPerWI),
			[]int{firstLaneID, firstLaneID + 1, firstLaneID + 2, firstLaneID + 3},
			[]uint64{0, 0, 0, 0},
		}
		*coalescedAddresses = append(*coalescedAddresses, access)
		return true
	}
	return false
}

func (c *DefaultCoalescer) isSameAddress(addresses []uint64) bool {
	for i := 0; i < len(addresses)-1; i++ {
		if addresses[i] != addresses[i+1] {
			return false
		}
	}
	return true
}

func (c *DefaultCoalescer) tryAdjacentAddressCoalesce(
	coalescedAddresses *[]CoalescedAccess,
	addresses []uint64,
	bytesPerWI int,
	firstLaneID int,
) bool {
	if c.canDoAdjacentCoalescing(addresses, bytesPerWI) {
		var access CoalescedAccess
		access.Addr = addresses[0]
		access.Size = uint64(c.CoalescingWidth * bytesPerWI)
		for i := 0; i < c.CoalescingWidth; i++ {
			access.LaneIDs = append(access.LaneIDs, firstLaneID+i)
			access.LaneAddrOffset = append(access.LaneAddrOffset, uint64(i*bytesPerWI))
		}
		*coalescedAddresses = append(*coalescedAddresses, access)
		return true
	}
	return false
}

func (c *DefaultCoalescer) canDoAdjacentCoalescing(
	addresses []uint64, unitBytes int,
) bool {
	return c.addressesAdjacent(addresses, unitBytes) &&
		c.addressesOnSameCacheLine(addresses)
}

func (c *DefaultCoalescer) addressesAdjacent(
	addresses []uint64,
	unitBytes int,
) bool {
	for i := 1; i < len(addresses); i++ {
		if addresses[i] != addresses[i-1]+uint64(unitBytes) {
			return false
		}
	}
	return true
}

func (c *DefaultCoalescer) addressesOnSameCacheLine(addresses []uint64) bool {
	firstLineID, _ := cache.GetCacheLineID(addresses[0], c.CacheLineSizeAsPowerOf2)

	for i := 1; i < len(addresses); i++ {
		lineID, _ := cache.GetCacheLineID(addresses[i], c.CacheLineSizeAsPowerOf2)
		if lineID != firstLineID {
			return false
		}
	}

	return true
}

func (c *DefaultCoalescer) doNotCoalesce(
	coalescedAddresses *[]CoalescedAccess,
	addresses []uint64,
	bytesPerWI int,
	firstLaneID int,
) {
	for laneID, addr := range addresses {
		pair := CoalescedAccess{addr, uint64(bytesPerWI),
			[]int{firstLaneID + laneID}, []uint64{0}}
		*coalescedAddresses = append(*coalescedAddresses, pair)
	}
}

// MockCoalescer is a coalescer for testing purposes
type MockCoalescer struct {
	ToReturn []CoalescedAccess
}

func (c *MockCoalescer) Coalesce(addresses []uint64, bytesPerWI int) []CoalescedAccess {
	return c.ToReturn
}
