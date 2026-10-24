package message

import (
	"github.com/sarchlab/akita/v5/sim/messaging"

	"github.com/sarchlab/mgpusim/v5/nvidia/trace"
)

// DriverToDeviceMsg asks a GPU to run a kernel.
type DriverToDeviceMsg struct {
	Kernel trace.KernelTrace
}

// DeviceToDriverMsg reports that a GPU has finished a kernel.
type DeviceToDriverMsg struct {
	KernelFinished bool
	DeviceID       string
}

// DeviceToSMMsg dispatches a thread block to an SM.
type DeviceToSMMsg struct {
	Threadblock trace.ThreadblockTrace
}

// SMToDeviceMsg reports that an SM has finished a thread block.
type SMToDeviceMsg struct {
	NumThreadFinished uint64
	SMID              string
}

// SMToSMSPMsg dispatches warps to an SM sub-partition.
type SMToSMSPMsg struct {
	WarpList []trace.WarpTrace
}

// SMSPToSMMsg reports that an SM sub-partition has finished a warp.
type SMSPToSMMsg struct {
	WarpFinished bool
	Warp         trace.WarpTrace
	SMSPID       string
}

// Protocol registers the values carried by each controller boundary.
var Protocol = messaging.DefineProtocol(
	messaging.RoleDef{Name: "driver", Sends: []any{DriverToDeviceMsg{}}},
	messaging.RoleDef{Name: "device", Sends: []any{DeviceToDriverMsg{}, DeviceToSMMsg{}}},
	messaging.RoleDef{Name: "sm", Sends: []any{SMToDeviceMsg{}, SMToSMSPMsg{}}},
	messaging.RoleDef{Name: "smsp", Sends: []any{SMSPToSMMsg{}}},
)
