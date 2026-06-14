package vm

import (
	"container/list"
	"sync"
)

// PID stands for Process ID.
type PID uint32

// A Page is an entry in the page table, maintaining the information about how
// to translate a virtual address to a physical address.
type Page struct {
	PID         PID    `json:"pid"`
	PAddr       uint64 `json:"p_addr"`
	VAddr       uint64 `json:"v_addr"`
	PageSize    uint64 `json:"page_size"`
	Valid       bool   `json:"valid"`
	DeviceID    uint64 `json:"device_id"`
	Unified     bool   `json:"unified"`
	IsMigrating bool   `json:"is_migrating"`
	IsPinned    bool   `json:"is_pinned"`
}

// A PageTable holds the a list of pages.
type PageTable interface {
	Insert(page Page)
	Remove(pid PID, vAddr uint64)
	Find(pid PID, Addr uint64) (Page, bool)
	Update(page Page)
	ReverseLookup(pAddr uint64) (Page, bool)
}

// NewPageTable creates an unnamed, unregistered PageTable. Prefer
// PageTableBuilder for a page table that should be a named simulation resource.
func NewPageTable(log2PageSize uint64) PageTable {
	return MakePageTableBuilder().WithLog2PageSize(log2PageSize).Build("")
}

// pageTableImpl is the default implementation of a Page Table
type pageTableImpl struct {
	sync.Mutex

	name         string
	log2PageSize uint64
	tables       map[PID]*processTable
}

// Name returns the name of the page table. It is empty for page tables created
// through NewPageTable; use PageTableBuilder to give it a name and register it
// as a simulation resource.
func (pt *pageTableImpl) Name() string {
	return pt.name
}

func (pt *pageTableImpl) getTable(pid PID) *processTable {
	pt.Lock()
	defer pt.Unlock()

	table, found := pt.tables[pid]
	if !found {
		table = &processTable{
			entries:      list.New(),
			entriesTable: make(map[uint64]*list.Element),
		}
		pt.tables[pid] = table
	}

	return table
}

func (pt *pageTableImpl) alignToPage(addr uint64) uint64 {
	return (addr >> pt.log2PageSize) << pt.log2PageSize
}

// GetLog2PageSize returns the log2 page size for this page table.
// This method allows the MMU to validate page size consistency.
func (pt *pageTableImpl) GetLog2PageSize() uint64 {
	return pt.log2PageSize
}

// Insert put a new page into the PageTable
func (pt *pageTableImpl) Insert(page Page) {
	table := pt.getTable(page.PID)
	table.insert(page)
}

// Remove removes the entry in the page table that contains the target
// address.
func (pt *pageTableImpl) Remove(pid PID, vAddr uint64) {
	table := pt.getTable(pid)
	table.remove(vAddr)
}

// Find returns the page that contains the given virtual address. The bool
// return value invicates if the page is found or not.
func (pt *pageTableImpl) Find(pid PID, vAddr uint64) (Page, bool) {
	table := pt.getTable(pid)
	vAddr = pt.alignToPage(vAddr)

	return table.find(vAddr)
}

// Update changes the field of an existing page. The PID and the VAddr field
// will be used to locate the page to update.
func (pt *pageTableImpl) Update(page Page) {
	table := pt.getTable(page.PID)
	table.update(page)
}

// ReverseLookup finds a page by its physical address across all processes.
func (pt *pageTableImpl) ReverseLookup(pAddr uint64) (Page, bool) {
	pt.Lock()
	defer pt.Unlock()

	for _, processTable := range pt.tables {
		if page, found := processTable.reverseLookup(pAddr); found {
			return page, true
		}
	}
	return Page{}, false
}

type processTable struct {
	sync.Mutex

	entries      *list.List
	entriesTable map[uint64]*list.Element
}

func (t *processTable) insert(page Page) {
	t.Lock()
	defer t.Unlock()

	t.pageMustNotExist(page.VAddr)

	elem := t.entries.PushBack(page)
	t.entriesTable[page.VAddr] = elem
}

func (t *processTable) remove(vAddr uint64) {
	t.Lock()
	defer t.Unlock()

	t.pageMustExist(vAddr)

	elem := t.entriesTable[vAddr]
	t.entries.Remove(elem)
	delete(t.entriesTable, vAddr)
}

func (t *processTable) update(page Page) {
	t.Lock()
	defer t.Unlock()

	t.pageMustExist(page.VAddr)

	elem := t.entriesTable[page.VAddr]
	elem.Value = page
}

func (t *processTable) find(vAddr uint64) (Page, bool) {
	t.Lock()
	defer t.Unlock()

	elem, found := t.entriesTable[vAddr]
	if found {
		return elem.Value.(Page), true
	}

	return Page{}, false
}

func (t *processTable) reverseLookup(pAddr uint64) (Page, bool) {
	t.Lock()
	defer t.Unlock()

	for elem := t.entries.Front(); elem != nil; elem = elem.Next() {
		page := elem.Value.(Page)
		if page.PAddr == pAddr {
			return page, true
		}
	}

	return Page{}, false
}

func (t *processTable) pageMustExist(vAddr uint64) {
	_, found := t.entriesTable[vAddr]
	if !found {
		panic("page does not exist")
	}
}

func (t *processTable) pageMustNotExist(vAddr uint64) {
	_, found := t.entriesTable[vAddr]
	if found {
		panic("page exist")
	}
}
