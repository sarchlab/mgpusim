package main

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"runtime/pprof"

	"flag"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/gcn3/kernels"
	"gitlab.com/yaotsu/gcn3/timing"
	"gitlab.com/yaotsu/gcn3/timing/cu"
	"gitlab.com/yaotsu/gcn3/trace"
	"gitlab.com/yaotsu/mem"
)

type hostComponent struct {
	*core.ComponentBase
}

func newHostComponent() *hostComponent {
	h := new(hostComponent)
	h.ComponentBase = core.NewComponentBase("host")
	h.AddPort("ToGpu")
	return h
}

func (h *hostComponent) Recv(req core.Req) *core.Error {
	switch req.(type) {
	case *kernels.LaunchKernelReq:
		log.Println("Kernel completed.")
	}
	return nil
}

func (h *hostComponent) Handle(evt core.Event) error {
	return nil
}

var (
	engine      core.Engine
	globalMem   *mem.IdealMemController
	gpu         *gcn3.Gpu
	host        *hostComponent
	connection  core.Connection
	hsaco       *insts.HsaCo
	logger      *log.Logger
	traceOutput *os.File
)

var cpuprofile = flag.String("cpuprofile", "prof.prof", "write cpu profile to file")
var kernel = flag.String("kernel", "../disasm/kernels.hsaco", "the kernel hsaco file")

func main() {
	flag.Parse()
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	runtime.SetBlockProfileRate(1)
	go func() {
		log.Println(http.ListenAndServe("localhost:8080", nil))
	}()

	// log.SetOutput(ioutil.Discard)
	logger = log.New(os.Stdout, "", 0)
	traceFile, err := os.Create("trace.out")
	if err != nil {
		log.Panic(err)
	}
	traceOutput = traceFile

	initPlatform()
	loadProgram()
	initMem()
	run()
	checkResult()
}

func initPlatform() {
	// Simulation engine
	engine = core.NewParallelEngine()

	// Connection
	connection = core.NewDirectConnection(engine)

	// Memory
	globalMem = mem.NewIdealMemController("GlobalMem", engine, 4*mem.GB)
	globalMem.Frequency = 1 * core.GHz
	globalMem.Latency = 2

	// Host
	host = newHostComponent()

	// Gpu
	gpu = gcn3.NewGpu("Gpu")
	commandProcessor := timing.NewCommandProcessor("Gpu.CommandProcessor")

	dispatcher := timing.NewDispatcher("Gpu.Dispatcher", engine,
		new(kernels.GridBuilderImpl))
	dispatcher.Freq = 1 * core.GHz
	wgCompleteLogger := new(timing.WGCompleteLogger)
	wgCompleteLogger.Logger = logger
	dispatcher.AcceptHook(wgCompleteLogger)

	gpu.CommandProcessor = commandProcessor
	gpu.Driver = host
	commandProcessor.Dispatcher = dispatcher
	commandProcessor.Driver = gpu
	cuBuilder := cu.NewBuilder()
	cuBuilder.Engine = engine
	cuBuilder.Freq = 1 * core.GHz
	cuBuilder.InstMem = globalMem
	cuBuilder.Decoder = insts.NewDisassembler()
	cuBuilder.ToInstMem = connection
	for i := 0; i < 4; i++ {
		cuBuilder.CUName = "cu" + string(i)
		computeUnit := cuBuilder.Build()
		dispatcher.CUs = append(dispatcher.CUs, computeUnit.Scheduler)
		core.PlugIn(computeUnit.Scheduler, "ToDispatcher", connection)

		// Hook
		mapWGLog := cu.NewMapWGLog(logger)
		computeUnit.Scheduler.AcceptHook(mapWGLog)
		dispatchWfHook := cu.NewDispatchWfLog(logger)
		computeUnit.Scheduler.AcceptHook(dispatchWfHook)

		if i == 0 {
			tracer := trace.NewInstTracer(traceOutput)
			computeUnit.Scheduler.AcceptHook(tracer)
			computeUnit.BranchUnit.AcceptHook(tracer)
			computeUnit.ScalarUnit.AcceptHook(tracer)
			computeUnit.SIMDUnits[0].AcceptHook(tracer)
			computeUnit.SIMDUnits[1].AcceptHook(tracer)
			computeUnit.SIMDUnits[2].AcceptHook(tracer)
			computeUnit.SIMDUnits[3].AcceptHook(tracer)
		}

	}

	// Connection
	core.PlugIn(gpu, "ToCommandProcessor", connection)
	core.PlugIn(gpu, "ToDriver", connection)
	core.PlugIn(commandProcessor, "ToDriver", connection)
	core.PlugIn(commandProcessor, "ToDispatcher", connection)
	core.PlugIn(host, "ToGpu", connection)
	core.PlugIn(dispatcher, "ToCommandProcessor", connection)
	core.PlugIn(dispatcher, "ToCUs", connection)
	core.PlugIn(globalMem, "Top", connection)
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

	err = globalMem.Storage.Write(0, hsacoData)
	if err != nil {
		log.Fatal(err)
	}

	hsaco = insts.NewHsaCoFromData(hsacoData)
}

func initMem() {
	// Write the filter
	filterData := make([]byte, 16*4)
	buffer := bytes.NewBuffer(filterData)
	for i := 0; i < 16; i++ {
		binary.Write(buffer, binary.LittleEndian, float32(i))
	}
	err := globalMem.Storage.Write(4*mem.KB, filterData)
	if err != nil {
		log.Fatal(err)
	}

	// Write the input
	inputData := make([]byte, 1024*4)
	buffer = bytes.NewBuffer(inputData)
	for i := 0; i < 1024; i++ {
		binary.Write(buffer, binary.LittleEndian, float32(i))
	}
	err = globalMem.Storage.Write(8*mem.KB, inputData)
	if err != nil {
		log.Fatal(err)
	}

}

func run() {
	kernelArgsBuffer := bytes.NewBuffer(make([]byte, 36))
	binary.Write(kernelArgsBuffer, binary.LittleEndian, uint64(8192))      // Input
	binary.Write(kernelArgsBuffer, binary.LittleEndian, uint64(8192+4096)) // Output
	binary.Write(kernelArgsBuffer, binary.LittleEndian, uint64(4096))      // Coeff
	binary.Write(kernelArgsBuffer, binary.LittleEndian, uint64(8192+8192)) // History
	binary.Write(kernelArgsBuffer, binary.LittleEndian, int(16))           // NumTap
	err := globalMem.Storage.Write(65536, kernelArgsBuffer.Bytes())
	if err != nil {
		log.Fatal(err)
	}

	req := kernels.NewLaunchKernelReq()
	req.HsaCo = hsaco
	req.Packet = new(kernels.HsaKernelDispatchPacket)
	req.Packet.GridSizeX = 256 * 4
	req.Packet.GridSizeY = 1
	req.Packet.GridSizeZ = 1
	req.Packet.WorkgroupSizeX = 256
	req.Packet.WorkgroupSizeY = 1
	req.Packet.WorkgroupSizeZ = 1
	req.Packet.KernelObject = 0
	req.Packet.KernargAddress = 65536

	req.SetSrc(host)
	req.SetDst(gpu)
	req.SetSendTime(0)
	connErr := connection.Send(req)
	if connErr != nil {
		log.Fatal(connErr)
	}

	engine.Run()
}

func checkResult() {

}
