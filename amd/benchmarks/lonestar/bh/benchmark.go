// Package bh implements the Barnes-Hut n-body simulation benchmark
// from the Lonestar suite. Phase 1 builds a quadtree on host;
// Phase 2 computes gravitational forces on GPU via tree traversal;
// Phase 3 updates body positions/velocities on GPU.
package bh

import (
	"log"
	"math"

	_ "embed"

	"github.com/sarchlab/mgpusim/v4/amd/driver"
	"github.com/sarchlab/mgpusim/v4/amd/insts"
)

const (
	maxNodesFactor = 5
	domainSize     = 100.0
	defaultDT      = 0.01
	defaultEps2    = 0.01
	defaultTheta   = 1.0
)

// TreeNode matches the GPU struct layout exactly (48 bytes).
type TreeNode struct {
	CX, CY            float32    // center of mass
	Mass               float32    // total mass
	XMin, YMin         float32    // bounding box
	XMax, YMax         float32    // bounding box
	Children           [4]int32   // child indices (-1 = no child)
	BodyID             int32      // body index if leaf (-1 = internal)
}

// ComputeForcesArgs defines kernel arguments for compute_forces_kernel.
type ComputeForcesArgs struct {
	PX        driver.Ptr // offset 0
	PY        driver.Ptr // offset 8
	Mass      driver.Ptr // offset 16
	FX        driver.Ptr // offset 24
	FY        driver.Ptr // offset 32
	Tree      driver.Ptr // offset 40
	NumBodies int32      // offset 48
	Root      int32      // offset 52
	Theta     float32    // offset 56
	Eps2      float32    // offset 60
}

// UpdateBodiesArgs defines kernel arguments for update_bodies_kernel.
type UpdateBodiesArgs struct {
	PX        driver.Ptr // offset 0
	PY        driver.Ptr // offset 8
	VX        driver.Ptr // offset 16
	VY        driver.Ptr // offset 24
	FX        driver.Ptr // offset 32
	FY        driver.Ptr // offset 40
	Mass      driver.Ptr // offset 48
	NumBodies int32      // offset 56
	DT        float32    // offset 60
}

// Benchmark defines the Barnes-Hut benchmark.
type Benchmark struct {
	driver  *driver.Driver
	context *driver.Context
	gpus    []int
	queues  []*driver.CommandQueue

	computeKernel *insts.KernelCodeObject
	updateKernel  *insts.KernelCodeObject

	NumBodies int
	BlockSize int

	useUnifiedMemory bool

	hPX, hPY       []float32
	hVX, hVY       []float32
	hMass           []float32
	hFX, hFY       []float32
	hTree           []TreeNode
	treeNumNodes    int

	dPX, dPY       driver.Ptr
	dVX, dVY       driver.Ptr
	dMass           driver.Ptr
	dFX, dFY       driver.Ptr
	dTree           driver.Ptr
}

//go:embed kernels_gfx942.hsaco
var hsacoBytes []byte

// NewBenchmark creates a new Barnes-Hut benchmark.
func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.NumBodies = 16384
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
	b.computeKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "compute_forces_kernel")
	if b.computeKernel == nil {
		log.Panic("Failed to load compute_forces_kernel")
	}
	b.updateKernel = insts.LoadKernelCodeObjectFromBytes(
		hsacoBytes, "update_bodies_kernel")
	if b.updateKernel == nil {
		log.Panic("Failed to load update_bodies_kernel")
	}
}

// simpleRand implements a basic LCG to match srand(42)/rand().
type simpleRand struct {
	state uint64
}

func newSimpleRand(seed uint64) *simpleRand {
	return &simpleRand{state: seed}
}

func (r *simpleRand) next() int32 {
	r.state = r.state*1103515245 + 12345
	return int32((r.state / 65536) % 2147483648)
}

func (b *Benchmark) generateBodies() {
	n := b.NumBodies
	b.hPX = make([]float32, n)
	b.hPY = make([]float32, n)
	b.hVX = make([]float32, n)
	b.hVY = make([]float32, n)
	b.hMass = make([]float32, n)
	b.hFX = make([]float32, n)
	b.hFY = make([]float32, n)

	rng := newSimpleRand(42)
	for i := 0; i < n; i++ {
		b.hPX[i] = float32(rng.next()%10000) / 10000.0 * domainSize
		b.hPY[i] = float32(rng.next()%10000) / 10000.0 * domainSize
		b.hVX[i] = 0.0
		b.hVY[i] = 0.0
		b.hMass[i] = 1.0 + float32(rng.next()%100)/100.0
	}
}

// quadtree builder
type quadTreeBuilder struct {
	nodes    []TreeNode
	numNodes int
	maxNodes int
}

