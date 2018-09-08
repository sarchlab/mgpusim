package gcn3

import "gitlab.com/akita/akita"

// FlushCommand requests the GPU to flush all the cache to the main memory
type FlushCommand struct {
	*akita.ReqBase
}

func NewFlushCommand(t akita.VTimeInSec, src *akita.Port, dst *akita.Port) *FlushCommand {
	cmd := new(FlushCommand)
	cmd.ReqBase = akita.NewReqBase()
	cmd.SetSendTime(t)
	cmd.SetSrc(src)
	cmd.SetDst(dst)
	return cmd
}
