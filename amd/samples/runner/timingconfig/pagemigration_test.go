package timingconfig

import (
	"testing"

	"github.com/sarchlab/akita/v5/simulation"
	"github.com/sarchlab/mgpusim/v5/amd/timing/pagemigration"
)

// TestBuildPageMigrationPlatforms assembles two-GPU platforms with every
// page-migration policy, which panics if a port is not declared, not
// assigned, or not connected, or if a Spec cannot be checkpointed.
func TestBuildPageMigrationPlatforms(t *testing.T) {
	for _, tc := range []struct {
		gpuType string
		policy  pagemigration.Policy
		unit    uint64
	}{
		{"r9nano", pagemigration.PolicyObserve, 12},
		{"r9nano", pagemigration.PolicyOnFault, 12},
		{"r9nano", pagemigration.PolicyAccessCounter, 21},
		{"mi300x", pagemigration.PolicyOnFault, 21},
		{"mi300x", pagemigration.PolicyAccessCounter, 21},
	} {
		t.Run(tc.gpuType+"/"+tc.policy.String(), func(t *testing.T) {
			s := simulation.MakeBuilder().
				WithoutMonitoring().
				WithOutputFileName(t.TempDir() + "/sim").
				Build()
			defer s.Terminate()

			spec := pagemigration.DefaultSpec()
			spec.Policy = tc.policy
			spec.Log2Unit = tc.unit

			MakeBuilder().
				WithSimulation(s).
				WithNumGPUs(2).
				WithGPUType(tc.gpuType).
				WithPageMigration(spec).
				Build()

			if tc.policy.String() == "observe" {
				return
			}

			for _, name := range []string{
				"GPU[1].PageMigrationDataMover",
				"GPU[2].PageMigrationDataMover",
			} {
				if s.GetComponentByName(name) == nil {
					t.Fatalf("component %s must be registered", name)
				}
			}
		})
	}
}
