package driver

import (
	"sync"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/driver/internal"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// DefaultSpec returns a copy of the default configuration. Callers obtain it,
// tweak the fields they care about, and pass it to WithSpec.
func DefaultSpec() Spec {
	return Spec{
		Freq:         1 * timing.GHz,
		Log2PageSize: 12,
	}
}

// A Builder can build a driver.
type Builder struct {
	spec      Spec
	resources Resources
	registrar modeling.Simulation
}

// MakeBuilder creates a driver builder seeded with the default spec.
func MakeBuilder() Builder {
	return Builder{spec: DefaultSpec()}
}

// WithSimulation wires the builder to a registrar (a *simulation.Simulation in
// assembly, or modeling.NewStandaloneRegistrar(engine) in isolated tests).
func (b Builder) WithSimulation(reg modeling.Simulation) Builder {
	b.registrar = reg
	return b
}

// WithSpec sets the entire configuration. Start from DefaultSpec() and tweak.
func (b Builder) WithSpec(spec Spec) Builder {
	b.spec = spec
	return b
}

// WithResources sets the shared resources (page table, global storage) the
// driver uses.
func (b Builder) WithResources(resources Resources) Builder {
	b.resources = resources
	return b
}

// Build creates a driver. It declares the driver's "GPU" port; the port
// instance is created externally and supplied with AssignPort.
func (b Builder) Build(name string) *Driver {
	if b.registrar == nil {
		panic("driver: WithSimulation is required")
	}

	driver := new(Driver)
	resources := b.resources
	resources.Runtime = driver
	driver.Comp = modeling.MakeBuilder[*Comp](b.spec).WithSimulation(b.registrar).WithResources(resources).Build(name)
	driver.State = State{CommandTasks: make(map[uint64]uint64), RequestTasks: make(map[uint64]uint64)}

	driver.engine = b.registrar.Engine()
	driver.Log2PageSize = b.spec.Log2PageSize

	memAllocatorImpl := internal.NewMemoryAllocator(
		b.resources.PageTable, b.spec.Log2PageSize)
	driver.memAllocator = memAllocatorImpl

	distributorImpl := newDistributorImpl(memAllocatorImpl)
	distributorImpl.pageSizeAsPowerOf2 = b.spec.Log2PageSize
	driver.distributor = distributorImpl

	if b.spec.UseMagicMemoryCopy {
		globalStorageMemoryCopyMiddleware := &globalStorageMemoryCopyMiddleware{
			driver: driver,
		}
		driver.middlewares = append(driver.middlewares,
			globalStorageMemoryCopyMiddleware)
	} else {
		defaultMemoryCopyMiddleware := &defaultMemoryCopyMiddleware{
			driver:       driver,
			cyclesPerD2H: b.spec.D2HCycles,
			cyclesPerH2D: b.spec.H2DCycles,
		}
		driver.middlewares = append(driver.middlewares,
			defaultMemoryCopyMiddleware)
	}

	driver.enqueueSignal = make(chan bool)
	driver.driverStopped = make(chan bool)
	driver.engineIdle = sync.NewCond(&driver.engineRunningMutex)
	driver.codeObjGPUAddrs = make(map[*insts.KernelCodeObject]Ptr)

	driver.unboundResults = make(map[uint64][]byte)
	b.createCPU(driver)
	b.registrar.RegisterResource(&driverCheckpoint{driver})

	return driver
}

func (b *Builder) createCPU(d *Driver) {
	cpu := &internal.Device{
		ID:       0,
		Type:     internal.DeviceTypeCPU,
		MemState: internal.NewDeviceMemoryState(d.Log2PageSize),
	}
	cpu.SetTotalMemSize(4 * mem.GB)

	d.memAllocator.RegisterDevice(cpu)
	d.devices = append(d.devices, cpu)
}
