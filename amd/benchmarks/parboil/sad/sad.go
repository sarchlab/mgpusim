// Package sad implements the Parboil SAD (Sum of Absolute Differences) benchmark.
// Performs motion estimation for video encoding by computing SADs between
// macroblocks and finding the best matching candidate positions.
package sad

import (
	"log"
	"math/rand"

	// embed hsaco files
	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

// Matches: ComputeSAD(ref_frame, cur_frame, frame_width, block_size_sad,
//
//	search_range, sad_out, num_mb_x, num_mb_y)
// ComputeSADKernelArgs defines the explicit kernel arguments for ComputeSAD.
// V5 implicit args (256 bytes) are filled automatically by the driver.
type ComputeSADKernelArgs struct {
	RefFrame     driver.Ptr
	CurFrame     driver.Ptr
	FrameWidth   int32
	BlockSizeSAD int32
	SearchRange  int32
	PadAlign     int32 // alignment padding for 8-byte SadOut
	SadOut       driver.Ptr
	NumMbX       int32
	NumMbY       int32
}

// FindMinSADKernelArgs defines the explicit kernel arguments for FindMinSAD.
// V5 implicit args (256 bytes) are filled automatically by the driver.
type FindMinSADKernelArgs struct {
	SadValues       driver.Ptr
	MinSad          driver.Ptr
	MinPos          driver.Ptr
	TotalCandidates int32
	NumMacroblocks  int32
}

// Benchmark defines the Parboil SAD benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	computeSADKernel *insts.KernelCodeObject
	findMinSADKernel *insts.KernelCodeObject

	FrameWidth          int
	BlockSizeSAD        int
	SearchRange         int
	NumThreadsPerBlock  int

	useUnifiedMemory bool

	hRefFrame []uint8
	hCurFrame []uint8
	hSadOut   []int32
	hMinSad   []int32
	hMinPos   []int32

	dRefFrame driver.Ptr
	dCurFrame driver.Ptr
	dSadOut   driver.Ptr
	dMinSad   driver.Ptr
	dMinPos   driver.Ptr
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

// NewBenchmark creates a new Parboil SAD benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.FrameWidth = 256
	b.BlockSizeSAD = 16
	b.SearchRange = 16
	b.NumThreadsPerBlock = 256
	return b
}

// SelectGPU selects GPU.
func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
}

// SetUnifiedMemory uses Unified Memory.
func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

func (b *Benchmark) loadProgram() {
	b.computeSADKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "ComputeSAD")
	if b.computeSADKernel == nil {
		log.Panic("Failed to load ComputeSAD kernel binary")
	}

	b.findMinSADKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "FindMinSAD")
	if b.findMinSADKernel == nil {
		log.Panic("Failed to load FindMinSAD kernel binary")
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
	rand.Seed(42)

	frameSize := b.FrameWidth * b.FrameWidth
	numMbX := b.FrameWidth / b.BlockSizeSAD
	numMbY := b.FrameWidth / b.BlockSizeSAD
	numMacroblocks := numMbX * numMbY
	searchDim := 2*b.SearchRange + 1
	totalCandidates := searchDim * searchDim

	b.hRefFrame = make([]uint8, frameSize)
	b.hCurFrame = make([]uint8, frameSize)
	b.hSadOut = make([]int32, numMacroblocks*totalCandidates)
	b.hMinSad = make([]int32, numMacroblocks)
	b.hMinPos = make([]int32, numMacroblocks)

	for i := 0; i < frameSize; i++ {
		b.hRefFrame[i] = uint8(rand.Intn(256))
		b.hCurFrame[i] = uint8(rand.Intn(256))
	}

	if b.useUnifiedMemory {
		b.dRefFrame = b.driver.AllocateUnifiedMemory(b.context, uint64(frameSize))
		b.dCurFrame = b.driver.AllocateUnifiedMemory(b.context, uint64(frameSize))
		b.dSadOut = b.driver.AllocateUnifiedMemory(b.context, uint64(numMacroblocks*totalCandidates*4))
		b.dMinSad = b.driver.AllocateUnifiedMemory(b.context, uint64(numMacroblocks*4))
		b.dMinPos = b.driver.AllocateUnifiedMemory(b.context, uint64(numMacroblocks*4))
	} else {
		b.dRefFrame = b.driver.AllocateMemory(b.context, uint64(frameSize))
		b.dCurFrame = b.driver.AllocateMemory(b.context, uint64(frameSize))
		b.dSadOut = b.driver.AllocateMemory(b.context, uint64(numMacroblocks*totalCandidates*4))
		b.dMinSad = b.driver.AllocateMemory(b.context, uint64(numMacroblocks*4))
		b.dMinPos = b.driver.AllocateMemory(b.context, uint64(numMacroblocks*4))
	}

	b.driver.MemCopyH2D(b.context, b.dRefFrame, b.hRefFrame)
	b.driver.MemCopyH2D(b.context, b.dCurFrame, b.hCurFrame)
	b.driver.MemCopyH2D(b.context, b.dSadOut, b.hSadOut)
	b.driver.MemCopyH2D(b.context, b.dMinSad, b.hMinSad)
	b.driver.MemCopyH2D(b.context, b.dMinPos, b.hMinPos)
}

