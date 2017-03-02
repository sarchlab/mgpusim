package main

import (
	"bytes"
	"debug/elf"
	"log"
	"os"

	"encoding/binary"

	"gitlab.com/yaotsu/core/conn"
	"gitlab.com/yaotsu/core/event"
	"gitlab.com/yaotsu/gcn3/emulator"
	"gitlab.com/yaotsu/mem"
)

var (
	engine     event.Engine
	globalMem  *mem.IdealMemController
	gpu        *emulator.Gpu
	host       *conn.MockComponent
	connection conn.Connection
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

	// Memory
	globalMem = mem.NewIdealMemController("GlobalMem", engine, 4*mem.GB)
	globalMem.Frequency = 800 * event.MHz
	globalMem.Latency = 2

	// Host
	host = conn.NewMockComponent()
	host.AddPort("ToGpu")

	// Gpu
	gpu = emulator.NewGpu("Gpu")
	commandProcessor := emulator.NewCommandProcessor("Gpu.CommandProcessor")
	gpu.CommandProcessor = commandProcessor
	gpu.Driver = host

	// Connection
	connection = conn.NewDirectConnection()
	conn.PlugIn(gpu, "ToCommandProcessor", connection)
	conn.PlugIn(commandProcessor, "ToDriver", connection)
	conn.PlugIn(host, "ToGpu", connection)
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

	req := emulator.NewLaunchKernelReq()
	req.Packet = new(emulator.HsaKernelDispatchPacket)
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
