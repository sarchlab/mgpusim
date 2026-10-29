package timingconfig

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/vm/mmu"
	simulation "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
)

// TestBuildPageMigrationPlatforms assembles and initializes two-GPU
// platforms with fault-driven page migration. Initialization checks the port
// bindings and the routing of every component.
func TestBuildPageMigrationPlatforms(t *testing.T) {
	for _, tc := range []struct {
		gpuType  string
		log2Unit uint64
	}{
		{"r9nano", 12},
		{"mi300x", 21},
	} {
		t.Run(tc.gpuType, func(t *testing.T) {
			s := simulation.MakeBuilder().Build()
			defer s.Terminate()

			spec := driver.DefaultPageMigrationSpec()
			spec.Policy = driver.PageMigrationFault
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

			found := make(map[string]bool)
			for _, c := range s.Components() {
				found[c.Name()] = true
				checkRouting(t, s, c, 2)

				if m, ok := c.(*mmu.Comp); ok &&
					m.Spec.FaultHandler != "Driver.PageMigration" {
					t.Fatalf("MMU fault handler is %q", m.Spec.FaultHandler)
				}
			}

			for _, name := range []string{"GPU[1].DataMover", "GPU[2].DataMover"} {
				if !found[name] {
					t.Fatalf("component %s must be registered", name)
				}
			}
		})
	}
}
