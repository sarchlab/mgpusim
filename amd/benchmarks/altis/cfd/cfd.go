// Package cfd runs the v1.1 Altis Euler solver on a tetrahedral mesh.
package cfd

import (
	_ "embed"
	"encoding/binary"
	"fmt"
	"log"
	"math"

	"github.com/sarchlab/mgpusim/v5/amd/arch"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

const timesteps = 10
const rkStages = 3

type copyArgs struct {
	Cells      int32
	_          uint32
	State, Old gpuruntime.Ptr
	Launch     benchmarks.GFX942LaunchDimensions
}
type factorArgs struct {
	Cells                int32
	_                    uint32
	State, Areas, Factor gpuruntime.Ptr
	Launch               benchmarks.GFX942LaunchDimensions
}
type fluxArgs struct {
	Cells                           int32
	_                               uint32
	Neighbors, Normals, State, Flux gpuruntime.Ptr
	Launch                          benchmarks.GFX942LaunchDimensions
}
type updateArgs struct {
	Stage, Cells             int32
	Old, State, Factor, Flux gpuruntime.Ptr
	Launch                   benchmarks.GFX942LaunchDimensions
}

// Benchmark owns the four ordered GPU stages and the input mesh.
type Benchmark struct {
	*State
	benchmarks.Checkpoint[State]
	rt                         *gpuruntime.Runtime
	copy, factor, flux, update *insts.KernelCodeObject
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

func NewBenchmark(rt *gpuruntime.Runtime) *Benchmark {
	b := &Benchmark{State: &State{}, rt: rt}
	b.Checkpoint = benchmarks.NewCheckpoint(b.State, b.bind)
	b.Context = rt.Init()
	return b
}

var gammaMinusOne = float32(1.4) - float32(1)

func farfield() [5]float32 {
	const speed float32 = 1.2
	density := float32(1.4)
	return [5]float32{density, density * speed, 0, 0, density*(0.5*(speed*speed)) + 1/gammaMinusOne}
}

func encodeFloats(values ...float32) []byte {
	data := make([]byte, binary.Size(values))
	if _, err := binary.Encode(data, binary.LittleEndian, values); err != nil {
		panic(err)
	}
	return data
}

func (b *Benchmark) loadProgram() {
	b.copy = insts.LoadKernelCodeObjectFromBytes(cdna3HSACOBytes, "copy_variables_kernel")
	b.factor = insts.LoadKernelCodeObjectFromBytes(cdna3HSACOBytes, "compute_step_factor_kernel")
	b.update = insts.LoadKernelCodeObjectFromBytes(cdna3HSACOBytes, "rk_update_kernel")
	ff := farfield()
	speed := float32(1.2)
	y, z := encodeFloats(0, 1, 0), encodeFloats(0, 0, 1)
	farMomentum := encodeFloats(float32(speed*ff[1])+1, 0, 0)
	farEnergy := encodeFloats(speed*float32(ff[4]+1), 0, 0)
	b.flux = insts.LoadGFX942KernelWithReadOnlyData(cdna3HSACOBytes, "compute_flux_kernel",
		insts.ReadOnlyConstant{Name: "_ZN10cfd_device11ff_variableE", Data: encodeFloats(ff[:]...)},
		insts.ReadOnlyConstant{Name: "_ZN10cfd_device31ff_flux_contribution_momentum_xE", Data: farMomentum},
		insts.ReadOnlyConstant{Name: "_ZN10cfd_device31ff_flux_contribution_momentum_yE", Data: y},
		insts.ReadOnlyConstant{Name: "_ZN10cfd_device31ff_flux_contribution_momentum_zE", Data: z},
		insts.ReadOnlyConstant{Name: "_ZN10cfd_device35ff_flux_contribution_density_energyE", Data: farEnergy})
	if b.copy == nil || b.factor == nil || b.update == nil || b.flux == nil {
		panic("could not load Altis CFD kernels")
	}
}

func (b *Benchmark) SelectGPU(gpus []int) { b.Gpus = gpus }
func (b *Benchmark) SetUnifiedMemory()    { b.UseUnifiedMemory = true }

func (b *Benchmark) setup() {
	if b.Arch != arch.CDNA3 {
		panic("Altis CFD requires CDNA3")
	}
	if b.N == 0 {
		b.N = 256
	}
	if b.BlockSize == 0 {
		b.BlockSize = 256
	}
	if b.N < 1 || b.BlockSize < 1 || b.BlockSize > 1024 || b.N > math.MaxInt32/12 {
		panic("CFD grid or block size exceeds the v1.1 limits")
	}
	b.Padded = (b.N + b.BlockSize - 1) / b.BlockSize * b.BlockSize
	if b.Padded > math.MaxInt32/12 {
		panic("CFD padded normal indexing exceeds signed 32-bit range")
	}
	b.loadProgram()
	b.rt.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.rt.CreateCommandQueue(b.Context)
	b.meshInput()
	n := b.Padded
	alloc := b.rt.AllocateMemory
	if b.UseUnifiedMemory {
		alloc = b.rt.AllocateUnifiedMemory
	}
	b.GState = alloc(b.Context, uint64(5*n*4))
	b.GOld = alloc(b.Context, uint64(5*n*4))
	b.GFlux = alloc(b.Context, uint64(5*n*4))
	b.GFactor = alloc(b.Context, uint64(n*4))
	b.GAreas = alloc(b.Context, uint64(n*4))
	b.GNormals = alloc(b.Context, uint64(12*n*4))
	b.GNeighbors = alloc(b.Context, uint64(4*n*4))
	b.Upload(b.rt, b.Queue, b.GState, b.Initial)
	b.Upload(b.rt, b.Queue, b.GAreas, b.Areas)
	b.Upload(b.rt, b.Queue, b.GNormals, b.Normals)
	b.Upload(b.rt, b.Queue, b.GNeighbors, b.Neighbors)
}

func (b *Benchmark) exec() {
	n := int32(b.Padded)
	grid, group := [3]uint32{uint32(n), 1, 1}, [3]uint16{uint16(b.BlockSize), 1, 1}
	launch := benchmarks.NewGFX942LaunchDimensions(grid, group)
	copyFields := copyArgs{Cells: n, State: b.GState, Old: b.GOld, Launch: launch}
	factor := factorArgs{Cells: n, State: b.GState, Areas: b.GAreas, Factor: b.GFactor, Launch: launch}
	flux := fluxArgs{
		Cells: n, Neighbors: b.GNeighbors, Normals: b.GNormals,
		State: b.GState, Flux: b.GFlux, Launch: launch,
	}
	update := updateArgs{Cells: n, Old: b.GOld, State: b.GState, Factor: b.GFactor, Flux: b.GFlux, Launch: launch}
	for t := 0; t < timesteps; t++ {
		b.rt.EnqueueLaunchKernel(b.Queue, b.copy, grid, group, &copyFields)
		b.rt.EnqueueLaunchKernel(b.Queue, b.factor, grid, group, &factor)
		for stage := int32(0); stage < rkStages; stage++ {
			update.Stage = stage
			b.rt.EnqueueLaunchKernel(b.Queue, b.flux, grid, group, &flux)
			b.rt.EnqueueLaunchKernel(b.Queue, b.update, grid, group, &update)
		}
	}
}

func (b *Benchmark) verify() error {
	expected := b.reference()
	labels := []string{"state", "old", "flux", "factor"}
	for phase, reference := range expected {
		values := make([]float32, len(reference))
		if err := benchmarks.Read(b.rt, b.Readbacks[phase], values); err != nil {
			return err
		}
		for i, value := range values {
			got, want := float64(value), reference[i]
			if math.IsNaN(got) || math.IsInf(got, 0) || math.IsNaN(want) || math.IsInf(want, 0) ||
				math.Abs(got-want) > 5e-6+1e-5*math.Abs(want) {
				return fmt.Errorf("%s[%d]: got %g, expected %g", labels[phase], i, got, want)
			}
		}
	}
	log.Printf("Passed!\n")
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
		b.exec()
	case 1:
		if !b.Verification {
			b.Ended = true
			return nil
		}
		for i, ptr := range []gpuruntime.Ptr{b.GState, b.GOld, b.GFlux, b.GFactor} {
			count := b.Padded
			if i < 3 {
				count *= 5
			}
			b.Readbacks = append(b.Readbacks, benchmarks.Download(b.rt, b.Queue, ptr, count*4))
		}
	case 2:
		if err := b.verify(); err != nil {
			return err
		}
		b.Readbacks = nil
		b.Ended = true
	}
	b.Phase++
	return nil
}
