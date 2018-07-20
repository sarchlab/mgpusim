package gcn3

import "gitlab.com/yaotsu/core"

// FlushCommand requests the GPU to flush all the cache to the main memory
type FlushCommand struct {
	*core.ReqBase
}

func NewFlushCommand(t core.VTimeInSec, src *core.Port, dst *core.Port) *FlushCommand {
	cmd := new(FlushCommand)
	cmd.ReqBase = core.NewReqBase()
	cmd.SetSendTime(t)
	cmd.SetSrc(src)
	cmd.SetDst(dst)
	return cmd
}
