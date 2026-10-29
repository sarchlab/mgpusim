package internal

import (
	"fmt"

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
	Log2PageSize, InitialAddress, StorageSize, NextPAddr uint64
}

func saveMemory(m DeviceMemoryState) MemoryState {
	impl, ok := m.(*deviceMemoryStateImpl)
	if !ok {
		panic("driver: unknown device memory allocator")
	}

	return MemoryState{
		Log2PageSize:   impl.log2PageSize,
		InitialAddress: impl.initialAddress,
		StorageSize:    impl.storageSize,
		NextPAddr:      impl.nextPAddr,
	}
}

func loadMemory(s MemoryState) (DeviceMemoryState, error) {
	if s.Log2PageSize >= 64 || s.NextPAddr < s.InitialAddress ||
		s.NextPAddr > s.InitialAddress+s.StorageSize {
		return nil, fmt.Errorf("allocator: invalid memory state")
	}

	return &deviceMemoryStateImpl{
		log2PageSize:   s.Log2PageSize,
		initialAddress: s.InitialAddress,
		storageSize:    s.StorageSize,
		nextPAddr:      s.NextPAddr,
	}, nil
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

// RestoreAllocator recreates shared device identities.
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
