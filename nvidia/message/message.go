package message

import (
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/v4/nvidia/nvidiaconfig"
)

type DriverToDeviceMsg struct {
	sim.MsgMeta

	Kernel nvidiaconfig.Kernel
}

type DeviceToDriverMsg struct {
	sim.MsgMeta

	KernelFinished bool
	DeviceID       string
}

type DeviceToSMMsg struct {
	sim.MsgMeta

	Threadblock nvidiaconfig.Threadblock
}

type SMToDeviceMsg struct {
	sim.MsgMeta

	ThreadblockFinished bool
	SMID                string
}

type SMToSubcoreMsg struct {
	sim.MsgMeta

	Warp nvidiaconfig.Warp
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

func (m *DriverToDeviceMsg) Clone() sim.Msg {
	cloneMsg := *m
	cloneMsg.ID = sim.GetIDGenerator().Generate()
	return &cloneMsg
}

func (m *DeviceToDriverMsg) Clone() sim.Msg {
	cloneMsg := *m
	cloneMsg.ID = sim.GetIDGenerator().Generate()
	return &cloneMsg
}

func (m *DeviceToSMMsg) Clone() sim.Msg {
	cloneMsg := *m
	cloneMsg.ID = sim.GetIDGenerator().Generate()
	return &cloneMsg
}

func (m *SMToDeviceMsg) Clone() sim.Msg {
	cloneMsg := *m
	cloneMsg.ID = sim.GetIDGenerator().Generate()
	return &cloneMsg
}

func (m *SMToSubcoreMsg) Clone() sim.Msg {
	cloneMsg := *m
	cloneMsg.ID = sim.GetIDGenerator().Generate()
	return &cloneMsg
}

func (m *SubcoreToSMMsg) Clone() sim.Msg {
	cloneMsg := *m
	cloneMsg.ID = sim.GetIDGenerator().Generate()
	return &cloneMsg
}
