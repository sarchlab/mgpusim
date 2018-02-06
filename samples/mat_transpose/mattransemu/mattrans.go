package main

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"syscall"

	"flag"

	"runtime/debug"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/core/connections"
	"gitlab.com/yaotsu/core/engines"
	"gitlab.com/yaotsu/core/util"
	"gitlab.com/yaotsu/gcn3"
	"gitlab.com/yaotsu/gcn3/emu"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/gcn3/kernels"
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
	engine     core.Engine
	globalMem  *mem.IdealMemController
	gpu        *gcn3.GPU
	host       *hostComponent
	connection core.Connection
	hsaco      *insts.HsaCo
	logger     *log.Logger
)

var cpuprofile = flag.String("cpuprofile", "prof.prof", "write cpu profile to file")
var kernel = flag.String("kernel", "../disasm/kernel.hsaco", "the kernel hsaco file")

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

	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		debug.PrintStack()
		os.Exit(1)
	}()

	// log.SetOutput(ioutil.Discard)
	logger = log.New(os.Stdout, "", 0)

	initPlatform()
	loadProgram()
	initMem()
	run()
	checkResult()
}

func initPlatform() {
	// Simulation engine
	engine = engines.NewSerialEngine()
	// engine.AcceptHook(core.NewLogEventHook(log.New(os.Stdout, "", 0)))

	// Connection
	connection = connections.NewDirectConnection(engine)

	// Memory
	globalMem = mem.NewIdealMemController("GlobalMem", engine, 4*mem.GB)
	globalMem.Freq = 1 * util.GHz
	globalMem.Latency = 1

	// Host
	host = newHostComponent()

	// Gpu
	gpu = gcn3.NewGPU("GPU")
	commandProcessor := gcn3.NewCommandProcessor("GPU.CommandProcessor")

	dispatcher := gcn3.NewDispatcher("GPU.Dispatcher", engine,
		new(kernels.GridBuilderImpl))
	dispatcher.Freq = 1 * util.GHz
	wgCompleteLogger := new(gcn3.WGCompleteLogger)
	wgCompleteLogger.Logger = logger
	dispatcher.AcceptHook(wgCompleteLogger)

	gpu.CommandProcessor = commandProcessor
	gpu.Driver = host
	commandProcessor.Dispatcher = dispatcher
	commandProcessor.Driver = gpu
	disassembler := insts.NewDisassembler()
	isaDebug, err := os.Create("isa.debug")
	if err != nil {
		fmt.Print("Isa debug file failed to open\n")
	}
	for i := 0; i < 4; i++ {
		scratchpadPreparer := emu.NewScratchpadPreparerImpl()
		alu := emu.NewALU(globalMem.Storage)
		computeUnit := emu.NewComputeUnit(fmt.Sprintf("%s.cu%d", gpu.Name(), i),
			engine, disassembler, scratchpadPreparer, alu)
		computeUnit.Freq = 1 * util.GHz
		computeUnit.GlobalMemStorage = globalMem.Storage
		dispatcher.CUs = append(dispatcher.CUs, computeUnit)
		core.PlugIn(computeUnit, "ToDispatcher", connection)

		wfHook := emu.NewWfHook(log.New(isaDebug, "", 0))
		computeUnit.AcceptHook(wfHook)
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
	fmt.Println(hsaco.Info())
}

var (
	numSample     = 128
	numFeature    = 8
	inputDataSize = 1024 * 4
)

func initMem() {
	// Write the input
	inputData := make([]byte, 0)
	buffer := bytes.NewBuffer(inputData)
	for i := 0; i < numSample; i++ {
		for j := 0; j < numFeature; j++ {
			binary.Write(buffer, binary.LittleEndian, float32(i*numFeature+j))
		}
	}
	err := globalMem.Storage.Write(4*mem.KB, buffer.Bytes())
	if err != nil {
		log.Fatal(err)
	}

}

func run() {
	kernelArgsBuffer := bytes.NewBuffer(make([]byte, 0))
	binary.Write(kernelArgsBuffer, binary.LittleEndian, uint64(4096))      // feature
	binary.Write(kernelArgsBuffer, binary.LittleEndian, uint64(4096+4096)) // feature_swap
	binary.Write(kernelArgsBuffer, binary.LittleEndian, int32(numSample))  // npoints
	binary.Write(kernelArgsBuffer, binary.LittleEndian, int32(numFeature)) // nfeatures
	err := globalMem.Storage.Write(65536, kernelArgsBuffer.Bytes())
	if err != nil {
		log.Fatal(err)
	}

	req := kernels.NewLaunchKernelReq()
	req.HsaCo = hsaco
	req.Packet = new(kernels.HsaKernelDispatchPacket)
	req.Packet.GridSizeX = 2 * 64
	req.Packet.GridSizeY = 1
	req.Packet.GridSizeZ = 1
	req.Packet.WorkgroupSizeX = 64
	req.Packet.WorkgroupSizeY = 1
	req.Packet.WorkgroupSizeZ = 1
	req.Packet.KernelObject = 0
	req.Packet.KernargAddress = 65536

	var buffer bytes.Buffer
	binary.Write(&buffer, binary.LittleEndian, req.Packet)
	err = globalMem.Storage.Write(0x11000, buffer.Bytes())
	if err != nil {
		log.Fatal(err)
	}

	req.PacketAddress = 0x11000
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
	buf, err := globalMem.Storage.Read(8*mem.KB, 1024*4)
	if err != nil {
		log.Fatal(nil)
	}

	for i := 0; i < numSample*numFeature; i++ {
		bits := binary.LittleEndian.Uint32(buf[i*4 : i*4+4])
		outputs := math.Float32frombits(bits)

		fmt.Printf("%d: %f\n", i, outputs)
	}
}
