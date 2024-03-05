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

	trace.fileHeader = readTraceHeader(scanner)
	trace.threadblocks = readThreadblocks(scanner)

	return trace
}

func readTraceHeader(scanner *bufio.Scanner) KernelFileHeader {
	header := KernelFileHeader{}

	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "-") {
			text := scanner.Text()
			elems := strings.Split(text, "=")
			if len(elems) != 2 {
				log.Panicf("Invalid trace header line: %s", text)
			}
			key := strings.TrimSpace(elems[0])
			value := strings.TrimSpace(elems[1])

			header.updateTraceHeaderParam(key[1:], value)
		} else if scanner.Text() != "" {
			break
		}
	}

	return header
}

const (
	threadblockBegin = "#BEGIN_TB"
	threadblockEnd   = "#END_TB"
)

func readThreadblocks(scanner *bufio.Scanner) []ThreadblockTrace {
	threadblockTraces := make([]ThreadblockTrace, 0)

	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == threadblockBegin {
			lines := make([]string, 0)
			for scanner.Scan() {
				if strings.TrimSpace(scanner.Text()) == threadblockEnd {
					tb := BuildThreadblock(lines)
					threadblockTraces = append(threadblockTraces, tb)
					break
				}
				lines = append(lines, scanner.Text())
			}
		}
	}

	return threadblockTraces
}

const warpPrefix = "warp"

func BuildThreadblock(lines []string) ThreadblockTrace {
	tb := ThreadblockTrace{}
	tb.ThreadblockDim = buildThreadblockDim(lines)

	for i, line := range lines {
		if strings.HasPrefix(line, warpPrefix) {
			wp := buildWarpHeader(lines[i], lines[i+1])
			for j := 0; j < int(wp.InstsCount); j++ {
				inst := buildInst(lines[i+2+j])
				wp.Instructions = append(wp.Instructions, inst)
			}

			tb.Warps = append(tb.Warps, wp)
		}
	}

	return tb
}

const threadblockDimPrefix = "thread block"

func buildThreadblockDim(lines []string) nvidia.Dim3 {
	for _, line := range lines {
		if strings.HasPrefix(line, threadblockDimPrefix) {
			d := nvidia.Dim3{}
			elems := strings.Split(line, "=")
			if len(elems) != 2 {
				log.Panicf("Invalid thread block dim line: %s", line)
			}

			value := strings.TrimSpace(elems[1])
			_, err := fmt.Sscanf(value, "%d,%d,%d", &d[0], &d[1], &d[2])
			if err != nil {
				log.Panicf("Invalid thread block dim value: %s", value)
			}

			return d
		}
	}
	return nvidia.Dim3{}
}

func buildWarpHeader(warpText string, instsText string) WarpTrace {
	wp := WarpTrace{}

	elems0 := strings.Split(warpText, "=")
	elems1 := strings.Split(instsText, "=")
	if len(elems0) != 2 || len(elems1) != 2 {
		log.Panicf("Invalid warp header: %s, %s", warpText, instsText)
	}

	_, err0 := fmt.Sscanf(strings.TrimSpace(elems0[1]), "%d", &wp.WarpID)
	_, err1 := fmt.Sscanf(strings.TrimSpace(elems1[1]), "%d", &wp.InstsCount)
	if err0 != nil || err1 != nil {
		log.Panicf("Invalid warp header: %s, %s", warpText, instsText)
	}

	return wp
}

func buildInst(line string) Instruction {
	inst := Instruction{}
	elems := strings.Fields(line)
	fmt.Sscanf(elems[0]+elems[1]+elems[2], "%x%x%d", &inst.PC, &inst.Mask, &inst.SrcNum)
	for i := 0; i < int(inst.SrcNum); i++ {
		inst.SrcRegs = append(inst.SrcRegs, nvidia.NewRegister(elems[3+i]))
	}

	fmt.Sscanf(elems[3+int(inst.SrcNum)], "%d", &inst.DestNum)
	for i := 0; i < int(inst.DestNum); i++ {
		inst.DestRegs = append(inst.DestRegs, nvidia.NewRegister(elems[4+int(inst.SrcNum)+i]))
	}

	updateInstMemoryPart(&inst, elems[4+int(inst.SrcNum)+int(inst.DestNum):])
	return inst
}

// [todo]: understand memory format
func updateInstMemoryPart(inst *Instruction, elems []string) {
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
