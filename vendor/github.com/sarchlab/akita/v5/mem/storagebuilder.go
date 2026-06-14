package mem

import "github.com/sarchlab/akita/v5/modeling"

// StorageBuilder builds Storage resources. When wired to a simulation through
// WithSimulation, the built storage registers itself as a simulation resource.
type StorageBuilder struct {
	capacity  uint64
	unitSize  uint64
	registrar modeling.Registrar
}

// MakeStorageBuilder returns a StorageBuilder with a default 4 KB unit size.
func MakeStorageBuilder() StorageBuilder {
	return StorageBuilder{
		unitSize: 4 * KB,
	}
}

// WithCapacity sets the capacity of the storage in bytes.
func (b StorageBuilder) WithCapacity(capacity uint64) StorageBuilder {
	b.capacity = capacity
	return b
}

// WithUnitSize sets the allocation unit size in bytes. A smaller unit reduces
// memory consumption for sparsely accessed storage.
func (b StorageBuilder) WithUnitSize(unitSize uint64) StorageBuilder {
	b.unitSize = unitSize
	return b
}

// WithSimulation wires the builder to a simulation so the built storage
// registers itself as a resource.
func (b StorageBuilder) WithSimulation(sim modeling.Registrar) StorageBuilder {
	b.registrar = sim
	return b
}

// Build creates the Storage with the given name. If the builder was wired to a
// simulation, the storage registers itself as a resource under that name.
func (b StorageBuilder) Build(name string) *Storage {
	storage := &Storage{
		name:     name,
		capacity: b.capacity,
		unitSize: b.unitSize,
		data:     make(map[uint64]*storageUnit),
	}

	if b.registrar != nil {
		b.registrar.RegisterResource(storage)
	}

	return storage
}
