package timingconfig

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/mem/datamover"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm/mmu"
	simulation "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
	"github.com/sarchlab/mgpusim/v5/amd/timing/cp"
	"github.com/sarchlab/mgpusim/v5/amd/timing/rdma"
)

const notificationPort = messaging.RemotePort("Driver.MigrationNotification")

// migrationPlatform is the page-migration wiring of a built platform.
type migrationPlatform struct {
	sim    *simulation.Simulation
	driver *driver.Comp
	mmu    *mmu.Comp
	rdmas  map[string]*rdma.Comp
	cps    map[string]*cp.Comp
	movers map[string]*datamover.Comp
}

// buildMigrationPlatform assembles and initializes a two-GPU platform.
// Initialization checks the port bindings of every component.
func buildMigrationPlatform(
	t *testing.T,
	gpuType string,
	configure func(Builder) Builder,
) migrationPlatform {
	t.Helper()

	s := simulation.MakeBuilder().Build()
	t.Cleanup(func() {
		if err := s.Terminate(); err != nil {
			t.Error(err)
		}
	})

	configure(MakeBuilder().WithSimulation(s).WithNumGPUs(2).WithGPUType(gpuType)).Build()

	if err := s.Initialize(); err != nil {
		t.Fatal(err)
	}

	p := migrationPlatform{
		sim:    s,
		rdmas:  map[string]*rdma.Comp{},
		cps:    map[string]*cp.Comp{},
		movers: map[string]*datamover.Comp{},
	}

	for _, c := range s.Components() {
		switch c := c.(type) {
		case *driver.Comp:
			p.driver = c
		case *mmu.Comp:
			p.mmu = c
		case *rdma.Comp:
			p.rdmas[c.Name()] = c
		case *cp.Comp:
			p.cps[c.Name()] = c
		case *datamover.Comp:
			p.movers[c.Name()] = c
		}
	}

	for _, c := range s.Components() {
		checkRouting(t, s, c, len(p.driver.Spec.GPUs))
	}

	return p
}

func defaultPlatform(b Builder) Builder { return b }

var gpuTypes = []struct {
	name         string
	log2PageSize uint64
	lowerMemory  string // what a page copy reads and writes below the L2
}{
	{"r9nano", 12, "DRAM"},
	{"mi300x", 21, "MALL"},
}

// By default, pages migrate on faults with the UVM parameters: the MMU
// reports the faults, and the RDMA engines count nothing.
func TestDefaultPlatformMigratesOnFaults(t *testing.T) {
	for _, g := range gpuTypes {
		t.Run(g.name, func(t *testing.T) {
			p := buildMigrationPlatform(t, g.name, defaultPlatform)

			if p.driver.Spec.PageMigration != driver.DefaultPageMigrationSpec() ||
				p.driver.Spec.PageMigration.Policy != driver.PageMigrationFault {
				t.Fatalf("driver policy: %+v", p.driver.Spec.PageMigration)
			}

			if p.mmu.Spec.FaultHandler != notificationPort {
				t.Fatalf("MMU fault handler is %q", p.mmu.Spec.FaultHandler)
			}

			for name, r := range p.rdmas {
				if r.Spec.AccessCounterHandler != "" {
					t.Fatalf("%s counts with the fault policy", name)
				}
			}
		})
	}
}

// Access-counter migration is another policy of the same specification: the
// MMU raises no faults, and the RDMA engines count per frame of the system
// page size.
func TestCounterPolicyCountsFramesOfTheSystemPageSize(t *testing.T) {
	for _, g := range gpuTypes {
		t.Run(g.name, func(t *testing.T) {
			spec := driver.DefaultPageMigrationSpec()
			spec.Policy = driver.PageMigrationCounter
			spec.AccessCounterThreshold = 16

			p := buildMigrationPlatform(t, g.name, func(b Builder) Builder {
				return b.WithPageMigration(spec)
			})

			if p.mmu.Spec.FaultHandler != "" || len(p.mmu.Ports.Notification) != 0 {
				t.Fatalf("MMU faults with the counter policy: %q", p.mmu.Spec.FaultHandler)
			}

			for name, r := range p.rdmas {
				if r.Spec.AccessCounterHandler != notificationPort ||
					r.Spec.AccessCounterThreshold != 16 ||
					r.Spec.Log2PageSize != g.log2PageSize {
					t.Fatalf("%s counters: %+v", name, r.Spec)
				}
			}
		})
	}
}

