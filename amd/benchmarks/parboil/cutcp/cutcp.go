// Package cutcp implements the Parboil v1.1 binned cutoff potential benchmark.
package cutcp

import (
	// embed hsaco files
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

// binDepth is the maximum number of atoms stored per spatial bin.
const binDepth = 16
const regionSide = 8
const regionPoints = 512
const binLength = 4

type Atom struct{ X, Y, Z, Charge float32 }
type BinOffset struct{ X, Y, Z int32 }

type KernelArgs struct {
	BinsX, BinsY                 int32
	Bins                         gpuruntime.Ptr
	Spacing, Cutoff2, InvCutoff2 float32
	_                            uint32
	Regions                      gpuruntime.Ptr
	Slab                         int32
	_                            uint32
	Launch                       benchmarks.GFX942LaunchDimensions
}

// Benchmark runs the v1.1 spatially binned potential calculation.
type Benchmark struct {
	*State
	benchmarks.Checkpoint[State]
	rt     *gpuruntime.Runtime
	kernel *insts.KernelCodeObject
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

func NewBenchmark(rt *gpuruntime.Runtime) *Benchmark {
	b := &Benchmark{State: &State{}, rt: rt}
	b.Checkpoint = benchmarks.NewCheckpoint(b.State, b.bind)
	b.Context = rt.Init()
	return b
}

func (b *Benchmark) loadProgram() {
	if len(b.Neighbors) == 0 {
		return
	}
	offsets := make([]byte, binary.Size(b.Neighbors))
	if _, err := binary.Encode(offsets, binary.LittleEndian, b.Neighbors); err != nil {
		panic(err)
	}
	count := binary.LittleEndian.AppendUint32(nil, uint32(len(b.Neighbors)))
	b.kernel = insts.LoadGFX942KernelWithReadOnlyData(cdna3HSACOBytes,
		"_ZL37cuda_cutoff_potential_lattice6overlapiiP15HIP_vector_typeIfLj4EEfffPfi",
		insts.ReadOnlyConstant{Name: "NbrListLen", Data: count}, insts.ReadOnlyConstant{Name: "NbrList", Data: offsets})
}

func (b *Benchmark) SelectGPU(gpus []int) { b.Gpus = gpus }
func (b *Benchmark) SetUnifiedMemory()    { b.UseUnifiedMemory = true }

func ceil32(x float32) int { return int(math.Ceil(float64(x))) }

func (b *Benchmark) configure() int {
	if b.Arch != arch.CDNA3 {
		panic("CutCP requires CDNA3")
	}
	if b.NumAtoms <= 0 {
		b.NumAtoms = 64
	}
	if b.GridSide <= 0 {
		b.GridSide = 16
	}
	if b.GridSpacing <= 0 {
		b.GridSpacing = 0.5
	}
	if b.Cutoff <= 0 {
		b.Cutoff = 12
	}
	b.SideRegions = (b.GridSide + regionSide - 1) / regionSide
	padding := ceil32(b.Cutoff / binLength)
	ratio := binLength / (regionSide * b.GridSpacing)
	factor := float32(math.Sqrt(3))
	switch {
	case math.Floor(float64(ratio)) == math.Ceil(float64(ratio)):
		if 2*padding+1 > 11 {
			panic("CutCP cutoff exceeds aligned neighbor-list capacity")
		}
	case regionSide*b.GridSpacing <= 2*binLength:
		factor *= 2
		// An unaligned region can reach one bin farther from its center bin.
		padding++
		if 2*padding+1 > 11 {
			panic("CutCP cutoff exceeds neighbor-list capacity")
		}
	default:
		panic("CutCP grid spacing exceeds one Angstrom")
	}
	limit := b.Cutoff + factor*binLength
	b.Neighbors = nil
	for z := -padding; z <= padding; z++ {
		for y := -padding; y <= padding; y++ {
			for x := -padding; x <= padding; x++ {
				if float32(x*x+y*y+z*z)*binLength*binLength < limit*limit {
					b.Neighbors = append(b.Neighbors, BinOffset{int32(x), int32(y), int32(z)})
				}
			}
		}
	}
	b.BinSide = ceil32(float32(regionSide*b.SideRegions)*b.GridSpacing/binLength) + 2*padding
	b.BinOrigin = ((padding*b.BinSide+padding)*b.BinSide + padding) * binDepth

	return padding
}

func (b *Benchmark) binAtoms(padding int) []Atom {
	size := b.BinSide
	bins := make([]Atom, size*size*size*binDepth)
	counts := make([]int, size*size*size)
	var seed uint32 = 42
	draw := func(lo, hi float32) float32 {
		seed = seed*1664525 + 1013904223
		// Match the HIP host's separate float32 multiply and add.
		return lo + float32(float32(seed&0xffff)/65535*(hi-lo))
	}
	domain := float32(b.GridSide) * b.GridSpacing
	for i := 0; i < b.NumAtoms; i++ {
		atom := Atom{draw(0, domain), draw(0, domain), draw(0, domain), draw(0.1, 1)}
		x, y, z := int(atom.X/binLength)+padding, int(atom.Y/binLength)+padding, int(atom.Z/binLength)+padding
		if x < 0 || x >= size || y < 0 || y >= size || z < 0 || z >= size || atom.Charge == 0 {
			continue
		}
		index := (z*size+y)*size + x
		if counts[index] >= binDepth {
			panic("CutCP atom bin exceeds the fixed 16-atom capacity")
		}
		bins[index*binDepth+counts[index]] = atom
		counts[index]++
	}
	b.Atoms = make([]Atom, 0, b.NumAtoms)
	for _, atom := range bins {
		if atom.Charge != 0 {
			b.Atoms = append(b.Atoms, atom)
		}
	}
	return bins
}

func (b *Benchmark) submit() {
	padding := b.configure()
	b.loadProgram()
	b.rt.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.rt.CreateCommandQueue(b.Context)
	bins := b.binAtoms(padding)
	alloc := b.rt.AllocateMemory
	if b.UseUnifiedMemory {
		alloc = b.rt.AllocateUnifiedMemory
	}
	b.GAtoms = alloc(b.Context, uint64(len(bins))*16)
	b.GPot = alloc(b.Context, uint64(b.regionSize())*4)
	b.Upload(b.rt, b.Queue, b.GAtoms, bins)
	b.Upload(b.rt, b.Queue, b.GPot, make([]float32, b.regionSize()))
	b.exec()
}

func (b *Benchmark) regionSize() int {
	return b.SideRegions * b.SideRegions * b.SideRegions * regionPoints
}

func (b *Benchmark) exec() {
	group := [3]uint16{8, 2, 8}
	grid := [3]uint32{uint32(b.SideRegions) * 8, uint32(b.SideRegions) * 2, 8}
	launch := benchmarks.NewGFX942LaunchDimensions(grid, group)
	cutoff2 := b.Cutoff * b.Cutoff
	for slab := 0; slab < b.SideRegions; slab++ {
		args := KernelArgs{BinsX: int32(b.BinSide), BinsY: int32(b.BinSide),
			Bins: b.GAtoms + gpuruntime.Ptr(b.BinOrigin*16), Spacing: b.GridSpacing, Cutoff2: cutoff2, InvCutoff2: 1 / cutoff2,
			Regions: b.GPot, Slab: int32(slab), Launch: launch}
		b.rt.EnqueueLaunchKernel(b.Queue, b.kernel, grid, group, &args)
	}
}

func bounds(coordinate, cutoff, spacing float32, n int) (int, int) {
	x, c, h := float64(coordinate), float64(cutoff), float64(spacing)
	return max(0, int(math.Floor((x-c)/h))-1), min(n-1, int(math.Ceil((x+c)/h))+1)
}

func (b *Benchmark) reference() []float32 {
	n, h := b.GridSide, b.GridSpacing
	cutoff2 := b.Cutoff * b.Cutoff
	inverse := 1 / cutoff2
	reference := make([]float32, n*n*n)
	for _, atom := range b.Atoms {
		x0, x1 := bounds(atom.X, b.Cutoff, h, n)
		y0, y1 := bounds(atom.Y, b.Cutoff, h, n)
		z0, z1 := bounds(atom.Z, b.Cutoff, h, n)
		for z := z0; z <= z1; z++ {
			for y := y0; y <= y1; y++ {
				for x := x0; x <= x1; x++ {
					dx, dy, dz := atom.X-float32(x)*h, atom.Y-float32(y)*h, atom.Z-float32(z)*h
					r2 := float32(dx*dx) + float32(dy*dy) + float32(dz*dz)
					if r2 < cutoff2 {
						s := 1 - r2*inverse
						i := (z*n+y)*n + x
						reference[i] += atom.Charge * (1 / float32(math.Sqrt(float64(r2)))) * s * s
					}
				}
			}
		}
	}
	return reference
}

func (b *Benchmark) verify() error {
	actual := make([]float32, b.regionSize())
	if err := benchmarks.Read(b.rt, b.Readbacks[0], actual); err != nil {
		return err
	}
	ref := b.reference()
	n, r := b.GridSide, b.SideRegions
	for i, want := range ref {
		z, y, x := i/(n*n), i/n%n, i%n
		region := ((z/8*r+y/8)*r + x/8) * regionPoints
		local := (z%8*8+y%8)*8 + x%8
		got := float64(actual[region+local])
		tol := 1e-3 * (math.Abs(float64(want)) + 1e-4)
		if math.IsNaN(got) || math.IsInf(got, 0) || math.IsNaN(float64(want)) ||
			math.IsInf(float64(want), 0) || math.Abs(got-float64(want)) > tol {
			return fmt.Errorf("potential at (%d,%d,%d): got %g, expected %g", x, y, z, got, want)
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
		b.submit()
	case 1:
		if !b.Verification {
			b.Ended = true
			return nil
		}
		b.Readbacks = append(b.Readbacks, benchmarks.Download(b.rt, b.Queue, b.GPot, b.regionSize()*4))
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
