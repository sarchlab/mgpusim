package protocol

import "github.com/sarchlab/akita/v5/sim/messaging"

// Protocol registers AMD GPU control and work-dispatch payloads for checkpoints.
var Protocol = messaging.DefineProtocol(
	messaging.RoleDef{
		Name: "driver",
		Sends: []any{
			FlushReq{},
			LaunchKernelReq{},
			MemCopyH2DReq{},
			MemCopyD2HReq{},
			ShootDownCommand{},
			RDMADrainCmdFromDriver{},
			RDMARestartCmdFromDriver{},
			GPURestartReq{},
		},
	},
	messaging.RoleDef{
		Name: "cp",
		Sends: []any{
			LaunchKernelRsp{},
			GeneralRsp{},
			MemCopyD2HRsp{},
			ShootDownCompleteRsp{},
			RDMADrainRspToDriver{},
			RDMARestartRspToDriver{},
			GPURestartRsp{},
			MapWGReq{},
			CUPipelineFlushReq{},
			CUPipelineRestartReq{},
			MemCopyH2DReq{},
			MemCopyD2HReq{},
		},
	},
	messaging.RoleDef{Name: "cu", Sends: []any{WGCompletionMsg{}, CUPipelineFlushRsp{}, CUPipelineRestartRsp{}}},
	messaging.RoleDef{Name: "dma", Sends: []any{GeneralRsp{}, MemCopyD2HRsp{}}},
)
