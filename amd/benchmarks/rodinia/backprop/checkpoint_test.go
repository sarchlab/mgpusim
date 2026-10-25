package backprop

import (
	"bytes"
	simulation "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/emusystem"
	"testing"
)

func TestRequestedDefaultsSurviveEffectiveDimensions(t *testing.T) {
	s := simulation.MakeBuilder().Build()
	rt := emusystem.MakeBuilder().WithSimulation(s).WithArchitecture(arch.CDNA3).Build()
	b := NewBenchmark(rt)
	b.Arch = arch.CDNA3
	if err := s.Initialize(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Terminate(); err != nil {
			t.Error(err)
		}
	})
	b.SelectGPU([]int{1})
	b.SetVerification(true)
	if err := b.Step(); err != nil {
		t.Fatal(err)
	}
	if b.InputN != 64 || b.HiddenN != 32 || b.OutputN != 4 {
		t.Fatal("test did not normalize default dimensions")
	}
	var buf bytes.Buffer
	if err := b.SaveCheckpoint(&buf); err != nil {
		t.Fatal(err)
	}
	restored := NewBenchmark(rt)
	restored.Arch = arch.CDNA3
	restored.SelectGPU([]int{1})
	restored.SetVerification(true)
	if err := restored.LoadCheckpoint(bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatal("same requested defaults rejected:", err)
	}
	if restored.InputN != 64 || restored.HiddenN != 32 || restored.OutputN != 4 {
		t.Fatal("effective dimensions were not restored")
	}
	mismatch := NewBenchmark(rt)
	mismatch.Arch = arch.CDNA3
	mismatch.SelectGPU([]int{1})
	mismatch.SetVerification(true)
	mismatch.InputN = 128
	if err := mismatch.LoadCheckpoint(bytes.NewReader(buf.Bytes())); err == nil {
		t.Fatal("different requested configuration accepted")
	}
}
