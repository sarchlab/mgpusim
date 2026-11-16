// Package raytracing implements the Altis ray tracing benchmark, ported from
// sarchlab/gpu_benchmarks (tier2/altis_raytracing) for the MGPUSim MI300X
// (CDNA3 / gfx942) model.
//
// It casts one ray per pixel through an image plane and intersects each ray
// with a set of randomly-placed spheres, shading the nearest hit with Phong
// lighting. One thread handles one pixel, writing an RGBA byte per pixel into
// the output image. The kernel binary is compiled for gfx942 only (see
// native/), so the benchmark must be run with `-arch cdna3` (the MI300X
// configuration).
package raytracing

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

// blockSize is the constant work-group size baked into the kernel (the native
// kernel uses a literal BLOCK_SIZE rather than blockDim.x, so no hidden ABI
// arguments are emitted).
const blockSize = 256

// floatsPerSphere is the number of float32 values per Sphere
// (cx, cy, cz, radius, r, g, b).
const floatsPerSphere = 7

// KernelArgs defines the kernel arguments for the gfx942 (CDNA3) kernel.
//
// The layout below is verified against the compiled kernel's AMDGPU metadata
// (kernarg_segment_size = 28): two 8-byte global_buffer pointers followed by
// three 4-byte by_value int scalars, packed with no padding (mgpusim
// serializes args with binary.Write, which does not insert alignment padding).
// The kernel uses a constant block size and reads only blockIdx/threadIdx, so
// no hidden ABI arguments are emitted.
type KernelArgs struct {
	Image      gpuruntime.Ptr // offset 0
	Spheres    gpuruntime.Ptr // offset 8
	Width      int32          // offset 16
	Height     int32          // offset 20
	NumSpheres int32          // offset 24
}

// Benchmark defines the ray tracing benchmark.
type Benchmark struct {
	*State
	benchmarks.Checkpoint[State]
	rt *gpuruntime.Runtime

	hsaco *insts.KernelCodeObject
}

//go:embed kernels_gfx942.hsaco
var cdna3HSACOBytes []byte

// NewBenchmark returns a new ray tracing benchmark.
func NewBenchmark(rt *gpuruntime.Runtime) *Benchmark {
	b := new(Benchmark)
	b.State = &State{}
	b.Checkpoint = benchmarks.NewCheckpoint(b.State, b.bind)

	b.rt = rt
	b.Context = b.rt.Init()

	return b
}

func (b *Benchmark) loadProgram() {
	b.hsaco = insts.LoadKernelCodeObjectFromBytes(
		cdna3HSACOBytes, "raytrace_kernel")
	if b.hsaco == nil {
		log.Panic("Failed to load kernel binary")
	}
}

// SelectGPU selects the GPUs to run on. Ray tracing uses a single GPU.
func (b *Benchmark) SelectGPU(gpus []int) {
	b.Gpus = gpus
}

// SetUnifiedMemory requests the use of unified memory.
func (b *Benchmark) SetUnifiedMemory() {
	b.UseUnifiedMemory = true
}

// Prepare the inputs for the first queued phase.
func (b *Benchmark) submit() {
	if b.Arch != arch.CDNA3 {
		log.Panic("the altis raytracing benchmark ships only a gfx942 " +
			"kernel; run with -arch cdna3 -gpu mi300x")
	}

	b.loadProgram()

	b.rt.SelectGPU(b.Context, b.Gpus[0])
	b.Queue = b.rt.CreateCommandQueue(b.Context)
	b.initMem()
	b.exec()
}

