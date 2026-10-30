package timingconfig

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/vm/mmu"
	simulation "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/timing/rdma"
)

// TestBuildPageMigrationPlatforms assembles and initializes two-GPU
// platforms with fault-driven and with access-counter-driven page migration.
// Initialization checks the port bindings and the routing of every
// component.
func TestBuildPageMigrationPlatforms(t *testing.T) {
	for _, tc := range []struct {
		gpuType  string
		log2Unit uint64
	}{
		{"r9nano", 12},
		{"mi300x", 21},
	} {
		for _, policy := range []string{
			driver.PageMigrationFault, driver.PageMigrationCounter,
		} {
			t.Run(tc.gpuType+"/"+policy, func(t *testing.T) {
				s := simulation.MakeBuilder().Build()
				defer s.Terminate()

				spec := driver.DefaultPageMigrationSpec()
				spec.Policy = policy
				spec.Log2Unit = tc.log2Unit

				MakeBuilder().
					WithSimulation(s).
					WithNumGPUs(2).
					WithGPUType(tc.gpuType).
					WithPageMigration(spec).
					Build()

				if err := s.Initialize(); err != nil {
					t.Fatal(err)
				}

				checkPageMigrationPlatform(t, s, spec)
			})
		}
	}
}

func checkPageMigrationPlatform(
	t *testing.T,
	s *simulation.Simulation,
	spec driver.PageMigrationSpec,
) {
	t.Helper()

	// The MMU faults only with the fault policy; the RDMA engines count only
	// with the counter policy.
	handler := messaging.RemotePort("Driver.PageMigration")
	faultHandler, counterHandler := handler, messaging.RemotePort("")

	if spec.Policy == driver.PageMigrationCounter {
		faultHandler, counterHandler = "", handler
	}

	found := make(map[string]bool)

	for _, c := range s.Components() {
		found[c.Name()] = true
		checkRouting(t, s, c, 2)

		switch c := c.(type) {
		case *mmu.Comp:
			if c.Spec.FaultHandler != faultHandler {
				t.Fatalf("MMU fault handler is %q, want %q",
					c.Spec.FaultHandler, faultHandler)
			}
		case *rdma.Comp:
			if c.Spec.AccessCounterHandler != counterHandler ||
				c.Spec.AccessCounterLog2Unit != spec.Log2Unit && counterHandler != "" {
				t.Fatalf("%s counters: %+v", c.Name(), c.Spec)
			}
		}
	}

	for _, name := range []string{"GPU[1].DataMover", "GPU[2].DataMover"} {
		if !found[name] {
			t.Fatalf("component %s must be registered", name)
		}
	}
}
