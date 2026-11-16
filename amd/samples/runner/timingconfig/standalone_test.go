package timingconfig

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/datamover"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/mmu"
	simulation "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	directconnection "github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks/heteromark/fir"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig/gpubuilder"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig/mi300x"
	"github.com/sarchlab/mgpusim/v5/amd/samples/runner/timingconfig/r9nano"
	"github.com/sarchlab/mgpusim/v5/amd/sampling"
)

// standaloneGPU is one GPU that a caller assembles itself with the exported
// builder of its type, without the platform builder of this package and as
// before page migration: the GPU builder gets no page-migration
// configuration, and the caller connects the ports that ExternalPorts
// returns, an MMU, and a driver that migrates nothing.
type standaloneGPU struct {
	sim    *simulation.Simulation
	rt     *gpuruntime.Runtime
	gpu    *gpubuilder.GPU
	driver *driver.Comp
}

//nolint:funlen // Keep the assembly of a caller in one place.
func buildStandaloneGPU(
	t *testing.T,
	gpuType string,
	log2PageSize uint64,
	basePortsOnly bool,
) standaloneGPU {
	t.Helper()

	s := simulation.MakeBuilder().Build()
	t.Cleanup(func() {
		if err := s.Terminate(); err != nil {
			t.Error(err)
		}
	})

	port := func(owner interface {
		BindPort(string, messaging.Port)
	}, name string) messaging.Port {
		p := twowaybuffered.MakeBuilder().WithSimulation(s).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: 4096, OutgoingCapacity: 4096}).
			Build()
		owner.BindPort(name, p)

		return p
	}

	storage := mem.MakeStorageBuilder().WithSimulation(s).WithCapacity(2 * deviceBytes).Build("GlobalStorage")
	pageTable := vm.MakePageTableBuilder().WithSimulation(s).WithLog2PageSize(log2PageSize).Build("PageTable")

	mmuSpec := mmu.DefaultSpec()
	mmuSpec.Freq = timing.GHz
	mmuSpec.Latency = 100
	mmuSpec.Log2PageSize = log2PageSize
	mmuComp := modeling.MakeBuilder[*mmu.Comp](mmuSpec).WithSimulation(s).
		WithResources(mmu.Resources{PageTable: pageTable}).
		Build("MMU")
	port(mmuComp, "Top")
	port(mmuComp, "Control")

	sampler := sampling.NewSimulationEngine(s, *sampling.SampledRunnerGranularyFlag,
		*sampling.SampledRunnerThresholdFlag, *sampling.SampledRunnerFlag)

	var builder gpubuilder.GPUBuilder
	if gpuType == "mi300x" {
		builder = mi300x.MakeBuilder().WithSampler(sampler).WithSimulation(s).WithMMU(mmuComp).
			WithLog2PageSize(log2PageSize).WithGlobalStorage(storage).WithDriverPort("Driver.GPU")
	} else {
		builder = r9nano.MakeBuilder().WithSampler(sampler).WithSimulation(s).WithMMU(mmuComp).
			WithLog2PageSize(log2PageSize).WithGlobalStorage(storage).WithDriverPort("Driver.GPU")
	}

	gpu := builder.
		WithGPUID(1).
		WithMemAddrOffset(deviceBytes).
		WithRDMAAddressMapper(mem.AddressMapper{
			InterleavingSize: deviceBytes,
			Destinations:     []messaging.RemotePort{"CPU", "GPU[1].RDMA.RDMADataOutside"},
		}).
		Build("GPU[1]")

	conn := directconnection.NewConnection("InterDeviceConn", s, timing.GHz)
	conn.BindPort(mmuComp.Ports.Top)

	// A caller connects either every port that the GPU hands out, or only
	// the ports that a GPU had before page migration.
	external := gpu.ExternalPorts()
	if basePortsOnly {
		external = append([]messaging.Port{
			gpu.CommandProcessorPort, gpu.RDMARequestPort, gpu.RDMADataPort,
		}, gpu.TranslationPorts...)
	}

	for _, p := range external {
		conn.BindPort(p)
	}

	// The driver of a platform whose GPUs cannot migrate pages.
	spec := driver.DefaultSpec()
	spec.PageMigration.Policy = driver.PageMigrationNone
	spec.H2DCycles, spec.D2HCycles = 500, 300
	spec.GPUs = []messaging.RemotePort{gpu.CommandProcessorPort.AsRemote()}

	runtimeSpec := gpuruntime.DefaultRuntimeSpec()
	runtimeSpec.Log2PageSize = log2PageSize
	runtimeSpec.Devices = []gpuruntime.DeviceProperties{{CUCount: 1, DRAMSize: deviceBytes}}

	rt := gpuruntime.NewRuntime(s, "Runtime", runtimeSpec, pageTable, storage)
	d := driver.MakeBuilder().WithSimulation(s).WithSpec(spec).
		WithResources(driver.Resources{Runtime: rt, PageTable: pageTable, GlobalStorage: storage}).
		Build("Driver")

	for _, name := range []string{
		driver.GPUPortName, driver.PageMigrationPortName, driver.MigrationNotificationPortName,
	} {
		conn.BindPort(port(d, name))
	}

	if err := s.Initialize(); err != nil {
		t.Fatal(err)
	}

	return standaloneGPU{sim: s, rt: rt, gpu: gpu, driver: d}
}

