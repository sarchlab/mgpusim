package driver

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/mgpusim/v5/amd/driver/internal"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// defaultSpec provides the default configuration of the driver.
var defaultSpec = Spec{
	Freq:         1 * timing.GHz,
	Log2PageSize: 12,
}

// DefaultSpec returns a copy of the default configuration. Callers obtain it,
// tweak the fields they care about, and pass it to WithSpec.
func DefaultSpec() Spec {
	return defaultSpec
}

// A Builder can build a driver.
type Builder struct {
	spec      Spec
	resources Resources
	registrar modeling.Registrar
}

// MakeBuilder creates a driver builder seeded with the default spec.
func MakeBuilder() Builder {
	return Builder{spec: defaultSpec}
}

// WithRegistrar wires the builder to a registrar (a *simulation.Simulation in
// assembly, or modeling.NewStandaloneRegistrar(engine) in isolated tests).
func (b Builder) WithRegistrar(reg modeling.Registrar) Builder {
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
		panic("driver: WithRegistrar is required")
	}

	driver := new(Driver)
	driver.Comp = modeling.NewBuilder[Spec, State, Resources]().
		WithEngine(b.registrar.GetEngine()).
		WithFreq(b.spec.Freq).
		WithSpec(b.spec).
		WithResources(b.resources).
		Build(name)
	driver.Comp.State = State{}

	driver.engine = b.registrar.GetEngine()
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

	// The middleware order mirrors the v4 tick order: send, memory-copy
	// middleware ticks, then return/command processing.
	driver.Comp.AddMiddleware(&sendMW{driver: driver})
	driver.Comp.AddMiddleware(&driverMiddlewaresMW{driver: driver})
	driver.Comp.AddMiddleware(&commandMW{driver: driver})

	driver.Comp.DeclarePort(GPUPortName)

	driver.enqueueSignal = make(chan bool)
	driver.driverStopped = make(chan bool)
	driver.codeObjGPUAddrs = make(map[*insts.KernelCodeObject]Ptr)

	b.createCPU(driver)

	b.registrar.RegisterComponent(driver.Comp)

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
