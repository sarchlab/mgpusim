package message

import (
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/v4/nvidia/trace"
)

type DriverToDeviceMsg struct {
	sim.MsgMeta

	Kernel trace.KernelTrace
}

type DeviceToDriverMsg struct {
	sim.MsgMeta

	KernelFinished bool
	DeviceID       string
}

type DeviceToSMMsg struct {
	sim.MsgMeta

	Threadblock trace.ThreadblockTrace
}

type SMToDeviceMsg struct {
	sim.MsgMeta

	ThreadblockFinished bool
	SMID                string
}

type SMToSMSPMsg struct {
	sim.MsgMeta

	Warp trace.WarpTrace
}

type SMSPToSMMsg struct {
	sim.MsgMeta

	WarpFinished bool
	SMSPID       string
}

type SMSPToSMMemReadMsg struct {
	sim.MsgMeta

	Address uint64
}

type SMSPToSMMemWriteMsg struct {
	sim.MsgMeta

	Address uint64
	Data    uint32
}

type SMToGPUMemReadMsg struct {
	sim.MsgMeta

	Address uint64
}

type SMToGPUMemWriteMsg struct {
	sim.MsgMeta

	Address uint64
	Data    uint32
}

type GPUtoSMMemReadMsg struct {
	sim.MsgMeta

	Address           uint64
	Rsp               mem.DataReadyRsp
	OriginalSMtoGPUID string
}

type GPUtoSMMemWriteMsg struct {
	sim.MsgMeta

	Address           uint64
	Rsp               mem.WriteDoneRsp
	OriginalSMtoGPUID string
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

func (m *SMToSMSPMsg) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

func (m *SMSPToSMMsg) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

func (m *SMSPToSMMemReadMsg) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

func (m *SMSPToSMMemWriteMsg) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

func (m *SMToGPUMemReadMsg) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

func (m *SMToGPUMemWriteMsg) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

func (m *GPUtoSMMemReadMsg) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

func (m *GPUtoSMMemWriteMsg) Meta() *sim.MsgMeta {
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

func (m *SMToSMSPMsg) Clone() sim.Msg {
	cloneMsg := *m
	cloneMsg.ID = sim.GetIDGenerator().Generate()
	return &cloneMsg
}

func (m *SMSPToSMMsg) Clone() sim.Msg {
	cloneMsg := *m
	cloneMsg.ID = sim.GetIDGenerator().Generate()
	return &cloneMsg
}

func (m *SMSPToSMMemReadMsg) Clone() sim.Msg {
	cloneMsg := *m
	cloneMsg.ID = sim.GetIDGenerator().Generate()
	return &cloneMsg
}

func (m *SMSPToSMMemWriteMsg) Clone() sim.Msg {
	cloneMsg := *m
	cloneMsg.ID = sim.GetIDGenerator().Generate()
	return &cloneMsg
}

func (m *SMToGPUMemReadMsg) Clone() sim.Msg {
	cloneMsg := *m
	cloneMsg.ID = sim.GetIDGenerator().Generate()
	return &cloneMsg
}

func (m *SMToGPUMemWriteMsg) Clone() sim.Msg {
	cloneMsg := *m
	cloneMsg.ID = sim.GetIDGenerator().Generate()
	return &cloneMsg
}

func (m *GPUtoSMMemReadMsg) Clone() sim.Msg {
	cloneMsg := *m
	cloneMsg.ID = sim.GetIDGenerator().Generate()
	return &cloneMsg
}

func (m *GPUtoSMMemWriteMsg) Clone() sim.Msg {
	cloneMsg := *m
	cloneMsg.ID = sim.GetIDGenerator().Generate()
	return &cloneMsg
}
