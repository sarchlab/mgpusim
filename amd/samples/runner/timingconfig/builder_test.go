package timingconfig

import (
	"fmt"
	"testing"

	simulation "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/mgpusim/v5/amd/timing/dma"
	"github.com/sarchlab/mgpusim/v5/amd/timing/rdma"
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
		checkRouting(t, s, c, numGPUs)
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

func TestBuildMI300XMultiGPUPlatform(t *testing.T) { buildPlatform(t, "mi300x", 2) }

func checkRouting(t *testing.T, s *simulation.Simulation, c any, numGPUs int) {
	t.Helper()
	switch c := c.(type) {
	case *rdma.Comp:
		mapper := c.Spec.RemoteRDMAAddressTable
		if len(mapper.Destinations) != numGPUs+1 || mapper.Find(0) != "CPU" {
			t.Fatal("missing CPU/GPU routing slots")
		}
		for i := 1; i <= numGPUs; i++ {
			name := fmt.Sprintf("GPU[%d].RDMA", i)
			var destination messaging.RemotePort
			for _, peer := range s.Components() {
				if r, ok := peer.(*rdma.Comp); ok && r.Name() == name {
					destination = r.Ports.RDMADataOutside.AsRemote()
				}
			}
			if destination == "" || mapper.Find(uint64(i)*mapper.InterleavingSize) != destination {
				t.Fatalf("%s routes GPU %d incorrectly", c.Name(), i)
			}
		}
		if c.Spec.LocalModules.Fallback != c.Ports.RDMARequestInside.AsRemote() {
			t.Fatal("wrong local fallback")
		}
	case *dma.Comp:
		for _, dst := range c.Spec.LocalDataSource.Destinations {
			if dst == "" {
				t.Fatal("empty DMA destination")
			}
		}
		if len(c.Spec.LocalDataSource.Destinations) == 0 {
			t.Fatal("missing DMA routes")
		}
	}
}
