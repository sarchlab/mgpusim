package spmv

import (
	"math"
	"testing"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/matrix/csr"
)

func TestCDNA3VerifyUsesFusedFP32Accumulation(t *testing.T) {
	const fused = 0x3fc00001
	benchmark := &Benchmark{State: &State{
		Arch: arch.CDNA3,
		Dim:  1,
		Matrix: csr.Matrix{
			RowOffsets:    []uint32{0, 2},
			ColumnNumbers: []uint32{0, 1},
			Values:        []float32{-0x1p-80, 1 + 0x1p-23},
		},
		Vec: []float32{1, 1.5},
		Out: []float32{math.Float32frombits(fused)},
	}}
	if err := benchmark.verify(); err != nil {
		t.Fatalf("exact fused output failed verification: %v", err)
	}

	benchmark.Out[0] = math.Float32frombits(fused + 1)
	if err := benchmark.verify(); err == nil {
		t.Fatal("one ULP error passed verification")
	}
}
