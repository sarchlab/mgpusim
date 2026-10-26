package timingconfig

import (
	"testing"

	simulation "github.com/sarchlab/akita/v5/sim"
)

// buildPlatform assembles a platform of the given GPU type and count. The
// assembly exercises the port naming, the port assignment, and the mapper
// snapshotting of all the components, all of which panic on error.
func buildPlatform(t *testing.T, gpuType string, numGPUs int) {
	t.Helper()

	s := simulation.MakeBuilder().
		WithOutputPath(t.TempDir() + "/sim.sqlite3").
		Build()
	defer s.Terminate()

	gpuDriver := MakeBuilder().
		WithSimulation(s).
		WithNumGPUs(numGPUs).
		WithGPUType(gpuType).
		Build()

	if err := s.Initialize(); err != nil {
		t.Fatal(err)
	}

	if gpuDriver == nil {
		t.Fatal("driver must not be nil")
	}

	if gpuDriver.GetNumGPUs() != numGPUs {
		t.Fatalf("expected %d GPUs, got %d", numGPUs, gpuDriver.GetNumGPUs())
	}

	found := make(map[string]bool)
	for _, c := range s.Components() {
		found[c.Name()] = true
	}
	if !found["Driver"] {
		t.Fatal("driver must be registered with the simulation")
	}

	cpName := "GPU[1].CommandProcessor"
	if !found[cpName] {
		t.Fatalf("component %s must be registered", cpName)
	}
}

func TestBuildR9NanoPlatform(t *testing.T) {
	buildPlatform(t, "r9nano", 1)
}

func TestBuildR9NanoMultiGPUPlatform(t *testing.T) {
	buildPlatform(t, "r9nano", 2)
}

func TestBuildMI300XPlatform(t *testing.T) {
	buildPlatform(t, "mi300x", 1)
}
