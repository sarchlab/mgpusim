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
		d.memState = newDeviceRegularMemoryState(size)
		return
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
