package emu

import (
	"testing"

	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// newBenchWavefront creates a Wavefront suitable for benchmarking.
// Uses WriteReg for setting EXEC/VCC/SCC so it works on both
// the main branch (exported fields) and gfx942_emu (unexported fields).
func newBenchWavefront(formatType insts.FormatType) *Wavefront {
	wf := NewWavefront(nil)

	inst := insts.NewInst()
	inst.FormatType = formatType

	switch formatType {
	case insts.SOP1:
		inst.FormatName = "SOP1"
		inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		inst.Dst = insts.NewSRegOperand(2, 2, 1)
	case insts.SOP2:
		inst.FormatName = "SOP2"
		inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		inst.Src1 = insts.NewIntOperand(1, 42)
		inst.Dst = insts.NewSRegOperand(2, 2, 1)
	case insts.SOPC:
		inst.FormatName = "SOPC"
		inst.Src0 = insts.NewSRegOperand(0, 0, 1)
		inst.Src1 = insts.NewIntOperand(1, 42)
	case insts.SOPK:
		inst.FormatName = "SOPK"
		inst.Dst = insts.NewSRegOperand(0, 0, 1)
		inst.SImm16 = insts.NewIntOperand(0, 100)
	case insts.SOPP:
		inst.FormatName = "SOPP"
		inst.SImm16 = insts.NewIntOperand(0, 4)
	case insts.VOP1:
		inst.FormatName = "VOP1"
		inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		inst.Dst = insts.NewVRegOperand(1, 1, 1)
	case insts.VOP2:
		inst.FormatName = "VOP2"
		inst.Src0 = insts.NewIntOperand(0, 1)
		inst.Src1 = insts.NewVRegOperand(0, 0, 1)
		inst.Dst = insts.NewVRegOperand(1, 1, 1)
	case insts.VOP3a:
		inst.FormatName = "VOP3a"
		inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		inst.Src2 = insts.NewVRegOperand(2, 2, 1)
		inst.Dst = insts.NewVRegOperand(3, 3, 1)
	case insts.VOP3b:
		inst.FormatName = "VOP3b"
		inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		inst.Src1 = insts.NewVRegOperand(1, 1, 1)
		inst.Dst = insts.NewVRegOperand(2, 2, 1)
		inst.SDst = insts.NewRegOperand(0, insts.VCCLO, 2)
	case insts.VOPC:
		inst.FormatName = "VOPC"
		inst.Src0 = insts.NewVRegOperand(0, 0, 1)
		inst.Src1 = insts.NewVRegOperand(1, 1, 1)
	case insts.FLAT:
		inst.FormatName = "FLAT"
		inst.Addr = insts.NewVRegOperand(0, 0, 2)
		inst.Data = insts.NewVRegOperand(2, 2, 1)
		inst.Dst = insts.NewVRegOperand(4, 4, 1)
	case insts.DS:
		inst.FormatName = "DS"
		inst.Addr = insts.NewVRegOperand(0, 0, 1)
		inst.Data = insts.NewVRegOperand(1, 1, 1)
		inst.Data1 = insts.NewVRegOperand(2, 2, 1)
		inst.Dst = insts.NewVRegOperand(3, 3, 1)
	case insts.SMEM:
		inst.FormatName = "SMEM"
		inst.Base = insts.NewSRegOperand(0, 0, 2)
		inst.Offset = insts.NewIntOperand(0, 0)
		inst.Data = insts.NewSRegOperand(2, 2, 1)
	}

	wf.inst = inst

	// Set up register values via WriteReg (compatible with both branches).
	for i := 0; i < 4; i++ {
		wf.WriteReg(insts.SReg(i), 1, 0, insts.Uint32ToBytes(uint32(i+100)))
	}
	for lane := 0; lane < 64; lane++ {
		for i := 0; i < 5; i++ {
			wf.WriteReg(insts.VReg(i), 1, lane, insts.Uint32ToBytes(uint32(lane*10+i)))
		}
	}

	// Set EXEC mask via WriteReg (works on both branches).
	wf.WriteReg(insts.Regs[insts.EXEC], 1, 0,
		insts.Uint64ToBytes(0xFFFFFFFFFFFFFFFF))
	// Set VCC via WriteReg.
	wf.WriteReg(insts.Regs[insts.VCC], 1, 0,
		insts.Uint64ToBytes(0xAAAAAAAAAAAAAAAA))
	// Set SCC via WriteReg.
	wf.WriteReg(insts.Regs[insts.SCC], 1, 0, []byte{1})

	return wf
}

// ---------------------------------------------------------------------------
// Benchmark 1: Prepare+Commit round-trip for various instruction formats
// ---------------------------------------------------------------------------

func BenchmarkPrepareCommit_VOP2(b *testing.B) {
	wf := newBenchWavefront(insts.VOP2)
	sp := NewScratchpadPreparerImpl(false)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sp.Prepare(wf, wf)
		sp.Commit(wf, wf)
	}
}

