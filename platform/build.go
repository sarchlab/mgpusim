package platform

import (
	"fmt"
	"log"
	"os"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3"
	"gitlab.com/akita/gcn3/driver"
	"gitlab.com/akita/gcn3/gpubuilder"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
	"gitlab.com/akita/mem/vm/mmu"
	"gitlab.com/akita/noc"
	"gitlab.com/akita/vis/trace"
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
	//engine.AcceptHook(akita.NewEventLogger(log.New(os.Stdout, "", 0)))

	mmuBuilder := mmu.MakeBuilder()
	mmuComponent := mmuBuilder.Build("MMU")
	gpuDriver := driver.NewDriver(engine, mmuComponent)
	connection := akita.NewDirectConnection(engine)

	gpuBuilder := gpubuilder.NewEmuGPUBuilder(engine)
	gpuBuilder.Driver = gpuDriver
	gpuBuilder.MMU = mmuComponent
	gpuBuilder.GPUMemAddrOffset = 4 * mem.GB
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
	engine.AcceptHook(akita.NewEventLogger(log.New(os.Stdout, "", 0)))

	mmuBuilder := mmu.MakeBuilder().
		WithEngine(engine).
		WithFreq(1 * akita.GHz)
	mmuComponent := mmuBuilder.Build("MMU")
	gpuDriver := driver.NewDriver(engine, mmuComponent)

	//connection := akita.NewDirectConnection(engine)
	connection := noc.NewFixedBandwidthConnection(32, engine, 1*akita.GHz)
	connection.SrcBufferCapacity = 40960000

	gpuBuilder := gpubuilder.NewR9NanoGPUBuilder().
		WithEngine(engine).
		WithExternalConn(connection).
		WithMMU(mmuComponent)

	if TraceVis {
		tracer := trace.NewMongoDBTracer()
		tracer.Init()
		hook := trace.NewHook(tracer)
		gpuBuilder.SetTraceHook(hook)

		gpuDriver.AcceptHook(hook)
	}

	rdmaAddressTable := new(cache.BankedLowModuleFinder)
	rdmaAddressTable.BankSize = 4 * mem.GB
	rdmaAddressTable.LowModules = append(rdmaAddressTable.LowModules, nil)
	for i := 1; i < numGPUs+1; i++ {
		name := fmt.Sprintf("GPU_%d", i)
		memAddrOffset := uint64(i) * 4 * mem.GB
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
	}

	connection.PlugIn(gpuDriver.ToGPUs)
	connection.PlugIn(mmuComponent.ToTop)

	return engine, gpuDriver
}
