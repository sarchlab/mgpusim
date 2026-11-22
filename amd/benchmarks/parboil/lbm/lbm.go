// Package lbm implements the Parboil v1.1 D3Q19 collision and scatter benchmark.
package lbm

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

// Q is the number of discrete velocities in the D3Q19 lattice.
const populations = 19
const omega float32 = 1.95

const (
	center = iota
	north
	south
	east
	west
	top
	bottom
	northEast
	northWest
	southEast
	southWest
	northTop
	northBottom
	southTop
	southBottom
	eastTop
	eastBottom
	westTop
	westBottom
	flagsSlot
)

var direction = [populations][3]int{
	{0, 0, 0}, {0, 1, 0}, {0, -1, 0}, {1, 0, 0}, {-1, 0, 0}, {0, 0, 1}, {0, 0, -1},
	{1, 1, 0}, {-1, 1, 0}, {1, -1, 0}, {-1, -1, 0}, {0, 1, 1}, {0, 1, -1}, {0, -1, 1}, {0, -1, -1},
	{1, 0, 1}, {1, 0, -1}, {-1, 0, 1}, {-1, 0, -1},
}

var opposite = [populations]int{center, south, north, west, east, bottom, top,
	southWest, southEast, northWest, northEast, southBottom, southTop, northBottom, northTop,
	westBottom, westTop, eastBottom, eastTop}

type Layout struct{ N, PX, Plane, Cells, Margin, Entries int32 }

func newLayout(n int) Layout {
	px := n + 8
	plane := px * n
	cells := plane * (n + 4)
	margin := 2 * plane
	return Layout{int32(n), int32(px), int32(plane), int32(cells), int32(margin), int32(20*cells + 40*margin)}
}

func (l Layout) cell(x, y, z int) int {
	return int(l.Margin) + x + y*int(l.PX) + z*int(l.Plane)
}

func (l Layout) deviceIndex(x, y, z, q int) int {
	return l.cell(x, y, z) + q*int(l.Cells)
}

func flags(x, y, z, n int) uint32 {
	if x == 0 || y == 0 || z == 0 || x == n-1 || y == n-1 || z == n-1 {
		return 1
	}
	if (z == 1 || z == n-2) && x > 1 && x < n-2 && y > 1 && y < n-2 {
		return 2
	}
	return 0
}

func weight(q int) float64 {
	switch {
	case q == 0:
		return 1.0 / 3.0
	case q <= bottom:
		return 1.0 / 18.0
	default:
		return 1.0 / 36.0
	}
}

type kernelArgs struct {
	Src, Dst gpuruntime.Ptr
	Layout   Layout
}

// Benchmark runs the upstream padded/scatter LBM kernel.
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
	b.kernel = insts.LoadKernelCodeObjectFromBytes(cdna3HSACOBytes, "lbm_collide_stream_kernel")
	if b.kernel == nil {
		panic("could not load Parboil LBM kernel")
	}
}

func (b *Benchmark) SelectGPU(gpus []int) { b.Gpus = gpus }
func (b *Benchmark) SetUnifiedMemory()    { b.UseUnifiedMemory = true }

func (b *Benchmark) setup() {
	if b.Arch != arch.CDNA3 {
		panic("Parboil LBM requires CDNA3")
	}
	if b.GridDim == 0 {
		b.GridDim = 64
	}
	if b.NumTimesteps == 0 {
		b.NumTimesteps = 100
	}
	if b.Tau != 0 && b.Tau != 1.0/omega {
		panic("Parboil v1.1 fixes omega at 1.95")
	}
	n := b.GridDim
	if n < 3 || n > 483 || 20*int64(n+8)*int64(n)*int64(n+8) > math.MaxInt32 || b.NumTimesteps < 1 {
		panic("unsupported LBM grid or timestep count")
	}
	if n == 256 && b.NumTimesteps == 500 {
		panic("Parboil v1.1 excludes grid 256 with 500 steps")
	}
	b.loadProgram()
	b.rt.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.rt.CreateCommandQueue(b.Context)
	b.Layout = newLayout(n)
	layout := b.Layout
	initial := make([]float32, layout.Entries)
	for z := -2; z < n+2; z++ {
		for y := 0; y < n; y++ {
			for x := 0; x < n; x++ {
				initial[layout.deviceIndex(x, y, z, flagsSlot)] = math.Float32frombits(flags(x, y, z, n))
				if z >= 0 && z < n {
					for q := 0; q < populations; q++ {
						initial[layout.deviceIndex(x, y, z, q)] = float32(weight(q))
					}
				}
			}
		}
	}
	alloc := b.rt.AllocateMemory
	if b.UseUnifiedMemory {
		alloc = b.rt.AllocateUnifiedMemory
	}
	b.GFSrc = alloc(b.Context, uint64(layout.Entries)*4)
	b.GFDst = alloc(b.Context, uint64(layout.Entries)*4)
	b.Upload(b.rt, b.Queue, b.GFSrc, initial)
	b.Upload(b.rt, b.Queue, b.GFDst, initial)
}

