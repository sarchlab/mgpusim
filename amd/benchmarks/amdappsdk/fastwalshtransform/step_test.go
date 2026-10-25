package fastwalshtransform

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/emusystem"
)

// Every queue must receive all six stages in the same host round. We only
// inspect submission here: the baseline multi-GPU algorithm shares an array,
// so this test makes no new assertion about multi-GPU numerical correctness.
func TestAllQueueStagesSubmittedTogether(t *testing.T) {
	s := sim.MakeBuilder().Build()
	rt := emusystem.MakeBuilder().WithSimulation(s).WithNumGPUs(2).Build()
	b := NewBenchmark(rt)
	b.Length = 64
	b.SelectGPU([]int{1, 2})
	r := runner.New(s, rt)
	r.AddBenchmark(b)
	if err := s.Initialize(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Terminate(); err != nil {
			t.Error(err)
		}
	})
	for i := 0; i < 2; i++ {
		if _, err := r.Step(); err != nil {
			t.Fatal(err)
		}
	}
	if b.Phase != 2 {
		t.Fatalf("submission stopped at phase %d", b.Phase)
	}
	var buf bytes.Buffer
	if err := rt.SaveCheckpoint(&buf); err != nil {
		t.Fatal(err)
	}
	var checkpoint struct {
		State struct {
			State struct {
				Queues []struct {
					Pending []struct {
						Packets []json.RawMessage `json:"Packets"`
					} `json:"Pending"`
				} `json:"Queues"`
			} `json:"State"`
		} `json:"state"`
	}
	if err := json.Unmarshal(buf.Bytes(), &checkpoint); err != nil {
		t.Fatal(err)
	}
	for _, queue := range b.queues {
		launches := 0
		for _, command := range checkpoint.State.State.Queues[queue-1].Pending {
			if len(command.Packets) != 0 {
				launches++
			}
		}
		if launches != 6 {
			t.Fatalf("queue %d has %d stages, want 6", queue, launches)
		}
	}
}
