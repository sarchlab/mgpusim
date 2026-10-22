package pagemigration

import (
	"bytes"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/addresstranslator"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/timing"
)

const (
	testMemSize = uint64(1) << 30
	us          = timing.VTimeInPicoSec(1_000_000)
)

// fakeTLBSpec, fakeTLBState, and fakeTLBMW form a TLB that only speaks the
// control protocol. It records the pages it is asked to invalidate and
// rejects an Invalidate while it is not paused, like a real TLB.
type fakeTLBSpec struct {
	Freq timing.Freq `json:"freq"`
}

type fakeTLBState struct {
	Paused  bool     `json:"paused"`
	Dropped []uint64 `json:"dropped"`
}

type fakeTLB = modeling.Component[fakeTLBSpec, fakeTLBState, modeling.None]

type fakeTLBMW struct {
	comp *fakeTLB
}

func (m *fakeTLBMW) Tick() bool {
	port := m.comp.GetPortByName("Control")

	msg := port.PeekIncoming()
	if msg == nil || !port.CanSend() {
		return false
	}

	req := msg.(memcontrolprotocol.Req)
	rsp := memcontrolprotocol.Rsp{
		MsgMeta: messaging.MsgMeta{
			ID:    timing.GetIDGenerator().Generate(),
			Src:   port.AsRemote(),
			Dst:   req.Src,
			RspTo: req.ID,
		},
		Command: req.Command,
		Success: true,
	}

	state := &m.comp.State

	switch req.Command {
	case memcontrolprotocol.CmdPause:
		state.Paused = true
	case memcontrolprotocol.CmdEnable:
		state.Paused = false
	case memcontrolprotocol.CmdInvalidate:
		if state.Paused {
			state.Dropped = append(state.Dropped, req.Addresses...)
		} else {
			rsp.Success = false
			rsp.Error = memcontrolprotocol.ErrMustBePausedOrDrained
		}
	}

	port.Send(rsp)
	port.RetrieveIncoming()

	return true
}

// fakeWaker stands for an address translator.
type fakeWaker struct {
	name   string
	engine timing.Engine
	woken  []timing.VTimeInPicoSec
}

func (w *fakeWaker) Name() string { return w.name }

func (w *fakeWaker) TickLater() {
	w.woken = append(w.woken, w.engine.CurrentTime())
}

// callbacks runs functions at given times.
type callbacks struct {
	fns map[uint64]func()
}

func (c *callbacks) Handle(e timing.Event) error {
	c.fns[e.(timing.EventBase).ID]()
	return nil
}

type testRig struct {
	engine  *timing.SerialEngine
	storage *mem.Storage
	pt      vm.PageTable
	tlbs    []*fakeTLB // of GPU 1 and GPU 2
	m       *Manager
	cb      *callbacks
}

func testSpec(p Policy) Spec {
	s := DefaultSpec()
	s.Policy = p
	s.SoftwareLatencyNS = 100
	s.ShootdownLatencyNS = 50
	s.CooldownNS = 0
	s.MaxMigrations = 0
	s.GPUMemSize = testMemSize
	s.FrameRegionOffset = testMemSize / 4 * 3
	s.FrameRegionSize = testMemSize / 4

	return s
}

func plug(
	reg modeling.Registrar,
	conn *directconnection.Comp,
	comp messaging.Component,
	name string,
	bufSize int,
) {
	port := modeling.MakePortBuilder().
		WithRegistrar(reg).
		WithComponent(comp).
		WithSpec(modeling.PortSpec{BufSize: bufSize}).
		Build(name)
	comp.AssignPort(name, port)
	conn.PlugIn(port)
}

// newRig builds two GPUs (device IDs 1 and 2). The L2 and the DRAM of each
// GPU are one ideal memory controller on a shared storage.
func newRig(spec Spec) *testRig {
	r := &testRig{
		engine:  timing.NewSerialEngine(),
		storage: mem.NewStorage(3 * testMemSize),
		pt:      vm.NewPageTable(12),
		cb:      &callbacks{fns: make(map[uint64]func())},
	}
	r.engine.RegisterHandler("Callbacks", r.cb)

	reg := modeling.NewStandaloneRegistrar(r.engine)
	connSpec := directconnection.Spec{Freq: 1 * timing.GHz}
	net := directconnection.MakeBuilder().WithRegistrar(reg).
		WithSpec(connSpec).Build("Net")

	r.m = MakeManagerBuilder().
		WithRegistrar(reg).
		WithSpec(spec).
		WithResources(ManagerResources{PageTable: r.pt}).
		Build("PageMigrationManager")

	if spec.Migrates() {
		plug(reg, net, r.m, "ToEngines", 64)
	}

	for dev := uint64(1); dev <= 2; dev++ {
		r.addGPU(reg, net, spec, dev)
	}

	return r
}

