// Package kmeans implements the Kmeans benchmark form Hetero-Mark.
package kmeans

import (
	// embed hsaco files
	_ "embed"
	"fmt"
	"math"
	"math/rand"
	"os"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

// GCN3 Kernel Arguments

// SwapArgs defines arguments for GCN3
type SwapArgs struct {
	Feature             gpuruntime.Ptr
	FeatureSwap         gpuruntime.Ptr
	NPoints             int32
	NFeatures           int32
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

// ComputeArgs defines arguments for GCN3
type ComputeArgs struct {
	Feature             gpuruntime.Ptr
	Clusters            gpuruntime.Ptr
	Membership          gpuruntime.Ptr
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

// CDNA3 Kernel Arguments

// CDNA3SwapArgs defines arguments for CDNA3 architecture (GFX942)
type CDNA3SwapArgs struct {
	Feature             gpuruntime.Ptr
	FeatureSwap         gpuruntime.Ptr
	NPoints             int32
	NFeatures           int32
	HiddenBlockCountX   uint32
	HiddenBlockCountY   uint32
	HiddenBlockCountZ   uint32
	HiddenGroupSizeX    uint16
	HiddenGroupSizeY    uint16
	HiddenGroupSizeZ    uint16
	HiddenRemainderX    uint16
	HiddenRemainderY    uint16
	HiddenRemainderZ    uint16
	Padding             [16]byte
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
	HiddenGridDims      uint16
}

// CDNA3ComputeArgs defines arguments for CDNA3 architecture (GFX942)
type CDNA3ComputeArgs struct {
	Feature             gpuruntime.Ptr
	Clusters            gpuruntime.Ptr
	Membership          gpuruntime.Ptr
	NPoints             int32
	NClusters           int32
	NFeatures           int32
	Offset              int32
	Size                int32
	Padding             int32
	HiddenBlockCountX   uint32
	HiddenBlockCountY   uint32
	HiddenBlockCountZ   uint32
	HiddenGroupSizeX    uint16
	HiddenGroupSizeY    uint16
	HiddenGroupSizeZ    uint16
	HiddenRemainderX    uint16
	HiddenRemainderY    uint16
	HiddenRemainderZ    uint16
	Padding2            [16]byte
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
	HiddenGridDims      uint16
}

// Benchmark defines a benchmark
type Benchmark struct {
	*State
	benchmarks.Checkpoint[checkpointConfig, State]
	rt *gpuruntime.Runtime

	computeKernel *insts.KernelCodeObject
	swapKernel    *insts.KernelCodeObject
}

// NewBenchmark makes a new benchmark
func NewBenchmark(rt *gpuruntime.Runtime) *Benchmark {
	b := new(Benchmark)
	b.State = &State{}
	b.Checkpoint = benchmarks.NewCheckpoint(b.State, b.checkpointConfig, b.bind)

	b.rt = rt
	b.Context = rt.Init()

	return b
}

//go:embed kernels.hsaco
var gcn3HSACOBytes []byte

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

func (b *Benchmark) loadKernels() {
	var hsacoBytes []byte
	if b.Arch == arch.CDNA3 {
		hsacoBytes = cdna3HSACOBytes
	} else {
		hsacoBytes = gcn3HSACOBytes
	}

	b.computeKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "kmeans_kernel_compute")
	b.swapKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "kmeans_kernel_swap")
}

// SelectGPU selects GPU
func (b *Benchmark) SelectGPU(gpuIDs []int) {
	b.Gpus = gpuIDs
	b.Queues = make([]gpuruntime.CommandQueue, len(b.Gpus))
	for i, gpu := range b.Gpus {
		b.rt.SelectGPU(b.Context, gpu)
		b.Queues[i] = b.rt.CreateCommandQueue(b.Context)
	}
}

// SetUnifiedMemory uses Unified Memory
func (b *Benchmark) SetUnifiedMemory() {
	b.UseUnifiedMemory = true
}

// Prepare the inputs for the first queued phase.
func (b *Benchmark) setup() {
	b.loadKernels()

	b.rt.SelectGPU(b.Context, b.Gpus[0])
	b.initMem()
}

func (b *Benchmark) initMem() {
	if b.UseUnifiedMemory {
		b.DFeatures = b.rt.AllocateUnifiedMemory(
			b.Context,
			uint64(b.NumPoints*b.NumFeatures*4))
		b.DFeaturesSwap = b.rt.AllocateUnifiedMemory(
			b.Context, uint64(b.NumPoints*b.NumFeatures*4))
		b.DMembership = b.rt.AllocateUnifiedMemory(
			b.Context, uint64(b.NumPoints*4))
	} else {
		b.DFeatures = b.rt.AllocateMemory(
			b.Context,
			uint64(b.NumPoints*b.NumFeatures*4))
		b.rt.Distribute(b.Context, b.DFeatures,
			uint64(b.NumPoints*b.NumFeatures*4), b.Gpus)

		b.DFeaturesSwap = b.rt.AllocateMemory(
			b.Context, uint64(b.NumPoints*b.NumFeatures*4))
		b.rt.Distribute(b.Context, b.DFeaturesSwap,
			uint64(b.NumPoints*b.NumFeatures*4), b.Gpus)

		b.DMembership = b.rt.AllocateMemory(
			b.Context, uint64(b.NumPoints*4))
		b.rt.Distribute(b.Context, b.DMembership,
			uint64(b.NumPoints*4), b.Gpus)
	}

	b.DClusters = make([]gpuruntime.Ptr, len(b.Gpus))
	for i, gpu := range b.Gpus {
		b.rt.SelectGPU(b.Context, gpu)
		if b.UseUnifiedMemory {
			b.DClusters[i] = b.rt.AllocateUnifiedMemory(
				b.Context, uint64(b.NumClusters*b.NumFeatures*4))
		} else {
			b.DClusters[i] = b.rt.AllocateMemory(
				b.Context, uint64(b.NumClusters*b.NumFeatures*4))
		}
	}

	// Use a local random source seeded with a fixed value so the generated
	// input is reproducible across runs. The top-level rand.Seed has been a
	// no-op since Go 1.24, so seeding the global generator no longer yields a
	// deterministic sequence; that made the feature data (and therefore the
	// clustering result) differ on every run.
	rng := rand.New(rand.NewSource(0))
	b.HFeatures = make([]float32, b.NumPoints*b.NumFeatures)
	for i := 0; i < b.NumPoints*b.NumFeatures; i++ {
		b.HFeatures[i] = rng.Float32()
		// b.hFeatures[i] = float32(i)
	}

	b.Upload(b.rt, b.Queues[0], b.DFeatures, b.HFeatures)
}

func (b *Benchmark) transposeFeatures() {
	for i, q := range b.Queues {
		numWI := b.NumPoints / len(b.Gpus)

		if b.Arch == arch.CDNA3 {
			globalSize := [3]uint32{uint32(numWI), 1, 1}
			localSize := [3]uint16{64, 1, 1}
			kernArg := CDNA3SwapArgs{
				Feature:             b.DFeatures,
				FeatureSwap:         b.DFeaturesSwap,
				NPoints:             int32(b.NumPoints),
				NFeatures:           int32(b.NumFeatures),
				HiddenBlockCountX:   globalSize[0] / uint32(localSize[0]),
				HiddenBlockCountY:   1,
				HiddenBlockCountZ:   1,
				HiddenGroupSizeX:    localSize[0],
				HiddenGroupSizeY:    localSize[1],
				HiddenGroupSizeZ:    localSize[2],
				HiddenRemainderX:    uint16(globalSize[0] % uint32(localSize[0])),
				HiddenRemainderY:    0,
				HiddenRemainderZ:    0,
				HiddenGlobalOffsetX: 0,
				HiddenGlobalOffsetY: 0,
				HiddenGlobalOffsetZ: 0,
				HiddenGridDims:      1,
			}

			b.rt.EnqueueLaunchKernel(
				q,
				b.swapKernel,
				globalSize,
				localSize,
				&kernArg,
			)
		} else {
			kernArg := SwapArgs{
				b.DFeatures,
				b.DFeaturesSwap,
				int32(b.NumPoints),
				int32(b.NumFeatures),
				int64(numWI * i), 0, 0,
			}

			b.rt.EnqueueLaunchKernel(
				q,
				b.swapKernel,
				[3]uint32{uint32(numWI), 1, 1},
				[3]uint16{64, 1, 1},
				&kernArg,
			)
		}
	}
}

func (b *Benchmark) verifySwap() error {
	gpuSwap := make([]float32, b.NumPoints*b.NumFeatures)
	if err := benchmarks.Read(b.rt, b.SwapReadback, gpuSwap); err != nil {
		return err
	}

	for i := 0; i < b.NumPoints; i++ {
		for j := 0; j < b.NumFeatures; j++ {
			if gpuSwap[j*b.NumPoints+i] != b.HFeatures[i*b.NumFeatures+j] {
				return fmt.Errorf("Swap error (%d, %d) expected %f, but get %f",
					i, j,
					b.HFeatures[i*b.NumFeatures+j],
					gpuSwap[j*b.NumPoints+i],
				)
			}
		}
	}
	return nil
}

func (b *Benchmark) initializeClusters() {
	b.HClusters = make([]float32, b.NumClusters*b.NumFeatures)
	for i := 0; i < b.NumClusters*b.NumFeatures; i++ {
		b.HClusters[i] = b.HFeatures[i]
	}
}

func (b *Benchmark) initializeMembership() {
	b.HMembership = make([]int32, b.NumPoints)
	for i := 0; i < b.NumPoints; i++ {
		b.HMembership[i] = -1
	}
}

func (b *Benchmark) enqueueComputeKernel(q gpuruntime.CommandQueue, gpuIndex int) {
	numWI := b.NumPoints / len(b.Gpus)

	if b.Arch == arch.CDNA3 {
		globalSize := [3]uint32{uint32(numWI), 1, 1}
		localSize := [3]uint16{64, 1, 1}
		kernArg := CDNA3ComputeArgs{
			Feature:             b.DFeaturesSwap,
			Clusters:            b.DClusters[gpuIndex],
			Membership:          b.DMembership,
			NPoints:             int32(b.NumPoints),
			NClusters:           int32(b.NumClusters),
			NFeatures:           int32(b.NumFeatures),
			Offset:              0,
			Size:                0,
			Padding:             0,
			HiddenBlockCountX:   globalSize[0] / uint32(localSize[0]),
			HiddenBlockCountY:   1,
			HiddenBlockCountZ:   1,
			HiddenGroupSizeX:    localSize[0],
			HiddenGroupSizeY:    localSize[1],
			HiddenGroupSizeZ:    localSize[2],
			HiddenRemainderX:    uint16(globalSize[0] % uint32(localSize[0])),
			HiddenRemainderY:    0,
			HiddenRemainderZ:    0,
			HiddenGlobalOffsetX: 0,
			HiddenGlobalOffsetY: 0,
			HiddenGlobalOffsetZ: 0,
			HiddenGridDims:      1,
		}

		b.rt.EnqueueLaunchKernel(
			q,
			b.computeKernel,
			globalSize,
			localSize,
			&kernArg,
		)
	} else {
		kernArg := ComputeArgs{
			b.DFeaturesSwap,
			b.DClusters[gpuIndex],
			b.DMembership,
			int32(b.NumPoints),
			int32(b.NumClusters),
			int32(b.NumFeatures),
			0, 0, 0,
			int64(numWI * gpuIndex), 0, 0,
		}

		b.rt.EnqueueLaunchKernel(
			q,
			b.computeKernel,
			[3]uint32{uint32(numWI), 1, 1},
			[3]uint16{64, 1, 1},
			&kernArg,
		)
	}
}

func (b *Benchmark) submitMembership() {
	for i, q := range b.Queues {
		b.Upload(b.rt, q, b.DClusters[i], b.HClusters)
		b.enqueueComputeKernel(q, i)
	}
}
func (b *Benchmark) consumeMembership() (float64, error) {
	newMembership := make([]int32, b.NumPoints)
	if err := benchmarks.Read(b.rt, b.Readback, newMembership); err != nil {
		return 0, err
	}

	delta := 0.0
	for i := 0; i < b.NumPoints; i++ {
		//fmt.Printf("%d - %d\n", i, newMembership[i])
		if newMembership[i] != b.HMembership[i] {
			delta++
			b.HMembership[i] = newMembership[i]
		}
	}

	return delta, nil
}

func (b *Benchmark) updateCentroids() {
	for i := 0; i < b.NumClusters*b.NumFeatures; i++ {
		b.HClusters[i] = 0
	}

	memberCount := make([]int, b.NumClusters)
	for i := 0; i < b.NumPoints; i++ {
		for j := 0; j < b.NumFeatures; j++ {
			featureIndex := i*b.NumFeatures + j
			clusterIndex := int(b.HMembership[i])*b.NumFeatures + j

			b.HClusters[clusterIndex] += b.HFeatures[featureIndex]
		}
		memberCount[b.HMembership[i]]++
	}

	for i := 0; i < b.NumClusters; i++ {
		for j := 0; j < b.NumFeatures; j++ {
			index := i*b.NumFeatures + j
			if memberCount[i] > 0 {
				b.HClusters[index] /= float32(memberCount[i])
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
			clusterIndex := int(b.HMembership[i])*b.NumFeatures + j
			distance := float64(b.HFeatures[featureIndex] - b.HClusters[clusterIndex])
			distanceSquare += distance * distance
		}
		mse += distanceSquare
	}

	mse /= float64(b.NumPoints)
	return mse
}

// Verify verifies
func (b *Benchmark) verify() error {
	gpuCentroids := make([]float32, b.NumClusters*b.NumFeatures)
	copy(gpuCentroids, b.HClusters)

	b.cpuKMeans()

	if err := b.compareCentroids(b.HClusters, gpuCentroids); err != nil {
		return err
	}

	cpuRMSE := b.calculateRMSE()
	if math.Abs(cpuRMSE-b.GpuRMSE) < 1e-12 {
		fmt.Fprintf(os.Stderr, "Passsed, RMSE %f\n", cpuRMSE)
	} else {
		return fmt.Errorf("error")
	}

	return nil
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

	fmt.Fprintf(os.Stderr, "CPU iterated %d times\n", numIterations)
}

func (b *Benchmark) updateMembershipCPU() float64 {
	newMembership := make([]int32, b.NumPoints)

	for i := 0; i < b.NumPoints; i++ {
		minDistance := float64(math.MaxFloat64)
		clusterIndex := 0

		for j := 0; j < b.NumClusters; j++ {
			dist := float64(0)

			for k := 0; k < b.NumFeatures; k++ {
				diff := float64(b.HFeatures[i*b.NumFeatures+k] - b.HClusters[j*b.NumFeatures+k])
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
		if newMembership[i] != b.HMembership[i] {
			delta++
			b.HMembership[i] = newMembership[i]
		}
	}

	return delta
}

func (b *Benchmark) compareCentroids(cpuCentroids, gpuCentroids []float32) error {
	for i := 0; i < b.NumClusters; i++ {
		for j := 0; j < b.NumFeatures; j++ {
			index := i*b.NumFeatures + j
			if cpuCentroids[index] != gpuCentroids[index] {
				return fmt.Errorf("centroid %d feature %d mismatch, CPU %f, GPU %f",
					i, j, cpuCentroids[index], gpuCentroids[index])
			}
		}
	}
	return nil
}

func (b *Benchmark) Step() error {
	if err := b.Prepare(); err != nil {
		return err
	}
	if err := b.ReleaseUploads(b.rt); err != nil {
		return err
	}

	if b.MaxIter <= 0 {
		return fmt.Errorf("kmeans requires MaxIter > 0")
	}
	switch b.Phase {
	case 0:
		b.setup()
		b.Phase = 1
	case 1:
		b.transposeFeatures()
		b.Phase = 2
	case 2:
		if b.Verification {
			b.SwapReadback = benchmarks.Download(b.rt, b.Queues[0], b.DFeaturesSwap, b.NumPoints*b.NumFeatures*4)
		}
		b.Phase = 3
	case 3:
		if b.Verification {
			if err := b.verifySwap(); err != nil {
				return err
			}
		}
		b.initializeClusters()
		b.initializeMembership()
		b.submitMembership()
		b.Phase = 4
	case 4:
		b.Readback = benchmarks.Download(b.rt, b.Queues[0], b.DMembership, b.NumPoints*4)
		b.Phase = 5
	case 5:
		return b.finishIteration()
	}
	return nil
}

func (b *Benchmark) finishIteration() error {
	delta, err := b.consumeMembership()
	if err != nil {
		return err
	}
	b.Iteration++
	b.updateCentroids()
	if delta > 0 && b.Iteration < b.MaxIter {
		b.submitMembership()
		b.Phase = 4
		return nil
	}
	fmt.Fprintf(os.Stderr, "GPU iterated %d times\n", b.Iteration)
	b.GpuRMSE = b.calculateRMSE()
	if b.Verification {
		if err := b.verify(); err != nil {
			return err
		}
	}
	b.Ended = true
	return nil
}