// genSpheres builds the deterministic sphere set using the same LCG as the
// original benchmark (seed 42).
func (b *Benchmark) genSpheres() {
	n := b.Spheres
	b.SphereData = make([]float32, n*floatsPerSphere)

	seed := uint32(42)
	next := func() float32 {
		seed = seed*1103515245 + 12345
		return float32(seed%10000) / 10000.0
	}

	for i := 0; i < n; i++ {
		base := i * floatsPerSphere
		b.SphereData[base+0] = next()*6.0 - 3.0 // cx
		b.SphereData[base+1] = next()*6.0 - 3.0 // cy
		b.SphereData[base+2] = next()*4.0 - 4.0 // cz
		b.SphereData[base+3] = next()*0.5 + 0.2 // radius
		b.SphereData[base+4] = next()*0.8 + 0.2 // r
		b.SphereData[base+5] = next()*0.8 + 0.2 // g
		b.SphereData[base+6] = next()*0.8 + 0.2 // b
	}
}

func (b *Benchmark) initMem() {
	if b.Width <= 0 {
		b.Width = 64
	}
	if b.Height <= 0 {
		b.Height = 64
	}
	if b.Spheres <= 0 {
		b.Spheres = 16
	}

	b.genSpheres()

	imageBytes := uint64(b.Width * b.Height * 4)
	sphereBytes := uint64(b.Spheres * floatsPerSphere * 4)

	if b.UseUnifiedMemory {
		b.GImage = b.rt.AllocateUnifiedMemory(b.Context, imageBytes)
		b.GSpheres = b.rt.AllocateUnifiedMemory(b.Context, sphereBytes)
	} else {
		b.GImage = b.rt.AllocateMemory(b.Context, imageBytes)
		b.GSpheres = b.rt.AllocateMemory(b.Context, sphereBytes)
	}

	b.Upload(b.rt, b.Queue, b.GSpheres, b.SphereData)
}

func (b *Benchmark) exec() {
	totalPixels := b.Width * b.Height
	gridX := uint32((totalPixels + blockSize - 1) / blockSize * blockSize)

	args := KernelArgs{
		Image:      b.GImage,
		Spheres:    b.GSpheres,
		Width:      int32(b.Width),
		Height:     int32(b.Height),
		NumSpheres: int32(b.Spheres),
	}

	b.rt.EnqueueLaunchKernel(
		b.Queue,
		b.hsaco,
		[3]uint32{gridX, 1, 1},
		[3]uint16{blockSize, 1, 1},
		&args,
	)
}

// cpuShadePixel computes the RGBA bytes for a single pixel using the exact
// same float32 math as the kernel, so the result is bit-reproducible.
func (b *Benchmark) cpuShadePixel(px, py int) [4]byte { //nolint:funlen,gocognit
	width := b.Width
	height := b.Height

	aspect := float32(width) / float32(height)
	fovScale := float32(1.0)

	u := (2.0*(float32(px)+0.5)/float32(width) - 1.0) * aspect * fovScale
	v := (1.0 - 2.0*(float32(py)+0.5)/float32(height)) * fovScale

	var ox, oy, oz float32 = 0.0, 0.0, 5.0
	dx, dy, dz := u, v, float32(-1.0)

	length := f32sqrt(dx*dx + dy*dy + dz*dz)
	dx /= length
	dy /= length
	dz /= length

	closestT := float32(1e20)
	closestID := -1

	for s := 0; s < b.Spheres; s++ {
		base := s * floatsPerSphere
		cx := b.SphereData[base+0]
		cy := b.SphereData[base+1]
		cz := b.SphereData[base+2]
		radius := b.SphereData[base+3]
		t := intersectSphere(ox, oy, oz, dx, dy, dz, cx, cy, cz, radius)
		if t > 0.0 && t < closestT {
			closestT = t
			closestID = s
		}
	}

	pr, pg, pb := float32(0.05), float32(0.05), float32(0.1)

	if closestID >= 0 {
		hx := ox + closestT*dx
		hy := oy + closestT*dy
		hz := oz + closestT*dz

		base := closestID * floatsPerSphere
		spcx := b.SphereData[base+0]
		spcy := b.SphereData[base+1]
		spcz := b.SphereData[base+2]
		spradius := b.SphereData[base+3]
		spr := b.SphereData[base+4]
		spg := b.SphereData[base+5]
		spb := b.SphereData[base+6]

		nx := (hx - spcx) / spradius
		ny := (hy - spcy) / spradius
		nz := (hz - spcz) / spradius

		var lx, ly, lz float32 = 0.577, 0.577, 0.577

		ndotl := nx*lx + ny*ly + nz*lz
		if ndotl < 0.0 {
			ndotl = 0.0
		}

		rx := 2.0*ndotl*nx - lx
		ry := 2.0*ndotl*ny - ly
		rz := 2.0*ndotl*nz - lz

		vx, vy, vz := -dx, -dy, -dz
		rdotv := rx*vx + ry*vy + rz*vz
		if rdotv < 0.0 {
			rdotv = 0.0
		}
		spec := rdotv * rdotv * rdotv * rdotv
		spec = spec * spec

		ambient := float32(0.15)
		pr = spr*(ambient+0.7*ndotl) + 0.3*spec
		pg = spg*(ambient+0.7*ndotl) + 0.3*spec
		pb = spb*(ambient+0.7*ndotl) + 0.3*spec

		if pr > 1.0 {
			pr = 1.0
		}
		if pg > 1.0 {
			pg = 1.0
		}
		if pb > 1.0 {
			pb = 1.0
		}
	}

	return [4]byte{
		byte(pr * 255.0),
		byte(pg * 255.0),
		byte(pb * 255.0),
		255,
	}
}

