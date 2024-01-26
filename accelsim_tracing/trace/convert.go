package trace

import "github.com/sarchlab/mgpusim/v3/accelsim_tracing/nvidia"

func (tb *ThreadBlock) GenerateNVThreadBlock() *nvidia.ThreadBlock {
	nvtb := &nvidia.ThreadBlock{
		WarpNum: len(tb.warps),
	}
	for _, wp := range tb.warps {
		nvtb.Warps = append(nvtb.Warps, wp.generateNVWarp())
	}
	return nvtb
}

func (wp *warp) generateNVWarp() *nvidia.Warp {
	nvwp := &nvidia.Warp{
		InstNum: len(wp.instructions),
	}
	for _, inst := range wp.instructions {
		nvwp.Insts = append(nvwp.Insts, inst.generateNVInst())
	}
	return nvwp
}

func (inst *instruction) generateNVInst() *nvidia.Instruction {
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