// copyKernel queues the FIR kernel with one tap of 1.0 on the GPU of a
// queue: the GPU reads floats at src and writes them at dst.
func copyKernel(
	t *testing.T,
	rt *gpuruntime.Runtime,
	ctx gpuruntime.Context,
	q gpuruntime.CommandQueue,
	gpuType string,
	dst, src gpuruntime.Ptr,
	floats int,
) {
	t.Helper()

	const group = 64

	file := "kernels.hsaco"
	if gpuType == "mi300x" {
		file = "kernels_gfx942.hsaco"
	}

	data, err := os.ReadFile(filepath.Join("..", "..", "..", "benchmarks", "heteromark", "fir", file))
	if err != nil {
		t.Fatal(err)
	}

	code := insts.LoadKernelCodeObjectFromBytes(data, "FIR")
	if code == nil {
		t.Fatal("missing FIR kernel")
	}

	one := rt.AllocateMemory(ctx, 4)
	history := rt.AllocateMemory(ctx, 4)
	benchmarks.Upload(rt, q, one, []float32{1})

	var args any = &fir.GCN3KernelArgs{Output: dst, Filter: one, Input: src, History: history, NumTaps: 1}
	if gpuType == "mi300x" {
		args = &fir.CDNA3KernelArgs{
			Output: dst, Filter: one, Input: src, History: history, NumTaps: 1,
			HiddenBlockCountX: uint32(floats / group), HiddenBlockCountY: 1, HiddenBlockCountZ: 1,
			HiddenGroupSizeX: group, HiddenGroupSizeY: 1, HiddenGroupSizeZ: 1,
		}
	}

	rt.EnqueueLaunchKernel(q, code, [3]uint32{uint32(floats), 1, 1}, [3]uint16{group, 1, 1}, args)
}

// The exported builders of R9 Nano and MI300X build a GPU that works on its
// own, with the configuration that they needed before page migration and no
// WithPageMigration: such a GPU has nothing to copy pages from, so it has no
// data mover, and every port that it hands out is a port. Whether the caller
// connects all of them or only the ports that a GPU had before, a host
// uploads to the GPU, a kernel runs on it, and the host downloads the result.
func TestAGPUBuiltOnItsOwnNeedsNoPageMigrationConfiguration(t *testing.T) {
	for _, g := range gpuTypes {
		for _, ports := range []string{"every-port", "ports-as-before"} {
			t.Run(g.name+"/"+ports, func(t *testing.T) {
				p := buildStandaloneGPU(t, g.name, g.log2PageSize, ports == "ports-as-before")
				p.mustHaveNoDataMover(t)
				p.mustRunAKernel(t, g.name)
			})
		}
	}
}

func (p standaloneGPU) mustHaveNoDataMover(t *testing.T) {
	t.Helper()

	for _, c := range p.sim.Components() {
		if mover, ok := c.(*datamover.Comp); ok {
			t.Fatalf("a GPU without peers has the data mover %s", mover.Name())
		}
	}

	if p.gpu.DataMoverPort != nil || p.gpu.CommandProcessor.Spec.DataMover != "" ||
		slices.Contains(p.gpu.ExternalPorts(), nil) {
		t.Fatalf("the GPU hands out a data mover that it does not have: %q",
			p.gpu.CommandProcessor.Spec.DataMover)
	}
}

