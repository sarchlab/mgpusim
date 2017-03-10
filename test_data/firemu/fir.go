package main

import (
	"bytes"
	"debug/elf"
	"log"
	"os"

	"encoding/binary"

	"gitlab.com/yaotsu/core/conn"
	"gitlab.com/yaotsu/core/event"
	"gitlab.com/yaotsu/gcn3"
	"gitlab.com/yaotsu/gcn3/disasm"
	"gitlab.com/yaotsu/gcn3/emu"
	"gitlab.com/yaotsu/mem"
)

var (
	engine     event.Engine
	globalMem  *mem.IdealMemController
	gpu        *gcn3.Gpu
	host       *conn.MockComponent
	connection conn.Connection
	hsaco      *disasm.HsaCo
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
	engine = event.NewSerialEngine()

	// Connection
	connection = conn.NewDirectConnection()

	// Memory
	globalMem = mem.NewIdealMemController("GlobalMem", engine, 4*mem.GB)
	globalMem.Frequency = 800 * event.MHz
	globalMem.Latency = 2

	// Host
	host = conn.NewMockComponent("host")
	host.AddPort("ToGpu")

	// Gpu
	gpu = gcn3.NewGpu("Gpu")
	commandProcessor := emu.NewCommandProcessor("Gpu.CommandProcessor")
	dispatcher := emu.NewDispatcher("Gpu.Dispatcher",
		emu.NewMapWGReqFactory())
	gpu.CommandProcessor = commandProcessor
	gpu.Driver = host
	commandProcessor.Dispatcher = dispatcher
	for i := 0; i < 4; i++ {
		scalarInstWorker := emu.NewScalarInstWorker()
		cu := emu.NewComputeUnit(
			"Gpu.CU"+string(i),
			engine,
			disasm.NewDisassembler(),
			globalMem,
			scalarInstWorker,
		)
		scalarInstWorker.CU = cu
		dispatcher.RegisterCU(cu)
		conn.PlugIn(cu, "ToDispatcher", connection)
		conn.PlugIn(dispatcher, "ToComputeUnits", connection)
		conn.PlugIn(cu, "ToInstMem", connection)
	}

	// Connection
	conn.PlugIn(gpu, "ToCommandProcessor", connection)
	conn.PlugIn(commandProcessor, "ToDriver", connection)
	conn.PlugIn(commandProcessor, "ToDispatcher", connection)
	conn.PlugIn(host, "ToGpu", connection)
	conn.PlugIn(dispatcher, "ToCommandProcessor", connection)
	conn.PlugIn(globalMem, "Top", connection)
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

	hsaco = disasm.NewHsaCoFromData(hsacoData)
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

	req := emu.NewLaunchKernelReq()
	req.HsaCo = hsaco
	req.Packet = new(emu.HsaKernelDispatchPacket)
	req.Packet.GridSizeX = 1024
	req.Packet.GridSizeY = 1
	req.Packet.GridSizeZ = 1
	req.Packet.WorkgroupSizeX = 256
	req.Packet.WorkgroupSizeY = 1
	req.Packet.WorkgroupSizeZ = 1
	req.Packet.KernelObject = 0
	req.Packet.KernargAddress = 65536

	req.SetSource(host)
	req.SetDestination(gpu)
	connErr := connection.Send(req)
	if connErr != nil {
		log.Fatal(connErr)
	}

	engine.Run()
}

func checkResult() {

}
