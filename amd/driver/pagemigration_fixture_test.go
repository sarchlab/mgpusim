package driver

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/addrtransprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
)

const testPageSize = 4096

// migrationFixture drives the page-migration middleware of a driver with two
// GPUs. It plays the MMU, the RDMA engines, and the command processors: it
// delivers notifications, keeps the commands the driver has sent and that are
// not answered yet, and answers the ones a test chooses.
type migrationFixture struct {
	*fixture
	t        *testing.T
	unsolved []messaging.Msg
}

// newMigrationFixture builds a driver with the default page-migration
// specification, changed by configure.
func newMigrationFixture(
	t *testing.T,
	configure func(*PageMigrationSpec),
) *migrationFixture {
	t.Helper()

	f := newConfiguredFixture(t, false, func(s *Spec) {
		configure(&s.PageMigration)
	})

	return &migrationFixture{fixture: f, t: t}
}

func noChange(*PageMigrationSpec) {}

func noPrefetch(s *PageMigrationSpec) { s.PrefetchThreshold = 0 }

func (f *migrationFixture) stats() PageMigrationStats {
	return f.c.State.PageMigration.Stats
}

func (f *migrationFixture) pid() vm.PID { return f.r.QueuePID(f.q) }

// unified allocates n unified pages in a row; they start on GPU 1.
func (f *migrationFixture) unified(n int) []vm.Page {
	f.t.Helper()

	ptr := uint64(f.r.AllocateUnifiedMemory(f.ctx, uint64(n)*testPageSize))
	pages := make([]vm.Page, n)

	for i := range pages {
		pages[i] = f.page(ptr + uint64(i)*testPageSize)
		if pages[i].DeviceID != 1 || !pages[i].Unified {
			f.t.Fatalf("unexpected unified page %+v", pages[i])
		}
	}

	return pages
}

// page returns the page of vAddr as the page table has it now.
func (f *migrationFixture) page(vAddr uint64) vm.Page {
	f.t.Helper()

	p, ok := f.p.Find(f.pid(), vAddr)
	if !ok {
		f.t.Fatalf("no page at %#x", vAddr)
	}

	return p
}

// place moves a page to a GPU in the page table only, as if it had migrated.
func (f *migrationFixture) place(page vm.Page, gpu uint64) {
	page.DeviceID = gpu
	f.p.Update(page)
}

func (f *migrationFixture) tick() { f.c.Middlewares.Migration.Tick() }

// ticks ticks until the middleware has nothing to do.
func (f *migrationFixture) ticks() {
	for range 100 {
		if !f.c.Middlewares.Migration.Tick() {
			return
		}
	}

	f.t.Fatal("the page-migration middleware never comes to rest")
}

// deliverFault puts the demand fault of a GPU on a page in the port.
func (f *migrationFixture) deliverFault(page vm.Page, gpu uint64) {
	f.pn.Deliver(messaging.Msg{
		ID:  f.s.NewID(),
		Src: "MMU.Notification[0]",
		Dst: f.pn.AsRemote(),
		Payload: addrtransprotocol.PageFault{
			PID: page.PID, VAddr: page.VAddr, DeviceID: gpu, Page: page,
		},
	})
}

// fault delivers a demand fault and lets the driver service it.
func (f *migrationFixture) fault(page vm.Page, gpu uint64) {
	f.deliverFault(page, gpu)
	f.ticks()
}

// deliverNotification puts the access-counter notification of a GPU for a
// physical address in the port.
func (f *migrationFixture) deliverNotification(pAddr uint64, gpu uint64) {
	f.pn.Deliver(messaging.Msg{
		ID:  f.s.NewID(),
		Src: messaging.RemotePort(fmt.Sprintf("GPU[%d].RDMA.AccessCounter", gpu)),
		Dst: f.pn.AsRemote(),
		Payload: protocol.AccessCounterNotification{
			PAddr: pAddr, DeviceID: gpu,
		},
	})
}

// notify delivers an access-counter notification and lets the driver
// service it.
func (f *migrationFixture) notify(pAddr uint64, gpu uint64) {
	f.deliverNotification(pAddr, gpu)
	f.ticks()
}

// collect takes the commands the driver has sent out of the port.
func (f *migrationFixture) collect() {
	for {
		msg, ok := f.pm.RetrieveOutgoing()
		if !ok {
			return
		}

		f.unsolved = append(f.unsolved, msg)
	}
}

