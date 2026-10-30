package driver

import (
	"reflect"
	"testing"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/addrtransprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
)

var migrationAgents = []messaging.RemotePort{
	"GPU[1].CommandProcessor.PageMigration",
	"GPU[2].CommandProcessor.PageMigration",
}

// newMigrationFixture builds a driver with fault-driven migration and no
// software latency, and one unified page, resident on GPU 1.
func newMigrationFixture(
	t *testing.T,
	configure func(*PageMigrationSpec),
) (*fixture, vm.Page) {
	t.Helper()

	f := newConfiguredFixture(t, false, func(s *Spec) {
		s.PageMigration = DefaultPageMigrationSpec()
		s.PageMigration.Policy = PageMigrationFault
		s.PageMigration.SoftwareLatencyNS = 0
		configure(&s.PageMigration)
		s.MigrationAgents = migrationAgents
	})

	ptr := f.r.AllocateUnifiedMemory(f.ctx, 4096)
	page, ok := f.p.Find(f.r.QueuePID(f.q), uint64(ptr))
	if !ok || page.DeviceID != 1 || !page.Unified {
		t.Fatalf("unexpected unified page %+v", page)
	}

	return f, page
}

func (f *fixture) migrationTick() { f.c.Middlewares.Migration.Tick() }

func (f *fixture) fault(page vm.Page, gpu uint64) {
	f.pm.Deliver(messaging.Msg{
		ID:  f.s.NewID(),
		Src: "MMU.Top",
		Dst: f.pm.AsRemote(),
		Payload: addrtransprotocol.PageFault{
			PID: page.PID, VAddr: page.VAddr, DeviceID: gpu, Page: page,
		},
	})
	f.migrationTick()
}

// commands returns the commands the driver sent, of the given payload type.
func (f *fixture) commands(t *testing.T, want any) []messaging.Msg {
	t.Helper()

	var msgs []messaging.Msg

	for {
		msg, ok := f.pm.RetrieveOutgoing()
		if !ok {
			return msgs
		}

		if reflect.TypeOf(msg.Payload) != reflect.TypeOf(want) {
			t.Fatalf("got %T, want %T", msg.Payload, want)
		}

		msgs = append(msgs, msg)
	}
}

// answer answers every command and ticks.
func (f *fixture) answer(msgs []messaging.Msg) {
	for _, msg := range msgs {
		f.pm.Deliver(messaging.Msg{
			ID:      f.s.NewID(),
			Src:     msg.Dst,
			Dst:     msg.Src,
			RspTo:   msg.ID,
			Payload: protocol.GeneralRsp{},
		})
	}

	f.migrationTick()
	f.migrationTick()
}

func destinations(msgs []messaging.Msg) []messaging.RemotePort {
	var dsts []messaging.RemotePort
	for _, m := range msgs {
		dsts = append(dsts, m.Dst)
	}

	return dsts
}

func TestFaultMigratesHoldCopyRemapShootdownRelease(t *testing.T) {
	f, page := newMigrationFixture(t, func(*PageMigrationSpec) {})

	f.fault(page, 2)
	f.migrationTick()

	holds := f.commands(t, protocol.PageHoldReq{})
	if !reflect.DeepEqual(destinations(holds), migrationAgents) {
		t.Fatalf("holds went to %v", destinations(holds))
	}

	if got := holds[0].Payload.(protocol.PageHoldReq).VAddrs; !reflect.DeepEqual(got, []uint64{page.VAddr}) {
		t.Fatalf("held %v", got)
	}

	// The page table changes only after the copy.
	if p, _ := f.p.Find(page.PID, page.VAddr); p.DeviceID != 1 {
		t.Fatal("page remapped before the copy")
	}

	f.answer(holds)
	copies := f.commands(t, protocol.PageCopyReq{})
	if len(copies) != 1 || copies[0].Dst != migrationAgents[1] {
		t.Fatalf("copies %v", copies)
	}

	c := copies[0].Payload.(protocol.PageCopyReq)
	if c.SrcAddress != page.PAddr || c.ByteSize != 4096 || f.r.DeviceIDByPAddr(c.DstAddress) != 2 {
		t.Fatalf("copy %+v", c)
	}

	f.answer(copies)
	if p, _ := f.p.Find(page.PID, page.VAddr); p.DeviceID != 2 || p.PAddr != c.DstAddress {
		t.Fatalf("page not remapped: %+v", p)
	}

	shootdowns := f.commands(t, protocol.PageShootdownReq{})
	if len(shootdowns) != 2 {
		t.Fatalf("%d shootdowns", len(shootdowns))
	}

	f.answer(shootdowns)
	releases := f.commands(t, protocol.PageReleaseReq{})
	if len(releases) != 2 {
		t.Fatalf("%d releases", len(releases))
	}

	f.answer(releases)
	s := f.c.State.PageMigration
	if s.Units[0].Phase != unitIdle || s.Stats.Migrations != 1 || len(s.Pending) != 0 {
		t.Fatalf("unit %+v, stats %+v", s.Units[0], s.Stats)
	}
}

