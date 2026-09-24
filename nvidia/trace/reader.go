package trace

import (
	"bufio"
	"fmt"
	"math/bits"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// KernelsListFileName is the index file written by the Accel-Sim trace
// post-processor. It lists memory copies and kernel trace files in launch
// order.
const KernelsListFileName = "kernelslist.g"

const (
	execMemcpyPrefix = "Memcpy"
	execKernelPrefix = "kernel"
	maxLineBytes     = 16 * 1024 * 1024
)

// TraceExecMeta describes one entry of kernelslist.g.
type TraceExecMeta struct {
	execType ExecType
	filepath string

	Direction ExecMemcpyDirection
	Address   uint64
	Length    uint64
}

// ExecType returns whether the entry is a kernel or a memory copy.
func (m *TraceExecMeta) ExecType() ExecType {
	return m.execType
}

// TraceReader reads the kernelslist.g file of a trace directory.
type TraceReader struct {
	directoryPath string
	execMetas     []TraceExecMeta
}

// GetExecMetas returns the entries of kernelslist.g in order.
func (r *TraceReader) GetExecMetas() []TraceExecMeta {
	return r.execMetas
}

// TraceReaderBuilder builds a TraceReader.
type TraceReaderBuilder struct {
	traceDirPath string
}

// WithTraceDirectory sets the directory that holds kernelslist.g.
func (b *TraceReaderBuilder) WithTraceDirectory(
	traceDirPath string,
) *TraceReaderBuilder {
	b.traceDirPath = traceDirPath
	return b
}

// Build reads kernelslist.g and returns the reader.
func (b *TraceReaderBuilder) Build() *TraceReader {
	if b.traceDirPath == "" {
		panic("trace directory must be set")
	}

	r := &TraceReader{directoryPath: b.traceDirPath}
	r.readKernelsList()

	return r
}

func (r *TraceReader) readKernelsList() {
	path := filepath.Join(r.directoryPath, KernelsListFileName)

	file, err := os.Open(path)
	if err != nil {
		panic(fmt.Sprintf("failed to open %s: %v", path, err))
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			r.execMetas = append(r.execMetas, r.parseExecLine(line))
		}
	}

	if err := scanner.Err(); err != nil {
		panic(fmt.Sprintf("failed to read %s: %v", path, err))
	}
}

func (r *TraceReader) parseExecLine(text string) TraceExecMeta {
	m := TraceExecMeta{}

	switch {
	case strings.HasPrefix(text, execMemcpyPrefix):
		m.execType = ExecMemcpy
		fields := strings.Split(text, ",")

		if len(fields) != 3 {
			panic(fmt.Sprintf("malformed memcpy line %q", text))
		}

		m.Direction = ExecMemcpyDirection(fields[0])
		m.Address = mustParseUint(fields[1])
		m.Length = mustParseUint(fields[2])
	case strings.HasPrefix(text, execKernelPrefix):
		m.execType = ExecKernel
		m.filepath = filepath.Join(r.directoryPath, text)
	default:
		panic(fmt.Sprintf("unknown entry %q in %s", text, KernelsListFileName))
	}

	return m
}

func mustParseUint(s string) uint64 {
	v, err := strconv.ParseUint(strings.TrimSpace(s), 0, 64)
	if err != nil {
		panic(fmt.Sprintf("failed to parse %q as an integer: %v", s, err))
	}

	return v
}

// kernelFileReader parses one .traceg file.
type kernelFileReader struct {
	scanner *bufio.Scanner
}

// ReadTrace loads the kernel trace file that a kernel entry points to.
func ReadTrace(meta TraceExecMeta) KernelTrace {
	if meta.execType != ExecKernel {
		panic("ReadTrace called on a non-kernel entry")
	}

	file, err := os.Open(meta.filepath)
	if err != nil {
		panic(fmt.Sprintf("failed to open %s: %v", meta.filepath, err))
	}
	defer file.Close()

	r := &kernelFileReader{scanner: bufio.NewScanner(file)}
	r.scanner.Buffer(make([]byte, 0, 64*1024), maxLineBytes)

	kernel := KernelTrace{ID: filepath.Base(meta.filepath)}
	r.readHeader(&kernel)
	r.readThreadblocks(&kernel)

	if err := r.scanner.Err(); err != nil {
		panic(fmt.Sprintf("failed to read %s: %v", meta.filepath, err))
	}

	return kernel
}

// nextNonEmptyLine advances the scanner to the next non-empty line.
func (r *kernelFileReader) nextNonEmptyLine() bool {
	for r.scanner.Scan() {
		if r.scanner.Text() != "" {
			return true
		}
	}

	return false
}

// seekPrefix advances to the first line, starting from the current one, that
// starts with prefix.
func (r *kernelFileReader) seekPrefix(prefix string) bool {
	if strings.HasPrefix(r.scanner.Text(), prefix) {
		return true
	}

	for r.nextNonEmptyLine() {
		if strings.HasPrefix(r.scanner.Text(), prefix) {
			return true
		}
	}

	return false
}

func (r *kernelFileReader) readHeader(kernel *KernelTrace) {
	for r.nextNonEmptyLine() {
		text := r.scanner.Text()
		if !strings.HasPrefix(text, "-") {
			return
		}

		key, value, found := strings.Cut(text[1:], "=")
		if !found {
			continue
		}

		kernel.FileHeader.updateTraceHeaderParam(
			strings.TrimSpace(key), strings.TrimSpace(value))
	}
}

