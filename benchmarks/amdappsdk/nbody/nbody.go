// Package nbody include the benchmark of NBody sample Derived from SDKSample base class
package nbody

import (
	"log"
	"math"
	"math/rand"

	"gitlab.com/akita/mgpusim/driver"
	"gitlab.com/akita/mgpusim/insts"
	"gitlab.com/akita/mgpusim/kernels"
)

type NBodyKernelArgs struct {
	Pos                 driver.GPUPtr
	Vel                 driver.GPUPtr
	NumBodies           int32
	DeltaTime           float32
	EpsSqr              float32
	LocalPos            driver.LocalPtr
	NewPosition         driver.GPUPtr
	NewVelocity         driver.GPUPtr
	HiddenGlobalOffsetX int64
	HiddenGlobalOffsetY int64
	HiddenGlobalOffsetZ int64
}

type Benchmark struct {
	driver           *driver.Driver
	context          *driver.Context
	gpus             []int
	queues           []*driver.CommandQueue
	useUnifiedMemory bool
	nbodyKernel      *insts.HsaCo
	NumParticles     int32
	delT             float32   // dT (timestep)
	espSqr           float32   // Softening Factor
	initPos          []float32 // initial position
	initVel          []float32 // initial velocity
	pos              []float32 // Output position
	vel              []float32 // Output velocity
	refPos           []float32 // Reference position
	refVel           []float32 // Reference velocity
	groupSize        int32     // Work-Group size
	NumIterations    int32
	isFirstLuanch    bool
	exchange         bool
	numBodies        int32
	currPos          driver.GPUPtr
	currVel          driver.GPUPtr
	newPos           driver.GPUPtr
	newVel           driver.GPUPtr
	dPos             *driver.GPUPtr
	dVel             *driver.GPUPtr
	dNewPos          *driver.GPUPtr
	dNewVel          *driver.GPUPtr
}

func NewBenchmark(driver *driver.Driver) *Benchmark {
	b := new(Benchmark)
	b.driver = driver
	b.context = driver.Init()
	b.loadProgram()
	b.groupSize = 64
	b.delT = 0.005
	b.espSqr = 500.0
	b.isFirstLuanch = true
	b.exchange = true
	b.NumIterations = 1
	b.NumParticles = 64

	if b.NumParticles < b.groupSize {
		b.NumParticles = b.groupSize
	}

	b.NumParticles = (b.NumParticles / b.groupSize) * b.groupSize
	b.numBodies = b.NumParticles

	return b
}

func (b *Benchmark) SelectGPU(gpus []int) {
	b.gpus = gpus
}

func (b *Benchmark) SetUnifiedMemory() {
	b.useUnifiedMemory = true
}

func (b *Benchmark) loadProgram() {
	hsacoBytes := _escFSMustByte(false, "/nbody.hsaco")

	b.nbodyKernel = kernels.LoadProgramFromMemory(hsacoBytes, "nbody_sim")
	if b.nbodyKernel == nil {
		log.Panic("Failed to load kernel binary")
	}
}

func (b *Benchmark) Run() {
	for _, gpu := range b.gpus {
		b.driver.SelectGPU(b.context, gpu)
		b.queues = append(b.queues, b.driver.CreateCommandQueue(b.context))
	}
	b.initMem()
	b.exec()
}

func (b *Benchmark) initMem() {
	b.initPos = make([]float32, b.numBodies*4)
	b.initVel = make([]float32, b.numBodies*4)
	b.pos = make([]float32, b.numBodies*4) // Should be aligned to 16
	b.vel = make([]float32, b.numBodies*4) // Should be aligned to 16

	b.fill()

	if b.useUnifiedMemory {
		b.currPos = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.numBodies*4*4))
		b.newPos = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.numBodies*4*4))
		b.currVel = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.numBodies*4*4))
		b.newVel = b.driver.AllocateUnifiedMemory(b.context,
			uint64(b.numBodies*4*4))
	} else {
		b.currPos = b.driver.AllocateMemory(b.context,
			uint64(b.numBodies*4*4))
		b.newPos = b.driver.AllocateMemory(b.context,
			uint64(b.numBodies*4*4))
		b.currVel = b.driver.AllocateMemory(b.context,
			uint64(b.numBodies*4*4))
		b.newVel = b.driver.AllocateMemory(b.context,
			uint64(b.numBodies*4*4))
	}
	b.driver.MemCopyH2D(b.context, b.currPos, b.pos)
	b.driver.MemCopyH2D(b.context, b.currVel, b.vel)

	b.dPos = &b.currPos
	b.dVel = &b.currVel
	b.dNewPos = &b.newPos
	b.dNewVel = &b.newVel
}