// remoteAccesses lets the caches of the GPU of an RDMA engine send it n reads
// in the frame at frame, one line after the other, and returns the
// access-counter notifications that the engine sent to the driver.
func remoteAccesses(
	t *testing.T,
	engine *rdma.Comp,
	frame, pageSize uint64,
	n int,
) []protocol.AccessCounterNotification {
	t.Helper()

	in, out := engine.Ports.RDMARequestInside, engine.Ports.RDMARequestOutside

	for i := range n {
		in.Deliver(messaging.Msg{
			ID:  engine.NewID(),
			Src: messaging.RemotePort(engine.Name() + ".TestL1Cache"),
			Dst: in.AsRemote(),
			Payload: memprotocol.ReadReq{
				Address: frame + uint64(i)*64%pageSize, AccessByteSize: 64,
			},
		})
		engine.Middlewares.Forward.Tick()

		if _, forwarded := out.RetrieveOutgoing(); !forwarded {
			t.Fatalf("%s did not forward a remote access", engine.Name())
		}
	}

	engine.Middlewares.Forward.Tick()

	var notifications []protocol.AccessCounterNotification

	for {
		msg, ok := engine.Ports.AccessCounter.RetrieveOutgoing()
		if !ok {
			return notifications
		}

		if msg.Dst != notificationPort {
			t.Fatalf("%s notifies %q", engine.Name(), msg.Dst)
		}

		notifications = append(notifications, msg.Payload.(protocol.AccessCounterNotification))
	}
}

func mustNotify(
	t *testing.T,
	what string,
	got []protocol.AccessCounterNotification,
	want ...protocol.AccessCounterNotification,
) {
	t.Helper()

	if !slices.Equal(got, want) {
		t.Fatalf("%s: notifications %+v, want %+v", what, got, want)
	}
}

// The access-counter threshold of the default specification is 256 remote
// demand accesses of one GPU to one physical page frame, whatever the system
// page size. On a platform with the counter policy and nothing else changed,
// the RDMA engine of a GPU says nothing for its first 255 accesses to a frame
// of another GPU, notifies the driver once with the 256th, and starts over:
// its next notification for the frame is the 512th access. The engine counts
// the requests that reach it, so accesses that repeat a line count, and a
// frame of 4 KiB reaches the threshold as well. The accesses of another GPU
// to the frame, and the accesses to another frame, are counted on their own.
func TestTheDefaultCounterThresholdIs256RemoteAccessesOfAGPUToAFrame(t *testing.T) {
	for _, g := range gpuTypes {
		t.Run(g.name, func(t *testing.T) {
			spec := driver.DefaultPageMigrationSpec()
			spec.Policy = driver.PageMigrationCounter

			p := buildMigrationPlatform(t, g.name, func(b Builder) Builder {
				return b.WithNumGPUs(3).WithPageMigration(spec)
			})

			second, third := p.rdmas["GPU[2].RDMA"], p.rdmas["GPU[3].RDMA"]
			for _, engine := range []*rdma.Comp{second, third} {
				if engine.Spec.AccessCounterThreshold != 256 {
					t.Fatalf("%s notifies at %d accesses", engine.Name(), engine.Spec.AccessCounterThreshold)
				}
			}

			// Two frames of GPU 1, next to each other.
			pageSize := uint64(1) << g.log2PageSize
			frame := deviceBytes + 4*pageSize
			neighbor := frame + pageSize

			notification := func(pAddr, gpu uint64) protocol.AccessCounterNotification {
				return protocol.AccessCounterNotification{PAddr: pAddr, DeviceID: gpu}
			}

			// GPU 2 and GPU 3 each access the frame 255 times, and GPU 2 the
			// frame next to it: 510 accesses to the frame, and no
			// notification.
			mustNotify(t, "255 accesses of GPU 2", remoteAccesses(t, second, frame, pageSize, 255))
			mustNotify(t, "255 accesses of GPU 3", remoteAccesses(t, third, frame, pageSize, 255))
			mustNotify(t, "255 accesses of GPU 2 to the neighbor",
				remoteAccesses(t, second, neighbor, pageSize, 255))

			// The 256th access of GPU 2 notifies, once, for GPU 2.
			mustNotify(t, "the 256th access of GPU 2",
				remoteAccesses(t, second, frame, pageSize, 1), notification(frame, 2))

			// The count started over: 255 more accesses say nothing, and the
			// second 256th notifies again.
			mustNotify(t, "accesses 257 to 511 of GPU 2", remoteAccesses(t, second, frame, pageSize, 255))
			mustNotify(t, "the 512th access of GPU 2",
				remoteAccesses(t, second, frame, pageSize, 1), notification(frame, 2))

			// GPU 3 and the neighbor frame reach their own 256th access.
			mustNotify(t, "the 256th access of GPU 3",
				remoteAccesses(t, third, frame, pageSize, 1), notification(frame, 3))
			mustNotify(t, "the 256th access of GPU 2 to the neighbor",
				remoteAccesses(t, second, neighbor, pageSize, 1), notification(neighbor, 2))
		})
	}
}

