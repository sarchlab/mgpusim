package platform

import (
	"log"
	"os"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/core/connections"
	"gitlab.com/yaotsu/core/engines"
	"gitlab.com/yaotsu/core/util"
	"gitlab.com/yaotsu/gcn3"
	"gitlab.com/yaotsu/gcn3/driver"
	"gitlab.com/yaotsu/gcn3/gpubuilder"
	"gitlab.com/yaotsu/mem"
)

var UseParallelEngine bool
var DebugISA bool
var TraceInst bool

// BuildEmuPlatform creates a simple platform for emulation purposes
func BuildEmuPlatform() (
	core.Engine,
	*gcn3.GPU,
	*driver.Driver,
	*mem.IdealMemController,
) {
	var engine core.Engine

	if UseParallelEngine {
		engine = engines.NewParallelEngine()
	} else {
		engine = engines.NewSerialEngine()
	}
	//engine.AcceptHook(util.NewEventLogger(log.New(os.Stdout, "", 0)))

	gpuDriver := driver.NewDriver(engine)
	connection := connections.NewDirectConnection(engine)

	gpuBuilder := gpubuilder.NewGPUBuilder(engine)
	gpuBuilder.Driver = gpuDriver
	if DebugISA {
		gpuBuilder.EnableISADebug = true
	}
	gpu, globalMem := gpuBuilder.BuildEmulationGPU()

	core.PlugIn(gpuDriver, "ToGPUs", connection)
	core.PlugIn(gpu, "ToDriver", connection)
	gpu.Driver = gpuDriver

	return engine, gpu, gpuDriver, globalMem
}

// BuildR9NanoPlatform creates a platform that equips with a R9Nano GPU
func BuildR9NanoPlatform() (
	core.Engine,
	*gcn3.GPU,
	*driver.Driver,
	*mem.IdealMemController,
) {
	var engine core.Engine

	if UseParallelEngine {
		engine = engines.NewParallelEngine()
	} else {
		engine = engines.NewSerialEngine()
	}
	engine.AcceptHook(util.NewEventLogger(log.New(os.Stdout, "", 0)))

	gpuDriver := driver.NewDriver(engine)
	connection := connections.NewDirectConnection(engine)

	gpuBuilder := gpubuilder.NewGPUBuilder(engine)
	gpuBuilder.Driver = gpuDriver
	if DebugISA {
		gpuBuilder.EnableISADebug = true
	}
	if TraceInst {
		gpuBuilder.EnableInstTracing = true
	}

	gpu, globalMem := gpuBuilder.BuildR9Nano()

	core.PlugIn(gpuDriver, "ToGPUs", connection)
	core.PlugIn(gpu, "ToDriver", connection)
	gpu.Driver = gpuDriver

	return engine, gpu, gpuDriver, globalMem
}
