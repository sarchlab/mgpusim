package driver

import (
	"slices"

	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
)

func DefaultSpec() Spec { return Spec{Freq: timing.GHz} }

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
	if b.resources.Runtime.pageTable != b.resources.PageTable {
		panic("driver: Runtime and component page tables differ")
	}
	if b.resources.Runtime.storage != b.resources.GlobalStorage {
		panic("driver: Runtime and component storage differ")
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
