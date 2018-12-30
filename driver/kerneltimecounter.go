package driver

import (
	"reflect"

	"gitlab.com/akita/gcn3"

	"gitlab.com/akita/akita"
)

// KernelTimeCounter is a hook to the driver that can calculate the
// amount of time spent on kernel execution.
type KernelTimeCounter struct {
	TotalTime akita.VTimeInSec
}

// Type specifies the type it hooks to.
func (c *KernelTimeCounter) Type() reflect.Type {
	return reflect.TypeOf((*gcn3.LaunchKernelReq)(nil))
}

// Pos specifies the position it hooks to.
func (c *KernelTimeCounter) Pos() akita.HookPos {
	return HookPosCommandComplete
}

// Func calculates the time spent on kernel execution.
func (c *KernelTimeCounter) Func(item interface{}, domain akita.Hookable, info interface{}) {
	// req := item.(*gcn3.LaunchKernelReq)

	// c.TotalTime += req.EndTime - req.StartTime
}
