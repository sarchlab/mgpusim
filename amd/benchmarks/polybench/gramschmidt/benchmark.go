// Package gramschmidt implements the Gram-Schmidt benchmark from Polybench.
// QR factorization of an M×N matrix using modified Gram-Schmidt.
package gramschmidt

import (
	"log"
	"math"
	"math/rand"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// NormKernelArgs defines gs_norm_kernel arguments
type NormKernelArgs struct {
	Q driver.Ptr
	R driver.Ptr
	M int32
	N int32
	K int32
}

// SqrtKernelArgs defines gs_sqrt_kernel arguments
// SqrtKernelArgs defines gs_sqrt_kernel arguments
// kernarg_size=16: 1 pointer (8) + 2 ints (8) = 16, no implicit args
type SqrtKernelArgs struct {
	R driver.Ptr
	N int32
	K int32
}

// NormalizeKernelArgs defines gs_normalize_kernel arguments
type NormalizeKernelArgs struct {
	Q driver.Ptr
	R driver.Ptr
	M int32
	N int32
	K int32
}

// DotKernelArgs defines gs_dot_kernel arguments
type DotKernelArgs struct {
	Q driver.Ptr
	R driver.Ptr
	M int32
	N int32
	K int32
	J int32
}

// UpdateKernelArgs defines gs_update_kernel arguments
type UpdateKernelArgs struct {
	Q driver.Ptr
	R driver.Ptr
	M int32
	N int32
	K int32
	J int32
}

// Benchmark defines a benchmark
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue
	kernel1 *insts.KernelCodeObject
	kernel2 *insts.KernelCodeObject
	kernel3 *insts.KernelCodeObject
	kernel4 *insts.KernelCodeObject
	kernel5 *insts.KernelCodeObject

	M, N             int
	q, r             []float32
	qOutput, rOutput []float32
	dQ, dR           driver.Ptr
	cpuQ, cpuR       []float32

	useUnifiedMemory bool
}

// NewBenchmark makes a new benchmark
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	return b
}

// SelectGPU selects GPU
func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
}

// SetUnifiedMemory uses Unified Memory
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

func (b *Benchmark) loadProgram() {
	b.kernel1 = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "gs_norm_kernel")
	if b.kernel1 == nil {
		log.Panic("Failed to load kernel binary")
	}

	b.kernel2 = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "gs_sqrt_kernel")
	if b.kernel2 == nil {
		log.Panic("Failed to load kernel binary")
	}

	b.kernel3 = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "gs_normalize_kernel")
	if b.kernel3 == nil {
		log.Panic("Failed to load kernel binary")
	}

	b.kernel4 = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "gs_dot_kernel")
	if b.kernel4 == nil {
		log.Panic("Failed to load kernel binary")
	}

	b.kernel5 = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "gs_update_kernel")
	if b.kernel5 == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// Run runs
func (b *Benchmark) Run() {
	b.loadProgram()

	for _, gpu := range b.gpus {
		b.driver.SelectGPU(b.context, gpu)
		b.queues = append(b.queues, b.driver.CreateCommandQueue(b.context))
	}

	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	rand.Seed(1)
	b.q = make([]float32, b.M*b.N)
	b.r = make([]float32, b.N*b.N)
	b.qOutput = make([]float32, b.M*b.N)
	b.rOutput = make([]float32, b.N*b.N)

	for i := 0; i < b.M*b.N; i++ {
		b.q[i] = float32(rand.Intn(100)) / 10.0
	}

	if b.useUnifiedMemory {
		b.dQ = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.M*b.N*4))
		b.dR = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.N*b.N*4))
	} else {
		b.dQ = b.driver.AllocateMemory(b.context,
			uint64(b.M*b.N*4))
		b.dR = b.driver.AllocateMemory(b.context,
			uint64(b.N*b.N*4))
	}
}

func (b *Benchmark) exec() {
	b.driver.MemCopyH2D(b.context, b.dQ, b.q)
	b.driver.MemCopyH2D(b.context, b.dR, b.r)

	localSize := [3]uint16{256, 1, 1}
	globalSizeX := uint32(((b.M-1)/256 + 1) * 256)
	globalSize := [3]uint32{globalSizeX, 1, 1}

	sqrtLocalSize := [3]uint16{1, 1, 1}
	sqrtGlobalSize := [3]uint32{1, 1, 1}

	for k := 0; k < b.N; k++ {
		zero := float32(0)
		b.driver.MemCopyH2D(b.context, b.rElemPtr(k, k), zero)

		b.launchNormKernel(localSize, globalSize, globalSizeX, k)
		b.launchSqrtKernel(sqrtLocalSize, sqrtGlobalSize, 1, k)
		b.launchNormalizeKernel(localSize, globalSize, globalSizeX, k)

		for j := k + 1; j < b.N; j++ {
			b.driver.MemCopyH2D(b.context, b.rElemPtr(k, j), zero)
			b.launchDotKernel(localSize, globalSize, globalSizeX, k, j)
			b.launchUpdateKernel(localSize, globalSize, globalSizeX, k, j)
		}
	}

	b.driver.MemCopyD2H(b.context, b.qOutput, b.dQ)
	b.driver.MemCopyD2H(b.context, b.rOutput, b.dR)
}

