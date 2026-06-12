// Package md5hash implements MD5 hash microbenchmark.
package md5hash

import (
	"log"
	"math/bits"

	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// KernelArgs defines kernel arguments for md5hash_kernel.
type KernelArgs struct {
	Input  driver.Ptr
	Output driver.Ptr
	N      int32
	Pad0   int32
}

// Benchmark defines the MD5 hash benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	kernel *insts.KernelCodeObject

	NumElements int
	BlockSize   int

	useUnifiedMemory bool

	hInput  []uint32
	hOutput []uint32

	dInput  driver.Ptr
	dOutput driver.Ptr
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

// NewBenchmark creates a new MD5 hash benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.NumElements = 1048576
	b.BlockSize = 256
	return b
}

// SelectGPU selects GPU.
func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
}

// SetUnifiedMemory uses unified memory.
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

// SetNumElements sets the number of elements.
func (b *Benchmark) SetNumElements(n int) {
	b.NumElements = n
}

func (b *Benchmark) loadProgram() {
	b.kernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "md5hash_kernel")
	if b.kernel == nil {
		log.Panic("Failed to load md5hash_kernel")
	}
}

// Run runs the benchmark.
func (b *Benchmark) Run() {
	b.loadProgram()

	for _, gpu := range b.gpus {
		b.driver.SelectGPU(b.context, gpu)
		b.queues = append(b.queues,
			b.driver.CreateCommandQueue(b.context))
	}

	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	n := b.NumElements

	// Each element is a 64-byte (16-word) input block
	b.hInput = make([]uint32, n*16)
	b.hOutput = make([]uint32, n*4)

	// Fill input with deterministic data
	for i := 0; i < n*16; i++ {
		b.hInput[i] = uint32(i*2654435761 + 1)
	}

	inputBytes := uint64(n * 16 * 4)
	outputBytes := uint64(n * 4 * 4)

	if b.useUnifiedMemory {
		b.dInput = b.driver.AllocateUnifiedMemory(b.context, inputBytes)
		b.dOutput = b.driver.AllocateUnifiedMemory(b.context, outputBytes)
	} else {
		b.dInput = b.driver.AllocateMemory(b.context, inputBytes)
		b.dOutput = b.driver.AllocateMemory(b.context, outputBytes)
	}

	b.driver.MemCopyH2D(b.context, b.dInput, b.hInput)
}

func (b *Benchmark) exec() {
	n := b.NumElements
	blockSize := b.BlockSize
	grid := (n + blockSize - 1) / blockSize

	globalSize := [3]uint32{uint32(grid * blockSize), 1, 1}
	localSize := [3]uint16{uint16(blockSize), 1, 1}

	args := KernelArgs{
		Input:  b.dInput,
		Output: b.dOutput,
		N:      int32(n),
	}
	b.driver.LaunchKernel(b.context, b.kernel,
		globalSize, localSize, &args)

	b.driver.MemCopyD2H(b.context, b.hOutput, b.dOutput)
}

// md5Block computes MD5 on a single 64-byte block (16 uint32 words).
func md5Block(M [16]uint32) [4]uint32 {
	// MD5 shift amounts
	s := [64]uint32{
		7, 12, 17, 22, 7, 12, 17, 22, 7, 12, 17, 22, 7, 12, 17, 22,
		5, 9, 14, 20, 5, 9, 14, 20, 5, 9, 14, 20, 5, 9, 14, 20,
		4, 11, 16, 23, 4, 11, 16, 23, 4, 11, 16, 23, 4, 11, 16, 23,
		6, 10, 15, 21, 6, 10, 15, 21, 6, 10, 15, 21, 6, 10, 15, 21,
	}

	// MD5 sin-based constants
	k := [64]uint32{
		0xd76aa478, 0xe8c7b756, 0x242070db, 0xc1bdceee,
		0xf57c0faf, 0x4787c62a, 0xa8304613, 0xfd469501,
		0x698098d8, 0x8b44f7af, 0xffff5bb1, 0x895cd7be,
		0x6b901122, 0xfd987193, 0xa679438e, 0x49b40821,
		0xf61e2562, 0xc040b340, 0x265e5a51, 0xe9b6c7aa,
		0xd62f105d, 0x02441453, 0xd8a1e681, 0xe7d3fbc8,
		0x21e1cde6, 0xc33707d6, 0xf4d50d87, 0x455a14ed,
		0xa9e3e905, 0xfcefa3f8, 0x676f02d9, 0x8d2a4c8a,
		0xfffa3942, 0x8771f681, 0x6d9d6122, 0xfde5380c,
		0xa4beea44, 0x4bdecfa9, 0xf6bb4b60, 0xbebfbc70,
		0x289b7ec6, 0xeaa127fa, 0xd4ef3085, 0x04881d05,
		0xd9d4d039, 0xe6db99e5, 0x1fa27cf8, 0xc4ac5665,
		0xf4292244, 0x432aff97, 0xab9423a7, 0xfc93a039,
		0x655b59c3, 0x8f0ccc92, 0xffeff47d, 0x85845dd1,
		0x6fa87e4f, 0xfe2ce6e0, 0xa3014314, 0x4e0811a1,
		0xf7537e82, 0xbd3af235, 0x2ad7d2bb, 0xeb86d391,
	}

	var a0 uint32 = 0x67452301
	var b0 uint32 = 0xefcdab89
	var c0 uint32 = 0x98badcfe
	var d0 uint32 = 0x10325476

	a, b, c, d := a0, b0, c0, d0

	for i := 0; i < 64; i++ {
		var f, g uint32
		if i < 16 {
			f = (b & c) | (^b & d)
			g = uint32(i)
		} else if i < 32 {
			f = (b & d) | (c & ^d)
			g = uint32((5*i + 1) % 16)
		} else if i < 48 {
			f = b ^ c ^ d
			g = uint32((3*i + 5) % 16)
		} else {
			f = c ^ (b | ^d)
			g = uint32((7 * i) % 16)
		}

		temp := d
		d = c
		c = b
		b = b + bits.RotateLeft32(a+f+k[i]+M[g], int(s[i]))
		a = temp
	}

	return [4]uint32{a0 + a, b0 + b, c0 + c, d0 + d}
}

// Verify verifies the result.
func (b *Benchmark) Verify() {
	n := b.NumElements
	errors := 0

	for i := 0; i < n; i++ {
		var M [16]uint32
		for j := 0; j < 16; j++ {
			M[j] = b.hInput[i*16+j]
		}

		expected := md5Block(M)

		for j := 0; j < 4; j++ {
			if b.hOutput[i*4+j] != expected[j] {
				if errors < 10 {
					log.Printf("Mismatch at element %d word %d: got 0x%08x, expected 0x%08x",
						i, j, b.hOutput[i*4+j], expected[j])
				}
				errors++
			}
		}
	}

	if errors > 0 {
		log.Panicf("FAIL: %d errors in md5hash verification", errors)
	}
	log.Printf("Passed!")
}
