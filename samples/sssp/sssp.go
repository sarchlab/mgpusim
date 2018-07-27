package main

import (
	"flag"
	"fmt"
	"math/rand"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3"
	"gitlab.com/yaotsu/gcn3/driver"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/gcn3/kernels"
	"gitlab.com/yaotsu/gcn3/platform"
	"gitlab.com/yaotsu/mem"
)

type SSSPKernel1Args struct {
	VertexArray         driver.GPUPtr
	EdgeArray           driver.GPUPtr
	WeightArray         driver.GPUPtr
	MaskArray           driver.GPUPtr
	CostArray           driver.GPUPtr
	UpdatingCostArray   driver.GPUPtr
	VertexCount         uint32
	EdgeCount           uint32
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

type SSSPKernel2Args struct {
	VertexArray         driver.GPUPtr
	EdgeArray           driver.GPUPtr
	WeightArray         driver.GPUPtr
	MaskArray           driver.GPUPtr
	CostArray           driver.GPUPtr
	UpdatingCostArray   driver.GPUPtr
	VertexCount         uint32
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

type InitBuffersArgs struct {
	MaskArray           driver.GPUPtr
	CostArray           driver.GPUPtr
	UpdatingCostArray   driver.GPUPtr
	SourceVertex        uint32
	VertexCount         uint32
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

var (
	engine           core.Engine
	globalMem        *mem.IdealMemController
	gpu              *gcn3.GPU
	gpuDriver        *driver.Driver
	ssspKernel1Hsaco *insts.HsaCo
	ssspKernel2Hsaco *insts.HsaCo
	initBuffersHsaco *insts.HsaCo

	// Params
	vertexCount       uint32
	edgeCount         uint32
	weightCount       uint32
	maskCount         uint32
	sourceVertexCount uint32
	resultCount       uint32

	// Host side buffers
	vertexArray     []uint32
	edgeArray       []uint32
	weightArray     []float32
	maskArray       []uint32
	sourceVertArray []uint32
	resultArray     []float32

	// Device side buffers
	gVertexArray       driver.GPUPtr
	gEdgeArray         driver.GPUPtr
	gWeightArray       driver.GPUPtr
	gMaskArray         driver.GPUPtr
	gCostArray         driver.GPUPtr
	gUpdatingCostArray driver.GPUPtr
)

// Common flags
var kernelFilePath = flag.String("kernel file path", "kernels.hsaco", "The path to the kernel hsaco file.")
var timing = flag.Bool("timing", false, "Run detailed timing simulation.")
var parallel = flag.Bool("parallel", false, "Run the simulation in parallel.")
var isaDebug = flag.Bool("debug-isa", false, "Generate the ISA debugging file.")
var instTracing = flag.Bool("trace-inst", false, "Generate instruction trace for visualization purposes.")
var verify = flag.Bool("verify", false, "Verify the emulation result.")
var memTracing = flag.Bool("trace-mem", false, "Generate memory trace")

// Application specific flags
var numSourceVerts = flag.Uint("numSource", 100, "Number of source vertices to search from")
var numGenerateVerts = flag.Uint("numVerts", 100000, "Number of vertices in randomly generated graph")
var numGenerateEdgesPerVert = flag.Uint("numEdgesPerVert", 10, "Number of edges per vertex in randomly generated graph")

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
}

func initPlatform() {
	if *timing {
		engine, gpu, gpuDriver, globalMem = platform.BuildR9NanoPlatform()
	} else {
		engine, gpu, gpuDriver, globalMem = platform.BuildEmuPlatform()
	}
}

func loadProgram() {
	ssspKernel1Hsaco = kernels.LoadProgram(*kernelFilePath, "sssp_kernel1")
	ssspKernel2Hsaco = kernels.LoadProgram(*kernelFilePath, "sssp_kernel2")
	initBuffersHsaco = kernels.LoadProgram(*kernelFilePath, "init_buffers")
}

func initHostMem() {
	vertexCount = uint32(*numGenerateVerts)
	edgesPerVertex := uint32(*numGenerateEdgesPerVert)
	edgeCount = vertexCount * edgesPerVertex
	weightCount = edgeCount
	maskCount = vertexCount
	sourceVertexCount = uint32(*numSourceVerts)

	fmt.Printf("Vertex Count: %d\n", vertexCount)
	fmt.Printf("Edge Count: %d\n", edgeCount)

	vertexArray = make([]uint32, vertexCount)
	edgeArray = make([]uint32, edgeCount)
	weightArray = make([]float32, edgeCount)
	maskArray = make([]uint32, maskCount)
	sourceVertArray = make([]uint32, sourceVertexCount)
	resultArray = make([]float32, sourceVertexCount*vertexCount)

	var i uint32
	for i = 0; i < vertexCount; i++ {
		vertexArray[i] = i * edgesPerVertex
		fmt.Printf("%d ", vertexArray[i])
	}
	fmt.Println()

	for i = 0; i < edgeCount; i++ {
		edgeArray[i] = rand.Uint32() % vertexCount
		weightArray[i] = float32(i%1000) / 1000.0
		fmt.Printf("%d, %f\n", edgeArray[i], weightArray[i])
	}

	for i = 0; i < sourceVertexCount; i++ {
		sourceVertArray[i] = i % vertexCount
		fmt.Printf("%d ", sourceVertArray[i])
	}
	fmt.Println()
}

func initDeviceMem(globalWorkSize uint32) {
	// Create device buffers
	gVertexArray = gpuDriver.AllocateMemory(globalMem.Storage, uint64(globalWorkSize*4))
	gEdgeArray = gpuDriver.AllocateMemory(globalMem.Storage, uint64(edgeCount*4))
	gWeightArray = gpuDriver.AllocateMemory(globalMem.Storage, uint64(edgeCount*4))
	gMaskArray = gpuDriver.AllocateMemory(globalMem.Storage, uint64(globalWorkSize*4))
	gCostArray = gpuDriver.AllocateMemory(globalMem.Storage, uint64(globalWorkSize*4))
	gUpdatingCostArray = gpuDriver.AllocateMemory(globalMem.Storage, uint64(globalWorkSize*4))

	// Host to device
	gpuDriver.MemoryCopyHostToDevice(gVertexArray, vertexArray, gpu.ToDriver)
	gpuDriver.MemoryCopyHostToDevice(gEdgeArray, edgeArray, gpu.ToDriver)
	gpuDriver.MemoryCopyHostToDevice(gWeightArray, weightArray, gpu.ToDriver)
}

func isMaskArrayEmpty(maskArray []uint32, count uint32) bool {
	var i uint32
	for i = 0; i < count; i++ {
		if maskArray[i] == 1 {
			return false
		}
	}

	return true
}

func roundWorkSizeUp(groupSize uint32, globalSize uint32) uint32 {
	remainder := globalSize % groupSize
	if remainder == 0 {
		return globalSize
	} else {
		return globalSize + groupSize - remainder
	}
}

func run() {

	// TODO: hard-coded
	var maxWorkGroupSize uint32 = 256
	fmt.Printf("MAX_WORKGROUP_SIZE: %d\n", maxWorkGroupSize)
	fmt.Printf("Computing '%d' results.\n", sourceVertexCount)

	var globalWorkSize uint32
	var localWorkSize uint16

	localWorkSize = uint16(maxWorkGroupSize)
	globalWorkSize = roundWorkSizeUp(uint32(localWorkSize), vertexCount)
	fmt.Printf("%d, %d\n", localWorkSize, globalWorkSize)

	initDeviceMem(globalWorkSize)

	ssspKernel1Args := SSSPKernel1Args{gVertexArray, gEdgeArray, gWeightArray, gMaskArray, gCostArray, gUpdatingCostArray, vertexCount, edgeCount, 0, 0, 0}
	ssspKernel2Args := SSSPKernel2Args{gVertexArray, gEdgeArray, gWeightArray, gMaskArray, gCostArray, gUpdatingCostArray, vertexCount, 0, 0, 0}
	for i := 0; i < int(sourceVertexCount); i++ {
		// Launch init_buffers kernel
		initBuffersArgs := InitBuffersArgs{gMaskArray, gCostArray, gUpdatingCostArray, sourceVertArray[i], vertexCount, 0, 0, 0}
		gpuDriver.LaunchKernel(initBuffersHsaco, gpu.ToDriver, globalMem.Storage,
			[3]uint32{globalWorkSize, 1, 1},
			[3]uint16{localWorkSize, 1, 1},
			&initBuffersArgs)

		mask := make([]uint32, globalWorkSize)
		gpuDriver.MemoryCopyDeviceToHost(mask, gMaskArray, gpu.ToDriver)
		cost := make([]float32, globalWorkSize)
		gpuDriver.MemoryCopyDeviceToHost(cost, gCostArray, gpu.ToDriver)
		update := make([]float32, globalWorkSize)
		gpuDriver.MemoryCopyDeviceToHost(update, gUpdatingCostArray, gpu.ToDriver)

		var i uint32
		for i = 0; i < globalWorkSize; i++ {
			fmt.Printf("%d, %f, %f\n", mask[i], cost[i], update[i])
		}

		// Read mask from device to host
		gpuDriver.MemoryCopyDeviceToHost(maskArray, gMaskArray, gpu.ToDriver)
		for i = 0; i < vertexCount; i++ {
			fmt.Printf("%d ", maskArray[i])
		}
		fmt.Println()

		for !isMaskArrayEmpty(maskArray, vertexCount) {
			// Launch without reading the result to improve performance
			for asycIter := 0; asycIter < 10; asycIter++ {

				// Launch SSSP_Kernel1
				gpuDriver.LaunchKernel(ssspKernel1Hsaco, gpu.ToDriver, globalMem.Storage,
					[3]uint32{globalWorkSize, 1, 1},
					[3]uint16{localWorkSize, 1, 1},
					&ssspKernel1Args)

				// Launch SSSP_Kernel2
				gpuDriver.LaunchKernel(ssspKernel2Hsaco, gpu.ToDriver, globalMem.Storage,
					[3]uint32{globalWorkSize, 1, 1},
					[3]uint16{localWorkSize, 1, 1},
					&ssspKernel2Args)
			}

			// Read mask from device to host
			gpuDriver.MemoryCopyDeviceToHost(maskArray, gMaskArray, gpu.ToDriver)
			for i = 0; i < vertexCount; i++ {
				fmt.Printf("%d ", maskArray[i])
			}
			fmt.Println()
		}

		// Copy the result back
		// result := make([]uint32, vertexCount)
		result := resultArray[uint32(i)*vertexCount : uint32(i)*vertexCount+vertexCount]
		gpuDriver.MemoryCopyDeviceToHost(result, gMaskArray, gpu.ToDriver)
		// for i = 0; i < vertexCount; i++ {
		// 	fmt.Println(result[i])
		// }
	}

}

func checkResult() {

}

func main() {
	configure()
	initPlatform()
	loadProgram()
	initHostMem()
	run()

	if *verify {
		checkResult()
	}
}
