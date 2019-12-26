package kmeans

import (
	"fmt"
	"log"
	"math"
	"math/rand"
	_ "net/http/pprof"

	"gitlab.com/akita/mgpusim/driver"
	"gitlab.com/akita/mgpusim/insts"
	"gitlab.com/akita/mgpusim/kernels"
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

type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	computeKernel *insts.HsaCo
	swapKernel    *insts.HsaCo

	NumClusters   int
	NumPoints     int
	NumFeatures   int
	MaxIter       int
	hFeatures     []float32
	dFeatures     driver.GPUPtr
	dFeaturesSwap driver.GPUPtr
	hMembership   []int32
	dMembership   driver.GPUPtr
	hClusters     []float32
	dClusters     []driver.GPUPtr

	gpuRMSE float64

	useUnifiedMemory bool
}

func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.context = driver.Init()

	b.loadKernels()

	return b
}

func (b *Benchmark) loadKernels() {
	hsacoBytes, err := Asset("kernels.hsaco")
	if err != nil {
		log.Panic(err)
	}
	b.computeKernel = kernels.LoadProgramFromMemory(
		hsacoBytes, "kmeans_kernel_compute")
	b.swapKernel = kernels.LoadProgramFromMemory(
		hsacoBytes, "kmeans_kernel_swap")
}

func (b *Benchmark) SelectGPU(gpuIDs []int) {
	b.gpus = gpuIDs
	b.queues = make([]*driver.CommandQueue, len(b.gpus))
	for i, gpu := range b.gpus {
		b.driver.SelectGPU(b.context, gpu)
		b.queues[i] = b.driver.CreateCommandQueue(b.context)
	}
}

// Use Unified Memory
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

func (b *Benchmark) Run() {
	b.driver.SelectGPU(b.context, b.gpus[0])
	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	if b.useUnifiedMemory {
		b.dFeatures = b.driver.AllocateUnifiedMemory(
			b.context,
			uint64(b.NumPoints*b.NumFeatures*4))
		b.dFeaturesSwap = b.driver.AllocateUnifiedMemory(
			b.context, uint64(b.NumPoints*b.NumFeatures*4))
		b.dMembership = b.driver.AllocateUnifiedMemory(
			b.context, uint64(b.NumPoints*4))
	} else {
		b.dFeatures = b.driver.AllocateMemory(
			b.context,
			uint64(b.NumPoints*b.NumFeatures*4))
		b.driver.Distribute(b.context, b.dFeatures,
			uint64(b.NumPoints*b.NumFeatures*4), b.gpus)

		b.dFeaturesSwap = b.driver.AllocateMemory(
			b.context, uint64(b.NumPoints*b.NumFeatures*4))
		b.driver.Distribute(b.context, b.dFeaturesSwap,
			uint64(b.NumPoints*b.NumFeatures*4), b.gpus)

		b.dMembership = b.driver.AllocateMemory(
			b.context, uint64(b.NumPoints*4))
		b.driver.Distribute(b.context, b.dMembership,
			uint64(b.NumPoints*4), b.gpus)
	}

	b.dClusters = make([]driver.GPUPtr, len(b.gpus))
	for i, gpu := range b.gpus {
		b.driver.SelectGPU(b.context, gpu)
		if b.useUnifiedMemory {
			b.dClusters[i] = b.driver.AllocateUnifiedMemory(
				b.context, uint64(b.NumClusters*b.NumFeatures*4))
		} else {
			b.dClusters[i] = b.driver.AllocateMemory(
				b.context, uint64(b.NumClusters*b.NumFeatures*4))
		}
	}

	rand.Seed(0)
	b.hFeatures = make([]float32, b.NumPoints*b.NumFeatures)
	for i := 0; i < b.NumPoints*b.NumFeatures; i++ {
		b.hFeatures[i] = rand.Float32()
		// b.hFeatures[i] = float32(i)
	}

	b.driver.MemCopyH2D(b.context, b.dFeatures, b.hFeatures)
}

func (b *Benchmark) exec() {
	b.transposeFeatures()
	b.kmeansClustering()
	b.gpuRMSE = b.calculateRMSE()
}

func (b *Benchmark) transposeFeatures() {
	for i, q := range b.queues {
		numWI := b.NumPoints / len(b.gpus)

		kernArg := KMeansSwapArgs{
			b.dFeatures,
			b.dFeaturesSwap,
			int32(b.NumPoints),
			int32(b.NumFeatures),
			int64(numWI * i), 0, 0,
		}

		b.driver.EnqueueLaunchKernel(
			q,
			b.swapKernel,
			[3]uint32{uint32(numWI), 1, 1},
			[3]uint16{64, 1, 1},
			&kernArg,
		)
	}

	for _, q := range b.queues {
		b.driver.DrainCommandQueue(q)
	}

	b.verifySwap()
}

func (b *Benchmark) verifySwap() {
	gpuSwap := make([]float32, b.NumPoints*b.NumFeatures)
	b.driver.MemCopyD2H(b.context, gpuSwap, b.dFeaturesSwap)

	for i := 0; i < b.NumPoints; i++ {
		for j := 0; j < b.NumFeatures; j++ {
			if gpuSwap[j*b.NumPoints+i] != b.hFeatures[i*b.NumFeatures+j] {
				log.Printf("Swap error (%d, %d) expected %f, but get %f",
					i, j,
					b.hFeatures[i*b.NumFeatures+j],
					gpuSwap[j*b.NumPoints+i],
				)
			}
		}
	}
}

