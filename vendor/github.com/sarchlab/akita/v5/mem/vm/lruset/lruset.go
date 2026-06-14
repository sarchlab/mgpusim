// Package lruset provides a shared LRU set implementation used by TLB and
// mmuCache. It tracks visit order and key-to-way mappings while remaining
// agnostic to the block payload stored by each consumer.
package lruset

import (
	"fmt"
	"sort"
)

// Set is a generic LRU set that tracks visit order per way. Its state is
// encapsulated; callers drive it through the methods, which keep the visit
// order and the key-to-way mappings consistent.
type Set struct {
	wayCount   int
	visitList  []int
	visitCount uint64
	lastVisits []uint64       // per-way last visit time
	keyMap     map[string]int // key -> wayID
}

// KeyString builds a canonical lookup key from two uint64 values (e.g. PID
// and virtual address, or PID and segment).
func KeyString(a uint64, b uint64) string {
	return fmt.Sprintf("%d%016x", a, b)
}

// NewSet creates a Set with the given number of ways. All ways start in the
// visit list so the first eviction returns way 0 (the least recently visited
// after initialisation).
func NewSet(numWays int) Set {
	s := Set{
		wayCount:   numWays,
		lastVisits: make([]uint64, numWays),
		keyMap:     make(map[string]int),
		visitList:  make([]int, 0, numWays),
	}
	for j := 0; j < numWays; j++ {
		s.Visit(j)
	}

	return s
}

// Lookup returns the wayID associated with the given key, if present.
func (s *Set) Lookup(key string) (wayID int, found bool) {
	wayID, ok := s.keyMap[key]
	if !ok {
		return 0, false
	}

	return wayID, true
}

// UpdateKey removes the old key mapping and installs the new one pointing to
// wayID. The caller is responsible for updating the block payload.
func (s *Set) UpdateKey(wayID int, oldKey, newKey string) {
	delete(s.keyMap, oldKey)
	s.keyMap[newKey] = wayID
}

// Remove drops the mapping for key, if present. The way it referenced
// becomes a lookup miss; its slot is reused when a future insertion
// installs a new key. The caller is responsible for updating the block
// payload.
func (s *Set) Remove(key string) {
	delete(s.keyMap, key)
}

// Evict removes and returns the least-recently-used wayID.
func (s *Set) Evict() (wayID int, ok bool) {
	if len(s.visitList) == 0 {
		return 0, false
	}
	wayID = s.visitList[0]
	s.visitList = s.visitList[1:]

	return wayID, true
}

// Visit marks wayID as most-recently-used.
func (s *Set) Visit(wayID int) {
	// Remove wayID from visit list if present
	for i, w := range s.visitList {
		if w == wayID {
			s.visitList = append(s.visitList[:i], s.visitList[i+1:]...)
			break
		}
	}

	s.visitCount++
	s.lastVisits[wayID] = s.visitCount

	// Insert in sorted position by lastVisit
	targetVisit := s.visitCount
	index := sort.Search(len(s.visitList), func(i int) bool {
		return s.lastVisits[s.visitList[i]] > targetVisit
	})
	s.visitList = append(s.visitList, 0)
	copy(s.visitList[index+1:], s.visitList[index:])
	s.visitList[index] = wayID
}
