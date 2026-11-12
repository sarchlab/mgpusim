package pagemigration

import (
	"testing"
)

// A page that a kernel has written holds its newest lines only in the L2 of
// its GPU. The migration must write them back before the copy, which reads
// the memory below the L2.
func TestMigrationCopiesTheDirtyLinesOfTheSource(t *testing.T) {
	for _, gpuType := range platforms {
		t.Run(gpuType, func(t *testing.T) {
			s := newSystem(t, gpuType)

			const floats = 1024
			data := ramp(1, floats)

			// GPU 1 writes the unified buffer with a kernel.
			staging := s.local(1, floats*floatBytes)
			shared := s.unified(floats * floatBytes)
			s.upload(staging, data)
			s.touch(1, shared, staging, floats)
			s.run()

			// The lines are dirty in the L2 of GPU 1: its memory does not
			// have the data yet.
			frame := s.page(shared)
			if !s.dirtyInL2(frame.PAddr, floats*floatBytes) {
				t.Fatal("the kernel output is in memory already; the test needs dirty L2 lines")
			}

			// GPU 2 reads the buffer: the page faults and migrates.
			result := s.local(2, floats*floatBytes)
			s.touch(2, result, shared, floats)
			s.run()

			moved := s.page(shared)
			if moved.DeviceID != 2 || moved.PAddr == frame.PAddr {
				t.Fatalf("the page did not migrate: %+v", moved)
			}

			mustEqualFloats(t, "what GPU 2 read", s.download(result, floats), data)
			mustEqualFloats(t, "the migrated buffer", s.download(shared, floats), data)

			st := s.stats()
			if st.Faults == 0 || st.Migrations == 0 || st.FramesRetired != st.PagesMigrated {
				t.Fatalf("stats %+v", st)
			}

			t.Logf("%s: %+v", gpuType, st)
		})
	}
}

// With the buddy allocator, the frames of a freed buffer are handed out
// again at once. The L2 of the destination still holds dirty lines of the
// freed buffer; a page that migrates into such a frame must not meet them,
// and they must not be written back over it.
func TestMigrationIntoAFrameThatWasUsedBefore(t *testing.T) {
	for _, gpuType := range platforms {
		t.Run(gpuType, func(t *testing.T) {
			s := newSystem(t, gpuType, buddyAllocator)

			const floats = 1024

			// GPU 2 writes junk into a page of its own with a kernel, so the
			// lines stay dirty in its L2, and the page is freed.
			junkSource := s.local(2, floats*floatBytes)
			junk := s.local(2, s.pageSize)
			s.upload(junkSource, ramp(-1e6, floats))
			s.touch(2, junk, junkSource, floats)
			s.run()

			freed := s.page(junk).PAddr
			if !s.dirtyInL2(freed, floats*floatBytes) {
				t.Fatal("the junk is in memory already; the test needs dirty L2 lines")
			}

			// A unified page on GPU 1 with data in the same place. GPU 2
			// reads it with the next kernel, so the page migrates before any
			// cache is flushed. The junk page is freed after everything else
			// that GPU 2 needs is allocated, so its frame goes to the page
			// that migrates.
			shared := s.unified(s.pageSize)
			result := s.local(2, floats*floatBytes)
			data := ramp(1, floats)

			s.upload(shared, data)
			s.touch(2, result, shared, floats)
			s.padBuddy(2, 2)
			s.free(junk)
			s.run()

			if page := s.page(shared); page.DeviceID != 2 || page.PAddr != freed {
				t.Fatalf("the page is in frame %#x of GPU %d, want the freed frame %#x of GPU 2",
					page.PAddr, page.DeviceID, freed)
			}

			mustEqualFloats(t, "what GPU 2 read", s.download(result, floats), data)
			mustEqualFloats(t, "the migrated page", s.download(shared, floats), data)
			t.Logf("%s: %+v", gpuType, s.stats())
		})
	}
}
