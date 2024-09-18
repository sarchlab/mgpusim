package tracereader

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/sarchlab/mgpusim/v3/accelsim_tracing/nvidia"
)

type TraceReader struct {
	directoryPath string

	execMetas []TraceExecMeta
}

func (r *TraceReader) GetExecMetas() []TraceExecMeta {
	return r.execMetas
}

type TraceExecMeta struct {
	execType nvidia.ExecType

	// exec kernel
	filename string
	filepath string

	// exec memcpy
	Direction nvidia.ExecMemcpyDirection
	Address   uint64
	Length    uint64
}

func (m *TraceExecMeta) ExecType() nvidia.ExecType {
	return m.execType
}

func ReadTrace(meta TraceExecMeta) KernelTrace {
	if meta.execType != nvidia.ExecKernel {
		log.Panicf("Invalid exec type: %d", meta.ExecType())
	}

	file, err := os.Open(meta.filepath)
	if err != nil {
		log.Panic(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	trace := KernelTrace{}

	trace.readTraceHeader(scanner)
	trace.readThreadblocks(scanner)

	return trace
}

func (t *KernelTrace) readTraceHeader(scanner *bufio.Scanner) {
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "-") {
			text := scanner.Text()
			elems := strings.Split(text, "=")
			if len(elems) != 2 {
				log.Panicf("Invalid trace header line: %s", text)
			}
			key := strings.TrimSpace(elems[0])
			value := strings.TrimSpace(elems[1])

			t.FileHeader.updateTraceHeaderParam(key[1:], value)
		} else if scanner.Text() != "" {
			if strings.HasPrefix(scanner.Text(), "#traces format") {
				break
			}
			log.Panic("extract failed due to missing #traces format line")
		}
	}
}

func (t *KernelTrace) readThreadblocks(scanner *bufio.Scanner) {
	t.tbIDToIndex = make(map[nvidia.Dim3]int32)
	t.threadblocks = make([]*ThreadblockTrace, 0)

	for scanner.Scan() {
		if scanner.Text() == "" {
			continue
		}

		inst := extractInst(scanner.Text())
		if tbIndex, exists := t.tbIDToIndex[inst.threadblockID]; exists {
			t.threadblocks[tbIndex].updateInstruction(inst)
		} else {
			tb := &ThreadblockTrace{
				id:            inst.threadblockID,
				Warps:         make([]*WarpTrace, 0),
				warpIDToIndex: make(map[int32]int32),
			}

			tb.updateInstruction(inst)
			t.threadblocks = append(t.threadblocks, tb)
			t.tbIDToIndex[inst.threadblockID] = int32(t.ThreadblocksCount() - 1)
		}
	}
}

func (tb *ThreadblockTrace) updateInstruction(inst Instruction) {
	if warpIndex, exists := tb.warpIDToIndex[inst.warpID]; exists {
		tb.Warps[warpIndex].updateInstruction(inst)
	} else {
		warp := &WarpTrace{
			id:           inst.warpID,
			InstsCount:   0,
			Instructions: make([]Instruction, 0),
		}

		warp.updateInstruction(inst)
		tb.Warps = append(tb.Warps, warp)
		tb.warpIDToIndex[inst.warpID] = int32(tb.WarpsCount() - 1)
	}
}

func (wp *WarpTrace) updateInstruction(inst Instruction) {
	wp.Instructions = append(wp.Instructions, inst)
	wp.InstsCount++
}

// #traces format = [line_num] PC mask dest_num [reg_dests] opcode src_num [reg_srcs] mem_width [adrrescompress?] [mem_addresses] immediate
func extractInst(line string) Instruction {
	inst := Instruction{}
	elems := strings.Fields(line)

	fmt.Sscanf(elems[0]+elems[1]+elems[2], "%d%d%d", &inst.threadblockID[0], &inst.threadblockID[1], &inst.threadblockID[2])
	fmt.Sscanf(elems[3], "%d", &inst.warpID)

	fmt.Sscanf(elems[4]+elems[5]+elems[6], "%x%x%d", &inst.PC, &inst.Mask, &inst.SrcNum)
	for i := 0; i < int(inst.SrcNum); i++ {
		inst.SrcRegs = append(inst.SrcRegs, nvidia.NewRegister(elems[6+i]))
	}

	// inst.OpCode = nvidia.NewOpcode(elems[6+int(inst.SrcNum)])

	fmt.Sscanf(elems[7+int(inst.SrcNum)], "%d", &inst.DestNum)
	for i := 0; i < int(inst.DestNum); i++ {
		inst.DestRegs = append(inst.DestRegs, nvidia.NewRegister(elems[7+int(inst.SrcNum)+i]))
	}

	updateInstMemoryPart(&inst, elems[7+int(inst.SrcNum)+int(inst.DestNum):])
	return inst
}

// [todo]: understand memory format
func updateInstMemoryPart(inst *Instruction, elems []string) {
	fmt.Sscanf(elems[0], "%d", &inst.MemWidth)
	if inst.MemWidth != 0 {
		fmt.Sscanf(elems[1]+elems[2], "%d0x%x", &inst.AddressCompress, &inst.MemAddress)
		switch inst.AddressCompress {
		case 1:
			fmt.Sscanf(elems[2], "%d", &inst.MemAddressSuffix1)
		case 2:
			for _, s := range elems[2 : len(elems)-1] {
				s32, _ := strconv.Atoi(s)
				inst.MemAddressSuffix2 = append(inst.MemAddressSuffix2, int32(s32))
			}
		}
	}
	imm, _ := strconv.Atoi(elems[len(elems)-1])
	inst.Immediate = int64(imm)
}
