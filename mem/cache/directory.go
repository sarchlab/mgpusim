package cache

import (
	"github.com/sarchlab/mgpusim/v3/mem/mem"
	"github.com/sarchlab/mgpusim/v3/mem/vm"
)

// A Block of a cache is the information that is associated with a cache line
type Block struct {
	PID          vm.PID
	Tag          uint64
	WayID        int
	SetID        int
	CacheAddress uint64
	IsValid      bool
	IsDirty      bool
	ReadCount    int
	IsLocked     bool
	DirtyMask    []bool
}

// A Set is a list of blocks where a certain piece memory can be stored at
type Set struct {
	Blocks   []*Block
	LRUQueue []*Block
}

// A Directory stores the information about what is stored in the cache.
type Directory interface {
	Lookup(pid vm.PID, address uint64) *Block
	FindVictim(address uint64) *Block
	Visit(block *Block)
	TotalSize() uint64
	WayAssociativity() int
	GetSets() []Set
	Reset()
}

// A DirectoryImpl is the default implementation of a Directory
//
// The directory can translate from the request address (can be either virtual
// address or physical address) to the cache based address.
type DirectoryImpl struct {
	NumSets       int
	NumWays       int
	BlockSize     int
	AddrConverter mem.AddressConverter

	Sets []Set

	victimFinder VictimFinder
}

// NewDirectory returns a new directory object
func NewDirectory(
	set, way, blockSize int,
	victimFinder VictimFinder,
) *DirectoryImpl {
	d := new(DirectoryImpl)
	d.victimFinder = victimFinder
	d.Sets = make([]Set, set)

	d.NumSets = set
	d.NumWays = way
	d.BlockSize = blockSize

	d.Reset()

	return d
}

// TotalSize returns the maximum number of bytes can be stored in the cache
func (d *DirectoryImpl) TotalSize() uint64 {
	return uint64(d.NumSets) * uint64(d.NumWays) * uint64(d.BlockSize)
}

// Get the set that a certain address should store at
func (d *DirectoryImpl) getSet(reqAddr uint64) (set *Set, setID int) {
	if d.AddrConverter != nil {
		reqAddr = d.AddrConverter.ConvertExternalToInternal(reqAddr)
	}

	setID = int(reqAddr / uint64(d.BlockSize) % uint64(d.NumSets))
	set = &d.Sets[setID]
	return
}

// Lookup finds the block that reqAddr. If the reqAddr is valid
// in the cache, return the block information. Otherwise, return nil
func (d *DirectoryImpl) Lookup(PID vm.PID, reqAddr uint64) *Block {
	set, _ := d.getSet(reqAddr)
	for _, block := range set.Blocks {
		if block.IsValid && block.Tag == reqAddr && block.PID == PID {
			return block
		}
	}
	return nil
}

// FindVictim returns a block that can be used to stored data at address addr.
//
// If it is valid, the cache controller need to decide what to do to evict the
// the data in the block
func (d *DirectoryImpl) FindVictim(addr uint64) *Block {
	set, _ := d.getSet(addr)
	block := d.victimFinder.FindVictim(set)
	return block
}

// Visit moves the block to the end of the LRUQueue
func (d *DirectoryImpl) Visit(block *Block) {
	set := d.Sets[block.SetID]
	for i, b := range set.LRUQueue {
		if b == block {
			set.LRUQueue = append(set.LRUQueue[:i], set.LRUQueue[i+1:]...)
			break
		}
	}
	set.LRUQueue = append(set.LRUQueue, block)
}

// GetSets returns all the sets in a directory
func (d *DirectoryImpl) GetSets() []Set {
	return d.Sets
}

// Reset will mark all the blocks in the directory invalid
func (d *DirectoryImpl) Reset() {
	d.Sets = make([]Set, d.NumSets)
	for i := 0; i < d.NumSets; i++ {
		for j := 0; j < d.NumWays; j++ {
			block := new(Block)
			block.IsValid = false
			block.SetID = i
			block.WayID = j
			block.CacheAddress = uint64(i*d.NumWays+j) * uint64(d.BlockSize)
			d.Sets[i].Blocks = append(d.Sets[i].Blocks, block)
			d.Sets[i].LRUQueue = append(d.Sets[i].LRUQueue, block)
		}
	}
}

// WayAssociativity returns the number of ways per set in the cache.
func (d *DirectoryImpl) WayAssociativity() int {
	return d.NumWays
}
