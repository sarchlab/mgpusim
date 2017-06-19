package main

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"fmt"
	"log"
	_ "net/http/pprof"
	"os"

	"flag"

	"gitlab.com/yaotsu/core"
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
	gpu        *gcn3.Gpu
	host       *hostComponent
	connection core.Connection
	hsaco      *insts.HsaCo
	logger     *log.Logger
)

var kernel = flag.String("kernel", "../vector_copy.hsaco",
	"the kernel hsaco file")

func main() {
	flag.Parse()

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
	engine = core.NewSerialEngine()

	// Connection
	connection = core.NewDirectConnection(engine)

	// Memory
	globalMem = mem.NewIdealMemController("GlobalMem", engine, 4*mem.GB)
	globalMem.Freq = 1 * core.GHz
	globalMem.Latency = 1

	// Host
	host = newHostComponent()

	// Gpu
	gpu = gcn3.NewGpu("GPU")
	commandProcessor := gcn3.NewCommandProcessor("GPU.CommandProcessor")

	dispatcher := gcn3.NewDispatcher("GPU.Dispatcher", engine,
		new(kernels.GridBuilderImpl))
	dispatcher.Freq = 1 * core.GHz
	wgCompleteLogger := new(gcn3.WGCompleteLogger)
	wgCompleteLogger.Logger = logger
	dispatcher.AcceptHook(wgCompleteLogger)

	gpu.CommandProcessor = commandProcessor
	gpu.Driver = host
	commandProcessor.Dispatcher = dispatcher
	commandProcessor.Driver = gpu
	disassembler := insts.NewDisassembler()
	for i := 0; i < 4; i++ {
		regInterface := new(emu.RegInterfaceImpl)
		scratchpadPreparer := emu.NewScratchpadPreparerImpl(regInterface)
		alu := new(emu.ALU)
		computeUnit := emu.NewComputeUnit(fmt.Sprintf("%s.cu%d", gpu.Name(), i),
			engine, disassembler, scratchpadPreparer, alu)
		computeUnit.Freq = 1 * core.GHz
		computeUnit.GlobalMemStorage = globalMem.Storage
		dispatcher.CUs = append(dispatcher.CUs, computeUnit)
		core.PlugIn(computeUnit, "ToDispatcher", connection)
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
	log.Println(hsaco.Info())
}

func initMem() {
	// Write the input
	inputData := make([]byte, 1024*4)
	buffer := bytes.NewBuffer(inputData)
	for i := 0; i < 1024; i++ {
		binary.Write(buffer, binary.LittleEndian, int32(i))
	}

	err := globalMem.Storage.Write(8*mem.KB, inputData)
	if err != nil {
		log.Fatal(err)
	}
}

func run() {
	kernelArgsBuffer := bytes.NewBuffer(make([]byte, 36))
	binary.Write(kernelArgsBuffer, binary.LittleEndian, uint64(8192))      // Input
	binary.Write(kernelArgsBuffer, binary.LittleEndian, uint64(8192+4096)) // Output
	err := globalMem.Storage.Write(0x10000, kernelArgsBuffer.Bytes())
	if err != nil {
		log.Fatal(err)
	}

	packet := new(kernels.HsaKernelDispatchPacket)
	packet.GridSizeX = 256 * 4
	packet.GridSizeY = 1
	packet.GridSizeZ = 1
	packet.WorkgroupSizeX = 256
	packet.WorkgroupSizeY = 1
	packet.WorkgroupSizeZ = 1
	packet.KernelObject = 0
	packet.KernargAddress = 0x10000
	var buffer bytes.Buffer
	binary.Write(&buffer, binary.LittleEndian, packet)
	err = globalMem.Storage.Write(0x11000, buffer.Bytes())
	if err != nil {
		log.Fatal(err)
	}

	req := kernels.NewLaunchKernelReq()
	req.HsaCo = hsaco
	req.Packet = packet
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
	buffer, err := globalMem.Storage.Read(8192, 1024*4)
	if err != nil {
		log.Fatal(nil)
	}

	for i := 0; i < 1024; i++ {
		copied := binary.LittleEndian.Uint32(buffer[i*4 : i*4+4])
		fmt.Printf("%d: %d\n", i, copied)
	}
}
