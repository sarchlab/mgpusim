package platform

import (
	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3"
	"gitlab.com/akita/gcn3/driver"
	"gitlab.com/akita/gcn3/gpubuilder"
	"gitlab.com/akita/mem"
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
	//engine.AcceptHook(akita.NewEventLogger(log.New(os.Stdout, "", 0)))

	gpuDriver := driver.NewDriver(engine)
	connection := akita.NewDirectConnection(engine)

	gpuBuilder := gpubuilder.NewGPUBuilder(engine)
	gpuBuilder.Driver = gpuDriver
	if DebugISA {
		gpuBuilder.EnableISADebug = true
	}
	if TraceMem {
		gpuBuilder.EnableMemTracing = true
	}
	gpu, globalMem := gpuBuilder.BuildEmulationGPU()
	gpuDriver.RegisterGPU(gpu)

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
	*mem.IdealMemController,
) {
	var engine akita.Engine

	if UseParallelEngine {
		engine = akita.NewParallelEngine()
	} else {
		engine = akita.NewSerialEngine()
	}
	//engine.AcceptHook(akita.NewEventLogger(log.New(os.Stdout, "", 0)))

	gpuDriver := driver.NewDriver(engine)
	connection := akita.NewDirectConnection(engine)

	gpuBuilder := gpubuilder.NewGPUBuilder(engine)
	gpuBuilder.Driver = gpuDriver
	if DebugISA {
		gpuBuilder.EnableISADebug = true
	}
	if TraceInst {
		gpuBuilder.EnableInstTracing = true
	}
	if TraceMem {
		gpuBuilder.EnableMemTracing = true
	}

	gpu, globalMem := gpuBuilder.BuildR9Nano()
	gpuDriver.RegisterGPU(gpu)

	connection.PlugIn(gpuDriver.ToGPUs)
	connection.PlugIn(gpu.ToDriver)
	gpu.Driver = gpuDriver.ToGPUs

	return engine, gpu, gpuDriver, globalMem
}
