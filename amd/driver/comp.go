package driver

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// GPUPortName is the logical name of the port the driver uses to talk to the
// GPUs' command processors. The port instance is created externally (in the
// platform configuration or test setup) and supplied with AssignPort.
const GPUPortName = "GPU"

// Spec contains the immutable configuration of the driver.
type Spec struct {
	Freq timing.Freq `json:"freq"`

	// Log2PageSize is the page size used by all the devices in the system,
	// as a power of 2.
	Log2PageSize uint64 `json:"log2_page_size"`

	// UseMagicMemoryCopy makes the driver copy memory directly through the
	// global storage rather than sending memory-copy requests to the GPUs.
	UseMagicMemoryCopy bool `json:"use_magic_memory_copy"`

	// D2HCycles and H2DCycles are the number of cycles the driver waits
	// before issuing the requests of a device-to-host or host-to-device
	// memory copy command.
	D2HCycles int `json:"d2h_cycles"`
	H2DCycles int `json:"h2d_cycles"`
}

// State contains the mutable runtime data of the driver.
//
// Contexts, queues, allocation state and outstanding copies are serialized by
// the named Runtime resource, which restores local object links from values.
type State struct {
	NextPID                    uint64
	CommandTasks, RequestTasks map[uint64]uint64
}

// Resources contains the references to the shared resources the driver uses.
type Resources struct {
	Runtime *Driver
	// PageTable is the global page table shared with the MMUs.
	PageTable *vm.PageTable

	// GlobalStorage is the global storage that backs all the simulated
	// memories.
	GlobalStorage *mem.Storage
}

// Comp is the Akita component of the driver.
type Comp = ticking.Component[Spec, State, Resources, Ports, Middlewares]
type Ports struct {
	GPU messaging.Port `akita:"role=github.com/sarchlab/mgpusim/v5/amd/protocol.driver"`
}
type Middlewares struct {
	Send    *sendMW
	Memory  *driverMiddlewaresMW
	Command *commandMW
}

func (m *Middlewares) Initialize(c *Comp) {
	d := c.Resources.Runtime
	m.Send = &sendMW{driver: d}
	m.Memory = &driverMiddlewaresMW{driver: d}
	m.Command = &commandMW{driver: d}
}