func (b *Benchmark) exec() {
	numMbX := b.FrameWidth / b.BlockSizeSAD
	numMbY := b.FrameWidth / b.BlockSizeSAD
	numMacroblocks := numMbX * numMbY
	searchDim := 2*b.SearchRange + 1
	totalCandidates := searchDim * searchDim

	// Launch ComputeSAD: grid = (numMacroblocks,1,1), block = (numThreadsPerBlock,1,1)
	sadGlobalSize := [3]uint32{uint32(numMacroblocks * b.NumThreadsPerBlock), 1, 1}
	sadLocalSize := [3]uint16{uint16(b.NumThreadsPerBlock), 1, 1}

	t0 := float64(b.driver.Engine.CurrentTime())
	b.launchComputeSAD(sadGlobalSize, sadLocalSize, numMbX, numMbY)
	t1 := float64(b.driver.Engine.CurrentTime())

	// Launch FindMinSAD: grid = ((numMacroblocks+255)/256,1,1), block = (256,1,1)
	minBlockSize := 256
	minBlocks := (numMacroblocks + minBlockSize - 1) / minBlockSize
	minGlobalSize := [3]uint32{uint32(minBlocks * minBlockSize), 1, 1}
	minLocalSize := [3]uint16{uint16(minBlockSize), 1, 1}

	b.launchFindMinSAD(minGlobalSize, minLocalSize, totalCandidates, numMacroblocks)
	t2 := float64(b.driver.Engine.CurrentTime())

	log.Printf("PER_KERNEL_TIME ComputeSAD %.15e\n", t1-t0)
	log.Printf("PER_KERNEL_TIME FindMinSAD %.15e\n", t2-t1)

	b.driver.MemCopyD2H(b.context, b.hMinSad, b.dMinSad)
	b.driver.MemCopyD2H(b.context, b.hMinPos, b.dMinPos)
}

func (b *Benchmark) launchComputeSAD(
	globalSize [3]uint32, localSize [3]uint16,
	numMbX, numMbY int,
) {
	args := ComputeSADKernelArgs{
		RefFrame:     b.dRefFrame,
		CurFrame:     b.dCurFrame,
		FrameWidth:   int32(b.FrameWidth),
		BlockSizeSAD: int32(b.BlockSizeSAD),
		SearchRange:  int32(b.SearchRange),
		SadOut:       b.dSadOut,
		NumMbX:       int32(numMbX),
		NumMbY:       int32(numMbY),
	}
	b.driver.LaunchKernel(b.context,
		b.computeSADKernel,
		globalSize, localSize,
		&args,
	)
}

func (b *Benchmark) launchFindMinSAD(
	globalSize [3]uint32, localSize [3]uint16,
	totalCandidates, numMacroblocks int,
) {
	args := FindMinSADKernelArgs{
		SadValues:       b.dSadOut,
		MinSad:          b.dMinSad,
		MinPos:          b.dMinPos,
		TotalCandidates: int32(totalCandidates),
		NumMacroblocks:  int32(numMacroblocks),
	}
	b.driver.LaunchKernel(b.context,
		b.findMinSADKernel,
		globalSize, localSize,
		&args,
	)
}

// Verify checks that MinSad values are within the expected range.
func (b *Benchmark) Verify() {
	maxPossibleSAD := int32(b.BlockSizeSAD * b.BlockSizeSAD * 255)
	numMbX := b.FrameWidth / b.BlockSizeSAD
	numMbY := b.FrameWidth / b.BlockSizeSAD
	numMacroblocks := numMbX * numMbY

	errors := 0
	for i := 0; i < numMacroblocks; i++ {
		if b.hMinSad[i] < 0 || b.hMinSad[i] > maxPossibleSAD {
			if errors < 5 {
				log.Printf("Macroblock %d: min_sad=%d out of range [0,%d]",
					i, b.hMinSad[i], maxPossibleSAD)
			}
			errors++
		}
	}

	if errors > 0 {
		log.Panicf("FAIL: %d macroblocks have invalid min SAD values", errors)
	}

	log.Printf("Passed!")
}
