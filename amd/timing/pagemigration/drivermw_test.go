package pagemigration

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/mgpusim/v5/amd/timing/rdma"
)

var _ = Describe("Driver middleware", func() {
	const gpuMem = uint64(4 << 30)

	var (
		host      *testComp
		state     DriverState
		mw        *DriverMiddleware
		port      messaging.Port
		pageTable vm.PageTable
		spec      Spec
		page      vm.Page
	)

	cp := func(dev uint64) messaging.RemotePort {
		return messaging.RemotePort([]string{"", "GPU1.CP", "GPU2.CP"}[dev])
	}

	build := func() {
		host = newTestHost("Driver", DriverPort)
		port = host.GetPortByName(DriverPort)
		state = NewDriverState()
		mw = NewDriverMiddleware(host, &state, spec, pageTable)
		mw.RegisterGPU(1, cp(1))
		mw.RegisterGPU(2, cp(2))
	}

	BeforeEach(func() {
		pageTable = vm.NewPageTable(12)
		page = vm.Page{PID: 1, VAddr: 0x10000, PAddr: 2*gpuMem + 0x5000,
			PageSize: 4096, DeviceID: 2, Valid: true}
		pageTable.Insert(page)

		spec = DefaultSpec()
		spec.Policy = PolicyOnFault
		spec.SoftwareLatencyNS = 0
		spec.ShootdownLatencyNS = 0
		spec.GPUMemSize = gpuMem
		build()
	})

	fault := func(dev uint64) {
		f := vmprotocol.PageFault{PID: 1, VAddr: 0x10000, DeviceID: dev,
			Page: page}
		f.ID = timing.GetIDGenerator().Generate()
		f.Dst = port.AsRemote()
		port.Deliver(f)
		mw.Tick()
	}

	// answer answers every request the driver sent, and returns them.
	answer := func() []messaging.Msg {
		msgs := drain(port)
		for _, msg := range msgs {
			port.Deliver(rspTo(msg))
		}

		mw.Tick()

		return msgs
	}

	dsts := func(msgs []messaging.Msg) []messaging.RemotePort {
		var d []messaging.RemotePort
		for _, m := range msgs {
			d = append(d, m.Meta().Dst)
		}

		return d
	}

	It("migrates a page on a fault: hold, copy, remap, shootdown, release",
		func() {
			fault(1)

			holds := answer()
			Expect(holds).To(HaveLen(2))
			Expect(holds[0]).To(BeAssignableToTypeOf(HoldReq{}))
			Expect(holds[0].(HoldReq).VAddrs).To(Equal([]uint64{0x10000}))
			Expect(dsts(holds)).To(ConsistOf(cp(1), cp(2)))

			// The page table changes only after the copy.
			p, _ := pageTable.Find(1, 0x10000)
			Expect(p.DeviceID).To(Equal(uint64(2)))

			copies := answer()
			Expect(copies).To(HaveLen(1))
			c := copies[0].(CopyReq)
			Expect(c.Dst).To(Equal(cp(1)))
			Expect(c.SrcPAddr).To(Equal(page.PAddr))
			Expect(c.DstPAddr).To(Equal(gpuMem + spec.FrameRegionOffset))
			Expect(c.Size).To(Equal(uint64(4096)))

			p, _ = pageTable.Find(1, 0x10000)
			Expect(p.DeviceID).To(Equal(uint64(1)))
			Expect(p.PAddr).To(Equal(c.DstPAddr))

			shootdowns := answer()
			Expect(shootdowns).To(HaveLen(2))
			Expect(shootdowns[0]).To(BeAssignableToTypeOf(ShootdownReq{}))

			releases := answer()
			Expect(releases).To(HaveLen(2))
			Expect(releases[0]).To(BeAssignableToTypeOf(ReleaseReq{}))
			Expect(dsts(releases)).To(ConsistOf(cp(1), cp(2)))

			Expect(state.Units[0].Phase).To(Equal(phaseIdle))
			Expect(state.Stats.Migrations).To(Equal(uint64(1)))
			Expect(state.Stats.PagesMigrated).To(Equal(uint64(1)))
			Expect(state.Pending).To(BeEmpty())
		})

	It("waits for the shootdown latency before the release", func() {
		spec.ShootdownLatencyNS = 1000
		build()

		fault(1)
		answer() // holds
		answer() // copy
		answer() // shootdowns

		Expect(drain(port)).To(BeEmpty())
		Expect(state.Units[0].Phase).To(Equal(phaseShootdown))
		Expect(mw.Tick()).To(BeTrue())
	})

	It("handles a fault from another GPU after the unit is idle again",
		func() {
			fault(1)
			fault(2) // the service of GPU 1 has started

			answer()
			answer()
			answer()
			answer() // releases; the late fault is handled

			Expect(state.Units[0].Faulters).To(Equal([]uint64{2}))
			Expect(state.Units[0].Phase).NotTo(Equal(phaseIdle))
		})

	It("maps a unit remotely on a fault in its cooldown", func() {
		fault(1)
		answer()
		answer()
		answer()
		answer()

		// The time does not advance, so the unit is in its cooldown.
		fault(2)

		holds := answer()
		Expect(holds).To(HaveLen(1))
		Expect(holds[0].Meta().Dst).To(Equal(cp(2)))

		p, _ := pageTable.Find(1, 0x10000)
		Expect(p.IsPinned).To(BeTrue())
		Expect(p.DeviceID).To(Equal(uint64(1)))

		releases := answer()
		Expect(releases).To(HaveLen(1))
		Expect(releases[0]).To(BeAssignableToTypeOf(ReleaseReq{}))
		Expect(state.Stats.RemoteMaps).To(Equal(uint64(1)))
		Expect(state.RemoteMaps).To(HaveLen(1))
	})

	It("pins a unit for good after MaxMigrations", func() {
		spec.MaxMigrations = 1
		build()

		fault(1)
		answer()
		answer()

		p, _ := pageTable.Find(1, 0x10000)
		Expect(p.IsPinned).To(BeTrue())
		Expect(state.Stats.Pinned).To(Equal(uint64(1)))
	})

	It("migrates on an access-counter notification", func() {
		spec.Policy = PolicyAccessCounter
		build()

		n := rdma.AccessCounterNotification{PAddr: page.PAddr, DeviceID: 1,
			Count: 256}
		n.ID = timing.GetIDGenerator().Generate()
		port.Deliver(n)
		mw.Tick()

		holds := answer()
		Expect(holds).To(HaveLen(2))
		Expect(state.Stats.CounterTriggers).To(Equal(uint64(1)))

		copies := answer()
		Expect(copies[0].Meta().Dst).To(Equal(cp(1)))
	})

	It("ignores the notifications with the observe policy", func() {
		spec.Policy = PolicyObserve
		build()

		n := rdma.AccessCounterNotification{PAddr: page.PAddr, DeviceID: 1}
		n.ID = timing.GetIDGenerator().Generate()
		port.Deliver(n)
		mw.Tick()

		Expect(drain(port)).To(BeEmpty())
		Expect(state.Stats.CounterNotifications).To(Equal(uint64(1)))
	})
})
