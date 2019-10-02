package platform

import (
	"fmt"
	"log"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3/driver"
	"gitlab.com/akita/gcn3/gpubuilder"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
	"gitlab.com/akita/mem/vm/mmu"
	"gitlab.com/akita/noc"
	"gitlab.com/akita/util/tracing"
)

var UseParallelEngine bool
var DebugISA bool
var TraceVis bool
var TraceMem bool

// BuildNEmuGPUPlatform creates a simple platform for emulation purposes
func BuildNEmuGPUPlatform(n int) (
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

	mmuBuilder := mmu.MakeBuilder()
	mmuComponent := mmuBuilder.Build("MMU")
	gpuDriver := driver.NewDriver(engine, mmuComponent)
	connection := akita.NewDirectConnection(engine)
	storage := mem.NewStorage(uint64(n+1) * 4 * mem.GB)

	gpuBuilder := gpubuilder.MakeEmuGPUBuilder().
		WithEngine(engine).
		WithDriver(gpuDriver).
		WithIOMMU(mmuComponent).
		WithMemCapacity(4 * mem.GB).
		WithStorage(storage)

	if DebugISA {
		gpuBuilder.EnableISADebug = true
	}
	if TraceMem {
		gpuBuilder.EnableMemTracing = true
	}

	for i := 0; i < n; i++ {
		gpu := gpuBuilder.
			WithMemOffset(uint64(i+1) * 4 * mem.GB).
			Build(fmt.Sprintf("GPU_%d", i+1))

		gpuDriver.RegisterGPU(gpu, 4*mem.GB)
		connection.PlugIn(gpu.ToDriver)
	}

	connection.PlugIn(gpuDriver.ToGPUs)

	return engine, gpuDriver
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
	//engine.AcceptHook(akita.NewEventLogger(log.New(os.Stdout, "", 0)))

	mmuBuilder := mmu.MakeBuilder().
		WithEngine(engine).
		WithFreq(1 * akita.GHz)
	mmuComponent := mmuBuilder.Build("MMU")
	gpuDriver := driver.NewDriver(engine, mmuComponent)

	//connection := akita.NewDirectConnection(engine)
	connection := noc.NewFixedBandwidthConnection(32, engine, 1*akita.GHz)
	connection.SrcBufferCapacity = 40960000

	mmuComponent.MigrationServiceProvider = gpuDriver.ToMMU

	gpuBuilder := gpubuilder.MakeR9NanoGPUBuilder().
		WithEngine(engine).
		WithExternalConn(connection).
		WithMMU(mmuComponent).
		WithNumCUPerShaderArray(4).
		WithNumShaderArray(16).
		WithNumMemoryBank(8)

	if TraceVis {
		tracer := tracing.NewMongoDBTracer()
		tracer.Init()
		tracing.CollectTrace(gpuDriver, tracer)

		gpuBuilder = gpuBuilder.WithVisTracer(tracer)
	}

	if TraceMem {
		gpuBuilder.EnableMemTracing = true
	}

	if DebugISA {
		gpuBuilder.EnableISADebug = true
	}

	rdmaAddressTable := new(cache.BankedLowModuleFinder)
	rdmaAddressTable.BankSize = 4 * mem.GB
	rdmaAddressTable.LowModules = append(rdmaAddressTable.LowModules, nil)

	pmcAddressTable := new(cache.BankedLowModuleFinder)
	pmcAddressTable.BankSize = 4 * mem.GB
	pmcAddressTable.LowModules = append(pmcAddressTable.LowModules, nil)

	for i := 1; i < numGPUs+1; i++ {
		name := fmt.Sprintf("GPU_%d", i)
		memAddrOffset := uint64(i) * 4 * mem.GB
		log.Printf("offset is %d \n", memAddrOffset)

		gpu := gpuBuilder.
			WithMemAddrOffset(memAddrOffset).
			Build(name, uint64(i))
		gpuDriver.RegisterGPU(gpu, 4*mem.GB)
		gpu.Driver = gpuDriver.ToGPUs

		gpu.RDMAEngine.RemoteRDMAAddressTable = rdmaAddressTable
		rdmaAddressTable.LowModules = append(
			rdmaAddressTable.LowModules,
			gpu.RDMAEngine.ToOutside)
		connection.PlugIn(gpu.RDMAEngine.ToOutside)

		gpu.PMC.RemotePMCAddressTable = pmcAddressTable
		pmcAddressTable.LowModules = append(
			pmcAddressTable.LowModules,
			gpu.PMC.RemotePort)
		connection.PlugIn(gpu.PMC.RemotePort)

		gpuDriver.RemotePMCPorts = append(gpuDriver.RemotePMCPorts, gpu.PMC.RemotePort)

	}

	connection.PlugIn(gpuDriver.ToGPUs)
	connection.PlugIn(mmuComponent.ToTop)
	connection.PlugIn(mmuComponent.MigrationPort)
	connection.PlugIn(gpuDriver.ToMMU)

	return engine, gpuDriver
}
