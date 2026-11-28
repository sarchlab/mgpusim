package cu

import (
	"encoding/binary"
	"fmt"
	"slices"
	"testing"

	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/timing/wavefront"
)

// gfx942 encodings of the instructions in the synthetic loops.
var (
	encSAdd    = []uint32{0x8103C103}             // s_add_i32 s3, s3, -1
	encSCmp    = []uint32{0xBF078003}             // s_cmp_lg_u32 s3, 0
	encVMov    = []uint32{0x7E080280}             // v_mov_b32_e32 v4, 0
	encVFma    = []uint32{0xD1CB0002, 0x041A0902} // v_fma_f32 v2, v2, v4, v6
	encSNop    = []uint32{0xBF800000}             // s_nop 0
	encEndPgm  = []uint32{0xBF810000}             // s_endpgm
	sccBranch1 = uint32(0xBF850000)               // s_cbranch_scc1, plus the displacement
	sBranch    = uint32(0xBF820000)               // s_branch, plus the displacement
)

// instMemLatency is the number of cycles from an instruction-memory request
// to its response reaching the compute unit, as for an L1 instruction cache
// hit on MI300X. The instruction can issue in the cycle after that.
const instMemLatency = 16

// loopKernel is a synthetic kernel: s_nop instructions up to the loop head,
// at offset bytes from the start of the code, then a loop of the given
// instructions, ending with s_cbranch_scc1 back to the loop head, then
// s_endpgm. Letters stand for s_add_i32 (S), which counts down, s_cmp_lg_u32
// (C), a 4-byte VALU instruction (v), an 8-byte VALU instruction (V), an
// s_branch to the next instruction (J), and the closing branch (B).
type loopKernel struct {
	offset int
	body   string
}

// code returns the kernel's instructions, placed at base, and the address of
// the closing branch.
func (k loopKernel) code(base uint64) ([]byte, uint64) {
	var words []uint32
	for range k.offset / 4 {
		words = append(words, encSNop...)
	}
	head := len(words)
	branch := uint64(0)
	for _, c := range k.body {
		switch c {
		case 'S':
			words = append(words, encSAdd...)
		case 'C':
			words = append(words, encSCmp...)
		case 'v':
			words = append(words, encVMov...)
		case 'V':
			words = append(words, encVFma...)
		case 'J':
			words = append(words, sBranch)
		case 'B':
			branch = base + uint64(4*len(words))
			displacement := head - (len(words) + 1)
			words = append(words, sccBranch1|uint32(uint16(int16(displacement))))
		default:
			panic("unknown instruction letter " + string(c))
		}
	}
	words = append(words, encEndPgm...)

	code := make([]byte, 4*len(words))
	for i, w := range words {
		binary.LittleEndian.PutUint32(code[4*i:], w)
	}

	return code, branch
}

// mi300xFetchSpec is a compute unit configured as the MI300X preset issues
// and fetches instructions.
func mi300xFetchSpec() Spec {
	spec := DefaultSpec()
	spec.IsCDNA3 = true
	spec.Freq = 1680 * timing.MHz
	spec.RegisterScoreboard = true
	spec.IssueRotation = true
	spec.VALUIssueOverlap = true
	spec.UniformVALUCycles = true
	spec.SCCToBranchCycles = 8
	spec.InstBlockByteSize = 32
	spec.InstBufByteSize = 64
	spec.InstBlockFetchCycles = 16
	spec.InstPrefetchLines = 10
	spec.InstPrefetchMem = "L1I"

	return spec
}

// fetchHarness runs real compute-unit units over code served by an
// instruction memory that answers every request after instMemLatency cycles.
type fetchHarness struct {
	t      *testing.T
	cu     *ComputeUnit
	engine *timing.SerialEngine
	port   *fakePort
	ace    *fakePort
	waves  []*Wavefront
	code   []byte
	base   uint64

	sentAt   []uint64 // cycle of each request in port.sent
	answered int
	// issues records, for each wave, the cycle and address of every
	// instruction it issues.
	issues [][]issueRecord
	lastID []uint64
	// checkpointAt replaces the compute unit's state with a copy restored
	// from its checkpoint form after the tick of that cycle, when positive.
	checkpointAt uint64
}

