package internal

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/sarchlab/akita/v5/sim/modeling"

	"github.com/sarchlab/akita/v5/mem/vm"
)

func TestAllocatorCheckpointPreservesAllocationAndFreeOrder(t *testing.T) {
	for _, buddy := range []bool{false, true} {
		name := "regular"
		if buddy {
			name = "buddy"
		}
		t.Run(name, func(t *testing.T) {
			checkAllocatorContinuation(t, buddy)
		})
	}
}

func checkAllocatorContinuation(t *testing.T, buddy bool) {
	t.Helper()
	table, original, devices := checkpointAllocator(buddy)
	block := original.Allocate(1, 3*4096, 1)
	original.Allocate(1, 5*4096, 2)
	original.Allocate(1, 4096, 3)
	original.Free(block) // Preserve a partly freed buddy block's shared tracker.
	var data bytes.Buffer
	if err := modeling.WriteCheckpoint(&data, struct{}{}, SnapshotAllocator(original, devices), nil); err != nil {
		t.Fatal(err)
	}
	var saved AllocatorState
	if err := modeling.ReadCheckpoint(&data, struct{}{}, &saved, nil); err != nil {
		t.Fatal(err)
	}
	restoredTable := vm.NewPageTable(12)
	for _, page := range saved.Pages {
		restoredTable.Insert(page)
	}
	restored, restoredDevices, err := RestoreAllocator(saved, restoredTable)
	if err != nil {
		t.Fatal(err)
	}
	if restoredDevices[3].ActualGPUs[0] != restoredDevices[1] {
		t.Fatal("unified GPU lost shared device identity")
	}
	for _, allocator := range []MemoryAllocator{original, restored} {
		allocator.Free(block + 4096)
		allocator.Free(block + 8192)
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

func checkpointAllocator(buddy bool) (*vm.PageTable, MemoryAllocator, []*Device) {
	table := vm.NewPageTable(12)
	original := NewMemoryAllocator(table, 12)
	var devices []*Device
	for id := 0; id < 3; id++ {
		var memory DeviceMemoryState = newDeviceRegularMemoryState(12)
		if buddy {
			memory = newDeviceBuddyMemoryState(12)
		}
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
		MemState:      newDeviceRegularMemoryState(12),
		UnifiedGPUIDs: []int{1, 2},
		ActualGPUs:    []*Device{devices[1], devices[2]},
	}
	original.RegisterDevice(unified)
	devices = append(devices, unified)
	return table, original, devices
}
