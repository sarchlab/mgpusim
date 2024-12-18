package internal

import "github.com/sarchlab/akita/v3/mem/vm"

// DeviceType marks the type of a device.
type DeviceType int

// Defines supported devices.
const (
	DeviceTypeInvalid DeviceType = iota
	DeviceTypeCPU
	DeviceTypeGPU
	DeviceTypeUnifiedGPU
)

// AllocatorType marks the type of memory allocator
type AllocatorType int

// Defines supported allocation algorithms
const (
	AllocatorTypeDefault AllocatorType = iota
	AllocatorTypeBuddy
)

// MemoryAllocatorType global flag variable for setting the allocator type
var MemoryAllocatorType = AllocatorTypeDefault

// DeviceProperties defines the properties of a device
type DeviceProperties struct {
	CUCount  int
	DRAMSize uint64
}

// Device is a CPU or GPU managed by the driver.
type Device struct {
	ID                 int
	Type               DeviceType
	UnifiedGPUIDs      []int
	ActualGPUs         []*Device
	nextActualGPUIndex int
	MemState           DeviceMemoryState
	Properties         DeviceProperties
	PageTable          vm.PageTable
}

// SetTotalMemSize sets total memory size
func (d *Device) SetTotalMemSize(size uint64) {
	d.MemState.setStorageSize(size)
}

func (d *Device) allocatePage() (pAddr uint64) {
	if d.Type == DeviceTypeUnifiedGPU {
		return d.allocateUnifiedGPUPage()
	}

	d.mustHaveSpaceLeft()
	pAddr = d.MemState.popNextAvailablePAddrs()

	return pAddr
}

func (d *Device) allocateMultiplePages(numPages int) (pAddrs []uint64) {
	if d.Type == DeviceTypeUnifiedGPU {
		return d.allocateMultipleUnifiedGPUPages(numPages)
	}

	d.mustHaveSpaceLeft()
	pAddrs = d.MemState.allocateMultiplePages(numPages)

	return pAddrs
}

func (d *Device) mustHaveSpaceLeft() {
	if d.MemState.noAvailablePAddrs() {
		panic("out of memory")
	}
}

func (d *Device) allocateUnifiedGPUPage() (pAddr uint64) {
	var devSelected *Device

	devSelected = nil
	for i := 0; i < len(d.ActualGPUs); i++ {
		devIndex := (d.nextActualGPUIndex + i) % len(d.ActualGPUs)
		dev := d.ActualGPUs[devIndex]

		if dev.MemState.noAvailablePAddrs() {
			continue
		}

		devSelected = dev
	}

	if devSelected == nil {
		panic("out of memory")
	}

	pAddr = devSelected.allocatePage()
	d.nextActualGPUIndex = (d.nextActualGPUIndex + 1) % len(d.ActualGPUs)
	return pAddr
}

func (d *Device) allocateMultipleUnifiedGPUPages(numPages int) (pAddrs []uint64) {
	dev := d.ActualGPUs[d.nextActualGPUIndex]
	pAddrs = dev.allocateMultiplePages(numPages)
	d.nextActualGPUIndex = (d.nextActualGPUIndex + 1) % len(d.ActualGPUs)
	return pAddrs
}
