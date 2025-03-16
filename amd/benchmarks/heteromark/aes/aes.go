// Package aes implements the AES benchmark form Hetero-Mark.
package aes

import (
	"crypto/aes"
	"log"
	"math/rand"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
	"github.com/sarchlab/mgpusim/v4/amd/kernels"
)

var expandedKey = []uint32{
	0x00010203, 0x04050607,
	0x08090a0b, 0x0c0d0e0f,
	0x10111213, 0x14151617,
	0x18191a1b, 0x1c1d1e1f,
	0xa573c29f, 0xa176c498,
	0xa97fce93, 0xa572c09c,
	0x1651a8cd, 0x0244beda,
	0x1a5da4c1, 0x0640bade,
	0xae87dff0, 0x0ff11b68,
	0xa68ed5fb, 0x03fc1567,
	0x6de1f148, 0x6fa54f92,
	0x75f8eb53, 0x73b8518d,
	0xc656827f, 0xc9a79917,
	0x6f294cec, 0x6cd5598b,
	0x3de23a75, 0x524775e7,
	0x27bf9eb4, 0x5407cf39,
	0x0bdc905f, 0xc27b0948,
	0xad5245a4, 0xc1871c2f,
	0x45f5a660, 0x17b2d387,
	0x300d4d33, 0x640a820a,
	0x7ccff71c, 0xbeb4fe54,
	0x13e6bbf0, 0xd261a7df,
	0xf01afafe, 0xe7a82979,
	0xd7a5644a, 0xb3afe640,
	0x2541fe71, 0x9bf50025,
	0x8813bbd5, 0x5a721c0a,
	0x4e5a6699, 0xa9f24fe0,
	0x7e572baa, 0xcdf8cdea,
	0x24fc79cc, 0xbf0979e9,
	0x371ac23c, 0x6d68de36}

var s = []uint8{
	0x63, 0x7c, 0x77, 0x7b, 0xf2, 0x6b, 0x6f, 0xc5, 0x30, 0x01, 0x67, 0x2b, 0xfe, 0xd7, 0xab, 0x76,
	0xca, 0x82, 0xc9, 0x7d, 0xfa, 0x59, 0x47, 0xf0, 0xad, 0xd4, 0xa2, 0xaf, 0x9c, 0xa4, 0x72, 0xc0,
	0xb7, 0xfd, 0x93, 0x26, 0x36, 0x3f, 0xf7, 0xcc, 0x34, 0xa5, 0xe5, 0xf1, 0x71, 0xd8, 0x31, 0x15,
	0x04, 0xc7, 0x23, 0xc3, 0x18, 0x96, 0x05, 0x9a, 0x07, 0x12, 0x80, 0xe2, 0xeb, 0x27, 0xb2, 0x75,
	0x09, 0x83, 0x2c, 0x1a, 0x1b, 0x6e, 0x5a, 0xa0, 0x52, 0x3b, 0xd6, 0xb3, 0x29, 0xe3, 0x2f, 0x84,
	0x53, 0xd1, 0x00, 0xed, 0x20, 0xfc, 0xb1, 0x5b, 0x6a, 0xcb, 0xbe, 0x39, 0x4a, 0x4c, 0x58, 0xcf,
	0xd0, 0xef, 0xaa, 0xfb, 0x43, 0x4d, 0x33, 0x85, 0x45, 0xf9, 0x02, 0x7f, 0x50, 0x3c, 0x9f, 0xa8,
	0x51, 0xa3, 0x40, 0x8f, 0x92, 0x9d, 0x38, 0xf5, 0xbc, 0xb6, 0xda, 0x21, 0x10, 0xff, 0xf3, 0xd2,
	0xcd, 0x0c, 0x13, 0xec, 0x5f, 0x97, 0x44, 0x17, 0xc4, 0xa7, 0x7e, 0x3d, 0x64, 0x5d, 0x19, 0x73,
	0x60, 0x81, 0x4f, 0xdc, 0x22, 0x2a, 0x90, 0x88, 0x46, 0xee, 0xb8, 0x14, 0xde, 0x5e, 0x0b, 0xdb,
	0xe0, 0x32, 0x3a, 0x0a, 0x49, 0x06, 0x24, 0x5c, 0xc2, 0xd3, 0xac, 0x62, 0x91, 0x95, 0xe4, 0x79,
	0xe7, 0xc8, 0x37, 0x6d, 0x8d, 0xd5, 0x4e, 0xa9, 0x6c, 0x56, 0xf4, 0xea, 0x65, 0x7a, 0xae, 0x08,
	0xba, 0x78, 0x25, 0x2e, 0x1c, 0xa6, 0xb4, 0xc6, 0xe8, 0xdd, 0x74, 0x1f, 0x4b, 0xbd, 0x8b, 0x8a,
	0x70, 0x3e, 0xb5, 0x66, 0x48, 0x03, 0xf6, 0x0e, 0x61, 0x35, 0x57, 0xb9, 0x86, 0xc1, 0x1d, 0x9e,
	0xe1, 0xf8, 0x98, 0x11, 0x69, 0xd9, 0x8e, 0x94, 0x9b, 0x1e, 0x87, 0xe9, 0xce, 0x55, 0x28, 0xdf,
	0x8c, 0xa1, 0x89, 0x0d, 0xbf, 0xe6, 0x42, 0x68, 0x41, 0x99, 0x2d, 0x0f, 0xb0, 0x54, 0xbb, 0x16,
}

