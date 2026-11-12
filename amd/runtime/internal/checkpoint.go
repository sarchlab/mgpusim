package internal

import (
	"container/list"
	"fmt"
	"slices"

	"github.com/sarchlab/akita/v5/mem/vm"
)

// AllocatorState contains the allocator's own bookkeeping. Page-table contents
// and physical storage are checkpointed by their registered Akita resources.
type AllocatorState struct {
	Log2PageSize, TotalStorageBytes uint64
	NextVAddr                       map[vm.PID]uint64
	Devices                         []DeviceState
}

type DeviceState struct {
	ID                          int
	Type                        DeviceType
	UnifiedGPUIDs, ActualGPUIDs []int
	NextActualGPU               int
	Properties                  DeviceProperties
	Memory                      MemoryState
}

type MemoryState struct {
	Buddy, Initialized                        bool
	Log2PageSize, InitialAddress, StorageSize uint64
	AvailableRuns                             []PageRun
	FreeLists                                 [][]uint64
	Split, Merge                              BitFieldState
	Trackers                                  map[uint64]BlockTrackerState
	PageToBlock                               map[uint64]uint64
}

// PageRun describes consecutive free pages in allocation order. Runs are never
// sorted: freed pages retain their position at the tail of the allocator queue.
type PageRun struct {
	Start, Count uint64
}

type BitFieldState struct {
	Present bool
	Size    uint64
	Field   []uint64
}
type BlockTrackerState struct {
	NumPages       int
	InitialAddress uint64
}

func saveBits(b *bitField) BitFieldState {
	if b == nil {
		return BitFieldState{}
	}
	return BitFieldState{true, b.size, b.field}
}
func loadBits(s BitFieldState) *bitField {
	if !s.Present {
		return nil
	}
	return &bitField{size: s.Size, field: s.Field}
}

func saveMemory(m DeviceMemoryState) MemoryState {
	switch m := m.(type) {
	case *deviceMemoryStateImpl:
		return MemoryState{Log2PageSize: m.log2PageSize,
			InitialAddress: m.initialAddress,
			StorageSize:    m.storageSize,
			AvailableRuns:  pageRuns(m.availablePAddrs, m.log2PageSize)}
	case *deviceBuddyMemoryState:
		s := MemoryState{Buddy: true,
			Initialized:    m.initFlag,
			Log2PageSize:   m.log2PageSize,
			InitialAddress: m.initialAddress,
			StorageSize:    m.storageSize,
			Split:          saveBits(m.bfBlockSplit),
			Merge:          saveBits(m.bfMergeList),
			Trackers:       make(map[uint64]BlockTrackerState),
			PageToBlock:    make(map[uint64]uint64),
			FreeLists: make([][]uint64,
				len(m.freeList))}
		for i := range m.freeList {
			for e := m.freeList[i].Front(); e != nil; e = e.Next() {
				s.FreeLists[i] = append(s.FreeLists[i], e.Value.(uint64))
			}
		}
		for page, b := range m.blockTracking {
			s.PageToBlock[page] = b.initialAddr
			s.Trackers[b.initialAddr] = BlockTrackerState{b.numOfPages, b.initialAddr}
		}
		return s
	default:
		panic("driver: unknown device memory allocator")
	}
}

func loadMemory(s MemoryState) (DeviceMemoryState, error) {
	if !s.Buddy {
		pages, err := expandPageRuns(s)
		if err != nil {
			return nil, err
		}
		return &deviceMemoryStateImpl{
			log2PageSize:    s.Log2PageSize,
			initialAddress:  s.InitialAddress,
			storageSize:     s.StorageSize,
			availablePAddrs: pages,
		}, nil
	}
	m := &deviceBuddyMemoryState{
		initFlag:       s.Initialized,
		log2PageSize:   s.Log2PageSize,
		initialAddress: s.InitialAddress,
		storageSize:    s.StorageSize,
		bfBlockSplit:   loadBits(s.Split),
		bfMergeList:    loadBits(s.Merge),
		freeList:       make([]list.List, len(s.FreeLists)),
		blockTracking:  make(map[uint64]*blockTracker),
	}
	for i, values := range s.FreeLists {
		for _, v := range values {
			m.freeList[i].PushBack(v)
		}
	}
	trackers := make(map[uint64]*blockTracker, len(s.Trackers))
	for id, b := range s.Trackers {
		trackers[id] = &blockTracker{numOfPages: b.NumPages, initialAddr: b.InitialAddress}
	}
	for page, id := range s.PageToBlock {
		b, ok := trackers[id]
		if !ok {
			return nil, fmt.Errorf("driver: missing buddy block %d", id)
		}
		m.blockTracking[page] = b
	}
	return m, nil
}