func (r *testRig) addGPU(
	reg modeling.Registrar,
	net *directconnection.Comp,
	spec Spec,
	dev uint64,
) {
	connSpec := directconnection.Spec{Freq: 1 * timing.GHz}

	tlb := modeling.NewBuilder[fakeTLBSpec, fakeTLBState, modeling.None]().
		WithEngine(r.engine).
		WithFreq(1 * timing.GHz).
		WithSpec(fakeTLBSpec{Freq: 1 * timing.GHz}).
		Build(fmt.Sprintf("GPU[%d].TLB", dev))
	tlb.AddMiddleware(&fakeTLBMW{comp: tlb})
	tlb.DeclarePort("Control")
	r.tlbs = append(r.tlbs, tlb)

	g := GPU{DeviceID: dev}

	if spec.Migrates() {
		memSpec := idealmemcontroller.DefaultSpec()
		memSpec.Latency = 20
		memSpec.Capacity = 3 * testMemSize
		ctrl := idealmemcontroller.MakeBuilder().
			WithRegistrar(reg).
			WithSpec(memSpec).
			WithResources(idealmemcontroller.Resources{Storage: r.storage}).
			Build(fmt.Sprintf("GPU[%d].Mem", dev))

		engineSpec := spec.EngineSpec()
		engineSpec.TLBs = []messaging.RemotePort{
			messaging.RemotePort(fmt.Sprintf("GPU[%d].TLB.Control", dev)),
		}
		mapper := &mem.SinglePortMapper{
			Port: messaging.RemotePort(fmt.Sprintf("GPU[%d].Mem.Top", dev)),
		}
		e := MakeEngineBuilder().
			WithRegistrar(reg).
			WithSpec(engineSpec).
			WithResources(EngineResources{L2Mapper: mapper, DRAMMapper: mapper}).
			Build(fmt.Sprintf("GPU[%d].PageMigrationEngine", dev))

		memConn := directconnection.MakeBuilder().WithRegistrar(reg).
			WithSpec(connSpec).Build(fmt.Sprintf("GPU[%d].MemConn", dev))
		plug(reg, memConn, ctrl, "Top", 64)
		plug(reg, memConn, ctrl, "Control", 1)
		plug(reg, memConn, e, "L2", 64)
		plug(reg, memConn, e, "DRAM", 64)
		plug(reg, memConn, tlb, "Control", 1)
		plug(reg, memConn, e, "TLBControl", 4)
		plug(reg, net, e, "Remote", 64)

		g.Engine = e
	}

	r.m.RegisterGPU(g)
}

func (r *testRig) addPage(vAddr, pAddr, dev uint64, fill byte) {
	r.pt.Insert(vm.Page{PID: 1, VAddr: vAddr, PAddr: pAddr, PageSize: 4096,
		Valid: true, DeviceID: dev})

	data := bytes.Repeat([]byte{fill}, 4096)
	for i := range data {
		data[i] += byte(i)
	}

	Expect(r.storage.Write(pAddr, data)).To(Succeed())
}

func (r *testRig) at(t timing.VTimeInPicoSec, f func()) {
	e := timing.MakeEventBase(t, "Callbacks")
	r.cb.fns[e.ID] = f
	r.engine.Schedule(e)
}

func (r *testRig) run() {
	Expect(r.engine.Run()).To(Succeed())
}

func (r *testRig) page(vAddr uint64) vm.Page {
	p, ok := r.pt.Find(1, vAddr)
	Expect(ok).To(BeTrue())

	return p
}

func (r *testRig) read(pAddr uint64) []byte {
	data, err := r.storage.Read(pAddr, 4096)
	Expect(err).NotTo(HaveOccurred())

	return data
}

func (r *testRig) waker(name string) *fakeWaker {
	return &fakeWaker{name: name, engine: r.engine}
}

