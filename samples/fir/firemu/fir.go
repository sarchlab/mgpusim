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
	connection core.Connection
	hsaco      *insts.HsaCo
	logger     *log.Logger
	gpuDriver  *driver.Driver

	dataSize    uint64
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

	err = globalMem.Storage.Write(0, hsacoData)
	if err != nil {
		log.Fatal(err)
	}

	hsaco = insts.NewHsaCoFromData(hsacoData)
	fmt.Println(hsaco.Info())
}

func initMem() {
	var i uint64

	dataSize = 1024
	gFilterData = gpuDriver.AllocateMemory(globalMem.Storage, 16*4)
	gInputData = gpuDriver.AllocateMemory(globalMem.Storage, dataSize*4)
	gOutputData = gpuDriver.AllocateMemory(globalMem.Storage, dataSize*4)

	filterData := make([]float32, 16)
	for i = 0; i < 16; i++ {
		filterData[i] = float32(i)
	}

	inputData := make([]float32, dataSize)
	for i = 0; i < dataSize; i++ {
		inputData[i] = float32(i)
	}

	gpuDriver.MemoryCopyHostToDevice(gFilterData, filterData, globalMem.Storage)
	gpuDriver.MemoryCopyHostToDevice(gInputData, inputData, globalMem.Storage)
}

func run() {
	//kernelArgsBuffer := bytes.NewBuffer(make([]byte, 0))
	//binary.Write(kernelArgsBuffer, binary.LittleEndian, uint64(8192+4096)) // Output
	//binary.Write(kernelArgsBuffer, binary.LittleEndian, uint64(4096))      // Coeff
	//binary.Write(kernelArgsBuffer, binary.LittleEndian, uint64(8192))      // Input
	//binary.Write(kernelArgsBuffer, binary.LittleEndian, uint32(16))        // NumTap
	//err := globalMem.Storage.Write(65536, kernelArgsBuffer.Bytes())
	//if err != nil {
	//	log.Fatal(err)
	//}
	//
	//req := kernels.NewLaunchKernelReq()
	//req.HsaCo = hsaco
	//req.Packet = new(kernels.HsaKernelDispatchPacket)
	//req.Packet.GridSizeX = 256 * 4
	//req.Packet.GridSizeY = 1
	//req.Packet.GridSizeZ = 1
	//req.Packet.WorkgroupSizeX = 256
	//req.Packet.WorkgroupSizeY = 1
	//req.Packet.WorkgroupSizeZ = 1
	//req.Packet.KernelObject = 0
	//req.Packet.KernargAddress = 65536
	//
	//var buffer bytes.Buffer
	//binary.Write(&buffer, binary.LittleEndian, req.Packet)
	//err = globalMem.Storage.Write(0x11000, buffer.Bytes())
	//if err != nil {
	//	log.Fatal(err)
	//}
	//
	//req.PacketAddress = 0x11000
	//req.SetSrc(host)
	//req.SetDst(gpu)
	//req.SetSendTime(0)
	//connErr := connection.Send(req)
	//if connErr != nil {
	//	log.Fatal(connErr)
	//}
	//
	//engine.Run()
}

func checkResult() {
	//buf, err := globalMem.Storage.Read(12*mem.KB, 1024*4)
	//if err != nil {
	//	log.Fatal(nil)
	//}
	//
	//for i := 0; i < 1024; i++ {
	//	bits := binary.LittleEndian.Uint32(buf[i*4 : i*4+4])
	//	filtered := math.Float32frombits(bits)
	//
	//	fmt.Printf("%d: %f\n", i, filtered)
	//}

}