// SnapshotAllocator copies allocator metadata for checkpoint serialization.
// The caller must exclude simulation and host allocation operations until the
// returned state has been serialized.
func SnapshotAllocator(allocator MemoryAllocator, devices []*Device) AllocatorState {
	a := allocator.(*memoryAllocatorImpl)
	s := AllocatorState{
		Log2PageSize:      a.log2PageSize,
		TotalStorageBytes: a.totalStorageByteSize,
		NextVAddr:         make(map[vm.PID]uint64),
	}
	for pid, p := range a.processMemoryStates {
		s.NextVAddr[pid] = p.nextVAddr
	}
	for _, d := range devices {
		v := DeviceState{
			ID:            d.ID,
			Type:          d.Type,
			UnifiedGPUIDs: d.UnifiedGPUIDs,
			NextActualGPU: d.nextActualGPUIndex,
			Properties:    d.Properties,
			Memory:        saveMemory(d.MemState),
		}
		for _, actual := range d.ActualGPUs {
			v.ActualGPUIDs = append(v.ActualGPUIDs, actual.ID)
		}
		s.Devices = append(s.Devices, v)
	}
	return s
}

// RestoreAllocator recreates shared device identities and buddy-block links.
func RestoreAllocator(s AllocatorState, table *vm.PageTable) (MemoryAllocator, []*Device, error) {
	a := NewMemoryAllocator(table, s.Log2PageSize).(*memoryAllocatorImpl)
	a.totalStorageByteSize = s.TotalStorageBytes
	for pid, next := range s.NextVAddr {
		a.processMemoryStates[pid] = &processMemoryState{pid: pid, nextVAddr: next}
	}
	devices := make([]*Device, len(s.Devices))
	for i, v := range s.Devices {
		m, err := loadMemory(v.Memory)
		if err != nil {
			return nil, nil, err
		}
		if _, exists := a.devices[v.ID]; exists {
			return nil, nil, fmt.Errorf("driver: duplicate device %d", v.ID)
		}
		d := &Device{
			ID:                 v.ID,
			Type:               v.Type,
			UnifiedGPUIDs:      v.UnifiedGPUIDs,
			nextActualGPUIndex: v.NextActualGPU,
			Properties:         v.Properties,
			MemState:           m,
		}
		devices[i] = d
		a.devices[d.ID] = d
	}
	for i, v := range s.Devices {
		for _, id := range v.ActualGPUIDs {
			d, ok := a.devices[id]
			if !ok {
				return nil, nil, fmt.Errorf("driver: missing unified GPU member %d", id)
			}
			devices[i].ActualGPUs = append(devices[i].ActualGPUs, d)
		}
	}
	return a, devices, nil
}

func pageRuns(pages []uint64, log2PageSize uint64) []PageRun {
	var runs []PageRun
	step := uint64(1) << log2PageSize
	for _, address := range pages {
		if n := len(runs); n > 0 {
			previous := &runs[n-1]
			if address >= previous.Start && address-previous.Start == previous.Count*step {
				previous.Count++
				continue
			}
		}
		runs = append(runs, PageRun{Start: address, Count: 1})
	}
	return runs
}

func expandPageRuns(s MemoryState) ([]uint64, error) {
	if s.Log2PageSize >= 64 {
		return nil, fmt.Errorf("allocator: invalid page size")
	}
	step := uint64(1) << s.Log2PageSize
	capacity := s.StorageSize / step
	if s.StorageSize%step != 0 {
		capacity++
	}
	var total uint64
	for _, run := range s.AvailableRuns {
		if run.Count == 0 || run.Count > capacity-total || run.Start < s.InitialAddress ||
			run.Start-s.InitialAddress >= s.StorageSize || (run.Start-s.InitialAddress)%step != 0 ||
			run.Count-1 > (s.StorageSize-1-(run.Start-s.InitialAddress))/step {
			return nil, fmt.Errorf("allocator: invalid free-page run")
		}
		total += run.Count
	}
	// Check intervals without a per-page map or changing allocation order.
	sorted := slices.Clone(s.AvailableRuns)
	slices.SortFunc(sorted, func(a, b PageRun) int {
		if a.Start < b.Start {
			return -1
		}
		if a.Start > b.Start {
			return 1
		}
		return 0
	})
	for i := 1; i < len(sorted); i++ {
		if sorted[i].Start-sorted[i-1].Start < sorted[i-1].Count*step {
			return nil, fmt.Errorf("allocator: overlapping free-page runs")
		}
	}
	pages := make([]uint64, 0, total)
	for _, run := range s.AvailableRuns {
		for i := uint64(0); i < run.Count; i++ {
			pages = append(pages, run.Start+i*step)
		}
	}
	return pages, nil
}