func TestNoMigrationPolicyReportsNothingToTheDriver(t *testing.T) {
	spec := driver.DefaultPageMigrationSpec()
	spec.Policy = driver.PageMigrationNone

	p := buildMigrationPlatform(t, "r9nano", func(b Builder) Builder {
		return b.WithPageMigration(spec)
	})

	if p.mmu.Spec.FaultHandler != "" {
		t.Fatalf("MMU fault handler is %q", p.mmu.Spec.FaultHandler)
	}

	for name, r := range p.rdmas {
		if r.Spec.AccessCounterHandler != "" {
			t.Fatalf("%s counts without migration", name)
		}
	}
}

// Each GPU has one data mover, which pulls the pages that migrate to the GPU
// and keeps 64 page copies in flight by default. It copies around the L2 on
// both ends: between the DRAMs on R9 Nano, and between the MALLs on MI300X.
func TestEachGPUHasOneDataMoverThatPullsAroundTheL2(t *testing.T) {
	for _, g := range gpuTypes {
		t.Run(g.name, func(t *testing.T) {
			p := buildMigrationPlatform(t, g.name, defaultPlatform)

			if len(p.movers) != 2 {
				t.Fatalf("%d data movers for two GPUs", len(p.movers))
			}

			for gpu := 1; gpu <= 2; gpu++ {
				checkDataMover(t, p, gpu, g.lowerMemory)
			}
		})
	}
}

func checkDataMover(t *testing.T, p migrationPlatform, gpu int, lowerMemory string) {
	t.Helper()

	name := fmt.Sprintf("GPU[%d]", gpu)
	peer := fmt.Sprintf("GPU[%d]", 3-gpu)
	local := uint64(gpu) * 4 << 30
	remote := uint64(3-gpu) * 4 << 30
	rdmaSpec := p.rdmas[name+".RDMA"].Spec
	cpSpec := p.cps[name+".CommandProcessor"].Spec

	mover := p.movers[name+".DataMover"]
	if mover == nil {
		t.Fatalf("%s has no data mover", name)
	}

	if mover.Spec.MaxTransactions != 64 {
		t.Fatalf("%s keeps %d page copies in flight", mover.Name(), mover.Spec.MaxTransactions)
	}

	// Inside is the local memory below the L2, and Outside the port of the
	// other GPU that reaches its memory below the L2.
	inside := string(mover.Spec.Inside.Find(local))
	if !strings.HasPrefix(inside, name+"."+lowerMemory+"[") {
		t.Fatalf("%s writes inside to %s", mover.Name(), inside)
	}

	if got := mover.Spec.Outside.Find(remote); got != messaging.RemotePort(
		peer+".RDMA.MigrationOutside") {
		t.Fatalf("%s reads outside from %s", mover.Name(), got)
	}

	// The RDMA engine passes the page copies of the other GPU to the same
	// memory below the L2, not to the L2 as it does with remote accesses.
	below := string(rdmaSpec.MigrationModules.Find(local))
	if !strings.HasPrefix(below, name+"."+lowerMemory+"[") ||
		!strings.HasPrefix(string(rdmaSpec.LocalModules.Find(local)), name+".L2Cache[") {
		t.Fatalf("%s routes page copies to %s", name, below)
	}

	if cpSpec.DataMover != mover.Ports.Top.AsRemote() {
		t.Fatalf("%s command processor copies with %q", name, cpSpec.DataMover)
	}
}

func TestDataMoverPagesComeFromTheSystemSpecification(t *testing.T) {
	spec := driver.DefaultPageMigrationSpec()
	spec.DataMoverPages = 8

	p := buildMigrationPlatform(t, "r9nano", func(b Builder) Builder {
		return b.WithPageMigration(spec)
	})

	for name, mover := range p.movers {
		if mover.Spec.MaxTransactions != 8 {
			t.Fatalf("%s keeps %d page copies in flight", name, mover.Spec.MaxTransactions)
		}
	}
}
