package pagemigration

import (
	"testing"

	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

// runUntil runs the simulation, a microsecond at a time, until cond holds.
func (s *system) runUntil(what string, cond func() bool) {
	s.t.Helper()

	const (
		step  = 1_000_000 // 1 us
		limit = 10_000    // 10 ms
	)

	at := s.engine.CurrentTime()

	for i := 0; !cond(); i++ {
		if i == limit {
			s.t.Fatalf("%s never happens; migration: %+v", what, s.stats())
		}

		at += step

		if err := s.engine.RunUntil(at); err != nil {
			s.t.Fatal(err)
		}
	}
}

// passTime lets simulated time pass while nothing may happen.
func (s *system) passTime(ps timing.VTimeInPicoSec) {
	s.t.Helper()

	until := s.engine.CurrentTime() + ps
	s.driver.TickAt(until)

	if err := s.engine.RunUntil(until); err != nil {
		s.t.Fatal(err)
	}
}

// napEnd returns the end of the nap of a GPU that is throttled on the page
// at ptr.
func (s *system) napEnd(ptr gpuruntime.Ptr, gpu int) (timing.VTimeInPicoSec, bool) {
	s.t.Helper()

	page := s.page(ptr)

	for _, p := range s.driver.State.PageMigration.Pages {
		if p.PID != page.PID || p.VAddr != page.VAddr {
			continue
		}

		for _, nap := range p.Throttled {
			if nap.GPU == gpu {
				return nap.Until, true
			}
		}
	}

	return 0, false
}

// lastMigration returns the destination of the last migration of the page at
// ptr that has started, and when it started.
func (s *system) lastMigration(ptr gpuruntime.Ptr) (dst int, start timing.VTimeInPicoSec) {
	s.t.Helper()

	page := s.page(ptr)

	for _, p := range s.driver.State.PageMigration.Pages {
		if p.PID == page.PID && p.VAddr == page.VAddr {
			return p.Dst, p.ServiceStart
		}
	}

	s.t.Fatalf("the page at %#x never migrated", ptr)

	return 0, 0
}

// mustLeaveAt runs the simulation to the given time and checks that the page
// at ptr starts to migrate to a GPU exactly then: not a picosecond before,
// and in the tick of the driver at that time.
func (s *system) mustLeaveAt(at timing.VTimeInPicoSec, ptr gpuruntime.Ptr, gpu int) {
	s.t.Helper()

	if err := s.engine.RunUntil(at - 1); err != nil {
		s.t.Fatal(err)
	}

	if dst, start := s.lastMigration(ptr); dst == gpu {
		s.t.Fatalf("the page left for GPU %d at %d ps, before %d ps", gpu, start, at)
	}

	if err := s.engine.RunUntil(at); err != nil {
		s.t.Fatal(err)
	}

	if dst, start := s.lastMigration(ptr); dst != gpu || start != at {
		s.t.Fatalf("at %d ps the last migration of the page is to GPU %d, from %d ps; want GPU %d",
			at, dst, start, gpu)
	}
}

// Three GPUs with real kernels. A page of GPU 1 thrashes; a kernel of GPU 2
// reads it and is throttled, and half a nap later a kernel of GPU 3 reads it
// and is throttled as well. Each kernel waits its own nap: the page leaves
// for GPU 2 exactly when the nap of GPU 2 ends, which the request of GPU 3
// does not postpone, and for GPU 3 exactly when the nap of GPU 3 ends.
//
//nolint:funlen // Keep the two staggered kernels together.
func TestStaggeredKernelsEachWaitForTheirOwnNap(t *testing.T) {
	const napPS = 400_000_000 // 400 us

	t.Run("r9nano", func(t *testing.T) {
		s := newSystem(t, "r9nano", gpus(3), migration(func(s *driver.PageMigrationSpec) {
			s.PrefetchThreshold = 0
			s.ThrashingThreshold = 1
			s.ThrashingLapseNS = 1_000_000
			s.ThrashingNapNS = napPS / 1000
		}))

		data := ramp(1, groupFloats)
		shared := s.unified(s.pageSize)
		s.upload(shared, data)

		// The page moves to GPU 2 and back within the lapse, and thrashes.
		mustEqualFloats(t, "first read of GPU 2", s.ping(2, shared), data)
		mustEqualFloats(t, "first read of GPU 1", s.ping(1, shared), data)

		if st := s.stats(); st.ThrashingPages != 1 || st.Throttles != 0 {
			t.Fatalf("the page is not detected as thrashing: %+v", st)
		}

		read2 := s.local(2, groupFloats*floatBytes)
		read3 := s.local(3, groupFloats*floatBytes)

		// The kernel of GPU 2 asks for the page first.
		s.touch(2, read2, shared, groupFloats)
		s.start()
		s.runUntil("the throttling of GPU 2", func() bool { return s.stats().Throttles == 1 })

		nap2, _ := s.napEnd(shared, 2)

		// The kernel of GPU 3 starts half a nap later.
		s.passTime(napPS / 2)
		s.touch(3, read3, shared, groupFloats)
		s.start()
		s.runUntil("the throttling of GPU 3", func() bool { return s.stats().Throttles == 2 })

		nap3, _ := s.napEnd(shared, 3)
		now := s.engine.CurrentTime()

		if still, throttled := s.napEnd(shared, 2); !throttled || still != nap2 ||
			now >= nap2 || nap3 < nap2+napPS/2 || nap3 > now+napPS {
			t.Fatalf("at %d ps the nap of GPU 2 ends at %d ps (was %d ps), that of GPU 3 at %d ps",
				now, still, nap2, nap3)
		}

		// Each migration starts when the nap of its GPU ends, to the
		// picosecond: the driver wakes for each nap by itself.
		s.mustLeaveAt(nap2, shared, 2)
		s.mustLeaveAt(nap3, shared, 3)

		// Both kernels end, and each has read the page.
		s.run()
		mustEqualFloats(t, "what GPU 2 read", s.download(read2, groupFloats), data)
		mustEqualFloats(t, "what GPU 3 read", s.download(read3, groupFloats), data)
		mustEqualOwners(t, "owner", s.owners(shared, 1), 3)

		st := s.stats()
		if st.Throttles != 2 || st.Migrations != 4 || st.Pins != 0 {
			t.Fatalf("stats %+v", st)
		}

		t.Logf("the naps of GPU 2 and GPU 3 ended at %d ps and %d ps: %+v", nap2, nap3, st)
	})
}
