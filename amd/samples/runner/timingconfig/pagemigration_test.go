package timingconfig

import (
	"fmt"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/mem/datamover"
	"github.com/sarchlab/akita/v5/mem/vm/mmu"
	simulation "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	"github.com/sarchlab/mgpusim/v5/amd/timing/cp"
	"github.com/sarchlab/mgpusim/v5/amd/timing/rdma"
)

const notificationPort = messaging.RemotePort("Driver.MigrationNotification")

// migrationPlatform is the page-migration wiring of a built platform.
type migrationPlatform struct {
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
		rdmas:  map[string]*rdma.Comp{},
		cps:    map[string]*cp.Comp{},
		movers: map[string]*datamover.Comp{},
	}

	for _, c := range s.Components() {
		checkRouting(t, s, c, 2)

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

// Each GPU has an inbound and an outbound data mover that keep 64 page
// copies in flight by default, and both copy around the L2 on both ends:
// between the DRAMs on R9 Nano, and between the MALLs on MI300X.
func TestEachGPUHasAnInboundAndAnOutboundDataMoverAroundTheL2(t *testing.T) {
	for _, g := range gpuTypes {
		t.Run(g.name, func(t *testing.T) {
			p := buildMigrationPlatform(t, g.name, defaultPlatform)

			if len(p.movers) != 4 {
				t.Fatalf("%d data movers for two GPUs", len(p.movers))
			}

			for gpu := 1; gpu <= 2; gpu++ {
				checkDataMovers(t, p, gpu, g.lowerMemory)
			}
		})
	}
}

func checkDataMovers(t *testing.T, p migrationPlatform, gpu int, lowerMemory string) {
	t.Helper()

	name := fmt.Sprintf("GPU[%d]", gpu)
	peer := fmt.Sprintf("GPU[%d]", 3-gpu)
	local := uint64(gpu) * 4 << 30
	remote := uint64(3-gpu) * 4 << 30
	rdmaSpec := p.rdmas[name+".RDMA"].Spec
	cpSpec := p.cps[name+".CommandProcessor"].Spec

	for _, direction := range []string{"Inbound", "Outbound"} {
		mover := p.movers[name+"."+direction+"DataMover"]
		if mover == nil {
			t.Fatalf("%s has no %s data mover", name, direction)
		}

		if mover.Spec.MaxTransactions != 64 {
			t.Fatalf("%s keeps %d page copies in flight", mover.Name(), mover.Spec.MaxTransactions)
		}

		// Inside is the local memory below the L2, and Outside the port of
		// the other GPU that reaches its memory below the L2.
		inside := string(mover.Spec.Inside.Find(local))
		if !strings.HasPrefix(inside, name+"."+lowerMemory+"[") {
			t.Fatalf("%s writes inside to %s", mover.Name(), inside)
		}

		if got := mover.Spec.Outside.Find(remote); got != messaging.RemotePort(
			peer+".RDMA.MigrationOutside") {
			t.Fatalf("%s reads outside from %s", mover.Name(), got)
		}
	}

	// The RDMA engine passes the page copies of the other GPU to the same
	// memory below the L2, not to the L2 as it does with remote accesses.
	below := string(rdmaSpec.MigrationModules.Find(local))
	if !strings.HasPrefix(below, name+"."+lowerMemory+"[") ||
		!strings.HasPrefix(string(rdmaSpec.LocalModules.Find(local)), name+".L2Cache[") {
		t.Fatalf("%s routes page copies to %s", name, below)
	}

	if cpSpec.InboundDataMover != p.movers[name+".InboundDataMover"].Ports.Top.AsRemote() ||
		cpSpec.OutboundDataMover != p.movers[name+".OutboundDataMover"].Ports.Top.AsRemote() {
		t.Fatalf("%s command processor: %q, %q", name,
			cpSpec.InboundDataMover, cpSpec.OutboundDataMover)
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
