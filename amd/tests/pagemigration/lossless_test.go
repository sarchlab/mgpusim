package pagemigration

import (
	"slices"
	"testing"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/addrtransprotocol"
	"github.com/sarchlab/akita/v5/mem/vm/pagetablemmu"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/mgpusim/v5/amd/benchmarks"
	"github.com/sarchlab/mgpusim/v5/amd/driver"
	rt "github.com/sarchlab/mgpusim/v5/amd/runtime"
)

// tinyQueue makes the driver take one fault at a time, so that faults that
// arrive together find the queue full.
func tinyQueue(s *driver.PageMigrationSpec) {
	s.QueueSize = 1
	s.BatchSize = 1
}

// The queue of the driver is full of a fault that waits for a memory copy,
// and a kernel keeps faulting on more pages. None of the faults may be lost:
// a lost fault leaves its access waiting forever, and the queue of the
// kernel stalls. The faults that find the queue full stay with the MMU.
func TestEveryFaultIsServicedWhenTheQueueIsFull(t *testing.T) {
	for _, gpuType := range platforms {
		t.Run(gpuType, func(t *testing.T) {
			s := newSystem(t, gpuType, migration(tinyQueue))

			// The kernel of GPU 2 reads across the page boundaries of the
			// buffer: 16 pages on R9 Nano, 2 pages of 2 MiB on MI300X. The
			// host reads a larger part of the buffer at the same time.
			const (
				floats     = 16 * 1024
				hostFloats = 64 * 1024
			)

			pages := max(2, floats*floatBytes/s.pageSize)
			shared := s.unified(max(2*s.pageSize, hostFloats*floatBytes))

			if pages == 2 {
				shared = s.at(shared, 1, 0) - floats*floatBytes/2
			}

			result := s.local(2, floats*floatBytes)
			data := ramp(1, hostFloats)
			s.upload(shared, data)

			// Start the prepared host read at GPU 2's first walk for the
			// shared input. Timed uploads and mapping setup can otherwise
			// let the copy finish before the kernel's first instruction.
			host := s.rt.AllocateHostBuffer(s.queues[0], hostFloats*floatBytes)
			s.touch(2, result, shared, floats)
			s.readDuringWalk(shared, floats*floatBytes, host)

			fromHost := make([]float32, hostFloats)
			if err := benchmarks.Read(s.rt, host, fromHost); err != nil {
				t.Fatal(err)
			}

			mustEqualFloats(t, "what the host read", fromHost, data)
			mustEqualFloats(t, "what GPU 2 read", s.download(result, floats), data[:floats])

			for _, owner := range s.owners(shared, pages) {
				if owner != 2 {
					t.Fatalf("owners %v", s.owners(shared, pages))
				}
			}

			st := s.stats()
			if st.DeferredFaults == 0 || st.QueueFullWaits == 0 || st.PagesMigrated < pages {
				t.Fatalf("no fault waited for the copy with the queue full: %+v", st)
			}

			t.Logf("%s: %+v", gpuType, st)
		})
	}
}

func (s *system) readDuringWalk(shared rt.Ptr, kernelBytes uint64, host rt.HostBuffer) {
	s.t.Helper()
	var gpu *pagetablemmu.Comp
	for _, component := range s.sim.Components() {
		if c, ok := component.(*pagetablemmu.Comp); ok && c.Name() == "GPU[2].GMMU" {
			gpu = c
			break
		}
	}
	sent := false
	s.runWatching(func() {
		if sent {
			return
		}
		for _, w := range gpu.State.Walks {
			va := w.VPN * s.pageSize
			if len(w.Waiters) > 0 && w.PID == s.pid() && va+s.pageSize > uint64(shared) && va < uint64(shared)+kernelBytes {
				s.rt.EnqueueMemCopyD2H(s.queues[0], host.Address, shared, host.ByteSize)
				s.start()
				sent = true
				return
			}
		}
	})
	if !sent {
		s.t.Fatal("GPU 2 never walked the shared input")
	}
}

// A fault that arrives while its page is being released is kept, and is
// serviced only after the last Release answer of the page.
//
//nolint:funlen // Keep the injection during the Release and what follows together.
func TestAFaultDuringTheReleaseIsKept(t *testing.T) {
	for _, gpuType := range platforms {
		t.Run(gpuType, func(t *testing.T) {
			s := newSystem(t, gpuType, migration(func(s *driver.PageMigrationSpec) {
				s.PrefetchThreshold = 0
			}))

			const floats = 4 * groupFloats

			data := ramp(1, floats)
			shared := s.unified(s.pageSize)
			result := s.local(2, floats*floatBytes)
			s.upload(shared, data)

			page := s.page(shared)
			w := watch(s, 2)
			injected, keptDuringRelease := false, false

			s.touch(2, result, shared, floats)
			s.runWatching(func() {
				w.sample()

				// The shootdown has just ended, so the driver has sent the
				// Release of the migration: another translator of GPU 2
				// reports a fault now.
				if !injected {
					if s.stats().ShootdownTimeSum != 0 {
						s.injectFault(page, 2)
						injected = true
					}

					return
				}

				// The driver has the fault while Release answers are still
				// out.
				migrating := s.driver.State.PageMigration.Pages[0]
				if s.stats().Migrations == 1 && s.stats().Releases == 0 &&
					migrating.PendingAcks > 0 && slices.Contains(migrating.Waiters, 2) {
					keptDuringRelease = true
				}
			})

			if !injected || !keptDuringRelease {
				t.Fatalf("no fault was kept during the Release (injected: %v)", injected)
			}

			if len(w.overlaps) != 0 {
				t.Fatal("the page was held again before its last Release answer")
			}

			// The fault was serviced afterwards: GPU 2 has the page, so it
			// was held and released once more.
			st := s.stats()
			if st.Migrations != 1 || st.Releases == 0 || st.RetainedFaults == 0 {
				t.Fatalf("stats %+v", st)
			}

			if w.held[page.VAddr].last <= w.released[page.VAddr].first {
				t.Fatalf("the kept fault was not serviced after the Release: %+v, %+v",
					w.held[page.VAddr], w.released[page.VAddr])
			}

			mustEqualFloats(t, "what GPU 2 read", s.download(result, floats), data)
			t.Logf("%s: %+v", gpuType, st)
		})
	}
}

// injectFault delivers the demand fault of a GPU on a page to the driver, as
// the MMU does for a translator whose walk faults.
func (s *system) injectFault(page vm.Page, gpu uint64) {
	port := s.driver.Ports.MigrationNotification
	port.Deliver(messaging.Msg{
		ID:  s.sim.NewID(),
		Src: "MMU.Notification[0]",
		Dst: port.AsRemote(),
		Payload: addrtransprotocol.PageFault{
			PID: page.PID, VAddr: page.VAddr, DeviceID: gpu, Page: page,
		},
	})
}