func (r *kernelFileReader) readThreadblocks(kernel *KernelTrace) {
	for r.seekPrefix("thread block") {
		tb := &ThreadblockTrace{FatherKernelID: kernel.ID}
		mustSscanf(r.scanner.Text(), "thread block = %d,%d,%d",
			&tb.ID[0], &tb.ID[1], &tb.ID[2])

		// Newer post-processors write cluster lines (cluster id, cluster
		// cta, cluster rank) between the thread block and its first warp.
		// Lines that are not warps are skipped until the thread block ends.
		for r.nextNonEmptyLine() {
			text := r.scanner.Text()
			if strings.HasPrefix(text, "#END_TB") ||
				strings.HasPrefix(text, "thread block") {
				break
			}

			if strings.HasPrefix(text, "warp") {
				tb.Warps = append(tb.Warps, r.readWarp(tb.ID))
			}
		}

		if len(tb.Warps) == 0 {
			panic(fmt.Sprintf("thread block %v in %s has no warps",
				tb.ID, kernel.ID))
		}

		kernel.Threadblocks = append(kernel.Threadblocks, tb)
	}
}

func (r *kernelFileReader) readWarp(tbID Dim3) *WarpTrace {
	wp := &WarpTrace{FatherThreadblockID: tbID}
	mustSscanf(r.scanner.Text(), "warp = %d", &wp.ID)

	if !r.seekPrefix("insts") {
		panic(fmt.Sprintf("cannot find the insts line of warp %d", wp.ID))
	}

	var instsCount int

	mustSscanf(r.scanner.Text(), "insts = %d", &instsCount)

	for j := range instsCount {
		if !r.nextNonEmptyLine() {
			panic("trace file ends in the middle of a warp")
		}

		inst := extractInst(r.scanner.Text(), uint64(j))
		inst.threadblockID = tbID
		inst.warpID = wp.ID
		wp.Instructions = append(wp.Instructions, inst)
	}

	return wp
}

func mustSscanf(text, format string, args ...any) {
	if _, err := fmt.Sscanf(text, format, args...); err != nil {
		panic(fmt.Sprintf("failed to parse %q: %v", text, err))
	}
}

// extractInst parses one instruction line. The format is
//
//	PC mask dest_num [reg_dests] opcode src_num [reg_srcs] mem_width
//	[address_compress] [mem_addresses] immediate
func extractInst(text string, instIndexInWarp uint64) *InstructionTrace {
	inst := &InstructionTrace{instIndexInWarp: instIndexInWarp}
	elems := strings.Fields(text)

	inst.PC = mustParseHex(elems[0])
	inst.Mask = mustParseHex(elems[1])
	inst.DestNum = mustAtoi(elems[2])

	for i := range inst.DestNum {
		inst.DestRegs = append(inst.DestRegs, Register{Name: elems[3+i]})
	}

	inst.OpCode = Opcode(elems[3+inst.DestNum])
	inst.SrcNum = mustAtoi(elems[4+inst.DestNum])

	for i := range inst.SrcNum {
		inst.SrcRegs = append(inst.SrcRegs,
			Register{Name: elems[5+inst.DestNum+i]})
	}

	updateInstMemoryPart(inst, elems[5+inst.DestNum+inst.SrcNum:])

	return inst
}

// updateInstMemoryPart parses "mem_width [address_compress] [addresses]
// immediate ...". Tracer version 5 ends the line with one immediate; version
// 6 adds a second immediate and register values, which are ignored. The
// number of addresses depends on the number of active threads in the mask:
//
//	compress 0: one address per active thread
//	compress 1: base address and stride
//	compress 2: base address and one delta per remaining active thread
func updateInstMemoryPart(inst *InstructionTrace, elems []string) {
	inst.MemWidth = mustAtoi(elems[0])
	next := 1

	if inst.MemWidth != 0 {
		inst.AddressCompress = mustAtoi(elems[1])
		inst.MemAddress = mustParseHex(elems[2])
		active := bits.OnesCount64(inst.Mask)

		switch inst.AddressCompress {
		case 0:
			next = 2 + max(active, 1)
		case 1:
			inst.MemAddressSuffix1 = mustAtoi(elems[3])
			next = 4
		case 2:
			next = 3 + max(active-1, 0)
			for _, d := range elems[3:min(next, len(elems))] {
				inst.MemAddressSuffix2 = append(inst.MemAddressSuffix2,
					int32(mustAtoi(d)))
			}
		default:
			panic(fmt.Sprintf("unknown address compression %d",
				inst.AddressCompress))
		}
	}

	if next < len(elems) {
		imm, err := strconv.ParseInt(elems[next], 0, 64)
		if err == nil {
			inst.Immediate = uint64(imm)
		}
	}
}

func mustParseHex(s string) uint64 {
	v, err := strconv.ParseUint(strings.TrimPrefix(s, "0x"), 16, 64)
	if err != nil {
		panic(fmt.Sprintf("failed to parse %q as hex: %v", s, err))
	}

	return v
}

func mustAtoi(s string) int {
	v, err := strconv.Atoi(s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse %q as an integer: %v", s, err))
	}

	return v
}
