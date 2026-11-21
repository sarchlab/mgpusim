package pagemigration

import (
	"testing"

	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	gpuruntime "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

// With the access-counter policy, GPU 2 reads the pages of GPU 1 remotely.
// When its accesses to a frame reach the threshold, the page migrates while
// the kernel runs, and, on R9 Nano, it prefetches the pages around it.
func TestCountersMigrateAndPrefetchWhileTheKernelRuns(t *testing.T) {
	for _, gpuType := range platforms {
		t.Run(gpuType, func(t *testing.T) {
			s := newSystem(t, gpuType, migration(func(s *driver.PageMigrationSpec) {
				s.Policy = driver.PageMigrationCounter
				s.AccessCounterThreshold = 16
			}))

			// An aligned group of eight pages on R9 Nano; a part of one page
			// of 2 MiB on MI300X.
			const floats = 8 * 1024

			pages := max(1, floats*floatBytes/s.pageSize)
			shared := s.alignedUnified(pages, 8)
			result := s.local(2, floats*floatBytes)
			data := ramp(1, floats)

			s.upload(shared, data)
			s.touch(2, result, shared, floats)
			s.run()

			mustEqualFloats(t, "what GPU 2 read", s.download(result, floats), data)

			st := s.stats()
			if st.Faults != 0 || st.Notifications == 0 || st.Migrations == 0 {
				t.Fatalf("no page migrated on a notification: %+v", st)
			}

			owners := s.owners(shared, pages)
			if pages == 1 {
				mustEqualOwners(t, "owner", owners, 2)
			} else if st.PagesPrefetched == 0 {
				t.Fatalf("no page was prefetched with a counter-triggered migration: %+v", st)
			}

			t.Logf("%s: owners %v; %+v", gpuType, owners, st)
		})
	}
}

// ping makes a GPU read one group of floats of a page and returns what it
// read.
func (s *system) ping(gpu int, page gpuruntime.Ptr) []float32 {
	s.t.Helper()

	result := s.local(gpu, groupFloats*floatBytes)
	s.touch(gpu, result, page, groupFloats)
	s.run()

	return s.download(result, groupFloats)
}

// Two GPUs take a page from each other: the page is detected as thrashing,
// the GPU that asks for it next is throttled, the page is then pinned where
// it is and read remotely, and the pin times out so that migration resumes.
//
//nolint:funlen // Keep the three stages of one thrashing page together.
func TestAThrashingPageIsThrottledPinnedAndUnpinned(t *testing.T) {
	const (
		napNS = 20_000
		pinNS = 60_000
	)

	for _, gpuType := range platforms {
		t.Run(gpuType, func(t *testing.T) {
			s := newSystem(t, gpuType, migration(func(s *driver.PageMigrationSpec) {
				s.PrefetchThreshold = 0
				s.ThrashingThreshold = 1
				s.ThrashingLapseNS = 1_000_000
				s.ThrashingNapNS = napNS
				s.PinThreshold = 1
				s.PinNS = pinNS
			}))

			data := ramp(1, groupFloats)
			shared := s.unified(s.pageSize)
			s.upload(shared, data)

			// Detection: the page moves to GPU 2 and back within the lapse.
			mustEqualFloats(t, "first read of GPU 2", s.ping(2, shared), data)
			mustEqualFloats(t, "first read of GPU 1", s.ping(1, shared), data)

			st := s.stats()
			if st.Migrations != 2 || st.ThrashingPages != 1 || st.Throttles != 0 {
				t.Fatalf("the page is not detected as thrashing: %+v", st)
			}

			// Throttling: GPU 2 waits for its nap before the page moves.
			start := s.engine.CurrentTime()
			mustEqualFloats(t, "throttled read of GPU 2", s.ping(2, shared), data)

			st = s.stats()
			if st.Throttles != 1 || st.Migrations != 3 || s.page(shared).DeviceID != 2 {
				t.Fatalf("GPU 2 was not throttled and then served: %+v", st)
			}

			if waited := s.engine.CurrentTime() - start; waited < napNS*1000 {
				t.Fatalf("the throttled kernel took %d ps, less than the nap", waited)
			}

			// Pinning: GPU 1 asks next. The page stays on GPU 2, pinned, and
			// GPU 1 reads it remotely.
			mustEqualFloats(t, "remote read of GPU 1", s.ping(1, shared), data)

			st = s.stats()
			pinned := s.page(shared)
			if st.Pins != 1 || st.Migrations != 3 || !pinned.IsPinned || pinned.DeviceID != 2 {
				t.Fatalf("the page is not pinned in place: %+v, %+v", pinned, st)
			}

			// The timeout of the pin is background work: the run ended
			// without waiting for it.
			pinnedUntil := s.driver.State.PageMigration.Pages[0].PinnedUntil
			if s.engine.CurrentTime() >= pinnedUntil || st.Unpins != 0 {
				t.Fatalf("the run waited for the pin: now %d, pinned until %d",
					s.engine.CurrentTime(), pinnedUntil)
			}

			// GPU 1 keeps reading remotely until the pin has timed out.
			s.pingUntil(1, shared, data, pinnedUntil)

			st = s.stats()
			if st.Unpins != 1 || s.page(shared).IsPinned {
				t.Fatalf("the pin did not time out: %+v", st)
			}

			// Migration resumes: the first read of GPU 1 after the timeout
			// faults and takes the page.
			mustEqualFloats(t, "read of GPU 1 after the pin", s.ping(1, shared), data)

			if st = s.stats(); st.Migrations != 4 || s.page(shared).DeviceID != 1 {
				t.Fatalf("migration did not resume after the pin: %+v", st)
			}

			t.Logf("%s: %+v", gpuType, st)
		})
	}
}

// pingUntil makes a GPU read a page again and again until the simulated time
// has passed until and the pin of the page is gone.
func (s *system) pingUntil(gpu int, page gpuruntime.Ptr, want []float32, until timing.VTimeInPicoSec) {
	s.t.Helper()

	for range 200 {
		mustEqualFloats(s.t, "remote read", s.ping(gpu, page), want)

		if s.engine.CurrentTime() > until && s.stats().Unpins > 0 {
			return
		}
	}

	s.t.Fatalf("the pin is still there at %d ps: %+v", s.engine.CurrentTime(), s.stats())
}