type issueRecord struct {
	cycle, addr uint64
}

func newFetchHarness(t *testing.T, spec Spec, code []byte, numWaves int) *fetchHarness {
	t.Helper()
	c, simulation := checkpointCUAndSimulation(t, false, spec)
	h := &fetchHarness{t: t, cu: c, engine: simulation.Engine().(*timing.SerialEngine),
		port: newFakePort("CU.InstMem"), code: code, base: 0x10000}
	// One fake memory serves fetches and prefetches; it answers each on the
	// instruction port, which takes either.
	c.ToInstMem = h.port
	c.ToInstPrefetch = h.port
	h.ace = newFakePort("CU.ACE")
	c.ToACE = h.ace

	group := lifecycleGroup(c, numWaves)
	raw := group.raw()
	raw.Packet.Present = true
	raw.Packet.Packet.KernelObject = h.base
	raw.CodeObject.Present, raw.CodeObject.HasSymbol = true, true
	raw.CodeObject.Version = insts.CodeObjectV5
	raw.CodeObject.Symbol.Size = uint64(len(code))
	for i, w := range group.waves() {
		w.SetPC(h.base)
		w.SetEXEC(^uint64(0))
		w.execution().State = wavefront.WfReady
		w.execution().SIMDID = i % numWfPools
		c.WfPools[i%numWfPools].AddWf(w)
		h.waves = append(h.waves, w)
	}
	h.issues = make([][]issueRecord, numWaves)
	h.lastID = make([]uint64, numWaves)

	return h
}

// cycle returns the current cycle number.
func (h *fetchHarness) cycle() uint64 {
	return h.cu.comp.Spec.Freq.Cycle(h.engine.CurrentTime())
}

// step answers the requests that are due and runs the compute unit for one
// cycle.
func (h *fetchHarness) step() {
	next := h.cu.comp.Spec.Freq.NextTick(h.engine.CurrentTime())
	nextCycle := h.cu.comp.Spec.Freq.Cycle(next)
	for h.answered < len(h.port.sent) && h.sentAt[h.answered]+instMemLatency <= nextCycle {
		h.answer(h.port.sent[h.answered])
		h.answered++
	}

	// The fake port does not notify the compute unit, so tick every edge.
	h.cu.comp.TickLater()
	if err := h.engine.RunUntil(next); err != nil {
		h.t.Fatal(err)
	}

	for len(h.sentAt) < len(h.port.sent) {
		h.sentAt = append(h.sentAt, h.cycle())
	}
	if !h.retired() {
		h.recordIssues()
	}

	if h.checkpointAt != 0 && h.cycle() == h.checkpointAt {
		replaceCUState(h.t, h.cu)
		h.cu.comp.State.AfterCheckpointLoad(h.cu.comp)
	}
}

// retired reports whether the work-group has finished and left the compute
// unit.
func (h *fetchHarness) retired() bool { return len(h.ace.sent) > 0 }

func (h *fetchHarness) answer(req messaging.Msg) {
	read := req.Payload.(memprotocol.ReadReq)
	data := make([]byte, read.AccessByteSize)
	if read.Address >= h.base && read.Address < h.base+uint64(len(h.code)) {
		copy(data, h.code[read.Address-h.base:])
	}
	h.port.incoming = append(h.port.incoming, messaging.Msg{ID: h.cu.comp.NewID(), RspTo: req.ID,
		Payload: memprotocol.DataReadyRsp{Data: data}})
}

func (h *fetchHarness) recordIssues() {
	for i, w := range h.waves {
		state := w.execution()
		if state.InstID == 0 || state.InstID == h.lastID[i] {
			continue
		}
		h.lastID[i] = state.InstID
		// The PC moves past an instruction only when it completes.
		h.issues[i] = append(h.issues[i], issueRecord{h.cycle(), w.PC()})
	}
}

// issueCycles returns the cycles at which the wave issued the instruction at
// the address.
func (h *fetchHarness) issueCycles(wave int, addr uint64) []uint64 {
	var cycles []uint64
	for _, r := range h.issues[wave] {
		if r.addr == addr {
			cycles = append(cycles, r.cycle)
		}
	}

	return cycles
}

