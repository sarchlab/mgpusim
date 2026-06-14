package vm

import "github.com/sarchlab/akita/v5/modeling"

// PageTableBuilder builds PageTable resources. When wired to a simulation
// through WithSimulation, the built page table registers itself as a resource.
type PageTableBuilder struct {
	log2PageSize uint64
	registrar    modeling.Registrar
}

// MakePageTableBuilder returns a PageTableBuilder with a default 12-bit (4 KB)
// page size.
func MakePageTableBuilder() PageTableBuilder {
	return PageTableBuilder{
		log2PageSize: 12,
	}
}

// WithLog2PageSize sets the log2 of the page size in bytes.
func (b PageTableBuilder) WithLog2PageSize(log2PageSize uint64) PageTableBuilder {
	b.log2PageSize = log2PageSize
	return b
}

// WithSimulation wires the builder to a simulation so the built page table
// registers itself as a resource.
func (b PageTableBuilder) WithSimulation(sim modeling.Registrar) PageTableBuilder {
	b.registrar = sim
	return b
}

// Build creates the PageTable with the given name. If the builder was wired to
// a simulation, the page table registers itself as a resource under that name.
func (b PageTableBuilder) Build(name string) PageTable {
	pt := &pageTableImpl{
		name:         name,
		log2PageSize: b.log2PageSize,
		tables:       make(map[PID]*processTable),
	}

	if b.registrar != nil {
		b.registrar.RegisterResource(pt)
	}

	return pt
}
