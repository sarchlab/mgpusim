package platform

import (
	"fmt"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3"
	"gitlab.com/akita/gcn3/driver"
	"gitlab.com/akita/gcn3/gpubuilder"
	"gitlab.com/akita/gcn3/trace"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
	"gitlab.com/akita/mem/vm"
	"gitlab.com/akita/noc"
)

var UseParallelEngine bool
var DebugISA bool
var TraceVis bool
var TraceMem bool

// BuildEmuPlatform creates a simple platform for emulation purposes
func BuildEmuPlatform() (
	akita.Engine,
	*gcn3.GPU,
	*driver.Driver,
	*mem.IdealMemController,
) {
	var engine akita.Engine

	if UseParallelEngine {
		engine = akita.NewParallelEngine()
	} else {
		engine = akita.NewSerialEngine()
	}
	// engine.AcceptHook(akita.NewEventLogger(log.New(os.Stdout, "", 0)))

	mmu := vm.NewMMU("MMU", engine, &vm.DefaultPageTableFactory{})
	gpuDriver := driver.NewDriver(engine, mmu)
	connection := akita.NewDirectConnection(engine)

	gpuBuilder := gpubuilder.NewEmuGPUBuilder(engine)
	gpuBuilder.Driver = gpuDriver
	gpuBuilder.MMU = mmu
	if DebugISA {
		gpuBuilder.EnableISADebug = true
	}
	if TraceMem {
		gpuBuilder.EnableMemTracing = true
	}
	gpu, globalMem := gpuBuilder.BuildEmulationGPU()
	gpuDriver.RegisterGPU(gpu, 4*mem.GB)

	connection.PlugIn(gpuDriver.ToGPUs)
	connection.PlugIn(gpu.ToDriver)
	gpu.Driver = gpuDriver.ToGPUs

	return engine, gpu, gpuDriver, globalMem
}

//BuildNR9NanoPlatform creates a platform that equips with several R9Nano GPUs
func BuildNR9NanoPlatform(
	numGPUs int,
) (
	akita.Engine,
	*driver.Driver,
) {
	var engine akita.Engine

	if UseParallelEngine {
		engine = akita.NewParallelEngine()
	} else {
		engine = akita.NewSerialEngine()
	}
	// engine.AcceptHook(akita.NewEventLogger(log.New(os.Stdout, "", 0)))

	mmu := vm.NewMMU("MMU", engine, &vm.DefaultPageTableFactory{})
	mmu.Latency = 100
	mmu.ShootdownLatency = 50
	gpuDriver := driver.NewDriver(engine, mmu)
	//connection := akita.NewDirectConnection(engine)
	connection := noc.NewFixedBandwidthConnection(32, engine, 1*akita.GHz)
	connection.SrcBufferCapacity = 40960000

	gpuBuilder := gpubuilder.R9NanoGPUBuilder{
		GPUName:           "GPU",
		Engine:            engine,
		Driver:            gpuDriver,
		EnableISADebug:    DebugISA,
		EnableMemTracing:  TraceMem,
		EnableInstTracing: TraceVis,
		EnableVisTracing:  TraceVis,
		MMU:               mmu,
		ExternalConn:      connection,
	}

	if TraceVis {
		tracer := &trace.Tracer{}
		tracer.Init()
		gpuBuilder.Tracer = tracer

		driverCommandTracer := trace.NewDriverCommandTracer(tracer)
		gpuDriver.AcceptHook(driverCommandTracer)
		engine.RegisterSimulationEndHandler(driverCommandTracer)

		driverReqTracer := trace.NewDriverRequestTracer(tracer)
		gpuDriver.AcceptHook(driverReqTracer)
	}

	rdmaAddressTable := new(cache.BankedLowModuleFinder)
	rdmaAddressTable.BankSize = 4 * mem.GB
	rdmaAddressTable.LowModules = append(rdmaAddressTable.LowModules, nil)
	for i := 1; i < numGPUs+1; i++ {
		gpuBuilder.GPUName = fmt.Sprintf("GPU_%d", i)
		gpuBuilder.GPUMemAddrOffset = uint64(i) * 4 * mem.GB
		gpu := gpuBuilder.Build()
		gpuDriver.RegisterGPU(gpu, 4*mem.GB)
		gpu.Driver = gpuDriver.ToGPUs

		gpu.RDMAEngine.RemoteRDMAAddressTable = rdmaAddressTable
		rdmaAddressTable.LowModules = append(
			rdmaAddressTable.LowModules,
			gpu.RDMAEngine.ToOutside)
		connection.PlugIn(gpu.RDMAEngine.ToOutside)
	}

	connection.PlugIn(gpuDriver.ToGPUs)
	connection.PlugIn(mmu.ToTop)

	return engine, gpuDriver
}