// loopPeriod runs one wave over the kernel until its loop has settled and
// returns the number of cycles per iteration.
func loopPeriod(t *testing.T, spec Spec, kernel loopKernel, checkpointAt uint64) (uint64, []uint64) {
	t.Helper()
	code, branchOffset := kernel.code(0)
	h := newFetchHarness(t, spec, code, 1)
	h.checkpointAt = checkpointAt
	branch := h.base + branchOffset

	const iterations = 8
	for range 4000 {
		h.step()
		if len(h.issueCycles(0, branch)) > iterations {
			break
		}
	}
	cycles := h.issueCycles(0, branch)
	if len(cycles) <= iterations {
		t.Fatalf("the loop branch issued only %d times", len(cycles))
	}

	return cycles[iterations] - cycles[iterations-1], cycles
}

// A single wave in a hot loop: the target block of a taken branch is
// available 20 cycles after the branch issues, each next block 16 cycles
// after the one before it or 20 cycles after the block two before it is used
// up, whichever is later, and an instruction issues only when all its bytes
// are present.
func TestBlockFetchLoopTiming(t *testing.T) {
	cases := []struct {
		name   string
		kernel loopKernel
		want   uint64
	}{
		{"empty loop", loopKernel{0, "SCB"}, 32},
		{"one 4-byte VALU", loopKernel{0, "vSCB"}, 36},
		{"four 4-byte VALU", loopKernel{0, "vvvvSCB"}, 48},
		{"four 8-byte VALU", loopKernel{0, "VVVVSCB"}, 48},
		{"sixteen 8-byte VALU", loopKernel{0, "VVVVVVVVVVVVVVVVSCB"}, 96},
		{"offset 28, 4-byte, k2", loopKernel{28, "vvSCB"}, 52},
		{"offset 20, 4-byte, k2", loopKernel{20, "vvSCB"}, 44},
		{"offset 8, 8-byte, k4", loopKernel{8, "VVVVSCB"}, 52},
		{"offset 20, 8-byte, k16", loopKernel{20, "VVVVVVVVVVVVVVVVSCB"}, 120},
		{"offset 28, 8-byte, k16", loopKernel{28, "VVVVVVVVVVVVVVVVSCB"}, 124},
		{"add at head, 8-byte, k16", loopKernel{0, "SVVVVVVVVVVVVVVVVCB"}, 108},
		{"VALU before branch, k2", loopKernel{0, "vSCvB"}, 36},
		{"compiled, 8-byte, k16", loopKernel{0, "VSVVVVVVVVVVVVVVCVB"}, 100},
		{"s_branch to the next instruction", loopKernel{0, "JSCB"}, 52},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got, _ := loopPeriod(t, mi300xFetchSpec(), c.kernel, 0); got != c.want {
				t.Fatalf("%d cycles per iteration, want %d", got, c.want)
			}
		})
	}
}

// The wait from s_cmp to the branch on SCC is hidden by one instruction in
// between, and costs one issue turn without it.
func TestSCCBranchWaitIsHiddenByOneInstruction(t *testing.T) {
	spec := mi300xFetchSpec()
	withWait, _ := loopPeriod(t, spec, loopKernel{0, "SCvB"}, 0)
	direct, _ := loopPeriod(t, spec, loopKernel{0, "SCB"}, 0)
	if withWait != 32 || direct != 32 {
		t.Fatalf("s_cmp, VALU, branch takes %d cycles and s_cmp, branch %d; want 32 for both",
			withWait, direct)
	}

	spec.SCCToBranchCycles = 0
	if got, _ := loopPeriod(t, spec, loopKernel{0, "SCB"}, 0); got != 28 {
		t.Fatalf("without the SCC wait the empty loop takes %d cycles, want 28", got)
	}
}

