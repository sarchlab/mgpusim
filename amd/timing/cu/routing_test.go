package cu

import (
	"bytes"
	"reflect"
	"slices"
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
)

func TestVectorMemModulesIsImmutableCheckpointConfiguration(t *testing.T) {
	simulation := sim.MakeBuilder().Build()
	t.Cleanup(func() { _ = simulation.Terminate() })
	spec := DefaultSpec()
	spec.VectorMemModules = mem.AddressMapper{
		Destinations: []messaging.RemotePort{"A", "B"}, InterleavingSize: 64,
		RangeLow: 128, RangeHigh: 256, Fallback: "Remote",
	}
	c := MakeBuilder().WithSimulation(simulation).WithSpec(spec).Build("Component")
	spec.VectorMemModules.Destinations[0] = "Changed"
	if c.Spec.VectorMemModules.Find(128) != "A" ||
		c.Spec.VectorMemModules.Find(192) != "B" ||
		c.Spec.VectorMemModules.Find(0) != "Remote" {
		t.Fatal("routing changed after caller mutation or lost range/interleaving")
	}
	initializeRoutingComponent(t, simulation, c)
	var checkpoint bytes.Buffer
	if err := c.SaveCheckpoint(&checkpoint); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*mem.AddressMapper){
		func(m *mem.AddressMapper) { m.Destinations[0] = "Changed" },
		func(m *mem.AddressMapper) { m.InterleavingSize = 128 },
		func(m *mem.AddressMapper) { m.RangeLow = 64 },
		func(m *mem.AddressMapper) { m.RangeHigh = 512 },
		func(m *mem.AddressMapper) { m.Fallback = "Other" },
	} {
		otherSimulation := sim.MakeBuilder().Build()
		otherSpec := c.Spec
		otherSpec.VectorMemModules.Destinations = slices.Clone(c.Spec.VectorMemModules.Destinations)
		change(&otherSpec.VectorMemModules)
		other := MakeBuilder().WithSimulation(otherSimulation).WithSpec(otherSpec).Build("Component")
		initializeRoutingComponent(t, otherSimulation, other)
		if err := other.LoadCheckpoint(bytes.NewReader(checkpoint.Bytes())); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(other.Spec, otherSpec) || !reflect.DeepEqual(other.State, c.State) {
			t.Fatal("restore must load state and retain configured routing")
		}
		_ = otherSimulation.Terminate()
	}
}

func initializeRoutingComponent(t *testing.T, s *sim.Simulation, c *Comp) {
	t.Helper()
	for _, name := range []string{"Top", "InstMem", "ScalarMem", "VectorMem"} {
		c.BindPort(name, twowaybuffered.MakeBuilder().WithSimulation(s).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: 4, OutgoingCapacity: 4}).Build())
	}
	if err := s.Initialize(); err != nil {
		t.Fatal(err)
	}
}

func TestVectorMemModulesRejectsInvalidRouting(t *testing.T) {
	for _, mapper := range []mem.AddressMapper{
		{},
		{Destinations: []messaging.RemotePort{"A", "B"}},
		{Destinations: []messaging.RemotePort{""}},
		{Destinations: []messaging.RemotePort{"A"}, RangeLow: 128, RangeHigh: 64, Fallback: "Remote"},
	} {
		func() {
			s := sim.MakeBuilder().Build()
			defer s.Terminate()
			spec := DefaultSpec()
			spec.VectorMemModules = mem.AddressMapper{Destinations: []messaging.RemotePort{"Valid"}}
			spec.VectorMemModules = mapper
			c := MakeBuilder().WithSimulation(s).WithSpec(spec).Build("Component")
			defer func() {
				if recover() == nil {
					t.Error("invalid routing reached middleware initialization")
				}
			}()
			c.Middlewares.Initialize(c)
		}()
	}
}
