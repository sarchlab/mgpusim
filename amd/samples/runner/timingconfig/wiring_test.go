package timingconfig_test

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig/shaderarray"
	"testing"
)

func TestCUDestinationsUseBuiltROBPorts(t *testing.T) {
	s := sim.MakeBuilder().Build()
	t.Cleanup(func() {
		if err := s.Terminate(); err != nil {
			t.Error(err)
		}
	})
	sa := shaderarray.MakeBuilder().WithSimulation(s).WithNumCUs(2).
		WithL1AddressMapper(&mem.AddressMapper{Destinations: []messaging.RemotePort{"Memory.Top"}}).
		WithL1TLBAddressMapper(&mem.AddressMapper{Destinations: []messaging.RemotePort{"Memory.Top"}}).
		Build("CustomShaderArray")
	for _, c := range sa.CUs {
		if c.Spec.InstMem != sa.L1IROB.Ports.Top.AsRemote() || c.Spec.ScalarMem != sa.L1SROB.Ports.Top.AsRemote() {
			t.Fatal("CU Spec does not refer to the actual bound ROB ports")
		}
		if c.Middlewares.CU != nil {
			t.Fatal("CU behavior was constructed before Initialize")
		}
	}
	if err := s.Initialize(); err != nil {
		t.Fatal(err)
	}
	for _, c := range sa.CUs {
		if c.Middlewares.CU == nil {
			t.Fatal("Initialize did not construct CU behavior")
		}
	}
}
