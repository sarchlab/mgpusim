// Package srad runs the Rodinia SRAD hybrid CPU-GPU diffusion benchmark.
package srad

import (
	// embed hsaco files
	_ "embed"
	"fmt"
	"log"
	"math"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

// blockSize is the (constant) thread-block dimension baked into the kernel.
const blockSize = 16

type sradFields struct {
	E, W, N, S, J, C gpuruntime.Ptr
	Cols, Rows       int32
}

type srad1Args struct {
	sradFields
	QSqr   float32
	_      uint32
	Launch benchmarks.GFX942LaunchDimensions
}
type srad2Args struct {
	sradFields
	Lambda, QSqr float32
	Launch       benchmarks.GFX942LaunchDimensions
}

// Benchmark preserves the image downloaded between iterations for the CPU ROI
// calculation and for independent validation of the actual final step.
type Benchmark struct {
	*State
	benchmarks.Checkpoint[State]
	rt            *gpuruntime.Runtime
	first, second *insts.KernelCodeObject
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

func NewBenchmark(rt *gpuruntime.Runtime) *Benchmark {
	b := &Benchmark{rt: rt, State: &State{}}
	b.Checkpoint = benchmarks.NewCheckpoint(b.State, b.bind)
	b.Context = rt.Init()
	return b
}

func (b *Benchmark) loadProgram() {
	b.first = insts.LoadKernelCodeObjectFromBytes(cdna3HSACOBytes, "srad_cuda_1")
	b.second = insts.LoadKernelCodeObjectFromBytes(cdna3HSACOBytes, "srad_cuda_2")
	if b.first == nil || b.second == nil {
		panic("could not load archived SRAD kernels")
	}
}

func (b *Benchmark) SelectGPU(gpus []int) { b.Gpus = gpus }
func (b *Benchmark) SetUnifiedMemory()    { b.UseUnifiedMemory = true }

func (b *Benchmark) setup() {
	if b.Arch != arch.CDNA3 {
		panic("SRAD requires CDNA3")
	}
	if b.ImageSize <= 0 {
		b.ImageSize = 32
	}
	if b.NumIterations <= 0 {
		b.NumIterations = 10
	}
	if b.Lambda == 0 {
		b.Lambda = 0.5
	}
	b.ImageSize = (b.ImageSize + blockSize - 1) / blockSize * blockSize
	n := b.ImageSize
	b.loadProgram()
	b.rt.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.rt.CreateCommandQueue(b.Context)
	b.Initial = make([]float32, n*n)
	b.Final = make([]float32, n*n)
	rand := benchmarks.NewGlibcRand(7)
	for i := range b.Initial {
		b.Initial[i] = float32(math.Exp(float64(float32(rand.Next()) / 2147483647)))
	}
	b.Current = append([]float32(nil), b.Initial...)
	alloc := b.rt.AllocateMemory
	if b.UseUnifiedMemory {
		alloc = b.rt.AllocateUnifiedMemory
	}
	for i := range b.Buffers {
		b.Buffers[i] = alloc(b.Context, uint64(4*n*n))
	}
}

func roiVariation(image []float32, n int) float32 {
	edge := min(n, 128)
	var sum, squareSum float32
	for i := 0; i < edge; i++ {
		for j := 0; j < edge; j++ {
			value := image[i*n+j]
			sum += value
			squareSum += float32(value * value)
		}
	}
	count := float32(edge * edge)
	mean := sum / count
	meanSquare := float32(mean * mean)
	return (squareSum/count - meanSquare) / meanSquare
}

func (b *Benchmark) launchStep() {
	n := b.ImageSize
	grid, group := [3]uint32{uint32(n), uint32(n), 1}, [3]uint16{blockSize, blockSize, 1}
	buffers := b.Buffers
	fields := sradFields{J: buffers[0], C: buffers[1], E: buffers[2], W: buffers[3], S: buffers[4], N: buffers[5],
		Cols: int32(n), Rows: int32(n)}
	q := roiVariation(b.Current, n)
	launch := benchmarks.NewGFX942LaunchDimensions(grid, group)
	first := srad1Args{sradFields: fields, QSqr: q, Launch: launch}
	second := srad2Args{sradFields: fields, Lambda: b.Lambda, QSqr: q, Launch: launch}
	b.Upload(b.rt, b.Queue, b.Buffers[0], b.Current)
	b.rt.EnqueueLaunchKernel(b.Queue, b.first, grid, group, &first)
	b.rt.EnqueueLaunchKernel(b.Queue, b.second, grid, group, &second)
}

func (b *Benchmark) cpuStep(input []float32, result *[6][]float32) {
	n := b.ImageSize
	q := roiVariation(input, n)
	J, C, E, W, S, N := result[0], result[1], result[2], result[3], result[4], result[5]
	for k, center := range input {
		row, col := k/n, k%n
		up, down := max(row-1, 0), min(row+1, n-1)
		left, right := max(col-1, 0), min(col+1, n-1)
		N[k], S[k] = input[up*n+col]-center, input[down*n+col]-center
		W[k], E[k] = input[row*n+left]-center, input[row*n+right]-center
		gradient := (float32(N[k]*N[k]) + float32(S[k]*S[k]) + float32(W[k]*W[k]) + float32(E[k]*E[k])) /
			float32(center*center)
		laplacian := (N[k] + S[k] + W[k] + E[k]) / center
		num := float32(0.5*float64(gradient) - (1.0/16.0)*float64(float32(laplacian*laplacian)))
		den := float32(1 + 0.25*float64(laplacian))
		qs := num / float32(den*den)
		den = (qs - q) / float32(q*(1+q))
		C[k] = min(1, max(0, float32(1/(1+float64(den)))))
	}
	for k, center := range input {
		row, col := k/n, k%n
		south, east := min(row+1, n-1)*n+col, row*n+min(col+1, n-1)
		divergence := float32(C[k]*N[k]) + float32(C[south]*S[k]) + float32(C[k]*W[k]) + float32(C[east]*E[k])
		J[k] = float32(float64(center) + 0.25*float64(b.Lambda)*float64(divergence))
	}
}

func cpuBuffers(n int) [6][]float32 {
	var result [6][]float32
	for i := range result {
		result[i] = make([]float32, n*n)
	}
	return result
}

func compareField(actual, expected []float32, tolerance float64, field int) error {
	for i, value := range actual {
		got, want := float64(value), float64(expected[i])
		if math.IsNaN(got) || math.IsInf(got, 0) || math.IsNaN(want) || math.IsInf(want, 0) ||
			math.Abs(got-want) > tolerance*math.Max(1, math.Abs(want)) {
			return fmt.Errorf("SRAD field %d at %d: got %g, expected %g", field, i, got, want)
		}
		if field == 1 && (got < 0 || got > 1 || want < 0 || want > 1) {
			return fmt.Errorf("SRAD coefficient at %d is outside [0,1]", i)
		}
	}
	return nil
}

func (b *Benchmark) verify() error {
	trajectory := cpuBuffers(b.ImageSize)
	prev := append([]float32(nil), b.Initial...)
	for i := 0; i < b.NumIterations; i++ {
		b.cpuStep(prev, &trajectory)
		prev, trajectory[0] = trajectory[0], prev
	}
	if err := compareField(b.Final, prev, 1e-3, 0); err != nil {
		return err
	}
	finalStep := cpuBuffers(b.ImageSize)
	b.cpuStep(b.Current, &finalStep)
	if err := compareField(b.Final, finalStep[0], 1e-5, 0); err != nil {
		return err
	}
	actual := make([]float32, len(b.Final))
	for i, readback := range b.Readbacks {
		if err := benchmarks.Read(b.rt, readback, actual); err != nil {
			return err
		}
		var tolerance float64
		if i == 0 {
			tolerance = 1e-5
		}
		if err := compareField(actual, finalStep[i+1], tolerance, i+1); err != nil {
			return err
		}
	}
	log.Printf("Passed!\n")
	return nil
}

func (b *Benchmark) handleImage() error {
	final := b.Iteration == b.NumIterations-1
	dst := b.Current
	if final {
		dst = b.Final
	}
	if err := benchmarks.Read(b.rt, b.Readbacks[0], dst); err != nil {
		return err
	}
	b.Readbacks = nil
	b.Iteration++
	if !final {
		b.launchStep()
		b.Phase = 1
		return nil
	}
	if !b.Verification {
		b.Ended = true
		return nil
	}
	for _, buffer := range b.Buffers[1:] {
		b.Readbacks = append(b.Readbacks, benchmarks.Download(b.rt, b.Queue, buffer, len(dst)*4))
	}
	b.Phase = 3
	return nil
}

func (b *Benchmark) Step() error {
	if err := b.Prepare(); err != nil {
		return err
	}
	if err := b.ReleaseUploads(b.rt); err != nil {
		return err
	}
	switch b.Phase {
	case 0:
		b.setup()
		b.launchStep()
		b.Phase = 1
	case 1:
		b.Readbacks = append(b.Readbacks, benchmarks.Download(b.rt, b.Queue, b.Buffers[0], len(b.Current)*4))
		b.Phase = 2
	case 2:
		return b.handleImage()
	case 3:
		if err := b.verify(); err != nil {
			return err
		}
		b.Readbacks = nil
		b.Ended = true
	}
	return nil
}
