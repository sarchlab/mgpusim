package backprop

import (
	"bytes"
	"testing"

	simulation "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/emusystem"
)

//nolint:funlen // Keep requested defaults and both lifecycle boundaries together.
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
	var initial bytes.Buffer
	if err := b.SaveCheckpoint(&initial); err != nil {
		t.Fatal(err)
	}
	same := NewBenchmark(rt)
	same.Arch = arch.CDNA3
	same.SelectGPU([]int{1})
	same.SetVerification(true)
	if err := same.LoadCheckpoint(bytes.NewReader(initial.Bytes())); err != nil {
		t.Fatal(err)
	}
	if same.InputN != 0 || same.HiddenN != 0 || same.OutputN != 0 {
		t.Fatal("loading unstarted state normalized defaults")
	}
	different := NewBenchmark(rt)
	different.Arch = arch.CDNA3
	different.SelectGPU([]int{1})
	different.SetVerification(true)
	different.InputN = 128
	if err := different.LoadCheckpoint(bytes.NewReader(initial.Bytes())); err == nil {
		t.Fatal("unstarted configuration mismatch accepted")
	}
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
