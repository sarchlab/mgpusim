package trace

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
)

// ExecType : Redefine necessary types from trace package
type ExecType int

const (
	ExecUndefined ExecType = iota
	ExecKernel
	ExecMemcpy
)

// type ExecMemcpyDirection int

// const (
// 	MemcpyHostToDevice ExecMemcpyDirection = iota
// 	MemcpyDeviceToHost
// )

type ExecMemcpyDirection string

const (
	ExecMemcpyDirectionUndefined ExecMemcpyDirection = ""
	H2D                          ExecMemcpyDirection = "MemcpyHtoD"
	D2H                          ExecMemcpyDirection = "MemcpyDtoH"
)

type Dim3 [3]int32

type Register struct {
	Name string
}

func NewRegister(name string) Register {
	return Register{Name: name}
}

type TraceReader struct {
	directoryPath string

	execMetas []TraceExecMeta
}

func (r *TraceReader) GetExecMetas() []TraceExecMeta {
	return r.execMetas
}

type TraceExecMeta struct {
	execType ExecType

	// exec kernel
	filename string
	filepath string

	// exec memcpy
	Direction ExecMemcpyDirection
	Address   uint64
	Length    uint64
}

func (m *TraceExecMeta) ExecType() ExecType {
	return m.execType
}

var kernelScanner *bufio.Scanner

func ReadTrace(meta TraceExecMeta) KernelTrace {
	if meta.execType != ExecKernel {
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
		// fmt.Println("input:", kernelScanner.Text())
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
	t.tbIDToIndex = make(map[Dim3]int32)
	t.Threadblocks = make([]*ThreadblockTrace, 0)

	for goToNextlineWithPrefixIncludingNow("thread block") {
		tb := &ThreadblockTrace{}
		fmt.Sscanf(kernelScanner.Text(), "thread block = %d,%d,%d", &tb.ID[0], &tb.ID[1], &tb.ID[2])

		for moveScannerToNextLine() {
			// fmt.Println("kernelScanner.Text():", kernelScanner.Text())
			if strings.HasPrefix(kernelScanner.Text(), "warp") {
				// fmt.Println("This is visited")
				wp := &WarpTrace{}
				fmt.Sscanf(kernelScanner.Text(), "warp = %d", &wp.ID)

				if !goToNextlineWithPrefixIncludingNow("insts") {
					log.WithField("line", kernelScanner.Text()).Panic("Cannot find insts line")
				}
				fmt.Sscanf(kernelScanner.Text(), "insts = %d", &wp.instsCount)

				for j := 0; j < int(wp.instsCount); j++ {
					moveScannerToNextLine()
					inst := extractInst(kernelScanner.Text())
					inst.threadblockID = tb.ID
					inst.warpID = wp.ID

					wp.Instructions = append(wp.Instructions, inst)
					// fmt.Printf("tb cnt: %d, warp cnt: %d, inst cnt: %d\n",
					// 	len(t.Threadblocks), len(tb.Warps), len(wp.Instructions))
				}

				tb.Warps = append(tb.Warps, wp)
			} else {
				break
			}
		}

		t.Threadblocks = append(t.Threadblocks, tb)
		t.tbIDToIndex[tb.ID] = int32(t.ThreadblocksCount() - 1)
	}
}

/*
#traces format = [line_num] PC mask dest_num [reg_dests] opcode src_num [reg_srcs] mem_width [adrrescompress?]

	0	 1		2					3		4					5		6

[mem_addresses] immediate

	7
*/
func extractInst(text string) *InstructionTrace {
	inst := &InstructionTrace{}
	elems := strings.Fields(text)

	fmt.Sscanf(elems[0], "%x", &inst.PC)
	fmt.Sscanf(elems[1], "%x", &inst.Mask)

	fmt.Sscanf(elems[2], "%d", &inst.DestNum)
	for i := 0; i < inst.DestNum; i++ {
		inst.DestRegs = append(inst.DestRegs, NewRegister(elems[2+i+1]))
	}

	inst.OpCode = NewOpcode(elems[3+int(inst.DestNum)])

	fmt.Sscanf(elems[4+inst.DestNum], "%d", &inst.SrcNum)
	for i := 0; i < inst.SrcNum; i++ {
		inst.SrcRegs = append(inst.SrcRegs, NewRegister(elems[4+inst.DestNum+i+1]))
	}

	updateInstMemoryPart(inst, elems[5+inst.DestNum+inst.SrcNum:])
	return inst
}

// [todo]: understand memory format
func updateInstMemoryPart(inst *InstructionTrace, elems []string) {
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
	inst.Immediate = uint64(imm)
}
