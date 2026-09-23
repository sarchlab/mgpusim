package message

import (
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"

	"github.com/sarchlab/mgpusim/v5/nvidia/trace"
)

// NewMeta creates the metadata of a new message from src to dst.
func NewMeta(src, dst messaging.RemotePort) messaging.MsgMeta {
	return messaging.MsgMeta{
		ID:  timing.GetIDGenerator().Generate(),
		Src: src,
		Dst: dst,
	}
}

// DriverToDeviceMsg asks a GPU to run a kernel.
type DriverToDeviceMsg struct {
	messaging.MsgMeta

	Kernel *trace.KernelTrace
}

// DeviceToDriverMsg reports that a GPU has finished a kernel.
type DeviceToDriverMsg struct {
	messaging.MsgMeta

	KernelFinished bool
	DeviceID       string
}

// DeviceToSMMsg dispatches a thread block to an SM.
type DeviceToSMMsg struct {
	messaging.MsgMeta

	Threadblock *trace.ThreadblockTrace
}

// SMToDeviceMsg reports that an SM has finished a thread block.
type SMToDeviceMsg struct {
	messaging.MsgMeta

	NumThreadFinished uint64
	SMID              string
}

// SMToSMSPMsg dispatches warps to an SM sub-partition.
type SMToSMSPMsg struct {
	messaging.MsgMeta

	WarpList []*trace.WarpTrace
}

// SMSPToSMMsg reports that an SM sub-partition has finished a warp.
type SMSPToSMMsg struct {
	messaging.MsgMeta

	WarpFinished bool
	Warp         *trace.WarpTrace
	SMSPID       string
}