func (b *Benchmark) exec() {
	n := uint32(b.GridDim)
	grid, group := [3]uint32{n * n, n, 1}, [3]uint16{uint16(n), 1, 1}
	margin := gpuruntime.Ptr(b.Layout.Margin * 4)
	src, dst := b.GFSrc, b.GFDst
	for t := 0; t < b.NumTimesteps; t++ {
		args := kernelArgs{Src: src + margin, Dst: dst + margin, Layout: b.Layout}
		b.rt.EnqueueLaunchKernel(b.Queue, b.kernel, grid, group, &args)
		src, dst = dst, src
	}
	b.Final = src
}

func velocity(f []float32) (rho, u, v, w float32) {
	for _, x := range f {
		rho += x
	}
	u = (f[east] - f[west] + f[northEast] - f[northWest] + f[southEast] - f[southWest] +
		f[eastTop] + f[eastBottom] - f[westTop] - f[westBottom]) / rho
	v = (f[north] - f[south] + f[northEast] + f[northWest] - f[southEast] - f[southWest] +
		f[northTop] + f[northBottom] - f[southTop] - f[southBottom]) / rho
	w = (f[top] - f[bottom] + f[northTop] - f[northBottom] + f[southTop] - f[southBottom] +
		f[eastTop] - f[eastBottom] + f[westTop] - f[westBottom]) / rho
	return
}

func (b *Benchmark) cpuReference() []float32 {
	layout := b.Layout
	n := b.GridDim
	states := [2][]float32{make([]float32, layout.Entries), make([]float32, layout.Entries)}
	for _, field := range states {
		for z := 0; z < n; z++ {
			for y := 0; y < n; y++ {
				for x := 0; x < n; x++ {
					base := 20 * layout.cell(x, y, z)
					for q := 0; q < populations; q++ {
						field[base+q] = float32(weight(q))
					}
				}
			}
		}
	}
	src, dst := states[0], states[1]
	for t := 0; t < b.NumTimesteps; t++ {
		for z := 0; z < n; z++ {
			for y := 0; y < n; y++ {
				for x := 0; x < n; x++ {
					b.relaxAndScatterCell(src, dst, x, y, z)
				}
			}
		}
		src, dst = dst, src
	}
	return src
}

func (b *Benchmark) relaxAndScatterCell(src, dst []float32, x, y, z int) {
	layout := b.Layout
	base := 20 * layout.cell(x, y, z)
	input := src[base : base+populations]
	rho, u, v, w := velocity(input)
	flags := flags(x, y, z, b.GridDim)
	if flags == 2 {
		u, v, w = 0.005, 0.002, 0
	}
	u2 := 1.5 * (u*u + v*v + w*w)
	for q, d := range direction {
		target := 20*layout.cell(x+d[0], y+d[1], z+d[2]) + q
		if flags == 1 {
			dst[target] = input[opposite[q]]
			continue
		}
		projection := float32(d[0])*u + float32(d[1])*v + float32(d[2])*w
		equilibrium := 1 + projection*(4.5*projection+3) - u2
		dst[target] = float32(float64((1-omega)*input[q]) + weight(q)*float64(omega)*float64(rho)*float64(equilibrium))
	}
}

func (b *Benchmark) verify() error {
	actual := make([]float32, b.Layout.Entries)
	if err := benchmarks.Read(b.rt, b.Readbacks[0], actual); err != nil {
		return err
	}
	expected := b.cpuReference()
	n := b.GridDim
	var cellActual [populations]float32
	for z := 0; z < n; z++ {
		for y := 0; y < n; y++ {
			for x := 0; x < n; x++ {
				layout := b.Layout
				for q := range cellActual {
					got := actual[layout.deviceIndex(x, y, z, q)]
					want := expected[20*layout.cell(x, y, z)+q]
					cellActual[q] = got
					if math.IsNaN(float64(got)) || math.IsInf(float64(got), 0) || math.Abs(float64(got)-float64(want)) > 1e-5 {
						return fmt.Errorf("population %d at (%d,%d,%d): got %g, expected %g", q, x, y, z, got, want)
					}
				}
				if got := math.Float32bits(actual[layout.deviceIndex(x, y, z, flagsSlot)]); got != flags(x, y, z, n) {
					return fmt.Errorf("incorrect LBM flags at (%d,%d,%d): %d", x, y, z, got)
				}
				_, u, v, w := velocity(cellActual[:])
				base := 20 * layout.cell(x, y, z)
				_, ru, rv, rw := velocity(expected[base : base+populations])
				dx, dy, dz := float64(u-ru), float64(v-rv), float64(w-rw)
				errorValue := math.Sqrt(dx*dx + dy*dy + dz*dz)
				if math.IsNaN(errorValue) || errorValue > 1e-5 {
					return fmt.Errorf("LBM velocity at (%d,%d,%d) differs by %g", x, y, z, errorValue)
				}
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
		b.Readbacks = append(b.Readbacks, benchmarks.Download(b.rt, b.Queue, b.Final, int(b.Layout.Entries)*4))
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
