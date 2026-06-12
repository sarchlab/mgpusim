// Package dmr implements the Delaunay Mesh Refinement benchmark
// from the Lonestar suite. It generates an initial triangulation,
// identifies triangles with small minimum angles, and iteratively
// refines them by inserting circumcenters.
package dmr

import (
	"log"
	"math"

	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

const (
	maxTrianglesFactor = 8
	maxPointsFactor    = 4
	maxPasses          = 50
	domainSize         = 100.0
	minAngleThreshold  = 25.0 // degrees
)

// Triangle matches the GPU struct layout (16 bytes).
type Triangle struct {
	V0, V1, V2 int32
	Active     int32
}

// IdentifyBadTrianglesArgs defines kernel arguments for identify_bad_triangles.
type IdentifyBadTrianglesArgs struct {
	Triangles     driver.Ptr // offset 0
	NumTriangles  int32      // offset 8
	Pad0          int32      // offset 12
	PX            driver.Ptr // offset 16
	PY            driver.Ptr // offset 24
	IsBad         driver.Ptr // offset 32
	MinAngleThresh float32   // offset 40
	Pad1          int32      // offset 44
	NumBad        driver.Ptr // offset 48
}

// RefineTrianglesArgs defines kernel arguments for refine_triangles.
type RefineTrianglesArgs struct {
	Triangles    driver.Ptr // offset 0
	NumTriangles int32      // offset 8
	Pad0         int32      // offset 12
	PX           driver.Ptr // offset 16
	PY           driver.Ptr // offset 24
	IsBad        driver.Ptr // offset 32
	TriLocks     driver.Ptr // offset 40
	PointCount   driver.Ptr // offset 48
	TriCount     driver.Ptr // offset 56
	MaxPoints    int32      // offset 64
	MaxTriangles int32      // offset 68
}

// Benchmark defines the DMR benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	identifyKernel *insts.KernelCodeObject
	refineKernel   *insts.KernelCodeObject

	NumPoints int
	MaxPasses int
	BlockSize int

	useUnifiedMemory bool

	hPX, hPY       []float32
	hTriangles      []Triangle
	hNumTriangles   int
	hNumPoints      int
	maxPoints       int
	maxTriangles    int

	dPX, dPY       driver.Ptr
	dTriangles     driver.Ptr
	dIsBad         driver.Ptr
	dTriLocks      driver.Ptr
	dNumBad        driver.Ptr
	dPointCount    driver.Ptr
	dTriCount      driver.Ptr
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

// NewBenchmark creates a new DMR benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.NumPoints = 16384
	b.MaxPasses = maxPasses
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

func (b *Benchmark) loadProgram() {
	b.identifyKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "identify_bad_triangles")
	if b.identifyKernel == nil {
		log.Panic("Failed to load identify_bad_triangles")
	}
	b.refineKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "refine_triangles")
	if b.refineKernel == nil {
		log.Panic("Failed to load refine_triangles")
	}
}

// simpleRand implements a basic LCG for deterministic random generation.
type simpleRand struct{ state uint64 }

func newDMRRand(seed uint64) *simpleRand {
	return &simpleRand{state: seed}
}

func (r *simpleRand) next() int32 {
	r.state = r.state*1103515245 + 12345
	return int32((r.state / 65536) % 2147483648)
}

func (b *Benchmark) generateMesh() {
	gridRes := int(math.Sqrt(float64(b.NumPoints)))
	if gridRes < 4 {
		gridRes = 4
	}

	initialPoints := (gridRes + 1) * (gridRes + 1)
	b.maxPoints = b.NumPoints * maxPointsFactor
	b.maxTriangles = b.NumPoints * maxTrianglesFactor
	b.hPX = make([]float32, b.maxPoints)
	b.hPY = make([]float32, b.maxPoints)
	b.hTriangles = make([]Triangle, b.maxTriangles)

	cellW := float32(domainSize) / float32(gridRes)
	cellH := float32(domainSize) / float32(gridRes)

	b.generateGridPoints(gridRes, cellW, cellH, initialPoints)
	b.hNumTriangles = b.generateTriangles(gridRes)
	b.hNumPoints = initialPoints
}

func (b *Benchmark) generateGridPoints(gridRes int, cellW, cellH float32,
	initialPoints int) {
	for j := 0; j <= gridRes; j++ {
		for i := 0; i <= gridRes; i++ {
			idx := j*(gridRes+1) + i
			if idx < b.maxPoints {
				b.hPX[idx] = float32(i) * cellW
				b.hPY[idx] = float32(j) * cellH
			}
		}
	}

	rng := newDMRRand(42)
	for j := 1; j < gridRes; j++ {
		for i := 1; i < gridRes; i++ {
			idx := j*(gridRes+1) + i
			if idx < b.maxPoints {
				b.hPX[idx] += (float32(rng.next()%1000)/1000.0 - 0.5) * cellW * 0.3
				b.hPY[idx] += (float32(rng.next()%1000)/1000.0 - 0.5) * cellH * 0.3
			}
		}
	}

	for i := initialPoints; i < b.maxPoints; i++ {
		b.hPX[i] = float32(rng.next()%10000) / 10000.0 * domainSize
		b.hPY[i] = float32(rng.next()%10000) / 10000.0 * domainSize
	}
}