// mustRunAKernel uploads floats to the GPU, lets a kernel copy them, and
// downloads the copy.
func (p standaloneGPU) mustRunAKernel(t *testing.T, gpuType string) {
	t.Helper()

	const floats = 256

	input := make([]float32, floats)
	for i := range input {
		input[i] = float32(i)*1.5 + 1
	}

	ctx := p.rt.Init()
	q := p.rt.CreateCommandQueue(ctx)
	source := p.rt.AllocateMemory(ctx, floats*4)
	result := p.rt.AllocateMemory(ctx, floats*4)

	benchmarks.Upload(p.rt, q, source, input)
	copyKernel(t, p.rt, ctx, q, gpuType, result, source, floats)
	download := benchmarks.Download(p.rt, q, result, floats*4)

	p.driver.TickLater()

	if err := p.sim.Run(); err != nil {
		t.Fatal(err)
	}

	if unfinished := p.rt.Unfinished(); unfinished[0] != 0 {
		t.Fatalf("the queue stalled with %d unfinished commands", unfinished[0])
	}

	got := make([]float32, floats)
	if err := benchmarks.Read(p.rt, download, got); err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(got, input) {
		t.Fatalf("the kernel output is %v..., want %v...", got[:4], input[:4])
	}
}

// The platform builder of this package configures page migration for every
// GPU, so every GPU of a platform has one data mover, also the GPU of the
// standard platform of one GPU, with either migration policy or none. The
// data mover works: asked by the command processor, it copies a page below
// the L2 of the GPU, reading through the RDMA engine as it reads from a peer.
func TestThePlatformOfOneGPUHasAWorkingDataMover(t *testing.T) {
	policies := []string{driver.PageMigrationFault, driver.PageMigrationCounter, driver.PageMigrationNone}

	for _, g := range gpuTypes {
		for _, policy := range policies {
			t.Run(g.name+"/"+policy, func(t *testing.T) {
				spec := driver.DefaultPageMigrationSpec()
				spec.Policy = policy

				p := buildMigrationPlatform(t, g.name, func(b Builder) Builder {
					return b.WithNumGPUs(1).WithPageMigration(spec)
				})

				mover := p.movers["GPU[1].DataMover"]
				cpComp := p.cps["GPU[1].CommandProcessor"]

				if len(p.movers) != 1 || mover == nil || cpComp.Spec.DataMover != mover.Ports.Top.AsRemote() {
					t.Fatalf("%d data movers; the command processor copies with %q", len(p.movers), cpComp.Spec.DataMover)
				}

				for _, port := range []messaging.Port{
					mover.Ports.Top, mover.Ports.Control, mover.Ports.Inside, mover.Ports.Outside,
				} {
					if port.Connection() == nil {
						t.Fatalf("%s is not connected", port.Name())
					}
				}

				if got := mover.Spec.Outside.Find(deviceBytes); got != "GPU[1].RDMA.MigrationOutside" {
					t.Fatalf("the data mover reads the memory of GPU 1 through %s", got)
				}

				// Only a driver that does not migrate leaves the answer of the
				// command processor alone, so that the test can play the driver.
				if policy == driver.PageMigrationNone {
					p.mustCopyAPageOfGPU1(t)
				}
			})
		}
	}
}

// mustCopyAPageOfGPU1 asks the command processor of GPU 1, as the driver
// does, to copy a page of GPU 1 to another frame of GPU 1, and checks the
// copy in the memory below the L2.
func (p migrationPlatform) mustCopyAPageOfGPU1(t *testing.T) {
	t.Helper()

	const (
		bytes        = 4096
		src   uint64 = deviceBytes + 16<<21
		dst          = src + 1<<21
	)

	page := make([]byte, bytes)
	for i := range page {
		page[i] = byte(1 + i%251)
	}

	storage := p.driver.Resources.GlobalStorage
	storage.Write(src, page)

	migration := p.cps["GPU[1].CommandProcessor"].Ports.PageMigration
	migration.Deliver(messaging.Msg{
		ID:      p.sim.NewID(),
		Src:     p.driver.Ports.PageMigration.AsRemote(),
		Dst:     migration.AsRemote(),
		Payload: protocol.PageCopyReq{SrcAddress: src, DstAddress: dst, PageSize: bytes},
	})

	if err := p.sim.Run(); err != nil {
		t.Fatal(err)
	}

	if got := storage.Read(dst, bytes); !slices.Equal(got, page) {
		t.Fatalf("the data mover did not copy the page: % x...", got[:8])
	}

	if answers := p.driver.Ports.PageMigration.IncomingBuf().Size(); answers != 1 {
		t.Fatalf("the command processor sent %d answers", answers)
	}
}
