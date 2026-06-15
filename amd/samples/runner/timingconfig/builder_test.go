package timingconfig

import (
	"testing"

	"github.com/sarchlab/akita/v5/simulation"
)

// buildPlatform assembles a platform of the given GPU type and count. The
// assembly exercises the port naming, the port assignment, and the mapper
// snapshotting of all the components, all of which panic on error.
func buildPlatform(t *testing.T, gpuType string, numGPUs int) {
	t.Helper()

	s := simulation.MakeBuilder().
		WithoutMonitoring().
		WithOutputFileName(t.TempDir() + "/sim").
		Build()
	defer s.Terminate()

	gpuDriver := MakeBuilder().
		WithSimulation(s).
		WithNumGPUs(numGPUs).
		WithGPUType(gpuType).
		Build()

	if gpuDriver == nil {
		t.Fatal("driver must not be nil")
	}

	if len(gpuDriver.GPUs) != numGPUs {
		t.Fatalf("expected %d GPUs, got %d", numGPUs, len(gpuDriver.GPUs))
	}

	if s.GetComponentByName("Driver") == nil {
		t.Fatal("driver must be registered with the simulation")
	}

	cpName := "GPU[1].CommandProcessor"
	if s.GetComponentByName(cpName) == nil {
		t.Fatalf("component %s must be registered", cpName)
	}
}

func TestBuildR9NanoPlatform(t *testing.T) {
	buildPlatform(t, "r9nano", 1)
}

func TestBuildR9NanoMultiGPUPlatform(t *testing.T) {
	buildPlatform(t, "r9nano", 2)
}

func TestBuildMI300APlatform(t *testing.T) {
	buildPlatform(t, "mi300a", 1)
}
