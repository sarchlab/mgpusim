package internal

import "testing"

// A unified GPU hands out pages round-robin across its actual GPUs. The
// allocation must come from the first GPU at or after the next index that
// still has free pages, not from the last such GPU in the rotation.
func TestUnifiedGPUPageAllocationTakesFirstGPUInRotation(t *testing.T) {
	const log2PageSize = 12
	const pageSize = uint64(1) << log2PageSize

	gpu0 := &Device{
		ID:       1,
		Type:     DeviceTypeGPU,
		MemState: NewDeviceMemoryState(log2PageSize, AllocatorTypeDefault),
	}
	gpu0.SetTotalMemSize(4 * pageSize)
	gpu0.MemState.setInitialAddress(0x1000)

	gpu1 := &Device{
		ID:       2,
		Type:     DeviceTypeGPU,
		MemState: NewDeviceMemoryState(log2PageSize, AllocatorTypeDefault),
	}
	gpu1.SetTotalMemSize(4 * pageSize)
	gpu1.MemState.setInitialAddress(0x10_0000)

	unified := &Device{
		ID:         3,
		Type:       DeviceTypeUnifiedGPU,
		ActualGPUs: []*Device{gpu0, gpu1},
		MemState:   NewDeviceMemoryState(log2PageSize, AllocatorTypeDefault),
	}

	// The rotation starts at GPU 0, so the first page must come from GPU 0.
	got := unified.allocatePage()
	if got != 0x1000 {
		t.Fatalf("round-robin: want page 0x%x from first GPU, got 0x%x", 0x1000, got)
	}

	// Next the rotation has moved on to GPU 1.
	got = unified.allocatePage()
	if got != 0x10_0000 {
		t.Fatalf("round-robin: want page 0x%x from second GPU, got 0x%x", 0x10_0000, got)
	}
}