func TestFaultOnALocalPageOnlyReleasesTheFaultingGPU(t *testing.T) {
	f, page := newMigrationFixture(t, func(*PageMigrationSpec) {})

	f.fault(page, 1)
	f.migrationTick()

	releases := f.commands(t, protocol.PageReleaseReq{})
	if !reflect.DeepEqual(destinations(releases), migrationAgents[:1]) {
		t.Fatalf("releases went to %v", destinations(releases))
	}

	f.answer(releases)
	if f.c.State.PageMigration.Stats.Releases != 1 {
		t.Fatal("release not counted")
	}
}

func TestFaultsOnAMigratingUnitAreIgnored(t *testing.T) {
	f, page := newMigrationFixture(t, func(*PageMigrationSpec) {})

	f.fault(page, 2)
	f.migrationTick()
	f.commands(t, protocol.PageHoldReq{})

	f.fault(page, 1)
	f.migrationTick()

	if got := f.c.State.PageMigration.Stats.IgnoredFaults; got != 1 {
		t.Fatalf("ignored %d faults", got)
	}
}

func TestBatchCollectsFaultsUntilTheTimerExpires(t *testing.T) {
	f, page := newMigrationFixture(t, func(s *PageMigrationSpec) {
		s.SoftwareLatencyNS = 5000
	})

	f.fault(page, 2)
	f.fault(page, 2)
	f.migrationTick()

	s := f.c.State.PageMigration
	if len(s.Batch) != 2 {
		t.Fatalf("batch %v", s.Batch)
	}

	if msgs := f.commands(t, protocol.PageHoldReq{}); len(msgs) != 0 {
		t.Fatal("serviced before the software latency")
	}
}

func migrate(t *testing.T, f *fixture, page vm.Page, gpu uint64) {
	t.Helper()

	f.fault(page, gpu)
	f.migrationTick()
	f.answer(f.commands(t, protocol.PageHoldReq{}))
	f.answer(f.commands(t, protocol.PageCopyReq{}))
	f.answer(f.commands(t, protocol.PageShootdownReq{}))
	f.answer(f.commands(t, protocol.PageReleaseReq{}))
}

func TestThrashingPinsAUnitAndThePinExpires(t *testing.T) {
	f, page := newMigrationFixture(t, func(s *PageMigrationSpec) {
		s.ThrashingThreshold = 1
		s.PinNS = 0
	})

	migrate(t, f, page, 2)

	// The time does not advance, so the next migration is within the lapse:
	// the unit is pinned instead, and only the faulting GPU is released.
	page, _ = f.p.Find(page.PID, page.VAddr)
	f.fault(page, 1)
	f.migrationTick()

	releases := f.commands(t, protocol.PageReleaseReq{})
	if !reflect.DeepEqual(destinations(releases), migrationAgents[:1]) {
		t.Fatalf("releases went to %v", destinations(releases))
	}

	if p, _ := f.p.Find(page.PID, page.VAddr); !p.IsPinned || p.DeviceID != 2 {
		t.Fatalf("page not pinned in place: %+v", p)
	}

	f.answer(releases)

	// The pin has expired: the unit is unpinned and its remote translations
	// are shot down on every GPU.
	shootdowns := f.commands(t, protocol.PageShootdownReq{})
	if !reflect.DeepEqual(destinations(shootdowns), migrationAgents) {
		t.Fatalf("unpin shootdowns went to %v", destinations(shootdowns))
	}

	if p, _ := f.p.Find(page.PID, page.VAddr); p.IsPinned {
		t.Fatal("page still pinned")
	}

	f.answer(shootdowns)
	s := f.c.State.PageMigration
	if s.Stats.Pins != 1 || s.Stats.Unpins != 1 || s.Units[0].Phase != unitIdle {
		t.Fatalf("stats %+v, unit %+v", s.Stats, s.Units[0])
	}
}

