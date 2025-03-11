package tracereader

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/sarchlab/mgpusim/nvidia_v4/accelsim_tracing/nvidia"
	log "github.com/sirupsen/logrus"
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

var kernelScanner *bufio.Scanner

func ReadTrace(meta TraceExecMeta) KernelTrace {
	if meta.execType != nvidia.ExecKernel {
		log.WithField("execType", meta.ExecType()).Panic("Invalid exec type")
	}

	file, err := os.Open(meta.filepath)
	if err != nil {
		log.WithError(err).WithField("filepath", meta.filepath).Error("Failed to open file")
	}
	defer file.Close()

	kernelScanner = bufio.NewScanner(file)
	trace := KernelTrace{}

	trace.readTraceHeader()
	trace.readThreadblocks()

	return trace
}

func moveScannerToNextLine() bool {
	for kernelScanner.Scan() {
		if kernelScanner.Text() != "" {
			return true
		}
	}
	return false
}
func goToNextlineWithPrefixIncludingNow(prefix string) bool {
	line := kernelScanner.Text()
	if strings.HasPrefix(line, prefix) {
		return true
	}
	for moveScannerToNextLine() {
		line = kernelScanner.Text()
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

func (t *KernelTrace) readTraceHeader() {
	for moveScannerToNextLine() {
		text := kernelScanner.Text()
		if strings.HasPrefix(text, "-") {
			elems := strings.Split(text, "=")
			key := strings.TrimSpace(elems[0])[1:]
			value := strings.TrimSpace(elems[1])
			t.FileHeader.updateTraceHeaderParam(key, value)
		} else {
			break
		}
	}
}

func (t *KernelTrace) readThreadblocks() {
	t.tbIDToIndex = make(map[nvidia.Dim3]int32)
	t.threadblocks = make([]*ThreadblockTrace, 0)

	for goToNextlineWithPrefixIncludingNow("thread block") {
		tb := &ThreadblockTrace{}
		fmt.Sscanf(kernelScanner.Text(), "thread block = %d,%d,%d", &tb.id[0], &tb.id[1], &tb.id[2])

		for moveScannerToNextLine() {
			if strings.HasPrefix(kernelScanner.Text(), "warp") {
				wp := &WarpTrace{}
				fmt.Sscanf(kernelScanner.Text(), "warp = %d", &wp.id)

				if !goToNextlineWithPrefixIncludingNow("insts") {
					log.WithField("line", kernelScanner.Text()).Panic("Cannot find insts line")
				}
				fmt.Sscanf(kernelScanner.Text(), "insts = %d", &wp.InstsCount)

				for j := 0; j < int(wp.InstsCount); j++ {
					moveScannerToNextLine()
					inst := extractInst(kernelScanner.Text())
					inst.threadblockID = tb.id
					inst.warpID = wp.id

					wp.Instructions = append(wp.Instructions, inst)
				}

				tb.Warps = append(tb.Warps, wp)
			} else {
				break
			}
		}

		t.threadblocks = append(t.threadblocks, tb)
		t.tbIDToIndex[tb.id] = int32(t.ThreadblocksCount() - 1)
	}
}

/*
	 #traces format = [line_num] PC mask dest_num [reg_dests] opcode src_num [reg_srcs] mem_width [adrrescompress?] [mem_addresses] immediate
								  0	 1		2					3		4					5		6									7
*/
func extractInst(text string) *Instruction {
	inst := &Instruction{}
	elems := strings.Fields(text)

	fmt.Sscanf(elems[0], "%x", &inst.PC)
	fmt.Sscanf(elems[1], "%x", &inst.Mask)

	fmt.Sscanf(elems[2], "%d", &inst.DestNum)
	for i := 0; i < int(inst.DestNum); i++ {
		inst.DestRegs = append(inst.DestRegs, nvidia.NewRegister(elems[2+i+1]))
	}

	//inst.OpCode = nvidia.NewOpcode(elems[3+int(inst.DestNum)])

	fmt.Sscanf(elems[4+int(inst.DestNum)], "%d", &inst.SrcNum)
	for i := 0; i < int(inst.SrcNum); i++ {
		inst.SrcRegs = append(inst.SrcRegs, nvidia.NewRegister(elems[4+int(inst.DestNum)+i+1]))
	}

	updateInstMemoryPart(inst, elems[5+int(inst.DestNum)+int(inst.SrcNum):])
	return inst
}

// [todo]: understand memory format
func updateInstMemoryPart(inst *Instruction, elems []string) {
	fmt.Sscanf(elems[0], "%d", &inst.MemWidth)

	if inst.MemWidth != 0 {
		fmt.Sscanf(elems[1], "%d", &inst.AddressCompress)
		fmt.Sscanf(elems[2], "%x", &inst.MemAddress)

		if inst.AddressCompress == 1 {
			fmt.Sscanf(elems[3], "%d", &inst.MemAddressSuffix1)
		} else if inst.AddressCompress == 2 {
			for _, s := range elems[3 : len(elems)-1] {
				s32, _ := strconv.Atoi(s)
				inst.MemAddressSuffix2 = append(inst.MemAddressSuffix2, int32(s32))
			}
		}
	}

	imm, _ := strconv.Atoi(elems[len(elems)-1])
	inst.Immediate = int64(imm)
}
