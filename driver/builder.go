package driver

import (
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/mgpusim/v3/driver/internal"
)

// A Builder can build a driver.
type Builder struct {
	engine              sim.Engine
	freq                sim.Freq
	log2PageSize        uint64
	pageTable           vm.PageTable
	globalStorage       *mem.Storage
	useMagicMemoryCopy  bool
	middlewareD2HCycles int
	middlewareH2DCycles int
}

// MakeBuilder creates a driver builder with some default configuration
// parameters.
func MakeBuilder() Builder {
	return Builder{
		freq: 1 * sim.GHz,
	}
}

// WithEngine sets the engine to use.
func (b Builder) WithEngine(e sim.Engine) Builder {
	b.engine = e
	return b
}

// WithFreq sets the frequency to use.
func (b Builder) WithFreq(freq sim.Freq) Builder {
	b.freq = freq
	return b
}

// WithPageTable sets the global page table.
func (b Builder) WithPageTable(pt vm.PageTable) Builder {
	b.pageTable = pt
	return b
}

// WithLog2PageSize sets the page size used by all the devices in the system
// as a power of 2.
func (b Builder) WithLog2PageSize(log2PageSize uint64) Builder {
	b.log2PageSize = log2PageSize
	return b
}

// WithGlobalStorage sets the global storage that the driver uses.
func (b Builder) WithGlobalStorage(storage *mem.Storage) Builder {
	b.globalStorage = storage
	return b
}

// WithMagicMemoryCopyMiddleware uses global storage as memory components
func (b Builder) WithMagicMemoryCopyMiddleware() Builder {
	b.useMagicMemoryCopy = true
	return b
}

func (b Builder) WithD2HCycles(d2hCycles int) Builder {
	b.middlewareD2HCycles = d2hCycles
	return b
}

func (b Builder) WithH2DCycles(h2dCycles int) Builder {
	b.middlewareH2DCycles = h2dCycles
	return b
}

// Build creates a driver.
func (b Builder) Build(name string) *Driver {
	driver := new(Driver)
	driver.TickingComponent = sim.NewTickingComponent(
		"Driver", b.engine, b.freq, driver)

	driver.Log2PageSize = b.log2PageSize

	memAllocatorImpl := internal.NewMemoryAllocator(b.pageTable, b.log2PageSize)
	driver.memAllocator = memAllocatorImpl

	distributorImpl := newDistributorImpl(memAllocatorImpl)
	distributorImpl.pageSizeAsPowerOf2 = b.log2PageSize
	driver.distributor = distributorImpl

	driver.pageTable = b.pageTable
	driver.globalStorage = b.globalStorage

	if b.useMagicMemoryCopy {
		globalStorageMemoryCopyMiddleware := &globalStorageMemoryCopyMiddleware{
			driver: driver,
		}
		driver.middlewares = append(driver.middlewares, globalStorageMemoryCopyMiddleware)
	} else {
		defaultMemoryCopyMiddleware := &defaultMemoryCopyMiddleware{
			driver:       driver,
			cyclesPerD2H: b.middlewareD2HCycles,
			cyclesPerH2D: b.middlewareH2DCycles,
		}
		driver.middlewares = append(driver.middlewares, defaultMemoryCopyMiddleware)
	}

	driver.gpuPort = sim.NewLimitNumMsgPort(driver, 40960000, "Driver.ToGPUs")
	driver.AddPort("GPU", driver.gpuPort)
	driver.mmuPort = sim.NewLimitNumMsgPort(driver, 1, "Driver.ToMMU")
	driver.AddPort("MMU", driver.mmuPort)

	driver.enqueueSignal = make(chan bool)
	driver.driverStopped = make(chan bool)

	b.createCPU(driver)

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