// isOf reports whether a command belongs to a page of a process.
func (f *migrationFixture) isOf(msg messaging.Msg, page vm.Page) bool {
	s := f.c.State.PageMigration

	pi, ok := s.Pending[msg.ID]
	if !ok {
		f.t.Fatalf("command %d is not pending", msg.ID)
	}

	return s.Pages[pi].PID == page.PID && s.Pages[pi].VAddr == page.VAddr
}

// of returns the commands of a page that are not answered yet, in the order
// the driver sent them.
func (f *migrationFixture) of(page vm.Page) []messaging.Msg {
	f.collect()

	var msgs []messaging.Msg

	for _, msg := range f.unsolved {
		if f.isOf(msg, page) {
			msgs = append(msgs, msg)
		}
	}

	return msgs
}

// run lets the driver tick for as long as it makes progress. Nothing else
// wakes it: the fixture answers no command by itself.
func (f *migrationFixture) run() {
	f.t.Helper()

	f.c.TickLater()

	if err := f.s.Engine().(*timing.SerialEngine).Run(); err != nil {
		f.t.Fatal(err)
	}
}

// outstanding returns every command that is not answered yet.
func (f *migrationFixture) outstanding() []messaging.Msg {
	f.collect()
	return slices.Clone(f.unsolved)
}

// answer answers commands and lets the driver react.
func (f *migrationFixture) answer(msgs ...messaging.Msg) {
	for _, msg := range msgs {
		f.unsolved = slices.DeleteFunc(f.unsolved, func(m messaging.Msg) bool {
			return m.ID == msg.ID
		})
		f.pm.Deliver(messaging.Msg{
			ID:      f.s.NewID(),
			Src:     msg.Dst,
			Dst:     msg.Src,
			RspTo:   msg.ID,
			Payload: protocol.GeneralRsp{},
		})
	}

	f.ticks()
}

// step answers every outstanding command of a page and returns what the
// commands were, as describe prints them.
func (f *migrationFixture) step(page vm.Page) []string {
	msgs := f.of(page)
	f.answer(msgs...)

	return describe(msgs)
}

// finish answers the commands of a page until its service ends.
func (f *migrationFixture) finish(page vm.Page) {
	f.t.Helper()

	for range 20 {
		if len(f.step(page)) == 0 {
			return
		}
	}

	f.t.Fatalf("the service of page %#x does not end", page.VAddr)
}

// finishAll answers every command until the driver sends no more.
func (f *migrationFixture) finishAll() {
	f.t.Helper()

	for range 200 {
		msgs := f.outstanding()
		if len(msgs) == 0 {
			return
		}

		f.answer(msgs...)
	}

	f.t.Fatal("the driver keeps sending commands")
}

// migrate faults a page to a GPU and runs the service to its end.
func (f *migrationFixture) migrate(page vm.Page, gpu uint64) {
	f.t.Helper()

	f.fault(page, gpu)
	f.finishAll()

	if got := f.page(page.VAddr).DeviceID; got != gpu {
		f.t.Fatalf("page %#x is on GPU %d, want %d", page.VAddr, got, gpu)
	}
}

// advanceTo runs the simulation to the given time, where the driver ticks.
func (f *migrationFixture) advanceTo(ns uint64) {
	f.t.Helper()

	f.c.TickAt(nsToPS(ns))

	if err := f.s.Engine().(*timing.SerialEngine).RunUntil(nsToPS(ns)); err != nil {
		f.t.Fatal(err)
	}

	f.ticks()
}

// runTo lets simulated time pass until the given time. Unlike advanceTo, it
// does not tick the driver there: the driver ticks only where it has asked
// for a tick itself.
func (f *migrationFixture) runTo(ns uint64) {
	f.t.Helper()

	if err := f.s.Engine().(*timing.SerialEngine).RunUntil(nsToPS(ns)); err != nil {
		f.t.Fatal(err)
	}
}

// describe prints commands as "<what>><GPU>", for example "Hold>2" for a
// PageHoldReq to the command processor of GPU 2.
func describe(msgs []messaging.Msg) []string {
	var out []string

	for _, msg := range msgs {
		what := reflect.TypeOf(msg.Payload).Name()
		what = strings.TrimSuffix(strings.TrimPrefix(what, "Page"), "Req")

		var gpu int
		if _, err := fmt.Sscanf(string(msg.Dst), "GPU[%d]", &gpu); err != nil {
			panic(err)
		}

		out = append(out, fmt.Sprintf("%s>%d", what, gpu))
	}

	return out
}

func mustEqual[T any](t *testing.T, what string, got, want T) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s: got %v, want %v", what, got, want)
	}
}