func (r *testRig) remoteRead(dev uint64, pAddr uint64) {
	req := memprotocol.ReadReq{Address: pAddr, AccessByteSize: 64}
	r.m.mw.remoteAccess(dev, req)
}

func expectViolation(f func()) {
	defer func() {
		_, ok := recover().(Violation)
		Expect(ok).To(BeTrue(), "expected an invariant violation")
	}()

	f()
}

var _ = Describe("Manager", func() {
	gpuBase := func(dev uint64) uint64 { return dev * testMemSize }
	firstFrame := func(dev uint64) uint64 {
		return gpuBase(dev) + testMemSize/4*3
	}

	It("migrates on a fault after the drain and copies the data", func() {
		r := newRig(testSpec(PolicyOnFault))
		src := gpuBase(1) + 0x1000
		r.addPage(0x10000, src, 1, 7)
		h1 := r.m.ATHook(1, true)
		h2 := r.m.ATHook(2, true)
		w := r.waker("GPU[2].AT")

		// GPU 1 has an access to the page in flight; it completes at 1 us.
		h1.Issued(1, 0x10040, src+0x40)
		r.at(us, func() { h1.Completed(src + 0x40) })

		held := false
		r.at(0, func() {
			_, ok := h2.Resolve(w, 1, 0x10080, r.page(0x10000))
			held = !ok
		})
		r.run()

		Expect(held).To(BeTrue())

		p := r.page(0x10000)
		Expect(p.DeviceID).To(Equal(uint64(2)))
		Expect(p.PAddr).To(Equal(firstFrame(2)))
		Expect(r.read(p.PAddr)).To(Equal(r.read(src)))

		s := r.m.Stats()
		Expect(s.Faults).To(Equal(uint64(1)))
		Expect(s.Migrations).To(Equal(uint64(1)))
		Expect(s.PagesMigrated).To(Equal(uint64(1)))
		Expect(s.BytesMigrated).To(Equal(uint64(4096)))
		Expect(s.Shootdowns).To(Equal(uint64(1)))
		Expect(s.DrainWaitSum).To(BeNumerically(">", 0),
			"the migration must wait for the access in flight")

		// The shootdown reaches the GPUs that accessed the page, through
		// the control protocol (the fake TLB rejects an unpaused Invalidate).
		Expect(r.tlbs[0].State.Dropped).To(Equal([]uint64{0x10000}))
		Expect(r.tlbs[0].State.Paused).To(BeFalse())
		Expect(r.tlbs[1].State.Dropped).To(BeEmpty())

		Expect(w.woken).To(HaveLen(1))
		Expect(w.woken[0]).To(BeNumerically(">=", us))

		// The faulting GPU now resolves a stale TLB copy to the new frame.
		np, ok := h2.Resolve(w, 1, 0x10080, vm.Page{PID: 1, VAddr: 0x10000,
			PAddr: src, DeviceID: 1})
		Expect(ok).To(BeTrue())
		Expect(np.PAddr).To(Equal(p.PAddr))
	})

	It("pins a unit at the cap and never faults from scalar translators", func() {
		spec := testSpec(PolicyOnFault)
		spec.MaxMigrations = 1
		r := newRig(spec)
		r.addPage(0x10000, gpuBase(1), 1, 1)
		h1 := r.m.ATHook(1, true)
		h2 := r.m.ATHook(2, true)
		w := r.waker("AT")

		r.at(0, func() { h2.Resolve(w, 1, 0x10000, r.page(0x10000)) })
		r.run()

		// Pinned on GPU 2: GPU 1's access goes remote without a fault.
		p, ok := h1.Resolve(w, 1, 0x10000, r.page(0x10000))
		Expect(ok).To(BeTrue())
		Expect(p.DeviceID).To(Equal(uint64(2)))
		Expect(r.m.Stats().Faults).To(Equal(uint64(1)))
		Expect(r.m.Stats().Pinned).To(Equal(uint64(1)))

		r2 := newRig(testSpec(PolicyOnFault))
		r2.addPage(0x10000, gpuBase(1), 1, 1)
		_, ok = r2.m.ATHook(2, false).Resolve(w, 1, 0x10000, r2.page(0x10000))
		Expect(ok).To(BeTrue())
		Expect(r2.m.Stats().Faults).To(BeZero())
	})

	It("does not migrate pages that are not on a GPU", func() {
		r := newRig(testSpec(PolicyOnFault))
		r.addPage(0x10000, 0x1000, 0, 1) // in the memory of the CPU
		w := r.waker("AT")

		_, ok := r.m.ATHook(2, true).Resolve(w, 1, 0x10000, r.page(0x10000))
		Expect(ok).To(BeTrue())
		Expect(r.m.Stats().Faults).To(BeZero())
	})

	It("migrates to the GPU with the most accesses at the threshold", func() {
		spec := testSpec(PolicyAccessCounter)
		spec.Threshold = 4
		r := newRig(spec)
		src := gpuBase(1) + 0x3000
		r.addPage(0x20000, src, 1, 3)
		h2 := r.m.ATHook(2, true)
		h2.Issued(1, 0x20000, src)
		h2.Completed(src)
		w := r.waker("AT")

		r.at(0, func() {
			for range 3 {
				r.remoteRead(2, src)
			}

			Expect(r.m.Stats().CounterTriggers).To(BeZero())

			// Asynchronous: accesses are not held before the shootdown.
			_, ok := h2.Resolve(w, 1, 0x20000, r.page(0x20000))
			Expect(ok).To(BeTrue())

			r.remoteRead(2, src)
		})
		r.run()

		p := r.page(0x20000)
		Expect(p.DeviceID).To(Equal(uint64(2)))
		Expect(r.m.Stats().CounterTriggers).To(Equal(uint64(1)))
		Expect(r.m.Stats().Migrations).To(Equal(uint64(1)))
		Expect(r.m.Stats().RemoteReadsByOwner[1]).To(Equal(uint64(4)))

		// GPU 1 now reads it remotely: it moves back, a ping-pong.
		h1 := r.m.ATHook(1, true)
		now := r.engine.CurrentTime()
		r.at(now, func() {
			h1.Issued(1, 0x20000, p.PAddr)
			h1.Completed(p.PAddr)

			for range 4 {
				r.remoteRead(1, p.PAddr)
			}
		})
		r.run()

		Expect(r.page(0x20000).DeviceID).To(Equal(uint64(1)))
		Expect(r.m.Stats().PingPong).To(Equal(uint64(1)))
		Expect(r.read(r.page(0x20000).PAddr)).To(Equal(r.read(src)))
	})

	It("moves the whole group of a 2 MB unit", func() {
		spec := testSpec(PolicyOnFault)
		spec.Log2Unit = 21
		r := newRig(spec)
		r.addPage(0x200000, gpuBase(1), 1, 1)
		r.addPage(0x201000, gpuBase(1)+0x1000, 1, 2)
		r.addPage(0x3ff000, gpuBase(2), 2, 3)        // already on GPU 2
		r.addPage(0x400000, gpuBase(1)+0x2000, 1, 4) // next unit
		h2 := r.m.ATHook(2, true)
		w := r.waker("AT")

		r.at(0, func() { h2.Resolve(w, 1, 0x201010, r.page(0x201000)) })
		r.run()

		for _, va := range []uint64{0x200000, 0x201000, 0x3ff000} {
			Expect(r.page(va).DeviceID).To(Equal(uint64(2)))
		}

		Expect(r.page(0x3ff000).PAddr).To(Equal(gpuBase(2)))
		Expect(r.page(0x400000).DeviceID).To(Equal(uint64(1)))
		Expect(r.m.Stats().PagesMigrated).To(Equal(uint64(2)))
		Expect(r.m.Stats().Migrations).To(Equal(uint64(1)))
	})

	It("never migrates in observe mode", func() {
		r := newRig(testSpec(PolicyObserve))
		r.addPage(0x10000, gpuBase(1), 1, 1)
		h2 := r.m.ATHook(2, true)
		w := r.waker("AT")
		h2.Issued(1, 0x10000, gpuBase(1))

		for range 1000 {
			r.remoteRead(2, gpuBase(1))
		}

		_, ok := h2.Resolve(w, 1, 0x10000, r.page(0x10000))
		Expect(ok).To(BeTrue())

		r.run()

		Expect(r.m.Stats().Migrations).To(BeZero())
		Expect(r.m.Stats().RemoteReads).To(Equal(uint64(1000)))
		Expect(r.page(0x10000).DeviceID).To(Equal(uint64(1)))
	})

	It("parses policies and validates the configuration", func() {
		for in, want := range map[string]Policy{
			"none":    PolicyNone,
			"fault":   PolicyOnFault,
			"counter": PolicyAccessCounter,
			"observe": PolicyObserve,
		} {
			p, err := ParsePolicy(in)
			Expect(err).NotTo(HaveOccurred())
			Expect(p).To(Equal(want))
		}

		_, err := ParsePolicy("bogus")
		Expect(err).To(HaveOccurred())

		s := testSpec(PolicyOnFault)
		s.Log2Unit = 11
		Expect(s.Validate()).NotTo(Succeed())

		s = testSpec(PolicyOnFault)
		s.FrameRegionOffset = testMemSize
		Expect(s.Validate()).NotTo(Succeed())
	})

	// A write in flight to the old frame when the migration starts must land
	// before the copy reads the page (drain), so the new frame has it.
	It("lets a write in flight land before the copy", func() {
		r := newRig(testSpec(PolicyOnFault))
		src := gpuBase(1) + 0x5000
		r.addPage(0x50000, src, 1, 9)
		h1 := r.m.ATHook(1, true)
		h2 := r.m.ATHook(2, true)
		w := r.waker("AT")

		h1.Issued(1, 0x50100, src+0x100) // GPU 1's write, in flight
		r.at(0, func() { h2.Resolve(w, 1, 0x50000, r.page(0x50000)) })
		r.at(3*us, func() {
			// The write is performed at the owner, then acknowledged.
			Expect(r.storage.Write(src+0x100, []byte{0xAB, 0xCD})).To(Succeed())
			h1.Completed(src + 0x100)
		})
		r.run()

		p := r.page(0x50000)
		Expect(p.DeviceID).To(Equal(uint64(2)))
		Expect(r.read(p.PAddr)[0x100:0x102]).To(Equal([]byte{0xAB, 0xCD}))
	})

	// After the remap no access may use the old frame (stale L1 copies of it
	// are unreachable), and frames are never reused.
	It("retires old frames and never reuses frames", func() {
		r := newRig(testSpec(PolicyOnFault))
		src := gpuBase(1) + 0x6000
		r.addPage(0x60000, src, 1, 1)
		h1 := r.m.ATHook(1, true)
		h2 := r.m.ATHook(2, true)
		w := r.waker("AT")

		h1.Issued(1, 0x60000, src)
		h1.Completed(src)
		r.at(0, func() { h2.Resolve(w, 1, 0x60000, r.page(0x60000)) })
		r.run()

		first := r.page(0x60000).PAddr

		expectViolation(func() { h1.Issued(1, 0x60000, src) })
		expectViolation(func() { r.remoteRead(2, src) })

		// Move it back: GPU 1 gets a fresh frame, not its old one.
		np := r.page(0x60000)
		h2.Issued(1, 0x60000, np.PAddr)
		h2.Completed(np.PAddr)
		r.at(r.engine.CurrentTime(), func() { h1.Resolve(w, 1, 0x60000, np) })
		r.run()

		back := r.page(0x60000)
		Expect(back.DeviceID).To(Equal(uint64(1)))
		Expect(back.PAddr).NotTo(Equal(src))
		Expect(back.PAddr).NotTo(Equal(first))
	})

	It("finds a cached line of a new frame with Check", func() {
		spec := testSpec(PolicyOnFault)
		spec.Check = true

		r := newRig(spec)
		r.addPage(0x70000, gpuBase(1)+0x7000, 1, 1)
		newFrame := firstFrame(2)
		r.m.mw.gpus[0].HoldsLine = func(a uint64) bool { return a == newFrame+0x40 }
		h2 := r.m.ATHook(2, true)
		w := r.waker("AT")
		r.at(0, func() { h2.Resolve(w, 1, 0x70000, r.page(0x70000)) })
		expectViolation(func() { _ = r.engine.Run() })

		r2 := newRig(spec)
		r2.addPage(0x70000, gpuBase(1)+0x7000, 1, 1)
		h := r2.m.ATHook(2, true)
		r2.at(0, func() { h.Resolve(w, 1, 0x70000, r2.page(0x70000)) })
		r2.run()

		Expect(r2.m.Stats().CheckedFrames).To(Equal(uint64(2))) // allocation, remap
	})
})

var _ addresstranslator.Waker = &fakeWaker{}
