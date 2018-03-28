package main

import (
	"debug/elf"
	"fmt"
	"log"
	_ "net/http/pprof"

	"gitlab.com/yaotsu/core/connections"

	"flag"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/core/engines"
	"gitlab.com/yaotsu/gcn3"
	"gitlab.com/yaotsu/gcn3/driver"
	"gitlab.com/yaotsu/gcn3/gpubuilder"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/mem"
)


type ValuKernelArgs struct {
	vector1             driver.GPUPtr
	vector2             driver.GPUPtr
	output              driver.GPUPtr
	hiddenGlobalOffsetX int64
	hiddenGlobalOffsetY int64
	hiddenGlobalOffsetZ int64
}

var (
	engine     core.Engine
	globalMem  *mem.IdealMemController
	gpu        *gcn3.GPU
	connection core.Connection
	hsaco      *insts.HsaCo
	logger     *log.Logger
	gpuDriver  *driver.Driver

	dataSize    int
	gVector1	driver.GPUPtr
	gVector2    driver.GPUPtr
	gOutput     driver.GPUPtr
)


var cpuprofile = flag.String("cpuprofile", "prof.prof", "write cpu profile to file")
var kernel = flag.String("kernel", "kernels.hsaco", "the kernel hsaco file")


func main() {
	//flag.Parse()
	//
	//f, err := os.Create("trace.out")
	//if err != nil {
	//	panic(err)
	//}
	//defer f.Close()
	//
	//err = trace.Start(f)
	//if err != nil {
	//	panic(err)
	//}
	//defer trace.Stop()
	//
	//logger = log.New(os.Stdout, "", 0)

	initPlatform()
	loadProgram()
	initMem()
	run()
	checkResult()
}


func initPlatform() {
	engine = engines.NewSerialEngine()
	//engine = engines.NewParallelEngine()
	//engine.AcceptHook(util.NewEventLogger(log.New(os.Stdout, "", 0)))

	gpuDriver = driver.NewDriver(engine)
	connection = connections.NewDirectConnection(engine)

	gpuBuilder := gpubuilder.NewGPUBuilder(engine)
	gpuBuilder.Driver = gpuDriver
	gpuBuilder.EnableISADebug = true
	gpu, globalMem = gpuBuilder.BuildEmulationGPU()

	core.PlugIn(gpuDriver, "ToGPUs", connection)
	core.PlugIn(gpu, "ToDriver", connection)
	gpu.Driver = gpuDriver
}

func loadProgram() {
	executable, err := elf.Open(*kernel)
	if err != nil {
		log.Fatal(err)
	}

	sec := executable.Section(".text")
	hsacoData, err := sec.Data()
	if err != nil {
		log.Fatal(err)
	}

	hsaco = insts.NewHsaCoFromData(hsacoData)
	fmt.Println(hsaco.Info())
}

func initMem() {
	dataSize = 32000
	gVector1 = gpuDriver.AllocateMemory(globalMem.Storage, uint64(dataSize*4))
	gVector2 = gpuDriver.AllocateMemory(globalMem.Storage, uint64(dataSize*4))
	gOutput =  gpuDriver.AllocateMemory(globalMem.Storage, uint64(dataSize*4))

	v1 := make([]float32, dataSize)
	v2 := make([]float32, dataSize)
	for i := 0; i < dataSize; i++ {
		v1[i] = float32(i)
		v2[i] = float32(i)
	}


	gpuDriver.MemoryCopyHostToDevice(gVector1, v1, globalMem.Storage)
	gpuDriver.MemoryCopyHostToDevice(gVector2, v2, globalMem.Storage)
}


func run() {
	kernArg := ValuKernelArgs{
		gVector1,
		gVector2,
		gOutput,
		0, 0, 0,
	}

	gpuDriver.LaunchKernel(hsaco, gpu, globalMem.Storage,
		[3]uint32{uint32(dataSize), 1, 1},
		[3]uint16{256, 1, 1},
		&kernArg,
	)
}

func checkResult() {
	output := make([]float32, dataSize)
	gpuDriver.MemoryCopyDeviceToHost(output, gOutput, globalMem.Storage)

	for i, o := range output {
		fmt.Printf("%d: %f\n", i, o)
	}
}