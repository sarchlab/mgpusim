package mccl

import "github.com/sarchlab/mgpusim/v4/amd/driver"

type pushKernelArgs struct {
	Src                       driver.Ptr
	Dst                       driver.Ptr
	Size                      uint32
	NumThread                 uint32
	OffsetX, OffsetY, OffsetZ int64
}

type allReduceReduceKernelArgs struct {
	Buf                       driver.Ptr
	Store                     driver.Ptr
	Size                      uint32
	NumThread                 uint32
	GPUNum                    uint32
	Last                      uint32
	OffsetX, OffsetY, OffsetZ int64
}
