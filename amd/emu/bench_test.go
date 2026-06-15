package emu

import (
	"testing"

	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// BenchmarkNewWavefront measures the time to allocate a new Wavefront.
// After removing the 4KB scratchpad allocation, this should be faster.
func BenchmarkNewWavefront(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewWavefront(nil)
	}
}

// mockBenchALU is a minimal ALU implementation for benchmarking executeInst.
type mockBenchALU struct{}

func (m *mockBenchALU) Run(state InstEmuState) {}
func (m *mockBenchALU) SetLDS(lds []byte)      {}
func (m *mockBenchALU) LDS() []byte            { return nil }
func (m *mockBenchALU) ArchName() string       { return "mock" }

// BenchmarkExecuteInst_VOP2 measures the overhead of executeInst (just alu.Run)
// for a VOP2 instruction using a no-op mock ALU.
func BenchmarkExecuteInst_VOP2(b *testing.B) {
	wf := NewWavefront(nil)
	inst := insts.NewInst()
	inst.FormatType = insts.VOP2
	inst.Src0 = insts.NewIntOperand(0, 1)
	inst.Src1 = insts.NewVRegOperand(0, 0, 1)
	inst.Dst = insts.NewVRegOperand(1, 1, 1)
	wf.inst = inst

	// Set EXEC mask
	wf.SetEXEC(0xFFFFFFFFFFFFFFFF)

	cu := &ComputeUnit{alu: &mockBenchALU{}}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cu.executeInst(wf)
	}
}