func (q *quadTreeBuilder) allocNode() int {
	if q.numNodes >= q.maxNodes {
		return -1
	}
	idx := q.numNodes
	q.numNodes++
	q.nodes[idx] = TreeNode{
		BodyID:   -1,
		Children: [4]int32{-1, -1, -1, -1},
	}
	return idx
}

func quadrant(x, y, midX, midY float32) int {
	q := 0
	if x >= midX {
		q |= 1
	}
	if y >= midY {
		q |= 2
	}
	return q
}

func childBounds(q int, xmin, ymin, xmax, ymax, midX, midY float32) (
	cxmin, cymin, cxmax, cymax float32) {
	if q&1 != 0 {
		cxmin = midX
		cxmax = xmax
	} else {
		cxmin = xmin
		cxmax = midX
	}
	if q&2 != 0 {
		cymin = midY
		cymax = ymax
	} else {
		cymin = ymin
		cymax = midY
	}
	return
}

func (q *quadTreeBuilder) insert(nodeIdx, bodyIdx int,
	bx, by, bmass, xmin, ymin, xmax, ymax float32, depth int) {
	if nodeIdx < 0 || depth > 30 {
		return
	}

	node := &q.nodes[nodeIdx]
	node.XMin, node.YMin = xmin, ymin
	node.XMax, node.YMax = xmax, ymax
	midX, midY := (xmin+xmax)*0.5, (ymin+ymax)*0.5

	if node.Mass == 0.0 && node.BodyID < 0 {
		node.CX, node.CY, node.Mass = bx, by, bmass
		node.BodyID = int32(bodyIdx)
		return
	}

	if node.BodyID >= 0 {
		q.splitLeaf(node, xmin, ymin, xmax, ymax, midX, midY, depth)
	}

	q.insertChild(node, bodyIdx, bx, by, bmass,
		xmin, ymin, xmax, ymax, midX, midY, depth)
	q.updateCenterOfMass(node)
}

func (q *quadTreeBuilder) splitLeaf(node *TreeNode,
	xmin, ymin, xmax, ymax, midX, midY float32, depth int) {
	oldBody, oldCX, oldCY, oldMass := int(node.BodyID), node.CX, node.CY, node.Mass
	node.BodyID = -1

	qi := quadrant(oldCX, oldCY, midX, midY)
	if node.Children[qi] < 0 {
		node.Children[qi] = int32(q.allocNode())
	}
	cx1, cy1, cx2, cy2 := childBounds(qi, xmin, ymin, xmax, ymax, midX, midY)
	q.insert(int(node.Children[qi]), oldBody, oldCX, oldCY, oldMass,
		cx1, cy1, cx2, cy2, depth+1)
}

func (q *quadTreeBuilder) insertChild(node *TreeNode, bodyIdx int,
	bx, by, bmass, xmin, ymin, xmax, ymax, midX, midY float32, depth int) {
	qi := quadrant(bx, by, midX, midY)
	if node.Children[qi] < 0 {
		node.Children[qi] = int32(q.allocNode())
	}
	cx1, cy1, cx2, cy2 := childBounds(qi, xmin, ymin, xmax, ymax, midX, midY)
	q.insert(int(node.Children[qi]), bodyIdx, bx, by, bmass,
		cx1, cy1, cx2, cy2, depth+1)
}

func (q *quadTreeBuilder) updateCenterOfMass(node *TreeNode) {
	totalMass, wcx, wcy := float32(0), float32(0), float32(0)
	for c := 0; c < 4; c++ {
		if ci := node.Children[c]; ci >= 0 {
			child := &q.nodes[ci]
			totalMass += child.Mass
			wcx += child.Mass * child.CX
			wcy += child.Mass * child.CY
		}
	}
	if totalMass > 0 {
		node.CX, node.CY, node.Mass = wcx/totalMass, wcy/totalMass, totalMass
	}
}

