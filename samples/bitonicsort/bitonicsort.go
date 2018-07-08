package main

import (
	"flag"
	"log"
	"math/rand"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3"
	"gitlab.com/yaotsu/gcn3/driver"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/gcn3/kernels"
	"gitlab.com/yaotsu/gcn3/platform"
	"gitlab.com/yaotsu/mem"
)

type BitonicKernelArgs struct {
	Input               driver.GPUPtr
	Stage               uint32
	PassOfStage         uint32
	Direction           uint32
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

var (
	engine    core.Engine
	globalMem *mem.IdealMemController
	gpu       *gcn3.GPU
	gpuDriver *driver.Driver
	hsaco     *insts.HsaCo

	length     int
	inputData  []uint32
	gInputData driver.GPUPtr
)

var kernelFilePath = flag.String(
	"kernel file path",
	"kernels.hsaco",
	"The path to the kernel hsaco file.",
)
var timing = flag.Bool("timing", false, "Run detailed timing simulation.")
var parallel = flag.Bool("parallel", false, "Run the simulation in parallel.")
var isaDebug = flag.Bool("debug-isa", false, "Generate the ISA debugging file.")
var instTracing = flag.Bool("trace-inst", false, "Generate instruction trace for visualization purposes.")
var verify = flag.Bool("verify", false, "Verify the emulation result.")
var lenInput = flag.Int("length", 65536, "The length of array to sort.")
var orderAscending = flag.Bool("order-asc", true, "Sorting in ascending order.")
var memTracing = flag.Bool("trace-mem", false, "Generate memory trace")

func main() {
	configure()
	initPlatform()
	loadProgram()
	initMem()
	run()

	if *verify {
		checkResult()
	}
}

func configure() {
	flag.Parse()

	if *parallel {
		platform.UseParallelEngine = true
	}

	if *isaDebug {
		platform.DebugISA = true
	}

	if *instTracing {
		platform.TraceInst = true
	}

	if *memTracing {
		platform.TraceMem = true
	}

	length = *lenInput
}

func initPlatform() {
	if *timing {
		engine, gpu, gpuDriver, globalMem = platform.BuildR9NanoPlatform()
	} else {
		engine, gpu, gpuDriver, globalMem = platform.BuildEmuPlatform()
	}
}

func loadProgram() {
	hsaco = kernels.LoadProgram(*kernelFilePath, "BitonicSort")
}

func initMem() {
	gInputData = gpuDriver.AllocateMemory(globalMem.Storage, uint64(length*4))

	inputData = make([]uint32, length)
	for i := 0; i < length; i++ {
		inputData[i] = rand.Uint32()
	}

	gpuDriver.MemoryCopyHostToDevice(gInputData, inputData, gpu.ToDriver)
}

func run() {

	numStages := 0
	for temp := length; temp > 1; temp >>= 1 {
		numStages++
	}

	direction := 1
	if *orderAscending == false {
		direction = 0
	}

	for stage := 0; stage < numStages; stage += 1 {
		for passOfStage := 0; passOfStage < stage+1; passOfStage++ {
			kernArg := BitonicKernelArgs{
				gInputData,
				uint32(stage),
				uint32(passOfStage),
				uint32(direction),
				0, 0, 0}
			gpuDriver.LaunchKernel(hsaco, gpu.ToDriver, globalMem.Storage,
				[3]uint32{uint32(length / 2), 1, 1},
				[3]uint16{256, 1, 1},
				&kernArg)
		}

	}
}

func checkResult() {
	gpuOutput := make([]uint32, length)
	gpuDriver.MemoryCopyDeviceToHost(gpuOutput, gInputData, gpu.ToDriver)

	for i := 0; i < length-1; i++ {
		if *orderAscending {
			if gpuOutput[i] > gpuOutput[i+1] {
				log.Fatalf("Error: array[%d] > array[%d]: %d %d\n", i, i+1,
					gpuOutput[i], gpuOutput[i+1])
			}
		} else {
			if gpuOutput[i] < gpuOutput[i+1] {
				log.Fatalf("Error: array[%d] < array[%d]: %d %d\n", i, i+1,
					gpuOutput[i], gpuOutput[i+1])
			}
		}
	}

	log.Printf("Passed!\n")
}