// KernelArgs defines kernel arguments
type KernelArgs struct {
	Input               driver.Ptr
	ExpandedKey         driver.Ptr
	S                   driver.Ptr
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

// Benchmark defines a benchmark
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	hsaco   *insts.HsaCo
	gpus    []int

	Length       int
	input        []byte
	key          []byte
	expandedKey  []uint32
	s            []byte
	gInput       driver.Ptr
	gExpandedKey []driver.Ptr
	gS           []driver.Ptr

	useUnifiedMemory bool
}

// NewBenchmark returns a benchmark
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = b.driver.Init()
	b.loadProgram()
	return b
}

// SelectGPU select GPU
func (b *Benchmark) SelectGPU(gpuIDs []int) {
	b.gpus = gpuIDs
	b.gExpandedKey = make([]driver.Ptr, len(gpuIDs))
	b.gS = make([]driver.Ptr, len(gpuIDs))
}

// SetUnifiedMemory uses Unified Memory
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

//go:embed kernels.hsaco
var hsacoBytes []byte

func (b *Benchmark) loadProgram() {
	b.hsaco = kernels.LoadProgramFromMemory(hsacoBytes, "Encrypt")
	if b.hsaco == nil {
		log.Panic("Failed to load kernel binary")
	}
}

func (b *Benchmark) initMem() {
	b.key = []byte{
		0, 1, 2, 3, 4, 5, 6, 7,
		8, 9, 10, 11, 12, 13, 14, 15,
		16, 17, 18, 19, 20, 21, 22, 23,
		24, 25, 26, 27, 28, 29, 30, 31,
	}
	b.expandedKey = expandedKey
	b.s = s

	b.input = make([]byte, b.Length)
	for i := 0; i < b.Length; i++ {
		b.input[i] = byte(rand.Uint32())
		//b.input[i] = byte(i)
		// b.input[i] = 0
	}

	if b.useUnifiedMemory {
		b.gInput = b.driver.AllocateUnifiedMemory(b.context, uint64(b.Length))
	} else {
		b.gInput = b.driver.AllocateMemory(b.context, uint64(b.Length))
		b.driver.Distribute(b.context, b.gInput, uint64(b.Length), b.gpus)
	}

	for i, gpu := range b.gpus {
		b.driver.SelectGPU(b.context, gpu)
		if b.useUnifiedMemory {
			b.gExpandedKey[i] = b.driver.AllocateUnifiedMemory(
				b.context, uint64(len(b.expandedKey)*4))
			b.gS[i] = b.driver.AllocateUnifiedMemory(b.context, uint64(len(b.s)))
		} else {
			b.gExpandedKey[i] = b.driver.AllocateMemory(
				b.context, uint64(len(b.expandedKey)*4))
			b.gS[i] = b.driver.AllocateMemory(b.context, uint64(len(b.s)))
		}

		b.driver.MemCopyH2D(b.context, b.gExpandedKey[i], b.expandedKey)
		b.driver.MemCopyH2D(b.context, b.gS[i], b.s)
	}

	b.driver.MemCopyH2D(b.context, b.gInput, b.input)
}

// Run runs
func (b *Benchmark) Run() {
	b.driver.SelectGPU(b.context, b.gpus[0])
	b.initMem()
	b.LaunchKernel()
}

// LaunchKernel launches kernel
func (b *Benchmark) LaunchKernel() {
	queues := make([]*driver.CommandQueue, len(b.gpus))
	numWi := b.Length / 16
	for i, gpu := range b.gpus {
		b.driver.SelectGPU(b.context, gpu)
		queues[i] = b.driver.CreateCommandQueue(b.context)

		kernArg := KernelArgs{
			b.gInput,
			b.gExpandedKey[i],
			b.gS[i],
			int64(i * numWi / len(b.gpus)), 0, 0}
		b.driver.EnqueueLaunchKernel(
			queues[i],
			b.hsaco,
			[3]uint32{uint32(numWi / len(b.gpus)), 1, 1},
			[3]uint16{64, 1, 1},
			&kernArg)
	}

	for _, q := range queues {
		b.driver.DrainCommandQueue(q)
	}
}

// Verify verifies
func (b *Benchmark) Verify() {
	gpuOutput := make([]byte, b.Length)
	b.driver.MemCopyD2H(b.context, gpuOutput, b.gInput)

	cpuOutput := b.cpuEncrypt()

	for i := 0; i < b.Length; i++ {
		if cpuOutput[i] != gpuOutput[i] {
			log.Panicf("Mismatch at position %d: should be %02x but get %02x",
				i, cpuOutput[i], gpuOutput[i])
		}
	}

	log.Printf("\nPassed!\n")
}

func (b *Benchmark) cpuEncrypt() []byte {
	cpuOutput := make([]byte, b.Length)
	cipherBlock, err := aes.NewCipher(b.key)
	if err != nil {
		panic(err)
	}
	for i := 0; i < b.Length/16; i++ {
		cipherBlock.Encrypt(cpuOutput[i*16:], b.input[i*16:])
	}
	return cpuOutput
}
