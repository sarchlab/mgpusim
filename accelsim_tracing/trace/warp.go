package trace

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/nvidia"
)

type warp struct {
	parent     *ThreadBlock
	rawContext struct {
		warpID     string
		instsCount string
	}

	warpID       int32
	instsCount   int32
	instructions []instruction
}

type instruction struct {
	parent  *warp
	rawText string

	PC                int32
	Mask              int64
	DestNum           int32
	DestRegs          []*nvidia.Register
	OpCode            *nvidia.Opcode
	SrcNum            int32
	SrcRegs           []*nvidia.Register
	MemWidth          int32
	AddressCompress   int32
	MemAddress        int64
	MemAddressSuffix1 int32
	MemAddressSuffix2 []int32
}

func parseWarp(lines []string) *warp {
	wp := &warp{}
	elems0 := strings.Split(lines[0], "=")
	elems1 := strings.Split(lines[1], "=")
	if len(elems0) != 2 || len(elems1) != 2 {
		log.Panicf("Invalid warp header: %s, %s", lines[0], lines[1])
	}

	wp.rawContext.warpID = lines[0]
	wp.rawContext.instsCount = lines[1]
	_, err0 := fmt.Sscanf(strings.TrimSpace(elems0[1]), "%d", &wp.warpID)
	_, err1 := fmt.Sscanf(strings.TrimSpace(elems1[1]), "%d", &wp.instsCount)
	if err0 != nil || err1 != nil {
		log.Panicf("Invalid warp header: %s, %s", lines[0], lines[1])
	}

	for i := 2; i < 2+int(wp.instsCount); i++ {
		inst := parseInst(lines[i])
		inst.parent = wp
		wp.instructions = append(wp.instructions, inst)
	}

	return wp
}

func parseInst(line string) instruction {
	inst := &instruction{}
	elems := strings.Fields(line)
	fmt.Sscanf(elems[0]+elems[1]+elems[2], "%x%x%d", &inst.PC, &inst.Mask, &inst.SrcNum)
	for i := 0; i < int(inst.SrcNum); i++ {
		inst.SrcRegs = append(inst.SrcRegs, nvidia.NewRegister(elems[3+i]))
	}

	fmt.Sscanf(elems[3+int(inst.SrcNum)], "%d", &inst.DestNum)
	for i := 0; i < int(inst.DestNum); i++ {
		inst.DestRegs = append(inst.DestRegs, nvidia.NewRegister(elems[4+int(inst.SrcNum)+i]))
	}

	inst.parseMemory(elems[4+int(inst.SrcNum)+int(inst.DestNum):])
	return *inst
}

// [todo]: understand memory format
func (inst *instruction) parseMemory(elems []string) {
	fmt.Sscanf(elems[0], "%d", &inst.MemWidth)
	if inst.MemWidth == 0 {
		return
	}
	
	fmt.Sscanf(elems[1]+elems[2], "%d0x%x", &inst.AddressCompress, &inst.MemAddress)
	switch inst.AddressCompress {
	case 1:
		fmt.Sscanf(elems[2], "%d", &inst.MemAddressSuffix1)
	case 2:
		for _, s := range elems[2:] {
			s32, _ := strconv.Atoi(s)
			inst.MemAddressSuffix2 = append(inst.MemAddressSuffix2, int32(s32))
		}
	}
}