func (b *Benchmark) buildTree() int {
	maxNodes := b.NumBodies * maxNodesFactor
	b.hTree = make([]TreeNode, maxNodes)

	builder := &quadTreeBuilder{
		nodes:    b.hTree,
		numNodes: 0,
		maxNodes: maxNodes,
	}

	root := builder.allocNode()
	b.hTree[root].XMin = 0
	b.hTree[root].YMin = 0
	b.hTree[root].XMax = domainSize
	b.hTree[root].YMax = domainSize

	for i := 0; i < b.NumBodies; i++ {
		bx := b.hPX[i]
		by := b.hPY[i]
		if bx < 0.001 {
			bx = 0.001
		}
		if bx > domainSize-0.001 {
			bx = domainSize - 0.001
		}
		if by < 0.001 {
			by = 0.001
		}
		if by > domainSize-0.001 {
			by = domainSize - 0.001
		}
		builder.insert(root, i, bx, by, b.hMass[i],
			0, 0, domainSize, domainSize, 0)
	}

	b.treeNumNodes = builder.numNodes
	return root
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
	b.generateBodies()

	n := b.NumBodies
	maxNodes := n * maxNodesFactor
	bytes4 := func(count int) uint64 { return uint64(count * 4) }

	if b.useUnifiedMemory {
		b.dPX = b.driver.AllocateUnifiedMemory(b.context, bytes4(n))
		b.dPY = b.driver.AllocateUnifiedMemory(b.context, bytes4(n))
		b.dVX = b.driver.AllocateUnifiedMemory(b.context, bytes4(n))
		b.dVY = b.driver.AllocateUnifiedMemory(b.context, bytes4(n))
		b.dMass = b.driver.AllocateUnifiedMemory(b.context, bytes4(n))
		b.dFX = b.driver.AllocateUnifiedMemory(b.context, bytes4(n))
		b.dFY = b.driver.AllocateUnifiedMemory(b.context, bytes4(n))
		b.dTree = b.driver.AllocateUnifiedMemory(b.context,
			uint64(maxNodes)*48)
	} else {
		b.dPX = b.driver.AllocateMemory(b.context, bytes4(n))
		b.dPY = b.driver.AllocateMemory(b.context, bytes4(n))
		b.dVX = b.driver.AllocateMemory(b.context, bytes4(n))
		b.dVY = b.driver.AllocateMemory(b.context, bytes4(n))
		b.dMass = b.driver.AllocateMemory(b.context, bytes4(n))
		b.dFX = b.driver.AllocateMemory(b.context, bytes4(n))
		b.dFY = b.driver.AllocateMemory(b.context, bytes4(n))
		b.dTree = b.driver.AllocateMemory(b.context,
			uint64(maxNodes)*48)
	}
}

func (b *Benchmark) exec() {
	n := b.NumBodies

	b.driver.MemCopyH2D(b.context, b.dPX, b.hPX)
	b.driver.MemCopyH2D(b.context, b.dPY, b.hPY)
	b.driver.MemCopyH2D(b.context, b.dVX, b.hVX)
	b.driver.MemCopyH2D(b.context, b.dVY, b.hVY)
	b.driver.MemCopyH2D(b.context, b.dMass, b.hMass)

	// Build tree on host
	root := b.buildTree()

	// Copy tree to GPU
	b.driver.MemCopyH2D(b.context, b.dTree, b.hTree[:b.treeNumNodes])

	blockSize := b.BlockSize
	grid := (n + blockSize - 1) / blockSize
	globalSize := [3]uint32{uint32(grid * blockSize), 1, 1}
	localSize := [3]uint16{uint16(blockSize), 1, 1}

	// Compute forces
	computeArgs := ComputeForcesArgs{
		PX:        b.dPX,
		PY:        b.dPY,
		Mass:      b.dMass,
		FX:        b.dFX,
		FY:        b.dFY,
		Tree:      b.dTree,
		NumBodies: int32(n),
		Root:      int32(root),
		Theta:     defaultTheta,
		Eps2:      defaultEps2,
	}
	b.driver.LaunchKernel(b.context, b.computeKernel,
		globalSize, localSize, &computeArgs)

	// Update bodies
	updateArgs := UpdateBodiesArgs{
		PX:        b.dPX,
		PY:        b.dPY,
		VX:        b.dVX,
		VY:        b.dVY,
		FX:        b.dFX,
		FY:        b.dFY,
		Mass:      b.dMass,
		NumBodies: int32(n),
		DT:        defaultDT,
	}
	b.driver.LaunchKernel(b.context, b.updateKernel,
		globalSize, localSize, &updateArgs)

	// Read results back
	b.driver.MemCopyD2H(b.context, b.hFX, b.dFX)
	b.driver.MemCopyD2H(b.context, b.hFY, b.dFY)
	b.driver.MemCopyD2H(b.context, b.hPX, b.dPX)
	b.driver.MemCopyD2H(b.context, b.hPY, b.dPY)
}

// Verify does a basic sanity check: forces should be non-trivially nonzero.
func (b *Benchmark) Verify() {
	n := b.NumBodies
	nonzero := 0
	for i := 0; i < n; i++ {
		if b.hFX[i] != 0 || b.hFY[i] != 0 {
			nonzero++
		}
	}

	if nonzero == 0 {
		log.Panicf("FAIL: all forces are zero")
	}

	ratio := float64(nonzero) / float64(n)
	if ratio < 0.5 {
		log.Panicf("FAIL: only %d/%d bodies have nonzero forces (%.1f%%)",
			nonzero, n, ratio*100)
	}

	// Check for NaN/Inf
	for i := 0; i < n; i++ {
		if math.IsNaN(float64(b.hFX[i])) || math.IsInf(float64(b.hFX[i]), 0) {
			log.Panicf("FAIL: NaN/Inf in FX[%d]", i)
		}
	}

	log.Printf("Passed! (%d/%d bodies with nonzero forces)", nonzero, n)
}
