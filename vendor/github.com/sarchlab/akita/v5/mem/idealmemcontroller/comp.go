package idealmemcontroller

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/modeling"
)

// Comp is an ideal memory controller that can perform read and write.
// Ideal memory controller always responds to the request in a fixed number of
// cycles. There is no limitation on the concurrency of this unit.
type Comp struct {
	*modeling.Component[Spec, State]

	storage *mem.Storage
}

// GetStorage returns the underlying storage.
func (c *Comp) GetStorage() *mem.Storage {
	return c.storage
}

// StorageName returns the name used to identify this component's storage.
func (c *Comp) StorageName() string {
	return c.GetSpec().StorageRef
}
