package driver

import (
	"reflect"

	"gitlab.com/akita/gcn3"

	"gitlab.com/akita/akita"
)

type KernelTimeCounter struct {
	TotalTime akita.VTimeInSec
}

func (c *KernelTimeCounter) Type() reflect.Type {
	return reflect.TypeOf((*gcn3.LaunchKernelReq)(nil))
}

func (c *KernelTimeCounter) Pos() akita.HookPos {
	return HookPosReqReturn
}

func (c *KernelTimeCounter) Func(item interface{}, domain akita.Hookable, info interface{}) {
	req := item.(*gcn3.LaunchKernelReq)

	c.TotalTime += req.EndTime - req.StartTime
}
