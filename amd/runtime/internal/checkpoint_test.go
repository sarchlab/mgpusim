package internal

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim/modeling"
)

func TestAllocatorCheckpointPreservesAllocation(t *testing.T) {
	table, original, devices := checkpointAllocator()
	block := original.Allocate(1, 3*4096, 1)
	original.Allocate(1, 5*4096, 2)
	original.Allocate(1, 4096, 3)
	original.Free(1, block, 4096)
	var data bytes.Buffer
	if err := modeling.WriteCheckpoint(&data, SnapshotAllocator(original, devices), nil); err != nil {
		t.Fatal(err)
	}
	var saved AllocatorState
	if err := modeling.ReadCheckpoint(&data, &saved, nil); err != nil {
		t.Fatal(err)
	}
	restoredTable := vm.NewPageTable(12)
	var tableData bytes.Buffer
	if err := table.SaveCheckpoint(&tableData); err != nil {
		t.Fatal(err)
	}
	if err := restoredTable.LoadCheckpoint(&tableData); err != nil {
		t.Fatal(err)
	}
	restored, restoredDevices, err := RestoreAllocator(saved, restoredTable)
	if err != nil {
		t.Fatal(err)
	}
	if restoredDevices[3].ActualGPUs[0] != restoredDevices[1] {
		t.Fatal("unified GPU lost shared device identity")
	}
	for _, allocator := range []MemoryAllocator{original, restored} {
		allocator.Free(1, block+4096, 4096)
		allocator.Free(1, block+8192, 4096)
	}
	for _, request := range []struct {
		size   uint64
		device int
	}{{7 * 4096, 1}, {4096, 3}, {2 * 4096, 2}, {4096, 3}} {
		want := original.Allocate(1, request.size, request.device)
		got := restored.Allocate(1, request.size, request.device)
		if got != want {
			t.Fatalf("virtual allocation changed: %x != %x", got, want)
		}
		for offset := uint64(0); offset < request.size; offset += 4096 {
			a, aok := table.Find(1, want+offset)
			b, bok := restoredTable.Find(1, got+offset)
			if !aok || !bok || a != b {
				t.Fatalf("physical placement changed: %+v != %+v", a, b)
			}
		}
	}
	if !reflect.DeepEqual(SnapshotAllocator(original, devices), SnapshotAllocator(restored, restoredDevices)) {
		t.Fatal("allocator metadata diverged after continuation")
	}
}

func checkpointAllocator() (*vm.PageTable, MemoryAllocator, []*Device) {
	table := vm.NewPageTable(12)
	original := NewMemoryAllocator(table, 12)
	var devices []*Device
	for id := 0; id < 3; id++ {
		memory := NewDeviceMemoryState(12)
		d := &Device{
			ID:         id,
			Type:       DeviceTypeGPU,
			MemState:   memory,
			Properties: DeviceProperties{DRAMSize: 1 << 20},
		}
		d.SetTotalMemSize(1 << 20)
		original.RegisterDevice(d)
		devices = append(devices, d)
	}
	unified := &Device{
		ID:            3,
		Type:          DeviceTypeUnifiedGPU,
		MemState:      NewDeviceMemoryState(12),
		UnifiedGPUIDs: []int{1, 2},
		ActualGPUs:    []*Device{devices[1], devices[2]},
	}
	original.RegisterDevice(unified)
	devices = append(devices, unified)
	return table, original, devices
}

func TestAllocatorNeverReusesAFreedPage(t *testing.T) {
	table, allocator, _ := checkpointAllocator()
	first := allocator.Allocate(1, 4096, 1)
	page, _ := table.Find(1, first)
	allocator.Free(1, first, 4096)

	second := allocator.Allocate(1, 4096, 1)
	got, _ := table.Find(1, second)
	if got.PAddr <= page.PAddr {
		t.Fatalf("freed page %#x reused or address decreased: %#x", page.PAddr, got.PAddr)
	}
}

func TestMemoryStatePreservesPartialLastPage(t *testing.T) {
	memory := &deviceMemoryStateImpl{log2PageSize: 12, storageSize: 4097}
	memory.setInitialAddress(17)
	restored, err := loadMemory(saveMemory(memory))
	if err != nil {
		t.Fatal(err)
	}
	for !memory.noAvailablePAddrs() {
		if got, want := restored.popNextAvailablePAddrs(), memory.popNextAvailablePAddrs(); got != want {
			t.Fatalf("page order: %d != %d", got, want)
		}
	}
	if !restored.noAvailablePAddrs() {
		t.Fatal("extra restored pages")
	}
}