// Restoring the compute unit's state from its checkpoint form, with blocks and
// prefetches in flight, changes no issue time.
func TestBlockFetchCheckpointRoundTrip(t *testing.T) {
	kernel := loopKernel{20, "VVVVVVVVVVVVVVVVSCB"}
	_, reference := loopPeriod(t, mi300xFetchSpec(), kernel, 0)
	for _, at := range []uint64{5, 37, 150, 333} {
		_, restored := loopPeriod(t, mi300xFetchSpec(), kernel, at)
		if !slices.Equal(reference, restored) {
			t.Fatalf("checkpoint at cycle %d: branch issues %v, want %v", at, restored, reference)
		}
	}
}

// straightLineCode is n bytes of 4-byte VALU instructions and an s_endpgm.
func straightLineCode(n int) []byte {
	var body []byte
	for range n / 4 {
		body = binary.LittleEndian.AppendUint32(body, encVMov[0])
	}
	return binary.LittleEndian.AppendUint32(body, encEndPgm[0])
}

// Waves that run the same cold code read each line ahead into the instruction
// cache once: a line that one wave has a request in flight for is not
// requested again for another.
func TestPrefetchReadsEachLineOnce(t *testing.T) {
	code := straightLineCode(2048)
	h := newFetchHarness(t, mi300xFetchSpec(), code, 4)
	for range 3000 {
		h.step()
		if h.retired() {
			break
		}
	}
	if !h.retired() {
		t.Fatal("the waves did not finish")
	}

	prefetched := make(map[uint64]int)
	for _, req := range h.port.sent {
		read := req.Payload.(memprotocol.ReadReq)
		if read.AccessByteSize == InstFetchByteSize {
			prefetched[read.Address]++
		}
		if end := h.base + uint64(len(code)); read.Address >= end {
			t.Fatalf("requested 0x%x past the end of the code at 0x%x", read.Address, end)
		}
	}
	if len(prefetched) == 0 {
		t.Fatal("nothing was prefetched")
	}
	for line, n := range prefetched {
		if n != 1 {
			t.Fatalf("line 0x%x was prefetched %d times", line, n)
		}
	}
}

// A wave reads at most InstPrefetchLines lines past the line of its next
// block.
func TestPrefetchStaysWithinItsWindow(t *testing.T) {
	code := straightLineCode(4096)
	h := newFetchHarness(t, mi300xFetchSpec(), code, 1)
	state := h.waves[0].execution()
	for range 400 {
		h.step()
		for _, info := range h.cu.comp.State.InstFetch {
			if !info.Prefetch {
				continue
			}
			demandLine := state.NextFetchPC &^ (InstFetchByteSize - 1)
			if info.Address > demandLine+10*InstFetchByteSize {
				t.Fatalf("cycle %d: line 0x%x in flight, next block at 0x%x", h.cycle(), info.Address,
					state.NextFetchPC)
			}
		}
	}
}

// Without block fetch the compute unit fetches whole lines, one at a time per
// wave, prefetches nothing, and flushes its buffer on every branch.
func TestLineFetchIsUnchanged(t *testing.T) {
	spec := mi300xFetchSpec()
	spec.InstBlockByteSize = 0
	spec.InstBlockFetchCycles = 0
	spec.InstPrefetchLines = 0
	spec.InstBufByteSize = DefaultInstBufByteSize
	code, branchOffset := loopKernel{0, "vvSCB"}.code(0)
	h := newFetchHarness(t, spec, code, 1)
	for range 600 {
		h.step()
		if inFlight := len(h.cu.comp.State.InstFetch); inFlight > 1 {
			t.Fatalf("cycle %d: %d fetches in flight", h.cycle(), inFlight)
		}
	}
	for _, req := range h.port.sent {
		read := req.Payload.(memprotocol.ReadReq)
		if read.AccessByteSize != InstFetchByteSize || read.Address%InstFetchByteSize != 0 {
			t.Fatalf("fetched %d bytes at 0x%x, want whole lines", read.AccessByteSize, read.Address)
		}
	}
	if n := len(h.issueCycles(0, h.base+branchOffset)); n < 4 {
		t.Fatalf("the loop branch issued only %d times", n)
	}
}

func (r issueRecord) String() string { return fmt.Sprintf("%d@0x%x", r.cycle, r.addr) }
