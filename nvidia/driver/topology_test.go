package driver

import (
	"github.com/sarchlab/mgpusim/v5/nvidia/trace"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/sim"
)

func TestDeviceSpecCopiedAndCheckpointCompatible(t *testing.T) {
	path := filepath.Join(t.TempDir(), "checkpoint.tar.gz")
	build := func(port string) *sim.Simulation {
		s := sim.MakeBuilder().Build()
		t.Cleanup(func() {
			if err := s.Terminate(); err != nil {
				t.Error(err)
			}
		})
		peers := []DevicePeer{{Name: "GPU", Port: "GPU.ToDriver"}}
		if port != "GPU.ToDriver" {
			peers[0].Port = "Other.ToDriver"
		}
		d := MakeBuilder().WithTraceStore(trace.NewStore(s, "Traces")).WithSimulation(s).WithDevices(peers).Build("Driver")
		peers[0].Name = "changed"
		if d.Spec.Devices[0].Name != "GPU" {
			t.Fatal("builder retained caller peer slice")
		}
		if err := s.Initialize(); err != nil {
			t.Fatal(err)
		}
		if len(d.State.FreeDevices) != 1 || d.State.FreeDevices[0] != "GPU" {
			t.Fatal("device ordering not initialized from Spec")
		}
		return s
	}
	source := build("GPU.ToDriver")
	if err := source.SaveCheckpoint(path); err != nil {
		t.Fatal(err)
	}
	same := build("GPU.ToDriver")
	if err := same.LoadCheckpoint(path); err != nil {
		t.Fatal(err)
	}
	changed := build("Other.ToDriver")
	if err := changed.LoadCheckpoint(path); err == nil || !strings.Contains(err.Error(), "spec") {
		t.Fatalf("changed peer not rejected by Spec compatibility: %v", err)
	}
}