func (b *Benchmark) kmeansClustering() {
	numIterations := 0
	delta := float64(1.0)

	b.initializeClusters()
	b.initializeMembership()

	for delta > 0 && numIterations < b.MaxIter {
		delta = b.updateMembership()
		numIterations++
		b.updateCentroids()
	}

	fmt.Printf("GPU iterated %d times\n", numIterations)
}

func (b *Benchmark) initializeClusters() {
	b.hClusters = make([]float32, b.NumClusters*b.NumFeatures)
	for i := 0; i < b.NumClusters*b.NumFeatures; i++ {
		b.hClusters[i] = b.hFeatures[i]
	}
}

func (b *Benchmark) initializeMembership() {
	b.hMembership = make([]int32, b.NumPoints)
	for i := 0; i < b.NumPoints; i++ {
		b.hMembership[i] = -1
	}
}

func (b *Benchmark) updateMembership() float64 {
	for i, q := range b.queues {
		b.driver.EnqueueMemCopyH2D(q, b.dClusters[i], b.hClusters)

		numWI := b.NumPoints / len(b.gpus)

		kernArg := KMeansComputeArgs{
			b.dFeaturesSwap,
			b.dClusters[i],
			b.dMembership,
			int32(b.NumPoints),
			int32(b.NumClusters),
			int32(b.NumFeatures),
			0, 0, 0,
			int64(numWI * i), 0, 0,
		}

		b.driver.EnqueueLaunchKernel(
			q,
			b.computeKernel,
			[3]uint32{uint32(numWI), 1, 1},
			[3]uint16{64, 1, 1},
			&kernArg,
		)
	}

	for _, q := range b.queues {
		b.driver.DrainCommandQueue(q)
	}

	newMembership := make([]int32, b.NumPoints)
	b.driver.MemCopyD2H(b.context, newMembership, b.dMembership)

	delta := 0.0
	for i := 0; i < b.NumPoints; i++ {
		//fmt.Printf("%d - %d\n", i, newMembership[i])
		if newMembership[i] != b.hMembership[i] {
			delta++
			b.hMembership[i] = newMembership[i]
		}
	}

	return delta
}

func (b *Benchmark) updateCentroids() {
	for i := 0; i < b.NumClusters*b.NumFeatures; i++ {
		b.hClusters[i] = 0
	}

	memberCount := make([]int, b.NumClusters)
	for i := 0; i < b.NumPoints; i++ {
		for j := 0; j < b.NumFeatures; j++ {
			featureIndex := i*b.NumFeatures + j
			clusterIndex := int(b.hMembership[i])*b.NumFeatures + j

			b.hClusters[clusterIndex] += b.hFeatures[featureIndex]
		}
		memberCount[b.hMembership[i]]++
	}

	for i := 0; i < b.NumClusters; i++ {
		for j := 0; j < b.NumFeatures; j++ {
			index := i*b.NumFeatures + j
			if memberCount[i] > 0 {
				b.hClusters[index] /= float32(memberCount[i])
			}
		}
	}
}

func (b *Benchmark) calculateRMSE() float64 {
	mse := float64(0.0)

	for i := 0; i < b.NumPoints; i++ {
		distanceSquare := float64(0.0)
		for j := 0; j < b.NumFeatures; j++ {
			featureIndex := i*b.NumFeatures + j
			clusterIndex := int(b.hMembership[i])*b.NumFeatures + j
			distance := float64(b.hFeatures[featureIndex] - b.hClusters[clusterIndex])
			distanceSquare += distance * distance
		}
		mse += distanceSquare
	}

	mse /= float64(b.NumPoints)
	return mse
}

func (b *Benchmark) Verify() {
	gpuCentroids := make([]float32, b.NumClusters*b.NumFeatures)
	copy(gpuCentroids, b.hClusters)

	b.cpuKMeans()

	b.compareCentroids(b.hClusters, gpuCentroids)

	cpuRMSE := b.calculateRMSE()
	if math.Abs(cpuRMSE-b.gpuRMSE) < 1e-12 {
		fmt.Printf("Passsed, RMSE %f\n", cpuRMSE)
	} else {
		log.Fatal("error")
	}
}

func (b *Benchmark) cpuKMeans() {
	numIterations := 0
	delta := float64(1.0)

	b.initializeClusters()
	b.initializeMembership()

	for delta > 0 && numIterations < b.MaxIter {
		delta = b.updateMembershipCPU()
		numIterations++
		b.updateCentroids()
	}

	fmt.Printf("CPU iterated %d times\n", numIterations)
}

func (b *Benchmark) updateMembershipCPU() float64 {
	newMembership := make([]int32, b.NumPoints)

	for i := 0; i < b.NumPoints; i++ {
		minDistance := float64(math.MaxFloat64)
		clusterIndex := 0

		for j := 0; j < b.NumClusters; j++ {
			dist := float64(0)

			for k := 0; k < b.NumFeatures; k++ {
				diff := float64(b.hFeatures[i*b.NumFeatures+k] - b.hClusters[j*b.NumFeatures+k])
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
	for i := 0; i < b.NumPoints; i++ {
		//fmt.Printf("%d - %d\n", i, newMembership[i])
		if newMembership[i] != b.hMembership[i] {
			delta++
			b.hMembership[i] = newMembership[i]
		}
	}

	return delta
}

func (b *Benchmark) compareCentroids(cpuCentroids, gpuCentroids []float32) {
	for i := 0; i < b.NumClusters; i++ {
		for j := 0; j < b.NumFeatures; j++ {
			index := i*b.NumFeatures + j
			if cpuCentroids[index] != gpuCentroids[index] {
				log.Panicf("centroid %d feature %d mismatch, CPU %f, GPU %f",
					i, j, cpuCentroids[index], gpuCentroids[index])
			}
		}
	}
}
