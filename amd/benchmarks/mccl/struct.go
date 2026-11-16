package mccl

import (
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

type pushKernelArgs struct {
	Src                       gpuruntime.Ptr
	Dst                       gpuruntime.Ptr
	Size                      uint32
	NumThread                 uint32
	OffsetX, OffsetY, OffsetZ int64
}

type allReduceReduceKernelArgs struct {
	Buf                       gpuruntime.Ptr
	Store                     gpuruntime.Ptr
	Size                      uint32
	NumThread                 uint32
	GPUNum                    uint32
	Last                      uint32
	OffsetX, OffsetY, OffsetZ int64
}
