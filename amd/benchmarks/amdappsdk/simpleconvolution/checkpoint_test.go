package simpleconvolution

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/emusystem"
)

func TestCheckpointRestoresMaskBeforeAndAfterSubmission(t *testing.T) {
	s := sim.MakeBuilder().Build()
	rt := emusystem.MakeBuilder().WithSimulation(s).WithNumGPUs(1).Build()
	b := NewBenchmark(rt)
	b.Width, b.Height = 16, 16
	b.SelectGPU([]int{1})
	b.SetMaskSize(3)
	if err := s.Initialize(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := s.Terminate(); err != nil {
			t.Error(err)
		}
	}()
	for phase := 0; phase < 2; phase++ {
		var saved bytes.Buffer
		if err := b.SaveCheckpoint(&saved); err != nil {
			t.Fatal(err)
		}
		for _, mask := range []uint32{3, 5} {
			other := NewBenchmark(rt)
			other.Width, other.Height = 16, 16
			other.SelectGPU([]int{1})
			other.SetMaskSize(mask)
			if err := other.LoadCheckpoint(bytes.NewReader(saved.Bytes())); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(other.State, b.State) {
				t.Fatal("saved mask state not restored")
			}
		}
		if phase == 0 {
			if err := b.Step(); err != nil {
				t.Fatal(err)
			}
		}
	}
}
