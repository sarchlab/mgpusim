package pagemigration

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/datamover"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/naming"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/heteromark/fir"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig"
)

// The copy kernel moves floats in work-groups of 64.
const (
	groupFloats = 64
	floatBytes  = 4
)

// platforms are the GPU models that every acceptance test runs on.
var platforms = []string{"r9nano", "mi300x"}

// slowWorkloads selects the workloads that take tens of minutes:
//
//	go test ./amd/tests/pagemigration/ -run SampleWorkloads -slow -timeout 3h
var slowWorkloads = flag.Bool("slow", false,
	"also run the page-migration workloads that take tens of minutes")

// option changes how a system is built.
type option func(*config)

type config struct {
	numGPUs   int
	spec      driver.PageMigrationSpec
	allocator gpuruntime.AllocatorType
	magicCopy bool
	resources []naming.Named
}

func gpus(n int) option { return func(c *config) { c.numGPUs = n } }

// resource registers a resource of the test with the simulation, so that it
// is saved in checkpoints.
func resource(r naming.Named) option {
	return func(c *config) { c.resources = append(c.resources, r) }
}

func migration(change func(*driver.PageMigrationSpec)) option {
	return func(c *config) { change(&c.spec) }
}

func buddyAllocator(c *config) { c.allocator = gpuruntime.AllocatorBuddy }

// magicCopies lets the driver copy between the host and the global storage
// directly, around the GPUs and their caches.
func magicCopies(c *config) { c.magicCopy = true }

// system is a complete timing platform with one command queue per GPU and a
// copy kernel: FIR with one tap of 1.0, which reads its input and writes its
// output, on R9 Nano (GCN3) and on MI300X (CDNA3).
type system struct {
	t       *testing.T
	gpuType string
	sim     *sim.Simulation
	engine  *timing.SerialEngine
	rt      *gpuruntime.Runtime
	driver  *driver.Comp
	storage *mem.Storage
	movers  []*datamover.Comp // by GPU-1

	ctx      gpuruntime.Context
	queues   []gpuruntime.CommandQueue // by GPU-1
	code     *insts.KernelCodeObject
	ones     []gpuruntime.Ptr // by GPU-1: the tap, in memory of the GPU
	history  []gpuruntime.Ptr // by GPU-1
	pageSize uint64
}

// newSystem builds and initializes a platform of two GPUs unless an option
// says otherwise, with the default page-migration specification.
func newSystem(t *testing.T, gpuType string, opts ...option) *system {
	t.Helper()

	if testing.Short() {
		t.Skip("page-migration acceptance builds and runs full simulations; run without -short")
	}

	cfg := config{numGPUs: 2, spec: driver.DefaultPageMigrationSpec()}
	for _, opt := range opts {
		opt(&cfg)
	}

	s := &system{t: t, gpuType: gpuType, sim: sim.MakeBuilder().Build()}
	t.Cleanup(func() {
		if err := s.sim.Terminate(); err != nil {
			t.Error(err)
		}
	})

	builder := timingconfig.MakeBuilder().
		WithSimulation(s.sim).
		WithNumGPUs(cfg.numGPUs).
		WithGPUType(gpuType).
		WithPageMigration(cfg.spec).
		WithAllocator(cfg.allocator)
	if cfg.magicCopy {
		builder = builder.WithMagicMemoryCopy()
	}

	s.rt = builder.Build()

	for _, r := range cfg.resources {
		s.sim.RegisterResource(r)
	}

	if err := s.sim.Initialize(); err != nil {
		t.Fatal(err)
	}

	s.engine = s.sim.Engine().(*timing.SerialEngine)
	s.movers = make([]*datamover.Comp, cfg.numGPUs)

	for _, c := range s.sim.Components() {
		switch c := c.(type) {
		case *driver.Comp:
			s.driver = c
		case *datamover.Comp:
			var gpu int
			if _, err := fmt.Sscanf(c.Name(), "GPU[%d].DataMover", &gpu); err == nil {
				s.movers[gpu-1] = c
			}
		}
	}

	s.storage = s.driver.Resources.GlobalStorage
	s.pageSize = uint64(1) << s.driver.Resources.PageTable.Log2PageSize()
	s.prepareWorkload(cfg.numGPUs)

	return s
}

