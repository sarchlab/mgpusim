package main

import (
	"bytes"
	"debug/elf"
	"log"
	"os"

	"encoding/binary"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/gcn3/kernels"
	"gitlab.com/yaotsu/gcn3/timing"
	"gitlab.com/yaotsu/mem"
)

var (
	engine     core.Engine
	globalMem  *mem.IdealMemController
	gpu        *gcn3.Gpu
	host       *core.MockComponent
	connection core.Connection
	hsaco      *insts.HsaCo
)

func main() {
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
	connection = core.NewDirectConnection()

	// Memory
	globalMem = mem.NewIdealMemController("GlobalMem", engine, 4*mem.GB)
	globalMem.Frequency = 800 * core.MHz
	globalMem.Latency = 2

	// Host
	host = core.NewMockComponent("host")
	host.AddPort("ToGpu")

	// Gpu
	gpu = gcn3.NewGpu("Gpu")
	commandProcessor := timing.NewCommandProcessor("Gpu.CommandProcessor")

	dispatcher := timing.NewDispatcher("Gpu.Dispatcher", engine,
		new(kernels.GridBuilderImpl))
	gpu.CommandProcessor = commandProcessor
	gpu.Driver = host
	commandProcessor.Dispatcher = dispatcher
	disassembler := insts.NewDisassembler()
	for i := 0; i < 4; i++ {
	}

	// Connection
	core.PlugIn(gpu, "ToCommandProcessor", connection)
	core.PlugIn(commandProcessor, "ToDriver", connection)
	core.PlugIn(commandProcessor, "ToDispatcher", connection)
	core.PlugIn(host, "ToGpu", connection)
	core.PlugIn(dispatcher, "ToCommandProcessor", connection)
	core.PlugIn(globalMem, "Top", connection)
}

func loadProgram() {
	executable, err := elf.Open(os.Args[1])
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
	req.Packet.GridSizeX = 1024
	req.Packet.GridSizeY = 1
	req.Packet.GridSizeZ = 1
	req.Packet.WorkgroupSizeX = 256
	req.Packet.WorkgroupSizeY = 1
	req.Packet.WorkgroupSizeZ = 1
	req.Packet.KernelObject = 0
	req.Packet.KernargAddress = 65536

	req.SetSrc(host)
	req.SetDst(gpu)
	connErr := connection.Send(req)
	if connErr != nil {
		log.Fatal(connErr)
	}

	engine.Run()
}

func checkResult() {

}
