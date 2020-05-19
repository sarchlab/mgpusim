package internal

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
	allocatorTypeDefault AllocatorType = iota
	allocatorTypeBuddy
)

var MemoryAllocatorType AllocatorType = allocatorTypeDefault

func (at *AllocatorType) UseDefaultAllocator() {
	*at = allocatorTypeDefault
}

func (at *AllocatorType) UseBuddyAllocator() {
	*at = allocatorTypeBuddy
}

// A device is a CPU or GPU managed by the driver.
type Device struct {
	ID                 int
	Type               DeviceType
	UnifiedGPUIDs      []int
	ActualGPUs         []*Device
	nextActualGPUIndex int
	memState           deviceMemoryState
}


func (d *Device) SetTotalMemSize(size uint64) {
	if d.memState == nil {
		switch MemoryAllocatorType {
		case allocatorTypeDefault:
			d.memState = newDeviceRegularMemoryState()
		case allocatorTypeBuddy:
			d.memState = newDeviceBuddyMemoryState()
		}
	}
	d.memState.setStorageSize(size)
}

func (d *Device) allocatePage() (pAddr uint64) {
	if d.Type == DeviceTypeUnifiedGPU {
		return d.allocateUnifiedGPUPage()
	}

	d.mustHaveSpaceLeft()
	pAddr = d.memState.popNextAvailablePAddrs()

	return pAddr
}

func (d *Device) allocateMultiplePages(numPages int) (pAddrs []uint64) {
	if d.Type == DeviceTypeUnifiedGPU {
		return d.allocateMultipleUnifiedGPUPages(numPages)
	}
	d.mustHaveSpaceLeft()
	pAddrs = d.memState.allocateMultiplePages(numPages)

	return pAddrs
}

func (d *Device) mustHaveSpaceLeft() {
	if d.memState.noAvailablePAddrs() {
		panic("out of memory")
	}
}

func (d *Device) allocateUnifiedGPUPage() (pAddr uint64) {
	dev := d.ActualGPUs[d.nextActualGPUIndex]
	pAddr = dev.allocatePage()
	d.nextActualGPUIndex = (d.nextActualGPUIndex + 1) % len(d.ActualGPUs)
	return pAddr
}

func (d *Device) allocateMultipleUnifiedGPUPages(numPages int) (pAddrs []uint64) {
	dev := d.ActualGPUs[d.nextActualGPUIndex]
	pAddrs = dev.allocateMultiplePages(numPages)
	d.nextActualGPUIndex = (d.nextActualGPUIndex + 1) % len(d.ActualGPUs)
	return pAddrs
}
