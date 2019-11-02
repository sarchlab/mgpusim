// Package platform provides predefined platform definitions.
package platform

import (
	"fmt"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3/driver"
	"gitlab.com/akita/gcn3/gpubuilder"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
	"gitlab.com/akita/mem/vm/mmu"
	"gitlab.com/akita/noc/networking/pcie"
	"gitlab.com/akita/util/tracing"
)

var UseParallelEngine bool
var DebugISA bool
var TraceVis bool
var TraceMem bool

const log2PageSize = 12

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

	mmuBuilder := mmu.MakeBuilder().
		WithLog2PageSize(log2PageSize)
	mmuComponent := mmuBuilder.Build("MMU")
	gpuDriver := driver.NewDriver(engine, mmuComponent, log2PageSize)
	connection := akita.NewDirectConnection("ExternalConn", engine, 1*akita.GHz)
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
		connection.PlugIn(gpu.CommandProcessor.ToDriver, 64)
	}

	connection.PlugIn(gpuDriver.ToGPUs, 4)

	return engine, gpuDriver
}

//nolint:gocyclo,funlen
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

	mmuBuilder := mmu.MakeBuilder().
		WithEngine(engine).
		WithFreq(1 * akita.GHz).
		WithLog2PageSize(log2PageSize)
	mmuComponent := mmuBuilder.Build("MMU")
	gpuDriver := driver.NewDriver(engine, mmuComponent, log2PageSize)

	//connection := akita.NewDirectConnection(engine)
	// connection := noc.NewFixedBandwidthConnection(32, engine, 1*akita.GHz)
	// connection.SrcBufferCapacity = 40960000
	pcieConnector := new(pcie.Connector).
		WithEngine(engine).
		WithVersion3().
		WithX16().
		WithNetworkName("PCIe")
	pcieConnector.CreateNetwork()
	rootComplexID := pcieConnector.CreateRootComplex(
		[]akita.Port{
			gpuDriver.ToGPUs,
			gpuDriver.ToMMU,
			mmuComponent.MigrationPort,
			mmuComponent.ToTop,
		})

	mmuComponent.MigrationServiceProvider = gpuDriver.ToMMU

	gpuBuilder := gpubuilder.MakeR9NanoGPUBuilder().
		WithEngine(engine).
		WithMMU(mmuComponent).
		WithNumCUPerShaderArray(4).
		WithNumShaderArray(16).
		WithNumMemoryBank(8).
		WithLog2PageSize(log2PageSize)

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

	lastSwitchID := rootComplexID
	for i := 1; i < numGPUs+1; i++ {
		if i%2 == 1 {
			lastSwitchID = pcieConnector.AddSwitch(lastSwitchID)
		}

		name := fmt.Sprintf("GPU%d", i)
		memAddrOffset := uint64(i) * 4 * mem.GB
		gpu := gpuBuilder.
			WithMemAddrOffset(memAddrOffset).
			Build(name, uint64(i))
		gpuDriver.RegisterGPU(gpu, 4*mem.GB)
		gpu.CommandProcessor.Driver = gpuDriver.ToGPUs

		gpu.RDMAEngine.RemoteRDMAAddressTable = rdmaAddressTable
		rdmaAddressTable.LowModules = append(
			rdmaAddressTable.LowModules,
			gpu.RDMAEngine.ToOutside)

		gpu.PMC.RemotePMCAddressTable = pmcAddressTable
		pmcAddressTable.LowModules = append(
			pmcAddressTable.LowModules,
			gpu.PMC.RemotePort)

		gpuDriver.RemotePMCPorts = append(
			gpuDriver.RemotePMCPorts, gpu.PMC.RemotePort)

		pcieConnector.PlugInDevice(lastSwitchID, gpu.ExternalPorts())
	}

	return engine, gpuDriver
}
