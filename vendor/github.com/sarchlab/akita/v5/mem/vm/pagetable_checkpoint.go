package vm

import (
	"container/list"
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

// pageTableCheckpoint is the serialized form of a page table: its shape
// (log2 page size) plus, for each process, the pages in list order. Pages are
// value structs with only exported fields, so they round-trip as JSON.
type pageTableCheckpoint struct {
	Log2PageSize uint64              `json:"log2_page_size"`
	Tables       []processTableEntry `json:"tables"`
}

// processTableEntry is one process's pages, kept in their list order so the
// restored table iterates identically (e.g. for ReverseLookup).
type processTableEntry struct {
	PID   PID    `json:"pid"`
	Pages []Page `json:"pages"`
}

// SaveCheckpoint writes the page table's shape and every page, grouped by
// process and sorted by PID so the output is deterministic.
func (pt *pageTableImpl) SaveCheckpoint(w io.Writer) error {
	pt.Lock()
	defer pt.Unlock()

	pids := make([]PID, 0, len(pt.tables))
	for pid := range pt.tables {
		pids = append(pids, pid)
	}
	sort.Slice(pids, func(i, j int) bool { return pids[i] < pids[j] })

	dto := pageTableCheckpoint{
		Log2PageSize: pt.log2PageSize,
		Tables:       make([]processTableEntry, 0, len(pids)),
	}
	for _, pid := range pids {
		table := pt.tables[pid]
		pages := make([]Page, 0, table.entries.Len())
		for elem := table.entries.Front(); elem != nil; elem = elem.Next() {
			pages = append(pages, elem.Value.(Page))
		}
		dto.Tables = append(dto.Tables, processTableEntry{PID: pid, Pages: pages})
	}

	return json.NewEncoder(w).Encode(dto)
}

// LoadCheckpoint rebuilds the per-process tables after checking that the saved
// shape matches the rebuilt page table. The page table is expected to be freshly
// built (empty), so restored pages cannot collide.
func (pt *pageTableImpl) LoadCheckpoint(r io.Reader) error {
	var dto pageTableCheckpoint
	if err := json.NewDecoder(r).Decode(&dto); err != nil {
		return fmt.Errorf("vm: decode page table %q: %w", pt.name, err)
	}

	if dto.Log2PageSize != pt.log2PageSize {
		return fmt.Errorf(
			"vm: page table %q log2 page size mismatch: checkpoint %d, rebuilt %d",
			pt.name, dto.Log2PageSize, pt.log2PageSize)
	}

	pt.Lock()
	defer pt.Unlock()

	pt.tables = make(map[PID]*processTable, len(dto.Tables))
	for _, entry := range dto.Tables {
		table := &processTable{
			entries:      list.New(),
			entriesTable: make(map[uint64]*list.Element, len(entry.Pages)),
		}
		for _, page := range entry.Pages {
			elem := table.entries.PushBack(page)
			table.entriesTable[page.VAddr] = elem
		}
		pt.tables[entry.PID] = table
	}

	return nil
}
