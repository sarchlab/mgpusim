package driver

import (
	"slices"

	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// DefaultSpec returns the default configuration. Pages migrate between GPUs
// on faults, with the parameters of the NVIDIA UVM driver; a platform whose
// GPUs cannot migrate pages sets the policy to PageMigrationNone.
func DefaultSpec() Spec {
	return Spec{
		Freq: timing.GHz,
		PageMigration: PageMigrationSpec{
			Policy:                 PageMigrationFault,
			BatchSize:              256,
			QueueSize:              1024,
			AccessCounterThreshold: 256,
			PrefetchThreshold:      51,
			ThrashingThreshold:     3,
			ThrashingLapseNS:       500000,
			ThrashingNapNS:         500000,
			ThrashingEpochNS:       2000000000,
			PinThreshold:           10,
			PinNS:                  150000000,
			DataMoverPages:         64,
		},
	}
}

type Builder struct {
	spec      Spec
	resources Resources
	registrar modeling.Simulation
}

func MakeBuilder() Builder                                     { return Builder{spec: DefaultSpec()} }
func (b Builder) WithSimulation(s modeling.Simulation) Builder { b.registrar = s; return b }
func (b Builder) WithSpec(s Spec) Builder                      { b.spec = s; return b }
func (b Builder) WithResources(r Resources) Builder            { b.resources = r; return b }
func (b Builder) Build(name string) *Comp {
	if name != "Driver" {
		panic("driver: component must be named Driver")
	}
	if b.registrar == nil || b.resources.Runtime == nil || b.resources.PageTable == nil {
		panic("driver: simulation and live resources are required")
	}
	if err := b.resources.Runtime.CheckMemory(b.resources.PageTable, b.resources.GlobalStorage); err != nil {
		panic(err.Error())
	}
	if b.spec.UseMagicMemoryCopy && b.resources.GlobalStorage == nil {
		panic("driver: magic copy requires global storage")
	}
	if len(b.spec.GPUs) != b.resources.Runtime.GetNumGPUs() {
		panic("driver: GPU destinations and properties differ in length")
	}
	b.spec.GPUs = slices.Clone(b.spec.GPUs)
	return modeling.MakeBuilder[*Comp](b.spec).WithSimulation(b.registrar).WithResources(b.resources).Build(name)
}
