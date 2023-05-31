// Package internal provides the definition required for defining TLB.
package internal

import (
	"fmt"

	"github.com/google/btree"
	"github.com/sarchlab/mgpusim/v3/mem/vm"
)

// A Set holds a certain number of pages.
type Set interface {
	Lookup(pid vm.PID, vAddr uint64) (wayID int, page vm.Page, found bool)
	Update(wayID int, page vm.Page)
	Evict() (wayID int, ok bool)
	Visit(wayID int)
}

// NewSet creates a new TLB set.
func NewSet(numWays int) Set {
	s := &setImpl{}
	s.blocks = make([]*block, numWays)
	s.visitTree = btree.New(2)
	s.vAddrWayIDMap = make(map[string]int)
	for i := range s.blocks {
		b := &block{}
		s.blocks[i] = b
		b.wayID = i
		s.Visit(i)
	}
	return s
}

type block struct {
	page      vm.Page
	wayID     int
	lastVisit uint64
}

func (b *block) Less(anotherBlock btree.Item) bool {
	return b.lastVisit < anotherBlock.(*block).lastVisit
}

type setImpl struct {
	blocks        []*block
	vAddrWayIDMap map[string]int
	visitTree     *btree.BTree
	visitCount    uint64
}

func (s *setImpl) keyString(pid vm.PID, vAddr uint64) string {
	return fmt.Sprintf("%d%016x", pid, vAddr)
}

func (s *setImpl) Lookup(pid vm.PID, vAddr uint64) (
	wayID int,
	page vm.Page,
	found bool,
) {
	key := s.keyString(pid, vAddr)
	wayID, ok := s.vAddrWayIDMap[key]
	if !ok {
		return 0, vm.Page{}, false
	}

	block := s.blocks[wayID]

	return block.wayID, block.page, true
}

func (s *setImpl) Update(wayID int, page vm.Page) {
	block := s.blocks[wayID]
	key := s.keyString(block.page.PID, block.page.VAddr)
	delete(s.vAddrWayIDMap, key)

	block.page = page
	key = s.keyString(page.PID, page.VAddr)
	s.vAddrWayIDMap[key] = wayID
}

func (s *setImpl) Evict() (wayID int, ok bool) {
	if s.hasNothingToEvict() {
		return 0, false
	}

	wayID = s.visitTree.DeleteMin().(*block).wayID
	return wayID, true
}

func (s *setImpl) Visit(wayID int) {
	block := s.blocks[wayID]
	s.visitTree.Delete(block)

	s.visitCount++
	block.lastVisit = s.visitCount
	s.visitTree.ReplaceOrInsert(block)
}

func (s *setImpl) hasNothingToEvict() bool {
	return s.visitTree.Len() == 0
}