func (b *Benchmark) generateTriangles(gridRes int) int {
	triIdx := 0
	for j := 0; j < gridRes; j++ {
		for i := 0; i < gridRes; i++ {
			bl := j*(gridRes+1) + i
			br, tl, tr := bl+1, (j+1)*(gridRes+1)+i, (j+1)*(gridRes+1)+i+1

			if bl < b.maxPoints && br < b.maxPoints &&
				tl < b.maxPoints && tr < b.maxPoints &&
				triIdx+1 < b.maxTriangles {
				b.hTriangles[triIdx] = Triangle{int32(bl), int32(br), int32(tl), 1}
				triIdx++
				b.hTriangles[triIdx] = Triangle{int32(br), int32(tr), int32(tl), 1}
				triIdx++
			}
		}
	}
	return triIdx
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
	b.generateMesh()

	if b.useUnifiedMemory {
		b.dPX = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.maxPoints*4))
		b.dPY = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.maxPoints*4))
		b.dTriangles = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.maxTriangles*16))
		b.dIsBad = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.maxTriangles*4))
		b.dTriLocks = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.maxTriangles*4))
		b.dNumBad = b.driver.AllocateUnifiedMemory(b.context, 4)
		b.dPointCount = b.driver.AllocateUnifiedMemory(b.context, 4)
		b.dTriCount = b.driver.AllocateUnifiedMemory(b.context, 4)
	} else {
		b.dPX = b.driver.AllocateMemory(b.context,
			uint64(b.maxPoints*4))
		b.dPY = b.driver.AllocateMemory(b.context,
			uint64(b.maxPoints*4))
		b.dTriangles = b.driver.AllocateMemory(b.context,
			uint64(b.maxTriangles*16))
		b.dIsBad = b.driver.AllocateMemory(b.context,
			uint64(b.maxTriangles*4))
		b.dTriLocks = b.driver.AllocateMemory(b.context,
			uint64(b.maxTriangles*4))
		b.dNumBad = b.driver.AllocateMemory(b.context, 4)
		b.dPointCount = b.driver.AllocateMemory(b.context, 4)
		b.dTriCount = b.driver.AllocateMemory(b.context, 4)
	}
}

func (b *Benchmark) exec() {
	b.driver.MemCopyH2D(b.context, b.dPX, b.hPX[:b.hNumPoints])
	b.driver.MemCopyH2D(b.context, b.dPY, b.hPY[:b.hNumPoints])
	b.driver.MemCopyH2D(b.context, b.dTriangles, b.hTriangles[:b.hNumTriangles])

	pointCount, triCount := int32(b.hNumPoints), int32(b.hNumTriangles)
	b.driver.MemCopyH2D(b.context, b.dPointCount, pointCount)
	b.driver.MemCopyH2D(b.context, b.dTriCount, triCount)

	for pass := 0; pass < b.MaxPasses; pass++ {
		b.driver.MemCopyD2H(b.context, &triCount, b.dTriCount)
		if triCount <= 0 {
			break
		}
		if !b.refinementPass(triCount) {
			break
		}
	}

	b.driver.MemCopyD2H(b.context, &triCount, b.dTriCount)
	if triCount > 0 && int(triCount) <= len(b.hTriangles) {
		b.driver.MemCopyD2H(b.context, b.hTriangles[:triCount], b.dTriangles)
	}
	b.hNumTriangles = int(triCount)
}

func (b *Benchmark) refinementPass(triCount int32) bool {
	blockSize := b.BlockSize
	grid := (int(triCount) + blockSize - 1) / blockSize
	globalSize := [3]uint32{uint32(grid * blockSize), 1, 1}
	localSize := [3]uint16{uint16(blockSize), 1, 1}

	numBad := int32(0)
	b.driver.MemCopyH2D(b.context, b.dNumBad, numBad)

	identifyArgs := IdentifyBadTrianglesArgs{
		Triangles: b.dTriangles, NumTriangles: triCount,
		PX: b.dPX, PY: b.dPY, IsBad: b.dIsBad,
		MinAngleThresh: minAngleThreshold, NumBad: b.dNumBad,
	}
	b.driver.LaunchKernel(b.context, b.identifyKernel,
		globalSize, localSize, &identifyArgs)

	b.driver.MemCopyD2H(b.context, &numBad, b.dNumBad)
	if numBad == 0 {
		return false
	}

	zeros := make([]int32, triCount)
	b.driver.MemCopyH2D(b.context, b.dTriLocks, zeros)

	refineArgs := RefineTrianglesArgs{
		Triangles: b.dTriangles, NumTriangles: triCount,
		PX: b.dPX, PY: b.dPY, IsBad: b.dIsBad,
		TriLocks: b.dTriLocks, PointCount: b.dPointCount,
		TriCount: b.dTriCount,
		MaxPoints: int32(b.maxPoints), MaxTriangles: int32(b.maxTriangles),
	}
	b.driver.LaunchKernel(b.context, b.refineKernel,
		globalSize, localSize, &refineArgs)
	return true
}

// Verify checks that mesh refinement produced some triangles.
func (b *Benchmark) Verify() {
	active := 0
	for i := 0; i < b.hNumTriangles; i++ {
		if b.hTriangles[i].Active != 0 {
			active++
		}
	}

	if active == 0 {
		log.Panicf("FAIL: no active triangles after refinement")
	}

	log.Printf("Passed! (%d active triangles)", active)
}
