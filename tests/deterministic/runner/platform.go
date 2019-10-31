package runner

import (
	"fmt"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3/driver"
	"gitlab.com/akita/gcn3/gpubuilder"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/cache"
	"gitlab.com/akita/mem/vm/mmu"
)

var UseParallelEngine bool
var DebugISA bool
var TraceVis bool
var TraceMem bool

func buildNR9NanoPlatformWithPerfectMemorySystem(
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
	gpuDriver := driver.NewDriver(engine, mmuComponent, 12)

	connection := akita.NewDirectConnection("ExternalConn", engine, 1*akita.GHz)

	gpuBuilder := gpubuilder.MakeR9NanoGPUBuilder().
		WithEngine(engine).
		WithMMU(mmuComponent).
		WithNumCUPerShaderArray(4).
		WithNumShaderArray(16).
		WithNumMemoryBank(8).
		WithLog2PageSize(12)

	if TraceMem {
		gpuBuilder.EnableMemTracing = true
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
		gpu.CommandProcessor.Driver = gpuDriver.ToGPUs

		gpu.RDMAEngine.RemoteRDMAAddressTable = rdmaAddressTable
		rdmaAddressTable.LowModules = append(
			rdmaAddressTable.LowModules,
			gpu.RDMAEngine.ToOutside)

		for _, port := range gpu.ExternalPorts() {
			connection.PlugIn(port, 64)
		}
	}

	connection.PlugIn(gpuDriver.ToGPUs, 1)
	connection.PlugIn(mmuComponent.ToTop, 1)

	return engine, gpuDriver
}
