// Package lruset provides a shared LRU set implementation used by TLB and
// mmuCache. It tracks visit order and key-to-way mappings while remaining
// agnostic to the block payload stored by each consumer.
package lruset

import (
	"fmt"
	"sort"
)

// Set is a generic LRU set that tracks visit order per way.
type Set struct {
	WayCount   int            `json:"way_count"`
	VisitList  []int          `json:"visit_list"`
	VisitCount uint64         `json:"visit_count"`
	LastVisits []uint64       `json:"last_visits"` // per-way last visit time
	KeyMap     map[string]int `json:"key_map"`     // key -> wayID
}

// KeyString builds a canonical lookup key from two uint64 values (e.g. PID
// and virtual address, or PID and segment).
func KeyString(a uint64, b uint64) string {
	return fmt.Sprintf("%d%016x", a, b)
}

// Lookup returns the wayID associated with the given key, if present.
func Lookup(s *Set, key string) (wayID int, found bool) {
	wayID, ok := s.KeyMap[key]
	if !ok {
		return 0, false
	}
	return wayID, true
}

// UpdateKey removes the old key mapping and installs the new one pointing to
// wayID. The caller is responsible for updating the block payload.
func UpdateKey(s *Set, wayID int, oldKey, newKey string) {
	delete(s.KeyMap, oldKey)
	s.KeyMap[newKey] = wayID
}

// Evict removes and returns the least-recently-used wayID.
func Evict(s *Set) (wayID int, ok bool) {
	if len(s.VisitList) == 0 {
		return 0, false
	}
	wayID = s.VisitList[0]
	s.VisitList = s.VisitList[1:]
	return wayID, true
}

// Visit marks wayID as most-recently-used.
func Visit(s *Set, wayID int) {
	// Remove wayID from visit list if present
	for i, w := range s.VisitList {
		if w == wayID {
			s.VisitList = append(s.VisitList[:i], s.VisitList[i+1:]...)
			break
		}
	}

	s.VisitCount++
	s.LastVisits[wayID] = s.VisitCount

	// Insert in sorted position by lastVisit
	targetVisit := s.VisitCount
	index := sort.Search(len(s.VisitList), func(i int) bool {
		return s.LastVisits[s.VisitList[i]] > targetVisit
	})
	s.VisitList = append(s.VisitList, 0)
	copy(s.VisitList[index+1:], s.VisitList[index:])
	s.VisitList[index] = wayID
}

// NewSet creates a Set with the given number of ways. All ways start in the
// visit list so the first eviction returns way 0 (the least recently visited
// after initialisation).
func NewSet(numWays int) Set {
	s := Set{
		WayCount:   numWays,
		LastVisits: make([]uint64, numWays),
		KeyMap:     make(map[string]int),
		VisitList:  make([]int, 0, numWays),
	}
	for j := 0; j < numWays; j++ {
		Visit(&s, j)
	}
	return s
}