func (b *Benchmark) rElemPtr(i, j int) driver.Ptr {
	return b.dR + driver.Ptr((i*b.N+j)*4)
}

func (b *Benchmark) launchNormKernel(localSize [3]uint16, globalSize [3]uint32, globalSizeX uint32, k int) {
	kernelArg := NormKernelArgs{
		Q: b.dQ,
		R: b.dR,
		M: int32(b.M),
		N: int32(b.N),
		K: int32(k),
	}
	b.driver.LaunchKernel(b.context, b.kernel1,
		globalSize, localSize, &kernelArg)
}

func (b *Benchmark) launchSqrtKernel(localSize [3]uint16, globalSize [3]uint32, globalSizeX uint32, k int) {
	kernelArg := SqrtKernelArgs{
		R: b.dR,
		N: int32(b.N),
		K: int32(k),
	}
	b.driver.LaunchKernel(b.context, b.kernel2,
		globalSize, localSize, &kernelArg)
}

func (b *Benchmark) launchNormalizeKernel(localSize [3]uint16, globalSize [3]uint32, globalSizeX uint32, k int) {
	kernelArg := NormalizeKernelArgs{
		Q: b.dQ,
		R: b.dR,
		M: int32(b.M),
		N: int32(b.N),
		K: int32(k),
	}
	b.driver.LaunchKernel(b.context, b.kernel3,
		globalSize, localSize, &kernelArg)
}

func (b *Benchmark) launchDotKernel(localSize [3]uint16, globalSize [3]uint32, globalSizeX uint32, k, j int) {
	kernelArg := DotKernelArgs{
		Q: b.dQ,
		R: b.dR,
		M: int32(b.M),
		N: int32(b.N),
		K: int32(k),
		J: int32(j),
	}
	b.driver.LaunchKernel(b.context, b.kernel4,
		globalSize, localSize, &kernelArg)
}

func (b *Benchmark) launchUpdateKernel(localSize [3]uint16, globalSize [3]uint32, globalSizeX uint32, k, j int) {
	kernelArg := UpdateKernelArgs{
		Q: b.dQ,
		R: b.dR,
		M: int32(b.M),
		N: int32(b.N),
		K: int32(k),
		J: int32(j),
	}
	b.driver.LaunchKernel(b.context, b.kernel5,
		globalSize, localSize, &kernelArg)
}

// Verify verifies
func (b *Benchmark) Verify() {
	b.cpuGramSchmidt()

	const relTol = 1e-2
	mismatchCount := 0

	for i := 0; i < b.M*b.N; i++ {
		if !float32Close(b.cpuQ[i], b.qOutput[i], relTol) {
			if mismatchCount < 10 {
				log.Printf("Mismatch in Q at %d, expected %v, but get %v",
					i, b.cpuQ[i], b.qOutput[i])
			}
			mismatchCount++
		}
	}

	for i := 0; i < b.N*b.N; i++ {
		if !float32Close(b.cpuR[i], b.rOutput[i], relTol) {
			if mismatchCount < 10 {
				log.Printf("Mismatch in R at %d, expected %v, but get %v",
					i, b.cpuR[i], b.rOutput[i])
			}
			mismatchCount++
		}
	}

	if mismatchCount > 0 {
		log.Panicf("%d mismatches found", mismatchCount)
	}

	log.Printf("Passed!\n")
}

func float32Close(a, b float32, relTol float64) bool {
	if a == b {
		return true
	}
	diff := math.Abs(float64(a) - float64(b))
	// Absolute tolerance for values near zero (catastrophic cancellation)
	if diff < 1e-3 {
		return true
	}
	mag := math.Max(math.Abs(float64(a)), math.Abs(float64(b)))
	if mag == 0 {
		return true
	}
	return diff/mag < relTol
}

func (b *Benchmark) cpuGramSchmidt() {
	b.cpuQ = make([]float32, b.M*b.N)
	b.cpuR = make([]float32, b.N*b.N)
	copy(b.cpuQ, b.q)

	for k := 0; k < b.N; k++ {
		var nrm float32
		for i := 0; i < b.M; i++ {
			nrm += b.cpuQ[i*b.N+k] * b.cpuQ[i*b.N+k]
		}

		b.cpuR[k*b.N+k] = float32(math.Sqrt(float64(nrm)))

		for i := 0; i < b.M; i++ {
			b.cpuQ[i*b.N+k] /= b.cpuR[k*b.N+k]
		}

		for j := k + 1; j < b.N; j++ {
			for i := 0; i < b.M; i++ {
				b.cpuR[k*b.N+j] += b.cpuQ[i*b.N+k] * b.cpuQ[i*b.N+j]
			}
			for i := 0; i < b.M; i++ {
				b.cpuQ[i*b.N+j] -= b.cpuQ[i*b.N+k] * b.cpuR[k*b.N+j]
			}
		}
	}
}
