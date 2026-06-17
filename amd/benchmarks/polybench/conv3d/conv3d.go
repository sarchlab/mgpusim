// Package conv3d implements the PolyBench 3D Convolution benchmark, ported
// from sarchlab/gpu_benchmarks (tier2/polybench_3dconv) for the MGPUSim
// MI300A (CDNA3 / gfx942) model.
//
// It convolves an NxNxN float volume with a small
// filter_size x filter_size x filter_size 3D filter, one work-item per
// output element. The kernel binary is compiled for gfx942 only (see
// native/), so the benchmark must be run with `-arch cdna3` (the MI300A
// configuration).
package conv3d

import (
	"log"
	"math"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// blockSize is the constant work-group edge length, matching BLOCK_SIZE in
// native/polybench_3dconv.cpp. Because the kernel uses this compile-time
// constant instead of blockDim, the compiler emits no hidden ABI arguments.
const blockSize = 8

// KernelArgs defines the kernel arguments for the gfx942 (CDNA3) kernel.
//
// The layout below is verified against the compiled kernel's AMDGPU
// metadata (kernarg_segment_size = 32): three 8-byte global_buffer
// pointers followed by two 4-byte by_value int scalars, packed with no
// padding (mgpusim serializes args with binary.Write, which does not
// insert alignment padding). The kernel reads only blockIdx/threadIdx with
// a constant block size, so no hidden ABI arguments are emitted.
type KernelArgs struct {
	Input      driver.Ptr // offset 0
	Filter     driver.Ptr // offset 8
	Output     driver.Ptr // offset 16
	N          int32      // offset 24
	FilterSize int32      // offset 28
}

// Benchmark defines the 3D convolution benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	queue   *driver.CommandQueue
	hsaco   *insts.KernelCodeObject
	gpus    []int

	Arch arch.Type
	// N is the edge length of the NxNxN volume.
	N int
	// FilterSize is the edge length of the cubic filter (e.g. 3).
	FilterSize int

	input   []float32
	filter  []float32
	gInput  driver.Ptr
	gFilter driver.Ptr
	gOutput driver.Ptr

	useUnifiedMemory bool
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new 3D convolution benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)

	b.driver = driver
	b.context = b.driver.Init()
	b.queue = driver.CreateCommandQueue(b.context)

	return b
}

func (b *Benchmark) loadProgram() {
	b.hsaco = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "conv3d_kernel")
	if b.hsaco == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// SelectGPU selects the GPUs to run on. conv3d uses a single GPU.
func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
}

// SetUnifiedMemory requests the use of unified memory.
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

// Run runs the benchmark.
func (b *Benchmark) Run() {
	if b.Arch != arch.CDNA3 {
		log.Panic("the polybench 3dconv benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300a")
	}

	b.loadProgram()

	b.driver.SelectGPU(b.context, b.gpus[0])
	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	if b.N <= 0 {
		b.N = 32
	}
	if b.FilterSize <= 0 {
		b.FilterSize = 3
	}

	n := b.N
	volElems := n * n * n
	filtElems := b.FilterSize * b.FilterSize * b.FilterSize

	// Deterministic host init, reproduced exactly in Verify().
	b.input = make([]float32, volElems)
	for i := 0; i < volElems; i++ {
		b.input[i] = float32(i%100) / 100.0
	}

	b.filter = make([]float32, filtElems)
	var filtSum float32
	for i := 0; i < filtElems; i++ {
		b.filter[i] = float32(i%10 + 1)
		filtSum += b.filter[i]
	}
	for i := 0; i < filtElems; i++ {
		b.filter[i] /= filtSum
	}

	if b.useUnifiedMemory {
		b.gInput = b.driver.AllocateUnifiedMemory(
			b.context, uint64(volElems*4))
		b.gFilter = b.driver.AllocateUnifiedMemory(
			b.context, uint64(filtElems*4))
		b.gOutput = b.driver.AllocateUnifiedMemory(
			b.context, uint64(volElems*4))
	} else {
		b.gInput = b.driver.AllocateMemory(b.context, uint64(volElems*4))
		b.gFilter = b.driver.AllocateMemory(b.context, uint64(filtElems*4))
		b.gOutput = b.driver.AllocateMemory(b.context, uint64(volElems*4))
	}

	b.driver.MemCopyH2D(b.context, b.gInput, b.input)
	b.driver.MemCopyH2D(b.context, b.gFilter, b.filter)
}

func (b *Benchmark) exec() {
	n := b.N
	gridDim := uint32((n + blockSize - 1) / blockSize)
	global := gridDim * blockSize

	args := KernelArgs{
		Input:      b.gInput,
		Filter:     b.gFilter,
		Output:     b.gOutput,
		N:          int32(n),
		FilterSize: int32(b.FilterSize),
	}

	// Kernel maps: x->k, y->j, z->i; all three dims span the NxNxN volume.
	b.driver.EnqueueLaunchKernel(
		b.queue,
		b.hsaco,
		[3]uint32{global, global, global},
		[3]uint16{blockSize, blockSize, blockSize},
		&args,
	)

	b.driver.DrainCommandQueue(b.queue)
}

// Verify checks the GPU result against a CPU reference computation.
func (b *Benchmark) Verify() { //nolint:funlen,gocognit
	n := b.N
	fs := b.FilterSize
	half := fs / 2

	gpuOut := make([]float32, n*n*n)
	b.driver.MemCopyD2H(b.context, gpuOut, b.gOutput)

	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			for k := 0; k < n; k++ {
				var sum float64
				for fi := 0; fi < fs; fi++ {
					ii := i - half + fi
					if ii < 0 || ii >= n {
						continue
					}
					for fj := 0; fj < fs; fj++ {
						jj := j - half + fj
						if jj < 0 || jj >= n {
							continue
						}
						for fk := 0; fk < fs; fk++ {
							kk := k - half + fk
							if kk < 0 || kk >= n {
								continue
							}
							sum += float64(b.input[(ii*n+jj)*n+kk]) *
								float64(b.filter[(fi*fs+fj)*fs+fk])
						}
					}
				}

				ref := sum
				got := float64(gpuOut[(i*n+j)*n+k])

				denom := math.Abs(ref)
				if denom < 1.0 {
					denom = 1.0
				}
				if math.Abs(ref-got)/denom > 1e-3 {
					log.Fatalf("At (%d,%d,%d), expected %f, but got %f.\n",
						i, j, k, ref, got)
				}
			}
		}
	}

	log.Printf("Passed!\n")
}
