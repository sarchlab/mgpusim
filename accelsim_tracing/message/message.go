package message

import (
	"github.com/sarchlab/accelsimtracing/nvidia"
	"github.com/sarchlab/akita/v3/sim"
)

type DriverToDeviceMsg struct {
	sim.MsgMeta

	Kernel nvidia.Kernel
}

type DeviceToDriverMsg struct {
	sim.MsgMeta

	KernelFinished bool
	DeviceID       string
}

type DeviceToSMMsg struct {
	sim.MsgMeta

	Threadblock nvidia.Threadblock
}

type SMToDeviceMsg struct {
	sim.MsgMeta

	ThreadblockFinished bool
	SMID                string
}

type SMToSubcoreMsg struct {
	sim.MsgMeta

	Warp nvidia.Warp
}

type SubcoreToSMMsg struct {
	sim.MsgMeta

	WarpFinished bool
	SubcoreID    string
}

func (m *DriverToDeviceMsg) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

func (m *DeviceToDriverMsg) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

func (m *DeviceToSMMsg) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

func (m *SMToDeviceMsg) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

func (m *SMToSubcoreMsg) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

func (m *SubcoreToSMMsg) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}
