package platform

import (
	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3"
	"gitlab.com/yaotsu/gcn3/driver"
	"gitlab.com/yaotsu/gcn3/gpubuilder"
	"gitlab.com/yaotsu/mem"
)

var UseParallelEngine bool
var DebugISA bool
var TraceInst bool
var TraceMem bool

// BuildEmuPlatform creates a simple platform for emulation purposes
func BuildEmuPlatform() (
	core.Engine,
	*gcn3.GPU,
	*driver.Driver,
	*mem.IdealMemController,
) {
	var engine core.Engine

	if UseParallelEngine {
		engine = core.NewParallelEngine()
	} else {
		engine = core.NewSerialEngine()
	}
	//engine.AcceptHook(util.NewEventLogger(log.New(os.Stdout, "", 0)))

	gpuDriver := driver.NewDriver(engine)
	connection := core.NewDirectConnection(engine)

	gpuBuilder := gpubuilder.NewGPUBuilder(engine)
	gpuBuilder.Driver = gpuDriver
	if DebugISA {
		gpuBuilder.EnableISADebug = true
	}
	if TraceMem {
		gpuBuilder.EnableMemTracing = true
	}
	gpu, globalMem := gpuBuilder.BuildEmulationGPU()

	connection.PlugIn(gpuDriver.ToGPUs)
	connection.PlugIn(gpu.ToDriver)
	gpu.Driver = gpuDriver.ToGPUs

	return engine, gpu, gpuDriver, globalMem
}

//BuildR9NanoPlatform creates a platform that equips with a R9Nano GPU
func BuildR9NanoPlatform() (
	core.Engine,
	*gcn3.GPU,
	*driver.Driver,
	*mem.IdealMemController,
) {
	var engine core.Engine

	if UseParallelEngine {
		engine = core.NewParallelEngine()
	} else {
		engine = core.NewSerialEngine()
	}
	//engine.AcceptHook(core.NewEventLogger(log.New(os.Stdout, "", 0)))

	gpuDriver := driver.NewDriver(engine)
	connection := core.NewDirectConnection(engine)

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

	connection.PlugIn(gpuDriver.ToGPUs)
	connection.PlugIn(gpu.ToDriver)
	gpu.Driver = gpuDriver.ToGPUs

	return engine, gpu, gpuDriver, globalMem
}
