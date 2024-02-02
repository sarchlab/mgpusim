package trace

import (
	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/nvidia"
)

func (kl *kernelList) convertToNV() *nvidia.KernelList {
	nvkl := &nvidia.KernelList{
		TraceExecs: make([]*nvidia.TraceExec, 0),
	}

	for _, tg := range kl.traceExecs {
		t := tg.execType()
		f := ""
		if t == "kernel" {
			f = tg.(*kernel).fileName
		}
		nvkl.TraceExecs = append(nvkl.TraceExecs, &nvidia.TraceExec{
			Type:     t,
			FilePath: kl.listDirPath + "/" + f,
		})
	}

	return nvkl
}

func (tg *traceGroup) convertToNV() *nvidia.TraceGroup {
	nvtg := &nvidia.TraceGroup{
		Header: tg.header.convertToNV(),
	}

	for _, tb := range tg.threadBlocks {
		nvtg.ThreadBlocks = append(nvtg.ThreadBlocks, tb.convertToNV())
	}

	return nvtg
}

func (th *traceGroupHeader) convertToNV() *nvidia.TraceGroupHeader {
	nvth := &nvidia.TraceGroupHeader{
		KernelName:            th.kernelName,
		KernelID:              th.kernelID,
		GridDim:               th.gridDim,
		BlockDim:              th.blockDim,
		Shmem:                 th.shmem,
		Nregs:                 th.nregs,
		BinaryVersion:         th.binaryVersion,
		CudaStreamID:          th.cudaStreamID,
		ShmemBaseAddr:         th.shmemBaseAddr,
		LocalMemBaseAddr:      th.localMemBaseAddr,
		NvbitVersion:          th.nvbitVersion,
		AccelsimTracerVersion: th.accelsimTracerVersion,
	}

	return nvth
}

func (tb *threadBlock) convertToNV() *nvidia.ThreadBlock {
	nvtb := &nvidia.ThreadBlock{
		WarpNum: len(tb.warps),
	}

	for _, wp := range tb.warps {
		nvtb.Warps = append(nvtb.Warps, wp.convertToNV())
	}

	return nvtb
}

func (wp *warp) convertToNV() *nvidia.Warp {
	nvwp := &nvidia.Warp{
		InstNum: len(wp.instructions),
	}

	for _, inst := range wp.instructions {
		nvwp.Insts = append(nvwp.Insts, inst.convertToNV())
	}

	return nvwp
}

func (inst *instruction) convertToNV() *nvidia.Instruction {
	nvinst := &nvidia.Instruction{
		PC:                inst.PC,
		Mask:              inst.Mask,
		DestNum:           inst.DestNum,
		DestRegs:          inst.DestRegs,
		OpCode:            inst.OpCode,
		SrcNum:            inst.SrcNum,
		SrcRegs:           inst.SrcRegs,
		MemWidth:          inst.MemWidth,
		AddressCompress:   inst.AddressCompress,
		MemAddress:        inst.MemAddress,
		MemAddressSuffix1: inst.MemAddressSuffix1,
		MemAddressSuffix2: inst.MemAddressSuffix2,
	}

	return nvinst
}
