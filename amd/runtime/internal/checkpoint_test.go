package internal

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim/modeling"
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
	original.Free(1, block, 4096) // Preserve a partly freed buddy block's shared tracker.
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

func TestFreePageRunsPreserveFragmentedQueue(t *testing.T) {
	const step = uint64(4096)
	pages := []uint64{8 * step, 9 * step, 10 * step, 2 * step, 6 * step, 7 * step, step}
	runs := pageRuns(pages, 12)
	if len(runs) != 4 {
		t.Fatalf("got %d runs: %v", len(runs), runs)
	}
	restored, err := expandPageRuns(MemoryState{Log2PageSize: 12, StorageSize: 16 * step, AvailableRuns: runs})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(pages, restored) {
		t.Fatalf("queue changed: %v", restored)
	}
	// Saving does not alias a live queue that can subsequently be reused.
	pages[0] = 0
	if runs[0].Start != 8*step {
		t.Fatal("snapshot aliases queue")
	}
}

func TestFreePageRunsRejectInvalidRanges(t *testing.T) {
	for _, run := range []PageRun{{0, 0}, {1, 1}, {0, 17}, {16 * 4096, 1}, {15 * 4096, 2}, {^uint64(0) - 4095, 2}} {
		_, err := expandPageRuns(MemoryState{Log2PageSize: 12, StorageSize: 16 * 4096, AvailableRuns: []PageRun{run}})
		if err == nil {
			t.Fatalf("accepted invalid run: %v", run)
		}
	}
}

func TestFreePageRunsCompactIdleDevice(t *testing.T) {
	pages := make([]uint64, 1<<20)
	for i := range pages {
		pages[i] = uint64(i) * 4096
	}
	runs := pageRuns(pages, 12)
	if !reflect.DeepEqual(runs, []PageRun{{0, 1 << 20}}) {
		t.Fatalf("idle device did not compact: %d runs", len(runs))
	}
}

func TestFreePageRunsRejectOverlap(t *testing.T) {
	for _, runs := range [][]PageRun{{{0, 2}, {4096, 2}}, {{0, 1}, {0, 1}}} {
		if _, err := expandPageRuns(MemoryState{Log2PageSize: 12, StorageSize: 16 * 4096, AvailableRuns: runs}); err == nil {
			t.Fatal("overlapping runs accepted")
		}
	}
}

func TestFreePageRunsPreservePartialLastPage(t *testing.T) {
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