func TestFaultsBeyondTheQueueAreDropped(t *testing.T) {
	f, page := newMigrationFixture(t, func(s *PageMigrationSpec) {
		s.SoftwareLatencyNS = 5000
		s.BatchSize = 1
	})

	f.fault(page, 2) // the batch
	f.fault(page, 2) // the next batch
	f.fault(page, 1) // dropped

	s := f.c.State.PageMigration
	if len(s.Batch) != 1 || len(s.NextBatch) != 1 || s.Stats.DroppedFaults != 1 {
		t.Fatalf("batch %v, next %v, dropped %d", s.Batch, s.NextBatch,
			s.Stats.DroppedFaults)
	}
}

func newCounterFixture(t *testing.T, configure func(*PageMigrationSpec)) (*fixture, vm.Page) {
	t.Helper()

	return newMigrationFixture(t, func(s *PageMigrationSpec) {
		s.Policy = PageMigrationCounter
		configure(s)
	})
}

func (f *fixture) notify(pAddr uint64, gpu uint64) {
	f.pm.Deliver(messaging.Msg{
		ID:  f.s.NewID(),
		Src: "GPU[2].RDMA.AccessCounter",
		Dst: f.pm.AsRemote(),
		Payload: protocol.AccessCounterNotification{
			PAddr: pAddr, DeviceID: gpu,
		},
	})
	f.migrationTick()
	f.migrationTick()
}

func TestNotificationMigratesTheUnitToTheNotifier(t *testing.T) {
	f, page := newCounterFixture(t, func(*PageMigrationSpec) {})

	f.notify(page.PAddr+0x40, 2)

	holds := f.commands(t, protocol.PageHoldReq{})
	if !reflect.DeepEqual(destinations(holds), migrationAgents) {
		t.Fatalf("holds went to %v", destinations(holds))
	}

	f.answer(holds)
	f.answer(f.commands(t, protocol.PageCopyReq{}))
	f.answer(f.commands(t, protocol.PageShootdownReq{}))
	f.answer(f.commands(t, protocol.PageReleaseReq{}))

	if p, _ := f.p.Find(page.PID, page.VAddr); p.DeviceID != 2 {
		t.Fatalf("page not migrated: %+v", p)
	}

	s := f.c.State.PageMigration.Stats
	if s.Notifications != 1 || s.Migrations != 1 || s.Faults != 0 || s.Releases != 0 {
		t.Fatalf("stats %+v", s)
	}
}

func TestNotificationsThatMoveNothingAreIgnored(t *testing.T) {
	f, page := newCounterFixture(t, func(*PageMigrationSpec) {})

	f.notify(page.PAddr, 1)       // already on the notifier
	f.notify(page.PAddr+1<<30, 2) // no page there

	if msgs := f.commands(t, protocol.PageReleaseReq{}); len(msgs) != 0 {
		t.Fatalf("sent %v", msgs)
	}

	if s := f.c.State.PageMigration.Stats; s.IgnoredNotifications != 2 {
		t.Fatalf("stats %+v", s)
	}
}

func TestThrashingNotificationsPinTheUnit(t *testing.T) {
	f, page := newCounterFixture(t, func(s *PageMigrationSpec) {
		s.ThrashingThreshold = 1
	})

	f.notify(page.PAddr, 2)
	f.answer(f.commands(t, protocol.PageHoldReq{}))
	f.answer(f.commands(t, protocol.PageCopyReq{}))
	f.answer(f.commands(t, protocol.PageShootdownReq{}))
	f.answer(f.commands(t, protocol.PageReleaseReq{}))

	// Within the lapse: the unit is pinned where it is, and nothing is sent.
	page, _ = f.p.Find(page.PID, page.VAddr)
	f.notify(page.PAddr, 1)

	if msgs := f.commands(t, protocol.PageHoldReq{}); len(msgs) != 0 {
		t.Fatalf("sent %v", msgs)
	}

	if p, _ := f.p.Find(page.PID, page.VAddr); !p.IsPinned || p.DeviceID != 2 {
		t.Fatalf("page not pinned in place: %+v", p)
	}

	f.notify(page.PAddr, 1)

	s := f.c.State.PageMigration.Stats
	if s.Pins != 1 || s.IgnoredNotifications != 1 {
		t.Fatalf("stats %+v", s)
	}
}
