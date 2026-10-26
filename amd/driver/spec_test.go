package driver

import (
	"bytes"
	"testing"

	"github.com/sarchlab/akita/v5/sim/timing"
)

func TestRuntimeAndDriverCheckpointConfigurationsAreIndependent(t *testing.T) {
	first := newFixture(t, false)
	second := newConfiguredFixture(t, false, func(s *Spec) { s.Freq = 2 * timing.GHz; s.D2HCycles = 17; s.H2DCycles = 23 })
	var runtime, component bytes.Buffer
	if err := first.r.SaveCheckpoint(&runtime); err != nil {
		t.Fatal(err)
	}
	if err := first.c.SaveCheckpoint(&component); err != nil {
		t.Fatal(err)
	}
	if err := second.r.LoadCheckpoint(bytes.NewReader(runtime.Bytes())); err != nil {
		t.Fatal("Driver timing leaked into Runtime configuration:", err)
	}
	if err := second.c.LoadCheckpoint(bytes.NewReader(component.Bytes())); err == nil {
		t.Fatal("Driver timing mismatch accepted")
	}
	if first.r.GetNumGPUs() != 2 || second.r.GetNumGPUs() != 2 {
		t.Fatal("GPU count did not come from Runtime devices")
	}
	second.r.config.Devices[0].CUCount++
	if err := second.r.LoadCheckpoint(bytes.NewReader(runtime.Bytes())); err == nil {
		t.Fatal("Runtime device mismatch accepted")
	}
}
