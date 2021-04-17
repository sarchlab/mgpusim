package mccl

import "gitlab.com/akita/mgpusim/v2/driver"

type pushKernelArgs struct {
	Src                       driver.GPUPtr
	Dst                       driver.GPUPtr
	Size                      uint32
	NumThread                 uint32
	OffsetX, OffsetY, OffsetZ int64
}

type allReduceReduceKernelArgs struct {
	Buf                       driver.GPUPtr
	Store                     driver.GPUPtr
	Size                      uint32
	NumThread                 uint32
	GPUNum                    uint32
	Last                      uint32
	OffsetX, OffsetY, OffsetZ int64
}