// prepareWorkload creates a queue per GPU and puts the tap of the copy
// kernel in the memory of each GPU, so that only the buffers a test passes
// to touch are shared.
func (s *system) prepareWorkload(numGPUs int) {
	s.t.Helper()

	file := "kernels.hsaco"
	if s.gpuType == "mi300x" {
		file = "kernels_gfx942.hsaco"
	}

	data, err := os.ReadFile(filepath.Join("..", "..", "benchmarks", "heteromark", "fir", file))
	if err != nil {
		s.t.Fatal(err)
	}

	s.code = insts.LoadKernelCodeObjectFromBytes(data, "FIR")
	if s.code == nil {
		s.t.Fatal("missing FIR kernel")
	}

	s.ctx = s.rt.Init()

	for gpu := 1; gpu <= numGPUs; gpu++ {
		s.rt.SelectGPU(s.ctx, gpu)
		q := s.rt.CreateCommandQueue(s.ctx)
		one := s.rt.AllocateMemory(s.ctx, floatBytes)
		benchmarks.Upload(s.rt, q, one, []float32{1})

		s.queues = append(s.queues, q)
		s.ones = append(s.ones, one)
		s.history = append(s.history, s.rt.AllocateMemory(s.ctx, floatBytes))
	}

	s.run()
}

// sibling returns the same system seen through a second context of its
// address space, made with InitWithExistingPID. What is queued through it
// goes to command queues of that context, one per GPU, and addresses the
// memory that the first context allocated.
func (s *system) sibling() *system {
	other := *s
	other.ctx = s.rt.InitWithExistingPID(s.ctx)
	other.queues = nil

	for gpu := 1; gpu <= len(s.queues); gpu++ {
		s.rt.SelectGPU(other.ctx, gpu)
		other.queues = append(other.queues, s.rt.CreateCommandQueue(other.ctx))
	}

	return &other
}

// local allocates memory of a GPU; it does not migrate.
func (s *system) local(gpu int, bytes uint64) gpuruntime.Ptr {
	s.rt.SelectGPU(s.ctx, gpu)
	return s.rt.AllocateMemory(s.ctx, bytes)
}

// unified allocates unified memory; its pages start on GPU 1.
func (s *system) unified(bytes uint64) gpuruntime.Ptr {
	return s.rt.AllocateUnifiedMemory(s.ctx, bytes)
}

// alignedUnified allocates a unified buffer of n pages whose first page
// number is a multiple of group. Single unified pages are allocated in front
// of it until the buffer starts there; nothing unified follows it.
func (s *system) alignedUnified(n, group uint64) gpuruntime.Ptr {
	for {
		ptr := uint64(s.unified(s.pageSize))
		if (ptr/s.pageSize+1)%group == 0 {
			return s.unified(n * s.pageSize)
		}
	}
}

// at returns the address of a float in a page of a buffer.
func (s *system) at(buffer gpuruntime.Ptr, page uint64, float uint64) gpuruntime.Ptr {
	return buffer + gpuruntime.Ptr(page*s.pageSize+float*floatBytes)
}

// padBuddy allocates pages of a GPU until the number of pages that the buddy
// allocator has handed out is a multiple of group, a power of two. The
// allocator then has no unused block smaller than group pages left, so the
// next pages it hands out are pages that are freed from now on. It relies on
// the buddy allocator handing unused pages out in address order, and on
// nothing of the GPU having been freed before.
func (s *system) padBuddy(gpu int, group uint64) {
	base := s.page(s.ones[gpu-1]).PAddr

	for {
		frame := s.page(s.local(gpu, 1)).PAddr
		if ((frame-base)/s.pageSize+1)%group == 0 {
			return
		}
	}
}

func (s *system) free(ptr gpuruntime.Ptr) {
	s.t.Helper()

	if err := s.rt.FreeMemory(s.ctx, ptr); err != nil {
		s.t.Fatal(err)
	}
}

// upload copies data from the host to device memory and runs the simulation
// until the data is there.
func (s *system) upload(dst gpuruntime.Ptr, data []float32) {
	s.t.Helper()

	benchmarks.Upload(s.rt, s.queues[0], dst, data)
	s.run()
}

// download runs the simulation and copies floats from device memory to the
// host.
func (s *system) download(src gpuruntime.Ptr, floats int) []float32 {
	s.t.Helper()

	buffer := benchmarks.Download(s.rt, s.queues[0], src, floats*floatBytes)
	s.run()

	out := make([]float32, floats)
	if err := benchmarks.Read(s.rt, buffer, out); err != nil {
		s.t.Fatal(err)
	}

	return out
}

