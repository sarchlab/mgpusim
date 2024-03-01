package msg

import (
	"github.com/sarchlab/accelsimtracing/benchmark"
	"github.com/sarchlab/akita/v3/sim"
)

// DriverToDeviceMsg: apply a new kernel to a device or answer a device request for more threadblocks
type DriverToDeviceMsg struct {
	sim.MsgMeta

	NewKernel   bool
	Threadblock benchmark.Threadblock
}

func (m *DriverToDeviceMsg) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

// DeviceToDriverMsg: report a finished threadblock or request more threadblocks
type DeviceToDriverMsg struct {
	sim.MsgMeta

	DeviceID            int64
	RequestMore         bool
	ThreadblockFinished bool
}

func (m *DeviceToDriverMsg) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

// DeviceToSubcoreMsg: apply a warp to a subcore
type DeviceToSubcoreMsg struct {
	sim.MsgMeta

	Warp benchmark.Warp
}

func (m *DeviceToSubcoreMsg) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

// SubcoreToDeviceMsg: report a finished warp
type SubcoreToDeviceMsg struct {
	sim.MsgMeta

	SubcoreID int64
}

func (m *SubcoreToDeviceMsg) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}
