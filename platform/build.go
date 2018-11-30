package platform

import (
	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3"
	"gitlab.com/akita/gcn3/driver"
	"gitlab.com/akita/gcn3/gpubuilder"
	"gitlab.com/akita/mem"
	"gitlab.com/akita/mem/vm"
)

var UseParallelEngine bool
var DebugISA bool
var TraceInst bool
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

	mmu := vm.NewMMU("MMU", engine)
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

//BuildR9NanoPlatform creates a platform that equips with a R9Nano GPU
func BuildR9NanoPlatform() (
	akita.Engine,
	*gcn3.GPU,
	*driver.Driver,
) {
	var engine akita.Engine

	if UseParallelEngine {
		engine = akita.NewParallelEngine()
	} else {
		engine = akita.NewSerialEngine()
	}
	//engine.AcceptHook(akita.NewEventLogger(log.New(os.Stdout, "", 0)))

	mmu := vm.NewMMU("MMU", engine)
	gpuDriver := driver.NewDriver(engine, mmu)
	connection := akita.NewDirectConnection(engine)

	gpuBuilder := gpubuilder.R9NanoGPUBuilder{
		GPUName:           "GPU",
		Engine:            engine,
		Driver:            gpuDriver,
		EnableISADebug:    DebugISA,
		EnableMemTracing:  TraceMem,
		EnableInstTracing: TraceInst,
		MMU:               mmu,
		ExternalConn:      connection,
	}

	gpu := gpuBuilder.Build()
	gpuDriver.RegisterGPU(gpu, 4*mem.GB)

	connection.PlugIn(gpuDriver.ToGPUs)
	connection.PlugIn(gpu.ToDriver)
	connection.PlugIn(mmu.ToTop)
	gpu.Driver = gpuDriver.ToGPUs

	return engine, gpu, gpuDriver
}
