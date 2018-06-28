package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/http"
	_ "net/http/pprof"

	"gitlab.com/yaotsu/core"
	"gitlab.com/yaotsu/gcn3"
	"gitlab.com/yaotsu/gcn3/driver"
	"gitlab.com/yaotsu/gcn3/insts"
	"gitlab.com/yaotsu/gcn3/kernels"
	"gitlab.com/yaotsu/gcn3/platform"
	"gitlab.com/yaotsu/mem"
)

type KMeansSwapArgs struct {
	Feature             driver.GPUPtr
	FeatureSwap         driver.GPUPtr
	NPoints             int32
	NFeatures           int32
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

type KMeansComputeArgs struct {
	Feature             driver.GPUPtr
	Clusters            driver.GPUPtr
	Membership          driver.GPUPtr
	NPoints             int32
	NClusters           int32
	NFeatures           int32
	Offset              int32
	Size                int32
	Padding             int32
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

var (
	engine        core.Engine
	globalMem     *mem.IdealMemController
	gpu           *gcn3.GPU
	gpuDriver     *driver.Driver
	computeKernel *insts.HsaCo
	swapKernel    *insts.HsaCo

	numClusters   int
	numPoints     int
	numFeatures   int
	hFeatures     []float32
	dFeatures     driver.GPUPtr
	dFeaturesSwap driver.GPUPtr
	hMembership   []int32
	dMembership   driver.GPUPtr
	hClusters     []float32
	dClusters     driver.GPUPtr

	gpuRMSE float64
)

var kernelFilePath = flag.String(
	"kernel file path",
	"kernels.hsaco",
	"The path to the kernel hsaco file.",
)
var timing = flag.Bool("timing", false, "Run detailed timing simulation.")
var parallel = flag.Bool("parallel", false, "Run the simulation in parallel.")
var verify = flag.Bool("verify", false, "Verify the emulation result.")
var isaDebug = flag.Bool("debug-isa", false, "Generate the ISA debugging file.")
var instTracing = flag.Bool("trace-inst", false, "Generate instruction trace for visualization purposes.")
var points = flag.Int("points", 4096, "The number of points.")
var clusters = flag.Int("clusters", 5, "The number of clusters.")
var features = flag.Int("features", 32, "The number of features for each point.")
var maxIter = flag.Int("max-iter", 20, "The maximum number of iterations to run")

func main() {
	configure()

	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

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

	numPoints = *points
	numFeatures = *features
	numClusters = *clusters
}

func initPlatform() {
	if *timing {
		engine, gpu, gpuDriver, globalMem = platform.BuildR9NanoPlatform()
	} else {
		engine, gpu, gpuDriver, globalMem = platform.BuildEmuPlatform()
	}
}

func loadProgram() {
	computeKernel = kernels.LoadProgram(*kernelFilePath, "kmeans_kernel_compute")
	swapKernel = kernels.LoadProgram(*kernelFilePath, "kmeans_kernel_swap")
}

func initMem() {
	dFeatures = gpuDriver.AllocateMemory(globalMem.Storage,
		uint64(numPoints*numFeatures*4))
	dFeaturesSwap = gpuDriver.AllocateMemory(globalMem.Storage,
		uint64(numPoints*numFeatures*4))
	dMembership = gpuDriver.AllocateMemory(globalMem.Storage,
		uint64(numPoints*4))
	dClusters = gpuDriver.AllocateMemory(globalMem.Storage,
		uint64(numClusters*numFeatures*4))

	rand.Seed(0)
	hFeatures = make([]float32, numPoints*numFeatures)
	for i := 0; i < numPoints*numFeatures; i++ {
		hFeatures[i] = rand.Float32()
		//hFeatures[i] = float32(i)
	}

	gpuDriver.MemoryCopyHostToDevice(dFeatures, hFeatures, gpu)
}

func run() {
	TransposeFeatures()
	KMeansClustering()
	gpuRMSE = CalculateRMSE()
}

func TransposeFeatures() {
	kernArg := KMeansSwapArgs{
		dFeatures,
		dFeaturesSwap,
		int32(numPoints),
		int32(numFeatures),
		0, 0, 0,
	}

	gpuDriver.LaunchKernel(swapKernel, gpu, globalMem.Storage,
		[3]uint32{uint32(numPoints), 1, 1},
		[3]uint16{64, 1, 1},
		&kernArg,
	)
}

func KMeansClustering() {
	numIterations := 0
	delta := float64(1.0)

	InitializeClusters()
	InitializeMembership()

	for delta > 0 && numIterations < *maxIter {
		delta = UpdateMembership()
		numIterations++
		UpdateCentroids()
	}

	fmt.Printf("GPU iterated %d times\n", numIterations)

}

func InitializeClusters() {
	hClusters = make([]float32, numClusters*numFeatures)
	for i := 0; i < numClusters*numFeatures; i++ {
		hClusters[i] = hFeatures[i]
	}
}

func InitializeMembership() {
	hMembership = make([]int32, numPoints)
	for i := 0; i < numPoints; i++ {
		hMembership[i] = -1
	}
}

func UpdateMembership() float64 {
	gpuDriver.MemoryCopyHostToDevice(dClusters, hClusters, gpu)

	kernArg := KMeansComputeArgs{
		dFeaturesSwap,
		dClusters,
		dMembership,
		int32(numPoints),
		int32(numClusters),
		int32(numFeatures),
		0, 0, 0,
		0, 0, 0,
	}

	gpuDriver.LaunchKernel(computeKernel, gpu, globalMem.Storage,
		[3]uint32{uint32(numPoints), 1, 1},
		[3]uint16{64, 1, 1},
		&kernArg,
	)

	newMembership := make([]int32, numPoints)
	gpuDriver.MemoryCopyDeviceToHost(newMembership, dMembership, gpu)

	delta := 0.0
	for i := 0; i < numPoints; i++ {
		//fmt.Printf("%d - %d\n", i, newMembership[i])
		if newMembership[i] != hMembership[i] {
			delta++
			hMembership[i] = newMembership[i]
		}
	}

	return delta
}

func UpdateCentroids() {
	for i := 0; i < numClusters*numFeatures; i++ {
		hClusters[i] = 0
	}

	memberCount := make([]int, numClusters)
	for i := 0; i < numPoints; i++ {
		for j := 0; j < numFeatures; j++ {
			featureIndex := i*numFeatures + j
			clusterIndex := int(hMembership[i])*numFeatures + j

			hClusters[clusterIndex] += hFeatures[featureIndex]
		}
		memberCount[hMembership[i]]++
	}

	for i := 0; i < numClusters; i++ {
		for j := 0; j < numFeatures; j++ {
			index := i*numFeatures + j
			if memberCount[i] > 0 {
				hClusters[index] /= float32(memberCount[i])
			}
		}
	}
}

func CalculateRMSE() float64 {
	mse := float64(0.0)

	for i := 0; i < numPoints; i++ {
		distanceSquare := float64(0.0)
		for j := 0; j < numFeatures; j++ {
			featureIndex := i*numFeatures + j
			clusterIndex := int(hMembership[i])*numFeatures + j
			distance := float64(hFeatures[featureIndex] - hClusters[clusterIndex])
			distanceSquare += distance * distance
		}
		mse += distanceSquare
	}

	mse /= float64(numPoints)
	return mse
}

func checkResult() {
	numIterations := 0
	delta := float64(1.0)

	InitializeClusters()
	InitializeMembership()

	for delta > 0 && numIterations < *maxIter {
		delta = UpdateMembershipCPU()
		numIterations++
		UpdateCentroids()
	}

	fmt.Printf("CPU iterated %d times\n", numIterations)

	cpuRMSE := CalculateRMSE()
	if math.Abs(cpuRMSE-gpuRMSE) < 1e-12 {
		fmt.Printf("Passsed, RMSE %f\n", cpuRMSE)
	} else {
		log.Fatal("error")
	}

}

func UpdateMembershipCPU() float64 {
	newMembership := make([]int32, numPoints)

	for i := 0; i < numPoints; i++ {
		minDistance := float64(math.MaxFloat64)
		clusterIndex := 0

		for j := 0; j < numClusters; j++ {
			dist := float64(0)

			for k := 0; k < numFeatures; k++ {
				diff := float64(hFeatures[i*numFeatures+k] - hClusters[j*numFeatures+k])
				dist += diff * diff
			}

			if dist < minDistance {
				minDistance = dist
				clusterIndex = j
			}

		}
		newMembership[i] = int32(clusterIndex)
	}

	delta := 0.0
	for i := 0; i < numPoints; i++ {
		//fmt.Printf("%d - %d\n", i, newMembership[i])
		if newMembership[i] != hMembership[i] {
			delta++
			hMembership[i] = newMembership[i]
		}
	}

	return delta
}