func intersectSphere(
	ox, oy, oz, dx, dy, dz, cx, cy, cz, radius float32,
) float32 {
	ex := ox - cx
	ey := oy - cy
	ez := oz - cz

	a := dx*dx + dy*dy + dz*dz
	bb := 2.0 * (ex*dx + ey*dy + ez*dz)
	c := ex*ex + ey*ey + ez*ez - radius*radius

	disc := bb*bb - 4.0*a*c
	if disc < 0.0 {
		return -1.0
	}

	sq := f32sqrt(disc)
	t0 := (-bb - sq) / (2.0 * a)
	t1 := (-bb + sq) / (2.0 * a)

	if t0 > 0.001 {
		return t0
	}
	if t1 > 0.001 {
		return t1
	}
	return -1.0
}

func f32sqrt(x float32) float32 {
	return float32(math.Sqrt(float64(x)))
}

// Verify checks the GPU result against a CPU reference computation, comparing
// every pixel channel. A small ±1 tolerance is allowed to account for
// float-rounding differences between the GPU sqrt and the host sqrt.
func (b *Benchmark) verify() error {
	totalPixels := b.Width * b.Height
	gpuImage := make([]byte, totalPixels*4)
	if err := benchmarks.Read(b.rt, b.Readbacks[0], gpuImage); err != nil {
		return err
	}

	errors := 0
	for py := 0; py < b.Height; py++ {
		for px := 0; px < b.Width; px++ {
			ref := b.cpuShadePixel(px, py)
			idx := (py*b.Width + px) * 4
			for c := 0; c < 4; c++ {
				diff := int(gpuImage[idx+c]) - int(ref[c])
				if diff < -1 || diff > 1 {
					if errors < 10 {
						log.Printf(
							"Mismatch at pixel (%d,%d) ch=%d: GPU=%d CPU=%d\n",
							px, py, c, gpuImage[idx+c], ref[c])
					}
					errors++
				}
			}
		}
	}

	if errors > 0 {
		return fmt.Errorf("%d channel errors in %d verified pixels\n",
			errors, totalPixels)
	}

	log.Printf("Passed!\n")

	return nil
}

func (b *Benchmark) enqueueVerification() {
	b.Readbacks = append(b.Readbacks, benchmarks.Download(b.rt, b.Queue, b.GImage, 1*((b.Width*b.Height)*4)))
}

// Step submits computation, then verification readback, then consumes results.
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
		b.Phase = 1
	case 1:
		if !b.Verification {
			b.Ended = true
			return nil
		}
		b.enqueueVerification()
		b.Phase = 2
	case 2:
		if err := b.verify(); err != nil {
			return err
		}
		b.Readbacks = nil
		b.Ended = true
	}
	return nil
}
