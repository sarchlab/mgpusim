package main

import (
	"debug/elf"
	"fmt"
	"log"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"gitlab.com/yaotsu/core/connections"
	"gitlab.com/yaotsu/core/util"

	"flag"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/core/engines"
	"gitlab.com/yaotsu/gcn3"
	"gitlab.com/yaotsu/gcn3/driver"
	"gitlab.com/yaotsu/gcn3/gpubuilder"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/mem"
)

type FirKernelArgs struct {
	output    driver.GPUPtr
	filter    driver.GPUPtr
	tempInput driver.GPUPtr
	numTaps   uint32
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
	numTaps     int
	gFilterData driver.GPUPtr
	gInputData  driver.GPUPtr
	gOutputData driver.GPUPtr
)

var cpuprofile = flag.String("cpuprofile", "prof.prof", "write cpu profile to file")
var kernel = flag.String("kernel", "../disasm/kernels.hsaco", "the kernel hsaco file")

func main() {
	flag.Parse()
	// if *cpuprofile != "" {
	// 	f, err := os.Create(*cpuprofile)
	// 	if err != nil {
	// 		log.Fatal(err)
	// 	}
	// 	pprof.StartCPUProfile(f)
	// 	defer pprof.StopCPUProfile()
	// }

	// runtime.SetBlockProfileRate(1)
	// go func() {
	// 	log.Println(http.ListenAndServe("localhost:8080", nil))
	// }()

	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		debug.PrintStack()
		os.Exit(1)
	}()

	// f, err := os.Create("trace.out")
	// if err != nil {
	// 	panic(err)
	// }
	// defer f.Close()

	// err = trace.Start(f)
	// if err != nil {
	// 	panic(err)
	// }
	// defer trace.Stop()

	// log.SetOutput(ioutil.Discard)
	logger = log.New(os.Stdout, "", 0)

	initPlatform()
	loadProgram()
	initMem()
	run()
	checkResult()
}

func initPlatform() {
	engine = engines.NewSerialEngine()
	engine.AcceptHook(util.NewEventLogger(log.New(os.Stdout, "", 0)))

	//host = newHostComponent()
	gpuDriver = driver.NewDriver(engine)
	connection = connections.NewDirectConnection(engine)

	gpuBuilder := gpubuilder.NewGPUBuilder(engine)
	gpuBuilder.Driver = gpuDriver
	gpuBuilder.EnableISADebug = true
	gpu, globalMem = gpuBuilder.BuildEmulationGPU()

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
	dataSize = 1024
	numTaps = 16
	gFilterData = gpuDriver.AllocateMemory(globalMem.Storage, uint64(numTaps*4))
	gInputData = gpuDriver.AllocateMemory(globalMem.Storage, uint64(dataSize*4))
	gOutputData = gpuDriver.AllocateMemory(globalMem.Storage, uint64(dataSize*4))

	filterData := make([]float32, numTaps)
	for i := 0; i < numTaps; i++ {
		filterData[i] = float32(i)
	}

	inputData := make([]float32, dataSize+numTaps)
	for i := 0; i < dataSize+numTaps; i++ {
		inputData[i] = float32(i)
	}

	gpuDriver.MemoryCopyHostToDevice(gFilterData, filterData, globalMem.Storage)
	gpuDriver.MemoryCopyHostToDevice(gInputData, inputData, globalMem.Storage)
}

func run() {
	kernArg := FirKernelArgs{
		gOutputData,
		gFilterData,
		gInputData,
		uint32(numTaps),
	}

	gpuDriver.LaunchKernel(hsaco, gpu, globalMem.Storage,
		[3]uint32{uint32(dataSize), 1, 1},
		[3]uint16{256, 1, 1},
		&kernArg,
	)
}

func checkResult() {

	output := make([]float32, dataSize)
	gpuDriver.MemoryCopyDeviceToHost(output, gInputData, globalMem.Storage)

	for i, o := range output {
		fmt.Printf("%d: %f\n", i, o)
	}
}