// touch queues the copy kernel on a GPU: the GPU reads floats at src and
// writes them at dst.
func (s *system) touch(gpu int, dst, src gpuruntime.Ptr, floats int) {
	s.t.Helper()

	if floats%groupFloats != 0 {
		s.t.Fatalf("the copy kernel moves multiples of %d floats, not %d", groupFloats, floats)
	}

	var args any = &fir.GCN3KernelArgs{
		Output:  dst,
		Filter:  s.ones[gpu-1],
		Input:   src,
		History: s.history[gpu-1],
		NumTaps: 1,
	}

	if s.gpuType == "mi300x" {
		args = &fir.CDNA3KernelArgs{
			Output:            dst,
			Filter:            s.ones[gpu-1],
			Input:             src,
			History:           s.history[gpu-1],
			NumTaps:           1,
			HiddenBlockCountX: uint32(floats / groupFloats),
			HiddenBlockCountY: 1,
			HiddenBlockCountZ: 1,
			HiddenGroupSizeX:  groupFloats,
			HiddenGroupSizeY:  1,
			HiddenGroupSizeZ:  1,
		}
	}

	s.rt.EnqueueLaunchKernel(s.queues[gpu-1], s.code,
		[3]uint32{uint32(floats), 1, 1}, [3]uint16{groupFloats, 1, 1}, args)
}

// start lets the driver take the queued commands.
func (s *system) start() { s.driver.TickLater() }

// run runs the queued commands to their end. A fault that is lost leaves a
// kernel waiting forever, which shows as a stalled queue.
func (s *system) run() {
	s.t.Helper()
	s.start()

	if err := s.engine.Run(); err != nil {
		s.t.Fatal(err)
	}

	s.mustBeDone()
}

func (s *system) mustBeDone() {
	s.t.Helper()

	for i, n := range s.rt.Unfinished() {
		if n != 0 {
			s.t.Fatalf("queue %d stalled with %d unfinished commands at %d ps; migration: %+v",
				i+1, n, s.engine.CurrentTime(), s.stats())
		}
	}

	m := s.driver.State.PageMigration
	if len(m.Pending) != 0 || len(m.Queue) != 0 || len(m.Windows) != 0 {
		s.t.Fatalf("migration did not come to rest: %d commands, %d requests, %d windows",
			len(m.Pending), len(m.Queue), len(m.Windows))
	}
}

// done reports whether every queued command has completed.
func (s *system) done() bool {
	for _, n := range s.rt.Unfinished() {
		if n != 0 {
			return false
		}
	}

	return true
}

// runWatching runs the queued commands to their end and calls watch after
// every nanosecond of simulated time.
func (s *system) runWatching(watch func()) {
	s.t.Helper()
	s.start()

	const limit = 100_000_000 // 100 ms

	for i := 0; !s.done(); i++ {
		if i == limit {
			s.t.Fatalf("the commands do not complete; migration: %+v", s.stats())
		}

		if err := s.engine.RunUntil(s.engine.CurrentTime() + 1000); err != nil {
			s.t.Fatal(err)
		}

		watch()
	}

	s.run()
}

func (s *system) stats() driver.PageMigrationStats {
	return s.driver.State.PageMigration.Stats
}

func (s *system) pid() vm.PID { return s.rt.QueuePID(s.queues[0]) }

// page returns the page of an address as the page table has it now.
func (s *system) page(ptr gpuruntime.Ptr) vm.Page {
	s.t.Helper()

	p, ok := s.driver.Resources.PageTable.Find(s.pid(), uint64(ptr))
	if !ok {
		s.t.Fatalf("no page at %#x", ptr)
	}

	return p
}

// owners returns the GPUs that have the n pages from ptr.
func (s *system) owners(ptr gpuruntime.Ptr, n uint64) []uint64 {
	s.t.Helper()

	owners := make([]uint64, n)
	for i := range owners {
		owners[i] = s.page(ptr + gpuruntime.Ptr(uint64(i)*s.pageSize)).DeviceID
	}

	return owners
}

// dirtyInL2 reports whether nothing of what a kernel wrote to a frame has
// reached the memory below the L2: the frame is still all zeros there, so
// the lines are in the L2 only.
func (s *system) dirtyInL2(pAddr, bytes uint64) bool {
	return !slices.ContainsFunc(s.storage.Read(pAddr, bytes), func(b byte) bool {
		return b != 0
	})
}

// ramp returns n floats that count up from start.
func ramp(start float32, n int) []float32 {
	data := make([]float32, n)
	for i := range data {
		data[i] = start + float32(i)
	}

	return data
}

func mustEqualFloats(t *testing.T, what string, got, want []float32) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("%s: %d floats, want %d", what, len(got), len(want))
	}

	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%s: float %d is %v, want %v", what, i, got[i], want[i])
		}
	}
}

func mustEqualOwners(t *testing.T, what string, got []uint64, want ...uint64) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("%s: owners %v, want %v", what, got, want)
	}

	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%s: owners %v, want %v", what, got, want)
		}
	}
}
