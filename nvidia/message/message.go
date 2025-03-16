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

func (s *DriverToDeviceMsg) Clone() sim.Msg {
	cloneMsg := *s
	cloneMsg.ID = sim.GetIDGenerator().Generate()
	return &cloneMsg
}

func (s *DeviceToDriverMsg) Clone() sim.Msg {
	cloneMsg := *s
	cloneMsg.ID = sim.GetIDGenerator().Generate()
	return &cloneMsg
}

func (s *DeviceToSMMsg) Clone() sim.Msg {
	cloneMsg := *s
	cloneMsg.ID = sim.GetIDGenerator().Generate()
	return &cloneMsg
}

func (s *SMToDeviceMsg) Clone() sim.Msg {
	cloneMsg := *s
	cloneMsg.ID = sim.GetIDGenerator().Generate()
	return &cloneMsg
}

func (s *SMToSubcoreMsg) Clone() sim.Msg {
	cloneMsg := *s
	cloneMsg.ID = sim.GetIDGenerator().Generate()
	return &cloneMsg
}

func (s *SubcoreToSMMsg) Clone() sim.Msg {
	cloneMsg := *s
	cloneMsg.ID = sim.GetIDGenerator().Generate()
	return &cloneMsg
}
