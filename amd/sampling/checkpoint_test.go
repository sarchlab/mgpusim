package sampling

import (
	"bytes"
	"testing"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/timing"
)

func TestSamplingCheckpointContinuesPrediction(t *testing.T) {
	for _, stop := range []int{1026, 1060} {
		original := NewSampledEngine(16, 0.03, false)
		collect := func(e *SampledEngine, i int) {
			issue := timing.VTimeInPicoSec(i * 1000)
			e.Collect(issue, issue+5000)
		}
		for i := 0; i < stop; i++ {
			collect(original, i)
		}
		var checkpoint bytes.Buffer
		if err := original.SaveCheckpoint(&checkpoint); err != nil {
			t.Fatal(err)
		}
		restored := NewSampledEngine(16, 0.03, false)
		if err := restored.LoadCheckpoint(bytes.NewReader(checkpoint.Bytes())); err != nil {
			t.Fatal(err)
		}
		wrongConfig := NewSampledEngine(32, 0.03, false)
		if err := wrongConfig.LoadCheckpoint(bytes.NewReader(checkpoint.Bytes())); err == nil {
			t.Fatal("accepted a different sampling window")
		}
		for i := stop; i < 1100; i++ {
			collect(original, i)
			collect(restored, i)
			want, active := original.Predict()
			got, restoredActive := restored.Predict()
			if got != want || active != restoredActive {
				t.Fatalf("sample %d: prediction differs after restore", i)
			}
		}
		prediction, active := restored.Predict()
		if !active || prediction < 4999 || prediction > 5001 {
			t.Fatalf("expected stable 5000ps prediction, got %d, active=%v", prediction, active)
		}
		var a, b bytes.Buffer
		if err := original.SaveCheckpoint(&a); err != nil {
			t.Fatal(err)
		}
		if err := restored.SaveCheckpoint(&b); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(a.Bytes(), b.Bytes()) {
			t.Fatal("prediction histories diverged")
		}
	}
}

func TestSamplingBelongsToSimulation(t *testing.T) {
	first, second := sim.MakeBuilder().Build(), sim.MakeBuilder().Build()
	defer first.Terminate()
	defer second.Terminate()
	a := NewSimulationEngine(first, 16, 0.03, true)
	b := NewSimulationEngine(second, 16, 0.03, true)
	for i := 0; i < 1100; i++ {
		a.Collect(timing.VTimeInPicoSec(i*1000), timing.VTimeInPicoSec(i*1000+5000))
	}
	if _, active := a.Predict(); !active {
		t.Fatal("first sampler did not stabilize")
	}
	if _, active := b.Predict(); active {
		t.Fatal("second simulation inherited sampling history")
	}
	b.Reset()
	if _, active := a.Predict(); !active {
		t.Fatal("reset of second simulation changed first sampler")
	}
}

func TestSamplingDisabledStateRestores(t *testing.T) {
	original := NewSampledEngine(16, 0.03, false)
	original.Disabled()
	var data bytes.Buffer
	if err := original.SaveCheckpoint(&data); err != nil {
		t.Fatal(err)
	}
	restored := NewSampledEngine(16, 0.03, false)
	if err := restored.LoadCheckpoint(&data); err != nil {
		t.Fatal(err)
	}
	if !restored.IfDisable() {
		t.Fatal("lost runtime disable state")
	}
}