func BenchmarkPrepareOnly_VOP2(b *testing.B) {
	wf := newBenchWavefront(insts.VOP2)
	sp := NewScratchpadPreparerImpl(false)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sp.Prepare(wf, wf)
	}
}

func BenchmarkCommitOnly_VOP2(b *testing.B) {
	wf := newBenchWavefront(insts.VOP2)
	sp := NewScratchpadPreparerImpl(false)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sp.Commit(wf, wf)
	}
}

func BenchmarkPrepareCommit_SOP1(b *testing.B) {
	wf := newBenchWavefront(insts.SOP1)
	sp := NewScratchpadPreparerImpl(false)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sp.Prepare(wf, wf)
		sp.Commit(wf, wf)
	}
}

func BenchmarkPrepareCommit_SOP2(b *testing.B) {
	wf := newBenchWavefront(insts.SOP2)
	sp := NewScratchpadPreparerImpl(false)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sp.Prepare(wf, wf)
		sp.Commit(wf, wf)
	}
}

func BenchmarkPrepareCommit_SOPC(b *testing.B) {
	wf := newBenchWavefront(insts.SOPC)
	sp := NewScratchpadPreparerImpl(false)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sp.Prepare(wf, wf)
		sp.Commit(wf, wf)
	}
}

func BenchmarkPrepareCommit_SOPK(b *testing.B) {
	wf := newBenchWavefront(insts.SOPK)
	sp := NewScratchpadPreparerImpl(false)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sp.Prepare(wf, wf)
		sp.Commit(wf, wf)
	}
}

func BenchmarkPrepareCommit_SOPP(b *testing.B) {
	wf := newBenchWavefront(insts.SOPP)
	sp := NewScratchpadPreparerImpl(false)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sp.Prepare(wf, wf)
		sp.Commit(wf, wf)
	}
}

func BenchmarkPrepareCommit_VOP1(b *testing.B) {
	wf := newBenchWavefront(insts.VOP1)
	sp := NewScratchpadPreparerImpl(false)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sp.Prepare(wf, wf)
		sp.Commit(wf, wf)
	}
}

func BenchmarkPrepareCommit_VOP3a(b *testing.B) {
	wf := newBenchWavefront(insts.VOP3a)
	sp := NewScratchpadPreparerImpl(false)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sp.Prepare(wf, wf)
		sp.Commit(wf, wf)
	}
}

func BenchmarkPrepareCommit_VOP3b(b *testing.B) {
	wf := newBenchWavefront(insts.VOP3b)
	sp := NewScratchpadPreparerImpl(false)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sp.Prepare(wf, wf)
		sp.Commit(wf, wf)
	}
}

func BenchmarkPrepareCommit_VOPC(b *testing.B) {
	wf := newBenchWavefront(insts.VOPC)
	sp := NewScratchpadPreparerImpl(false)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sp.Prepare(wf, wf)
		sp.Commit(wf, wf)
	}
}

func BenchmarkPrepareCommit_FLAT(b *testing.B) {
	wf := newBenchWavefront(insts.FLAT)
	sp := NewScratchpadPreparerImpl(false)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sp.Prepare(wf, wf)
		sp.Commit(wf, wf)
	}
}

func BenchmarkPrepareCommit_DS(b *testing.B) {
	wf := newBenchWavefront(insts.DS)
	sp := NewScratchpadPreparerImpl(false)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sp.Prepare(wf, wf)
		sp.Commit(wf, wf)
	}
}

func BenchmarkPrepareCommit_SMEM(b *testing.B) {
	wf := newBenchWavefront(insts.SMEM)
	sp := NewScratchpadPreparerImpl(false)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sp.Prepare(wf, wf)
		sp.Commit(wf, wf)
	}
}