func (b *Benchmark) exec() {
	globalSize := [3]uint32{uint32(b.numBodies), 1, 1}
	localSize := [3]uint16{uint16(b.groupSize), 1, 1}

	for i := int32(0); i < b.NumIterations; i++ {
		args := NBodyKernelArgs{
			Pos:                 *b.dPos,
			Vel:                 *b.dVel,
			NumBodies:           b.numBodies,
			DeltaTime:           b.delT,
			EpsSqr:              b.espSqr,
			LocalPos:            64 * 4 * 4,
			NewPosition:         *b.dNewPos,
			NewVelocity:         *b.dNewVel,
			HiddenGlobalOffsetX: 0,
			HiddenGlobalOffsetY: 0,
			HiddenGlobalOffsetZ: 0,
		}

		b.driver.LaunchKernel(b.context,
			b.nbodyKernel,
			globalSize, localSize,
			&args,
		)

		b.dPos, b.dNewPos = b.dNewPos, b.dPos
		b.dVel, b.dNewVel = b.dNewVel, b.dVel
	}

	b.driver.MemCopyD2H(b.context, b.pos, *b.dPos)
}

func (b *Benchmark) Verify() {
	b.refPos = make([]float32, b.numBodies*4)
	b.refVel = make([]float32, b.numBodies*4)
	copy(b.refPos, b.initPos)
	copy(b.refVel, b.initVel)

	for i := int32(0); i < b.NumIterations; i++ {
		b.nbodyCPU()
	}

	mismatch := false
	for i := int32(0); i < (b.numBodies * 4); i++ {
		if b.refPos[i] != b.pos[i] {
			mismatch = true
			log.Printf("not match at (%d), expected %g to equal %g\n",
				i,
				b.pos[i], b.refPos[i])
		}
	}

	if mismatch {
		panic("Mismatch!\n")
	}
	log.Printf("Passed!\n")
}

func (b *Benchmark) nbodyCPU() {
	for i := int32(0); i < b.numBodies; i++ {
		myIndex := int32(4 * i)
		acc := [3]float32{0.0, 0.0, 0.0}
		for j := int32(0); j < b.numBodies; j++ {
			r := [3]float32{0.0, 0.0, 0.0}
			index := int32(4 * j)

			distSqr := float32(0.0)
			for k := int32(0); k < 3; k++ {
				r[k] = b.refPos[index+k] - b.refPos[myIndex+k]
				distSqr += r[k] * r[k]
			}

			invDist := 1.0 / float32(math.Sqrt(float64(distSqr+b.espSqr)))
			invDistCube := invDist * invDist * invDist
			s := b.refPos[index+3] * invDistCube

			for k := int32(0); k < 3; k++ {
				acc[k] += s * r[k]
			}
		}

		for k := int32(0); k < 3; k++ {
			b.refPos[myIndex+k] += b.refVel[myIndex+k]*b.delT + 0.5*acc[k]*b.delT*b.delT
			b.refVel[myIndex+k] += acc[k] * b.delT
		}
	}
}

func random(randMax float32, randMin float32) float32 {
	result := rand.Float32()
	result = ((1.0-result)*randMin + result*randMax)
	return result
}

func (b *Benchmark) fill() {
	for i := int32(0); i < b.numBodies; i++ {
		index := int32(4 * i)

		for j := int32(0); j < 3; j++ {
			// b.initPos[index+j] = random(3, 50)
			b.initPos[index+j] = 1.0
		}
		// b.initPos[index+3] = random(1, 1000)
		b.initPos[index+3] = 1.0

		for j := int32(0); j < 3; j++ {
			b.initVel[index+j] = 0.0
		}

		b.initVel[3] = 0.0 // unused
	}

	copy(b.pos, b.initPos)
	copy(b.vel, b.initVel)
}
