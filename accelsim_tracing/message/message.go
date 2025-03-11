package message

import (
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/nvidia_v4/accelsim_tracing/nvidia"
)

type DriverToDeviceMsg struct {
	sim.MsgMeta

	Kernel nvidia.Kernel
}

// v3 not included
func (s *DriverToDeviceMsg) Clone() sim.Msg {
    cloneMsg := *s
    cloneMsg.ID = sim.GetIDGenerator().Generate()
    return &cloneMsg
}

type DeviceToDriverMsg struct {
	sim.MsgMeta

	KernelFinished bool
	DeviceID       string
}

// v3 not included
func (s *DeviceToDriverMsg) Clone() sim.Msg {
    cloneMsg := *s
    cloneMsg.ID = sim.GetIDGenerator().Generate()
    return &cloneMsg
}

type DeviceToSMMsg struct {
	sim.MsgMeta

	Threadblock nvidia.Threadblock
}

// v3 not included
func (s *DeviceToSMMsg) Clone() sim.Msg {
    cloneMsg := *s
    cloneMsg.ID = sim.GetIDGenerator().Generate()
    return &cloneMsg
}

type SMToDeviceMsg struct {
	sim.MsgMeta

	ThreadblockFinished bool
	SMID                string
}

// v3 not included
func (s *SMToDeviceMsg) Clone() sim.Msg {
    cloneMsg := *s
    cloneMsg.ID = sim.GetIDGenerator().Generate()
    return &cloneMsg
}

type SMToSubcoreMsg struct {
	sim.MsgMeta

	Warp nvidia.Warp
}

// v3 not included
func (s *SMToSubcoreMsg) Clone() sim.Msg {
    cloneMsg := *s
    cloneMsg.ID = sim.GetIDGenerator().Generate()
    return &cloneMsg
}

type SubcoreToSMMsg struct {
	sim.MsgMeta

	WarpFinished bool
	SubcoreID    string
}

// v3 not included
func (s *SubcoreToSMMsg) Clone() sim.Msg {
    cloneMsg := *s
    cloneMsg.ID = sim.GetIDGenerator().Generate()
    return &cloneMsg
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
